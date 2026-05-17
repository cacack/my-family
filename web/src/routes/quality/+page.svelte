<script lang="ts">
	import { untrack } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import {
		api,
		type ValidationIssue,
		type DuplicatePair
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Tabs from '$lib/components/ui/tabs';
	import SeverityBadge from '$lib/components/SeverityBadge.svelte';

	const pageSize = 20;
	const ERROR_MESSAGE_MAX_LEN = 200;
	// Separator for serializing (person1_id, person2_id) into a single Set key.
	// `::` is illegal in a UUID, so the round-trip is unambiguous and parsePairKey
	// can hard-assert exactly two segments.
	const PAIR_KEY_SEPARATOR = '::';

	// Shared class strings for the desktop tables. Adapted from the evidence
	// page (post-#392 pattern). A future refactor can lift these into a shared
	// module — kept inline here to limit PR scope.
	const TH_CLASS =
		'whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600';
	const TD_CLASS = 'border-b border-slate-100 px-4 py-3 text-slate-800';
	const TD_NOWRAP = `${TD_CLASS} whitespace-nowrap font-medium`;
	const TD_CENTER = `${TD_CLASS} text-center`;
	const ROW_CLICKABLE = 'cursor-pointer transition-colors hover:bg-slate-50';
	const SUBJECT_LINK = 'text-blue-500 no-underline hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500';

	// Active tab
	let activeTab = $state('validation');

	// --- Validation tab state ---
	let issues: ValidationIssue[] = $state([]);
	let validationTotal = $state(0); // total matching the active severity filter
	let validationPage = $state(1);
	let errorCount = $state(0);
	let warningCount = $state(0);
	let infoCount = $state(0);
	let validationLoading = $state(false);
	let validationError: string | null = $state(null);
	let severityFilter = $state<'all' | 'error' | 'warning' | 'info'>('all');

	// --- Duplicates tab state ---
	let duplicates: DuplicatePair[] = $state([]);
	let duplicatesTotal = $state(0);
	let duplicatesPage = $state(1);
	let duplicatesLoading = $state(false);
	let duplicatesError: string | null = $state(null);
	let dismissError: string | null = $state(null);
	let dismissBusy = $state(false);
	// Per-row "Dismissing…" label target. dismissBusy gates ALL dismiss buttons
	// (per-row + bulk) to prevent overlapping calls; dismissingKey just tells us
	// which row to relabel.
	let dismissingKey: string | null = $state(null);
	const selectedKeys: SvelteSet<string> = new SvelteSet();

	// --- Helpers ---
	function pairKey(p1: string, p2: string): string {
		return `${p1}${PAIR_KEY_SEPARATOR}${p2}`;
	}

	function parsePairKey(k: string): [string, string] {
		const parts = k.split(PAIR_KEY_SEPARATOR);
		if (parts.length !== 2 || !parts[0] || !parts[1]) {
			throw new Error(`malformed pair key: ${k}`);
		}
		return [parts[0], parts[1]];
	}

	function truncateError(msg: unknown, fallback: string): string {
		const raw =
			typeof msg === 'string' && msg
				? msg
				: (msg as { message?: string } | null | undefined)?.message || fallback;
		return raw.length > ERROR_MESSAGE_MAX_LEN
			? `${raw.slice(0, ERROR_MESSAGE_MAX_LEN)}…`
			: raw;
	}

	function toggleRowSelection(p1: string, p2: string) {
		const k = pairKey(p1, p2);
		if (selectedKeys.has(k)) {
			selectedKeys.delete(k);
		} else {
			selectedKeys.add(k);
		}
	}

	function togglePageSelection() {
		const pageKeys = duplicates.map((d) => pairKey(d.person1_id, d.person2_id));
		const allSelected = pageKeys.length > 0 && pageKeys.every((k) => selectedKeys.has(k));
		if (allSelected) {
			for (const k of pageKeys) selectedKeys.delete(k);
		} else {
			for (const k of pageKeys) selectedKeys.add(k);
		}
	}

	function clearSelection() {
		selectedKeys.clear();
	}

	// --- Data loading ---

	async function loadValidationIssues() {
		validationLoading = true;
		validationError = null;
		try {
			const result = await api.getValidationIssues({
				severity: severityFilter === 'all' ? undefined : severityFilter,
				limit: pageSize,
				offset: (validationPage - 1) * pageSize
			});
			issues = result.issues;
			validationTotal = result.total;
			// Counts always reflect the full unfiltered set from the backend, so
			// they remain stable regardless of the active filter or page.
			errorCount = result.error_count;
			warningCount = result.warning_count;
			infoCount = result.info_count;
		} catch (e) {
			validationError = truncateError(e, 'Failed to load validation issues');
		} finally {
			validationLoading = false;
		}
	}

	async function loadDuplicates() {
		duplicatesLoading = true;
		duplicatesError = null;
		try {
			const result = await api.getPersonsDuplicates({
				limit: pageSize,
				offset: (duplicatesPage - 1) * pageSize
			});
			duplicates = result.duplicates;
			duplicatesTotal = result.total;
		} catch (e) {
			duplicatesError = truncateError(e, 'Failed to load duplicates');
		} finally {
			duplicatesLoading = false;
		}
	}

	async function dismissOne(p1: string, p2: string) {
		if (dismissBusy) return;
		dismissError = null;
		dismissBusy = true;
		dismissingKey = pairKey(p1, p2);
		try {
			await api.dismissDuplicate(p1, p2);
			selectedKeys.delete(pairKey(p1, p2));
			await loadDuplicates();
		} catch (e) {
			dismissError = truncateError(e, 'Failed to dismiss duplicate');
		} finally {
			dismissBusy = false;
			dismissingKey = null;
		}
	}

	async function dismissSelected() {
		if (selectedKeys.size === 0 || dismissBusy) return;
		dismissError = null;
		dismissBusy = true;
		try {
			const dismissals = [...selectedKeys].map((k) => {
				const [person1_id, person2_id] = parsePairKey(k);
				return { person1_id, person2_id };
			});
			await api.batchDismissDuplicates({ dismissals });
			clearSelection();
			await loadDuplicates();
		} catch (e) {
			dismissError = truncateError(e, 'Failed to dismiss selected duplicates');
		} finally {
			dismissBusy = false;
		}
	}

	// Load data when active tab changes. Switching tabs resets the active
	// pagination so a stale page number from a previous visit doesn't render
	// an empty list.
	$effect(() => {
		const tab = activeTab;
		untrack(() => {
			if (tab === 'validation') {
				validationPage = 1;
				loadValidationIssues();
			} else if (tab === 'duplicates') {
				duplicatesPage = 1;
				loadDuplicates();
			}
		});
	});

	// Derived values
	const totalIssuesCount = $derived(errorCount + warningCount + infoCount);
	const validationTotalPages = $derived(Math.max(1, Math.ceil(validationTotal / pageSize)));
	const duplicatesTotalPages = $derived(Math.ceil(duplicatesTotal / pageSize));

	const filterPills = $derived([
		{ key: 'all' as const, label: 'All', count: totalIssuesCount },
		{ key: 'error' as const, label: 'Errors', count: errorCount },
		{ key: 'warning' as const, label: 'Warnings', count: warningCount },
		{ key: 'info' as const, label: 'Info', count: infoCount }
	]);

	const pageAllSelected = $derived(
		duplicates.length > 0 &&
			duplicates.every((d) => selectedKeys.has(pairKey(d.person1_id, d.person2_id)))
	);
</script>

<svelte:head>
	<title>Quality | My Family</title>
</svelte:head>

<div class="mx-auto max-w-screen-xl p-6">
	<header class="mb-6">
		<div>
			<h1 class="m-0 text-2xl text-slate-800">Quality</h1>
			<p class="mt-1 text-sm text-slate-500">
				Find and fix data quality issues and potential duplicate persons.
			</p>
		</div>
	</header>

	<Tabs.Root bind:value={activeTab}>
		<Tabs.List>
			<Tabs.Trigger value="validation">
				Validation Issues
				{#if totalIssuesCount > 0}
					<span class="ml-1 text-xs text-slate-500">({totalIssuesCount})</span>
				{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="duplicates">
				Duplicates
				{#if duplicatesTotal > 0}
					<span class="ml-1 text-xs text-slate-500">({duplicatesTotal})</span>
				{/if}
			</Tabs.Trigger>
		</Tabs.List>

		<!-- Validation Issues Tab -->
		<Tabs.Content value="validation">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<div class="flex flex-wrap gap-1">
					{#each filterPills as filter}
						<Button
							variant={severityFilter === filter.key ? 'default' : 'outline'}
							size="sm"
							aria-pressed={severityFilter === filter.key}
							onclick={() => {
								severityFilter = filter.key;
								validationPage = 1;
								loadValidationIssues();
							}}
						>
							{filter.label} ({filter.count})
						</Button>
					{/each}
				</div>
				<div class="flex items-center gap-3">
					{#if validationLoading}
						<div class="flex items-center gap-2" role="status" aria-live="polite">
							<svg
								class="size-4 animate-spin text-slate-500"
								viewBox="0 0 24 24"
								fill="none"
								aria-hidden="true"
							>
								<circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" stroke-opacity="0.25" />
								<path d="M22 12a10 10 0 0 1-10 10" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
							</svg>
							<span class="text-sm text-slate-500">Scanning…</span>
						</div>
					{/if}
					<Button variant="outline" size="sm" onclick={loadValidationIssues} disabled={validationLoading}>
						Scan now
					</Button>
				</div>
			</div>

			{#if validationLoading}
				<div class="p-12 text-center text-slate-500">Loading validation issues...</div>
			{:else if validationError}
				<div class="p-12 text-center text-red-600">
					<p class="m-0 mb-4">{validationError}</p>
					<Button variant="outline" onclick={loadValidationIssues}>Retry</Button>
				</div>
			{:else if issues.length === 0}
				<div class="p-12 text-center text-slate-500">
					<p class="m-0 mb-2">No issues found at this severity.</p>
					<p class="text-[0.8125rem] text-slate-400">
						Try a different severity filter or run a fresh scan.
					</p>
				</div>
			{:else}
				<div class="overflow-x-auto">
					<table class="w-full border-collapse text-sm">
						<thead>
							<tr>
								<th class={TH_CLASS}>Severity</th>
								<th class="{TH_CLASS} hidden sm:table-cell">Code</th>
								<th class={TH_CLASS}>Message</th>
								<th class={TH_CLASS}>Record</th>
							</tr>
						</thead>
						<tbody>
							{#each issues as issue}
								<tr>
									<td class={TD_NOWRAP}>
										<SeverityBadge severity={issue.severity} />
									</td>
									<td class="{TD_CLASS} hidden font-mono text-xs text-slate-500 sm:table-cell">
										{issue.code}
									</td>
									<td class={TD_CLASS}>{issue.message}</td>
									<td class={TD_CLASS}>
										{#if issue.record_id}
											<a href="/persons/{issue.record_id}" class={SUBJECT_LINK}>
												{issue.record_id.slice(0, 8)}...
											</a>
										{:else}
											<span class="text-slate-400">—</span>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				{#if validationTotalPages > 1}
					<div
						class="mt-8 flex items-center justify-center gap-4 border-t border-slate-200 pt-4"
					>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								if (validationPage > 1) {
									validationPage--;
									loadValidationIssues();
								}
							}}
							disabled={validationPage === 1 || validationLoading}
						>
							Previous
						</Button>
						<span class="text-sm text-slate-500">
							Page {validationPage} of {validationTotalPages}
						</span>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								if (validationPage < validationTotalPages) {
									validationPage++;
									loadValidationIssues();
								}
							}}
							disabled={validationPage >= validationTotalPages || validationLoading}
						>
							Next
						</Button>
					</div>
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Duplicates Tab -->
		<Tabs.Content value="duplicates">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<span class="text-sm text-slate-500">
					{duplicatesTotal}
					{duplicatesTotal === 1 ? 'potential duplicate' : 'potential duplicates'}
				</span>
			</div>

			{#if selectedKeys.size > 0}
				<div
					class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-md border border-slate-200 bg-slate-50 px-4 py-3"
				>
					<span class="text-sm font-medium text-slate-700">
						{selectedKeys.size} selected
					</span>
					<div class="flex gap-2">
						<Button variant="outline" size="sm" onclick={clearSelection} disabled={dismissBusy}>
							Clear
						</Button>
						<Button size="sm" onclick={dismissSelected} disabled={dismissBusy}>
							{dismissBusy ? 'Dismissing…' : 'Dismiss selected'}
						</Button>
					</div>
				</div>
			{/if}

			{#if dismissError}
				<div
					class="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
				>
					{dismissError}
				</div>
			{/if}

			{#if duplicatesLoading}
				<div class="p-12 text-center text-slate-500">Loading duplicates...</div>
			{:else if duplicatesError}
				<div class="p-12 text-center text-red-600">
					<p class="m-0 mb-4">{duplicatesError}</p>
					<Button variant="outline" onclick={loadDuplicates}>Retry</Button>
				</div>
			{:else if duplicates.length === 0}
				<div class="p-12 text-center text-slate-500">
					<p class="m-0 mb-2">No potential duplicates detected.</p>
					<p class="text-[0.8125rem] text-slate-400">
						The duplicate detector compares persons by name, dates, and places.
					</p>
				</div>
			{:else}
				<div class="overflow-x-auto">
					<table class="w-full border-collapse text-sm">
						<thead>
							<tr>
								<th class="{TH_CLASS} w-10">
									<Checkbox
										aria-label="Select all duplicates on this page"
										checked={pageAllSelected}
										onCheckedChange={() => togglePageSelection()}
									/>
								</th>
								<th class={TH_CLASS}>Person 1</th>
								<th class={TH_CLASS}>Person 2</th>
								<th class={TH_CLASS}>Confidence</th>
								<th class="{TH_CLASS} hidden sm:table-cell">Match Reasons</th>
								<th class={TH_CLASS}>Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each duplicates as pair}
								{@const key = pairKey(pair.person1_id, pair.person2_id)}
								{@const isSelected = selectedKeys.has(key)}
								{@const reasons = pair.match_reasons ?? []}
								{@const visibleReasons = reasons.slice(0, 3)}
								{@const extraReasonCount = Math.max(0, reasons.length - visibleReasons.length)}
								<tr class={ROW_CLICKABLE}>
									<td class={TD_CENTER}>
										<Checkbox
											aria-label="Select duplicate pair {pair.person1_name} and {pair.person2_name}"
											checked={isSelected}
											onCheckedChange={() => toggleRowSelection(pair.person1_id, pair.person2_id)}
										/>
									</td>
									<td class={TD_CLASS}>
										<a href="/persons/{pair.person1_id}" class={SUBJECT_LINK}>
											{pair.person1_name}
										</a>
									</td>
									<td class={TD_CLASS}>
										<a href="/persons/{pair.person2_id}" class={SUBJECT_LINK}>
											{pair.person2_name}
										</a>
									</td>
									<td class={TD_NOWRAP}>
										{(pair.confidence * 100).toFixed(0)}%
									</td>
									<td class="{TD_CLASS} hidden sm:table-cell">
										<div class="flex flex-wrap gap-1">
											{#each visibleReasons as reason}
												<Badge variant="secondary" class="text-[0.6875rem]">{reason}</Badge>
											{/each}
											{#if extraReasonCount > 0}
												<Badge variant="outline" class="text-[0.6875rem]">
													+{extraReasonCount} more
												</Badge>
											{/if}
										</div>
									</td>
									<td class={TD_NOWRAP}>
										<div class="flex gap-2">
											<Button
												variant="outline"
												size="sm"
												href="/quality/merge/{pair.person1_id}/{pair.person2_id}"
											>
												Compare / Merge
											</Button>
											<Button
												variant="ghost"
												size="sm"
												onclick={() => dismissOne(pair.person1_id, pair.person2_id)}
												disabled={dismissBusy}
											>
												{dismissingKey === key ? 'Dismissing…' : 'Dismiss'}
											</Button>
										</div>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				{#if duplicatesTotalPages > 1}
					<div
						class="mt-8 flex items-center justify-center gap-4 border-t border-slate-200 pt-4"
					>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								if (duplicatesPage > 1) {
									duplicatesPage--;
									loadDuplicates();
								}
							}}
							disabled={duplicatesPage === 1 || duplicatesLoading}
						>
							Previous
						</Button>
						<span class="text-sm text-slate-500">
							Page {duplicatesPage} of {duplicatesTotalPages}
						</span>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								if (duplicatesPage < duplicatesTotalPages) {
									duplicatesPage++;
									loadDuplicates();
								}
							}}
							disabled={duplicatesPage >= duplicatesTotalPages || duplicatesLoading}
						>
							Next
						</Button>
					</div>
				{/if}
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>
