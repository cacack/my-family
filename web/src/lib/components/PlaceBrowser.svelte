<script lang="ts">
	import { api, type PlaceEntry } from '$lib/api/client';

	let places: PlaceEntry[] = $state([]);
	let breadcrumb: string[] = $state([]);
	let currentParent = $state('');
	let loading = $state(true);

	async function loadPlaces(parent: string = '') {
		loading = true;
		try {
			const result = await api.getPlaceHierarchy(parent);
			places = result.items;
			breadcrumb = result.breadcrumb || [];
			currentParent = parent;
		} catch (e) {
			console.error('Failed to load places:', e);
		} finally {
			loading = false;
		}
	}

	function navigateToPlace(place: PlaceEntry) {
		if (place.has_children) {
			loadPlaces(place.full_name);
		}
	}

	function navigateToBreadcrumb(index: number) {
		if (index < 0) {
			// Go to root
			loadPlaces('');
		} else {
			// Build the path up to this breadcrumb
			const path = breadcrumb
				.slice(0, index + 1)
				.reverse()
				.join(', ');
			loadPlaces(path);
		}
	}

	$effect(() => {
		loadPlaces();
	});
</script>

<div class="place-browser">
	{#if loading}
		<div class="loading" role="status" aria-live="polite">Loading places...</div>
	{:else}
		<!-- Breadcrumb Navigation -->
		<nav class="breadcrumb" aria-label="Place hierarchy">
			<button class="breadcrumb-item" class:active={breadcrumb.length === 0} onclick={() => navigateToBreadcrumb(-1)}>
				<svg class="home-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
					<polyline points="9 22 9 12 15 12 15 22" />
				</svg>
				All Places
			</button>

			{#each breadcrumb as crumb, i}
				<span class="breadcrumb-separator">/</span>
				<button
					class="breadcrumb-item"
					class:active={i === breadcrumb.length - 1}
					onclick={() => navigateToBreadcrumb(i)}
				>
					{crumb}
				</button>
			{/each}
		</nav>

		<!-- Place List -->
		<div class="place-list">
			{#if places.length === 0}
				<div class="empty">
					{#if currentParent}
						No sub-locations found for this place
					{:else}
						No places found in the database
					{/if}
				</div>
			{:else}
				<div class="place-grid">
					{#each places as place}
						{#if place.has_children}
							<button class="place-item expandable" onclick={() => navigateToPlace(place)}>
								<div class="place-info">
									<span class="place-name">{place.name}</span>
									{#if place.has_children}
										<span class="place-hint">Click to view sub-locations</span>
									{/if}
								</div>
								<div class="place-actions">
									<span class="place-count">{place.count}</span>
									<svg class="chevron-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
										<polyline points="9 18 15 12 9 6" />
									</svg>
								</div>
							</button>
						{:else}
							<a
								href="/browse/places/{encodeURIComponent(place.full_name)}"
								class="place-item"
							>
								<div class="place-info">
									<span class="place-name">{place.name}</span>
								</div>
								<span class="place-count">{place.count}</span>
							</a>
						{/if}
					{/each}
				</div>
			{/if}
		</div>

		{#if currentParent && places.length > 0}
			<div class="view-all">
				<a href="/browse/places/{encodeURIComponent(currentParent)}" class="view-all-link">
					View all people from {breadcrumb[breadcrumb.length - 1] || currentParent}
				</a>
			</div>
		{/if}
	{/if}
</div>

<style>
	.place-browser {
		max-width: 100%;
	}

	.loading,
	.empty {
		text-align: center;
		padding: 2rem;
		color: #64748b;
	}

	.breadcrumb {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
		padding: 1rem;
		background: #f8fafc;
		border-radius: 8px;
		margin-bottom: 1.5rem;
	}

	.breadcrumb-item {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.375rem 0.75rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		color: #475569;
		cursor: pointer;
		transition: all 0.15s ease;
	}

	.breadcrumb-item:hover {
		background: #f1f5f9;
		border-color: #3b82f6;
		color: #3b82f6;
	}

	.breadcrumb-item.active {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
		cursor: default;
	}

	.home-icon {
		width: 1rem;
		height: 1rem;
	}

	.breadcrumb-separator {
		color: #94a3b8;
	}

	.place-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 0.75rem;
	}

	.place-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem 1.25rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		text-decoration: none;
		color: inherit;
		cursor: pointer;
		transition: all 0.15s ease;
		text-align: left;
		width: 100%;
	}

	.place-item:hover {
		background: #f8fafc;
		border-color: #3b82f6;
		transform: translateY(-1px);
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
	}

	.place-item.expandable {
		border-left: 3px solid #3b82f6;
	}

	.place-info {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.place-name {
		font-weight: 500;
		color: #1e293b;
	}

	.place-hint {
		font-size: 0.75rem;
		color: #94a3b8;
	}

	.place-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.place-count {
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
	}

	.chevron-icon {
		width: 1.25rem;
		height: 1.25rem;
		color: #94a3b8;
	}

	.place-item:hover .chevron-icon {
		color: #3b82f6;
	}

	.view-all {
		margin-top: 1.5rem;
		text-align: center;
	}

	.view-all-link {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1.5rem;
		background: #f1f5f9;
		color: #475569;
		border-radius: 8px;
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 500;
		transition: all 0.15s ease;
	}

	.view-all-link:hover {
		background: #e2e8f0;
		color: #1e293b;
	}

	/* Mobile responsive */
	@media (max-width: 640px) {
		.breadcrumb {
			padding: 0.75rem;
		}

		.breadcrumb-item {
			padding: 0.25rem 0.5rem;
			font-size: 0.8125rem;
		}

		.place-grid {
			grid-template-columns: 1fr;
		}

		.place-item {
			padding: 0.875rem 1rem;
		}
	}
</style>
