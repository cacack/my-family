<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		api,
		type EvidenceAnalysisResponse,
		type EvidenceAnalysisCreateRequest
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

	let analysis: EvidenceAnalysisResponse | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleting = $state(false);
	let isNew = $state(false);

	function emptyFormData(subjectId = '') {
		return {
			fact_type: 'person_birth',
			subject_id: subjectId,
			conclusion: '',
			research_status: 'unknown' as 'certain' | 'probable' | 'possible' | 'unknown',
			notes: '',
			citation_ids: [] as string[]
		};
	}

	let formData = $state(emptyFormData());

	let newCitationId = $state('');

	// Monotonic request id to guard against stale async completions on fast route changes.
	let loadSeq = 0;

	async function loadAnalysis(id: string, urlSubjectId?: string) {
		const seq = ++loadSeq;
		if (id === 'new') {
			analysis = null;
			error = null;
			newCitationId = '';
			formData = emptyFormData(urlSubjectId ?? '');
			isNew = true;
			editing = true;
			loading = false;
			return;
		}
		isNew = false;
		editing = false;
		loading = true;
		error = null;
		try {
			const result = await api.getEvidenceAnalysis(id);
			if (seq !== loadSeq) return;
			analysis = result;
			resetForm();
		} catch (e) {
			if (seq !== loadSeq) return;
			error = (e as { message?: string }).message || 'Failed to load analysis';
			analysis = null;
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	function resetForm() {
		if (analysis) {
			formData = {
				fact_type: analysis.fact_type,
				subject_id: analysis.subject_id,
				conclusion: analysis.conclusion,
				research_status: analysis.research_status || 'unknown',
				notes: analysis.notes || '',
				citation_ids: [...(analysis.citation_ids || [])]
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

	function addCitation() {
		const id = newCitationId.trim();
		if (id && !formData.citation_ids.includes(id)) {
			formData.citation_ids = [...formData.citation_ids, id];
			newCitationId = '';
		}
	}

	function removeCitation(id: string) {
		formData.citation_ids = formData.citation_ids.filter((c) => c !== id);
	}

	async function saveAnalysis() {
		if (!formData.subject_id.trim()) {
			error = 'Subject ID is required';
			return;
		}
		if (!formData.conclusion.trim()) {
			error = 'Conclusion is required';
			return;
		}

		saving = true;
		error = null;
		try {
			if (isNew) {
				const data: EvidenceAnalysisCreateRequest = {
					fact_type: formData.fact_type,
					subject_id: formData.subject_id.trim(),
					conclusion: formData.conclusion.trim(),
					research_status: formData.research_status,
					notes: formData.notes.trim() || undefined,
					citation_ids: formData.citation_ids
				};
				const created = await api.createEvidenceAnalysis(data);
				goto(`/evidence/analyses/${created.id}`);
			} else if (analysis) {
				const updated = await api.updateEvidenceAnalysis(analysis.id, {
					fact_type: formData.fact_type,
					subject_id: formData.subject_id.trim(),
					conclusion: formData.conclusion.trim(),
					research_status: formData.research_status,
					notes: formData.notes.trim() || undefined,
					citation_ids: formData.citation_ids,
					version: analysis.version
				});
				analysis = updated;
				resetForm();
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

	async function deleteAnalysis() {
		if (!analysis) return;
		if (!confirm('Delete this analysis? This cannot be undone.')) return;

		deleting = true;
		error = null;
		try {
			await api.deleteEvidenceAnalysis(analysis.id, analysis.version);
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
			untrack(() => loadAnalysis(id, subjectId ?? undefined));
		}
	});
</script>

<svelte:head>
	<title>{isNew ? 'New Analysis' : analysis ? 'Analysis' : 'Evidence Analysis'} | My Family</title>
</svelte:head>

<div class="mx-auto max-w-3xl p-6">
	<header class="mb-6 flex items-center justify-between">
		<a href="/evidence" class="text-sm text-slate-500 no-underline hover:text-blue-500">
			&larr; Evidence
		</a>
		{#if analysis && !editing}
			<div class="flex gap-2">
				<Button variant="outline" onclick={startEdit}>Edit</Button>
				<Button variant="destructive" onclick={deleteAnalysis} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete'}
				</Button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="p-12 text-center text-slate-500">Loading...</div>
	{:else if error && !analysis && !isNew}
		<div class="p-12 text-center text-red-600">
			<p class="m-0 mb-4">{error}</p>
			<Button variant="outline" onclick={() => loadAnalysis($page.params.id!)}>Retry</Button>
		</div>
	{:else if editing}
		<form
			class="rounded-xl border border-slate-200 bg-white p-6"
			onsubmit={(e) => {
				e.preventDefault();
				saveAnalysis();
			}}
		>
			<h1 class="m-0 mb-6 text-xl text-slate-800">
				{isNew ? 'New Evidence Analysis' : 'Edit Analysis'}
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
				<Textarea
					id="conclusion"
					bind:value={formData.conclusion}
					rows={3}
					required
					aria-label="Conclusion"
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

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="notes" class="text-sm text-slate-600">Notes</Label>
				<Textarea id="notes" bind:value={formData.notes} rows={3} aria-label="Notes" />
			</div>

			<div class="mb-4">
				<h3 class="m-0 mb-2 text-sm text-slate-600">Citation IDs</h3>
				{#if formData.citation_ids.length > 0}
					<ul class="m-0 mb-2 flex list-none flex-col gap-1 p-0">
						{#each formData.citation_ids as cid}
							<li class="flex items-center gap-2">
								<code class="rounded bg-slate-100 px-2 py-1 text-[0.8125rem]">{cid}</code>
								<button
									type="button"
									class="rounded border-none bg-transparent px-1.5 py-0.5 text-sm text-red-600 hover:bg-red-50"
									onclick={() => removeCitation(cid)}
									aria-label="Remove citation {cid}"
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
						bind:value={newCitationId}
						placeholder="Citation UUID"
						aria-label="New citation ID"
						class="flex-1"
					/>
					<Button type="button" variant="outline" onclick={addCitation}>Add</Button>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3 border-t border-slate-200 pt-4">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Analysis' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if analysis}
		<div class="rounded-xl border border-slate-200 bg-white p-6">
			<div class="mb-6 border-b border-slate-200 pb-4">
				<h1 class="m-0 mb-2 text-2xl text-slate-800">Evidence Analysis</h1>
				<div class="flex items-center gap-2">
					<Badge variant="secondary">{formatFactType(analysis.fact_type)}</Badge>
					{#if analysis.research_status}
						<UncertaintyBadge status={analysis.research_status} showLabel />
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
								href="/{subjectRoute(analysis.fact_type)}/{analysis.subject_id}"
								class="text-blue-500 no-underline hover:underline"
							>
								{analysis.subject_id}
							</a>
						</dd>
						<dt class="text-[0.8125rem] text-slate-400">Fact Type</dt>
						<dd class="m-0 text-sm text-slate-800">{formatFactType(analysis.fact_type)}</dd>
						{#if analysis.conflict_id}
							<dt class="text-[0.8125rem] text-slate-400">Conflict</dt>
							<dd class="m-0 text-sm text-slate-800">
								<a
									href="/evidence/conflicts/{analysis.conflict_id}"
									class="text-blue-500 no-underline hover:underline"
								>
									{analysis.conflict_id.slice(0, 8)}...
								</a>
							</dd>
						{/if}
					</dl>
				</div>
			</div>

			<div class="mb-6">
				<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
					Conclusion
				</h2>
				<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-slate-600">
					{analysis.conclusion}
				</p>
			</div>

			{#if analysis.notes}
				<div class="mb-6">
					<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
						Notes
					</h2>
					<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-slate-600">
						{analysis.notes}
					</p>
				</div>
			{/if}

			{#if analysis.citation_ids && analysis.citation_ids.length > 0}
				<div class="mb-6">
					<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
						Citations ({analysis.citation_ids.length})
					</h2>
					<ul class="m-0 flex list-none flex-col gap-1.5 p-0">
						{#each analysis.citation_ids as cid}
							<li>
								<code class="rounded bg-slate-100 px-2 py-1 text-[0.8125rem]">{cid}</code>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			<div
				class="mt-6 flex flex-wrap gap-6 border-t border-slate-200 pt-4 text-xs text-slate-400"
			>
				{#if analysis.created_at}
					<span>Created: {new Date(analysis.created_at).toLocaleDateString()}</span>
				{/if}
				{#if analysis.updated_at}
					<span>Updated: {new Date(analysis.updated_at).toLocaleDateString()}</span>
				{/if}
				<span>Version: {analysis.version}</span>
			</div>
		</div>
	{/if}
</div>
