<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		api,
		type EvidenceAnalysisResponse,
		type EvidenceConflictResponse,
		type ResearchLogResponse,
		type ProofSummaryResponse
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as Tabs from '$lib/components/ui/tabs';
	import UncertaintyBadge from '$lib/components/UncertaintyBadge.svelte';
	import { formatFactType, subjectRoute, formatDate } from '$lib/utils/evidence';

	const pageSize = 20;

	// Active tab
	let activeTab = $state('analyses');

	// Analyses state
	let analyses: EvidenceAnalysisResponse[] = $state([]);
	let analysesTotal = $state(0);
	let analysesPage = $state(1);
	let analysesLoading = $state(false);
	let analysesError: string | null = $state(null);

	// Conflicts state
	let conflicts: EvidenceConflictResponse[] = $state([]);
	let conflictsTotal = $state(0);
	let conflictsPage = $state(1);
	let conflictsLoading = $state(false);
	let conflictsError: string | null = $state(null);
	let conflictStatusFilter = $state<'all' | 'open' | 'resolved'>('all');
	let openConflictsCount = $state(0);
	let openConflictsLoaded = $state(false);

	// Research logs state
	let logs: ResearchLogResponse[] = $state([]);
	let logsTotal = $state(0);
	let logsPage = $state(1);
	let logsLoading = $state(false);
	let logsError: string | null = $state(null);

	// Proof summaries state
	let summaries: ProofSummaryResponse[] = $state([]);
	let summariesTotal = $state(0);
	let summariesPage = $state(1);
	let summariesLoading = $state(false);
	let summariesError: string | null = $state(null);

	function truncate(text: string, maxLen = 80): string {
		if (text.length <= maxLen) return text;
		return text.slice(0, maxLen) + '...';
	}

	// --- Data loading ---

	async function loadAnalyses() {
		analysesLoading = true;
		analysesError = null;
		try {
			const result = await api.listEvidenceAnalyses({
				limit: pageSize,
				offset: (analysesPage - 1) * pageSize
			});
			analyses = result.analyses;
			analysesTotal = result.total ?? 0;
		} catch (e) {
			analysesError = (e as { message?: string }).message || 'Failed to load analyses';
		} finally {
			analysesLoading = false;
		}
	}

	async function loadConflicts() {
		conflictsLoading = true;
		conflictsError = null;
		try {
			const statusParam = conflictStatusFilter === 'all' ? undefined : conflictStatusFilter;
			const result = await api.listEvidenceConflicts({
				limit: pageSize,
				offset: (conflictsPage - 1) * pageSize,
				status: statusParam as 'open' | 'resolved' | undefined
			});
			conflicts = result.conflicts;
			conflictsTotal = result.total ?? 0;

			// Get open count for badge
			if (conflictStatusFilter === 'open') {
				openConflictsCount = result.total;
				openConflictsLoaded = true;
			} else if (!openConflictsLoaded) {
				try {
					const openResult = await api.listEvidenceConflicts({ limit: 1, status: 'open' });
					openConflictsCount = openResult.total;
					openConflictsLoaded = true;
				} catch {
					// ignore - badge count is non-critical
				}
			}
		} catch (e) {
			conflictsError = (e as { message?: string }).message || 'Failed to load conflicts';
		} finally {
			conflictsLoading = false;
		}
	}

	async function loadLogs() {
		logsLoading = true;
		logsError = null;
		try {
			const result = await api.listResearchLogs({
				limit: pageSize,
				offset: (logsPage - 1) * pageSize
			});
			logs = result.logs;
			logsTotal = result.total ?? 0;
		} catch (e) {
			logsError = (e as { message?: string }).message || 'Failed to load research logs';
		} finally {
			logsLoading = false;
		}
	}

	async function loadSummaries() {
		summariesLoading = true;
		summariesError = null;
		try {
			const result = await api.listProofSummaries({
				limit: pageSize,
				offset: (summariesPage - 1) * pageSize
			});
			summaries = result.summaries;
			summariesTotal = result.total ?? 0;
		} catch (e) {
			summariesError = (e as { message?: string }).message || 'Failed to load proof summaries';
		} finally {
			summariesLoading = false;
		}
	}

	// Load data when tab changes
	$effect(() => {
		const tab = activeTab;
		untrack(() => {
			switch (tab) {
				case 'analyses': loadAnalyses(); break;
				case 'conflicts': loadConflicts(); break;
				case 'logs': loadLogs(); break;
				case 'summaries': loadSummaries(); break;
			}
		});
	});

	// Derived page counts
	const analysesTotalPages = $derived(Math.ceil(analysesTotal / pageSize));
	const conflictsTotalPages = $derived(Math.ceil(conflictsTotal / pageSize));
	const logsTotalPages = $derived(Math.ceil(logsTotal / pageSize));
	const summariesTotalPages = $derived(Math.ceil(summariesTotal / pageSize));
