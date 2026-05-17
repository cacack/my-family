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
	import {
		formatFactType,
		subjectRoute,
		formatDate,
		outcomeBadgeProps,
		conflictBadgeProps
	} from '$lib/utils/evidence';

	const pageSize = 20;

	// Shared class strings for the desktop tables. Same pattern as the
	// {#snippet pagination} below — single place to change when the table styling
	// evolves, rather than 20+ <th>/<td> sites.
	const TH_CLASS =
		'whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-slate-600';
	const TD_CLASS = 'border-b border-slate-100 px-4 py-3 text-slate-800';
	const TD_NOWRAP = `${TD_CLASS} whitespace-nowrap font-medium`;
	const TD_TRUNCATE = `${TD_CLASS} max-w-xs overflow-hidden text-ellipsis whitespace-nowrap`;
	const TD_CENTER = `${TD_CLASS} text-center`;
	const ROW_CLICKABLE = 'cursor-pointer transition-colors hover:bg-slate-50';
	const SUBJECT_LINK = 'text-blue-500 no-underline hover:underline';

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
		<Button
			variant="outline"
			size="sm"
			onclick={() => {
				if (currentPage > 1) onPrev();
			}}
			disabled={currentPage === 1 || loading}
		>
			Previous
		</Button>
		<span class="text-sm text-slate-500">Page {currentPage} of {totalPages}</span>
		<Button
			variant="outline"
			size="sm"
			onclick={() => {
				if (currentPage < totalPages) onNext();
			}}
			disabled={currentPage >= totalPages || loading}
		>
			Next
		</Button>
	</div>
{/snippet}

