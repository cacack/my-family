<script lang="ts">
	import { page } from '$app/stores';
	import { untrack } from 'svelte';
	import { api, type Person } from '$lib/api/client';
	import PersonCard from '$lib/components/PersonCard.svelte';

	let place = $derived(decodeURIComponent($page.params.place ?? ''));
	let persons: Person[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let currentPage = $state(1);
	const pageSize = 20;

	async function loadPersons() {
		loading = true;
		try {
			const result = await api.getPersonsByCemetery(place, {
				limit: pageSize,
				offset: (currentPage - 1) * pageSize
			});
			persons = result.items;
			total = result.total;
		} catch (e) {
			console.error('Failed to load persons:', e);
		} finally {
			loading = false;
		}
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
		// Subscribe only to place changes
		void place;
		untrack(() => {
			currentPage = 1;
			loadPersons();
		});
	});

	const totalPages = $derived(Math.ceil(total / pageSize));
</script>

<svelte:head>
	<title>{place} | Browse Cemeteries | My Family</title>
</svelte:head>

<div class="cemetery-detail-page">
	<header class="page-header">
		<nav class="breadcrumb" aria-label="Breadcrumb">
			<a href="/browse/cemeteries">Browse Cemeteries</a>
			<span class="separator">/</span>
			<span class="current">{place}</span>
		</nav>
		<h1>People at "{place}"</h1>
		{#if total > 0}
			<p class="result-count">{total} {total === 1 ? 'person' : 'people'} found</p>
		{/if}
	</header>

	{#if loading}
		<div class="loading" role="status" aria-live="polite">Loading...</div>
	{:else if persons.length === 0}
		<div class="empty">
			<p>No people found at "{place}".</p>
			<a href="/browse/cemeteries" class="back-link">Back to cemetery browser</a>
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
	.cemetery-detail-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		margin-bottom: 1.5rem;
	}

	.breadcrumb {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 1rem;
		font-size: 0.875rem;
	}

	.breadcrumb a {
		color: #3b82f6;
		text-decoration: none;
	}

	.breadcrumb a:hover {
		text-decoration: underline;
	}

	.breadcrumb .separator {
		color: #94a3b8;
	}

	.breadcrumb .current {
		color: #64748b;
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.result-count {
		margin: 0.5rem 0 0;
		color: #64748b;
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

	.back-link {
		display: inline-block;
		padding: 0.5rem 1rem;
		background: #f1f5f9;
		color: #475569;
		border-radius: 6px;
		text-decoration: none;
	}

	.back-link:hover {
		background: #e2e8f0;
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
</style>
