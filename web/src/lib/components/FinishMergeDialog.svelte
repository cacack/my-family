<script lang="ts" module>
	import type { BranchMergeResumeRefusalCode } from '$lib/api/client';

	/** How many ready entities the dialog lists before summarising the rest. */
	export const READY_PREVIEW_LIMIT = 20;

	/**
	 * What to offer after a refusal that ends the attempt. `retry` re-issues the
	 * resume, which is always safe (it replays only what is missing); `refresh`
	 * asks the page for the branch as it now stands.
	 */
	export type FinishMergeRecovery = 'close' | 'retry' | 'refresh';

	export interface FinishMergeFailureCopy {
		title: string;
		body: string;
		recovery: FinishMergeRecovery;
	}

	/**
	 * The refusals that end an attempt, in the user's words. The three that
	 * leave the user deciding (`merge_resume_needs_resolution`,
	 * `merge_dangling_reference`, `validation_error`) are not here: those keep
	 * the decisions on screen with a notice instead.
	 */
	export const FINISH_MERGE_FAILURE_COPY: Partial<Record<BranchMergeResumeRefusalCode, FinishMergeFailureCopy>> = {
		merge_partially_applied: {
			title: 'Finishing the merge stopped partway again',
			body:
				'Whatever reached the mainline in this attempt stays there. Trying again is safe: it replays only what is still missing and never repeats what already landed.',
			recovery: 'retry'
		},
		merge_resume_concurrent: {
			title: 'Another attempt to finish this merge got there first',
			body:
				'It recorded its decisions before this one could, so nothing was replayed here. Refresh to see what is still left.',
			recovery: 'refresh'
		},
		invalid_resolution: {
			title: 'One of your decisions no longer applies',
			body:
				'The mainline changed an entity again while you were deciding, so the choice you made is not one it accepts any more. Nothing was written. Refresh to decide against the mainline as it now stands.',
			recovery: 'refresh'
		},
		merge_not_claimed: {
			title: 'This branch has no merge to finish',
			body: 'The branch was never merged, or it was deleted. Nothing was written. Refresh to see its current state.',
			recovery: 'refresh'
		},
		branch_too_large: {
			title: 'This branch is bigger than one merge can scan',
			body:
				'The branch has more changes than a merge can read in one go, so finishing it from here would replay only part of it. Nothing was written.',
			recovery: 'close'
		}
	};

	/** Anything the dialog does not recognise: never assumed retryable. */
	export const GENERIC_FINISH_FAILURE: FinishMergeFailureCopy = {
		title: 'The merge could not be finished',
		body:
			'The server refused with something this page does not recognise, so it cannot say whether anything was written. Refresh the branch before trying again.',
		recovery: 'refresh'
	};
</script>

