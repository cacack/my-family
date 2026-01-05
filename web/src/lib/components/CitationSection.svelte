<script lang="ts">
	import { api, type Citation, type Source } from '$lib/api/client';

	interface Props {
		personId: string;
	}

	let { personId }: Props = $props();

	let citations: Citation[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	// Add citation form state
	let showAddForm = $state(false);
	let saving = $state(false);
	let deleteConfirm: string | null = $state(null);

	// Source search state
	let sourceSearchQuery = $state('');
	let sourceSearchResults: Source[] = $state([]);
	let sourceSearchLoading = $state(false);
	let showSourceDropdown = $state(false);
	let selectedSource: Source | null = $state(null);
	let sourceDebounceTimer: ReturnType<typeof setTimeout> | null = null;

	// New citation form
	let newCitation = $state({
		source_id: '',
		fact_type: 'general',
		page: '',
		volume: '',
		source_quality: '',
		informant_type: '',
		evidence_type: '',
		quoted_text: '',
		analysis: ''
	});

	async function loadCitations() {
		loading = true;
		error = null;
		try {
			const result = await api.getPersonCitations(personId);
			citations = result.citations;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load citations';
		} finally {
			loading = false;
		}
	}

	async function searchSources(query: string) {
		if (query.length < 2) {
			sourceSearchResults = [];
			return;
		}

		sourceSearchLoading = true;
		try {
			const result = await api.searchSources(query, 10);
			sourceSearchResults = result.sources;
		} catch {
			sourceSearchResults = [];
		} finally {
			sourceSearchLoading = false;
		}
	}

	function handleSourceInput(e: Event) {
		const input = e.target as HTMLInputElement;
		sourceSearchQuery = input.value;
		showSourceDropdown = true;
		selectedSource = null;
		newCitation.source_id = '';

		if (sourceDebounceTimer) {
			clearTimeout(sourceDebounceTimer);
		}
		sourceDebounceTimer = setTimeout(() => {
			searchSources(sourceSearchQuery);
		}, 300);
	}

	function selectSource(source: Source) {
		selectedSource = source;
		sourceSearchQuery = source.title;
		newCitation.source_id = source.id;
		showSourceDropdown = false;
		sourceSearchResults = [];
	}

	function handleSourceFocus() {
		if (sourceSearchQuery.length >= 2 && sourceSearchResults.length > 0) {
			showSourceDropdown = true;
		}
	}

	function handleSourceBlur() {
		setTimeout(() => {
			showSourceDropdown = false;
		}, 200);
	}

	function openAddForm() {
		newCitation = {
			source_id: '',
			fact_type: 'general',
			page: '',
			volume: '',
			source_quality: '',
			informant_type: '',
			evidence_type: '',
			quoted_text: '',
			analysis: ''
		};
		sourceSearchQuery = '';
		selectedSource = null;
		sourceSearchResults = [];
		showAddForm = true;
		error = null;
	}

	function cancelAdd() {
		showAddForm = false;
		error = null;
	}

	async function saveNewCitation() {
		if (!newCitation.source_id) {
			error = 'Please select a source';
			return;
		}

		saving = true;
		error = null;
		try {
			await api.createCitation({
				source_id: newCitation.source_id,
				fact_type: newCitation.fact_type,
				fact_owner_id: personId,
				page: newCitation.page.trim() || undefined,
				volume: newCitation.volume.trim() || undefined,
				source_quality: newCitation.source_quality || undefined,
				informant_type: newCitation.informant_type || undefined,
				evidence_type: newCitation.evidence_type || undefined,
				quoted_text: newCitation.quoted_text.trim() || undefined,
				analysis: newCitation.analysis.trim() || undefined
			});
			showAddForm = false;
			loadCitations();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create citation';
		} finally {
			saving = false;
		}
	}

	async function deleteCitation(citation: Citation) {
		try {
			await api.deleteCitation(citation.id, citation.version);
			deleteConfirm = null;
			loadCitations();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to delete citation';
		}
	}

	function formatFactType(type: string): string {
		return type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	$effect(() => {
		if (personId) {
			loadCitations();
		}
	});
</script>

<div class="citation-section">
	<div class="section-header">
		<h2>Citations <span class="count-badge">{citations.length}</span></h2>
		{#if !showAddForm}
			<button class="btn btn-small" onclick={openAddForm}>Add Citation</button>
		{/if}
	</div>

	{#if error}
		<div class="section-error" role="alert">{error}</div>
	{/if}

	{#if showAddForm}
		<form class="add-form" onsubmit={(e) => { e.preventDefault(); saveNewCitation(); }}>
			<div class="form-group">
				<label for="source-search-input">
					Source <span class="required">*</span>
				</label>
				<div class="source-search">
					<input
						id="source-search-input"
						type="text"
						value={sourceSearchQuery}
						oninput={handleSourceInput}
						onfocus={handleSourceFocus}
						onblur={handleSourceBlur}
						placeholder="Search for a source..."
						class="source-input"
					/>
					{#if sourceSearchLoading}
						<span class="loading-indicator"></span>
					{/if}
					{#if showSourceDropdown && (sourceSearchResults.length > 0 || (sourceSearchQuery.length >= 2 && !sourceSearchLoading))}
						<div class="source-dropdown">
							{#if sourceSearchResults.length === 0}
								<div class="no-results">No sources found</div>
							{:else}
								{#each sourceSearchResults as source}
									<button
										type="button"
										class="source-option"
										onclick={() => selectSource(source)}
									>
										<span class="source-title">{source.title}</span>
										{#if source.author}
											<span class="source-author">by {source.author}</span>
										{/if}
									</button>
								{/each}
							{/if}
						</div>
					{/if}
				</div>
				{#if selectedSource}
					<div class="selected-source">
						Selected: <strong>{selectedSource.title}</strong>
					</div>
				{/if}
			</div>

			<div class="form-row">
				<label>
					Fact Type
					<select bind:value={newCitation.fact_type}>
						<option value="general">General</option>
						<option value="birth">Birth</option>
						<option value="death">Death</option>
						<option value="marriage">Marriage</option>
						<option value="baptism">Baptism</option>
						<option value="burial">Burial</option>
						<option value="residence">Residence</option>
						<option value="occupation">Occupation</option>
						<option value="immigration">Immigration</option>
						<option value="military">Military</option>
						<option value="education">Education</option>
					</select>
				</label>
			</div>

			<div class="form-row">
				<label>
					Page
					<input type="text" bind:value={newCitation.page} placeholder="e.g., 42" />
				</label>
				<label>
					Volume
					<input type="text" bind:value={newCitation.volume} placeholder="e.g., Vol. 3" />
				</label>
			</div>

			<div class="form-row three-cols">
				<label>
					Source Quality
					<select bind:value={newCitation.source_quality}>
						<option value="">Not specified</option>
						<option value="original">Original</option>
						<option value="derivative">Derivative</option>
					</select>
				</label>
				<label>
					Informant Type
					<select bind:value={newCitation.informant_type}>
						<option value="">Not specified</option>
						<option value="primary">Primary</option>
						<option value="secondary">Secondary</option>
					</select>
				</label>
				<label>
					Evidence Type
					<select bind:value={newCitation.evidence_type}>
						<option value="">Not specified</option>
						<option value="direct">Direct</option>
						<option value="indirect">Indirect</option>
					</select>
				</label>
			</div>

			<label>
				Quoted Text
				<textarea bind:value={newCitation.quoted_text} rows="2" placeholder="Exact text from the source..."></textarea>
			</label>

			<label>
				Analysis
				<textarea bind:value={newCitation.analysis} rows="2" placeholder="Your interpretation of the evidence..."></textarea>
			</label>

			<div class="form-actions">
				<button type="button" class="btn" onclick={cancelAdd} disabled={saving}>Cancel</button>
				<button type="submit" class="btn btn-primary" disabled={saving}>
					{saving ? 'Saving...' : 'Add Citation'}
				</button>
			</div>
		</form>
	{/if}

	{#if loading}
		<div class="loading-state" role="status" aria-live="polite">Loading citations...</div>
	{:else if citations.length === 0 && !showAddForm}
		<div class="empty-state">
			<p>No citations yet.</p>
			<button class="btn btn-small" onclick={openAddForm}>Add the first citation</button>
		</div>
	{:else if citations.length > 0}
		<ul class="citation-list">
			{#each citations as citation}
				<li class="citation-item">
					<div class="citation-header">
						<a href="/sources/{citation.source_id}" class="source-link">
							{citation.source_title}
						</a>
						<span class="fact-type">{formatFactType(citation.fact_type)}</span>
					</div>

					{#if citation.page || citation.volume}
						<div class="citation-details">
							{#if citation.page}
								<span>Page: {citation.page}</span>
							{/if}
							{#if citation.volume}
								<span>Volume: {citation.volume}</span>
							{/if}
						</div>
					{/if}

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

					{#if citation.analysis}
						<p class="analysis">{citation.analysis}</p>
					{/if}

					<div class="citation-actions">
						{#if deleteConfirm === citation.id}
							<span class="delete-confirm">Delete this citation?</span>
							<button class="btn btn-small btn-danger" onclick={() => deleteCitation(citation)}>Yes, Delete</button>
							<button class="btn btn-small" onclick={() => deleteConfirm = null}>Cancel</button>
						{:else}
							<button class="btn btn-small btn-text" onclick={() => deleteConfirm = citation.id}>Delete</button>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.citation-section {
		margin-top: 1.5rem;
	}

	.section-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	.section-header h2 {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.count-badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.25rem;
		height: 1.25rem;
		padding: 0 0.375rem;
		background: #dbeafe;
		border-radius: 9999px;
		font-size: 0.6875rem;
		font-weight: 600;
		color: #3b82f6;
		text-transform: none;
		letter-spacing: 0;
	}

	.btn {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		color: #475569;
	}

	.btn:hover {
		background: #f1f5f9;
	}

	.btn-small {
		padding: 0.375rem 0.75rem;
		font-size: 0.8125rem;
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

	.btn-text {
		background: transparent;
		border-color: transparent;
		color: #64748b;
	}

	.btn-text:hover {
		color: #dc2626;
		background: transparent;
	}

	.section-error {
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		color: #dc2626;
		font-size: 0.875rem;
		margin-bottom: 1rem;
	}

	.add-form {
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		margin-bottom: 1rem;
	}

	.form-group {
		margin-bottom: 1rem;
	}

	.form-group > label {
		display: block;
		font-size: 0.875rem;
		color: #475569;
		margin-bottom: 0.375rem;
	}

	.source-search {
		position: relative;
	}

	.source-input {
		width: 100%;
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		background: white;
	}

	.source-input:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.loading-indicator {
		position: absolute;
		right: 0.75rem;
		top: 50%;
		transform: translateY(-50%);
		width: 1rem;
		height: 1rem;
		border: 2px solid #e2e8f0;
		border-top-color: #3b82f6;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: translateY(-50%) rotate(360deg);
		}
	}

	.source-dropdown {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
		z-index: 100;
		max-height: 200px;
		overflow-y: auto;
	}

	.no-results {
		padding: 0.75rem 1rem;
		color: #94a3b8;
		font-size: 0.875rem;
		text-align: center;
	}

	.source-option {
		display: block;
		width: 100%;
		padding: 0.625rem 1rem;
		border: none;
		background: none;
		cursor: pointer;
		text-align: left;
		transition: background 0.15s;
	}

	.source-option:hover {
		background: #f1f5f9;
	}

	.source-title {
		display: block;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.source-author {
		display: block;
		font-size: 0.75rem;
		color: #94a3b8;
		margin-top: 0.125rem;
	}

	.selected-source {
		margin-top: 0.5rem;
		padding: 0.5rem;
		background: #dcfce7;
		border-radius: 4px;
		font-size: 0.8125rem;
		color: #166534;
	}

	.form-row {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.form-row.three-cols {
		grid-template-columns: repeat(3, 1fr);
	}

	.add-form label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.required {
		color: #dc2626;
	}

	.add-form input,
	.add-form select,
	.add-form textarea {
		padding: 0.5rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		background: white;
	}

	.add-form input:focus,
	.add-form select:focus,
	.add-form textarea:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.add-form textarea {
		resize: vertical;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.loading-state,
	.empty-state {
		text-align: center;
		padding: 2rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	.empty-state p {
		margin: 0 0 1rem;
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
		flex-wrap: wrap;
	}

	.source-link {
		color: #3b82f6;
		text-decoration: none;
		font-size: 0.9375rem;
		font-weight: 500;
	}

	.source-link:hover {
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
		margin: 0.5rem 0;
		padding: 0.75rem;
		background: #f8fafc;
		border-left: 3px solid #cbd5e1;
		font-size: 0.875rem;
		color: #475569;
		font-style: italic;
	}

	.analysis {
		margin: 0.5rem 0 0;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.citation-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.75rem;
		padding-top: 0.75rem;
		border-top: 1px solid #f1f5f9;
	}

	.delete-confirm {
		font-size: 0.8125rem;
		color: #dc2626;
	}
</style>
