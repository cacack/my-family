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
	import UncertaintyBadge from '$lib/components/UncertaintyBadge.svelte';
	import { formatFactType, subjectRoute } from '$lib/utils/evidence';

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

<div class="detail-page">
	<header class="page-header">
		<a href="/evidence" class="back-link">&larr; Evidence</a>
		{#if analysis && !editing}
			<div class="actions">
				<Button variant="outline" onclick={startEdit}>Edit</Button>
				<Button variant="destructive" onclick={deleteAnalysis} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete'}
				</Button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error && !analysis && !isNew}
		<div class="error">
			<p>{error}</p>
			<Button variant="outline" onclick={() => loadAnalysis($page.params.id!)}>Retry</Button>
		</div>
	{:else if editing}
		<form class="edit-form" onsubmit={(e) => { e.preventDefault(); saveAnalysis(); }}>
			<h1>{isNew ? 'New Evidence Analysis' : 'Edit Analysis'}</h1>

			{#if error}
				<div class="form-error" role="alert">{error}</div>
			{/if}

			<div class="form-row">
				<label>
					Fact Type <span class="required">*</span>
					<select bind:value={formData.fact_type} aria-label="Fact Type">
						{#each factTypes as ft}
							<option value={ft}>{formatFactType(ft)}</option>
						{/each}
					</select>
				</label>
				<label>
					Subject ID <span class="required">*</span>
					<input type="text" bind:value={formData.subject_id} required placeholder="Person or family UUID" aria-label="Subject ID" />
				</label>
			</div>

			<label>
				Conclusion <span class="required">*</span>
				<textarea bind:value={formData.conclusion} rows="3" required aria-label="Conclusion"></textarea>
			</label>

			<div class="form-row">
				<label>
					Research Status
					<select bind:value={formData.research_status} aria-label="Research Status">
						{#each researchStatuses as s}
							<option value={s}>{s.charAt(0).toUpperCase() + s.slice(1)}</option>
						{/each}
					</select>
				</label>
			</div>

			<label>
				Notes
				<textarea bind:value={formData.notes} rows="3" aria-label="Notes"></textarea>
			</label>

			<div class="list-field">
				<h3>Citation IDs</h3>
				{#if formData.citation_ids.length > 0}
					<ul class="id-list">
						{#each formData.citation_ids as cid}
							<li>
								<code>{cid}</code>
								<button type="button" class="remove-btn" onclick={() => removeCitation(cid)} aria-label="Remove citation {cid}">x</button>
							</li>
						{/each}
					</ul>
				{/if}
				<div class="add-id-row">
					<input type="text" bind:value={newCitationId} placeholder="Citation UUID" aria-label="New citation ID" />
					<Button type="button" variant="outline" onclick={addCitation}>Add</Button>
				</div>
			</div>

			<div class="form-actions">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Analysis' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if analysis}
		<div class="detail-card">
			<div class="detail-header">
				<h1>Evidence Analysis</h1>
				<div class="header-badges">
					<Badge variant="secondary">{formatFactType(analysis.fact_type)}</Badge>
					{#if analysis.research_status}
						<UncertaintyBadge status={analysis.research_status} showLabel />
					{/if}
				</div>
			</div>

			{#if error}
				<div class="form-error" role="alert">{error}</div>
			{/if}

			<div class="info-grid">
				<div class="info-section">
					<h2>Details</h2>
					<dl>
						<dt>Subject</dt>
						<dd><a href="/{subjectRoute(analysis.fact_type)}/{analysis.subject_id}">{analysis.subject_id}</a></dd>
						<dt>Fact Type</dt>
						<dd>{formatFactType(analysis.fact_type)}</dd>
						{#if analysis.conflict_id}
							<dt>Conflict</dt>
							<dd><a href="/evidence/conflicts/{analysis.conflict_id}">{analysis.conflict_id.slice(0, 8)}...</a></dd>
						{/if}
					</dl>
				</div>
			</div>

			<div class="info-section">
				<h2>Conclusion</h2>
				<p class="text-content">{analysis.conclusion}</p>
			</div>

			{#if analysis.notes}
				<div class="info-section">
					<h2>Notes</h2>
					<p class="text-content">{analysis.notes}</p>
				</div>
			{/if}

			{#if analysis.citation_ids && analysis.citation_ids.length > 0}
				<div class="info-section">
					<h2>Citations ({analysis.citation_ids.length})</h2>
					<ul class="linked-ids">
						{#each analysis.citation_ids as cid}
							<li><code>{cid}</code></li>
						{/each}
					</ul>
				</div>
			{/if}

			<div class="meta-footer">
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

<style>
	.detail-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
	}

	.back-link {
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.loading {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.error {
		text-align: center;
		padding: 3rem;
		color: #dc2626;
	}

	.error p {
		margin: 0 0 1rem;
	}

	.detail-card {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.detail-header {
		margin-bottom: 1.5rem;
		padding-bottom: 1rem;
		border-bottom: 1px solid #e2e8f0;
	}

	.detail-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.header-badges {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.info-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
		gap: 1.5rem;
		margin-bottom: 1.5rem;
	}

	.info-section {
		margin-bottom: 1.5rem;
	}

	.info-section h2 {
		margin: 0 0 0.75rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.info-section dl {
		margin: 0;
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.25rem 1rem;
	}

	.info-section dt {
		color: #94a3b8;
		font-size: 0.8125rem;
	}

	.info-section dd {
		margin: 0;
		color: #1e293b;
		font-size: 0.875rem;
	}

	.info-section dd a {
		color: #3b82f6;
		text-decoration: none;
	}

	.info-section dd a:hover {
		text-decoration: underline;
	}

	.text-content {
		margin: 0;
		color: #475569;
		font-size: 0.875rem;
		white-space: pre-wrap;
		line-height: 1.6;
	}

	.linked-ids {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.linked-ids code {
		font-size: 0.8125rem;
		background: #f1f5f9;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
	}

	.meta-footer {
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
		display: flex;
		gap: 1.5rem;
		flex-wrap: wrap;
		font-size: 0.75rem;
		color: #94a3b8;
	}

	/* Edit form styles */
	.edit-form {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.edit-form h1 {
		margin: 0 0 1.5rem;
		font-size: 1.25rem;
		color: #1e293b;
	}

	.form-error {
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		color: #dc2626;
		font-size: 0.875rem;
		margin-bottom: 1rem;
	}

	.form-row {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.edit-form label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
		margin-bottom: 1rem;
	}

	.required {
		color: #dc2626;
	}

	.edit-form input,
	.edit-form select,
	.edit-form textarea {
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.edit-form input:focus,
	.edit-form select:focus,
	.edit-form textarea:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.edit-form textarea {
		resize: vertical;
	}

	.list-field {
		margin-bottom: 1rem;
	}

	.list-field h3 {
		margin: 0 0 0.5rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.id-list {
		list-style: none;
		padding: 0;
		margin: 0 0 0.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.id-list li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.id-list code {
		font-size: 0.8125rem;
		background: #f1f5f9;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
	}

	.remove-btn {
		background: none;
		border: none;
		color: #dc2626;
		cursor: pointer;
		font-size: 0.875rem;
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
	}

	.remove-btn:hover {
		background: #fef2f2;
	}

	.add-id-row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.add-id-row input {
		flex: 1;
		padding: 0.5rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	@media (max-width: 640px) {
		.form-row {
			grid-template-columns: 1fr;
		}
	}
</style>
