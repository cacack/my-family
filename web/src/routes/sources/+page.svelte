<script lang="ts">
	import { page } from '$app/stores';
	import { api, type Source } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import FormRow from '$lib/components/FormRow.svelte';
	import SourceCard from '$lib/components/SourceCard.svelte';
	import { DEFAULT_SOURCE_TYPE, SOURCE_TYPES } from '$lib/utils/sourceTypes';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import ActionError from '$lib/components/ActionError.svelte';
	import { asDatePhrase, unreadableDateField } from '$lib/utils/dateInput';
	import { clampPageToTotal, readEnum, readPositiveInt, setQuery } from '$lib/utils/urlState';

	let sources: Source[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let loadError: string | null = $state(null);
	let showAddForm = $state(false);
	let saving = $state(false);
	let error: string | null = $state(null);
	let unreadableDate = $state(false);
	const pageSize = 20;

	// Search, sort and page live in the URL (see urlState.ts), so Back, reload
	// and a copied link restore the list.
	const SORTS = ['title', 'author', 'citation_count'] as const;
	const DEFAULTS = { sort: 'title', order: 'asc', q: '', page: 1 } as const;

	const params = $derived($page.url.searchParams);
	const sort = $derived(readEnum(params, 'sort', SORTS, DEFAULTS.sort));
	const order = $derived(readEnum(params, 'order', ['asc', 'desc'] as const, DEFAULTS.order));
	const searchQuery = $derived(params.get('q') ?? DEFAULTS.q);
	const currentPage = $derived(readPositiveInt(params, 'page', DEFAULTS.page));

	// The search box is a draft while typing; the URL takes it after a pause.
	// A URL change from elsewhere (Back, a link) refills the box, but the
	// search this page just wrote does not, so it never eats keystrokes.
	let searchInput = $state('');
	let lastWrittenQuery: string | null = null;
	$effect.pre(() => {
		const q = searchQuery;
		if (q !== lastWrittenQuery) searchInput = q;
		lastWrittenQuery = null;
	});

	// New source form state
	let newSource = $state({
		source_type: DEFAULT_SOURCE_TYPE,
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

	// Back/Forward can start loads faster than they finish: only the latest one may land.
	let loadSeq = 0;

	async function loadSources() {
		const seq = ++loadSeq;
		loading = true;
		loadError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.listSources({
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize,
				sort,
				order,
				q: searchQuery || undefined
			});
			if (seq !== loadSeq) return;
			if (clampPageToTotal($page.url, requestedPage, result.total, pageSize)) return;
			sources = result.sources;
			total = result.total;
		} catch (e) {
			if (seq !== loadSeq) return;
			console.error('Failed to load sources:', e);
			loadError = 'Failed to load sources. Please try again.';
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	// Search and sort refine the view (replace); a new search or sort starts at page 1.
	function refine(updates: Record<string, string>) {
		setQuery($page.url, { ...updates, page: 1 }, { defaults: DEFAULTS });
	}

	function goToPage(n: number) {
		setQuery($page.url, { page: n }, { push: true, defaults: DEFAULTS });
	}

	function handleSortChange(e: Event) {
		refine({ sort: (e.target as HTMLSelectElement).value });
	}

	function handleOrderChange() {
		setQuery($page.url, { order: order === 'asc' ? 'desc' : 'asc' }, { defaults: DEFAULTS });
	}

	function search(q: string) {
		if (debounceTimer) clearTimeout(debounceTimer);
		lastWrittenQuery = q;
		refine({ q });
	}

	function handleSearchInput(e: Event) {
		searchInput = (e.target as HTMLInputElement).value;
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => search(searchInput), 300);
	}

	function clearSearch() {
		searchInput = '';
		search('');
	}

	function prevPage() {
		if (currentPage > 1) goToPage(currentPage - 1);
	}

	function nextPage() {
		if (currentPage * pageSize < total) goToPage(currentPage + 1);
	}

	function openAddForm() {
		newSource = {
			source_type: DEFAULT_SOURCE_TYPE,
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
		unreadableDate = false;
	}

	function cancelAdd() {
		showAddForm = false;
		error = null;
		unreadableDate = false;
	}

	async function saveNewSource() {
		if (!newSource.title.trim()) {
			error = 'Title is required';
			return;
		}

		saving = true;
		error = null;
		unreadableDate = false;
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
			unreadableDate = unreadableDateField(e, ['publish_date']) !== null;
		} finally {
			saving = false;
		}
	}

	/** Create with an unreadable publish date as a date phrase, so it is kept as text (#957). */
	function keepDateAsText() {
		newSource.publish_date = asDatePhrase(newSource.publish_date);
		saveNewSource();
	}

	$effect(() => {
		loadSources();
	});

	// A search still waiting on its pause must not write into the next page's URL.
	$effect(() => () => {
		if (debounceTimer) clearTimeout(debounceTimer);
	});

	const totalPages = $derived(Math.ceil(total / pageSize));
</script>

<svelte:head>
	<title>Sources | My Family</title>
</svelte:head>

<div class="sources-page">
	<PageHeader title="Sources">
		{#snippet actions()}
			<div class="controls">
				<div class="search-wrapper">
					<svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="11" cy="11" r="8" />
						<path d="m21 21-4.35-4.35" />
					</svg>
					<input
						type="text"
						value={searchInput}
						oninput={handleSearchInput}
						placeholder="Search sources..."
						aria-label="Search sources"
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
				<button
					class="order-btn"
					onclick={handleOrderChange}
					title="Toggle sort order"
					aria-label="Sort order: {order === 'asc' ? 'ascending' : 'descending'}"
				>
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
				<Button onclick={openAddForm}>Add Source</Button>
			</div>
		{/snippet}
	</PageHeader>

	{#if showAddForm}
		<div class="add-form-container">
			<form class="add-form" onsubmit={(e) => { e.preventDefault(); saveNewSource(); }}>
				<h2>Add New Source</h2>

				{#if error}
					<ActionError
						message={error}
						action={unreadableDate ? { label: 'Keep as text', onclick: keepDateAsText, disabled: saving } : undefined}
					/>
				{/if}

				<FormRow>
					<label>
						Source Type
						<select bind:value={newSource.source_type}>
							{#each SOURCE_TYPES as type (type.value)}
								<option value={type.value}>{type.label}</option>
							{/each}
						</select>
					</label>
					<label>
						<span>Title <span class="required">*</span></span>
						<input type="text" bind:value={newSource.title} required />
					</label>
				</FormRow>

				<FormRow>
					<label>
						Author
						<input type="text" bind:value={newSource.author} />
					</label>
					<label>
						Publisher
						<input type="text" bind:value={newSource.publisher} />
					</label>
				</FormRow>

				<FormRow>
					<label>
						Publish Date
						<input type="text" bind:value={newSource.publish_date} placeholder="e.g., 1920 or 15 Mar 1920" />
					</label>
					<label>
						URL
						<input type="url" bind:value={newSource.url} />
					</label>
				</FormRow>

				<FormRow>
					<label>
						Repository Name
						<input type="text" bind:value={newSource.repository_name} placeholder="e.g., National Archives" />
					</label>
					<label>
						Collection Name
						<input type="text" bind:value={newSource.collection_name} />
					</label>
				</FormRow>

				<FormRow>
					<label>
						Call Number
						<input type="text" bind:value={newSource.call_number} />
					</label>
				</FormRow>

				<label>
					Notes
					<textarea bind:value={newSource.notes} rows="3"></textarea>
				</label>

				<div class="form-actions">
					<Button variant="outline" onclick={cancelAdd} disabled={saving}>Cancel</Button>
					<Button type="submit" disabled={saving}>
						{saving ? 'Saving...' : 'Create Source'}
					</Button>
				</div>
			</form>
		</div>
	{/if}

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if loadError}
		<ErrorState message={loadError} onRetry={loadSources} />
	{:else if sources.length === 0}
		<div class="empty">
			{#if searchQuery}
				<p>No sources found matching "{searchQuery}".</p>
				<Button variant="outline" onclick={clearSearch}>Clear Search</Button>
			{:else}
				<p>No sources found.</p>
				<Button onclick={openAddForm}>Add Source</Button>
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
		color: var(--color-text-muted);
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
		color: var(--color-text-muted);
	}

	.empty p {
		margin: 0 0 1rem;
	}

	.sources-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr));
		gap: 1rem;
	}

	.pagination {
		display: flex;
		flex-wrap: wrap;
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
		color: var(--color-text-muted);
	}
</style>
