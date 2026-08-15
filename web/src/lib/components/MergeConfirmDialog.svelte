<script lang="ts" module>
	import type { BranchMergeRefusalCode, MergeConflict, MergeResolution } from '$lib/api/client';

	/**
	 * Mirrors the maxLength on `BranchMergeRequest.note` in openapi.yaml, and is
	 * pinned to it by a spec-drift test in `$lib/api/client.test.ts`. Counted in
	 * UTF-8 bytes, because that is the unit `internal/domain/branch.go` enforces.
	 */
	export const NOTE_MAX_LENGTH = 1000;

	/**
	 * How many left-behind entities the dialog renders before summarising the
	 * rest. Excluding most of a large branch would otherwise put a second copy of
	 * the page's whole change list (itself up to the server's 1000-entity cap)
	 * into the DOM on top of the one still mounted behind the overlay. Confirming
	 * a plan does not require re-reading every row of it; the counts above and
	 * below this list stay exact.
	 */
	export const EXCLUDED_PREVIEW_LIMIT = 20;

	/** One entity named in the plan - enough to say what it is and which one. */
	export interface MergePlanEntity {
		streamId: string;
		entityType: string;
		entityName: string;
	}

	/** A conflict the user has already decided, and the side that won it. */
	export interface MergePlanDecision {
		conflict: MergeConflict;
		resolution: MergeResolution;
	}

	/**
	 * What the merge is about to do, as the page computed it from the comparison.
	 * The page owns this: it holds the resolution state and builds the request.
	 */
	export interface MergePlan {
		/** Entities whose branch changes will be replayed onto the mainline. */
		mergingCount: number;
		/** Entities deliberately left behind - a `main` resolution drops these. */
		excluded: MergePlanEntity[];
		/** Every detected conflict, with the side chosen for it. */
		decisions: MergePlanDecision[];
		/** True when the comparison behind this plan hit the read cap. */
		hasMore: boolean;
	}

	/**
	 * The recovery that is actually valid for a failure. Only `retry` re-issues
	 * the merge - offering it on a failure the API documents as non-retryable
	 * would invite a second request that can only make things worse.
	 */
	export type MergeRecovery = 'close' | 'retry' | 'recompare';

	export interface MergeFailureCopy {
		/** Names the real cause, in the user's terms. */
		title: string;
		/** Paraphrases the endpoint's own explanation - never spec prose verbatim. */
		body: string;
		recovery: MergeRecovery;
		/** The tracking issue for failures the product cannot yet resolve. */
		issue?: { number: number; url: string };
	}

	/**
	 * Every refusal `POST /branches/{id}/merge` documents, side by side, so the
	 * whole verdict vocabulary can be read at once. Keyed by the code the API
	 * client narrows to, which is what keeps this exhaustive as the spec grows.
	 */
	export const MERGE_REFUSAL_COPY: Record<BranchMergeRefusalCode, MergeFailureCopy> = {
		merge_conflicts: {
			title: 'The server found conflicts that still need decisions',
			body:
				"The conflict list from the comparison was only advisory - the server re-runs the check when you merge, and this run found conflicts with no decision. Nothing was written. Close this and work through the conflicts as the server now reports them.",
			recovery: 'close'
		},
		merge_plan_stale: {
			title: 'The mainline moved while this merge was being planned',
			body:
				'Someone changed an entity this merge would replay, after the plan was worked out against it. Replaying over that change without showing it to you would bury it, so nothing was written and the branch is still active. Merging again re-plans against the mainline as it now stands.',
			recovery: 'retry'
		},
		branch_too_large: {
			title: 'This branch is bigger than one merge can scan',
			body:
				'The scan of the branch itself hit its cap, so its list of changes is incomplete and merging would quietly promote only part of it. Nothing was written. Trying again will not help - the cap is fixed and the branch will not shrink. Promoting a branch in pieces needs partial merge, which does not exist yet.',
			recovery: 'close',
			issue: { number: 684, url: 'https://github.com/cacack/my-family/issues/684' }
		},
		main_too_far_ahead: {
			title: 'The mainline has moved too far to list the conflicts completely',
			body:
				'This is about the mainline, not the size of your branch - the branch may be tiny. So much has landed on main for the entities this branch touched that the conflict list cannot be trusted to be complete. Nothing was written. Compare again to see where things stand.',
			recovery: 'recompare'
		},
		branch_not_active: {
			title: 'This branch is no longer active',
			body:
				'It has already been merged or archived, and a terminal branch accepts no further changes. Nothing was written. Compare again to see its current state.',
			recovery: 'recompare'
		},
		merge_already_claimed: {
			title: 'Another merge of this branch got there first',
			body:
				'A concurrent request won the claim on this branch. This request wrote nothing at all. Compare again to see what the merge that won actually did.',
			recovery: 'recompare'
		},
		merge_dangling_reference: {
			title: 'An entity you left behind is still referenced by one being merged',
			body:
				"Excluding it would leave the mainline holding a relationship that points at a person the mainline will not have. Decisions are made per entity, but the branch's events reference each other across entities, so leaving a person out does not leave the links to them out. Nothing was written - close this and revisit what you are excluding.",
			recovery: 'close'
		},
		merge_partially_applied: {
			title: 'The merge was claimed but did not finish',
			body:
				"The branch is already marked merged while the replay onto the mainline stopped partway. Do not merge again: the branch is terminal, so a second attempt would only report that, which is no evidence the work completed. Compare again to see exactly what did and did not land - the server's message below says how far the replay got, and whether the mainline was modified at all.",
			recovery: 'recompare',
			issue: { number: 685, url: 'https://github.com/cacack/my-family/issues/685' }
		},
		invalid_resolution: {
			title: 'One of your decisions is not one this conflict accepts',
			body:
				"The comparison offered a choice the merge's own re-run of the conflict check no longer supports - typically because the mainline deleted an entity the branch was still editing, which leaves keeping the mainline's version as the only outcome that means anything. Nothing was written. Comparing again refetches which sides each conflict accepts and clears the decision that no longer applies.",
			recovery: 'recompare'
		},
		validation_error: {
			title: 'The merge request itself was rejected',
			body:
				'The server refused the request before considering the merge - the note is the usual cause, and its limit is counted in bytes, so accented and non-Latin characters cost more than one each. Nothing was written. The message below is the server\'s own; compare again and retry with the request corrected.',
			recovery: 'recompare'
		}
	};

	/**
	 * The fallback for anything the client does not recognise. Deliberately
	 * `close`: an unknown failure is never assumed to be retryable, because a
	 * retry on the wrong one is the harmful direction.
	 */
	export const GENERIC_MERGE_FAILURE: MergeFailureCopy = {
		title: 'The merge did not go through',
		body:
			'The server refused this merge with something this page does not recognise, so it cannot say whether anything was written. Compare the branch against the mainline before trying again.',
		recovery: 'close'
	};
