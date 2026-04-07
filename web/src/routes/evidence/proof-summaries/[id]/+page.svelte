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

<div class="detail-page">
	<header class="page-header">
		<a href="/evidence" class="back-link">&larr; Evidence</a>
		{#if summary && !editing}
			<div class="actions">
				<Button variant="outline" onclick={startEdit}>Edit</Button>
				<Button variant="destructive" onclick={deleteSummary} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete'}
				</Button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error && !summary && !isNew}
		<div class="error">
			<p>{error}</p>
			<Button variant="outline" onclick={() => loadSummary($page.params.id!)}>Retry</Button>
		</div>
	{:else if editing}
		<form class="edit-form" onsubmit={(e) => { e.preventDefault(); saveSummary(); }}>
			<h1>{isNew ? 'New Proof Summary' : 'Edit Proof Summary'}</h1>

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
				<input type="text" bind:value={formData.conclusion} required aria-label="Conclusion" />
			</label>

			<label>
				Argument <span class="required">*</span>
				<textarea bind:value={formData.argument} rows="10" required placeholder="Present the full proof argument, evaluating each piece of evidence..." aria-label="Argument"></textarea>
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

			<div class="list-field">
				<h3>Linked Analysis IDs</h3>
				{#if formData.analysis_ids.length > 0}
					<ul class="id-list">
						{#each formData.analysis_ids as aid}
							<li>
								<code>{aid}</code>
								<button type="button" class="remove-btn" onclick={() => removeAnalysis(aid)} aria-label="Remove analysis {aid}">x</button>
							</li>
						{/each}
					</ul>
				{/if}
				<div class="add-id-row">
					<input type="text" bind:value={newAnalysisId} placeholder="Analysis UUID" aria-label="New analysis ID" />
					<Button type="button" variant="outline" onclick={addAnalysis}>Add</Button>
				</div>
			</div>

			<div class="form-actions">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Proof Summary' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if summary}
		<div class="detail-card">
			<div class="detail-header">
				<h1>Proof Summary</h1>
				<div class="header-badges">
					<Badge variant="secondary">{formatFactType(summary.fact_type)}</Badge>
					{#if summary.research_status}
						<UncertaintyBadge status={summary.research_status} showLabel />
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
						<dd><a href="/{subjectRoute(summary.fact_type)}/{summary.subject_id}">{summary.subject_id}</a></dd>
						<dt>Fact Type</dt>
						<dd>{formatFactType(summary.fact_type)}</dd>
					</dl>
				</div>
			</div>

			<div class="info-section">
				<h2>Conclusion</h2>
				<p class="text-content conclusion-text">{summary.conclusion}</p>
			</div>

			<div class="info-section argument-section">
				<h2>Argument</h2>
				<div class="argument-text">{summary.argument}</div>
			</div>

			{#if linkedAnalyses.length > 0}
				<div class="info-section">
					<h2>Supporting Analyses ({linkedAnalyses.length})</h2>
					<div class="analysis-cards">
						{#each linkedAnalyses as la}
							<a href="/evidence/analyses/{la.id}" class="analysis-card">
								<div class="analysis-card-header">
									<span class="analysis-fact-type">{formatFactType(la.fact_type)}</span>
									{#if la.research_status}
										<UncertaintyBadge status={la.research_status} showLabel size="small" />
									{/if}
								</div>
								<p class="analysis-conclusion">{la.conclusion}</p>
								{#if la.citation_ids && la.citation_ids.length > 0}
									<span class="analysis-meta">{la.citation_ids.length} citations</span>
								{/if}
							</a>
						{/each}
					</div>
				</div>
			{:else if summary.analysis_ids && summary.analysis_ids.length > 0}
				<div class="info-section">
					<h2>Linked Analysis IDs ({summary.analysis_ids.length})</h2>
					<ul class="linked-ids">
						{#each summary.analysis_ids as aid}
							<li><a href="/evidence/analyses/{aid}"><code>{aid}</code></a></li>
						{/each}
					</ul>
				</div>
			{/if}

			<div class="meta-footer">
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

	.conclusion-text {
		font-weight: 500;
		font-size: 1rem;
		color: #1e293b;
	}

	.argument-section {
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1.25rem;
	}

	.argument-text {
		margin: 0;
		color: #334155;
		font-size: 0.9375rem;
		white-space: pre-wrap;
		line-height: 1.8;
	}

	.linked-ids {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.linked-ids a {
		color: #3b82f6;
		text-decoration: none;
	}

	.linked-ids a:hover {
		text-decoration: underline;
	}

	.linked-ids code {
		font-size: 0.8125rem;
		background: #f1f5f9;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
	}

	.analysis-cards {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.analysis-card {
		display: block;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		text-decoration: none;
		color: inherit;
		transition: border-color 0.15s;
	}

	.analysis-card:hover {
		border-color: #3b82f6;
	}

	.analysis-card-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	.analysis-fact-type {
		font-weight: 600;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.analysis-conclusion {
		margin: 0;
		font-size: 0.8125rem;
		color: #475569;
		line-height: 1.4;
	}

	.analysis-meta {
		display: inline-block;
		margin-top: 0.5rem;
		font-size: 0.75rem;
		color: #94a3b8;
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
