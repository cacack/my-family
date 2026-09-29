/**
 * Words for an interrupted merge (#830): a merged branch whose replay onto the
 * mainline stopped partway, as `merge_state: incomplete` and `merge_pending`
 * report it. Shared by the branch list, the branch page, the banner notice and
 * the finish-merge dialog, so they all say the same thing.
 */
import type { Branch, MergePendingEntity } from '$lib/api/client';
import type { ResolvableConflict } from '$lib/components/MergeConflictResolver.svelte';

type PendingReason = MergePendingEntity['reason'];

/**
 * True for a merged branch whose merge did not finish, or whose merge state
 * the server could not read (`unknown`): both are flagged and offer Finish
 * merge, whose resume either completes the merge or says what is wrong.
 */
export function isIncompleteMerge(branch: Pick<Branch, 'status' | 'merge_state'> | null | undefined): boolean {
	return branch?.status === 'merged' && (branch.merge_state === 'incomplete' || branch.merge_state === 'unknown');
}

/** True for a merged branch whose merge state the server could not read. */
export function isUnreadableMerge(branch: Pick<Branch, 'status' | 'merge_state'> | null | undefined): boolean {
	return branch?.status === 'merged' && branch.merge_state === 'unknown';
}

/** The entities still to replay; empty for anything but an incomplete merge. */
export function pendingEntities(branch: Pick<Branch, 'merge_pending'> | null | undefined): MergePendingEntity[] {
	return branch?.merge_pending ?? [];
}

/** How many pending entities need the user's decision before the merge can finish. */
export function decisionsNeeded(pending: MergePendingEntity[]): number {
	return pending.filter((entity) => entity.needs_resolution).length;
}

/** What the list card and page banner say when the merge state could not be read. */
export const UNREADABLE_MERGE_SUMMARY =
	'This page could not work out how far it got. Finishing the merge checks it, and either completes it or says what is wrong.';

/** One line summing up what is left, for the list card and the page banner. */
export function incompleteMergeSummary(pending: MergePendingEntity[]): string {
	const repair = pending.filter((entity) => entity.reason === 'needs_repair').length;
	const replay = pending.length - repair;
	const needed = decisionsNeeded(pending);
	const parts: string[] = [];
	if (replay > 0) {
		parts.push(`${replay} ${replay === 1 ? 'entity has' : 'entities have'} not reached the mainline yet`);
	}
	if (repair > 0) {
		parts.push(
			`${repair} ${repair === 1 ? 'entity reached' : 'entities reached'} the mainline's history but not its data`
		);
	}
	if (parts.length === 0) return 'Finishing the merge checks the mainline and completes it.';
	const left = parts.join(', and ');
	if (needed === 0) return `${left}. Finishing the merge takes care of ${pending.length === 1 ? 'it' : 'them'}.`;
	return `${left}, and ${needed === 1 ? 'one needs' : `${needed} need`} your decision first.`;
}

/** The summary line for a flagged merged branch, whatever its state. */
export function branchMergeSummary(
	branch: Pick<Branch, 'status' | 'merge_state' | 'merge_pending'> | null | undefined
): string {
	return isUnreadableMerge(branch) ? UNREADABLE_MERGE_SUMMARY : incompleteMergeSummary(pendingEntities(branch));
}

/** The short badge naming why an entity is pending. */
export function pendingReasonLabel(reason: PendingReason): string {
	switch (reason) {
		case 'ready':
			return 'Ready to replay';
		case 'main_changed':
			return 'Changed on the mainline since';
		case 'main_removed':
			return 'Deleted on the mainline since';
		case 'no_plan':
			return 'Not covered by the merge plan';
		case 'breaks_reference':
			return 'Would break a reference';
		case 'needs_repair':
			return 'Needs repair';
		default:
			return reason;
	}
}

/** Why the entity needs a decision, in the user's terms. */
export function pendingReasonDetail(reason: PendingReason): string {
	switch (reason) {
		case 'ready':
			return 'Nothing changed since the merge was planned, so finishing it replays this as planned.';
		case 'main_changed':
			return "The mainline changed this after you reviewed the merge, so the decision you made then no longer describes it. Choose again with the mainline's change in view.";
		case 'main_removed':
			return 'The mainline deleted this (or merged the person into another) after the merge started.';
		case 'no_plan':
			return 'This merge was started by an older version of the app that did not record which changes it would replay, so nothing says whether this one should be.';
		case 'breaks_reference':
			return 'Replaying this would leave the mainline pointing at something it no longer has, or delete mainline data this branch never saw.';
		case 'needs_repair':
			return "Its changes reached the mainline's history, but the mainline's data does not show them yet. Finishing the merge rebuilds it from the history; nothing is replayed twice.";
		default:
			return 'This entity needs a decision before the merge can finish.';
	}
}

/** Why only keeping the mainline's version is offered for an entity. */
export function pendingSoleOptionReason(reason: PendingReason): string {
	switch (reason) {
		case 'main_removed':
			return "Replaying the branch's changes cannot bring a deleted entity back, so taking the branch's version is not offered - it would report success while the entity stayed deleted.";
		case 'breaks_reference':
			return "Taking the branch's version would be refused for the reference it breaks, so finishing the merge without this entity's branch changes is the way forward.";
		default:
			return 'Only one resolution would actually produce the outcome it names for this entity.';
	}
}

/**
 * A pending entity in the shape `MergeConflictResolver` decides, so an
 * interrupted merge's decisions use the same picker as the merge review.
 */
export function pendingAsResolvable(entity: MergePendingEntity): ResolvableConflict {
	return {
		stream_id: entity.stream_id,
		entity_type: entity.entity_type,
		entity_name: entity.entity_name,
		detail: pendingReasonDetail(entity.reason),
		supported_resolutions: entity.supported_resolutions
	};
}
