<script lang="ts">
	import { api, type BrickWallEntry } from '$lib/api/client';

	let entries: BrickWallEntry[] = $state([]);
	let activeCount = $state(0);
	let resolvedCount = $state(0);
	let loading = $state(true);
	let error: string | null = $state(null);
	let includeResolved = $state(false);

	async function loadBrickWalls() {
		loading = true;
		error = null;
		try {
			const result = await api.getBrickWalls(includeResolved);
			entries = result.items;
			activeCount = result.active_count;
			resolvedCount = result.resolved_count;
		} catch (e) {
			console.error('Failed to load brick walls:', e);
			error = 'Failed to load brick wall data. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadBrickWalls();
	});

	function toggleResolved() {
		includeResolved = !includeResolved;
	}

	function formatRelativeTime(dateStr: string): string {
		const date = new Date(dateStr);
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

		if (diffDays === 0) return 'today';
		if (diffDays === 1) return '1 day ago';
		if (diffDays < 30) return `${diffDays} days ago`;
		const diffMonths = Math.floor(diffDays / 30);
		if (diffMonths === 1) return '1 month ago';
		if (diffMonths < 12) return `${diffMonths} months ago`;
		const diffYears = Math.floor(diffMonths / 12);
		if (diffYears === 1) return '1 year ago';
		return `${diffYears} years ago`;
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString();
	}
</script>

<svelte:head>
	<title>Brick Walls | My Family</title>
</svelte:head>

<div class="browse-page">
	<header class="page-header">
		<h1>Brick Walls</h1>
		<p class="description">Track and celebrate research breakthroughs.</p>
	</header>

	{#if loading}
		<div class="loading" role="status" aria-live="polite">Loading brick walls...</div>
	{:else if error}
		<div class="error" role="alert">
			<p>{error}</p>
			<button onclick={loadBrickWalls}>Retry</button>
		</div>
	{:else if activeCount === 0 && resolvedCount === 0}
		<div class="empty">No brick walls yet. Mark a person as a brick wall from their profile.</div>
	{:else}
		<div class="summary-stats">
			<div class="stat">
				<span class="stat-value">{activeCount}</span>
				<span class="stat-label">Active</span>
			</div>
			<div class="stat">
				<span class="stat-value">{resolvedCount}</span>
				<span class="stat-label">Resolved</span>
			</div>
		</div>

		<div class="controls">
			<label class="toggle-label">
				<input type="checkbox" checked={includeResolved} onchange={toggleResolved} />
				Show resolved brick walls
			</label>
		</div>

		{#if entries.length === 0}
			<div class="empty">
				{#if includeResolved}
					No brick walls found.
				{:else}
					No active brick walls. Toggle above to see resolved ones.
				{/if}
			</div>
		{:else}
			<div class="brick-wall-grid" aria-label="Brick wall list">
				{#each entries as entry}
					<div class="brick-wall-card" class:resolved={entry.resolved_at}>
						<div class="card-header">
							<a href="/persons/{entry.person_id}" class="person-link">
								{entry.person_name}
							</a>
							{#if entry.resolved_at}
								<span class="badge badge-resolved">
									<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="badge-icon">
										<polyline points="20 6 9 17 4 12" />
									</svg>
									Resolved
								</span>
							{:else}
								<span class="badge badge-active">
									<svg viewBox="0 0 24 24" fill="currentColor" class="badge-icon">
										<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/>
									</svg>
									Active
								</span>
							{/if}
						</div>
						{#if entry.note}
							<p class="note">{entry.note}</p>
						{/if}
						<div class="card-footer">
							{#if entry.resolved_at}
								<span class="meta">Resolved {formatDate(entry.resolved_at)}</span>
							{:else}
								<span class="meta">Marked {formatRelativeTime(entry.since)}</span>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.browse-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		margin-bottom: 2rem;
	}

	.page-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.description {
		margin: 0;
		color: #64748b;
		font-size: 0.9375rem;
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

	.summary-stats {
		display: flex;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	.stat {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 1rem 2rem;
		background: white;
		border-radius: 8px;
		border: 1px solid #e2e8f0;
	}

	.stat-value {
		font-size: 1.5rem;
		font-weight: 700;
		color: #1e293b;
	}

	.stat-label {
		font-size: 0.8125rem;
		color: #64748b;
		margin-top: 0.25rem;
	}

	.controls {
		margin-bottom: 1.5rem;
	}

	.toggle-label {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: #475569;
		cursor: pointer;
	}

	.toggle-label input {
		accent-color: #3b82f6;
	}

	.brick-wall-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
		gap: 0.75rem;
	}

	.brick-wall-card {
		padding: 1.25rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-left: 4px solid #f59e0b;
		border-radius: 8px;
		transition: all 0.15s ease;
	}

	.brick-wall-card:hover {
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
	}

	.brick-wall-card.resolved {
		border-left-color: #22c55e;
	}

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	.person-link {
		font-weight: 600;
		color: #1e293b;
		text-decoration: none;
		font-size: 0.9375rem;
	}

	.person-link:hover {
		color: #3b82f6;
	}

	.badge {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.125rem 0.5rem;
		border-radius: 9999px;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.025em;
	}

	.badge-icon {
		width: 0.75rem;
		height: 0.75rem;
	}

	.badge-active {
		background: #fef3c7;
		color: #b45309;
	}

	.badge-resolved {
		background: #dcfce7;
		color: #15803d;
	}

	.note {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		color: #475569;
		line-height: 1.5;
	}

	.card-footer {
		display: flex;
		align-items: center;
	}

	.meta {
		font-size: 0.75rem;
		color: #94a3b8;
	}

	@media (max-width: 640px) {
		.brick-wall-grid {
			grid-template-columns: 1fr;
		}

		.summary-stats {
			justify-content: center;
		}
	}
</style>
