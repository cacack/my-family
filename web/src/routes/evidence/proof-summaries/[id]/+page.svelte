<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		api,
		isVersionConflict,
		recordLoadError,
		type ProofSummaryResponse,
		type ProofSummaryCreateRequest,
		type EvidenceAnalysisResponse,
		type FactType,
		type ResearchStatus
	} from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import UncertaintyBadge from '#lib/components/UncertaintyBadge.svelte';
	import { FACT_TYPES, formatFactType, subjectTypeOf } from '#lib/utils/evidence.js';
	import { SubjectNames, subjectHref } from '#lib/utils/subjectNames.svelte.js';
	import { nativeSelectClass } from '#lib/utils/forms.js';
	import { RESEARCH_STATUS_OPTIONS, optionLabel } from '#lib/utils/enumOptions.js';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import ActionBar from '#lib/components/ActionBar.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import InfoSection from '#lib/components/InfoSection.svelte';
	import FormRow from '#lib/components/FormRow.svelte';
	import SaveConflict from '#lib/components/SaveConflict.svelte';
	import { SaveConflictFlow, type ConflictRow } from '#lib/utils/saveConflict.svelte.js';

	const factTypes = FACT_TYPES;

	let summary: ProofSummaryResponse | null = $state(null);
	let linkedAnalyses: EvidenceAnalysisResponse[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleting = $state(false);
	let isNew = $state(false);

	let formData = $state({
		fact_type: 'person_birth' as FactType,
		subject_id: '',
		conclusion: '',
		argument: '',
		research_status: 'unknown' as ResearchStatus,
		analysis_ids: [] as string[]
	});

	let newAnalysisId = $state('');

	// The summary stores its subject's id; the page shows the subject's name.
	const subjects = new SubjectNames();
	function loadSubject(saved: ProofSummaryResponse) {
		subjects.load([{ type: subjectTypeOf(saved.fact_type), id: saved.subject_id }]);
	}

	const saveConflict = new SaveConflictFlow<ProofSummaryResponse, typeof formData>({
		base: () => summary,
		formFrom,
		form: () => formData,
		adopt: (latest, form) => {
			summary = latest;
			formData = form;
		},
		save: () => saveSummary()
	});

	// The edit form's fields, in its order, for comparing with the latest version.
	const conflictRows: ConflictRow<typeof formData>[] = [
		{ key: 'fact_type', label: 'Fact Type', format: (value) => formatFactType(String(value)) },
		{ key: 'subject_id', label: 'Subject ID' },
		{ key: 'conclusion', label: 'Conclusion' },
		{ key: 'argument', label: 'Argument' },
		{ key: 'research_status', label: 'Research Status', format: (value) => optionLabel(RESEARCH_STATUS_OPTIONS, String(value)) },
		{ key: 'analysis_ids', label: 'Linked Analysis IDs' }
	];

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
			loadSubject(result);
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
			error = recordLoadError(e, 'proof summary');
			summary = null;
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	/** The edit form's values for a proof summary as saved. */
	function formFrom(saved: ProofSummaryResponse): typeof formData {
		return {
			fact_type: saved.fact_type,
			subject_id: saved.subject_id,
			conclusion: saved.conclusion,
			argument: saved.argument,
			research_status: saved.research_status || 'unknown',
			analysis_ids: [...(saved.analysis_ids || [])]
		};
	}

	function resetForm() {
		if (summary) {
			formData = formFrom(summary);
		}
	}

	function startEdit() {
		resetForm();
		saveConflict.clear();
		editing = true;
	}

	function cancelEdit() {
		if (isNew) {
			goto('/evidence');
			return;
		}
		resetForm();
		saveConflict.clear();
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
		saveConflict.clear();
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
				loadSubject(updated);
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
			// Only a stale version is compared with the latest (#940); other 409s
			// are refusals, and a proof summary that can't be re-read shows the refusal.
			const id = summary?.id;
			if (!(id && isVersionConflict(e) && (await saveConflict.load(() => api.getProofSummary(id))))) {
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
		const id = page.params.id;
		// Track subjectId so navigating ?subjectId=A → ?subjectId=B re-prefills
		const subjectId = page.url?.searchParams?.get('subjectId');
		if (id) {
			untrack(() => loadSummary(id, subjectId ?? undefined));
		}
	});
</script>

<svelte:head>
	<title>{isNew ? 'New Proof Summary' : summary ? `${formatFactType(summary.fact_type)} Proof Summary` : 'Proof Summary'} | My Family</title>
</svelte:head>

{#snippet recordActions()}
	<Button variant="outline" onclick={startEdit}>Edit</Button>
	<Button variant="destructive" onclick={deleteSummary} disabled={deleting}>
		{deleting ? 'Deleting...' : 'Delete'}
	</Button>
{/snippet}

{#snippet analysisCard(la: EvidenceAnalysisResponse)}
	<a
		href="/evidence/analyses/{la.id}"
		class="block rounded-lg border border-slate-200 p-4 text-inherit no-underline transition-colors hover:border-blue-500"
		data-slot="card-link"
	>
		<div class="mb-2 flex items-center justify-between">
			<span class="text-sm font-semibold text-foreground">
				{formatFactType(la.fact_type)}
			</span>
			{#if la.research_status}
				<UncertaintyBadge status={la.research_status} showLabel size="small" />
			{/if}
		</div>
		<p class="m-0 text-[0.8125rem] leading-snug text-muted-foreground">{la.conclusion}</p>
		{#if la.citation_ids && la.citation_ids.length > 0}
			<span class="mt-2 inline-block text-xs text-muted-foreground">
				{la.citation_ids.length} citations
			</span>
		{/if}
	</a>
{/snippet}

<div class="mx-auto max-w-3xl p-6">
	<ActionBar
		back={{ href: '/evidence', label: 'Evidence' }}
		actions={summary && !editing ? recordActions : undefined}
	/>

	{#if loading}
		<div class="p-12 text-center text-muted-foreground">Loading...</div>
	{:else if error && !summary && !isNew}
		<ErrorState message={error} onRetry={() => loadSummary(page.params.id!)} />
	{:else if editing}
		<form
			class="rounded-xl border border-slate-200 bg-white p-6"
			onsubmit={(e) => {
				e.preventDefault();
				saveSummary();
			}}
		>
			<PageHeader title={isNew ? 'New Proof Summary' : 'Edit Proof Summary'} />

			{#if error}
				<ActionError message={error} />
			{/if}

			<FormRow>
				<div class="flex flex-col gap-1.5">
					<Label for="fact-type" class="text-sm text-muted-foreground">
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
					<Label for="subject-id" class="text-sm text-muted-foreground">
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
			</FormRow>

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="conclusion" class="text-sm text-muted-foreground">
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
				<Label for="argument" class="text-sm text-muted-foreground">
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

			<FormRow>
				<div class="flex flex-col gap-1.5">
					<Label for="research-status" class="text-sm text-muted-foreground">Research Status</Label>
					<select
						id="research-status"
						bind:value={formData.research_status}
						aria-label="Research Status"
						class={nativeSelectClass}
					>
						{#each RESEARCH_STATUS_OPTIONS as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
				</div>
			</FormRow>

			<div class="mb-4">
				<h3 class="m-0 mb-2 text-sm text-muted-foreground">Linked Analysis IDs</h3>
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

			<!-- Beside the Save button, where the user is looking when it fails. -->
			{#if saveConflict.latest}
				<SaveConflict
					noun="proof summary"
					fields={saveConflict.fields(conflictRows)}
					onKeepMine={saveConflict.keepMine}
					onUseLatest={saveConflict.useLatest}
					busy={saving}
				/>
			{/if}

			<div class="mt-6 flex justify-end gap-3 border-t border-slate-200 pt-4">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Proof Summary' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if summary}
		{@const subject = { type: subjectTypeOf(summary.fact_type), id: summary.subject_id } as const}
		<div class="rounded-xl border border-slate-200 bg-white p-6">
			{#snippet summaryBadges()}
				<Badge variant="secondary">{formatFactType(summary!.fact_type)}</Badge>
				{#if summary!.research_status}
					<UncertaintyBadge status={summary!.research_status} showLabel />
				{/if}
			{/snippet}
			<PageHeader title="Proof Summary" badges={summaryBadges} />

			{#if error}
				<ActionError message={error} />
			{/if}

			<InfoSection
				title="Details"
				fields={[
					{ label: 'Subject', value: subjects.label(subject), href: subjectHref(subject) },
					{ label: 'Fact Type', value: formatFactType(summary.fact_type) }
				]}
			/>

			<InfoSection title="Conclusion">
				<p class="m-0 whitespace-pre-wrap text-base font-medium leading-relaxed text-foreground">
					{summary.conclusion}
				</p>
			</InfoSection>

			<div class="mb-6 rounded-lg border border-slate-200 bg-slate-50 p-5">
				<InfoSection title="Argument">
					<div class="m-0 whitespace-pre-wrap text-[0.9375rem] leading-loose text-foreground">
						{summary.argument}
					</div>
				</InfoSection>
			</div>

			{#if linkedAnalyses.length > 0}
				<InfoSection title={`Supporting Analyses (${linkedAnalyses.length})`}>
					<div class="flex flex-col gap-3">
						{#each linkedAnalyses as la}
							{@render analysisCard(la)}
						{/each}
					</div>
				</InfoSection>
			{:else if summary.analysis_ids && summary.analysis_ids.length > 0}
				<InfoSection title={`Linked Analysis IDs (${summary.analysis_ids.length})`}>
					<ul class="m-0 flex list-none flex-col gap-1.5 p-0">
						{#each summary.analysis_ids as aid}
							<li>
								<a href="/evidence/analyses/{aid}" class="text-link no-underline hover:underline">
									<code class="rounded bg-slate-100 px-2 py-1 text-[0.8125rem]">{aid}</code>
								</a>
							</li>
						{/each}
					</ul>
				</InfoSection>
			{/if}

			<div
				class="mt-6 flex flex-wrap gap-6 border-t border-slate-200 pt-4 text-xs text-muted-foreground"
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
