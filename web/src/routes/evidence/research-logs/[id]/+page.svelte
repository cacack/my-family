<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		api,
		type ResearchLogResponse,
		type ResearchLogCreateRequest
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { toRFC3339 } from '$lib/utils/evidence';

	const outcomes = ['found', 'not_found', 'inconclusive'] as const;

	let log: ResearchLogResponse | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleting = $state(false);
	let isNew = $state(false);

	let formData = $state({
		subject_id: '',
		subject_type: 'person',
		repository: '',
		search_description: '',
		outcome: 'inconclusive' as 'found' | 'not_found' | 'inconclusive',
		notes: '',
		search_date: new Date().toISOString().split('T')[0]
	});

	function formatOutcome(outcome: string): string {
		return outcome.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	async function loadLog(id: string) {
		if (id === 'new') {
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
			log = await api.getResearchLog(id);
			resetForm();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load research log';
			log = null;
		} finally {
			loading = false;
		}
	}

	function resetForm() {
		if (log) {
			formData = {
				subject_id: log.subject_id,
				subject_type: log.subject_type,
				repository: log.repository,
				search_description: log.search_description,
				outcome: log.outcome,
				notes: log.notes || '',
				search_date: log.search_date.split('T')[0]
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

	async function saveLog() {
		const errors: string[] = [];
		if (!formData.subject_id.trim()) errors.push('Subject ID is required');
		if (!formData.repository.trim()) errors.push('Repository is required');
		if (!formData.search_description.trim()) errors.push('Search description is required');
		if (errors.length > 0) {
			error = errors.join('. ');
			return;
		}

		saving = true;
		error = null;
		try {
			if (isNew) {
				const data: ResearchLogCreateRequest = {
					subject_id: formData.subject_id.trim(),
					subject_type: formData.subject_type.trim(),
					repository: formData.repository.trim(),
					search_description: formData.search_description.trim(),
					outcome: formData.outcome,
					notes: formData.notes.trim() || undefined,
					search_date: toRFC3339(formData.search_date)
				};
				const created = await api.createResearchLog(data);
				goto(`/evidence/research-logs/${created.id}`);
			} else if (log) {
				await api.updateResearchLog(log.id, {
					subject_id: formData.subject_id.trim(),
					subject_type: formData.subject_type.trim(),
					repository: formData.repository.trim(),
					search_description: formData.search_description.trim(),
					outcome: formData.outcome,
					notes: formData.notes.trim() || undefined,
					search_date: toRFC3339(formData.search_date),
					version: log.version
				});
				await loadLog(log.id);
				editing = false;
			}
		} catch (e) {
			const msg = (e as { message?: string }).message || 'Failed to save';
			if (msg.includes('conflict') || msg.includes('version')) {
				error = 'Version conflict: someone else modified this record. Please reload and try again.';
			} else {
				error = msg;
			}
		} finally {
			saving = false;
		}
	}

	async function deleteLog() {
		if (!log) return;
		if (!confirm('Delete this research log? This cannot be undone.')) return;

		deleting = true;
		try {
			await api.deleteResearchLog(log.id, log.version);
			goto('/evidence');
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to delete';
			deleting = false;
		}
	}

	$effect(() => {
		const id = $page.params.id;
		if (id) {
			untrack(() => loadLog(id));
		}
	});
</script>

<svelte:head>
	<title>{isNew ? 'New Research Log' : 'Research Log'} | My Family</title>
</svelte:head>

<div class="detail-page">
	<header class="page-header">
		<a href="/evidence" class="back-link">&larr; Evidence</a>
		{#if log && !editing}
			<div class="actions">
				<Button variant="outline" onclick={startEdit}>Edit</Button>
				<Button variant="destructive" onclick={deleteLog} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete'}
				</Button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error && !log && !isNew}
		<div class="error">
			<p>{error}</p>
			<Button variant="outline" onclick={() => loadLog($page.params.id!)}>Retry</Button>
		</div>
	{:else if editing}
		<form class="edit-form" onsubmit={(e) => { e.preventDefault(); saveLog(); }}>
			<h1>{isNew ? 'New Research Log' : 'Edit Research Log'}</h1>

			{#if error}
				<div class="form-error">{error}</div>
			{/if}

			<div class="form-row">
				<label>
					Subject ID <span class="required">*</span>
					<input type="text" bind:value={formData.subject_id} required placeholder="Person or family UUID" />
				</label>
				<label>
					Subject Type
					<select bind:value={formData.subject_type}>
						<option value="person">Person</option>
						<option value="family">Family</option>
					</select>
				</label>
			</div>

			<div class="form-row">
				<label>
					Repository <span class="required">*</span>
					<input type="text" bind:value={formData.repository} required placeholder="e.g., National Archives" />
				</label>
				<label>
					Search Date
					<input type="date" bind:value={formData.search_date} />
				</label>
			</div>

			<label>
				Search Description <span class="required">*</span>
				<textarea bind:value={formData.search_description} rows="3" required></textarea>
			</label>

			<div class="form-row">
				<label>
					Outcome
					<select bind:value={formData.outcome}>
						{#each outcomes as o}
							<option value={o}>{formatOutcome(o)}</option>
						{/each}
					</select>
				</label>
			</div>

			<label>
				Notes
				<textarea bind:value={formData.notes} rows="3"></textarea>
			</label>

			<div class="form-actions">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Research Log' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if log}
		<div class="detail-card">
			<div class="detail-header">
				<h1>Research Log</h1>
				<div class="header-badges">
					{#if log.outcome === 'found'}
						<Badge class="bg-green-50 text-green-700 border-green-200">Found</Badge>
					{:else if log.outcome === 'not_found'}
						<Badge variant="destructive">Not Found</Badge>
					{:else}
						<Badge class="bg-yellow-50 text-yellow-700 border-yellow-200">Inconclusive</Badge>
					{/if}
				</div>
			</div>

			<div class="info-grid">
				<div class="info-section">
					<h2>Details</h2>
					<dl>
						<dt>Subject</dt>
						<dd><a href="/{log.subject_type === 'family' ? 'families' : 'persons'}/{log.subject_id}">{log.subject_id}</a></dd>
						<dt>Subject Type</dt>
						<dd>{log.subject_type.charAt(0).toUpperCase() + log.subject_type.slice(1)}</dd>
						<dt>Repository</dt>
						<dd>{log.repository}</dd>
						<dt>Search Date</dt>
						<dd>{new Date(log.search_date).toLocaleDateString()}</dd>
						<dt>Outcome</dt>
						<dd>{formatOutcome(log.outcome)}</dd>
					</dl>
				</div>
			</div>

			<div class="info-section">
				<h2>Search Description</h2>
				<p class="text-content">{log.search_description}</p>
			</div>

			{#if log.notes}
				<div class="info-section">
					<h2>Notes</h2>
					<p class="text-content">{log.notes}</p>
				</div>
			{/if}

			<div class="meta-footer">
				{#if log.created_at}
					<span>Created: {new Date(log.created_at).toLocaleDateString()}</span>
				{/if}
				{#if log.updated_at}
					<span>Updated: {new Date(log.updated_at).toLocaleDateString()}</span>
				{/if}
				<span>Version: {log.version}</span>
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
