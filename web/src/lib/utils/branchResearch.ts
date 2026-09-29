/**
 * Shared vocabulary for a branch's research record (#835): the outcome labels
 * and the limits the server enforces. Mirrors `BranchOutcome`, `Branch` and
 * `BranchUpdate` in `internal/api/openapi.yaml`.
 */
import type { BranchChangeEntry, BranchOutcome, BranchSubject } from '$lib/api/client';

export const HYPOTHESIS_MAX_LENGTH = 2000;
export const MAX_SUBJECTS = 50;
export const MAX_PROOF_SUMMARIES = 20;

/** Every outcome, in the order a picker should offer them. */
export const BRANCH_OUTCOMES: readonly BranchOutcome[] = [
	'open',
	'proved',
	'disproved',
	'inconclusive',
	'superseded'
];

export const OUTCOME_LABELS: Record<BranchOutcome, string> = {
	open: 'Open',
	proved: 'Proved',
	disproved: 'Disproved',
	inconclusive: 'Inconclusive',
	superseded: 'Superseded'
};

/** A pre-#835 branch (or an older server) may omit the outcome: it is open. */
export function branchOutcome(outcome: BranchOutcome | undefined | null): BranchOutcome {
	return outcome ?? 'open';
}

/** Where a subject's own page lives. */
export function subjectHref(subject: Pick<BranchSubject, 'type' | 'id'>): string {
	return subject.type === 'family' ? `/families/${subject.id}` : `/persons/${subject.id}`;
}

/** A subject's display name, or a fallback that says what it was. */
export function subjectLabel(subject: BranchSubject): string {
	if (subject.name) return subject.name;
	return subject.type === 'family' ? 'Unnamed family' : 'Unnamed person';
}

export function proofSummaryHref(id: string): string {
	return `/evidence/proof-summaries/${id}`;
}

/**
 * The persons and families a branch created or changed and still has, as
 * subject candidates. Person search reads the mainline, so this is how a
 * person or family that exists only on the branch can be picked. `changes` is
 * the comparison's `branch_changes` (oldest first): an entity whose last change
 * deleted it is left out, and the most recent name wins.
 */
export function branchSubjectCandidates(changes: readonly BranchChangeEntry[]): BranchSubject[] {
	const byId = new Map<string, { subject: BranchSubject; deleted: boolean }>();
	for (const entry of changes) {
		if (entry.entity_type !== 'person' && entry.entity_type !== 'family') continue;
		const prev = byId.get(entry.entity_id);
		byId.set(entry.entity_id, {
			subject: {
				type: entry.entity_type,
				id: entry.entity_id,
				name: entry.entity_name || prev?.subject.name
			},
			deleted: entry.action === 'deleted'
		});
	}
	return [...byId.values()].filter((c) => !c.deleted).map((c) => c.subject);
}
