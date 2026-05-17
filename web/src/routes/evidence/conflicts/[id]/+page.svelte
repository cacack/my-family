<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/stores';
	import {
		api,
		type EvidenceConflictResponse,
		type EvidenceAnalysisResponse
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import UncertaintyBadge from '$lib/components/UncertaintyBadge.svelte';
	import { formatFactType, subjectRoute } from '$lib/utils/evidence';

	let conflict: EvidenceConflictResponse | null = $state(null);
	let linkedAnalyses: EvidenceAnalysisResponse[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let resolving = $state(false);
	let resolutionText = $state('');

	async function loadConflict(id: string) {
		loading = true;
		error = null;
		linkedAnalyses = [];
		try {
			conflict = await api.getEvidenceConflict(id);
			// Fetch linked analyses in parallel
			if (conflict.analysis_ids && conflict.analysis_ids.length > 0) {
				const results = await Promise.allSettled(
					conflict.analysis_ids.map((aid) => api.getEvidenceAnalysis(aid))
				);
				linkedAnalyses = results
					.filter((r): r is PromiseFulfilledResult<EvidenceAnalysisResponse> => r.status === 'fulfilled')
					.map((r) => r.value);
			}
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load conflict';
			conflict = null;
		} finally {
			loading = false;
		}
	}

	async function resolveConflict() {
		if (!conflict) return;
		if (!resolutionText.trim()) {
			error = 'Resolution text is required';
			return;
		}

		resolving = true;
		error = null;
		try {
			await api.resolveEvidenceConflict(conflict.id, {
				resolution: resolutionText.trim(),
				version: conflict.version
			});
			await loadConflict(conflict.id);
			resolutionText = '';
		} catch (e) {
			const status = (e as { status?: number }).status;
			if (status === 409) {
				error = 'Version conflict: someone else modified this record. Please reload and try again.';
			} else {
				error = (e as { message?: string }).message || 'Failed to resolve conflict';
			}
		} finally {
			resolving = false;
		}
	}

	$effect(() => {
		const id = $page.params.id;
		if (id) {
			untrack(() => loadConflict(id));
		}
	});
</script>

<svelte:head>
	<title>Evidence Conflict | My Family</title>
</svelte:head>

<div class="mx-auto max-w-3xl p-6">
	<header class="mb-6 flex items-center justify-between">
		<a href="/evidence" class="text-sm text-slate-500 no-underline hover:text-blue-500">
			&larr; Evidence
		</a>
	</header>

	{#if loading}
		<div class="p-12 text-center text-slate-500">Loading...</div>
	{:else if error && !conflict}
		<div class="p-12 text-center text-red-600">
			<p class="m-0 mb-4">{error}</p>
			<Button variant="outline" onclick={() => loadConflict($page.params.id!)}>Retry</Button>
		</div>
	{:else if conflict}
		<div class="rounded-xl border border-slate-200 bg-white p-6">
			<div class="mb-6 border-b border-slate-200 pb-4">
				<h1 class="m-0 mb-2 text-2xl text-slate-800">Evidence Conflict</h1>
				<div class="flex items-center gap-2">
					<Badge variant="secondary">{formatFactType(conflict.fact_type)}</Badge>
					{#if conflict.status === 'open'}
						<Badge variant="destructive">Open</Badge>
					{:else}
						<Badge class="border-green-200 bg-green-50 text-green-700">Resolved</Badge>
					{/if}
				</div>
			</div>

			{#if error}
				<div
					class="mb-4 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-600"
					role="alert"
				>
					{error}
				</div>
			{/if}

			<div
				class="mb-6 grid grid-cols-[repeat(auto-fit,minmax(250px,1fr))] gap-6"
			>
				<div class="mb-6">
					<h2
						class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500"
					>
						Details
					</h2>
					<dl class="m-0 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
						<dt class="text-[0.8125rem] text-slate-400">Subject</dt>
						<dd class="m-0 text-sm text-slate-800">
							<a
								href="/{subjectRoute(conflict.fact_type)}/{conflict.subject_id}"
								class="text-blue-500 no-underline hover:underline"
							>
								{conflict.subject_id}
							</a>
						</dd>
						<dt class="text-[0.8125rem] text-slate-400">Fact Type</dt>
						<dd class="m-0 text-sm text-slate-800">{formatFactType(conflict.fact_type)}</dd>
						<dt class="text-[0.8125rem] text-slate-400">Status</dt>
						<dd
							class="m-0 text-sm font-medium {conflict.status === 'open'
								? 'text-red-600'
								: 'text-green-600'}"
						>
							{conflict.status.charAt(0).toUpperCase() + conflict.status.slice(1)}
						</dd>
					</dl>
				</div>
			</div>

			<div class="mb-6">
				<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
					Description
				</h2>
				<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-slate-600">
					{conflict.description}
				</p>
			</div>

			{#if conflict.resolution}
				<div class="mb-6 rounded-lg border border-green-200 bg-green-50 p-4">
					<h2
						class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500"
					>
						Resolution
					</h2>
					<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-slate-600">
						{conflict.resolution}
					</p>
				</div>
			{/if}

			{#if linkedAnalyses.length > 0}
				<div class="mb-6">
					<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
						Linked Analyses ({linkedAnalyses.length})
					</h2>
					<div class="flex flex-col gap-3">
						{#each linkedAnalyses as la}
							<a
								href="/evidence/analyses/{la.id}"
								class="block rounded-lg border border-slate-200 p-4 text-inherit no-underline transition-colors hover:border-blue-500"
							>
								<div class="mb-2 flex items-center justify-between">
									<span class="text-sm font-semibold text-slate-800">
										{formatFactType(la.fact_type)}
									</span>
									{#if la.research_status}
										<UncertaintyBadge status={la.research_status} showLabel size="small" />
									{/if}
								</div>
								<p class="m-0 text-[0.8125rem] leading-snug text-slate-600">{la.conclusion}</p>
								{#if la.citation_ids && la.citation_ids.length > 0}
									<span class="mt-2 inline-block text-xs text-slate-400">
										{la.citation_ids.length} citations
									</span>
								{/if}
							</a>
						{/each}
					</div>
				</div>
			{:else if conflict.analysis_ids && conflict.analysis_ids.length > 0}
				<div class="mb-6">
					<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
						Linked Analysis IDs ({conflict.analysis_ids.length})
					</h2>
					<ul class="m-0 flex list-none flex-col gap-1.5 p-0">
						{#each conflict.analysis_ids as aid}
							<li>
								<a
									href="/evidence/analyses/{aid}"
									class="text-blue-500 no-underline hover:underline"
								>
									<code class="rounded bg-slate-100 px-2 py-1 text-[0.8125rem]">{aid}</code>
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			{#if conflict.status === 'open'}
				<div class="mt-6 border-t-2 border-red-200 pt-6">
					<h2 class="m-0 mb-2 text-base font-semibold text-slate-800">Resolve Conflict</h2>
					<p class="m-0 mb-4 text-[0.8125rem] text-slate-500">
						Provide a resolution explaining how this conflict was addressed.
					</p>
					<div class="flex flex-col gap-1.5">
						<Label for="resolution" class="text-sm text-slate-600">
							Resolution <span class="text-red-600">*</span>
						</Label>
						<Textarea
							id="resolution"
							bind:value={resolutionText}
							rows={4}
							placeholder="Describe how this conflict was resolved..."
							aria-label="Resolution text"
						/>
					</div>
					<div class="mt-4 flex justify-end">
						<Button onclick={resolveConflict} disabled={resolving}>
							{resolving ? 'Resolving...' : 'Resolve Conflict'}
						</Button>
					</div>
				</div>
			{/if}

			<div
				class="mt-6 flex flex-wrap gap-6 border-t border-slate-200 pt-4 text-xs text-slate-400"
			>
				{#if conflict.created_at}
					<span>Created: {new Date(conflict.created_at).toLocaleDateString()}</span>
				{/if}
				{#if conflict.updated_at}
					<span>Updated: {new Date(conflict.updated_at).toLocaleDateString()}</span>
				{/if}
				<span>Version: {conflict.version}</span>
			</div>
		</div>
	{/if}
</div>
