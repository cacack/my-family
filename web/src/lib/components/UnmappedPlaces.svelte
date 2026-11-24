<script lang="ts">
	/**
	 * The no-coordinates worklist (#1561, ADR-009 §9): each place with records
	 * that have no coordinates, most such records first, with the pairs other
	 * records there already carry. It only reads; each name links to the place's
	 * people.
	 */
	import { api, type UnmappedPlace } from '#lib/api/client.js';
	import ErrorState from './ErrorState.svelte';
	import LoadingState from './LoadingState.svelte';
	import EmptyState from './EmptyState.svelte';

	let entries: UnmappedPlace[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	async function load() {
		loading = true;
		error = null;
		try {
			entries = (await api.listUnmappedPlaces()).items;
		} catch (e) {
			console.error('Failed to load places without coordinates:', e);
			error = 'Failed to load places without coordinates. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	function records(n: number): string {
		return `${n} ${n === 1 ? 'record' : 'records'}`;
	}
</script>

{#if loading}
	<LoadingState label="Loading places..." />
{:else if error}
	<ErrorState message={error} onRetry={load} />
{:else if entries.length === 0}
	<EmptyState
		message="Every place has coordinates."
		description="No record names a place without a latitude and longitude."
	/>
{:else}
	<p class="summary">{entries.length} {entries.length === 1 ? 'place' : 'places'} without coordinates</p>
	<ul class="unmapped-list">
		{#each entries as entry (entry.id)}
			<li class="unmapped-item">
				<div class="unmapped-head">
					<a href="/browse/places/{encodeURIComponent(entry.spelling)}" class="unmapped-name">{entry.name}</a>
					<span class="unmapped-count">{records(entry.count)} without coordinates</span>
				</div>
				{#if entry.suggestions.length > 0}
					<div class="suggestions">
						<span class="suggestions-label">Other records here have:</span>
						<ul>
							{#each entry.suggestions as s (s.latitude + ' ' + s.longitude)}
								<li><span class="pair">{s.latitude}, {s.longitude}</span> ({records(s.count)})</li>
							{/each}
						</ul>
					</div>
				{:else}
					<p class="no-suggestions">No other record here has coordinates.</p>
				{/if}
			</li>
		{/each}
	</ul>
{/if}

<style>
	.summary {
		margin: 0 0 1rem;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.unmapped-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.unmapped-item {
		padding: 1rem 1.25rem;
		background: white;
		border: 1px solid var(--a11y-color-border);
		border-radius: 8px;
	}

	.unmapped-head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.25rem 1rem;
	}

	.unmapped-name {
		font-weight: 500;
		color: var(--color-link);
		overflow-wrap: anywhere;
		min-width: 0;
	}

	.unmapped-count {
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.suggestions,
	.no-suggestions {
		margin: 0.5rem 0 0;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.suggestions ul {
		margin: 0.25rem 0 0;
		padding-left: 1.25rem;
	}

	.pair {
		font-family: ui-monospace, monospace;
		color: var(--color-text);
		overflow-wrap: anywhere;
	}
</style>