<script lang="ts">
	/**
	 * Finishes a merge whose replay onto the mainline stopped partway (#830).
	 *
	 * Shows what is left: the entities the merge will replay as planned, and
	 * the ones the mainline changed or removed since, which need a decision -
	 * made in the same `MergeConflictResolver` the merge review uses, with the
	 * reason each one needs it. The page owns the request (`onresume`), exactly
	 * as it owns the merge for `MergeConfirmDialog`.
	 */
	import { untrack } from 'svelte';
	import {
		isBranchMergeResumeRefusal,
		type Branch,
		type BranchMergeResumeRefusal,
		type BranchMergeResumeRequest,
		type BranchMergeResumeResult,
		type MergeBlocker,
		type MergePendingEntity,
		type MergeResolution
	} from '$lib/api/client';
	import MergeConflictResolver from '$lib/components/MergeConflictResolver.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { entityTypeLabel, unnamedEntityLabel } from '$lib/utils/changeEntries';
	import { blockedEntityIds, describeBlocker } from '$lib/utils/mergeBlockers';
	import {
		incompleteMergeSummary,
		isUnreadableMerge,
		pendingAsResolvable,
		pendingEntities,
		pendingReasonLabel,
		pendingSoleOptionReason
	} from '$lib/utils/mergeState';

	interface Props {
		open: boolean;
		/** The merged branch, carrying `merge_pending`. */
		branch: Branch;
		/** Issues the resume. Resolves with the result, throws the refusal. */
		onresume: (request: BranchMergeResumeRequest) => Promise<BranchMergeResumeResult>;
		onclose: () => void;
		/** The merge is finished; the page adopts the result. */
		onfinished?: (result: BranchMergeResumeResult) => void;
		/** The user asked for the branch as it now stands. */
		onrefresh?: () => void;
	}

	let { open, branch, onresume, onclose, onfinished, onrefresh }: Props = $props();

	let pending: MergePendingEntity[] = $state([]);
	let resolutions: Map<string, MergeResolution> = $state(new Map());
	let rationales: Map<string, string> = $state(new Map());
	let inFlight = $state(false);
	let result: BranchMergeResumeResult | null = $state(null);
	let failure: FinishMergeFailureCopy | null = $state(null);
	let failureMessage: string | null = $state(null);
	/** A refusal that leaves the user deciding, shown above the decisions. */
	let notice: string | null = $state(null);
	let blockers: MergeBlocker[] = $state([]);

	// A reopened dialog starts from the branch as the page now has it. Only
	// opening resets it: the page adopts the finished branch while the success
	// summary is still showing, and that must not wipe the summary.
	$effect(() => {
		if (open) {
			pending = untrack(() => pendingEntities(branch));
			resolutions = new Map();
			rationales = new Map();
			result = null;
			failure = null;
			failureMessage = null;
			notice = null;
			blockers = [];
		}
	});

	const ready = $derived(pending.filter((entity) => !entity.needs_resolution && entity.reason !== 'needs_repair'));
	const repairs = $derived(pending.filter((entity) => entity.reason === 'needs_repair'));
	const shownReady = $derived(ready.slice(0, READY_PREVIEW_LIMIT));
	const needing = $derived(pending.filter((entity) => entity.needs_resolution));
	const reasons = $derived(new Map(needing.map((entity) => [entity.stream_id, entity.reason])));
	const decisions = $derived(needing.map(pendingAsResolvable));
	const undecided = $derived(needing.filter((entity) => !resolutions.has(entity.stream_id)).length);
	const blocked = $derived(blockedEntityIds(blockers));

	function plural(n: number, one: string, many = `${one}s`): string {
		return `${n} ${n === 1 ? one : many}`;
	}

	const summary = $derived(
		isUnreadableMerge(branch) && pending.length === 0
			? 'This page could not work out how far the merge got. Finishing it checks the mainline, and either completes the merge or says what is wrong.'
			: pending.length === 0
				? 'Nothing is left to replay: finishing checks the mainline and completes the merge.'
				: incompleteMergeSummary(pending)
	);

	function resolve(streamId: string, resolution: MergeResolution) {
		resolutions = new Map(resolutions).set(streamId, resolution);
		blockers = [];
	}

	function resolveAll(entries: Array<[string, MergeResolution]>) {
		const next = new Map(resolutions);
		for (const [streamId, resolution] of entries) next.set(streamId, resolution);
		resolutions = next;
		blockers = [];
	}

	function setRationale(streamId: string, rationale: string) {
		rationales = new Map(rationales).set(streamId, rationale);
	}

	function request(): BranchMergeResumeRequest {
		const live = new Set(needing.map((entity) => entity.stream_id));
		const entries = [...resolutions]
			.filter(([streamId]) => live.has(streamId))
			.map(([stream_id, resolution]) => {
				const rationale = rationales.get(stream_id)?.trim();
				return { stream_id, resolution, ...(rationale ? { rationale } : {}) };
			});
		return entries.length > 0 ? { resolutions: entries } : {};
	}

	/**
	 * A refusal that keeps the user deciding: add what now needs a decision,
	 * keep every decision already made.
	 *
	 * The refusal lists only the entities that request left undecided - an
	 * entity the request resolved is replayed or left behind by it, never
	 * pending - so its list is merged into the dialog's, not substituted for
	 * it: an entity decided here and absent from the refusal keeps its
	 * decision and rationale for the next attempt. A listed entity replaces
	 * the dialog's copy of it (typically a ready one the mainline has since
	 * changed), and a decision it no longer accepts is dropped.
	 */
	function keepDeciding(refusal: BranchMergeResumeRefusal) {
		if (refusal.code === 'merge_resume_needs_resolution' && refusal.pending) {
			const fresh = new Map(refusal.pending.map((entity) => [entity.stream_id, entity]));
			const merged = pending.map((entity) => fresh.get(entity.stream_id) ?? entity);
			const known = new Set(pending.map((entity) => entity.stream_id));
			pending = [...merged, ...refusal.pending.filter((entity) => !known.has(entity.stream_id))];
			const accepts = (streamId: string, resolution: MergeResolution) => {
				const entity = pending.find((candidate) => candidate.stream_id === streamId);
				return !!entity && entity.needs_resolution && entity.supported_resolutions.includes(resolution);
			};
			resolutions = new Map([...resolutions].filter(([streamId, resolution]) => accepts(streamId, resolution)));
			rationales = new Map([...rationales].filter(([streamId]) => resolutions.has(streamId)));
			notice =
				'The mainline changed since this list was loaded, so more entities need a decision. Nothing was written.';
			return;
		}
		if (refusal.code === 'merge_dangling_reference') {
			blockers = refusal.blockers ?? [];
			notice =
				'These decisions would break a reference between entities, so nothing was written. Keep the mainline\'s version for the entity at fault, or take the branch\'s for the one it references.';
			return;
		}
		notice = refusal.message;
	}

	async function handleFinish(event: Event) {
		event.preventDefault();
		if (inFlight || undecided > 0) return;
		inFlight = true;
		notice = null;
		failure = null;
		failureMessage = null;
		try {
			result = await onresume(request());
			onfinished?.(result);
		} catch (e) {
			if (isBranchMergeResumeRefusal(e)) {
				const copy = FINISH_MERGE_FAILURE_COPY[e.code];
				if (copy) {
					failure = copy;
					failureMessage = e.message;
				} else {
					keepDeciding(e);
				}
			} else {
				const message = typeof e === 'object' && e !== null ? (e as { message?: unknown }).message : undefined;
				failure = GENERIC_FINISH_FAILURE;
				failureMessage = (typeof message === 'string' && message) || null;
			}
		} finally {
			inFlight = false;
		}
	}

	function handleRefresh() {
		onrefresh?.();
		onclose();
	}
