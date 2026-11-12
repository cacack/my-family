<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		api,
		isVersionConflict,
		recordLoadError,
		type EvidenceAnalysisResponse,
		type EvidenceAnalysisCreateRequest,
		type FactType,
		type ResearchStatus
	} from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import UncertaintyBadge from '#lib/components/UncertaintyBadge.svelte';
	import { FACT_TYPES, formatFactType, isFactType, subjectTypeOf } from '#lib/utils/evidence.js';
	import { SubjectNames, subjectHref } from '#lib/utils/subjectNames.svelte.js';
	import { nativeSelectClass } from '#lib/utils/forms.js';
	import { RESEARCH_STATUS_OPTIONS, optionLabel } from '#lib/utils/enumOptions.js';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import ActionBar from '#lib/components/ActionBar.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import InfoSection from '#lib/components/InfoSection.svelte';
	import FormRow from '#lib/components/FormRow.svelte';
	import SaveConflict from '#lib/components/SaveConflict.svelte';
	import { SaveConflictFlow, type ConflictRow } from '#lib/utils/saveConflict.svelte.js';

	// Every fact type the API accepts, so a fact the merge review links here
	// (#838) is always offered.
	const factTypes = FACT_TYPES;

	let analysis: EvidenceAnalysisResponse | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleteOpen = $state(false);
	let isNew = $state(false);

	/**
	 * The initial fact type: a prefilled one (the merge review's "add
	 * analysis", #838) when the form offers it; otherwise a default that fits
	 * the subject - a family's marriage, a person's birth.
	 */
	function initialFactType(factType: string, subjectType: string): FactType {
		if (isFactType(factType)) return factType;
		return subjectType === 'family' ? 'family_marriage' : 'person_birth';
	}

	function emptyFormData(subjectId = '', factType = '', subjectType = '') {
		return {
			fact_type: initialFactType(factType, subjectType),
			subject_id: subjectId,
			conclusion: '',
			research_status: 'unknown' as ResearchStatus,
			notes: '',
			citation_ids: [] as string[]
		};
	}

	let formData = $state(emptyFormData());

	// The analysis stores its subject's id; the page shows the subject's name.
	const subjects = new SubjectNames();
	function loadSubject(saved: EvidenceAnalysisResponse) {
		subjects.load([{ type: subjectTypeOf(saved.fact_type), id: saved.subject_id }]);
	}

	let newCitationId = $state('');

	const saveConflict = new SaveConflictFlow<EvidenceAnalysisResponse, typeof formData>({
		base: () => analysis,
		formFrom,
		form: () => formData,
		adopt: (latest, form) => {
			analysis = latest;
			formData = form;
		},
		save: () => saveAnalysis()
	});

	// The edit form's fields, in its order, for comparing with the latest version.
	const conflictRows: ConflictRow<typeof formData>[] = [
		{ key: 'fact_type', label: 'Fact Type', format: (value) => formatFactType(String(value)) },
		{ key: 'subject_id', label: 'Subject ID' },
		{ key: 'conclusion', label: 'Conclusion' },
		{ key: 'research_status', label: 'Research Status', format: (value) => optionLabel(RESEARCH_STATUS_OPTIONS, String(value)) },
		{ key: 'notes', label: 'Notes' },
		{ key: 'citation_ids', label: 'Citation IDs' }
	];

	// Monotonic request id to guard against stale async completions on fast route changes.
	let loadSeq = 0;

	// A linked fact type the form does not offer, said aloud rather than
	// silently swapped for the default.
	let unknownFactType: string | null = $state(null);

	async function loadAnalysis(
		id: string,
		urlSubjectId?: string,
		urlFactType?: string,
		urlSubjectType?: string
	) {
		const seq = ++loadSeq;
		unknownFactType = null;
		if (id === 'new') {
			analysis = null;
			error = null;
			newCitationId = '';
			formData = emptyFormData(urlSubjectId ?? '', urlFactType ?? '', urlSubjectType ?? '');
			if (urlFactType && !isFactType(urlFactType)) unknownFactType = urlFactType;
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
			loadSubject(result);
			resetForm();
		} catch (e) {
			if (seq !== loadSeq) return;
			error = recordLoadError(e, 'analysis');
			analysis = null;
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	/** The edit form's values for an analysis as saved. */
	function formFrom(saved: EvidenceAnalysisResponse): typeof formData {
		return {
			fact_type: saved.fact_type,
			subject_id: saved.subject_id,
			conclusion: saved.conclusion,
			research_status: saved.research_status || 'unknown',
			notes: saved.notes || '',
			citation_ids: [...(saved.citation_ids || [])]
		};
	}

	function resetForm() {
		if (analysis) {
			formData = formFrom(analysis);
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
		saveConflict.clear();
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
				loadSubject(updated);
				resetForm();
				editing = false;
			}
		} catch (e) {
			// Only a stale version is compared with the latest (#940); other 409s
			// are refusals, and an analysis that can't be re-read shows the refusal.
			const id = analysis?.id;
			if (!(id && isVersionConflict(e) && (await saveConflict.load(() => api.getEvidenceAnalysis(id))))) {
				error = (e as { message?: string }).message || 'Failed to save';
			}
		} finally {
			saving = false;
		}
	}

	/** Run from the delete dialog, which shows a failure. */
	async function deleteAnalysis() {
		if (!analysis) return;
		await api.deleteEvidenceAnalysis(analysis.id, analysis.version);
		await goto('/evidence');
	}

	$effect(() => {
		const id = page.params.id;
		// Track subjectId (and factType) so navigating ?subjectId=A → ?subjectId=B re-prefills
		const subjectId = page.url?.searchParams?.get('subjectId');
		const factType = page.url?.searchParams?.get('factType');
		const subjectType = page.url?.searchParams?.get('subjectType');
		if (id) {
			untrack(() =>
				loadAnalysis(id, subjectId ?? undefined, factType ?? undefined, subjectType ?? undefined)
			);
		}
	});
</script>

<svelte:head>
	<title>{isNew ? 'New Analysis' : analysis ? `${formatFactType(analysis.fact_type)} Analysis` : 'Evidence Analysis'} | My Family</title>
</svelte:head>

{#snippet recordActions()}
	<Button variant="outline" onclick={startEdit}>Edit</Button>
	<Button variant="destructive" onclick={() => (deleteOpen = true)}>Delete</Button>
{/snippet}

<ConfirmDialog
	bind:open={deleteOpen}
	title="Delete this analysis?"
	description="This cannot be undone."
	onconfirm={deleteAnalysis}
/>

<div class="mx-auto max-w-3xl p-6">
	<ActionBar
		back={{ href: '/evidence', label: 'Evidence' }}
		actions={analysis && !editing ? recordActions : undefined}
	/>

	{#if loading}
		<div class="p-12 text-center text-muted-foreground">Loading...</div>
	{:else if error && !analysis && !isNew}
		<ErrorState message={error} onRetry={() => loadAnalysis(page.params.id!)} />
	{:else if editing}
		<form
			class="rounded-xl border border-slate-200 bg-white p-6"
			onsubmit={(e) => {
				e.preventDefault();
				saveAnalysis();
			}}
		>
			<PageHeader title={isNew ? 'New Evidence Analysis' : 'Edit Analysis'} />

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
					{#if unknownFactType}
						<p class="m-0 text-xs text-amber-700" role="note">
							The linked fact type "{unknownFactType}" is not one an analysis can have; choose the
							fact this analysis is about.
						</p>
					{/if}
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
				<Textarea
					id="conclusion"
					bind:value={formData.conclusion}
					rows={3}
					required
					aria-label="Conclusion"
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

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="notes" class="text-sm text-muted-foreground">Notes</Label>
				<Textarea id="notes" bind:value={formData.notes} rows={3} aria-label="Notes" />
			</div>

			<div class="mb-4">
				<h3 class="m-0 mb-2 text-sm text-muted-foreground">Citation IDs</h3>
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

			<!-- Beside the Save button, where the user is looking when it fails. -->
			{#if saveConflict.latest}
				<SaveConflict
					noun="analysis"
					fields={saveConflict.fields(conflictRows)}
					onKeepMine={saveConflict.keepMine}
					onUseLatest={saveConflict.useLatest}
					busy={saving}
				/>
			{/if}

			<div class="mt-6 flex justify-end gap-3 border-t border-slate-200 pt-4">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Analysis' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if analysis}
		{@const subject = { type: subjectTypeOf(analysis.fact_type), id: analysis.subject_id } as const}
		<div class="rounded-xl border border-slate-200 bg-white p-6">
			{#snippet analysisBadges()}
				<Badge variant="secondary">{formatFactType(analysis!.fact_type)}</Badge>
				{#if analysis!.research_status}
					<UncertaintyBadge status={analysis!.research_status} showLabel />
				{/if}
			{/snippet}
			<PageHeader title="Evidence Analysis" badges={analysisBadges} />

			{#if error}
				<ActionError message={error} />
			{/if}

			<InfoSection
				title="Details"
				fields={[
					{ label: 'Subject', value: subjects.label(subject), href: subjectHref(subject) },
					{ label: 'Fact Type', value: formatFactType(analysis.fact_type) },
					...(analysis.conflict_id
						? [
								{
									label: 'Conflict',
									value: 'View conflict',
									href: `/evidence/conflicts/${analysis.conflict_id}`
								}
							]
						: [])
				]}
			/>

			<InfoSection title="Conclusion">
				<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
					{analysis.conclusion}
				</p>
			</InfoSection>

			{#if analysis.notes}
				<InfoSection title="Notes">
					<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
						{analysis.notes}
					</p>
				</InfoSection>
			{/if}

			{#if analysis.citation_ids && analysis.citation_ids.length > 0}
				<InfoSection title={`Citations (${analysis.citation_ids.length})`}>
					<ul class="m-0 flex list-none flex-col gap-1.5 p-0">
						{#each analysis.citation_ids as cid}
							<li>
								<code class="rounded bg-slate-100 px-2 py-1 text-[0.8125rem]">{cid}</code>
							</li>
						{/each}
					</ul>
				</InfoSection>
			{/if}

			<div
				class="mt-6 flex flex-wrap gap-6 border-t border-slate-200 pt-4 text-xs text-muted-foreground"
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
