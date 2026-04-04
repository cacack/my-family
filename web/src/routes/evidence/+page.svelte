<script lang="ts">
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

	function formatFactType(factType: string): string {
		return factType
			.replace(/_/g, ' ')
			.replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function subjectRoute(factType: string): string {
		return factType.startsWith('family_') ? 'families' : 'persons';
	}

	function truncate(text: string, maxLen = 80): string {
		if (text.length <= maxLen) return text;
		return text.slice(0, maxLen) + '...';
	}

	function formatDate(dateStr: string): string {
		try {
			return new Date(dateStr).toLocaleDateString();
		} catch {
			return dateStr;
		}
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
			analysesTotal = result.total;
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
			conflictsTotal = result.total;

			// Get open count for badge (only if not already filtered to open)
			if (conflictStatusFilter !== 'open') {
				try {
					const openResult = await api.listEvidenceConflicts({ limit: 1, status: 'open' });
					openConflictsCount = openResult.total;
				} catch {
					// ignore - badge count is non-critical
				}
			} else {
				openConflictsCount = result.total;
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
			logsTotal = result.total;
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
			summariesTotal = result.total;
		} catch (e) {
			summariesError = (e as { message?: string }).message || 'Failed to load proof summaries';
		} finally {
			summariesLoading = false;
		}
	}

	// Load data when tab changes
	$effect(() => {
		if (activeTab === 'analyses') loadAnalyses();
	});
	$effect(() => {
		if (activeTab === 'conflicts') loadConflicts();
	});
	$effect(() => {
		if (activeTab === 'logs') loadLogs();
	});
	$effect(() => {
		if (activeTab === 'summaries') loadSummaries();
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

<div class="evidence-page">
	<header class="page-header">
		<div>
			<h1>Evidence Analysis</h1>
			<p class="subtitle">GPS-compliant research tracking and proof management</p>
		</div>
	</header>

	<Tabs.Root bind:value={activeTab}>
		<Tabs.List>
			<Tabs.Trigger value="analyses">Analyses</Tabs.Trigger>
			<Tabs.Trigger value="conflicts">
				Conflicts
				{#if openConflictsCount > 0}
					<Badge variant="destructive" class="ml-1 h-5 min-w-5 px-1.5 text-xs">{openConflictsCount}</Badge>
				{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="logs">Research Logs</Tabs.Trigger>
			<Tabs.Trigger value="summaries">Proof Summaries</Tabs.Trigger>
		</Tabs.List>

		<!-- Analyses Tab -->
		<Tabs.Content value="analyses">
			<div class="tab-header">
				<span class="tab-count">{analysesTotal} {analysesTotal === 1 ? 'analysis' : 'analyses'}</span>
				<Button href="/evidence/analyses/new">New Analysis</Button>
			</div>

			{#if analysesLoading}
				<div class="loading">Loading analyses...</div>
			{:else if analysesError}
				<div class="error-state">
					<p>{analysesError}</p>
					<Button variant="outline" onclick={loadAnalyses}>Retry</Button>
				</div>
			{:else if analyses.length === 0}
				<div class="empty">
					<p>No evidence analyses yet.</p>
					<p class="empty-hint">Create an analysis to evaluate evidence for a genealogical fact.</p>
					<Button href="/evidence/analyses/new">New Analysis</Button>
				</div>
			{:else}
				<!-- Desktop table -->
				<div class="table-wrapper desktop-only">
					<table>
						<thead>
							<tr>
								<th>Fact Type</th>
								<th>Subject</th>
								<th>Conclusion</th>
								<th>Status</th>
								<th>Citations</th>
							</tr>
						</thead>
						<tbody>
							{#each analyses as analysis}
								<tr class="clickable" onclick={() => window.location.href = `/evidence/analyses/${analysis.id}`}>
									<td class="fact-type">{formatFactType(analysis.fact_type)}</td>
									<td><a href="/{subjectRoute(analysis.fact_type)}/{analysis.subject_id}" onclick={(e) => e.stopPropagation()}>{analysis.subject_id.slice(0, 8)}...</a></td>
									<td class="truncated">{truncate(analysis.conclusion)}</td>
									<td>
										{#if analysis.research_status}
											<UncertaintyBadge status={analysis.research_status} showLabel />
										{:else}
											<span class="text-muted">--</span>
										{/if}
									</td>
									<td class="count">{analysis.citation_ids?.length ?? 0}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="cards-wrapper mobile-only">
					{#each analyses as analysis}
						<a href="/evidence/analyses/{analysis.id}" class="card">
							<div class="card-top">
								<span class="card-fact-type">{formatFactType(analysis.fact_type)}</span>
								{#if analysis.research_status}
									<UncertaintyBadge status={analysis.research_status} showLabel size="small" />
								{/if}
							</div>
							<p class="card-conclusion">{truncate(analysis.conclusion, 120)}</p>
							<div class="card-meta">
								<span>{analysis.citation_ids?.length ?? 0} citations</span>
							</div>
						</a>
					{/each}
				</div>

				{#if analysesTotalPages > 1}
					<div class="pagination">
						<button onclick={() => { analysesPage--; loadAnalyses(); }} disabled={analysesPage === 1}>Previous</button>
						<span>Page {analysesPage} of {analysesTotalPages}</span>
						<button onclick={() => { analysesPage++; loadAnalyses(); }} disabled={analysesPage >= analysesTotalPages}>Next</button>
					</div>
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Conflicts Tab -->
		<Tabs.Content value="conflicts">
			<div class="tab-header">
				<div class="filter-buttons">
					<button class="filter-btn" class:active={conflictStatusFilter === 'all'} onclick={() => { conflictStatusFilter = 'all'; conflictsPage = 1; loadConflicts(); }}>All</button>
					<button class="filter-btn" class:active={conflictStatusFilter === 'open'} onclick={() => { conflictStatusFilter = 'open'; conflictsPage = 1; loadConflicts(); }}>Open</button>
					<button class="filter-btn" class:active={conflictStatusFilter === 'resolved'} onclick={() => { conflictStatusFilter = 'resolved'; conflictsPage = 1; loadConflicts(); }}>Resolved</button>
				</div>
				<span class="tab-count">{conflictsTotal} {conflictsTotal === 1 ? 'conflict' : 'conflicts'}</span>
			</div>

			{#if conflictsLoading}
				<div class="loading">Loading conflicts...</div>
			{:else if conflictsError}
				<div class="error-state">
					<p>{conflictsError}</p>
					<Button variant="outline" onclick={loadConflicts}>Retry</Button>
				</div>
			{:else if conflicts.length === 0}
				<div class="empty">
					<p>No conflicts found.</p>
					<p class="empty-hint">Conflicts are auto-detected when analyses for the same fact disagree.</p>
				</div>
			{:else}
				<!-- Desktop table -->
				<div class="table-wrapper desktop-only">
					<table>
						<thead>
							<tr>
								<th>Fact Type</th>
								<th>Subject</th>
								<th>Description</th>
								<th>Status</th>
							</tr>
						</thead>
						<tbody>
							{#each conflicts as conflict}
								<tr class="clickable" onclick={() => window.location.href = `/evidence/conflicts/${conflict.id}`}>
									<td class="fact-type">{formatFactType(conflict.fact_type)}</td>
									<td><a href="/{subjectRoute(conflict.fact_type)}/{conflict.subject_id}" onclick={(e) => e.stopPropagation()}>{conflict.subject_id.slice(0, 8)}...</a></td>
									<td class="truncated">{truncate(conflict.description)}</td>
									<td>
										{#if conflict.status === 'open'}
											<Badge variant="destructive">Open</Badge>
										{:else}
											<Badge class="bg-green-50 text-green-700 border-green-200">Resolved</Badge>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="cards-wrapper mobile-only">
					{#each conflicts as conflict}
						<a href="/evidence/conflicts/{conflict.id}" class="card">
							<div class="card-top">
								<span class="card-fact-type">{formatFactType(conflict.fact_type)}</span>
								{#if conflict.status === 'open'}
									<Badge variant="destructive">Open</Badge>
								{:else}
									<Badge class="bg-green-50 text-green-700 border-green-200">Resolved</Badge>
								{/if}
							</div>
							<p class="card-conclusion">{truncate(conflict.description, 120)}</p>
						</a>
					{/each}
				</div>

				{#if conflictsTotalPages > 1}
					<div class="pagination">
						<button onclick={() => { conflictsPage--; loadConflicts(); }} disabled={conflictsPage === 1}>Previous</button>
						<span>Page {conflictsPage} of {conflictsTotalPages}</span>
						<button onclick={() => { conflictsPage++; loadConflicts(); }} disabled={conflictsPage >= conflictsTotalPages}>Next</button>
					</div>
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Research Logs Tab -->
		<Tabs.Content value="logs">
			<div class="tab-header">
				<span class="tab-count">{logsTotal} {logsTotal === 1 ? 'log entry' : 'log entries'}</span>
				<Button href="/evidence/research-logs/new">New Research Log</Button>
			</div>

			{#if logsLoading}
				<div class="loading">Loading research logs...</div>
			{:else if logsError}
				<div class="error-state">
					<p>{logsError}</p>
					<Button variant="outline" onclick={loadLogs}>Retry</Button>
				</div>
			{:else if logs.length === 0}
				<div class="empty">
					<p>No research logs yet.</p>
					<p class="empty-hint">Log your research activities to track what repositories and records you have searched.</p>
					<Button href="/evidence/research-logs/new">New Research Log</Button>
				</div>
			{:else}
				<!-- Desktop table -->
				<div class="table-wrapper desktop-only">
					<table>
						<thead>
							<tr>
								<th>Subject</th>
								<th>Repository</th>
								<th>Search Description</th>
								<th>Outcome</th>
								<th>Date</th>
							</tr>
						</thead>
						<tbody>
							{#each logs as log}
								<tr class="clickable" onclick={() => window.location.href = `/evidence/research-logs/${log.id}`}>
									<td><a href="/{log.subject_type === 'family' ? 'families' : 'persons'}/{log.subject_id}" onclick={(e) => e.stopPropagation()}>{log.subject_id.slice(0, 8)}...</a></td>
									<td>{log.repository}</td>
									<td class="truncated">{truncate(log.search_description)}</td>
									<td>
										{#if log.outcome === 'found'}
											<Badge class="bg-green-50 text-green-700 border-green-200">Found</Badge>
										{:else if log.outcome === 'not_found'}
											<Badge variant="destructive">Not Found</Badge>
										{:else}
											<Badge class="bg-yellow-50 text-yellow-700 border-yellow-200">Inconclusive</Badge>
										{/if}
									</td>
									<td class="date">{formatDate(log.search_date)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="cards-wrapper mobile-only">
					{#each logs as log}
						<a href="/evidence/research-logs/{log.id}" class="card">
							<div class="card-top">
								<span class="card-fact-type">{log.repository}</span>
								{#if log.outcome === 'found'}
									<Badge class="bg-green-50 text-green-700 border-green-200">Found</Badge>
								{:else if log.outcome === 'not_found'}
									<Badge variant="destructive">Not Found</Badge>
								{:else}
									<Badge class="bg-yellow-50 text-yellow-700 border-yellow-200">Inconclusive</Badge>
								{/if}
							</div>
							<p class="card-conclusion">{truncate(log.search_description, 120)}</p>
							<div class="card-meta">
								<span>{formatDate(log.search_date)}</span>
							</div>
						</a>
					{/each}
				</div>

				{#if logsTotalPages > 1}
					<div class="pagination">
						<button onclick={() => { logsPage--; loadLogs(); }} disabled={logsPage === 1}>Previous</button>
						<span>Page {logsPage} of {logsTotalPages}</span>
						<button onclick={() => { logsPage++; loadLogs(); }} disabled={logsPage >= logsTotalPages}>Next</button>
					</div>
				{/if}
			{/if}
		</Tabs.Content>

		<!-- Proof Summaries Tab -->
		<Tabs.Content value="summaries">
			<div class="tab-header">
				<span class="tab-count">{summariesTotal} {summariesTotal === 1 ? 'summary' : 'summaries'}</span>
				<Button href="/evidence/proof-summaries/new">New Proof Summary</Button>
			</div>

			{#if summariesLoading}
				<div class="loading">Loading proof summaries...</div>
			{:else if summariesError}
				<div class="error-state">
					<p>{summariesError}</p>
					<Button variant="outline" onclick={loadSummaries}>Retry</Button>
				</div>
			{:else if summaries.length === 0}
				<div class="empty">
					<p>No proof summaries yet.</p>
					<p class="empty-hint">Create a proof summary to document your conclusions about a genealogical fact.</p>
					<Button href="/evidence/proof-summaries/new">New Proof Summary</Button>
				</div>
			{:else}
				<!-- Desktop table -->
				<div class="table-wrapper desktop-only">
					<table>
						<thead>
							<tr>
								<th>Fact Type</th>
								<th>Subject</th>
								<th>Conclusion</th>
								<th>Status</th>
								<th>Analyses</th>
							</tr>
						</thead>
						<tbody>
							{#each summaries as summary}
								<tr class="clickable" onclick={() => window.location.href = `/evidence/proof-summaries/${summary.id}`}>
									<td class="fact-type">{formatFactType(summary.fact_type)}</td>
									<td><a href="/{subjectRoute(summary.fact_type)}/{summary.subject_id}" onclick={(e) => e.stopPropagation()}>{summary.subject_id.slice(0, 8)}...</a></td>
									<td class="truncated">{truncate(summary.conclusion)}</td>
									<td>
										{#if summary.research_status}
											<UncertaintyBadge status={summary.research_status} showLabel />
										{:else}
											<span class="text-muted">--</span>
										{/if}
									</td>
									<td class="count">{summary.analysis_ids?.length ?? 0}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="cards-wrapper mobile-only">
					{#each summaries as summary}
						<a href="/evidence/proof-summaries/{summary.id}" class="card">
							<div class="card-top">
								<span class="card-fact-type">{formatFactType(summary.fact_type)}</span>
								{#if summary.research_status}
									<UncertaintyBadge status={summary.research_status} showLabel size="small" />
								{/if}
							</div>
							<p class="card-conclusion">{truncate(summary.conclusion, 120)}</p>
							<div class="card-meta">
								<span>{summary.analysis_ids?.length ?? 0} linked analyses</span>
							</div>
						</a>
					{/each}
				</div>

				{#if summariesTotalPages > 1}
					<div class="pagination">
						<button onclick={() => { summariesPage--; loadSummaries(); }} disabled={summariesPage === 1}>Previous</button>
						<span>Page {summariesPage} of {summariesTotalPages}</span>
						<button onclick={() => { summariesPage++; loadSummaries(); }} disabled={summariesPage >= summariesTotalPages}>Next</button>
					</div>
				{/if}
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>

<style>
	.evidence-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		margin-bottom: 1.5rem;
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.subtitle {
		margin: 0.25rem 0 0;
		font-size: 0.875rem;
		color: #64748b;
	}

	.tab-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
		padding-top: 0.5rem;
	}

	.tab-count {
		font-size: 0.875rem;
		color: #64748b;
	}

	.filter-buttons {
		display: flex;
		gap: 0.25rem;
	}

	.filter-btn {
		padding: 0.375rem 0.75rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.8125rem;
		color: #64748b;
		cursor: pointer;
		transition: all 0.15s;
	}

	.filter-btn:hover {
		background: #f1f5f9;
		color: #1e293b;
	}

	.filter-btn.active {
		background: #eff6ff;
		color: #3b82f6;
		border-color: #3b82f6;
	}

	/* Table styles */
	.table-wrapper {
		overflow-x: auto;
	}

	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}

	th {
		text-align: left;
		padding: 0.75rem 1rem;
		border-bottom: 2px solid #e2e8f0;
		color: #475569;
		font-weight: 600;
		white-space: nowrap;
	}

	td {
		padding: 0.75rem 1rem;
		border-bottom: 1px solid #f1f5f9;
		color: #1e293b;
	}

	tr.clickable {
		cursor: pointer;
		transition: background 0.1s;
	}

	tr.clickable:hover {
		background: #f8fafc;
	}

	.fact-type {
		font-weight: 500;
		white-space: nowrap;
	}

	.truncated {
		max-width: 300px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.count {
		text-align: center;
	}

	.date {
		white-space: nowrap;
	}

	td a {
		color: #3b82f6;
		text-decoration: none;
	}

	td a:hover {
		text-decoration: underline;
	}

	.text-muted {
		color: #94a3b8;
	}

	/* Card styles (mobile) */
	.cards-wrapper {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.card {
		display: block;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		text-decoration: none;
		color: inherit;
		transition: border-color 0.15s, box-shadow 0.15s;
	}

	.card:hover {
		border-color: #cbd5e1;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
	}

	.card-top {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	.card-fact-type {
		font-weight: 600;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.card-conclusion {
		margin: 0;
		font-size: 0.8125rem;
		color: #475569;
		line-height: 1.4;
	}

	.card-meta {
		margin-top: 0.5rem;
		font-size: 0.75rem;
		color: #94a3b8;
	}

	/* Responsive: show table on desktop, cards on mobile */
	.desktop-only {
		display: block;
	}

	.mobile-only {
		display: none;
	}

	@media (max-width: 768px) {
		.desktop-only {
			display: none;
		}

		.mobile-only {
			display: flex;
		}

		.tab-header {
			flex-wrap: wrap;
			gap: 0.5rem;
		}
	}

	/* Loading / empty / error states */
	.loading,
	.empty {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.empty p {
		margin: 0 0 0.5rem;
	}

	.empty-hint {
		font-size: 0.8125rem;
		color: #94a3b8;
		margin-bottom: 1rem !important;
	}

	.error-state {
		text-align: center;
		padding: 3rem;
		color: #dc2626;
	}

	.error-state p {
		margin: 0 0 1rem;
	}

	/* Pagination */
	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 2rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.pagination button {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.pagination button:hover:not(:disabled) {
		background: #f1f5f9;
	}

	.pagination button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.pagination span {
		font-size: 0.875rem;
		color: #64748b;
	}
</style>
