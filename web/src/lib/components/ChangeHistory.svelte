<script lang="ts">
	import { api, type ChangeEntry, type ChangeHistoryResponse } from '$lib/api/client';
	import DiffView from './DiffView.svelte';

	interface Props {
		entityType?: string;
		entityId?: string;
	}

	let { entityType, entityId }: Props = $props();

	let history: ChangeHistoryResponse | null = $state(null);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error: string | null = $state(null);
	let expandedEntries: Set<string> = $state(new Set());

	// Filter state for global view
	let filterEntityType = $state('');

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
			default:
				return '';
		}
	}

	function getEntityLink(entry: ChangeEntry): string | null {
		if (entry.action === 'deleted') {
			return null;
		}
		switch (entry.entity_type) {
			case 'person':
				return `/persons/${entry.entity_id}`;
			case 'family':
				return `/families/${entry.entity_id}`;
			case 'source':
				return `/sources/${entry.entity_id}`;
			default:
				return null;
		}
	}

	function toggleExpanded(id: string) {
		const newSet = new Set(expandedEntries);
		if (newSet.has(id)) {
			newSet.delete(id);
		} else {
			newSet.add(id);
		}
		expandedEntries = newSet;
	}

	async function loadHistory() {
		loading = true;
		error = null;
		try {
			if (entityType && entityId) {
				// Entity-specific history
				if (entityType === 'person') {
					history = await api.getPersonHistory(entityId, { limit: PAGE_SIZE, offset: 0 });
				} else if (entityType === 'family') {
					history = await api.getFamilyHistory(entityId, { limit: PAGE_SIZE, offset: 0 });
				} else if (entityType === 'source') {
					history = await api.getSourceHistory(entityId, { limit: PAGE_SIZE, offset: 0 });
				}
			} else {
				// Global history
				history = await api.getGlobalHistory({
					entity_type: filterEntityType || undefined,
					limit: PAGE_SIZE,
					offset: 0
				});
			}
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load history';
			history = null;
		} finally {
			loading = false;
		}
	}

	async function loadMore() {
		if (!history || !history.has_more) return;

		loadingMore = true;
		try {
			const nextOffset = history.offset + history.limit;
			let moreHistory: ChangeHistoryResponse;

			if (entityType && entityId) {
				if (entityType === 'person') {
					moreHistory = await api.getPersonHistory(entityId, { limit: PAGE_SIZE, offset: nextOffset });
				} else if (entityType === 'family') {
					moreHistory = await api.getFamilyHistory(entityId, { limit: PAGE_SIZE, offset: nextOffset });
				} else if (entityType === 'source') {
					moreHistory = await api.getSourceHistory(entityId, { limit: PAGE_SIZE, offset: nextOffset });
				} else {
					return;
				}
			} else {
				moreHistory = await api.getGlobalHistory({
					entity_type: filterEntityType || undefined,
					limit: PAGE_SIZE,
					offset: nextOffset
				});
			}

			history = {
				...moreHistory,
				items: [...history.items, ...moreHistory.items]
			};
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load more';
		} finally {
			loadingMore = false;
		}
	}

	function handleFilterChange() {
		loadHistory();
	}

	$effect(() => {
		loadHistory();
	});
</script>

<div class="change-history">
	{#if !entityType && !entityId}
		<div class="filters">
			<label>
				Entity Type
				<select bind:value={filterEntityType} onchange={handleFilterChange}>
					<option value="">All</option>
					<option value="person">Person</option>
					<option value="family">Family</option>
					<option value="source">Source</option>
					<option value="citation">Citation</option>
				</select>
			</label>
		</div>
	{/if}

	{#if loading}
		<div class="loading">Loading history...</div>
	{:else if error}
		<div class="error">{error}</div>
	{:else if history && history.items.length > 0}
		<div class="timeline">
			{#each history.items as entry (entry.id)}
				{@const link = getEntityLink(entry)}
				{@const hasChanges = entry.changes && Object.keys(entry.changes).length > 0}
				{@const isExpanded = expandedEntries.has(entry.id)}

				<div class="timeline-entry">
					<div class="entry-header">
						<span class="timestamp">{formatTimestamp(entry.timestamp)}</span>
						<span class="action-badge {getActionBadgeClass(entry.action)}">{entry.action}</span>
					</div>
					<div class="entry-body">
						<span class="entity-type">{entry.entity_type}</span>
						{#if link}
							<a href={link} class="entity-name">{entry.entity_name}</a>
						{:else}
							<span class="entity-name deleted">{entry.entity_name}</span>
						{/if}
					</div>

					{#if hasChanges && entry.action === 'updated'}
						<button class="toggle-changes" onclick={() => toggleExpanded(entry.id)}>
							{isExpanded ? 'Hide changes' : 'Show changes'}
							<span class="toggle-icon">{isExpanded ? '−' : '+'}</span>
						</button>

						{#if isExpanded && entry.changes}
							<div class="changes-container">
								<DiffView changes={entry.changes} />
							</div>
						{/if}
					{:else if hasChanges && entry.action === 'created' && entry.changes}
						<div class="changes-container">
							<DiffView changes={entry.changes} />
						</div>
					{/if}
				</div>
			{/each}
		</div>

		{#if history.has_more}
			<div class="load-more">
				<button class="btn" onclick={loadMore} disabled={loadingMore}>
					{loadingMore ? 'Loading...' : 'Load more'}
				</button>
			</div>
		{/if}
	{:else}
		<div class="empty">No changes recorded yet.</div>
	{/if}
</div>

<style>
	.change-history {
		width: 100%;
	}

	.filters {
		display: flex;
		gap: 1rem;
		margin-bottom: 1.5rem;
		flex-wrap: wrap;
	}

	.filters label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.filters select {
		padding: 0.5rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		min-width: 150px;
	}

	.filters select:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
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
		gap: 0.75rem;
	}

	.timeline-entry {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
	}

	.entry-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.timestamp {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.action-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-size: 0.75rem;
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

	.entry-body {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.entity-type {
		font-size: 0.75rem;
		color: #94a3b8;
		text-transform: capitalize;
		padding: 0.125rem 0.375rem;
		background: #f1f5f9;
		border-radius: 4px;
	}

	.entity-name {
		font-weight: 500;
		color: #1e293b;
		text-decoration: none;
	}

	a.entity-name:hover {
		color: #3b82f6;
	}

	.entity-name.deleted {
		color: #94a3b8;
		text-decoration: line-through;
	}

	.toggle-changes {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		margin-top: 0.75rem;
		padding: 0.375rem 0.625rem;
		border: none;
		background: #f1f5f9;
		border-radius: 4px;
		font-size: 0.75rem;
		color: #64748b;
		cursor: pointer;
		transition: background 0.15s;
	}

	.toggle-changes:hover {
		background: #e2e8f0;
	}

	.toggle-icon {
		font-weight: 600;
	}

	.changes-container {
		margin-top: 0.75rem;
		padding-top: 0.75rem;
		border-top: 1px solid #e2e8f0;
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
