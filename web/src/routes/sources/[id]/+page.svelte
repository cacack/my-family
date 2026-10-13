<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, isVersionConflict, recordLoadError, type SourceDetail, type SourceType, type Citation } from '$lib/api/client';
	import ExternalLinks from '$lib/components/ExternalLinks.svelte';
	import InfoSection from '$lib/components/InfoSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import ActionBar from '$lib/components/ActionBar.svelte';
	import FormRow from '$lib/components/FormRow.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { SOURCE_TYPES } from '$lib/utils/sourceTypes';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import ActionError from '$lib/components/ActionError.svelte';
	import SaveConflict from '$lib/components/SaveConflict.svelte';
	import { SaveConflictFlow, type ConflictRow } from '$lib/utils/saveConflict.svelte';

	let source: SourceDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleting = $state(false);

	// Form state
	let formData = $state({
		source_type: '' as SourceType | '',
		title: '',
		author: '',
		publisher: '',
		publish_date: '',
		url: '',
		repository_name: '',
		collection_name: '',
		call_number: '',
		notes: ''
	});

	const saveConflict = new SaveConflictFlow<SourceDetail, typeof formData>({
		base: () => source,
		formFrom,
		form: () => formData,
		adopt: (latest, form) => {
			source = latest;
			formData = form;
		},
		save: () => saveSource()
	});

	// The edit form's fields, in its order, for comparing with the latest version.
	const conflictRows: ConflictRow<typeof formData>[] = [
		{ key: 'source_type', label: 'Source Type', format: (value) => sourceTypeLabel(String(value)) },
		{ key: 'title', label: 'Title' },
		{ key: 'author', label: 'Author' },
		{ key: 'publisher', label: 'Publisher' },
		{ key: 'publish_date', label: 'Publish Date' },
		{ key: 'url', label: 'URL' },
		{ key: 'repository_name', label: 'Repository Name' },
		{ key: 'collection_name', label: 'Collection Name' },
		{ key: 'call_number', label: 'Call Number' },
		{ key: 'notes', label: 'Notes' }
	];

	function sourceTypeLabel(value: string): string {
		return SOURCE_TYPES.find((type) => type.value === value)?.label ?? value;
	}

	async function loadSource(id: string) {
		loading = true;
		error = null;
		try {
			source = await api.getSource(id);
			resetForm();
		} catch (e) {
			error = recordLoadError(e, 'source');
			source = null;
		} finally {
			loading = false;
		}
	}

	/** The edit form's values for a source as saved. */
	function formFrom(saved: SourceDetail): typeof formData {
		return {
			source_type: saved.source_type,
			title: saved.title,
			author: saved.author || '',
			publisher: saved.publisher || '',
			publish_date: saved.publish_date || '',
			url: saved.url || '',
			repository_name: saved.repository_name || '',
			collection_name: saved.collection_name || '',
			call_number: saved.call_number || '',
			notes: saved.notes || ''
		};
	}

	function resetForm() {
		if (source) {
			formData = formFrom(source);
		}
	}

	function startEdit() {
		resetForm();
		editing = true;
		error = null;
		saveConflict.clear();
	}

	function cancelEdit() {
		resetForm();
		editing = false;
		error = null;
		saveConflict.clear();
	}

	async function saveSource() {
		if (!source) return;
		if (!formData.title.trim()) {
			error = 'Title is required';
			return;
		}

		saving = true;
		error = null;
		saveConflict.clear();
		try {
			await api.updateSource(source.id, {
				source_type: formData.source_type || undefined,
				// Optional fields are always sent: an empty string clears the field,
				// whereas an omitted one is left unchanged.
				title: formData.title.trim(),
				author: formData.author.trim(),
				publisher: formData.publisher.trim(),
				publish_date: formData.publish_date.trim(),
				url: formData.url.trim(),
				repository_name: formData.repository_name.trim(),
				collection_name: formData.collection_name.trim(),
				call_number: formData.call_number.trim(),
				notes: formData.notes.trim(),
				version: source.version
			});
			await loadSource(source.id);
			editing = false;
		} catch (e) {
			// Only a stale version is compared with the latest (#940); other 409s
			// are refusals, and a source that can't be re-read shows the refusal.
			const id = source.id;
			if (!(isVersionConflict(e) && (await saveConflict.load(() => api.getSource(id))))) {
				error = (e as { message?: string }).message || 'Failed to save';
			}
		} finally {
			saving = false;
		}
	}

	async function deleteSource() {
		if (!source) return;
		if (!confirm(`Delete "${source.title}"? This cannot be undone.`)) return;

		deleting = true;
		error = null;
		try {
			await api.deleteSource(source.id, source.version);
			goto('/sources');
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to delete';
			deleting = false;
		}
	}

	function formatSourceType(type: string): string {
		return type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function formatFactType(type: string): string {
		return type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	$effect(() => {
		const id = $page.params.id;
		if (id) {
			loadSource(id);
		}
	});

	// Computed property for citations list
	function getCitations(): Citation[] {
		return source?.citations ?? [];
	}
</script>

<svelte:head>
	<title>{source ? source.title : 'Source'} | My Family</title>
</svelte:head>

{#snippet sourceActions()}
	<Button variant="outline" onclick={startEdit}>Edit</Button>
	<Button variant="destructive" onclick={deleteSource} disabled={deleting}>
		{deleting ? 'Deleting...' : 'Delete'}
	</Button>
{/snippet}

<div class="source-page">
	<ActionBar back={{ href: '/sources', label: 'Sources' }} actions={source && !editing ? sourceActions : undefined} />

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error && !source}
		<ErrorState message={error} onRetry={() => loadSource($page.params.id ?? '')} />
	{:else if source}
		<!-- In the form, the error shows in it; out of it, a failed delete shows here. -->
		{#if error && !editing}
			<ActionError message={error} />
		{/if}
		{#if editing}
			<form class="edit-form" onsubmit={(e) => { e.preventDefault(); saveSource(); }}>
				{#if error}
					<div class="form-error">{error}</div>
				{/if}

				<FormRow>
					<label>
						Source Type
						<select bind:value={formData.source_type}>
							{#each SOURCE_TYPES as type (type.value)}
								<option value={type.value}>{type.label}</option>
							{/each}
						</select>
					</label>
					<label>
						<span>Title <span class="required">*</span></span>
						<input type="text" bind:value={formData.title} required />
					</label>
				</FormRow>

				<FormRow>
					<label>
						Author
						<input type="text" bind:value={formData.author} />
					</label>
					<label>
						Publisher
						<input type="text" bind:value={formData.publisher} />
					</label>
				</FormRow>

				<FormRow>
					<label>
						Publish Date
						<input type="text" bind:value={formData.publish_date} placeholder="e.g., 1920 or 15 Mar 1920" />
					</label>
					<label>
						URL
						<input type="url" bind:value={formData.url} />
					</label>
				</FormRow>

				<FormRow>
					<label>
						Repository Name
						<input type="text" bind:value={formData.repository_name} placeholder="e.g., National Archives" />
					</label>
					<label>
						Collection Name
						<input type="text" bind:value={formData.collection_name} />
					</label>
				</FormRow>

				<FormRow>
					<label>
						Call Number
						<input type="text" bind:value={formData.call_number} />
					</label>
				</FormRow>

				<label>
					Notes
					<textarea bind:value={formData.notes} rows="4"></textarea>
				</label>

				<!-- Beside the Save button, where the user is looking when it fails. -->
				{#if saveConflict.latest}
					<SaveConflict
						noun="source"
						fields={saveConflict.fields(conflictRows)}
						onKeepMine={saveConflict.keepMine}
						onUseLatest={saveConflict.useLatest}
						busy={saving}
					/>
				{/if}

				<div class="form-actions">
					<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
					<Button type="submit" disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</Button>
				</div>
			</form>
		{:else}
			<div class="source-detail">
				<div class="source-header">
					<div class="source-icon">
						<svg viewBox="0 0 24 24" fill="currentColor">
							<path d="M19 2H6c-1.206 0-3 .799-3 3v14c0 2.201 1.794 3 3 3h15v-2H6.012C5.55 19.988 5 19.806 5 19s.55-.988 1.012-1H21V4c0-1.103-.897-2-2-2zm0 14H5V5c0-.806.55-.988 1-1h13v12z" />
						</svg>
					</div>
					<div class="source-title">
						<h1>{source.title}</h1>
						<Badge variant="secondary">{formatSourceType(source.source_type)}</Badge>
						{#if source.citation_count}
							<span class="citation-badge">{source.citation_count} {source.citation_count === 1 ? 'citation' : 'citations'}</span>
						{/if}
					</div>
				</div>

				<div class="info-grid">
					{#if source.author || source.publisher || source.publish_date}
						<InfoSection
							title="Publication Info"
							hideEmpty
							fields={[
								{ label: 'Author', value: source.author },
								{ label: 'Publisher', value: source.publisher },
								{ label: 'Date', value: source.publish_date }
							]}
						/>
					{/if}

					{#if source.repository_name || source.collection_name || source.call_number}
						<InfoSection
							title="Repository Info"
							hideEmpty
							fields={[
								{ label: 'Repository', value: source.repository_name },
								{ label: 'Collection', value: source.collection_name },
								{ label: 'Call Number', value: source.call_number }
							]}
						/>
					{/if}
				</div>

				{#if source.url}
					<InfoSection title="URL">
						<a href={source.url} target="_blank" rel="noopener noreferrer" class="source-url">{source.url}</a>
					</InfoSection>
				{/if}

				{#if source.notes}
					<InfoSection title="Notes">
						<p class="notes">{source.notes}</p>
					</InfoSection>
				{/if}

				<!-- Guard here (in addition to ExternalLinks' own empty check) so the
				     "External links" heading is suppressed when there are none. -->
				{#if source.external_ids && source.external_ids.length > 0}
					<InfoSection title="External links">
						<ExternalLinks externalIds={source.external_ids} />
					</InfoSection>
				{/if}

				{#if getCitations().length > 0}
					<InfoSection title={`Citations (${getCitations().length})`}>
						<ul class="citation-list">
							{#each getCitations() as citation}
								<li class="citation-item">
									<div class="citation-header">
										<a href="/persons/{citation.fact_owner_id}" class="person-link">
											View Person
										</a>
										<span class="fact-type">{formatFactType(citation.fact_type)}</span>
									</div>
									<div class="citation-details">
										{#if citation.page}
											<span>Page: {citation.page}</span>
										{/if}
										{#if citation.volume}
											<span>Volume: {citation.volume}</span>
										{/if}
									</div>
									{#if citation.source_quality || citation.informant_type || citation.evidence_type}
										<div class="quality-badges">
											{#if citation.source_quality}
												<span class="quality-badge" class:original={citation.source_quality === 'original'} class:derivative={citation.source_quality === 'derivative'}>
													{citation.source_quality}
												</span>
											{/if}
											{#if citation.informant_type}
												<span class="quality-badge" class:primary={citation.informant_type === 'primary'} class:secondary={citation.informant_type === 'secondary'}>
													{citation.informant_type}
												</span>
											{/if}
											{#if citation.evidence_type}
												<span class="quality-badge" class:direct={citation.evidence_type === 'direct'} class:indirect={citation.evidence_type === 'indirect'}>
													{citation.evidence_type}
												</span>
											{/if}
										</div>
									{/if}
									{#if citation.quoted_text}
										<blockquote class="quoted-text">"{citation.quoted_text}"</blockquote>
									{/if}
								</li>
							{/each}
						</ul>
					</InfoSection>
				{/if}
			</div>
		{/if}
	{/if}
</div>

<style>
	.source-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.loading {
		text-align: center;
		padding: 3rem;
		color: var(--color-text-muted);
	}

	.source-detail {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.source-header {
		display: flex;
		align-items: flex-start;
		gap: 1rem;
		margin-bottom: 1.5rem;
		padding-bottom: 1.5rem;
		border-bottom: 1px solid #e2e8f0;
	}

	.source-icon {
		flex-shrink: 0;
		width: 4rem;
		height: 4rem;
		border-radius: 12px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #f1f5f9;
		color: #64748b;
	}

	.source-icon svg {
		width: 2rem;
		height: 2rem;
	}

	.source-title {
		min-width: 0;
	}

	.source-title h1 {
		margin: 0;
		overflow-wrap: anywhere;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.citation-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		background: #dbeafe;
		border-radius: 4px;
		font-size: 0.75rem;
		color: var(--color-link);
		margin-top: 0.5rem;
	}

	.info-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(16rem, 100%), 1fr));
		gap: 1.5rem;
		margin-bottom: 1.5rem;
	}

	/* The grid gap spaces the sections, not their own margins. */
	.info-grid > :global(.info-section) {
		margin-bottom: 0;
	}

	.source-url {
		color: var(--color-link);
		text-decoration: none;
		font-size: 0.875rem;
		word-break: break-all;
	}

	.source-url:hover {
		text-decoration: underline;
	}

	.notes {
		margin: 0;
		color: #475569;
		font-size: 0.875rem;
		white-space: pre-wrap;
	}

	.citation-list {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.citation-item {
		padding: 1rem;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		margin-bottom: 0.75rem;
	}

	.citation-item:last-child {
		margin-bottom: 0;
	}

	.citation-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.person-link {
		color: var(--color-link);
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 500;
	}

	.person-link:hover {
		text-decoration: underline;
	}

	.fact-type {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		background: #f1f5f9;
		border-radius: 4px;
		font-size: 0.75rem;
		color: #475569;
	}

	.citation-details {
		font-size: 0.8125rem;
		color: var(--color-text-muted);
		margin-bottom: 0.5rem;
	}

	.citation-details span {
		margin-right: 1rem;
	}

	.quality-badges {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 0.5rem;
		flex-wrap: wrap;
	}

	.quality-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-size: 0.6875rem;
		text-transform: uppercase;
		font-weight: 500;
		background: #f1f5f9;
		color: var(--color-text-muted);
	}

	.quality-badge.original,
	.quality-badge.primary,
	.quality-badge.direct {
		background: #dcfce7;
		color: #166534;
	}

	.quality-badge.derivative,
	.quality-badge.secondary,
	.quality-badge.indirect {
		background: #fef9c3;
		color: #854d0e;
	}

	.quoted-text {
		margin: 0.5rem 0 0;
		padding: 0.75rem;
		background: #f8fafc;
		border-left: 3px solid #cbd5e1;
		font-size: 0.875rem;
		color: #475569;
		font-style: italic;
	}

	/* Edit form styles */
	.edit-form {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
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

	.edit-form label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
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
</style>
