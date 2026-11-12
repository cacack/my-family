<script lang="ts">
	import { untrack } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { page } from '$app/state';
	import {
		api,
		type ValidationIssue,
		type DuplicatePair,
		type DismissedDuplicate
	} from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import * as Tabs from '#lib/components/ui/tabs/index.js';
	import SeverityBadge from '#lib/components/SeverityBadge.svelte';
	import MainlineNotice from '#lib/components/MainlineNotice.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { clampPageToTotal, readEnum, readPositiveInt, setQuery } from '#lib/utils/urlState.js';

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
		'whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-muted-foreground';
	const TD_CLASS = 'border-b border-slate-100 px-4 py-3 text-foreground';
	const TD_NOWRAP = `${TD_CLASS} whitespace-nowrap font-medium`;
	const TD_CENTER = `${TD_CLASS} text-center`;
	const ROW_HOVER = 'transition-colors hover:bg-slate-50';
	const SUBJECT_LINK = 'text-link no-underline hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500';

	// The active tab, its page and the severity filter live in the URL (see
	// urlState.ts), so Back, reload and a copied link restore the view. `page`
	// belongs to the active tab: switching tabs starts the new one at page 1
	// and drops the severity filter. Tab and page changes push; the filter
	// refines the view, so it replaces.
	// `not-duplicates` lists the pairs marked "Not a duplicate" (#1036), where
	// they can be restored.
	const TABS = ['validation', 'duplicates', 'not-duplicates'] as const;
	const SEVERITIES = ['all', 'error', 'warning', 'info'] as const;
	const DEFAULTS = { tab: 'validation', page: 1, severity: 'all' } as const;

	const params = $derived(page.url.searchParams);
	const activeTab = $derived(readEnum(params, 'tab', TABS, DEFAULTS.tab));
	const currentPage = $derived(readPositiveInt(params, 'page', DEFAULTS.page));
	const severityFilter = $derived(
		activeTab === 'validation' ? readEnum(params, 'severity', SEVERITIES, DEFAULTS.severity) : DEFAULTS.severity
	);

	function selectTab(tab: string) {
		setQuery(page.url, { tab, page: null, severity: null }, { push: true, defaults: DEFAULTS });
	}

	function goToPage(n: number) {
		setQuery(page.url, { page: n }, { push: true, defaults: DEFAULTS });
	}

	function selectSeverity(severity: (typeof SEVERITIES)[number]) {
		setQuery(page.url, { severity, page: null }, { defaults: DEFAULTS });
	}

	// --- Validation tab state ---
	let issues: ValidationIssue[] = $state([]);
	let validationTotal = $state(0); // total matching the active severity filter
	let errorCount = $state(0);
	let warningCount = $state(0);
	let infoCount = $state(0);
	let validationLoading = $state(false);
	let validationError: string | null = $state(null);

	// --- Duplicates tab state ---
	let duplicates: DuplicatePair[] = $state([]);
	let duplicatesTotal = $state(0);
	let duplicatesLoading = $state(false);
	let duplicatesError: string | null = $state(null);
	let dismissError: string | null = $state(null);
	let dismissBusy = $state(false);
	// Per-row "Dismissing…" label target. dismissBusy gates ALL dismiss buttons
	// (per-row + bulk) to prevent overlapping calls; dismissingKey just tells us
	// which row to relabel.
	let dismissingKey: string | null = $state(null);
	const selectedKeys: SvelteSet<string> = new SvelteSet();

	// --- Not duplicates tab state (#1036) ---
	let dismissed: DismissedDuplicate[] = $state([]);
	let dismissedTotal = $state(0);
	let dismissedLoading = $state(false);
	let dismissedError: string | null = $state(null);
	let restoreError: string | null = $state(null);
	// The pair being restored; one restore at a time.
	let restoringKey: string | null = $state(null);

	// --- Helpers ---
	const RECORD_ROUTES: Record<NonNullable<ValidationIssue['record_type']>, string> = {
		person: 'persons',
		family: 'families',
		source: 'sources'
	};

	/** The page of the record an issue is about; a record of unknown kind is taken for a person. */
	function recordHref(issue: ValidationIssue): string {
		return `/${RECORD_ROUTES[issue.record_type ?? 'person']}/${issue.record_id}`;
	}

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

	// Tab switches, Back and paging can start loads faster than they finish:
	// only each tab's latest load may land.
	let validationSeq = 0;
	let duplicatesSeq = 0;
	let dismissedSeq = 0;

	async function loadValidationIssues() {
		const seq = ++validationSeq;
		validationLoading = true;
		validationError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.getValidationIssues({
				severity: severityFilter === 'all' ? undefined : severityFilter,
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize
			});
			if (seq !== validationSeq) return;
			if (clampPageToTotal(page.url, requestedPage, result.total, pageSize)) return;
			issues = result.issues;
			validationTotal = result.total;
			// Counts always reflect the full unfiltered set from the backend, so
			// they remain stable regardless of the active filter or page.
			errorCount = result.error_count;
			warningCount = result.warning_count;
			infoCount = result.info_count;
		} catch (e) {
			if (seq !== validationSeq) return;
			validationError = truncateError(e, 'Failed to load validation issues');
		} finally {
			if (seq === validationSeq) validationLoading = false;
		}
	}

	async function loadDuplicates() {
		const seq = ++duplicatesSeq;
		duplicatesLoading = true;
		duplicatesError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.getPersonsDuplicates({
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize
			});
			if (seq !== duplicatesSeq) return;
			if (clampPageToTotal(page.url, requestedPage, result.total, pageSize)) return;
			duplicates = result.duplicates;
			duplicatesTotal = result.total;
		} catch (e) {
			if (seq !== duplicatesSeq) return;
			duplicatesError = truncateError(e, 'Failed to load duplicates');
		} finally {
			if (seq === duplicatesSeq) duplicatesLoading = false;
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
			const response = await api.batchDismissDuplicates({ dismissals });
			// Each pair is its own command and can fail alone: keep the failed
			// pairs selected and say so, rather than letting them silently reappear.
			const failed = response.results.filter((r) => !r.success);
			clearSelection();
			for (const r of failed) selectedKeys.add(pairKey(r.person1_id, r.person2_id));
			if (failed.length > 0) {
				const reasons = [...new Set(failed.map((r) => r.error).filter(Boolean))].join('; ');
				dismissError =
					`${failed.length} of ${response.total} pairs could not be marked not duplicates` +
					(reasons ? `: ${reasons}` : '');
			}
			await loadDuplicates();
		} catch (e) {
			dismissError = truncateError(e, 'Failed to dismiss selected duplicates');
		} finally {
			dismissBusy = false;
		}
	}

	async function loadDismissed() {
		const seq = ++dismissedSeq;
		dismissedLoading = true;
		dismissedError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.listDismissedDuplicates({
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize
			});
			if (seq !== dismissedSeq) return;
			if (clampPageToTotal(page.url, requestedPage, result.total, pageSize)) return;
			dismissed = result.dismissals;
			dismissedTotal = result.total;
		} catch (e) {
			if (seq !== dismissedSeq) return;
			dismissedError = truncateError(e, 'Failed to load pairs marked not a duplicate');
		} finally {
			if (seq === dismissedSeq) dismissedLoading = false;
		}
	}

	async function restoreOne(p1: string, p2: string) {
		if (restoringKey) return;
		restoreError = null;
		restoringKey = pairKey(p1, p2);
		try {
			await api.restoreDuplicate(p1, p2);
			await loadDismissed();
		} catch (e) {
			restoreError = truncateError(e, 'Failed to restore the pair');
		} finally {
			restoringKey = null;
		}
	}

	function formatDismissedAt(iso: string): string {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString();
	}

	// Load the active tab whenever the URL changes its tab, page or filter.
	$effect(() => {
		const tab = activeTab;
		void currentPage;
		void severityFilter;
		untrack(() => {
			if (tab === 'validation') {
				loadValidationIssues();
			} else if (tab === 'duplicates') {
				loadDuplicates();
			} else {
				loadDismissed();
			}
		});
	});

	// Derived values
	const totalIssuesCount = $derived(errorCount + warningCount + infoCount);
	const validationTotalPages = $derived(Math.max(1, Math.ceil(validationTotal / pageSize)));
	const duplicatesTotalPages = $derived(Math.max(1, Math.ceil(duplicatesTotal / pageSize)));
	const dismissedTotalPages = $derived(Math.max(1, Math.ceil(dismissedTotal / pageSize)));

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