</script>

<script lang="ts">
	/**
	 * Preview -> confirm -> outcome for merging a research branch into the
	 * mainline.
	 *
	 * This component issues no merge request of its own, and never leaves the
	 * branch itself. The page owns the request, because it also owns the
	 * resolution state a `merge_conflicts` refusal forces it to rebuild; and the
	 * `activeBranch` store's return-to-mainline action reloads the page, which
	 * would destroy the success summary the instant it rendered. Both are
	 * surfaced here as affordances and performed there.
	 */
	import {
		isBranchMergeRefusal,
		type Branch,
		type BranchMergeRefusal,
		type BranchMergeResult
	} from '$lib/api/client';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';

	interface Props {
		open: boolean;
		branch: Branch;
		plan: MergePlan;
		/** True when the branch being merged is the one the user is standing on. */
		isActiveBranch?: boolean;
		/**
		 * Performs the merge. Resolves with the result, throws the refusal.
		 *
		 * The rejection value may be *anything*, not only a `BranchMergeRefusal` -
		 * a transport error, a bare `Promise.reject()`, a string. Whatever is not a
		 * documented refusal is rendered as a generic, non-retryable failure.
		 */
		onconfirm: (note: string) => Promise<BranchMergeResult>;
		onclose: () => void;
		/**
		 * A refusal landed. The page needs this because `merge_conflicts` carries
		 * a fresh `conflicts` array it must re-render its pickers from.
		 */
		onrefused?: (refusal: BranchMergeRefusal) => void;
		/** The user asked for a fresh comparison; the page re-runs it. */
		onrecompare?: () => void;
		/** The user asked to leave the merged branch; the page performs it. */
		onreturntomainline?: () => void;
	}

	let {
		open,
		branch,
		plan,
		isActiveBranch = false,
		onconfirm,
		onclose,
		onrefused,
		onrecompare,
		onreturntomainline
	}: Props = $props();

	let note = $state('');
	let pending = $state(false);
	let refusal: BranchMergeRefusal | null = $state(null);
	/** Set instead of `refusal` when the failure is not a documented refusal. */
	let unrecognized: string | null = $state(null);
	let result: BranchMergeResult | null = $state(null);

	// Both halves measure the trimmed note, because the trimmed note is what
	// gets sent: trailing whitespace must not cost the user a character.
	const trimmedNote = $derived(note.trim());
	// Measured in UTF-8 *bytes*, not JS string length, because bytes are what the
	// server counts (`len(b.MergeNote) > 1000` in `internal/domain/branch.go`).
	// The two only diverge in the direction that hurts: `.length` would admit
	// notes the server rejects, since `é` costs two bytes and a CJK character
	// three, so ~340 CJK characters would read "340/1000" and still be refused.
	const noteBytes = $derived(new TextEncoder().encode(trimmedNote).length);
	const noteValid = $derived(noteBytes <= NOTE_MAX_LENGTH);

	// `$derived.by` rather than `$derived`: read inline, TypeScript still has
	// `refusal` and `unrecognized` narrowed to their `null` initialisers.
	const failure: MergeFailureCopy | null = $derived.by(() => {
		if (refusal) return MERGE_REFUSAL_COPY[refusal.code];
		return unrecognized === null ? null : GENERIC_MERGE_FAILURE;
	});
	const failureMessage = $derived.by(() => refusal?.message ?? unrecognized);

	const previewSummary = $derived(
		`${entityCount(plan.mergingCount)} from this branch will be replayed onto the mainline. ` +
			(plan.excluded.length === 0
				? 'Nothing is being left behind.'
				: `${entityCount(plan.excluded.length)} will be left behind.`)
	);

	// Only the head of the exclusion list is rendered; the counts around it are
	// always the true totals, so nothing about the plan is hidden - only rows.
	const shownExclusions = $derived(plan.excluded.slice(0, EXCLUDED_PREVIEW_LIMIT));
	const hiddenExclusionCount = $derived(plan.excluded.length - shownExclusions.length);

	function entityCount(n: number): string {
		return `${n} ${n === 1 ? 'entity' : 'entities'}`;
	}

	function resolutionLabel(resolution: MergeResolution): string {
		return resolution === 'branch' ? 'This branch wins' : 'The mainline wins';
	}

	function resolutionDetail(resolution: MergeResolution): string {
		return resolution === 'branch'
			? "The branch's version is replayed onto the mainline."
			: "The mainline's version stands; this branch's changes to it are dropped.";
	}

	// A reopened dialog starts fresh - a previous outcome must never be mistaken
	// for this attempt's.
	$effect(() => {
		if (open) {
			note = '';
			refusal = null;
			unrecognized = null;
			result = null;
		}
	});

	async function handleConfirm(event: Event) {
		// Without this the AlertDialog closes on click and the outcome renders
		// somewhere the user is no longer looking.
		event.preventDefault();
		if (pending || !noteValid) return;

		pending = true;
		refusal = null;
		unrecognized = null;
		try {
			result = await onconfirm(trimmedNote);
		} catch (e) {
			if (isBranchMergeRefusal(e)) {
				refusal = e;
				onrefused?.(e);
			} else {
				// Read the message defensively: `isBranchMergeRefusal` answers false for
				// `null` and `undefined` too, and a bare `Promise.reject()` is legal JS
				// (and the default of several mocking helpers). Reaching straight for
				// `.message` on that would throw *inside the error handler*, turning a
				// failure this dialog can explain into an unhandled rejection.
				const message = typeof e === 'object' && e !== null ? (e as { message?: unknown }).message : undefined;
				unrecognized =
					(typeof message === 'string' && message) || 'The merge could not be completed.';
			}
		} finally {
			pending = false;
		}
	}

	function handleRecompare() {
		onrecompare?.();
		onclose();
	}
