<script lang="ts">
	import { page } from '$app/state';
	import { api, type Person } from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import PersonCard from '#lib/components/PersonCard.svelte';
	import { RESEARCH_STATUS_OPTIONS } from '#lib/utils/enumOptions.js';
	import { clampPageToTotal, readEnum, readPositiveInt, setQuery } from '#lib/utils/urlState.js';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import NavigationNotice from '#lib/components/NavigationNotice.svelte';

	let persons: Person[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let loadError: string | null = $state(null);
	const pageSize = 20;

	// Sort, filter and page live in the URL (see urlState.ts), so Back, reload
	// and a copied link restore the list.
	const SORTS = ['surname', 'given_name', 'birth_date', 'updated_at'] as const;
	// One "Not assessed" bucket: the API's unknown status and no status at all (#1110).
	const STATUS_FILTER_OPTIONS = RESEARCH_STATUS_OPTIONS.map((o) =>
		o.value === 'unknown' ? { value: 'unassessed' as const, label: o.label } : o
	);
	const STATUSES = ['', ...STATUS_FILTER_OPTIONS.map((o) => o.value)] as const;
	const DEFAULTS = { sort: 'surname', order: 'asc', status: '', page: 1 } as const;

	const params = $derived(page.url.searchParams);
	const sort = $derived(readEnum(params, 'sort', SORTS, DEFAULTS.sort));
	const order = $derived(readEnum(params, 'order', ['asc', 'desc'] as const, DEFAULTS.order));
	// Links made before the one bucket named its two statuses: read them as it.
	const LEGACY_STATUSES = ['unset', 'unknown'] as const;
	const researchStatusFilter = $derived.by(() => {
		const status = readEnum(params, 'status', [...STATUSES, ...LEGACY_STATUSES], DEFAULTS.status);
		return (LEGACY_STATUSES as readonly string[]).includes(status) ? 'unassessed' : status;
	});
	const currentPage = $derived(readPositiveInt(params, 'page', DEFAULTS.page));

	// Back/Forward can start loads faster than they finish: only the latest one may land.
	let loadSeq = 0;

	async function loadPersons() {
		const seq = ++loadSeq;
		loading = true;
		loadError = null;
		try {
			const requestedPage = currentPage;
			const result = await api.listPersons({
				limit: pageSize,
				offset: (requestedPage - 1) * pageSize,
				sort,
				order,
				research_status: researchStatusFilter || undefined
			});
			if (seq !== loadSeq) return;
			if (clampPageToTotal(page.url, requestedPage, result.total, pageSize)) return;
			persons = result.items;
			total = result.total;
		} catch (e) {
			if (seq !== loadSeq) return;
			console.error('Failed to load persons:', e);
			loadError = 'Failed to load people. Please try again.';
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	// Sort and filter refine the view (replace); a filter change starts at page 1.
	function refine(updates: Record<string, string>) {
		setQuery(page.url, { ...updates, page: 1 }, { defaults: DEFAULTS });
	}

	function goToPage(n: number) {
		setQuery(page.url, { page: n }, { push: true, defaults: DEFAULTS });
	}

	// Choosing a sort resets the order to its natural direction (ascending for
	// every sort here), as /search does.
	function handleSortChange(e: Event) {
		refine({ sort: (e.target as HTMLSelectElement).value, order: DEFAULTS.order });
	}

	function handleOrderChange() {
		setQuery(page.url, { order: order === 'asc' ? 'desc' : 'asc' }, { defaults: DEFAULTS });
	}

	function handleStatusFilterChange(e: Event) {
		refine({ status: (e.target as HTMLSelectElement).value });
	}

	function prevPage() {
		if (currentPage > 1) goToPage(currentPage - 1);
	}

	function nextPage() {
		if (currentPage * pageSize < total) goToPage(currentPage + 1);
	}

	$effect(() => {
		loadPersons();
	});

	const totalPages = $derived(Math.ceil(total / pageSize));
</script>

<svelte:head>
	<title>People | My Family</title>
</svelte:head>

<div class="persons-page">
	<PageHeader title="People">
		{#snippet actions()}
			<Button variant="outline" href="/persons/add">Add Person</Button>
			<Button variant="secondary" href="/persons/quick">Quick Capture</Button>
		{/snippet}
	</PageHeader>

	<NavigationNotice />

	<div class="toolbar">
		<button
			type="button"
			class="chip"
			aria-pressed={researchStatusFilter === 'possible'}
			onclick={() => refine({ status: researchStatusFilter === 'possible' ? '' : 'possible' })}
		>
			Quick Captures
		</button>
		<div class="controls">
			<label>
				Confidence:
				<select value={researchStatusFilter} onchange={handleStatusFilterChange}>
					<option value="">All</option>
					{#each STATUS_FILTER_OPTIONS as option (option.value)}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
			</label>
			<label>
				Sort by:
				<select value={sort} onchange={handleSortChange}>
					<option value="surname">Surname</option>
					<option value="given_name">Given Name</option>
					<option value="birth_date">Birth Date</option>
					<option value="updated_at">Last Updated</option>
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
		</div>
	</div>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if loadError}
		<ErrorState message={loadError} onRetry={loadPersons} />
	{:else if persons.length === 0}
		<div class="empty">
			<p>No people found.</p>
			<div class="empty-actions">
				<Button href="/persons/add">Add Person</Button>
				<Button variant="outline" href="/import">Import GEDCOM</Button>
			</div>
		</div>
	{:else}
		<div class="persons-grid">
			{#each persons as person}
				<PersonCard {person} href="/persons/{person.id}" />
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
	.persons-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.controls {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
	}

	.controls label {
		display: flex;
		flex-wrap: wrap;
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

	.loading,
	.empty {
		text-align: center;
		padding: 3rem;
		color: var(--color-text-muted);
	}

	.empty p {
		margin: 0 0 1rem;
	}

	.empty-actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: 0.5rem;
	}

	.persons-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(280px, 100%), 1fr));
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

	.toolbar {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.5rem;
		flex-wrap: wrap;
	}

	.chip {
		padding: 0.375rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 9999px;
		background: white;
		font-size: 0.8125rem;
		cursor: pointer;
		color: var(--color-text-muted);
		transition: all 0.15s;
	}

	.chip:hover {
		border-color: #cbd5e1;
		background: #f8fafc;
	}

	.chip[aria-pressed='true'] {
		background: #fef3c7;
		border-color: #fbbf24;
		color: #92400e;
	}
</style>
