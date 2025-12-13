<script lang="ts">
	import { api, type FamilyDetail } from '$lib/api/client';
	import FamilyCard from '$lib/components/FamilyCard.svelte';

	let families: FamilyDetail[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let currentPage = $state(1);
	const pageSize = 20;

	async function loadFamilies() {
		loading = true;
		try {
			const result = await api.listFamilies({
				limit: pageSize,
				offset: (currentPage - 1) * pageSize
			});
			families = result.items;
			total = result.total;
		} catch (e) {
			console.error('Failed to load families:', e);
		} finally {
			loading = false;
		}
	}

	function prevPage() {
		if (currentPage > 1) {
			currentPage--;
			loadFamilies();
		}
	}

	function nextPage() {
		if (currentPage * pageSize < total) {
			currentPage++;
			loadFamilies();
		}
	}

	$effect(() => {
		loadFamilies();
	});

	const totalPages = $derived(Math.ceil(total / pageSize));
</script>

<svelte:head>
	<title>Families | My Family</title>
</svelte:head>

<div class="families-page">
	<header class="page-header">
		<h1>Families</h1>
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if families.length === 0}
		<div class="empty">
			<p>No families found.</p>
			<a href="/import" class="btn-primary">Import GEDCOM</a>
		</div>
	{:else}
		<div class="families-grid">
			{#each families as family}
				<FamilyCard {family} href="/families/{family.id}" />
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
	.families-page {
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

	.families-grid {
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
