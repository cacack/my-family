<script lang="ts">
	import { page } from '$app/stores';
	import { api, type Person, type ResearchStatus } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import PersonCard from '$lib/components/PersonCard.svelte';
	import { RESEARCH_STATUS_OPTIONS } from '$lib/utils/enumOptions';
	import ErrorState from '$lib/components/ErrorState.svelte';

	let persons: Person[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let loadError: string | null = $state(null);
	let currentPage = $state(1);
	let sort = $state<'surname' | 'given_name' | 'birth_date' | 'updated_at'>('surname');
	let order = $state<'asc' | 'desc'>('asc');
	let researchStatusFilter = $state<ResearchStatus | 'unset' | ''>('');
	const pageSize = 20;

	async function loadPersons() {
		loading = true;
		loadError = null;
		try {
			const result = await api.listPersons({
				limit: pageSize,
				offset: (currentPage - 1) * pageSize,
				sort,
				order,
				research_status: researchStatusFilter || undefined
			});
			persons = result.items;
			total = result.total;
		} catch (e) {
			console.error('Failed to load persons:', e);
			loadError = 'Failed to load people. Please try again.';
		} finally {
			loading = false;
		}
	}

	function handleSortChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		sort = select.value as typeof sort;
		currentPage = 1;
		loadPersons();
	}

	function handleOrderChange() {
		order = order === 'asc' ? 'desc' : 'asc';
		loadPersons();
	}

	function handleStatusFilterChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		researchStatusFilter = select.value as typeof researchStatusFilter;
		currentPage = 1;
		loadPersons();
	}

	function prevPage() {
		if (currentPage > 1) {
			currentPage--;
			loadPersons();
		}
	}

	function nextPage() {
		if (currentPage * pageSize < total) {
			currentPage++;
			loadPersons();
		}
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

	{#if $page.state.notice}
		<p class="notice" role="status">{$page.state.notice}</p>
	{/if}

	<div class="toolbar">
		<button
			class="chip"
			class:chip-active={researchStatusFilter === 'possible'}
			onclick={() => {
				researchStatusFilter = researchStatusFilter === 'possible' ? '' : 'possible';
				currentPage = 1;
				loadPersons();
			}}
		>
			Quick Captures
		</button>
		<div class="controls">
			<label>
				Confidence:
				<select value={researchStatusFilter} onchange={handleStatusFilterChange}>
					<option value="">All</option>
					{#each RESEARCH_STATUS_OPTIONS as option (option.value)}
						<option value={option.value}>{option.label}</option>
					{/each}
					<option value="unset">Not assessed</option>
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
			<Button href="/import">Import GEDCOM</Button>
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

	.notice {
		margin: 0 0 1rem;
		padding: 0.75rem 1rem;
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		border-radius: 6px;
		color: #166534;
		font-size: 0.875rem;
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

	.persons-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
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
		color: #64748b;
		transition: all 0.15s;
	}

	.chip:hover {
		border-color: #cbd5e1;
		background: #f8fafc;
	}

	.chip-active {
		background: #fef3c7;
		border-color: #fbbf24;
		color: #92400e;
	}
</style>
