<script lang="ts">
	/**
	 * Read-only comparison of a research branch against the mainline.
	 *
	 * Deliberately has no merge action and no resolution pickers - merging with
	 * review is #95. This page's job is to show the divergence honestly:
	 *
	 * - `overlapping_stream_ids` is a *hint*: entities both sides touched. An
	 *   entity can overlap without conflicting.
	 * - `conflicts` is the *verdict*: the changes that are actually incompatible.
	 * - `has_more` means one side hit the read cap, so what is rendered below is
	 *   only part of the diff.
	 */
	import { page } from '$app/stores';
	import {
		api,
		type ApiError,
		type BranchChangeEntry,
		type BranchComparisonResult,
		type MergeConflict
	} from '$lib/api/client';
	import { activeBranch, switchBranch } from '$lib/stores/activeBranch.svelte';
	import DiffView from '$lib/components/DiffView.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';

	let comparison: BranchComparisonResult | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let notFound = $state(false);

	const branchId = $derived($page.params.id);
	const conflicts: MergeConflict[] = $derived.by(() => comparison?.conflicts ?? []);
	const conflictedStreamIds = $derived.by(() => new Set(conflicts.map((c) => c.stream_id)));
	/** Overlaps the conflict detector cleared - the interesting part of the hint. */
	const cleanOverlaps = $derived.by(() =>
		(comparison?.overlapping_stream_ids ?? []).filter((id) => !conflictedStreamIds.has(id))
	);

	function formatTimestamp(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	function entityLink(entry: BranchChangeEntry): string | null {
		if (entry.action === 'deleted') return null;
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

	function conflictLabel(kind: MergeConflict['kind']): string {
		switch (kind) {
			case 'edit_edit':
				return 'Both sides edited';
			case 'delete_edit':
				return 'Deleted on one side';
			case 'create_create':
				return 'Created on both sides';
			default:
				return kind;
		}
	}

	/**
	 * A soft navigation between two `/branches/{id}` entries reuses this
	 * component rather than recreating it, so a slow first response can land
	 * after the second branch's. Every assignment below is therefore gated on
	 * the requested id still being the routed one — otherwise the page would
	 * show one branch's changes and conflicts under another's name.
	 */
	async function loadComparison(id: string) {
		loading = true;
		error = null;
		notFound = false;
		try {
			const result = await api.compareBranch(id);
			if (id !== branchId) return;
			comparison = result;
		} catch (e) {
			if (id !== branchId) return;
			const apiError = e as ApiError;
			if (apiError.status === 404) {
				notFound = true;
			} else {
				error = apiError.message || 'Failed to load branch comparison';
			}
			comparison = null;
		} finally {
			if (id === branchId) {
				loading = false;
			}
		}
	}

	$effect(() => {
		const id = branchId;
		if (id) {
			loadComparison(id);
		}
	});
</script>

<svelte:head>
	<title>{comparison ? `${comparison.branch.name} | Branches` : 'Branch Comparison'} | My Family</title>
</svelte:head>

{#snippet changeList(entries: BranchChangeEntry[], emptyText: string)}
	{#if entries.length === 0}
		<p class="side-empty">{emptyText}</p>
	{:else}
		<ol class="change-list">
			{#each entries as entry (entry.id)}
				{@const link = entityLink(entry)}
				<li class="change-entry" class:contested={conflictedStreamIds.has(entry.entity_id)}>
					<div class="change-head">
						<span class="change-time">{formatTimestamp(entry.timestamp)}</span>
						<Badge
							variant={entry.action === 'deleted' ? 'destructive' : 'secondary'}
							class="capitalize"
						>
							{entry.action}
						</Badge>
					</div>
					<div class="change-body">
						<span class="entity-type">{entry.entity_type}</span>
						{#if link}
							<a href={link} class="entity-name">{entry.entity_name || 'Unnamed'}</a>
						{:else}
							<span class="entity-name deleted">{entry.entity_name || 'Unnamed'}</span>
						{/if}
					</div>
					{#if entry.changes && Object.keys(entry.changes).length > 0}
						<div class="change-diff">
							<DiffView changes={entry.changes} />
						</div>
					{/if}
				</li>
			{/each}
		</ol>
	{/if}
{/snippet}

<div class="compare-page">
	<a href="/branches" class="back-link">&larr; All branches</a>

	{#if loading}
		<div class="state" role="status" aria-live="polite">Loading comparison...</div>
	{:else if notFound}
		<div class="state empty">
			<h2>Branch not found</h2>
			<p>It may have been deleted, or the branch registry is not configured on this server.</p>
		</div>
	{:else if error}
		<div class="state error" role="alert">{error}</div>
	{:else if comparison}
		<header class="page-header">
			<div>
				<div class="title-row">
					<h1>{comparison.branch.name}</h1>
					<Badge variant={comparison.branch.status === 'active' ? 'default' : 'secondary'} class="capitalize">
						{comparison.branch.status}
					</Badge>
				</div>
				{#if comparison.branch.description}
					<p class="description">{comparison.branch.description}</p>
				{/if}
				<p class="anchor">Compared against the mainline from position {comparison.base_position}.</p>
			</div>
			{#if comparison.branch.status === 'active' && activeBranch.id !== comparison.branch.id}
				<Button variant="outline" onclick={() => switchBranch(comparison?.branch ?? null)}>
					Switch to branch
				</Button>
			{/if}
		</header>

		{#if comparison.has_more}
			<div class="truncation" role="note">
				One or both sides hit the read cap, so this comparison is <strong>partial</strong>. More
				changes exist than are shown below.
			</div>
		{/if}

		<section class="verdict">
			<h2>Conflicts</h2>
			<p class="section-hint">
				Entities whose branch and mainline changes are actually incompatible. Resolving them is
				part of merging, which is not available here yet.
			</p>
			{#if conflicts.length === 0}
				<p class="clean">No conflicts. This branch's changes are compatible with the mainline.</p>
			{:else}
				<ul class="conflict-list">
					{#each conflicts as conflict (conflict.stream_id)}
						<li class="conflict">
							<div class="conflict-head">
								<span class="entity-type">{conflict.entity_type}</span>
								<span class="conflict-name">{conflict.entity_name || 'Unnamed entity'}</span>
								<Badge variant="destructive">{conflictLabel(conflict.kind)}</Badge>
							</div>
							<p class="conflict-detail">{conflict.detail}</p>
							{#if conflict.fields && conflict.fields.length > 0}
								<p class="conflict-fields">
									Contested fields: {conflict.fields.join(', ')}
								</p>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="hint">
			<h2>Also changed on both sides</h2>
			<p class="section-hint">
				A divergence hint for human review, not a verdict - these entities were touched by both
				the branch and the mainline but their changes do not conflict.
			</p>
			{#if cleanOverlaps.length === 0}
				<p class="clean">
					{comparison.overlapping_stream_ids.length === 0
						? 'No entities were changed on both sides.'
						: 'Every entity changed on both sides is listed as a conflict above.'}
				</p>
			{:else}
				<ul class="overlap-list">
					{#each cleanOverlaps as streamId (streamId)}
						<li><code>{streamId}</code></li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="changes">
			<div class="side">
				<h2>On this branch</h2>
				<p class="side-count">{comparison.branch_change_count} change{comparison.branch_change_count === 1 ? '' : 's'} since the fork</p>
				{@render changeList(comparison.branch_changes, 'No changes on this branch yet.')}
			</div>
			<div class="side">
				<h2>On the mainline</h2>
				<p class="side-count">
					{comparison.main_change_count} change{comparison.main_change_count === 1 ? '' : 's'} to the
					same entities since the fork
				</p>
				{@render changeList(
					comparison.main_changes,
					'The mainline has not touched any of the entities this branch changed.'
				)}
			</div>
		</section>
	{/if}
</div>

<style>
	.compare-page {
		max-width: 1100px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.back-link {
		display: inline-block;
		margin-bottom: 1rem;
		font-size: 0.875rem;
		color: #64748b;
		text-decoration: none;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.page-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1.5rem;
	}

	.title-row {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		flex-wrap: wrap;
	}

	.title-row h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.description {
		margin: 0.375rem 0 0;
		font-size: 0.875rem;
		color: #475569;
	}

	.anchor {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #94a3b8;
	}

	.truncation {
		margin-bottom: 1.5rem;
		padding: 0.75rem 1rem;
		background: #fef3c7;
		border: 1px solid #f59e0b;
		border-radius: 6px;
		font-size: 0.8125rem;
		color: #92400e;
	}

	section {
		margin-bottom: 2rem;
	}

	section h2 {
		margin: 0 0 0.25rem;
		font-size: 1rem;
		color: #1e293b;
	}

	.section-hint {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		color: #64748b;
		max-width: 52rem;
	}

	.clean {
		margin: 0;
		padding: 0.75rem 1rem;
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		border-radius: 6px;
		font-size: 0.875rem;
		color: #166534;
	}

	.conflict-list,
	.overlap-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.conflict {
		padding: 0.75rem 1rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
	}

	.conflict-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.conflict-name {
		font-weight: 600;
		color: #1e293b;
	}

	.conflict-detail {
		margin: 0.375rem 0 0;
		font-size: 0.875rem;
		color: #7f1d1d;
	}

	.conflict-fields {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #b91c1c;
	}

	.overlap-list li {
		font-size: 0.8125rem;
		color: #475569;
	}

	.overlap-list code {
		font-size: 0.75rem;
		background: #f1f5f9;
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
	}

	/* Two sides at desktop width; stacked on narrow screens so the diff never
	   forces horizontal scrolling. */
	.changes {
		display: grid;
		grid-template-columns: 1fr;
		gap: 1.5rem;
	}

	@media (min-width: 768px) {
		.changes {
			grid-template-columns: 1fr 1fr;
		}
	}

	.side-count {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.side-empty {
		margin: 0;
		font-size: 0.875rem;
		color: #94a3b8;
	}

	.change-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.change-entry {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.875rem;
	}

	.change-entry.contested {
		border-color: #fca5a5;
	}

	.change-head {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		margin-bottom: 0.375rem;
		flex-wrap: wrap;
	}

	.change-time {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.change-body {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
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

	.change-diff {
		margin-top: 0.625rem;
		padding-top: 0.625rem;
		border-top: 1px solid #e2e8f0;
	}

	.state {
		padding: 2rem;
		text-align: center;
		color: #64748b;
	}

	.state.error {
		color: #dc2626;
	}

	.state.empty {
		background: white;
		border: 1px dashed #cbd5e1;
		border-radius: 8px;
	}

	.state.empty h2 {
		margin: 0 0 0.375rem;
		font-size: 1rem;
		color: #1e293b;
	}
</style>
