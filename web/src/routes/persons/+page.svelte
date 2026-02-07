<script lang="ts">
	import { api, type Person, type ResearchStatus } from '$lib/api/client';
	import PersonCard from '$lib/components/PersonCard.svelte';

	let persons: Person[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let currentPage = $state(1);
	let sort = $state<'surname' | 'given_name' | 'birth_date' | 'updated_at'>('surname');
	let order = $state<'asc' | 'desc'>('asc');
	let researchStatusFilter = $state<ResearchStatus | 'unset' | ''>('');
	const pageSize = 20;

	async function loadPersons() {
		loading = true;
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
	<header class="page-header">
		<h1>People</h1>
		<div class="header-actions">
			<a href="/persons/add" class="btn">Add Person</a>
			<a href="/persons/quick" class="btn btn-accent">Quick Capture</a>
		</div>
	</header>

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
					<option value="certain">Certain</option>
					<option value="probable">Probable</option>
					<option value="possible">Possible</option>
					<option value="unknown">Unknown</option>
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
		</div>
	</div>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if persons.length === 0}
		<div class="empty">
			<p>No people found.</p>
			<a href="/import" class="btn-primary">Import GEDCOM</a>
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

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
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

	.loading,
	.empty {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.empty p {
		margin: 0 0 1rem;
	}

	.btn-primary {
		display: inline-block;
		padding: 0.75rem 1.5rem;
		background: #3b82f6;
		color: white;
		border-radius: 8px;
		text-decoration: none;
		font-weight: 500;
	}

	.btn-primary:hover {
		background: #2563eb;
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

	.header-actions {
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

	.btn-accent {
		background: #f0f9ff;
		border-color: #7dd3fc;
		color: #0369a1;
	}

	.btn-accent:hover {
		background: #e0f2fe;
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
