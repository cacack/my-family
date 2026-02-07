<script lang="ts">
	import { api, type RestorePointsResponse } from '$lib/api/client';

	interface Props {
		entityType: 'person' | 'family' | 'source' | 'citation';
		entityId: string;
		currentVersion: number;
		onSelectVersion: (version: number, summary: string) => void;
	}

	let { entityType, entityId, currentVersion, onSelectVersion }: Props = $props();

	let restorePoints: RestorePointsResponse | null = $state(null);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error: string | null = $state(null);

	const PAGE_SIZE = 20;

	function formatTimestamp(iso: string): string {
		const date = new Date(iso);
		return date.toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	function getActionBadgeClass(action: string): string {
		switch (action) {
			case 'created':
				return 'badge-created';
			case 'updated':
				return 'badge-updated';
			case 'deleted':
				return 'badge-deleted';
			case 'linked':
			case 'unlinked':
				return 'badge-linked';
			default:
				return '';
		}
	}

	async function fetchRestorePoints(type: string, id: string, offset: number = 0) {
		const params = { limit: PAGE_SIZE, offset };
		switch (type) {
			case 'person':
				return api.getPersonRestorePoints(id, params);
			case 'family':
				return api.getFamilyRestorePoints(id, params);
			case 'source':
				return api.getSourceRestorePoints(id, params);
			case 'citation':
				return api.getCitationRestorePoints(id, params);
			default:
				throw new Error('Unknown entity type: ' + type);
		}
	}

	async function loadRestorePoints() {
		loading = true;
		error = null;
		try {
			restorePoints = await fetchRestorePoints(entityType, entityId);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load restore points';
			restorePoints = null;
		} finally {
			loading = false;
		}
	}

	async function loadMore() {
		if (!restorePoints || !restorePoints.has_more) return;

		loadingMore = true;
		try {
			const nextOffset = restorePoints.items.length;
			const morePoints = await fetchRestorePoints(entityType, entityId, nextOffset);

			restorePoints = {
				...morePoints,
				items: [...restorePoints.items, ...morePoints.items]
			};
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load more';
		} finally {
			loadingMore = false;
		}
	}

	$effect(() => {
		if (entityType && entityId) {
			loadRestorePoints();
		}
	});
</script>

<div class="restore-point-browser">
	{#if loading}
		<div class="loading" role="status" aria-live="polite">Loading restore points...</div>
	{:else if error}
		<div class="error" role="alert">{error}</div>
	{:else if restorePoints && restorePoints.items.length > 0}
		<p class="description">
			Select a version to restore this {entityType} to a previous state.
			A new version will be created with the restored data.
		</p>

		<div class="timeline">
			{#each restorePoints.items as point (point.version)}
				{@const isCurrent = point.version === currentVersion}

				<div class="timeline-entry" class:current={isCurrent}>
					<div class="version-marker">
						<span class="version-number">v{point.version}</span>
					</div>
					<div class="entry-content">
						<div class="entry-header">
							<span class="timestamp">{formatTimestamp(point.timestamp)}</span>
							<span class="action-badge {getActionBadgeClass(point.action)}">{point.action}</span>
							{#if isCurrent}
								<span class="current-badge">Current</span>
							{/if}
						</div>
						<div class="entry-summary">{point.summary}</div>
						{#if !isCurrent}
							<button
								class="restore-btn"
								onclick={() => onSelectVersion(point.version, point.summary)}
							>
								Restore to this version
							</button>
						{/if}
					</div>
				</div>
			{/each}
		</div>

		{#if restorePoints.has_more}
			<div class="load-more">
				<button class="btn" onclick={loadMore} disabled={loadingMore}>
					{loadingMore ? 'Loading...' : 'Load more'}
				</button>
			</div>
		{/if}
	{:else}
		<div class="empty">No restore points available.</div>
	{/if}
</div>

<style>
	.restore-point-browser {
		width: 100%;
	}

	.description {
		margin: 0 0 1rem;
		font-size: 0.8125rem;
		color: #64748b;
		line-height: 1.5;
	}

	.loading,
	.error,
	.empty {
		text-align: center;
		padding: 2rem;
		color: #64748b;
	}

	.error {
		color: #dc2626;
	}

	.timeline {
		display: flex;
		flex-direction: column;
		gap: 0;
		position: relative;
	}

	.timeline::before {
		content: '';
		position: absolute;
		left: 1.25rem;
		top: 0.75rem;
		bottom: 0.75rem;
		width: 2px;
		background: #e2e8f0;
	}

	.timeline-entry {
		display: flex;
		gap: 0.75rem;
		padding: 0.75rem 0;
		position: relative;
	}

	.version-marker {
		display: flex;
		align-items: flex-start;
		justify-content: center;
		width: 2.5rem;
		flex-shrink: 0;
		z-index: 1;
	}

	.version-number {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 2rem;
		height: 1.5rem;
		padding: 0 0.375rem;
		background: #f1f5f9;
		border: 2px solid #e2e8f0;
		border-radius: 9999px;
		font-size: 0.6875rem;
		font-weight: 600;
		color: #64748b;
	}

	.current .version-number {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.entry-content {
		flex: 1;
		min-width: 0;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.75rem 1rem;
	}

	.current .entry-content {
		border-color: #93c5fd;
		background: #eff6ff;
	}

	.entry-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 0.375rem;
	}

	.timestamp {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.action-badge {
		display: inline-block;
		padding: 0.0625rem 0.375rem;
		border-radius: 4px;
		font-size: 0.6875rem;
		font-weight: 500;
		text-transform: capitalize;
	}

	.badge-created {
		background: #22c55e;
		color: white;
	}

	.badge-updated {
		background: #3b82f6;
		color: white;
	}

	.badge-deleted {
		background: #ef4444;
		color: white;
	}

	.badge-linked {
		background: #8b5cf6;
		color: white;
	}

	.current-badge {
		display: inline-block;
		padding: 0.0625rem 0.375rem;
		border-radius: 4px;
		font-size: 0.6875rem;
		font-weight: 600;
		background: #dbeafe;
		color: #1d4ed8;
	}

	.entry-summary {
		font-size: 0.8125rem;
		color: #475569;
	}

	.restore-btn {
		display: inline-block;
		margin-top: 0.5rem;
		padding: 0.25rem 0.625rem;
		border: 1px solid #f59e0b;
		border-radius: 4px;
		background: #fffbeb;
		color: #b45309;
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s;
	}

	.restore-btn:hover {
		background: #fef3c7;
		border-color: #d97706;
	}

	.load-more {
		display: flex;
		justify-content: center;
		margin-top: 1rem;
	}

	.btn {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		color: #475569;
	}

	.btn:hover:not(:disabled) {
		background: #f1f5f9;
	}

	.btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
</style>
