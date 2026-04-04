<script lang="ts">
	import {
		api,
		type EvidenceAnalysisResponse,
		type EvidenceConflictResponse,
		type ResearchLogResponse
	} from '$lib/api/client';
	import UncertaintyBadge from './UncertaintyBadge.svelte';
	import { Badge } from '$lib/components/ui/badge';

	interface Props {
		subjectId: string;
	}

	let { subjectId }: Props = $props();

	let analyses: EvidenceAnalysisResponse[] = $state([]);
	let conflicts: EvidenceConflictResponse[] = $state([]);
	let researchLogs: ResearchLogResponse[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let expanded = $state(false);

	let openConflictCount = $derived(conflicts.filter((c) => c.status === 'open').length);
	let hasData = $derived(analyses.length > 0 || conflicts.length > 0 || researchLogs.length > 0);

	let analysesByFactType = $derived(() => {
		const grouped = new Map<string, EvidenceAnalysisResponse[]>();
		for (const a of analyses) {
			const key = a.fact_type;
			if (!grouped.has(key)) grouped.set(key, []);
			grouped.get(key)!.push(a);
		}
		return grouped;
	});

	function formatFactType(type: string): string {
		return type
			.replace(/^(person_|family_)/, '')
			.replace(/_/g, ' ')
			.replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function subjectRoute(factType: string): string {
		return factType.startsWith('family_') ? 'families' : 'persons';
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString();
	}

	function summaryText(): string {
		const parts: string[] = [];
		if (analyses.length > 0) {
			parts.push(`${analyses.length} ${analyses.length === 1 ? 'analysis' : 'analyses'}`);
		}
		if (conflicts.length > 0) {
			const openCount = openConflictCount;
			const label = `${conflicts.length} ${conflicts.length === 1 ? 'conflict' : 'conflicts'}`;
			parts.push(openCount > 0 ? `${label} (${openCount} open)` : label);
		}
		if (researchLogs.length > 0) {
			parts.push(
				`${researchLogs.length} research ${researchLogs.length === 1 ? 'log' : 'logs'}`
			);
		}
		return parts.join(', ');
	}

	async function loadData() {
		loading = true;
		error = null;
		try {
			// TODO: Add GET /evidence-analyses/by-subject/{subjectId} backend endpoint
			// to avoid over-fetching. Currently no subject-scoped analyses API exists,
			// so we fetch globally and filter client-side.
			const [analysisResult, conflictResult, logResult] = await Promise.all([
				api.listEvidenceAnalyses({ limit: 100 }),
				api.getConflictsBySubject(subjectId),
				api.getResearchLogsBySubject(subjectId)
			]);
			analyses = analysisResult.analyses.filter((a) => a.subject_id === subjectId);
			conflicts = conflictResult;
			researchLogs = logResult;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load evidence data';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (subjectId) {
			loadData();
		}
	});
</script>

<div class="evidence-panel">
	<button class="panel-header" onclick={() => (expanded = !expanded)}>
		<h2>
			Evidence & Research
			{#if !loading && hasData}
				<span class="summary-text">{summaryText()}</span>
			{/if}
			{#if openConflictCount > 0}
				<Badge variant="destructive" class="ml-1 text-[0.625rem]">{openConflictCount}</Badge>
			{/if}
		</h2>
		<span class="expand-icon">{expanded ? '\u2212' : '+'}</span>
	</button>

	{#if expanded}
		<div class="panel-content">
			{#if loading}
				<div class="loading-state" role="status" aria-live="polite">Loading evidence data...</div>
			{:else if error}
				<div class="error-state" role="alert">{error}</div>
			{:else if !hasData}
				<div class="empty-state">
					<p>No evidence data yet. Start by adding an analysis or logging your research.</p>
					<div class="empty-actions">
						<a href="/evidence/analyses/new?subjectId={subjectId}" class="action-link"
							>Add Analysis</a
						>
						<a href="/evidence/research-logs/new?subjectId={subjectId}" class="action-link"
							>Log Research</a
						>
					</div>
				</div>
			{:else}
				<!-- Analyses -->
				{#if analyses.length > 0}
					<div class="sub-section">
						<div class="sub-header">
							<h3>Analyses <span class="count-badge">{analyses.length}</span></h3>
							<a href="/evidence/analyses/new?subjectId={subjectId}" class="add-link"
								>Add Analysis</a
							>
						</div>
						{#each [...analysesByFactType().entries()] as [factType, items]}
							<div class="fact-group">
								<h4 class="fact-type-label">{formatFactType(factType)}</h4>
								<ul class="analysis-list">
									{#each items as analysis}
										<li class="analysis-item">
											<a href="/evidence/analyses/{analysis.id}" class="analysis-link">
												<span class="analysis-conclusion">{analysis.conclusion}</span>
												<span class="analysis-meta">
													{#if analysis.research_status}
														<UncertaintyBadge
															status={analysis.research_status}
															size="small"
															showLabel={true}
														/>
													{/if}
													{#if analysis.citation_ids && analysis.citation_ids.length > 0}
														<span class="citation-count"
															>{analysis.citation_ids.length}
															{analysis.citation_ids.length === 1
																? 'citation'
																: 'citations'}</span
														>
													{/if}
												</span>
											</a>
										</li>
									{/each}
								</ul>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Conflicts -->
				{#if conflicts.length > 0}
					<div class="sub-section">
						<div class="sub-header">
							<h3>Conflicts <span class="count-badge">{conflicts.length}</span></h3>
						</div>
						<ul class="conflict-list">
							{#each conflicts as conflict}
								<li
									class="conflict-item"
									class:conflict-open={conflict.status === 'open'}
									class:conflict-resolved={conflict.status === 'resolved'}
								>
									<a href="/evidence/conflicts/{conflict.id}" class="conflict-link">
										<div class="conflict-header-row">
											<span class="conflict-description">{conflict.description}</span>
											<Badge
												variant={conflict.status === 'open' ? 'destructive' : 'secondary'}
												class="text-[0.625rem] uppercase"
											>
												{conflict.status}
											</Badge>
										</div>
										{#if conflict.analysis_ids && conflict.analysis_ids.length > 0}
											<span class="conflict-analyses"
												>{conflict.analysis_ids.length} linked
												{conflict.analysis_ids.length === 1
													? 'analysis'
													: 'analyses'}</span
											>
										{/if}
									</a>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				<!-- Research Logs -->
				{#if researchLogs.length > 0}
					<div class="sub-section">
						<div class="sub-header">
							<h3>
								Research Logs <span class="count-badge">{researchLogs.length}</span>
							</h3>
							<a
								href="/evidence/research-logs/new?subjectId={subjectId}"
								class="add-link">Log Research</a
							>
						</div>
						<ul class="log-list">
							{#each researchLogs as log}
								<li class="log-item">
									<div class="log-date">{formatDate(log.search_date)}</div>
									<div class="log-body">
										<div class="log-header-row">
											<span class="log-repository">{log.repository}</span>
											<Badge
												variant="outline"
												class={log.outcome === 'found'
													? 'border-green-200 bg-green-50 text-green-700 dark:border-green-800 dark:bg-green-950 dark:text-green-400'
													: log.outcome === 'not_found'
														? 'border-red-200 bg-red-50 text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-400'
														: 'border-yellow-200 bg-yellow-50 text-yellow-700 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-400'}
											>
												{log.outcome.replace('_', ' ')}
											</Badge>
										</div>
										<span class="log-description">{log.search_description}</span>
									</div>
								</li>
							{/each}
						</ul>
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</div>

<style>
	.evidence-panel {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid #e2e8f0;
	}

	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		padding: 0;
		border: none;
		background: none;
		cursor: pointer;
		text-align: left;
	}

	.panel-header h2 {
		display: flex;
		align-items: center;
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.summary-text {
		margin-left: 0.75rem;
		font-size: 0.75rem;
		font-weight: 400;
		color: #94a3b8;
		text-transform: none;
		letter-spacing: 0;
	}

	.expand-icon {
		font-size: 1.25rem;
		font-weight: 600;
		color: #64748b;
	}

	.panel-content {
		margin-top: 1rem;
	}

	.loading-state,
	.empty-state {
		text-align: center;
		padding: 1.5rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	.empty-state p {
		margin: 0 0 1rem;
	}

	.empty-actions {
		display: flex;
		gap: 1rem;
		justify-content: center;
	}

	.error-state {
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		color: #dc2626;
		font-size: 0.875rem;
	}

	.sub-section {
		margin-bottom: 1.25rem;
	}

	.sub-section:last-child {
		margin-bottom: 0;
	}

	.sub-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	.sub-header h3 {
		margin: 0;
		font-size: 0.8125rem;
		font-weight: 600;
		color: #475569;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.count-badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.125rem;
		height: 1.125rem;
		padding: 0 0.3rem;
		background: #dbeafe;
		border-radius: 9999px;
		font-size: 0.625rem;
		font-weight: 600;
		color: #3b82f6;
	}

	.add-link {
		font-size: 0.75rem;
		color: #3b82f6;
		text-decoration: none;
	}

	.add-link:hover {
		text-decoration: underline;
	}

	.action-link {
		font-size: 0.8125rem;
		color: #3b82f6;
		text-decoration: none;
		padding: 0.375rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
	}

	.action-link:hover {
		background: #f1f5f9;
	}

	/* Analyses */
	.fact-group {
		margin-bottom: 0.75rem;
	}

	.fact-group:last-child {
		margin-bottom: 0;
	}

	.fact-type-label {
		margin: 0 0 0.25rem;
		font-size: 0.75rem;
		font-weight: 500;
		color: #94a3b8;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.analysis-list,
	.conflict-list,
	.log-list {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.analysis-item {
		margin-bottom: 0.25rem;
	}

	.analysis-link {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.5rem 0.625rem;
		border-radius: 6px;
		text-decoration: none;
		color: #1e293b;
		transition: background 0.15s;
	}

	.analysis-link:hover {
		background: #f1f5f9;
	}

	.analysis-conclusion {
		font-size: 0.8125rem;
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.analysis-meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-shrink: 0;
	}

	.citation-count {
		font-size: 0.6875rem;
		color: #94a3b8;
	}

	/* Conflicts */
	.conflict-item {
		margin-bottom: 0.5rem;
		border-radius: 6px;
		overflow: hidden;
	}

	.conflict-item.conflict-open {
		background: #fffbeb;
		border: 1px solid #fde68a;
	}

	.conflict-item.conflict-resolved {
		background: #f8fafc;
		border: 1px solid #e2e8f0;
	}

	.conflict-link {
		display: block;
		padding: 0.625rem 0.75rem;
		text-decoration: none;
		color: #1e293b;
	}

	.conflict-link:hover {
		opacity: 0.85;
	}

	.conflict-header-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.conflict-description {
		font-size: 0.8125rem;
		flex: 1;
	}

	.conflict-analyses {
		display: block;
		font-size: 0.6875rem;
		color: #94a3b8;
		margin-top: 0.25rem;
	}

	/* Research Logs */
	.log-item {
		display: flex;
		gap: 0.75rem;
		padding: 0.5rem 0;
		border-bottom: 1px solid #f1f5f9;
	}

	.log-item:last-child {
		border-bottom: none;
	}

	.log-date {
		flex-shrink: 0;
		font-size: 0.6875rem;
		color: #94a3b8;
		min-width: 5rem;
		padding-top: 0.125rem;
	}

	.log-body {
		flex: 1;
		min-width: 0;
	}

	.log-header-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.125rem;
	}

	.log-repository {
		font-size: 0.8125rem;
		font-weight: 500;
		color: #1e293b;
	}

	.log-description {
		display: block;
		font-size: 0.75rem;
		color: #64748b;
	}
</style>
