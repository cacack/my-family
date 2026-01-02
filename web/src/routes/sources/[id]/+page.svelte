<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, type SourceDetail, type Citation } from '$lib/api/client';

	let source: SourceDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleting = $state(false);

	// Form state
	let formData = $state({
		source_type: '',
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

	async function loadSource(id: string) {
		loading = true;
		error = null;
		try {
			source = await api.getSource(id);
			resetForm();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load source';
			source = null;
		} finally {
			loading = false;
		}
	}

	function resetForm() {
		if (source) {
			formData = {
				source_type: source.source_type,
				title: source.title,
				author: source.author || '',
				publisher: source.publisher || '',
				publish_date: source.publish_date || '',
				url: source.url || '',
				repository_name: source.repository_name || '',
				collection_name: source.collection_name || '',
				call_number: source.call_number || '',
				notes: source.notes || ''
			};
		}
	}

	function startEdit() {
		resetForm();
		editing = true;
	}

	function cancelEdit() {
		resetForm();
		editing = false;
	}

	async function saveSource() {
		if (!source) return;
		if (!formData.title.trim()) {
			error = 'Title is required';
			return;
		}

		saving = true;
		error = null;
		try {
			await api.updateSource(source.id, {
				source_type: formData.source_type || undefined,
				title: formData.title.trim() || undefined,
				author: formData.author.trim() || undefined,
				publisher: formData.publisher.trim() || undefined,
				publish_date: formData.publish_date.trim() || undefined,
				url: formData.url.trim() || undefined,
				repository_name: formData.repository_name.trim() || undefined,
				collection_name: formData.collection_name.trim() || undefined,
				call_number: formData.call_number.trim() || undefined,
				notes: formData.notes.trim() || undefined,
				version: source.version
			});
			await loadSource(source.id);
			editing = false;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function deleteSource() {
		if (!source) return;
		if (!confirm(`Delete "${source.title}"? This cannot be undone.`)) return;

		deleting = true;
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

<div class="source-page">
	<header class="page-header">
		<a href="/sources" class="back-link">&larr; Sources</a>
		{#if source && !editing}
			<div class="actions">
				<button class="btn" onclick={startEdit}>Edit</button>
				<button class="btn btn-danger" onclick={deleteSource} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete'}
				</button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error && !source}
		<div class="error">{error}</div>
	{:else if source}
		{#if editing}
			<form class="edit-form" onsubmit={(e) => { e.preventDefault(); saveSource(); }}>
				{#if error}
					<div class="form-error">{error}</div>
				{/if}

				<div class="form-row">
					<label>
						Source Type
						<select bind:value={formData.source_type}>
							<option value="document">Document</option>
							<option value="book">Book</option>
							<option value="newspaper">Newspaper</option>
							<option value="census">Census</option>
							<option value="vital_record">Vital Record</option>
							<option value="church_record">Church Record</option>
							<option value="military_record">Military Record</option>
							<option value="immigration_record">Immigration Record</option>
							<option value="land_record">Land Record</option>
							<option value="court_record">Court Record</option>
							<option value="photograph">Photograph</option>
							<option value="oral_history">Oral History</option>
							<option value="website">Website</option>
							<option value="other">Other</option>
						</select>
					</label>
					<label>
						Title <span class="required">*</span>
						<input type="text" bind:value={formData.title} required />
					</label>
				</div>

				<div class="form-row">
					<label>
						Author
						<input type="text" bind:value={formData.author} />
					</label>
					<label>
						Publisher
						<input type="text" bind:value={formData.publisher} />
					</label>
				</div>

				<div class="form-row">
					<label>
						Publish Date
						<input type="text" bind:value={formData.publish_date} placeholder="e.g., 1920 or 15 Mar 1920" />
					</label>
					<label>
						URL
						<input type="url" bind:value={formData.url} />
					</label>
				</div>

				<div class="form-row">
					<label>
						Repository Name
						<input type="text" bind:value={formData.repository_name} placeholder="e.g., National Archives" />
					</label>
					<label>
						Collection Name
						<input type="text" bind:value={formData.collection_name} />
					</label>
				</div>

				<div class="form-row">
					<label>
						Call Number
						<input type="text" bind:value={formData.call_number} />
					</label>
				</div>

				<label>
					Notes
					<textarea bind:value={formData.notes} rows="4"></textarea>
				</label>

				<div class="form-actions">
					<button type="button" class="btn" onclick={cancelEdit} disabled={saving}>Cancel</button>
					<button type="submit" class="btn btn-primary" disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
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
						<span class="type-badge">{formatSourceType(source.source_type)}</span>
						{#if source.citation_count > 0}
							<span class="citation-badge">{source.citation_count} {source.citation_count === 1 ? 'citation' : 'citations'}</span>
						{/if}
					</div>
				</div>

				<div class="info-grid">
					{#if source.author || source.publisher || source.publish_date}
						<div class="info-section">
							<h2>Publication Info</h2>
							<dl>
								{#if source.author}
									<dt>Author</dt>
									<dd>{source.author}</dd>
								{/if}
								{#if source.publisher}
									<dt>Publisher</dt>
									<dd>{source.publisher}</dd>
								{/if}
								{#if source.publish_date}
									<dt>Date</dt>
									<dd>{source.publish_date}</dd>
								{/if}
							</dl>
						</div>
					{/if}

					{#if source.repository_name || source.collection_name || source.call_number}
						<div class="info-section">
							<h2>Repository Info</h2>
							<dl>
								{#if source.repository_name}
									<dt>Repository</dt>
									<dd>{source.repository_name}</dd>
								{/if}
								{#if source.collection_name}
									<dt>Collection</dt>
									<dd>{source.collection_name}</dd>
								{/if}
								{#if source.call_number}
									<dt>Call Number</dt>
									<dd>{source.call_number}</dd>
								{/if}
							</dl>
						</div>
					{/if}
				</div>

				{#if source.url}
					<div class="info-section">
						<h2>URL</h2>
						<a href={source.url} target="_blank" rel="noopener noreferrer" class="source-url">{source.url}</a>
					</div>
				{/if}

				{#if source.notes}
					<div class="info-section">
						<h2>Notes</h2>
						<p class="notes">{source.notes}</p>
					</div>
				{/if}

				{#if getCitations().length > 0}
					<div class="info-section">
						<h2>Citations ({getCitations().length})</h2>
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
					</div>
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

	.btn {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		text-decoration: none;
		color: #475569;
	}

	.btn:hover {
		background: #f1f5f9;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	.btn-danger {
		color: #dc2626;
		border-color: #fecaca;
	}

	.btn-danger:hover {
		background: #fef2f2;
	}

	.loading,
	.error {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.error {
		color: #dc2626;
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

	.source-title h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.type-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		background: #f1f5f9;
		border-radius: 4px;
		font-size: 0.75rem;
		color: #64748b;
		margin-top: 0.5rem;
		margin-right: 0.5rem;
	}

	.citation-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		background: #dbeafe;
		border-radius: 4px;
		font-size: 0.75rem;
		color: #3b82f6;
		margin-top: 0.5rem;
	}

	.info-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
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

	.source-url {
		color: #3b82f6;
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
		color: #3b82f6;
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
		color: #64748b;
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
		color: #64748b;
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
