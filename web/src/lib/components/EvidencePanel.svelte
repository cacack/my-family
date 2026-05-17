<script lang="ts">
	import { untrack } from 'svelte';
	import {
		api,
		type EvidenceAnalysisResponse,
		type EvidenceConflictResponse,
		type ResearchLogResponse
	} from '$lib/api/client';
	import UncertaintyBadge from './UncertaintyBadge.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { formatFactTypeShort, subjectRoute, formatDate } from '$lib/utils/evidence';

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

	let analysesByFactType = $derived.by(() => {
		const grouped = new Map<string, EvidenceAnalysisResponse[]>();
		for (const a of analyses) {
			const key = a.fact_type;
			if (!grouped.has(key)) grouped.set(key, []);
			grouped.get(key)!.push(a);
		}
		return grouped;
	});

	// Use formatFactTypeShort in person/family context where prefix is redundant
	const formatFactType = formatFactTypeShort;

	function outcomeBadgeClass(outcome: string): string {
		switch (outcome) {
			case 'found':
				return 'border-green-200 bg-green-50 text-green-700 dark:border-green-800 dark:bg-green-950 dark:text-green-400';
			case 'not_found':
				return 'border-red-200 bg-red-50 text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-400';
			default:
				return 'border-yellow-200 bg-yellow-50 text-yellow-700 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-400';
		}
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
			untrack(() => loadData());
		}
	});
</script>

