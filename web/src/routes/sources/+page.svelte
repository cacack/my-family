<script lang="ts">
	import { api, type Source } from '$lib/api/client';
	import SourceCard from '$lib/components/SourceCard.svelte';

	let sources: Source[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let currentPage = $state(1);
	let sort = $state<'title' | 'author' | 'citation_count'>('title');
	let order = $state<'asc' | 'desc'>('asc');
	let searchQuery = $state('');
	let showAddForm = $state(false);
	let saving = $state(false);
	let error: string | null = $state(null);
	const pageSize = 20;

	// New source form state
	let newSource = $state({
		source_type: 'document',
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

	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	async function loadSources() {
		loading = true;
		try {
			const result = await api.listSources({
				limit: pageSize,
				offset: (currentPage - 1) * pageSize,
				sort,
				order,
				q: searchQuery || undefined
			});
			sources = result.sources;
			total = result.total;
		} catch (e) {
			console.error('Failed to load sources:', e);
		} finally {
			loading = false;
		}
	}

	function handleSortChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		sort = select.value as typeof sort;
		currentPage = 1;
		loadSources();
	}

	function handleOrderChange() {
		order = order === 'asc' ? 'desc' : 'asc';
		loadSources();
	}

	function handleSearchInput(e: Event) {
		const input = e.target as HTMLInputElement;
		searchQuery = input.value;

		if (debounceTimer) {
			clearTimeout(debounceTimer);
		}
		debounceTimer = setTimeout(() => {
			currentPage = 1;
			loadSources();
		}, 300);
	}

	function prevPage() {
		if (currentPage > 1) {
			currentPage--;
			loadSources();
		}
	}

	function nextPage() {
		if (currentPage * pageSize < total) {
			currentPage++;
			loadSources();
		}
	}

	function openAddForm() {
		newSource = {
			source_type: 'document',
			title: '',
			author: '',
			publisher: '',
			publish_date: '',
			url: '',
			repository_name: '',
			collection_name: '',
			call_number: '',
			notes: ''
		};
		showAddForm = true;
		error = null;
	}

	function cancelAdd() {
		showAddForm = false;
		error = null;
	}

	async function saveNewSource() {
		if (!newSource.title.trim()) {
			error = 'Title is required';
			return;
		}

		saving = true;
		error = null;
		try {
			await api.createSource({
				source_type: newSource.source_type,
				title: newSource.title.trim(),
				author: newSource.author.trim() || undefined,
				publisher: newSource.publisher.trim() || undefined,
				publish_date: newSource.publish_date.trim() || undefined,
				url: newSource.url.trim() || undefined,
				repository_name: newSource.repository_name.trim() || undefined,
				collection_name: newSource.collection_name.trim() || undefined,
				call_number: newSource.call_number.trim() || undefined,
				notes: newSource.notes.trim() || undefined
			});
			showAddForm = false;
			loadSources();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create source';
		} finally {
			saving = false;
		}
	}

	$effect(() => {
		loadSources();
	});

	const totalPages = $derived(Math.ceil(total / pageSize));
</script>

<svelte:head>
	<title>Sources | My Family</title>
</svelte:head>

<div class="sources-page">
	<header class="page-header">
		<h1>Sources</h1>
		<div class="controls">
			<div class="search-wrapper">
				<svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="11" cy="11" r="8" />
					<path d="m21 21-4.35-4.35" />
				</svg>
				<input
					type="text"
					value={searchQuery}
					oninput={handleSearchInput}
					placeholder="Search sources..."
					class="search-input"
				/>
			</div>
			<label>
				Sort by:
				<select value={sort} onchange={handleSortChange}>
					<option value="title">Title</option>
					<option value="author">Author</option>
					<option value="citation_count">Citations</option>
				</select>
			</label>
			<button class="order-btn" onclick={handleOrderChange} title="Toggle sort order">
				{#if order === 'asc'}
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M12 5v14M5 12l7-7 7 7" />
					</svg>
				{:else}
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M12 19V5M5 12l7 7 7-7" />
					</svg>
				{/if}
			</button>
			<button class="btn btn-primary" onclick={openAddForm}>Add Source</button>
		</div>
	</header>

	{#if showAddForm}
		<div class="add-form-container">
			<form class="add-form" onsubmit={(e) => { e.preventDefault(); saveNewSource(); }}>
				<h2>Add New Source</h2>

				{#if error}
					<div class="form-error">{error}</div>
				{/if}

				<div class="form-row">
					<label>
						Source Type
						<select bind:value={newSource.source_type}>
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
						<input type="text" bind:value={newSource.title} required />
					</label>
				</div>

				<div class="form-row">
					<label>
						Author
						<input type="text" bind:value={newSource.author} />
					</label>
					<label>
						Publisher
						<input type="text" bind:value={newSource.publisher} />
					</label>
				</div>

				<div class="form-row">
					<label>
						Publish Date
						<input type="text" bind:value={newSource.publish_date} placeholder="e.g., 1920 or 15 Mar 1920" />
					</label>
					<label>
						URL
						<input type="url" bind:value={newSource.url} />
					</label>
				</div>

				<div class="form-row">
					<label>
						Repository Name
						<input type="text" bind:value={newSource.repository_name} placeholder="e.g., National Archives" />
					</label>
					<label>
						Collection Name
						<input type="text" bind:value={newSource.collection_name} />
					</label>
				</div>

				<div class="form-row">
					<label>
						Call Number
						<input type="text" bind:value={newSource.call_number} />
					</label>
				</div>

				<label>
					Notes
					<textarea bind:value={newSource.notes} rows="3"></textarea>
				</label>

				<div class="form-actions">
					<button type="button" class="btn" onclick={cancelAdd} disabled={saving}>Cancel</button>
					<button type="submit" class="btn btn-primary" disabled={saving}>
						{saving ? 'Saving...' : 'Create Source'}
					</button>
				</div>
			</form>
		</div>
	{/if}

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if sources.length === 0}
		<div class="empty">
			{#if searchQuery}
				<p>No sources found matching "{searchQuery}".</p>
				<button class="btn" onclick={() => { searchQuery = ''; loadSources(); }}>Clear Search</button>
			{:else}
				<p>No sources found.</p>
				<button class="btn btn-primary" onclick={openAddForm}>Add Source</button>
			{/if}
		</div>
	{:else}
		<div class="sources-grid">
			{#each sources as source}
				<SourceCard {source} href="/sources/{source.id}" />
			{/each}
		</div>

		{#if totalPages > 1}
			<div class="pagination">
				<button onclick={prevPage} disabled={currentPage === 1}>Previous</button>
				<span>Page {currentPage} of {totalPages}</span>
				<button onclick={nextPage} disabled={currentPage >= totalPages}>Next</button>
			</div>
		{/if}
	{/if}
</div>

<style>
	.sources-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
		flex-wrap: wrap;
		gap: 1rem;
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.controls {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.search-wrapper {
		position: relative;
		display: flex;
		align-items: center;
	}

	.search-icon {
		position: absolute;
		left: 0.75rem;
		width: 1rem;
		height: 1rem;
		color: #94a3b8;
		pointer-events: none;
	}

	.search-input {
		width: 200px;
		padding: 0.5rem 0.75rem 0.5rem 2.25rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.search-input:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.controls label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.controls select {
		padding: 0.375rem 0.75rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	.order-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 2.25rem;
		height: 2.25rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		cursor: pointer;
	}

	.order-btn:hover {
		background: #f1f5f9;
	}

	.order-btn svg {
		width: 1rem;
		height: 1rem;
		color: #64748b;
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

	.add-form-container {
		margin-bottom: 1.5rem;
	}

	.add-form {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.add-form h2 {
		margin: 0 0 1rem;
		font-size: 1.125rem;
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
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
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
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.loading,
	.empty {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.empty p {
		margin: 0 0 1rem;
	}

	.sources-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
		gap: 1rem;
	}

	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 2rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.pagination button {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.pagination button:hover:not(:disabled) {
		background: #f1f5f9;
	}

	.pagination button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.pagination span {
		font-size: 0.875rem;
		color: #64748b;
	}
</style>