<div class="mx-auto max-w-page-list p-6">
	<MainlineNotice
		message="On a research branch, duplicate pairs and the pairs marked not a duplicate follow your branch, but validation issues still come from the mainline."
	/>
	<PageHeader
		title="Quality"
		description="Find and fix data quality issues and potential duplicate persons."
	/>

	<Tabs.Root value={activeTab} onValueChange={selectTab}>
		<!-- Wraps rather than scrolls, so every tab stays visible on a phone. -->
		<Tabs.List class="max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto">
			<Tabs.Trigger value="validation">
				Validation Issues
				{#if totalIssuesCount > 0}
					<span class="ml-1 text-xs text-muted-foreground">({totalIssuesCount})</span>
				{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="duplicates">
				Duplicates
				{#if duplicatesTotal > 0}
					<span class="ml-1 text-xs text-muted-foreground">({duplicatesTotal})</span>
				{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="not-duplicates">
				Not duplicates
				{#if dismissedTotal > 0}
					<span class="ml-1 text-xs text-muted-foreground">({dismissedTotal})</span>
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
							onclick={() => selectSeverity(filter.key)}
						>
							{filter.label} ({filter.count})
						</Button>
					{/each}
				</div>
				<div class="flex items-center gap-3">
					{#if validationLoading}
						<div class="flex items-center gap-2" role="status" aria-live="polite">
							<svg
								class="size-4 animate-spin text-muted-foreground"
								viewBox="0 0 24 24"
								fill="none"
								aria-hidden="true"
							>
								<circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" stroke-opacity="0.25" />
								<path d="M22 12a10 10 0 0 1-10 10" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
							</svg>
							<span class="text-sm text-muted-foreground">Scanning…</span>
						</div>
					{/if}
					<Button variant="outline" size="sm" onclick={loadValidationIssues} disabled={validationLoading}>
						Scan now
					</Button>
				</div>
			</div>

			{#if validationLoading}
				<div class="p-12 text-center text-muted-foreground">Loading validation issues...</div>
			{:else if validationError}
				<ErrorState message={validationError} onRetry={loadValidationIssues} />
			{:else if issues.length === 0}
				<div class="p-12 text-center text-muted-foreground">
					<p class="m-0 mb-2">No issues found at this severity.</p>
					<p class="text-[0.8125rem] text-muted-foreground">
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
									<td class="{TD_CLASS} hidden font-mono text-xs text-muted-foreground sm:table-cell">
										{issue.code}
									</td>
									<td class={TD_CLASS}>{issue.message}</td>
									<td class={TD_CLASS}>
										{#if issue.record_id}
											<a href={recordHref(issue)} class={SUBJECT_LINK}>
												{issue.record_name || 'Unnamed'}
											</a>
										{:else}
											<span class="text-muted-foreground">—</span>
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
								if (currentPage > 1) goToPage(currentPage - 1);
							}}
							disabled={currentPage === 1 || validationLoading}
						>
							Previous
						</Button>
						<span class="text-sm text-muted-foreground">
							Page {currentPage} of {validationTotalPages}
						</span>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								if (currentPage < validationTotalPages) goToPage(currentPage + 1);
							}}
							disabled={currentPage >= validationTotalPages || validationLoading}
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
				<span class="text-sm text-muted-foreground">
					{duplicatesTotal}
					{duplicatesTotal === 1 ? 'potential duplicate' : 'potential duplicates'}
				</span>
				<span class="text-[0.8125rem] text-muted-foreground">
					Mark a pair <strong>Not a duplicate</strong> when the two records are different people
					(say, a father and son); it stays off this list and can be restored from Not duplicates.
				</span>
			</div>

			{#if selectedKeys.size > 0}
				<div
					class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-md border border-slate-200 bg-slate-50 px-4 py-3"
				>
					<span class="text-sm font-medium text-foreground">
						{selectedKeys.size} selected
					</span>
					<div class="flex gap-2">
						<Button variant="outline" size="sm" onclick={clearSelection} disabled={dismissBusy}>
							Clear
						</Button>
						<Button size="sm" onclick={dismissSelected} disabled={dismissBusy}>
							{dismissBusy ? 'Saving…' : 'Mark selected not duplicates'}
						</Button>
					</div>
				</div>
			{/if}

			{#if dismissError}
				<ActionError message={dismissError} />
			{/if}

			{#if duplicatesLoading}
				<div class="p-12 text-center text-muted-foreground">Loading duplicates...</div>
			{:else if duplicatesError}
				<ErrorState message={duplicatesError} onRetry={loadDuplicates} />
			{:else if duplicates.length === 0}
				<div class="p-12 text-center text-muted-foreground">
					<p class="m-0 mb-2">No potential duplicates detected.</p>
					<p class="text-[0.8125rem] text-muted-foreground">
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
								<tr class={ROW_HOVER}>
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
												aria-label="Mark {pair.person1_name} and {pair.person2_name} as not a duplicate"
												onclick={() => dismissOne(pair.person1_id, pair.person2_id)}
												disabled={dismissBusy}
											>
												{dismissingKey === key ? 'Saving…' : 'Not a duplicate'}
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
								if (currentPage > 1) goToPage(currentPage - 1);
							}}
							disabled={currentPage === 1 || duplicatesLoading}
						>
							Previous
						</Button>
						<span class="text-sm text-muted-foreground">
							Page {currentPage} of {duplicatesTotalPages}
						</span>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								if (currentPage < duplicatesTotalPages) goToPage(currentPage + 1);
							}}
							disabled={currentPage >= duplicatesTotalPages || duplicatesLoading}
						>
							Next
						</Button>
					</div>
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Not duplicates Tab (#1036) -->
		<Tabs.Content value="not-duplicates">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<span class="text-sm text-muted-foreground">
					{dismissedTotal}
					{dismissedTotal === 1 ? 'pair' : 'pairs'} marked not a duplicate
				</span>
			</div>

			{#if restoreError}
				<ActionError message={restoreError} />
			{/if}

			{#if dismissedLoading}
				<div class="p-12 text-center text-muted-foreground">Loading…</div>
			{:else if dismissedError}
				<ErrorState message={dismissedError} onRetry={loadDismissed} />
			{:else if dismissed.length === 0}
				<div class="p-12 text-center text-muted-foreground">
					<p class="m-0 mb-2">No pairs are marked not a duplicate.</p>
					<p class="text-[0.8125rem] text-muted-foreground">
						Pairs you mark "Not a duplicate" on the Duplicates tab are listed here.
					</p>
				</div>
			{:else}
				<div class="overflow-x-auto">
					<table class="w-full border-collapse text-sm">
						<thead>
							<tr>
								<th class={TH_CLASS}>Person 1</th>
								<th class={TH_CLASS}>Person 2</th>
								<th class="{TH_CLASS} hidden sm:table-cell">Reason</th>
								<th class="{TH_CLASS} hidden sm:table-cell">Marked</th>
								<th class={TH_CLASS}>Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each dismissed as pair (pairKey(pair.person1_id, pair.person2_id))}
								{@const key = pairKey(pair.person1_id, pair.person2_id)}
								<tr>
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
									<td class="{TD_CLASS} hidden sm:table-cell">
										{#if pair.reason}
											{pair.reason}
										{:else}
											<span class="text-muted-foreground">—</span>
										{/if}
									</td>
									<td class="{TD_NOWRAP} hidden sm:table-cell">
										{formatDismissedAt(pair.dismissed_at)}
									</td>
									<td class={TD_NOWRAP}>
										<Button
											variant="outline"
											size="sm"
											aria-label="Restore {pair.person1_name} and {pair.person2_name} as a possible duplicate"
											onclick={() => restoreOne(pair.person1_id, pair.person2_id)}
											disabled={restoringKey !== null}
										>
											{restoringKey === key ? 'Restoring…' : 'Restore'}
										</Button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				{#if dismissedTotalPages > 1}
					<div
						class="mt-8 flex items-center justify-center gap-4 border-t border-slate-200 pt-4"
					>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								if (currentPage > 1) goToPage(currentPage - 1);
							}}
							disabled={currentPage === 1 || dismissedLoading}
						>
							Previous
						</Button>
						<span class="text-sm text-muted-foreground">
							Page {currentPage} of {dismissedTotalPages}
						</span>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								if (currentPage < dismissedTotalPages) goToPage(currentPage + 1);
							}}
							disabled={currentPage >= dismissedTotalPages || dismissedLoading}
						>
							Next
						</Button>
					</div>
				{/if}
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>
