<script lang="ts">
	import { api, type DiscoverySuggestion } from '$lib/api/client';

	let suggestions: DiscoverySuggestion[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let error: string | null = $state(null);
	let activeFilter: string | null = $state(null);

	const typeLabels: Record<string, string> = {
		missing_data: 'Missing Data',
		orphan: 'Orphans',
		unassessed: 'Unassessed',
		quality_gap: 'Quality Gaps',
		brick_wall_resolved: 'Breakthroughs'
	};

	let filteredSuggestions = $derived(
		activeFilter
			? suggestions.filter((s) => s.type === activeFilter)
			: suggestions
	);

	let availableTypes = $derived(
		[...new Set(suggestions.map((s) => s.type))]
	);

	let displayedSuggestions = $derived(filteredSuggestions);

	let hasMore = $derived(total > suggestions.length);

	async function loadFeed() {
		loading = true;
		error = null;
		try {
			const result = await api.getDiscoveryFeed(20);
			suggestions = result.items;
			total = result.total;
		} catch (e) {
			console.error('Failed to load discovery feed:', e);
			error = 'Failed to load suggestions.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadFeed();
	});

	function toggleFilter(type: string) {
		if (activeFilter === type) {
			activeFilter = null;
		} else {
			activeFilter = type;
		}
	}
</script>

<div class="discovery-feed">
	{#if loading}
		<div class="loading" role="status" aria-live="polite">Loading suggestions...</div>
	{:else if error}
		<div class="error" role="alert">{error}</div>
	{:else if suggestions.length === 0}
		<div class="empty">Your tree looks great! No suggestions right now.</div>
	{:else}
		{#if availableTypes.length > 1}
			<div class="filter-chips" role="group" aria-label="Filter suggestions by type">
				{#each availableTypes as type}
					<button
						class="chip"
						class:active={activeFilter === type}
						onclick={() => toggleFilter(type)}
					>
						{typeLabels[type] || type}
					</button>
				{/each}
			</div>
		{/if}

		<div class="feed-grid">
			{#each displayedSuggestions as suggestion}
				<a href={suggestion.action_url} class="feed-card" data-type={suggestion.type}>
					<div class="card-icon">
						{#if suggestion.type === 'missing_data'}
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<circle cx="11" cy="11" r="8" />
								<line x1="21" y1="21" x2="16.65" y2="16.65" />
							</svg>
						{:else if suggestion.type === 'orphan'}
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
								<path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
							</svg>
						{:else if suggestion.type === 'unassessed'}
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M9 11l3 3L22 4" />
								<path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11" />
							</svg>
						{:else if suggestion.type === 'brick_wall_resolved'}
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
							</svg>
						{:else}
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<line x1="18" y1="20" x2="18" y2="10" />
								<line x1="12" y1="20" x2="12" y2="4" />
								<line x1="6" y1="20" x2="6" y2="14" />
							</svg>
						{/if}
					</div>
					<div class="card-content">
						<h4 class="card-title">{suggestion.title}</h4>
						<p class="card-desc">{suggestion.description}</p>
					</div>
				</a>
			{/each}
		</div>

		{#if hasMore}
			<div class="view-more">
				<span class="view-more-text">
					Showing {suggestions.length} of {total} suggestions
				</span>
			</div>
		{/if}
	{/if}
</div>

<style>
	.discovery-feed {
		max-width: 100%;
	}

	.loading,
	.empty {
		text-align: center;
		padding: 1.5rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	.error {
		text-align: center;
		padding: 1.5rem;
		color: #dc2626;
		font-size: 0.875rem;
	}

	.filter-chips {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1rem;
		flex-wrap: wrap;
	}

	.chip {
		padding: 0.375rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 9999px;
		background: white;
		font-size: 0.75rem;
		font-weight: 500;
		color: #64748b;
		cursor: pointer;
		transition: all 0.15s;
	}

	.chip:hover {
		border-color: #cbd5e1;
		background: #f8fafc;
	}

	.chip.active {
		background: #eff6ff;
		border-color: #3b82f6;
		color: #3b82f6;
	}

	.feed-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 0.75rem;
	}

	.feed-card {
		display: flex;
		gap: 0.75rem;
		padding: 1rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		text-decoration: none;
		color: inherit;
		transition: all 0.15s ease;
	}

	.feed-card:hover {
		border-color: #3b82f6;
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
		transform: translateY(-1px);
	}

	.feed-card:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	.card-icon {
		flex-shrink: 0;
		width: 2rem;
		height: 2rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 8px;
		padding: 0.375rem;
	}

	.card-icon svg {
		width: 100%;
		height: 100%;
	}

	[data-type="missing_data"] .card-icon {
		background: #fef3c7;
		color: #b45309;
	}

	[data-type="orphan"] .card-icon {
		background: #fce7f3;
		color: #be185d;
	}

	[data-type="unassessed"] .card-icon {
		background: #e0e7ff;
		color: #4338ca;
	}

	[data-type="brick_wall_resolved"] .card-icon {
		background: #dcfce7;
		color: #15803d;
	}

	[data-type="quality_gap"] .card-icon {
		background: #f0f9ff;
		color: #0369a1;
	}

	.card-content {
		min-width: 0;
	}

	.card-title {
		margin: 0 0 0.25rem;
		font-size: 0.8125rem;
		font-weight: 600;
		color: #1e293b;
	}

	.card-desc {
		margin: 0;
		font-size: 0.75rem;
		color: #64748b;
		line-height: 1.4;
		overflow: hidden;
		text-overflow: ellipsis;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
	}

	.view-more {
		text-align: center;
		margin-top: 1rem;
	}

	.view-more-text {
		font-size: 0.8125rem;
		color: #64748b;
	}

	@media (max-width: 640px) {
		.feed-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