</script>

<svelte:head>
	<title>Evidence Analysis | My Family</title>
</svelte:head>

{#snippet pagination(currentPage: number, totalPages: number, loading: boolean, onPrev: () => void, onNext: () => void)}
	<div
		class="mt-8 flex items-center justify-center gap-4 border-t border-slate-200 pt-4"
	>
		<Button variant="outline" size="sm" onclick={onPrev} disabled={currentPage === 1 || loading}>
			Previous
		</Button>
		<span class="text-sm text-slate-500">Page {currentPage} of {totalPages}</span>
		<Button
			variant="outline"
			size="sm"
			onclick={onNext}
			disabled={currentPage >= totalPages || loading}
		>
			Next
		</Button>
	</div>
{/snippet}

<div class="mx-auto max-w-screen-xl p-6">
	<header class="mb-6">
		<div>
			<h1 class="m-0 text-2xl text-slate-800">Evidence Analysis</h1>
			<p class="mt-1 text-sm text-slate-500">
				GPS-compliant research tracking and proof management
			</p>
		</div>
	</header>

	<Tabs.Root bind:value={activeTab}>
		<Tabs.List>
			<Tabs.Trigger value="analyses">Analyses</Tabs.Trigger>
			<Tabs.Trigger value="conflicts">
				Conflicts
				{#if openConflictsCount > 0}
					<Badge variant="destructive" class="ml-1 h-5 min-w-5 px-1.5 text-xs">
						{openConflictsCount}
					</Badge>
				{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="logs">Research Logs</Tabs.Trigger>
			<Tabs.Trigger value="summaries">Proof Summaries</Tabs.Trigger>
		</Tabs.List>

		<!-- Analyses Tab -->
		<Tabs.Content value="analyses">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<span class="text-sm text-slate-500">
					{analysesTotal} {analysesTotal === 1 ? 'analysis' : 'analyses'}
				</span>
				<Button href="/evidence/analyses/new">New Analysis</Button>
			</div>

			{#if analysesLoading}
				<div class="p-12 text-center text-slate-500">Loading analyses...</div>
			{:else if analysesError}
				<div class="p-12 text-center text-red-600">
					<p class="m-0 mb-4">{analysesError}</p>
					<Button variant="outline" onclick={loadAnalyses}>Retry</Button>
				</div>
			{:else if analyses.length === 0}
				<div class="p-12 text-center text-slate-500">
					<p class="m-0 mb-2">No evidence analyses yet.</p>
					<p class="mb-4 text-[0.8125rem] text-slate-400">
						Create an analysis to evaluate evidence for a genealogical fact.
					</p>
					<Button href="/evidence/analyses/new">New Analysis</Button>
				</div>
			{:else}
				<!-- Desktop table -->
				<div class="hidden overflow-x-auto md:block">
					<table class="w-full border-collapse text-sm">
						<thead>
							<tr>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Fact Type</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Subject</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Conclusion</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Status</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Citations</th>
							</tr>
						</thead>
						<tbody>
							{#each analyses as analysis}
								<tr
									class="cursor-pointer transition-colors hover:bg-slate-50"
									tabindex="0"
									role="link"
									onclick={() => goto(`/evidence/analyses/${analysis.id}`)}
									onkeydown={(e) => {
										if (e.key === 'Enter' || e.key === ' ') {
											e.preventDefault();
											goto(`/evidence/analyses/${analysis.id}`);
										}
									}}
								>
									<td class="whitespace-nowrap border-b border-slate-100 px-4 py-3 font-medium text-slate-800">
										{formatFactType(analysis.fact_type)}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">
										<a
											href="/{subjectRoute(analysis.fact_type)}/{analysis.subject_id}"
											class="text-blue-500 no-underline hover:underline"
											onclick={(e) => e.stopPropagation()}
										>
											{analysis.subject_id.slice(0, 8)}...
										</a>
									</td>
									<td class="max-w-xs overflow-hidden text-ellipsis whitespace-nowrap border-b border-slate-100 px-4 py-3 text-slate-800">
										{truncate(analysis.conclusion)}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">
										{#if analysis.research_status}
											<UncertaintyBadge status={analysis.research_status} showLabel />
										{:else}
											<span class="text-slate-400">--</span>
										{/if}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-center text-slate-800">
										{analysis.citation_ids?.length ?? 0}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="flex flex-col gap-3 md:hidden">
					{#each analyses as analysis}
						<a
							href="/evidence/analyses/{analysis.id}"
							class="block rounded-lg border border-slate-200 bg-white p-4 text-inherit no-underline transition-shadow hover:border-slate-300 hover:shadow-sm"
						>
							<div class="mb-2 flex items-center justify-between">
								<span class="text-sm font-semibold text-slate-800">
									{formatFactType(analysis.fact_type)}
								</span>
								{#if analysis.research_status}
									<UncertaintyBadge status={analysis.research_status} showLabel size="small" />
								{/if}
							</div>
							<p class="m-0 text-[0.8125rem] leading-snug text-slate-600">
								{truncate(analysis.conclusion, 120)}
							</p>
							<div class="mt-2 text-xs text-slate-400">
								<span>{analysis.citation_ids?.length ?? 0} citations</span>
							</div>
						</a>
					{/each}
				</div>

				{#if analysesTotalPages > 1}
					{@render pagination(
						analysesPage,
						analysesTotalPages,
						analysesLoading,
						() => {
							analysesPage--;
							loadAnalyses();
						},
						() => {
							analysesPage++;
							loadAnalyses();
						}
					)}
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Conflicts Tab -->
		<Tabs.Content value="conflicts">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<div class="flex gap-1">
					{#each [{ key: 'all', label: 'All' }, { key: 'open', label: 'Open' }, { key: 'resolved', label: 'Resolved' }] as filter}
						<Button
							variant={conflictStatusFilter === filter.key ? 'default' : 'outline'}
							size="sm"
							aria-pressed={conflictStatusFilter === filter.key}
							onclick={() => {
								conflictStatusFilter = filter.key as 'all' | 'open' | 'resolved';
								conflictsPage = 1;
								loadConflicts();
							}}
						>
							{filter.label}
						</Button>
					{/each}
				</div>
				<span class="text-sm text-slate-500">
					{conflictsTotal} {conflictsTotal === 1 ? 'conflict' : 'conflicts'}
				</span>
			</div>

			{#if conflictsLoading}
				<div class="p-12 text-center text-slate-500">Loading conflicts...</div>
			{:else if conflictsError}
				<div class="p-12 text-center text-red-600">
					<p class="m-0 mb-4">{conflictsError}</p>
					<Button variant="outline" onclick={loadConflicts}>Retry</Button>
				</div>
			{:else if conflicts.length === 0}
				<div class="p-12 text-center text-slate-500">
					<p class="m-0 mb-2">No conflicts found.</p>
					<p class="mb-4 text-[0.8125rem] text-slate-400">
						Conflicts are auto-detected when analyses for the same fact disagree.
					</p>
				</div>
			{:else}
				<!-- Desktop table -->
				<div class="hidden overflow-x-auto md:block">
					<table class="w-full border-collapse text-sm">
						<thead>
							<tr>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Fact Type</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Subject</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Description</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Status</th>
							</tr>
						</thead>
						<tbody>
							{#each conflicts as conflict}
								<tr
									class="cursor-pointer transition-colors hover:bg-slate-50"
									tabindex="0"
									role="link"
									onclick={() => goto(`/evidence/conflicts/${conflict.id}`)}
									onkeydown={(e) => {
										if (e.key === 'Enter' || e.key === ' ') {
											e.preventDefault();
											goto(`/evidence/conflicts/${conflict.id}`);
										}
									}}
								>
									<td class="whitespace-nowrap border-b border-slate-100 px-4 py-3 font-medium text-slate-800">
										{formatFactType(conflict.fact_type)}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">
										<a
											href="/{subjectRoute(conflict.fact_type)}/{conflict.subject_id}"
											class="text-blue-500 no-underline hover:underline"
											onclick={(e) => e.stopPropagation()}
										>
											{conflict.subject_id.slice(0, 8)}...
										</a>
									</td>
									<td class="max-w-xs overflow-hidden text-ellipsis whitespace-nowrap border-b border-slate-100 px-4 py-3 text-slate-800">
										{truncate(conflict.description)}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">
										{#if conflict.status === 'open'}
											<Badge variant="destructive">Open</Badge>
										{:else}
											<Badge class="border-green-200 bg-green-50 text-green-700">Resolved</Badge>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="flex flex-col gap-3 md:hidden">
					{#each conflicts as conflict}
						<a
							href="/evidence/conflicts/{conflict.id}"
							class="block rounded-lg border border-slate-200 bg-white p-4 text-inherit no-underline transition-shadow hover:border-slate-300 hover:shadow-sm"
						>
							<div class="mb-2 flex items-center justify-between">
								<span class="text-sm font-semibold text-slate-800">
									{formatFactType(conflict.fact_type)}
								</span>
								{#if conflict.status === 'open'}
									<Badge variant="destructive">Open</Badge>
								{:else}
									<Badge class="border-green-200 bg-green-50 text-green-700">Resolved</Badge>
								{/if}
							</div>
							<p class="m-0 text-[0.8125rem] leading-snug text-slate-600">
								{truncate(conflict.description, 120)}
							</p>
						</a>
					{/each}
				</div>

				{#if conflictsTotalPages > 1}
					{@render pagination(
						conflictsPage,
						conflictsTotalPages,
						conflictsLoading,
						() => {
							conflictsPage--;
							loadConflicts();
						},
						() => {
							conflictsPage++;
							loadConflicts();
						}
					)}
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Research Logs Tab -->
		<Tabs.Content value="logs">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<span class="text-sm text-slate-500">
					{logsTotal} {logsTotal === 1 ? 'log entry' : 'log entries'}
				</span>
				<Button href="/evidence/research-logs/new">New Research Log</Button>
			</div>

			{#if logsLoading}
				<div class="p-12 text-center text-slate-500">Loading research logs...</div>
			{:else if logsError}
				<div class="p-12 text-center text-red-600">
					<p class="m-0 mb-4">{logsError}</p>
					<Button variant="outline" onclick={loadLogs}>Retry</Button>
				</div>
			{:else if logs.length === 0}
				<div class="p-12 text-center text-slate-500">
					<p class="m-0 mb-2">No research logs yet.</p>
					<p class="mb-4 text-[0.8125rem] text-slate-400">
						Log your research activities to track what repositories and records you have searched.
					</p>
					<Button href="/evidence/research-logs/new">New Research Log</Button>
				</div>
			{:else}
				<!-- Desktop table -->
				<div class="hidden overflow-x-auto md:block">
					<table class="w-full border-collapse text-sm">
						<thead>
							<tr>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Subject</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Repository</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Search Description</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Outcome</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Date</th>
							</tr>
						</thead>
						<tbody>
							{#each logs as log}
								<tr
									class="cursor-pointer transition-colors hover:bg-slate-50"
									tabindex="0"
									role="link"
									onclick={() => goto(`/evidence/research-logs/${log.id}`)}
									onkeydown={(e) => {
										if (e.key === 'Enter' || e.key === ' ') {
											e.preventDefault();
											goto(`/evidence/research-logs/${log.id}`);
										}
									}}
								>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">
										<a
											href="/{log.subject_type === 'family' ? 'families' : 'persons'}/{log.subject_id}"
											class="text-blue-500 no-underline hover:underline"
											onclick={(e) => e.stopPropagation()}
										>
											{log.subject_id.slice(0, 8)}...
										</a>
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">{log.repository}</td>
									<td class="max-w-xs overflow-hidden text-ellipsis whitespace-nowrap border-b border-slate-100 px-4 py-3 text-slate-800">
										{truncate(log.search_description)}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">
										{#if log.outcome === 'found'}
											<Badge class="border-green-200 bg-green-50 text-green-700">Found</Badge>
										{:else if log.outcome === 'not_found'}
											<Badge variant="destructive">Not Found</Badge>
										{:else}
											<Badge class="border-yellow-200 bg-yellow-50 text-yellow-700">Inconclusive</Badge>
										{/if}
									</td>
									<td class="whitespace-nowrap border-b border-slate-100 px-4 py-3 text-slate-800">
										{formatDate(log.search_date)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="flex flex-col gap-3 md:hidden">
					{#each logs as log}
						<a
							href="/evidence/research-logs/{log.id}"
							class="block rounded-lg border border-slate-200 bg-white p-4 text-inherit no-underline transition-shadow hover:border-slate-300 hover:shadow-sm"
						>
							<div class="mb-2 flex items-center justify-between">
								<span class="text-sm font-semibold text-slate-800">{log.repository}</span>
								{#if log.outcome === 'found'}
									<Badge class="border-green-200 bg-green-50 text-green-700">Found</Badge>
								{:else if log.outcome === 'not_found'}
									<Badge variant="destructive">Not Found</Badge>
								{:else}
									<Badge class="border-yellow-200 bg-yellow-50 text-yellow-700">Inconclusive</Badge>
								{/if}
							</div>
							<p class="m-0 text-[0.8125rem] leading-snug text-slate-600">
								{truncate(log.search_description, 120)}
							</p>
							<div class="mt-2 text-xs text-slate-400">
								<span>{formatDate(log.search_date)}</span>
							</div>
						</a>
					{/each}
				</div>

				{#if logsTotalPages > 1}
					{@render pagination(
						logsPage,
						logsTotalPages,
						logsLoading,
						() => {
							logsPage--;
							loadLogs();
						},
						() => {
							logsPage++;
							loadLogs();
						}
					)}
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Proof Summaries Tab -->
		<Tabs.Content value="summaries">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<span class="text-sm text-slate-500">
					{summariesTotal} {summariesTotal === 1 ? 'summary' : 'summaries'}
				</span>
				<Button href="/evidence/proof-summaries/new">New Proof Summary</Button>
			</div>

			{#if summariesLoading}
				<div class="p-12 text-center text-slate-500">Loading proof summaries...</div>
			{:else if summariesError}
				<div class="p-12 text-center text-red-600">
					<p class="m-0 mb-4">{summariesError}</p>
					<Button variant="outline" onclick={loadSummaries}>Retry</Button>
				</div>
			{:else if summaries.length === 0}
				<div class="p-12 text-center text-slate-500">
					<p class="m-0 mb-2">No proof summaries yet.</p>
					<p class="mb-4 text-[0.8125rem] text-slate-400">
						Create a proof summary to document your conclusions about a genealogical fact.
					</p>
					<Button href="/evidence/proof-summaries/new">New Proof Summary</Button>
				</div>
			{:else}
				<!-- Desktop table -->
				<div class="hidden overflow-x-auto md:block">
					<table class="w-full border-collapse text-sm">
						<thead>
							<tr>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Fact Type</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Subject</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Conclusion</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Status</th>
								<th class="whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600">Analyses</th>
							</tr>
						</thead>
						<tbody>
							{#each summaries as summary}
								<tr
									class="cursor-pointer transition-colors hover:bg-slate-50"
									tabindex="0"
									role="link"
									onclick={() => goto(`/evidence/proof-summaries/${summary.id}`)}
									onkeydown={(e) => {
										if (e.key === 'Enter' || e.key === ' ') {
											e.preventDefault();
											goto(`/evidence/proof-summaries/${summary.id}`);
										}
									}}
								>
									<td class="whitespace-nowrap border-b border-slate-100 px-4 py-3 font-medium text-slate-800">
										{formatFactType(summary.fact_type)}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">
										<a
											href="/{subjectRoute(summary.fact_type)}/{summary.subject_id}"
											class="text-blue-500 no-underline hover:underline"
											onclick={(e) => e.stopPropagation()}
										>
											{summary.subject_id.slice(0, 8)}...
										</a>
									</td>
									<td class="max-w-xs overflow-hidden text-ellipsis whitespace-nowrap border-b border-slate-100 px-4 py-3 text-slate-800">
										{truncate(summary.conclusion)}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-slate-800">
										{#if summary.research_status}
											<UncertaintyBadge status={summary.research_status} showLabel />
										{:else}
											<span class="text-slate-400">--</span>
										{/if}
									</td>
									<td class="border-b border-slate-100 px-4 py-3 text-center text-slate-800">
										{summary.analysis_ids?.length ?? 0}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="flex flex-col gap-3 md:hidden">
					{#each summaries as summary}
						<a
							href="/evidence/proof-summaries/{summary.id}"
							class="block rounded-lg border border-slate-200 bg-white p-4 text-inherit no-underline transition-shadow hover:border-slate-300 hover:shadow-sm"
						>
							<div class="mb-2 flex items-center justify-between">
								<span class="text-sm font-semibold text-slate-800">
									{formatFactType(summary.fact_type)}
								</span>
								{#if summary.research_status}
									<UncertaintyBadge status={summary.research_status} showLabel size="small" />
								{/if}
							</div>
							<p class="m-0 text-[0.8125rem] leading-snug text-slate-600">
								{truncate(summary.conclusion, 120)}
							</p>
							<div class="mt-2 text-xs text-slate-400">
								<span>{summary.analysis_ids?.length ?? 0} linked analyses</span>
							</div>
						</a>
					{/each}
				</div>

				{#if summariesTotalPages > 1}
					{@render pagination(
						summariesPage,
						summariesTotalPages,
						summariesLoading,
						() => {
							summariesPage--;
							loadSummaries();
						},
						() => {
							summariesPage++;
							loadSummaries();
						}
					)}
				{/if}
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>