</script>

<AlertDialog.Root
	{open}
	onOpenChange={(isOpen) => {
		if (!isOpen && !inFlight) onclose();
	}}
>
	<AlertDialog.Content
		class="max-h-[85vh] overflow-y-auto sm:max-w-2xl"
		escapeKeydownBehavior={inFlight ? 'ignore' : 'close'}
		interactOutsideBehavior={inFlight ? 'ignore' : 'close'}
	>
		<AlertDialog.Header>
			<AlertDialog.Title>
				{#if result}
					Finished merging {branch.name}
				{:else if failure}
					{failure.title}
				{:else}
					Finish merging {branch.name}?
				{/if}
			</AlertDialog.Title>
			<AlertDialog.Description>
				{#if result}
					Everything this branch's merge set out to promote is now on the mainline.
				{:else if failure}
					{failure.body}
				{:else}
					{summary} Only what is missing is replayed; nothing that already landed is repeated.
				{/if}
			</AlertDialog.Description>
		</AlertDialog.Header>

		{#if result}
			<dl class="summary">
				<div>
					<dt>Events replayed now</dt>
					<dd>{result.replayed_event_count}</dd>
				</div>
				<div>
					<dt>Already on the mainline</dt>
					<dd>{result.already_replayed_stream_ids.length}</dd>
				</div>
				<div>
					<dt>Left behind</dt>
					<dd>{result.skipped_stream_ids.length}</dd>
				</div>
			</dl>
		{:else if failure}
			{#if failureMessage}
				<div class="failure" role="alert">
					<p class="server-message">{failureMessage}</p>
				</div>
			{/if}
		{:else}
			{#if notice}
				<div class="notice" role="alert">
					<p>{notice}</p>
					{#if blockers.length > 0}
						<ul class="blocker-list" aria-label="Merge blockers">
							{#each blockers as blocker (`${blocker.kind}:${blocker.stream_id}:${blocker.referenced_id}`)}
								<li>{describeBlocker(blocker)}</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/if}

			{#if needing.length > 0}
				<section class="plan-section" aria-labelledby="finish-decisions-heading">
					<h3 id="finish-decisions-heading">Needs your decision ({needing.length})</h3>
					<MergeConflictResolver
						conflicts={decisions}
						{resolutions}
						onresolve={resolve}
						onresolveall={resolveAll}
						{rationales}
						onrationale={setRationale}
						disabled={inFlight}
						{blocked}
						noun="change"
						badgeOf={(conflict) => pendingReasonLabel(reasons.get(conflict.stream_id) ?? 'main_changed')}
						soleOptionReasonOf={(conflict) =>
							pendingSoleOptionReason(reasons.get(conflict.stream_id) ?? 'main_removed')}
					/>
				</section>
			{/if}

			{#if repairs.length > 0}
				<section class="plan-section" aria-labelledby="finish-repair-heading">
					<h3 id="finish-repair-heading">Will be repaired from the mainline's history ({repairs.length})</h3>
					<ul class="plan-list">
						{#each repairs.slice(0, READY_PREVIEW_LIMIT) as entity (entity.stream_id)}
							<li>
								<span class="entity-type">{entityTypeLabel(entity.entity_type)}</span>
								<span class="entity-name">{entity.entity_name || unnamedEntityLabel(entity.entity_type)}</span>
							</li>
						{/each}
					</ul>
					{#if repairs.length > READY_PREVIEW_LIMIT}
						<p class="section-hint">+{repairs.length - READY_PREVIEW_LIMIT} more not listed here.</p>
					{/if}
				</section>
			{/if}

			{#if ready.length > 0}
				<section class="plan-section" aria-labelledby="finish-ready-heading">
					<h3 id="finish-ready-heading">Will be replayed as planned ({ready.length})</h3>
					<ul class="plan-list">
						{#each shownReady as entity (entity.stream_id)}
							<li>
								<span class="entity-type">{entityTypeLabel(entity.entity_type)}</span>
								<span class="entity-name">{entity.entity_name || unnamedEntityLabel(entity.entity_type)}</span>
							</li>
						{/each}
					</ul>
					{#if ready.length > shownReady.length}
						<p class="section-hint">+{ready.length - shownReady.length} more not listed here.</p>
					{/if}
				</section>
			{/if}
		{/if}

		<AlertDialog.Footer>
			{#if result}
				<AlertDialog.Cancel>Done</AlertDialog.Cancel>
			{:else if failure}
				<AlertDialog.Cancel>Close</AlertDialog.Cancel>
				{#if failure.recovery === 'retry'}
					<AlertDialog.Action disabled={inFlight} onclick={handleFinish}>Try again</AlertDialog.Action>
				{:else if failure.recovery === 'refresh'}
					<AlertDialog.Action onclick={handleRefresh}>Refresh</AlertDialog.Action>
				{/if}
			{:else}
				{#if undecided > 0}
					<p class="undecided" role="status">
						{plural(undecided, 'entity', 'entities')} still {undecided === 1 ? 'needs' : 'need'} a decision.
					</p>
				{/if}
				<AlertDialog.Cancel
					disabled={inFlight}
					aria-disabled={inFlight}
					class="aria-disabled:pointer-events-none aria-disabled:opacity-50"
				>
					Cancel
				</AlertDialog.Cancel>
				<AlertDialog.Action disabled={inFlight || undecided > 0} onclick={handleFinish}>
					{inFlight ? 'Finishing...' : 'Finish merge'}
				</AlertDialog.Action>
			{/if}
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<style>
	.plan-section h3 {
		margin: 0 0 0.5rem;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: #64748b;
	}

	.plan-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.plan-list li {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		padding: 0.5rem 0.75rem;
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

	.section-hint {
		margin: 0.5rem 0 0;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.notice {
		padding: 0.75rem;
		background: #fff7ed;
		border: 1px solid #fdba74;
		border-radius: 6px;
		color: #7c2d12;
		font-size: 0.8125rem;
	}

	.notice p {
		margin: 0;
	}

	.blocker-list {
		margin: 0.5rem 0 0;
		padding-left: 1.25rem;
		overflow-wrap: anywhere;
	}

	.failure {
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

	.undecided {
		margin: 0 auto 0 0;
		align-self: center;
		font-size: 0.8125rem;
		color: #b45309;
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
</style>
