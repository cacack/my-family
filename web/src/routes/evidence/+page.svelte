<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import {
		api,
		type EvidenceAnalysisResponse,
		type EvidenceConflictResponse,
		type ResearchLogResponse,
		type ProofSummaryResponse,
		type ConflictStatus
	} from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import * as Tabs from '#lib/components/ui/tabs/index.js';
	import UncertaintyBadge from '#lib/components/UncertaintyBadge.svelte';
	import {
		formatFactType,
		subjectTypeOf,
		formatDate,
		outcomeBadgeProps,
		conflictBadgeProps,
		readableConflictDescription
	} from '#lib/utils/evidence.js';
	import { SubjectNames, subjectHref, type Subject } from '#lib/utils/subjectNames.svelte.js';
	import { clampPageToTotal, readEnum, readPositiveInt, setQuery } from '#lib/utils/urlState.js';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';

	const pageSize = 20;

	// Shared class strings for the desktop tables. Same pattern as the
	// {#snippet pagination} below — single place to change when the table styling
	// evolves, rather than 20+ <th>/<td> sites.
	const TH_CLASS =
		'whitespace-nowrap border-b-2 border-slate-200 px-4 py-3 text-left font-semibold text-muted-foreground';
	const TD_CLASS = 'border-b border-slate-100 px-4 py-3 text-foreground';
	const TD_NOWRAP = `${TD_CLASS} whitespace-nowrap font-medium`;
	const TD_TRUNCATE = `${TD_CLASS} max-w-xs overflow-hidden text-ellipsis whitespace-nowrap`;
	const TD_CENTER = `${TD_CLASS} text-center`;
	// Each row's first cell holds a real link to the record (#1108), so a row
	// can be opened in a new tab; its ::after stretches over the row to keep
	// the whole row clickable, and the subject link sits above it.
	const ROW = 'relative transition-colors hover:bg-slate-50';
	const ROW_LINK = 'text-inherit no-underline after:absolute after:inset-0 hover:underline';
	const SUBJECT_LINK = 'relative z-10 text-link no-underline hover:underline';

	// Evidence records store a subject id; the page shows the subject's name.
	const subjects = new SubjectNames();
	const factSubject = (record: { fact_type: string; subject_id: string }): Subject => ({
		type: subjectTypeOf(record.fact_type),
		id: record.subject_id
	});
	const logSubject = (log: ResearchLogResponse): Subject => ({
		type: log.subject_type === 'family' ? 'family' : 'person',
		id: log.subject_id
	});

	// The active tab, its page and the Conflicts status filter live in the URL
	// (see urlState.ts), so Back, reload and a copied link restore the view.
	// `page` belongs to the active tab: switching tabs starts the new one at
	// page 1 and drops the Conflicts filter. Tab and page changes push; the
	// filter refines the view, so it replaces.
	const TABS = ['analyses', 'conflicts', 'logs', 'summaries'] as const;
	const STATUSES = ['all', 'open', 'resolved'] as const satisfies readonly ('all' | ConflictStatus)[];
	const CONFLICT_FILTERS = [
		{ key: 'all', label: 'All' },
		{ key: 'open', label: 'Open' },
		{ key: 'resolved', label: 'Resolved' }
	] as const satisfies readonly { key: (typeof STATUSES)[number]; label: string }[];
	const DEFAULTS = { tab: 'analyses', page: 1, status: 'all' } as const;

	const params = $derived(page.url.searchParams);
	const activeTab = $derived(readEnum(params, 'tab', TABS, DEFAULTS.tab));
	const currentPage = $derived(readPositiveInt(params, 'page', DEFAULTS.page));
	const conflictStatusFilter = $derived(
		activeTab === 'conflicts' ? readEnum(params, 'status', STATUSES, DEFAULTS.status) : DEFAULTS.status
	);

	function selectTab(tab: string) {
		setQuery(page.url, { tab, page: null, status: null }, { push: true, defaults: DEFAULTS });
	}

	function goToPage(n: number) {
		setQuery(page.url, { page: n }, { push: true, defaults: DEFAULTS });
	}

	function selectConflictStatus(status: (typeof STATUSES)[number]) {
		setQuery(page.url, { status, page: null }, { defaults: DEFAULTS });
	}

	// Analyses state
	let analyses: EvidenceAnalysisResponse[] = $state([]);
	let analysesTotal = $state(0);
	let analysesLoading = $state(false);
	let analysesError: string | null = $state(null);

	// Conflicts state
	let conflicts: EvidenceConflictResponse[] = $state([]);
	let conflictsTotal = $state(0);
	let conflictsLoading = $state(false);
	let conflictsError: string | null = $state(null);
	let openConflictsCount = $state(0);
	let openConflictsLoaded = $state(false);

	// Research logs state
	let logs: ResearchLogResponse[] = $state([]);
	let logsTotal = $state(0);
	let logsLoading = $state(false);
	let logsError: string | null = $state(null);

	// Proof summaries state
	let summaries: ProofSummaryResponse[] = $state([]);
	let summariesTotal = $state(0);
	let summariesLoading = $state(false);
	let summariesError: string | null = $state(null);

	function conflictDescription(conflict: EvidenceConflictResponse): string {
		return readableConflictDescription(
			conflict.description,
			conflict.fact_type,
			conflict.subject_id,
			subjects.name(factSubject(conflict))
		);
	}

	function truncate(text: string, maxLen = 80): string {
		if (text.length <= maxLen) return text;
		return text.slice(0, maxLen) + '...';
	}

	// --- Data loading ---

	// Tab switches, Back and paging can start loads faster than they finish:
	// only each tab's latest load may land.
	let analysesSeq = 0;
	let conflictsSeq = 0;
	let logsSeq = 0;
	let summariesSeq = 0;

	async function loadAnalyses() {
		const seq = ++analysesSeq;
		analysesLoading = true;
		analysesError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.listEvidenceAnalyses({
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize
			});
			if (seq !== analysesSeq) return;
			if (clampPageToTotal(page.url, requestedPage, result.total ?? 0, pageSize)) return;
			analyses = result.analyses;
			subjects.load(result.analyses.map(factSubject));
			analysesTotal = result.total ?? 0;
		} catch (e) {
			if (seq !== analysesSeq) return;
			analysesError = (e as { message?: string }).message || 'Failed to load analyses';
		} finally {
			if (seq === analysesSeq) analysesLoading = false;
		}
	}

	async function loadConflicts() {
		const seq = ++conflictsSeq;
		conflictsLoading = true;
		conflictsError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.listEvidenceConflicts({
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize,
				status: conflictStatusFilter === 'all' ? undefined : conflictStatusFilter
			});
			if (seq !== conflictsSeq) return;
			if (clampPageToTotal(page.url, requestedPage, result.total ?? 0, pageSize)) return;
			conflicts = result.conflicts;
			subjects.load(result.conflicts.map(factSubject));
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
			if (seq !== conflictsSeq) return;
			conflictsError = (e as { message?: string }).message || 'Failed to load conflicts';
		} finally {
			if (seq === conflictsSeq) conflictsLoading = false;
		}
	}

	async function loadLogs() {
		const seq = ++logsSeq;
		logsLoading = true;
		logsError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.listResearchLogs({
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize
			});
			if (seq !== logsSeq) return;
			if (clampPageToTotal(page.url, requestedPage, result.total ?? 0, pageSize)) return;
			logs = result.logs;
			subjects.load(result.logs.map(logSubject));
			logsTotal = result.total ?? 0;
		} catch (e) {
			if (seq !== logsSeq) return;
			logsError = (e as { message?: string }).message || 'Failed to load research logs';
		} finally {
			if (seq === logsSeq) logsLoading = false;
		}
	}

	async function loadSummaries() {
		const seq = ++summariesSeq;
		summariesLoading = true;
		summariesError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.listProofSummaries({
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize
			});
			if (seq !== summariesSeq) return;
			if (clampPageToTotal(page.url, requestedPage, result.total ?? 0, pageSize)) return;
			summaries = result.summaries;
			subjects.load(result.summaries.map(factSubject));
			summariesTotal = result.total ?? 0;
		} catch (e) {
			if (seq !== summariesSeq) return;
			summariesError = (e as { message?: string }).message || 'Failed to load proof summaries';
		} finally {
			if (seq === summariesSeq) summariesLoading = false;
		}
	}

	// Load the active tab whenever the URL changes its tab, page or filter.
	$effect(() => {
		const tab = activeTab;
		void currentPage;
		void conflictStatusFilter;
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
		<span class="text-sm text-muted-foreground">Page {currentPage} of {totalPages}</span>
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
		data-slot="card-link"
	>
		<div class="mb-2 flex items-center justify-between">
			<span class="text-sm font-semibold text-foreground">{topLeft}</span>
			{@render badge()}
		</div>
		<p class="m-0 text-[0.8125rem] leading-snug text-muted-foreground">{body}</p>
		{#if meta}
			<div class="mt-2 text-xs text-muted-foreground">{meta}</div>
		{/if}
	</a>
{/snippet}

<div class="mx-auto max-w-page-list p-6">
	<PageHeader
		title="Evidence Analysis"
		description="GPS-compliant research tracking and proof management"
	/>

	<Tabs.Root value={activeTab} onValueChange={selectTab}>
		<!-- Wraps rather than scrolls, so every tab stays visible on a phone (#1108). -->
		<Tabs.List class="max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto">
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
				<span class="text-sm text-muted-foreground">
					{analysesTotal} {analysesTotal === 1 ? 'analysis' : 'analyses'}
				</span>
				<Button href="/evidence/analyses/new">New Analysis</Button>
			</div>

			{#if analysesLoading}
				<div class="p-12 text-center text-muted-foreground">Loading analyses...</div>
			{:else if analysesError}
				<ErrorState message={analysesError} onRetry={loadAnalyses} />
			{:else if analyses.length === 0}
				<div class="p-12 text-center text-muted-foreground">
					<p class="m-0 mb-2">No evidence analyses yet.</p>
					<p class="mb-4 text-[0.8125rem] text-muted-foreground">
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
								<tr class={ROW}>
									<td class={TD_NOWRAP}>
										<a href="/evidence/analyses/{analysis.id}" class={ROW_LINK}>
											{formatFactType(analysis.fact_type)}
										</a>
									</td>
									<td class={TD_CLASS}>
										<a href={subjectHref(factSubject(analysis))} class={SUBJECT_LINK}>
											{subjects.label(factSubject(analysis))}
										</a>
									</td>
									<td class={TD_TRUNCATE}>{truncate(analysis.conclusion)}</td>
									<td class={TD_CLASS}>
										{#if analysis.research_status}
											<UncertaintyBadge status={analysis.research_status} showLabel />
										{:else}
											<span class="text-muted-foreground">--</span>
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
							`${subjects.label(factSubject(analysis))} · ${analysis.citation_ids?.length ?? 0} citations`,
							analysisBadge
						)}
					{/each}
				</div>

				{#if analysesTotalPages > 1}
					{@render pagination(
						currentPage,
						analysesTotalPages,
						analysesLoading,
						() => goToPage(currentPage - 1),
						() => goToPage(currentPage + 1)
					)}
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Conflicts Tab -->
		<Tabs.Content value="conflicts">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<div class="flex gap-1">
					{#each CONFLICT_FILTERS as filter (filter.key)}
						<Button
							variant={conflictStatusFilter === filter.key ? 'default' : 'outline'}
							size="sm"
							aria-pressed={conflictStatusFilter === filter.key}
							onclick={() => selectConflictStatus(filter.key)}
						>
							{filter.label}
						</Button>
					{/each}
				</div>
				<span class="text-sm text-muted-foreground">
					{conflictsTotal} {conflictsTotal === 1 ? 'conflict' : 'conflicts'}
				</span>
			</div>

			{#if conflictsLoading}
				<div class="p-12 text-center text-muted-foreground">Loading conflicts...</div>
			{:else if conflictsError}
				<ErrorState message={conflictsError} onRetry={loadConflicts} />
			{:else if conflicts.length === 0}
				<div class="p-12 text-center text-muted-foreground">
					<p class="m-0 mb-2">No conflicts found.</p>
					<p class="mb-4 text-[0.8125rem] text-muted-foreground">
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
								<tr class={ROW}>
									<td class={TD_NOWRAP}>
										<a href="/evidence/conflicts/{conflict.id}" class={ROW_LINK}>
											{formatFactType(conflict.fact_type)}
										</a>
									</td>
									<td class={TD_CLASS}>
										<a href={subjectHref(factSubject(conflict))} class={SUBJECT_LINK}>
											{subjects.label(factSubject(conflict))}
										</a>
									</td>
									<td class={TD_TRUNCATE}>{truncate(conflictDescription(conflict))}</td>
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
							truncate(conflictDescription(conflict), 120),
							subjects.label(factSubject(conflict)),
							conflictMobileBadge
						)}
					{/each}
				</div>

				{#if conflictsTotalPages > 1}
					{@render pagination(
						currentPage,
						conflictsTotalPages,
						conflictsLoading,
						() => goToPage(currentPage - 1),
						() => goToPage(currentPage + 1)
					)}
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Research Logs Tab -->
		<Tabs.Content value="logs">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<span class="text-sm text-muted-foreground">
					{logsTotal} {logsTotal === 1 ? 'log entry' : 'log entries'}
				</span>
				<Button href="/evidence/research-logs/new">New Research Log</Button>
			</div>

			{#if logsLoading}
				<div class="p-12 text-center text-muted-foreground">Loading research logs...</div>
			{:else if logsError}
				<ErrorState message={logsError} onRetry={loadLogs} />
			{:else if logs.length === 0}
				<div class="p-12 text-center text-muted-foreground">
					<p class="m-0 mb-2">No research logs yet.</p>
					<p class="mb-4 text-[0.8125rem] text-muted-foreground">
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
								<tr class={ROW}>
									<td class={TD_CLASS}>
										<a href={subjectHref(logSubject(log))} class={SUBJECT_LINK}>
											{subjects.label(logSubject(log))}
										</a>
									</td>
									<td class={TD_CLASS}>
										<a href="/evidence/research-logs/{log.id}" class={ROW_LINK}>
											{log.repository}
										</a>
									</td>
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
							`${subjects.label(logSubject(log))} · ${formatDate(log.search_date)}`,
							logMobileBadge
						)}
					{/each}
				</div>

				{#if logsTotalPages > 1}
					{@render pagination(
						currentPage,
						logsTotalPages,
						logsLoading,
						() => goToPage(currentPage - 1),
						() => goToPage(currentPage + 1)
					)}
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Proof Summaries Tab -->
		<Tabs.Content value="summaries">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-2 pt-2">
				<span class="text-sm text-muted-foreground">
					{summariesTotal} {summariesTotal === 1 ? 'summary' : 'summaries'}
				</span>
				<Button href="/evidence/proof-summaries/new">New Proof Summary</Button>
			</div>

			{#if summariesLoading}
				<div class="p-12 text-center text-muted-foreground">Loading proof summaries...</div>
			{:else if summariesError}
				<ErrorState message={summariesError} onRetry={loadSummaries} />
			{:else if summaries.length === 0}
				<div class="p-12 text-center text-muted-foreground">
					<p class="m-0 mb-2">No proof summaries yet.</p>
					<p class="mb-4 text-[0.8125rem] text-muted-foreground">
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
								<tr class={ROW}>
									<td class={TD_NOWRAP}>
										<a href="/evidence/proof-summaries/{summary.id}" class={ROW_LINK}>
											{formatFactType(summary.fact_type)}
										</a>
									</td>
									<td class={TD_CLASS}>
										<a href={subjectHref(factSubject(summary))} class={SUBJECT_LINK}>
											{subjects.label(factSubject(summary))}
										</a>
									</td>
									<td class={TD_TRUNCATE}>{truncate(summary.conclusion)}</td>
									<td class={TD_CLASS}>
										{#if summary.research_status}
											<UncertaintyBadge status={summary.research_status} showLabel />
										{:else}
											<span class="text-muted-foreground">--</span>
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
							`${subjects.label(factSubject(summary))} · ${summary.analysis_ids?.length ?? 0} linked analyses`,
							summaryBadge
						)}
					{/each}
				</div>

				{#if summariesTotalPages > 1}
					{@render pagination(
						currentPage,
						summariesTotalPages,
						summariesLoading,
						() => goToPage(currentPage - 1),
						() => goToPage(currentPage + 1)
					)}
				{/if}
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>
