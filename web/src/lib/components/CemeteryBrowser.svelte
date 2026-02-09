<script lang="ts">
	import { api, type CemeteryEntry } from '$lib/api/client';

	let entries: CemeteryEntry[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let searchQuery = $state('');

	let filteredEntries = $derived(
		searchQuery
			? entries.filter((e) => e.place.toLowerCase().includes(searchQuery.toLowerCase()))
			: entries
	);

	async function loadCemeteries() {
		loading = true;
		error = null;
		try {
			const result = await api.getCemeteryIndex();
			entries = result.items;
		} catch (e) {
			console.error('Failed to load cemeteries:', e);
			error = 'Failed to load cemetery data. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadCemeteries();
	});
</script>

<div class="cemetery-browser">
	{#if loading}
		<div class="loading" role="status" aria-live="polite">Loading cemeteries...</div>
	{:else if error}
		<div class="error" role="alert">
			<p>{error}</p>
			<button onclick={loadCemeteries}>Retry</button>
		</div>
	{:else if entries.length === 0}
		<div class="empty">No burial or cremation records found</div>
	{:else}
		<!-- Search Filter -->
		<div class="search-bar">
			<label for="cemetery-search" class="sr-only">Filter cemeteries</label>
			<input
				id="cemetery-search"
				type="text"
				placeholder="Filter cemeteries..."
				bind:value={searchQuery}
				aria-label="Filter cemeteries by name"
			/>
			{#if searchQuery}
				<span class="filter-count" aria-live="polite">
					{filteredEntries.length} of {entries.length}
				</span>
			{/if}
		</div>

		<!-- Cemetery List -->
		<div class="cemetery-list" aria-label="Cemeteries and burial places">
			{#if filteredEntries.length === 0}
				<div class="empty">No cemeteries matching "{searchQuery}"</div>
			{:else}
				<div class="cemetery-grid">
					{#each filteredEntries as entry}
						<a
							href="/browse/cemeteries/{encodeURIComponent(entry.place)}"
							class="cemetery-item"
						>
							<div class="cemetery-info">
								<span class="cemetery-name">{entry.place}</span>
							</div>
							<span class="cemetery-count" aria-label="{entry.count} {entry.count === 1 ? 'person' : 'people'}">{entry.count}</span>
						</a>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.cemetery-browser {
		max-width: 100%;
	}

	.loading,
	.empty {
		text-align: center;
		padding: 2rem;
		color: #64748b;
	}

	.error {
		text-align: center;
		padding: 2rem;
		color: #dc2626;
	}

	.error button {
		margin-top: 1rem;
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.error button:hover {
		background: #f1f5f9;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border-width: 0;
	}

	.search-bar {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 1.5rem;
		padding: 0.75rem 1rem;
		background: #f8fafc;
		border-radius: 8px;
	}

	.search-bar input {
		flex: 1;
		padding: 0.5rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		background: white;
	}

	.search-bar input:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
		border-color: #3b82f6;
	}

	.filter-count {
		font-size: 0.8125rem;
		color: #64748b;
		white-space: nowrap;
	}

	.cemetery-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 0.75rem;
	}

	.cemetery-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem 1.25rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		text-decoration: none;
		color: inherit;
		transition: all 0.15s ease;
	}

	.cemetery-item:hover {
		background: #f8fafc;
		border-color: #3b82f6;
		transform: translateY(-1px);
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
	}

	.cemetery-item:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	.cemetery-info {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		min-width: 0;
	}

	.cemetery-name {
		font-weight: 500;
		color: #1e293b;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.cemetery-count {
		display: flex;
		align-items: center;
		justify-content: center;
		min-width: 28px;
		height: 28px;
		padding: 0 8px;
		background: #f1f5f9;
		color: #475569;
		font-size: 0.75rem;
		font-weight: 600;
		border-radius: 14px;
		flex-shrink: 0;
	}

	/* Mobile responsive */
	@media (max-width: 640px) {
		.search-bar {
			padding: 0.5rem 0.75rem;
		}

		.cemetery-grid {
			grid-template-columns: 1fr;
		}

		.cemetery-item {
			padding: 0.875rem 1rem;
		}
	}
</style>
