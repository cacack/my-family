<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		api,
		type ProofSummaryResponse,
		type ProofSummaryCreateRequest,
		type EvidenceAnalysisResponse
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import UncertaintyBadge from '$lib/components/UncertaintyBadge.svelte';
	import { formatFactType, subjectRoute } from '$lib/utils/evidence';
	import { nativeSelectClass } from '$lib/utils/forms';

	const factTypes = [
		'person_birth', 'person_death', 'person_name', 'person_gender',
		'family_marriage', 'family_divorce', 'person_burial', 'person_baptism',
		'person_census', 'person_immigration', 'person_emigration', 'person_naturalization',
		'person_military', 'person_graduation', 'person_retirement', 'person_occupation',
		'person_residence', 'person_education', 'person_religion', 'person_title',
		'person_description', 'person_note', 'family_annulment', 'family_engagement'
	];

	const researchStatuses = ['certain', 'probable', 'possible', 'unknown'] as const;

	let summary: ProofSummaryResponse | null = $state(null);
	let linkedAnalyses: EvidenceAnalysisResponse[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleting = $state(false);
	let isNew = $state(false);

	let formData = $state({
		fact_type: 'person_birth',
		subject_id: '',
		conclusion: '',
		argument: '',
		research_status: 'unknown' as 'certain' | 'probable' | 'possible' | 'unknown',
		analysis_ids: [] as string[]
	});

	let newAnalysisId = $state('');

	// Monotonic request id to guard against stale async completions on fast route changes.
	let loadSeq = 0;

	async function loadSummary(id: string, urlSubjectId?: string) {
		const seq = ++loadSeq;
		if (id === 'new') {
			summary = null;
			linkedAnalyses = [];
			error = null;
			newAnalysisId = '';
			formData = {
				fact_type: 'person_birth',
				subject_id: urlSubjectId ?? '',
				conclusion: '',
				argument: '',
				research_status: 'unknown',
				analysis_ids: []
			};
			isNew = true;
			editing = true;
			loading = false;
			return;
		}
		isNew = false;
		editing = false;
		loading = true;
		error = null;
		linkedAnalyses = [];
		try {
			const result = await api.getProofSummary(id);
			if (seq !== loadSeq) return;
			summary = result;
			resetForm();
			// Fetch linked analyses
			if (result.analysis_ids && result.analysis_ids.length > 0) {
				const results = await Promise.allSettled(
					result.analysis_ids.map((aid) => api.getEvidenceAnalysis(aid))
				);
				if (seq !== loadSeq) return;
				linkedAnalyses = results
					.filter((r): r is PromiseFulfilledResult<EvidenceAnalysisResponse> => r.status === 'fulfilled')
					.map((r) => r.value);
			}
		} catch (e) {
			if (seq !== loadSeq) return;
			error = (e as { message?: string }).message || 'Failed to load proof summary';
			summary = null;
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	function resetForm() {
		if (summary) {
			formData = {
				fact_type: summary.fact_type,
				subject_id: summary.subject_id,
				conclusion: summary.conclusion,
				argument: summary.argument,
				research_status: summary.research_status || 'unknown',
				analysis_ids: [...(summary.analysis_ids || [])]
			};
		}
	}

	function startEdit() {
		resetForm();
		editing = true;
	}

	function cancelEdit() {
		if (isNew) {
			goto('/evidence');
			return;
		}
		resetForm();
		editing = false;
	}

	function addAnalysis() {
		const id = newAnalysisId.trim();
		if (id && !formData.analysis_ids.includes(id)) {
			formData.analysis_ids = [...formData.analysis_ids, id];
			newAnalysisId = '';
		}
	}

	function removeAnalysis(id: string) {
		formData.analysis_ids = formData.analysis_ids.filter((a) => a !== id);
	}

	async function saveSummary() {
		if (!formData.subject_id.trim()) {
			error = 'Subject ID is required';
			return;
		}
		if (!formData.conclusion.trim()) {
			error = 'Conclusion is required';
			return;
		}
		if (!formData.argument.trim()) {
			error = 'Argument is required';
			return;
		}

		saving = true;
		error = null;
		try {
			if (isNew) {
				const data: ProofSummaryCreateRequest = {
					fact_type: formData.fact_type,
					subject_id: formData.subject_id.trim(),
					conclusion: formData.conclusion.trim(),
					argument: formData.argument.trim(),
					research_status: formData.research_status,
					analysis_ids: formData.analysis_ids
				};
				const created = await api.createProofSummary(data);
				goto(`/evidence/proof-summaries/${created.id}`);
			} else if (summary) {
				const updated = await api.updateProofSummary(summary.id, {
					fact_type: formData.fact_type,
					subject_id: formData.subject_id.trim(),
					conclusion: formData.conclusion.trim(),
					argument: formData.argument.trim(),
					research_status: formData.research_status,
					analysis_ids: formData.analysis_ids,
					version: summary.version
				});
				summary = updated;
				resetForm();
				// Refresh linked analyses since analysis_ids may have changed.
				if (updated.analysis_ids && updated.analysis_ids.length > 0) {
					const results = await Promise.allSettled(
						updated.analysis_ids.map((aid) => api.getEvidenceAnalysis(aid))
					);
					linkedAnalyses = results
						.filter((r): r is PromiseFulfilledResult<EvidenceAnalysisResponse> => r.status === 'fulfilled')
						.map((r) => r.value);
				} else {
					linkedAnalyses = [];
				}
				editing = false;
			}
		} catch (e) {
			const status = (e as { status?: number }).status;
			if (status === 409) {
				error = 'Version conflict: someone else modified this record. Please reload and try again.';
			} else {
				error = (e as { message?: string }).message || 'Failed to save';
			}
		} finally {
			saving = false;
		}
	}

	async function deleteSummary() {
		if (!summary) return;
		if (!confirm('Delete this proof summary? This cannot be undone.')) return;

		deleting = true;
		error = null;
		try {
			await api.deleteProofSummary(summary.id, summary.version);
			await goto('/evidence');
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to delete';
		} finally {
			deleting = false;
		}
	}

	$effect(() => {
		const id = $page.params.id;
		// Track subjectId so navigating ?subjectId=A → ?subjectId=B re-prefills
		const subjectId = $page.url?.searchParams?.get('subjectId');
		if (id) {
			untrack(() => loadSummary(id, subjectId ?? undefined));
		}
	});
</script>

<svelte:head>
	<title>{isNew ? 'New Proof Summary' : 'Proof Summary'} | My Family</title>
</svelte:head>

<div class="mx-auto max-w-3xl p-6">
	<header class="mb-6 flex items-center justify-between">
		<a href="/evidence" class="text-sm text-slate-500 no-underline hover:text-blue-500">
			&larr; Evidence
		</a>
		{#if summary && !editing}
			<div class="flex gap-2">
				<Button variant="outline" onclick={startEdit}>Edit</Button>
				<Button variant="destructive" onclick={deleteSummary} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete'}
				</Button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="p-12 text-center text-slate-500">Loading...</div>
	{:else if error && !summary && !isNew}
		<div class="p-12 text-center text-red-600">
			<p class="m-0 mb-4">{error}</p>
			<Button variant="outline" onclick={() => loadSummary($page.params.id!)}>Retry</Button>
		</div>
	{:else if editing}
		<form
			class="rounded-xl border border-slate-200 bg-white p-6"
			onsubmit={(e) => {
				e.preventDefault();
				saveSummary();
			}}
		>
			<h1 class="m-0 mb-6 text-xl text-slate-800">
				{isNew ? 'New Proof Summary' : 'Edit Proof Summary'}
			</h1>

			{#if error}
				<div
					class="mb-4 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-600"
					role="alert"
				>
					{error}
				</div>
			{/if}

			<div class="mb-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<Label for="fact-type" class="text-sm text-slate-600">
						Fact Type <span class="text-red-600">*</span>
					</Label>
					<select
						id="fact-type"
						bind:value={formData.fact_type}
						aria-label="Fact Type"
						class={nativeSelectClass}
					>
						{#each factTypes as ft}
							<option value={ft}>{formatFactType(ft)}</option>
						{/each}
					</select>
				</div>
				<div class="flex flex-col gap-1.5">
					<Label for="subject-id" class="text-sm text-slate-600">
						Subject ID <span class="text-red-600">*</span>
					</Label>
					<Input
						id="subject-id"
						type="text"
						bind:value={formData.subject_id}
						required
						placeholder="Person or family UUID"
						aria-label="Subject ID"
					/>
				</div>
			</div>

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="conclusion" class="text-sm text-slate-600">
					Conclusion <span class="text-red-600">*</span>
				</Label>
				<Input
					id="conclusion"
					type="text"
					bind:value={formData.conclusion}
					required
					aria-label="Conclusion"
				/>
			</div>

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="argument" class="text-sm text-slate-600">
					Argument <span class="text-red-600">*</span>
				</Label>
				<Textarea
					id="argument"
					bind:value={formData.argument}
					rows={10}
					required
					placeholder="Present the full proof argument, evaluating each piece of evidence..."
					aria-label="Argument"
				/>
			</div>

			<div class="mb-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<Label for="research-status" class="text-sm text-slate-600">Research Status</Label>
					<select
						id="research-status"
						bind:value={formData.research_status}
						aria-label="Research Status"
						class={nativeSelectClass}
					>
						{#each researchStatuses as s}
							<option value={s}>{s.charAt(0).toUpperCase() + s.slice(1)}</option>
						{/each}
					</select>
				</div>
			</div>

			<div class="mb-4">
				<h3 class="m-0 mb-2 text-sm text-slate-600">Linked Analysis IDs</h3>
				{#if formData.analysis_ids.length > 0}
					<ul class="m-0 mb-2 flex list-none flex-col gap-1 p-0">
						{#each formData.analysis_ids as aid}
							<li class="flex items-center gap-2">
								<code class="rounded bg-slate-100 px-2 py-1 text-[0.8125rem]">{aid}</code>
								<button
									type="button"
									class="rounded border-none bg-transparent px-1.5 py-0.5 text-sm text-red-600 hover:bg-red-50"
									onclick={() => removeAnalysis(aid)}
									aria-label="Remove analysis {aid}"
								>
									x
								</button>
							</li>
						{/each}
					</ul>
				{/if}
				<div class="flex items-center gap-2">
					<Input
						type="text"
						bind:value={newAnalysisId}
						placeholder="Analysis UUID"
						aria-label="New analysis ID"
						class="flex-1"
					/>
					<Button type="button" variant="outline" onclick={addAnalysis}>Add</Button>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3 border-t border-slate-200 pt-4">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Proof Summary' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if summary}
		<div class="rounded-xl border border-slate-200 bg-white p-6">
			<div class="mb-6 border-b border-slate-200 pb-4">
				<h1 class="m-0 mb-2 text-2xl text-slate-800">Proof Summary</h1>
				<div class="flex items-center gap-2">
					<Badge variant="secondary">{formatFactType(summary.fact_type)}</Badge>
					{#if summary.research_status}
						<UncertaintyBadge status={summary.research_status} showLabel />
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
								href="/{subjectRoute(summary.fact_type)}/{summary.subject_id}"
								class="text-blue-500 no-underline hover:underline"
							>
								{summary.subject_id}
							</a>
						</dd>
						<dt class="text-[0.8125rem] text-slate-400">Fact Type</dt>
						<dd class="m-0 text-sm text-slate-800">{formatFactType(summary.fact_type)}</dd>
					</dl>
				</div>
			</div>

			<div class="mb-6">
				<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
					Conclusion
				</h2>
				<p
					class="m-0 whitespace-pre-wrap text-base font-medium leading-relaxed text-slate-800"
				>
					{summary.conclusion}
				</p>
			</div>

			<div class="mb-6 rounded-lg border border-slate-200 bg-slate-50 p-5">
				<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
					Argument
				</h2>
				<div class="m-0 whitespace-pre-wrap text-[0.9375rem] leading-loose text-slate-700">
					{summary.argument}
				</div>
			</div>

			{#if linkedAnalyses.length > 0}
				<div class="mb-6">
					<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
						Supporting Analyses ({linkedAnalyses.length})
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
			{:else if summary.analysis_ids && summary.analysis_ids.length > 0}
				<div class="mb-6">
					<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
						Linked Analysis IDs ({summary.analysis_ids.length})
					</h2>
					<ul class="m-0 flex list-none flex-col gap-1.5 p-0">
						{#each summary.analysis_ids as aid}
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

			<div
				class="mt-6 flex flex-wrap gap-6 border-t border-slate-200 pt-4 text-xs text-slate-400"
			>
				{#if summary.created_at}
					<span>Created: {new Date(summary.created_at).toLocaleDateString()}</span>
				{/if}
				{#if summary.updated_at}
					<span>Updated: {new Date(summary.updated_at).toLocaleDateString()}</span>
				{/if}
				<span>Version: {summary.version}</span>
			</div>
		</div>
	{/if}
</div>