{#snippet mobileCard(href: string, topLeft: string, body: string, meta: string | null, badge: import('svelte').Snippet)}
	<a
		href={href}
		class="block rounded-lg border border-slate-200 bg-white p-4 text-inherit no-underline transition-shadow hover:border-slate-300 hover:shadow-sm"
	>
		<div class="mb-2 flex items-center justify-between">
			<span class="text-sm font-semibold text-slate-800">{topLeft}</span>
			{@render badge()}
		</div>
		<p class="m-0 text-[0.8125rem] leading-snug text-slate-600">{body}</p>
		{#if meta}
			<div class="mt-2 text-xs text-slate-400">{meta}</div>
		{/if}
	</a>
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
								<th class={TH_CLASS}>Fact Type</th>
								<th class={TH_CLASS}>Subject</th>
								<th class={TH_CLASS}>Conclusion</th>
								<th class={TH_CLASS}>Status</th>
								<th class={TH_CLASS}>Citations</th>
							</tr>
						</thead>
						<tbody>
							{#each analyses as analysis}
								<tr
									class={ROW_CLICKABLE}
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
									<td class={TD_NOWRAP}>{formatFactType(analysis.fact_type)}</td>
									<td class={TD_CLASS}>
										<a
											href="/{subjectRoute(analysis.fact_type)}/{analysis.subject_id}"
											class={SUBJECT_LINK}
											onclick={(e) => e.stopPropagation()}
										>
											{analysis.subject_id.slice(0, 8)}...
										</a>
									</td>
									<td class={TD_TRUNCATE}>{truncate(analysis.conclusion)}</td>
									<td class={TD_CLASS}>
										{#if analysis.research_status}
											<UncertaintyBadge status={analysis.research_status} showLabel />
										{:else}
											<span class="text-slate-400">--</span>
										{/if}
									</td>
									<td class={TD_CENTER}>{analysis.citation_ids?.length ?? 0}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="flex flex-col gap-3 md:hidden">
					{#each analyses as analysis}
						{#snippet analysisBadge()}
							{#if analysis.research_status}
								<UncertaintyBadge
									status={analysis.research_status}
									showLabel
									size="small"
								/>
							{/if}
						{/snippet}
						{@render mobileCard(
							`/evidence/analyses/${analysis.id}`,
							formatFactType(analysis.fact_type),
							truncate(analysis.conclusion, 120),
							`${analysis.citation_ids?.length ?? 0} citations`,
							analysisBadge
						)}
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
								<th class={TH_CLASS}>Fact Type</th>
								<th class={TH_CLASS}>Subject</th>
								<th class={TH_CLASS}>Description</th>
								<th class={TH_CLASS}>Status</th>
							</tr>
						</thead>
						<tbody>
							{#each conflicts as conflict}
								{@const statusBadge = conflictBadgeProps(conflict.status)}
								<tr
									class={ROW_CLICKABLE}
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
									<td class={TD_NOWRAP}>{formatFactType(conflict.fact_type)}</td>
									<td class={TD_CLASS}>
										<a
											href="/{subjectRoute(conflict.fact_type)}/{conflict.subject_id}"
											class={SUBJECT_LINK}
											onclick={(e) => e.stopPropagation()}
										>
											{conflict.subject_id.slice(0, 8)}...
										</a>
									</td>
									<td class={TD_TRUNCATE}>{truncate(conflict.description)}</td>
									<td class={TD_CLASS}>
										<Badge variant={statusBadge.variant} class={statusBadge.class}>
											{statusBadge.label}
										</Badge>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="flex flex-col gap-3 md:hidden">
					{#each conflicts as conflict}
						{@const statusBadge = conflictBadgeProps(conflict.status)}
						{#snippet conflictMobileBadge()}
							<Badge variant={statusBadge.variant} class={statusBadge.class}>
								{statusBadge.label}
							</Badge>
						{/snippet}
						{@render mobileCard(
							`/evidence/conflicts/${conflict.id}`,
							formatFactType(conflict.fact_type),
							truncate(conflict.description, 120),
							null,
							conflictMobileBadge
						)}
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
								<th class={TH_CLASS}>Subject</th>
								<th class={TH_CLASS}>Repository</th>
								<th class={TH_CLASS}>Search Description</th>
								<th class={TH_CLASS}>Outcome</th>
								<th class={TH_CLASS}>Date</th>
							</tr>
						</thead>
						<tbody>
							{#each logs as log}
								{@const outcomeBadge = outcomeBadgeProps(log.outcome)}
								<tr
									class={ROW_CLICKABLE}
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
									<td class={TD_CLASS}>
										<a
											href="/{log.subject_type === 'family' ? 'families' : 'persons'}/{log.subject_id}"
											class={SUBJECT_LINK}
											onclick={(e) => e.stopPropagation()}
										>
											{log.subject_id.slice(0, 8)}...
										</a>
									</td>
									<td class={TD_CLASS}>{log.repository}</td>
									<td class={TD_TRUNCATE}>{truncate(log.search_description)}</td>
									<td class={TD_CLASS}>
										<Badge variant={outcomeBadge.variant} class={outcomeBadge.class}>
											{outcomeBadge.label}
										</Badge>
									</td>
									<td class={`${TD_CLASS} whitespace-nowrap`}>
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
						{@const outcomeBadge = outcomeBadgeProps(log.outcome)}
						{#snippet logMobileBadge()}
							<Badge variant={outcomeBadge.variant} class={outcomeBadge.class}>
								{outcomeBadge.label}
							</Badge>
						{/snippet}
						{@render mobileCard(
							`/evidence/research-logs/${log.id}`,
							log.repository,
							truncate(log.search_description, 120),
							formatDate(log.search_date),
							logMobileBadge
						)}
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
								<th class={TH_CLASS}>Fact Type</th>
								<th class={TH_CLASS}>Subject</th>
								<th class={TH_CLASS}>Conclusion</th>
								<th class={TH_CLASS}>Status</th>
								<th class={TH_CLASS}>Analyses</th>
							</tr>
						</thead>
						<tbody>
							{#each summaries as summary}
								<tr
									class={ROW_CLICKABLE}
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
									<td class={TD_NOWRAP}>{formatFactType(summary.fact_type)}</td>
									<td class={TD_CLASS}>
										<a
											href="/{subjectRoute(summary.fact_type)}/{summary.subject_id}"
											class={SUBJECT_LINK}
											onclick={(e) => e.stopPropagation()}
										>
											{summary.subject_id.slice(0, 8)}...
										</a>
									</td>
									<td class={TD_TRUNCATE}>{truncate(summary.conclusion)}</td>
									<td class={TD_CLASS}>
										{#if summary.research_status}
											<UncertaintyBadge status={summary.research_status} showLabel />
										{:else}
											<span class="text-slate-400">--</span>
										{/if}
									</td>
									<td class={TD_CENTER}>{summary.analysis_ids?.length ?? 0}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="flex flex-col gap-3 md:hidden">
					{#each summaries as summary}
						{#snippet summaryBadge()}
							{#if summary.research_status}
								<UncertaintyBadge
									status={summary.research_status}
									showLabel
									size="small"
								/>
							{/if}
						{/snippet}
						{@render mobileCard(
							`/evidence/proof-summaries/${summary.id}`,
							formatFactType(summary.fact_type),
							truncate(summary.conclusion, 120),
							`${summary.analysis_ids?.length ?? 0} linked analyses`,
							summaryBadge
						)}
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
