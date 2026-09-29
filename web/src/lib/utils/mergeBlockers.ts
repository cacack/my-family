/**
 * Words for merge blockers (#831): the cross-entity references a merge would
 * break, as `POST /branches/{id}/merge/precheck` and a
 * `409 merge_dangling_reference` report them. Shared by the review's blockers
 * panel and the merge dialog, so both say the same thing.
 */
import type { MergeBlocker, MergeResolution } from '$lib/api/client';
import { entityTypeLabel, unnamedEntityLabel } from '$lib/utils/changeEntries';

/** An entity's name, or its type when nothing names it. */
export function blockerEntityName(blocker: MergeBlocker): string {
	return blocker.entity_name || unnamedEntityLabel(blocker.entity_type);
}

/** The referenced entity's name, or its type when nothing names it. */
export function blockerReferencedName(blocker: MergeBlocker): string {
	return blocker.referenced_name || unnamedEntityLabel(blocker.referenced_type);
}

function subject(blocker: MergeBlocker): string {
	return `${entityTypeLabel(blocker.entity_type)} "${blockerEntityName(blocker)}"`;
}

function referenced(blocker: MergeBlocker): string {
	return `${entityTypeLabel(blocker.referenced_type).toLowerCase()} "${blockerReferencedName(blocker)}"`;
}

/** One sentence saying what the merge would break, by name. */
export function describeBlocker(blocker: MergeBlocker): string {
	const e = subject(blocker);
	const r = referenced(blocker);
	switch (blocker.kind) {
		case 'missing_person':
			return `${e} names ${r}, who will not exist on the mainline.`;
		case 'missing_source':
			return `${e} cites ${r}, which will not exist on the mainline.`;
		case 'source_delete_orphans_citation':
			return `Deleting ${e} would also delete the mainline's ${r}, which still cites it.`;
		case 'missing_media_owner':
			return `${e} is attached to ${r}, which will not exist on the mainline.`;
		case 'owner_delete_orphans_media':
			return `Deleting ${e} would also delete the mainline's ${r}, which this branch never saw.`;
		case 'missing_gps_artifact':
			return `${e} edits research the mainline no longer has.`;
		case 'missing_gps_subject':
			return `${e} is about ${r}, which will not exist on the mainline.`;
		case 'subject_delete_orphans_gps':
			return `Deleting ${e} would also delete the mainline's ${r}, added or changed after the fork.`;
		default:
			return `${e} references ${r}, which the merge would break.`;
	}
}

/** The label of the one-click fix; its visible text is also its accessible name. */
export function blockerFixLabel(blocker: MergeBlocker): string {
	return blocker.suggested_resolution === 'include_referenced'
		? `Include ${blockerReferencedName(blocker)}`
		: `Also leave out ${blockerEntityName(blocker)}`;
}

/** The single resolution a blocker's suggested fix sets: which entity, which side. */
export function blockerFix(blocker: MergeBlocker): { streamId: string; resolution: MergeResolution } {
	return blocker.suggested_resolution === 'include_referenced'
		? { streamId: blocker.referenced_id, resolution: 'branch' }
		: { streamId: blocker.stream_id, resolution: 'main' };
}

/** Every entity a blocker involves, for highlighting the rows they appear on. */
export function blockedEntityIds(blockers: MergeBlocker[]): Set<string> {
	const ids = new Set<string>();
	for (const blocker of blockers) {
		ids.add(blocker.stream_id);
		ids.add(blocker.referenced_id);
	}
	return ids;
}
