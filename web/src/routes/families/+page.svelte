<script lang="ts">
	import { page } from '$app/stores';
	import { api, type FamilyDetail } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import FamilyCard from '$lib/components/FamilyCard.svelte';
	import { readPositiveInt, setQuery } from '$lib/utils/urlState';

	let families: FamilyDetail[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let loadError: string | null = $state(null);
	const pageSize = 20;
	// The page lives in the URL (see urlState.ts), so Back and reload restore it.
	const currentPage = $derived(readPositiveInt($page.url.searchParams, 'page', 1));

	// Back/Forward can start loads faster than they finish: only the latest one may land.
	let loadSeq = 0;

	async function loadFamilies() {
		const seq = ++loadSeq;
		loading = true;
		loadError = null;
		try {
			const result = await api.listFamilies({
				limit: pageSize,
				offset: (currentPage - 1) * pageSize
			});
			if (seq !== loadSeq) return;
			families = result.items;
			total = result.total;
		} catch (e) {
			if (seq !== loadSeq) return;
			console.error('Failed to load families:', e);
			loadError = 'Failed to load families. Please try again.';
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	function goToPage(n: number) {
		setQuery($page.url, { page: n }, { push: true, defaults: { page: 1 } });
	}

	function prevPage() {
		if (currentPage > 1) goToPage(currentPage - 1);
	}

	function nextPage() {
		if (currentPage * pageSize < total) goToPage(currentPage + 1);
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
	<PageHeader title="Families" />

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if loadError}
		<div class="load-error" role="alert">
			<p>{loadError}</p>
			<Button variant="outline" onclick={loadFamilies}>Retry</Button>
		</div>
	{:else if families.length === 0}
		<div class="empty">
			<p>No families found.</p>
			<Button href="/import">Import GEDCOM</Button>
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

	.loading,
	.empty {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.empty p {
		margin: 0 0 1rem;
	}

	.load-error {
		text-align: center;
		padding: 3rem;
		color: #dc2626;
	}

	.load-error p {
		margin: 0 0 1rem;
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
