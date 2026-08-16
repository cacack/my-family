<script lang="ts">
	/**
	 * Compare a research branch against the mainline, and merge it.
	 *
	 * The single `compareBranch` call behind this page returns the diff and the
	 * conflict verdict together, so the merge review lives inline here rather
	 * than on a route of its own. What the comparison means:
	 *
	 * - `overlapping_stream_ids` is a *hint*: entities both sides touched. An
	 *   entity can overlap without conflicting.
	 * - `conflicts` is the compare-time *verdict*: the changes that are actually
	 *   incompatible. It is **advisory only** - `POST /branches/{id}/merge`
	 *   re-runs detection itself and ignores whatever compare said, so a merge
	 *   can come back with a different list (see `handleRefused`).
	 * - `has_more` means one side hit the read cap, so what is rendered below is
	 *   only part of the diff.
	 *
	 * Merge affordances appear only while the branch is `active`. A `merged` or
	 * `archived` branch accepts no further writes, so it keeps the read-only
	 * conflict rendering below.
	 */
	import { page } from '$app/stores';
	import {
		api,
		type ApiError,
		type BranchChangeEntry,
		type BranchComparisonResult,
		type BranchMergeRefusal,
		type BranchMergeResult,
		type MergeConflict,
		type MergeResolution
	} from '$lib/api/client';
	import { activeBranch, returnToMainline, switchBranch } from '$lib/stores/activeBranch.svelte';
	import DiffView from '$lib/components/DiffView.svelte';
	import MergeConflictResolver from '$lib/components/MergeConflictResolver.svelte';
	import MergeConfirmDialog, {
		type MergePlan,
		type MergePlanEntity
	} from '$lib/components/MergeConfirmDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';

	let comparison: BranchComparisonResult | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let notFound = $state(false);

	/**
	 * The conflict list a `409 merge_conflicts` refusal carried, which supersedes
	 * the comparison's advisory one. Null until the server has re-verdicted.
	 */
	let serverConflicts: MergeConflict[] | null = $state(null);
	/**
	 * The decision taken for each conflicting entity, keyed by `stream_id`.
	 *
	 * Reassigned, never mutated: a plain `Map` is not a `SvelteMap`, so an
	 * in-place `.set()` would leave every reader of this state unrepainted.
	 */
	let resolutions: Map<string, MergeResolution> = $state(new Map());
	/**
	 * Entities the user opted out of the merge, by `stream_id`. Excluding is
	 * keyed by *entity*, not by change entry: one entity can appear in several
	 * entries and all of them must show and toggle the same decision.
	 */
	let excluded: Set<string> = $state(new Set());
	let merging = $state(false);
	let confirmOpen = $state(false);

	// `?? ''` so the id is a plain string everywhere below; the `$effect` already
	// treats an absent id as "nothing to load", and empty is absent.
	const branchId = $derived($page.params.id ?? '');
	const conflicts: MergeConflict[] = $derived.by(
		() => serverConflicts ?? comparison?.conflicts ?? []
	);
	const conflictedStreamIds = $derived.by(() => new Set(conflicts.map((c) => c.stream_id)));
	/** Overlaps the conflict detector cleared - the interesting part of the hint. */
	const cleanOverlaps = $derived.by(() =>
		(comparison?.overlapping_stream_ids ?? []).filter((id) => !conflictedStreamIds.has(id))
	);

	/**
	 * Only an active branch can be merged; terminal ones are a read-only record.
	 *
	 * `$derived.by` rather than `$derived`: read inline, TypeScript narrows
	 * `comparison` back to its `null` initialiser and `.branch` collapses.
	 */
	const mergeable = $derived.by(() => comparison?.branch.status === 'active');

	/**
	 * Every entity this branch changed, first entry wins for the name. The merge
	 * request may only name entities from here (plus the server's own conflicts):
	 * an id the branch never touched is a `400`, not a no-op.
	 */
	const branchEntities: Map<string, MergePlanEntity> = $derived.by(() => {
		const seen = new Map<string, MergePlanEntity>();
		for (const entry of comparison?.branch_changes ?? []) {
			if (!seen.has(entry.entity_id)) {
				seen.set(entry.entity_id, {
					streamId: entry.entity_id,
					entityType: entry.entity_type,
					entityName: entry.entity_name ?? ''
				});
			}
		}
		return seen;
	});

	/**
	 * Exactly what goes on the wire: conflict decisions and exclusions folded
	 * into one entry per `stream_id`.
	 *
	 * Exclusion is applied last and wins outright, because "leave this entity
	 * behind" *is* a `main` resolution - the only partial-merge shape the API
	 * supports (ADR-005, Merge). Folding them into one map is what stops a
	 * conflicted-and-excluded entity being sent twice, which would be the client
	 * contradicting itself.
	 */
	const mergeResolutions: Map<string, MergeResolution> = $derived.by(() => {
		const folded = new Map<string, MergeResolution>();
		for (const conflict of conflicts) {
			const decision = resolutions.get(conflict.stream_id);
			if (decision) folded.set(conflict.stream_id, decision);
		}
		for (const streamId of excluded) folded.set(streamId, 'main');
		return folded;
	});

	/**
	 * Counted against `mergeResolutions`, not the raw `resolutions` map, because
	 * that is what actually goes on the wire. Ticking "leave out of the merge" on
	 * a conflicted entity *is* a decision the server will honour - reading the raw
	 * map would call it undecided and leave "Review & merge" disabled with no way
	 * out but the radio the user has already made moot.
	 */
	const undecidedCount = $derived(
		conflicts.filter((conflict) => !mergeResolutions.has(conflict.stream_id)).length
	);
	const undecidedLabel = $derived.by(() => {
		if (conflicts.length === 0) return 'No conflicts to resolve.';
		const total = `${conflicts.length} conflict${conflicts.length === 1 ? '' : 's'}`;
		return undecidedCount === 0
			? `All ${total} decided.`
			: `${undecidedCount} of ${total} still undecided.`;
	});

	const mergePlan: MergePlan = $derived.by(() => {
		const leftBehind = [...mergeResolutions]
			.filter(([, resolution]) => resolution === 'main')
			.map(([streamId]) => planEntity(streamId));
		// Only entities this comparison actually listed can be counted, so a
		// truncated comparison undercounts - which is precisely what `hasMore`
		// discloses in the dialog.
		const listedExclusions = leftBehind.filter((e) => branchEntities.has(e.streamId)).length;
		return {
			mergingCount: branchEntities.size - listedExclusions,
			excluded: leftBehind,
			decisions: conflicts.flatMap((conflict) => {
				const resolution = mergeResolutions.get(conflict.stream_id);
				return resolution ? [{ conflict, resolution }] : [];
			}),
			hasMore: comparison?.has_more ?? false
		};
	});

	/** Names an entity for the plan, falling back to the conflict's own labels. */
	function planEntity(streamId: string): MergePlanEntity {
		const known = branchEntities.get(streamId);
		if (known) return known;
		const conflict = conflicts.find((c) => c.stream_id === streamId);
		return {
			streamId,
			entityType: conflict?.entity_type ?? 'entity',
			entityName: conflict?.entity_name ?? ''
		};
	}

	function resolveConflict(streamId: string, resolution: MergeResolution) {
		resolutions = new Map(resolutions).set(streamId, resolution);
	}

	function toggleExclusion(streamId: string) {
		const next = new Set(excluded);
		if (!next.delete(streamId)) next.add(streamId);
		excluded = next;
	}

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
	// Comparing the routed id is not enough on its own: A -> B -> A navigation
	// issues two requests for the SAME id, and the first can land after the
	// second and overwrite newer data with older. A monotonic token is what
	// actually orders them, so every state write is gated on it — the same
	// pattern BranchSwitcher uses for its list fetches.
	let comparisonRequest = 0;

	async function loadComparison(id: string) {
		const request = ++comparisonRequest;
		loading = true;
		error = null;
		notFound = false;
		// Decisions belong to the comparison they were made against. Clearing them
		// here, under the same token that gates every other write, is what stops a
		// late response from repopulating one branch's conflicts while another
		// branch's decisions are still held.
		serverConflicts = null;
		resolutions = new Map();
		excluded = new Set();
		confirmOpen = false;
		// `merging` is per-comparison too: it disables this page's resolver,
		// exclusion checkboxes and merge button, and a merge issued for the branch
		// we just navigated away from must not disable the new one's. It is cleared
		// here *as well as* in `performMerge`'s `finally` - not moved. The `finally`
		// stays ungated on the token on purpose: gating it would leave `merging`
		// stuck true forever whenever a navigation raced the merge, which is worse
		// than the bug being fixed.
		merging = false;
		try {
			const result = await api.compareBranch(id);
			if (request !== comparisonRequest) return;
			comparison = result;
		} catch (e) {
			if (request !== comparisonRequest) return;
			const apiError = e as ApiError;
			if (apiError.status === 404) {
				notFound = true;
			} else {
				error = apiError.message || 'Failed to load branch comparison';
			}
			comparison = null;
		} finally {
			if (request === comparisonRequest) {
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

	/**
	 * The comparison token the in-flight merge was issued under, so an outcome
	 * that lands after a soft navigation cannot rewrite the new branch's state.
	 */
	let mergeComparisonRequest = 0;

	/**
	 * Issues the merge. Resolves with the result and *throws* the refusal, which
	 * is the contract `MergeConfirmDialog` renders its outcome from.
	 */
	async function performMerge(note: string): Promise<BranchMergeResult> {
		const request = (mergeComparisonRequest = comparisonRequest);
		const id = branchId;
		merging = true;
		try {
			const result = await api.mergeBranch(id, {
				...(note ? { note } : {}),
				resolutions: [...mergeResolutions].map(([stream_id, resolution]) => ({
					stream_id,
					resolution
				}))
			});
			// Adopt the branch as the merge left it, so the page stops offering
			// merge affordances behind the still-open success summary. Gated on the
			// token like every other write: the user may have navigated away while
			// the merge was in flight.
			if (request === comparisonRequest && comparison) {
				comparison = { ...comparison, branch: result.branch };
			}
			return result;
		} finally {
			merging = false;
		}
	}

	/**
	 * The merge endpoint's verdict overrides the comparison's. On
	 * `merge_conflicts` it carries the *whole* conflict list as the server now
	 * sees it, so the pickers are rebuilt from that and every decision the server
	 * no longer reports a conflict for is dropped - carrying one forward would
	 * silently re-send a choice made about a conflict that no longer exists.
	 */
	function handleRefused(refusal: BranchMergeRefusal) {
		if (mergeComparisonRequest !== comparisonRequest) return;
		if (refusal.code !== 'merge_conflicts') return;
		const fresh = refusal.conflicts ?? [];
		serverConflicts = fresh;
		const live = new Set(fresh.map((conflict) => conflict.stream_id));
		resolutions = new Map([...resolutions].filter(([streamId]) => live.has(streamId)));
	}
</script>

<svelte:head>
	<title>{comparison ? `${comparison.branch.name} | Branches` : 'Branch Comparison'} | My Family</title>
</svelte:head>

{#snippet changeList(entries: BranchChangeEntry[], emptyText: string, excludable: boolean)}
	{#if entries.length === 0}
		<p class="side-empty">{emptyText}</p>
	{:else}
		<ol class="change-list">
			{#each entries as entry (entry.id)}
				{@const link = entityLink(entry)}
				<!-- Branch side only: the mainline's own changes are never "being merged",
				     so marking them left behind would be meaningless. -->
				{@const isExcluded = excludable && excluded.has(entry.entity_id)}
				<li
					class="change-entry"
					class:contested={conflictedStreamIds.has(entry.entity_id)}
					class:excluded={isExcluded}
				>
					<div class="change-head">
						<span class="change-time">{formatTimestamp(entry.timestamp)}</span>
						<Badge
							variant={entry.action === 'deleted' ? 'destructive' : 'secondary'}
							class="capitalize"
						>
							{entry.action}
						</Badge>
						{#if isExcluded}
							<!-- Words, not just the dashed border: the state must not depend on sight. -->
							<Badge variant="outline">Not merging</Badge>
						{/if}
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
					{#if excludable && mergeable}
						<!--
							Keyed by `entity_id`, so an entity changed several times on this
							branch toggles and reads back as one decision across all of its
							entries. The visible text opens the accessible name so the two
							cannot disagree (WCAG 2.5.3).
						-->
						<div class="exclude-row">
							<Checkbox
								checked={isExcluded}
								onCheckedChange={() => toggleExclusion(entry.entity_id)}
								disabled={merging}
								aria-label="Leave out of the merge: {entry.entity_name || 'unnamed entity'}"
							/>
							<span class="exclude-text">Leave out of the merge</span>
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
			<div class="header-actions">
				{#if mergeable && activeBranch.id !== comparison.branch.id}
					<Button variant="outline" onclick={() => switchBranch(comparison?.branch ?? null)}>
						Switch to branch
					</Button>
				{/if}
				{#if mergeable}
					<div class="merge-bar">
						<p class="undecided" role="status">{undecidedLabel}</p>
						<Button onclick={() => (confirmOpen = true)} disabled={undecidedCount > 0 || merging}>
							Review &amp; merge
						</Button>
					</div>
				{/if}
			</div>
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
				{#if mergeable}
					Entities whose branch and mainline changes are actually incompatible. Every one needs a
					decision before this branch can be merged.
				{:else}
					Entities whose branch and mainline changes were incompatible. This branch is
					{comparison.branch.status} and accepts no further changes, so this is a record rather
					than a decision.
				{/if}
			</p>
			{#if conflicts.length === 0}
				<p class="clean">No conflicts. This branch's changes are compatible with the mainline.</p>
			{:else if mergeable}
				<!--
					Shown from `mergeResolutions`, not `resolutions`, so the picker always
					reads back what will actually be sent: an excluded entity displays the
					`main` its exclusion folds on top, rather than the branch's-version
					choice the payload overrides. Nothing is lost by this - the fold is
					one-way and `resolutions` still holds the original decision, so
					unticking the exclusion restores it. `onresolve` writes to
					`resolutions` (never the derived map), which is what keeps that true.
				-->
				<MergeConflictResolver
					{conflicts}
					resolutions={mergeResolutions}
					onresolve={resolveConflict}
					disabled={merging}
				/>
			{:else}
				<!--
					A terminal branch gets the read-only list, not a disabled resolver: it
					can never take another write, so offering pickers at all - even inert
					ones - would suggest a decision is still outstanding.
				-->
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
			<!-- The two sides render the same entities in the same shape, so only the
			     container tells them apart. The E2E suite needs a handle that survives
			     a CSS rename to assert which side a change landed on. -->
			<div class="side" data-testid="branch-changes">
				<h2>On this branch</h2>
				<p class="side-count">{comparison.branch_change_count} change{comparison.branch_change_count === 1 ? '' : 's'} since the fork</p>
				{@render changeList(comparison.branch_changes, 'No changes on this branch yet.', true)}
			</div>
			<div class="side" data-testid="main-changes">
				<h2>On the mainline</h2>
				<p class="side-count">
					{comparison.main_change_count} change{comparison.main_change_count === 1 ? '' : 's'} to the
					same entities since the fork
				</p>
				{@render changeList(
					comparison.main_changes,
					'The mainline has not touched any of the entities this branch changed.',
					false
				)}
			</div>
		</section>

		<!--
			The dialog owns preview -> confirm -> outcome, but issues nothing and
			navigates nowhere. Both belong here: this page holds the resolution state
			a `merge_conflicts` refusal forces it to rebuild, and `returnToMainline()`
			reloads the page - calling it automatically on success would destroy the
			summary the instant it rendered, so the user has to ask for it.
		-->
		<MergeConfirmDialog
			open={confirmOpen}
			branch={comparison.branch}
			plan={mergePlan}
			isActiveBranch={activeBranch.id === comparison.branch.id}
			onconfirm={performMerge}
			onclose={() => (confirmOpen = false)}
			onrefused={handleRefused}
			onrecompare={() => loadComparison(branchId)}
			onreturntomainline={returnToMainline}
		/>
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

	/* Wraps under the heading at narrow widths rather than squeezing beside it. */
	.header-actions {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.merge-bar {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.undecided {
		margin: 0;
		font-size: 0.8125rem;
		color: #64748b;
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

	/* Dashed and dimmed, alongside the "Not merging" badge - shape and words, so
	   the exclusion never rests on colour alone. */
	.change-entry.excluded {
		border-style: dashed;
		border-color: #94a3b8;
		background: #f8fafc;
	}

	.exclude-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.625rem;
		padding-top: 0.625rem;
		border-top: 1px solid #e2e8f0;
	}

	.exclude-text {
		font-size: 0.8125rem;
		color: #475569;
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