<div class="mt-6 border-t border-slate-200 pt-6">
	<button
		class="flex w-full cursor-pointer items-center justify-between border-none bg-transparent p-0 text-left"
		onclick={() => (expanded = !expanded)}
		aria-expanded={expanded}
		aria-controls="evidence-panel-content"
	>
		<h2 class="m-0 flex items-center text-sm font-semibold uppercase tracking-wider text-slate-500">
			Evidence &amp; Research
			{#if !loading && hasData}
				<span class="ml-3 text-xs font-normal normal-case tracking-normal text-slate-400">
					{summaryText()}
				</span>
			{/if}
			{#if openConflictCount > 0}
				<Badge variant="destructive" class="ml-1 text-[0.625rem]">{openConflictCount}</Badge>
			{/if}
		</h2>
		<span class="text-xl font-semibold text-slate-500">{expanded ? '−' : '+'}</span>
	</button>

	{#if expanded}
		<div class="mt-4" id="evidence-panel-content">
			{#if loading}
				<div class="p-6 text-center text-sm text-slate-500" role="status" aria-live="polite">
					Loading evidence data...
				</div>
			{:else if error}
				<div
					class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-600"
					role="alert"
				>
					{error}
				</div>
			{:else if !hasData}
				<div class="p-6 text-center text-sm text-slate-500">
					<p class="m-0 mb-4">
						No evidence data yet. Start by adding an analysis or logging your research.
					</p>
					<div class="flex justify-center gap-4">
						<a
							href="/evidence/analyses/new?subjectId={subjectId}"
							class="rounded-md border border-slate-200 px-3 py-1.5 text-[0.8125rem] text-blue-500 no-underline hover:bg-slate-100"
						>
							Add Analysis
						</a>
						<a
							href="/evidence/research-logs/new?subjectId={subjectId}"
							class="rounded-md border border-slate-200 px-3 py-1.5 text-[0.8125rem] text-blue-500 no-underline hover:bg-slate-100"
						>
							Log Research
						</a>
					</div>
				</div>
			{:else}
				<!-- Analyses -->
				{#if analyses.length > 0}
					<div class="mb-5 last:mb-0">
						<div class="mb-2 flex items-center justify-between">
							<h3 class="m-0 flex items-center gap-2 text-[0.8125rem] font-semibold text-slate-600">
								Analyses
								<span
									class="inline-flex h-[1.125rem] min-w-[1.125rem] items-center justify-center rounded-full bg-blue-100 px-[0.3rem] text-[0.625rem] font-semibold text-blue-500"
								>
									{analyses.length}
								</span>
							</h3>
							<a
								href="/evidence/analyses/new?subjectId={subjectId}"
								class="text-xs text-blue-500 no-underline hover:underline"
							>
								Add Analysis
							</a>
						</div>
						{#each [...analysesByFactType.entries()] as [factType, items]}
							<div class="mb-3 last:mb-0">
								<h4
									class="m-0 mb-1 text-xs font-medium uppercase tracking-wide text-slate-400"
								>
									{formatFactType(factType)}
								</h4>
								<ul class="m-0 list-none p-0">
									{#each items as analysis}
										<li class="mb-1">
											<a
												href="/evidence/analyses/{analysis.id}"
												class="flex items-center justify-between gap-3 rounded-md px-2.5 py-2 text-slate-800 no-underline transition-colors hover:bg-slate-100"
											>
												<span
													class="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-[0.8125rem]"
												>
													{analysis.conclusion}
												</span>
												<span class="flex flex-shrink-0 items-center gap-2">
													{#if analysis.research_status}
														<UncertaintyBadge
															status={analysis.research_status}
															size="small"
															showLabel={true}
														/>
													{/if}
													{#if analysis.citation_ids && analysis.citation_ids.length > 0}
														<span class="text-[0.6875rem] text-slate-400">
															{analysis.citation_ids.length}
															{analysis.citation_ids.length === 1 ? 'citation' : 'citations'}
														</span>
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
					<div class="mb-5 last:mb-0">
						<div class="mb-2 flex items-center justify-between">
							<h3 class="m-0 flex items-center gap-2 text-[0.8125rem] font-semibold text-slate-600">
								Conflicts
								<span
									class="inline-flex h-[1.125rem] min-w-[1.125rem] items-center justify-center rounded-full bg-blue-100 px-[0.3rem] text-[0.625rem] font-semibold text-blue-500"
								>
									{conflicts.length}
								</span>
							</h3>
						</div>
						<ul class="m-0 list-none p-0">
							{#each conflicts as conflict}
								<li
									class="mb-2 overflow-hidden rounded-md border {conflict.status === 'open'
										? 'border-amber-200 bg-amber-50'
										: 'border-slate-200 bg-slate-50'}"
								>
									<a
										href="/evidence/conflicts/{conflict.id}"
										class="block px-3 py-2.5 text-slate-800 no-underline hover:opacity-85"
									>
										<div class="flex items-center justify-between gap-2">
											<span class="flex-1 text-[0.8125rem]">{conflict.description}</span>
											<Badge
												variant={conflict.status === 'open' ? 'destructive' : 'secondary'}
												class="text-[0.625rem] uppercase"
											>
												{conflict.status}
											</Badge>
										</div>
										{#if conflict.analysis_ids && conflict.analysis_ids.length > 0}
											<span class="mt-1 block text-[0.6875rem] text-slate-400">
												{conflict.analysis_ids.length} linked
												{conflict.analysis_ids.length === 1 ? 'analysis' : 'analyses'}
											</span>
										{/if}
									</a>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				<!-- Research Logs -->
				{#if researchLogs.length > 0}
					<div class="mb-5 last:mb-0">
						<div class="mb-2 flex items-center justify-between">
							<h3 class="m-0 flex items-center gap-2 text-[0.8125rem] font-semibold text-slate-600">
								Research Logs
								<span
									class="inline-flex h-[1.125rem] min-w-[1.125rem] items-center justify-center rounded-full bg-blue-100 px-[0.3rem] text-[0.625rem] font-semibold text-blue-500"
								>
									{researchLogs.length}
								</span>
							</h3>
							<a
								href="/evidence/research-logs/new?subjectId={subjectId}"
								class="text-xs text-blue-500 no-underline hover:underline"
							>
								Log Research
							</a>
						</div>
						<ul class="m-0 list-none p-0">
							{#each researchLogs as log}
								<li class="flex gap-3 border-b border-slate-100 py-2 last:border-b-0">
									<div class="min-w-20 flex-shrink-0 pt-0.5 text-[0.6875rem] text-slate-400">
										{formatDate(log.search_date)}
									</div>
									<div class="min-w-0 flex-1">
										<div class="mb-0.5 flex items-center gap-2">
											<span class="text-[0.8125rem] font-medium text-slate-800">
												{log.repository}
											</span>
											<Badge variant="outline" class={outcomeBadgeClass(log.outcome)}>
												{log.outcome.replace('_', ' ')}
											</Badge>
										</div>
										<span class="block text-xs text-slate-500">{log.search_description}</span>
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