</script>

<AlertDialog.Root
	{open}
	onOpenChange={(isOpen) => {
		// A request in flight holds the dialog open so its outcome has somewhere
		// to land. Ignoring this callback is not enough on its own - bits-ui
		// closes itself on Escape whatever the parent does with `open`, which is
		// why the content sets `escapeKeydownBehavior` too.
		if (!isOpen && !pending) onclose();
	}}
>
	<AlertDialog.Content
		class="max-h-[85vh] overflow-y-auto sm:max-w-2xl"
		escapeKeydownBehavior={pending ? 'ignore' : 'close'}
		interactOutsideBehavior={pending ? 'ignore' : 'close'}
	>
		<AlertDialog.Header>
			<AlertDialog.Title>
				{#if result}
					Merged {branch.name} into the mainline
				{:else if failure}
					{failure.title}
				{:else}
					Merge {branch.name} into the mainline?
				{/if}
			</AlertDialog.Title>
			<AlertDialog.Description>
				{#if result}
					This branch's research is now part of the mainline. The branch is merged and accepts no
					further changes.
				{:else if failure}
					{failure.body}
				{:else}
					{previewSummary}
				{/if}
			</AlertDialog.Description>
		</AlertDialog.Header>

		{#if result}
			<dl class="summary">
				<div>
					<dt>Events replayed</dt>
					<dd>{result.replayed_event_count}</dd>
				</div>
				<div>
					<dt>Entities left behind</dt>
					<dd>{result.skipped_stream_ids.length}</dd>
				</div>
				<div>
					<dt>Recorded at position</dt>
					<dd>{result.merged_at_position}</dd>
				</div>
			</dl>
			<p class="hint">
				Positions are one global sequence shared by every branch, so this one marks where the merge
				landed in the log - it is not a count of what was promoted.
			</p>
			{#if isActiveBranch}
				<p class="hint">
					You are still standing on this branch. It is merged now, so further research belongs on
					the mainline.
				</p>
			{/if}
		{:else if failure}
			<div class="failure" role="alert">
				{#if failureMessage}
					<p class="server-message">{failureMessage}</p>
				{/if}
				{#if failure.issue}
					<p class="issue-link">
						Tracked as
						<a href={failure.issue.url} target="_blank" rel="noopener noreferrer">
							#{failure.issue.number}
						</a>.
					</p>
				{/if}
			</div>
		{:else}
			{#if plan.hasMore}
				<div class="truncation" role="note">
					This review is based on a <strong>partial</strong> comparison - one side hit the read cap,
					so there are changes it never showed you. You are deciding with incomplete information.
				</div>
			{/if}

			{#if plan.decisions.length > 0}
				<section class="plan-section">
					<h3>Conflicts you decided</h3>
					<ul class="plan-list">
						{#each plan.decisions as decision (decision.conflict.stream_id)}
							<li>
								<div class="entity-head">
									<span class="entity-type">{decision.conflict.entity_type}</span>
									<span class="entity-name">{decision.conflict.entity_name || 'Unnamed entity'}</span>
									<Badge variant={decision.resolution === 'branch' ? 'default' : 'secondary'}>
										{resolutionLabel(decision.resolution)}
									</Badge>
								</div>
								<p class="entity-detail">{resolutionDetail(decision.resolution)}</p>
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if plan.excluded.length > 0}
				<section class="plan-section">
					<h3>Left behind ({plan.excluded.length})</h3>
					<p class="section-hint">
						These are not being promoted. The mainline keeps whatever it has, and this branch's
						changes to them are dropped.
					</p>
					<ul class="plan-list">
						{#each shownExclusions as entity (entity.streamId)}
							<li>
								<div class="entity-head">
									<span class="entity-type">{entity.entityType}</span>
									<span class="entity-name">{entity.entityName || 'Unnamed entity'}</span>
									<Badge variant="outline">Not merging</Badge>
								</div>
							</li>
						{/each}
					</ul>
					{#if hiddenExclusionCount > 0}
						<p class="section-hint more">
							+{hiddenExclusionCount} more not listed here. All {plan.excluded.length} are left behind.
						</p>
					{/if}
				</section>
			{/if}

			<div class="field">
				<Label for="merge-note">Note (optional)</Label>
				<Textarea
					id="merge-note"
					bind:value={note}
					rows={3}
					disabled={pending}
					aria-invalid={!noteValid}
					placeholder="Why this research is being promoted"
				/>
				<span class="field-hint" class:over={!noteValid}>
					{noteBytes}/{NOTE_MAX_LENGTH}
				</span>
				{#if !noteValid}
					<p class="field-error" role="alert">
						The note is {noteBytes - NOTE_MAX_LENGTH} bytes over the limit. Accented and non-Latin
						characters cost more than one byte each.
					</p>
				{/if}
			</div>
		{/if}

		<AlertDialog.Footer>
			{#if result}
				{#if isActiveBranch}
					<Button variant="outline" onclick={() => onreturntomainline?.()}>
						Return to mainline
					</Button>
				{/if}
				<AlertDialog.Cancel>Done</AlertDialog.Cancel>
			{:else if failure}
				<AlertDialog.Cancel>Close</AlertDialog.Cancel>
				{#if failure.recovery === 'retry'}
					<AlertDialog.Action disabled={pending} onclick={handleConfirm}>
						Try merging again
					</AlertDialog.Action>
				{:else if failure.recovery === 'recompare'}
					<AlertDialog.Action onclick={handleRecompare}>Compare again</AlertDialog.Action>
				{/if}
			{:else}
				<!--
					`disabled` only stops bits-ui's own close handler; it never reaches
					the button element, so `aria-disabled` and the matching Tailwind
					variants are what actually say so to a user or a screen reader.
				-->
				<AlertDialog.Cancel
					disabled={pending}
					aria-disabled={pending}
					class="aria-disabled:pointer-events-none aria-disabled:opacity-50"
				>
					Cancel
				</AlertDialog.Cancel>
				<AlertDialog.Action disabled={pending || !noteValid} onclick={handleConfirm}>
					{pending ? 'Merging...' : 'Merge branch'}
				</AlertDialog.Action>
			{/if}
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<style>
	.truncation {
		padding: 0.75rem;
		background: #fef3c7;
		border: 1px solid #fcd34d;
		border-radius: 6px;
		color: #92400e;
		font-size: 0.8125rem;
	}

	.plan-section h3 {
		margin: 0 0 0.375rem;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: #64748b;
	}

	.section-hint {
		margin: 0 0 0.5rem;
		font-size: 0.8125rem;
		color: #64748b;
	}

	/* Sits under the list rather than above it. */
	.section-hint.more {
		margin: 0.5rem 0 0;
	}

	.plan-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.plan-list li {
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		padding: 0.625rem 0.75rem;
	}

	.entity-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.entity-type {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #94a3b8;
	}

	.entity-name {
		font-size: 0.875rem;
		font-weight: 600;
		color: #1e293b;
		overflow-wrap: anywhere;
	}

	.entity-detail {
		margin: 0.375rem 0 0;
		font-size: 0.8125rem;
		color: #475569;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.field-hint {
		align-self: flex-end;
		font-size: 0.6875rem;
		color: #94a3b8;
	}

	.field-hint.over {
		color: #dc2626;
	}

	.field-error {
		margin: 0;
		font-size: 0.8125rem;
		color: #dc2626;
	}

	.failure {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
	}

	.server-message {
		margin: 0;
		font-size: 0.8125rem;
		color: #991b1b;
		overflow-wrap: anywhere;
	}

	.issue-link {
		margin: 0;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.issue-link a {
		color: #3b82f6;
	}

	.summary {
		display: flex;
		gap: 1.5rem;
		flex-wrap: wrap;
		margin: 0;
	}

	.summary div {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}

	.summary dt {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #94a3b8;
	}

	.summary dd {
		margin: 0;
		font-size: 1rem;
		font-weight: 600;
		color: #1e293b;
	}

	.hint {
		margin: 0;
		font-size: 0.8125rem;
		color: #64748b;
	}
</style>
