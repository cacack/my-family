/**
 * Shared vocabulary for a branch's research record (#835): the outcome labels
 * and the limits the server enforces. Mirrors `BranchOutcome`, `Branch` and
 * `BranchUpdate` in `internal/api/openapi.yaml`.
 */
import type {
	BranchChangeEntry,
	BranchCloseOutcome,
	BranchOutcome,
	BranchSubject,
	PromoteResearchLogsResult
} from '$lib/api/client';

export const HYPOTHESIS_MAX_LENGTH = 2000;
/** Mirrors `BranchCloseRequest.reason` maxLength (#836). */
export const CLOSE_REASON_MAX_LENGTH = 2000;
export const MAX_SUBJECTS = 50;
export const MAX_PROOF_SUMMARIES = 20;

/** Every outcome, in the order a picker should offer them. */
export const BRANCH_OUTCOMES: readonly BranchOutcome[] = [
	'open',
	'proved',
	'disproved',
	'inconclusive',
	'superseded',
	'abandoned'
];

/**
 * The outcomes a branch can be closed with (#836), in picker order. `open` is
 * no verdict and a proved branch is merged rather than closed.
 */
export const CLOSE_OUTCOMES: readonly BranchCloseOutcome[] = [
	'disproved',
	'inconclusive',
	'superseded',
	'abandoned'
];

export const OUTCOME_LABELS: Record<BranchOutcome, string> = {
	open: 'Open',
	proved: 'Proved',
	disproved: 'Disproved',
	inconclusive: 'Inconclusive',
	superseded: 'Superseded',
	abandoned: 'Abandoned'
};

/** One line explaining each close outcome, shown under the picker. */
export const CLOSE_OUTCOME_HINTS: Record<BranchCloseOutcome, string> = {
	disproved: 'The evidence shows the hypothesis is false.',
	inconclusive: 'The search was reasonably exhaustive but did not settle the question.',
	superseded: 'Other research has overtaken this question.',
	abandoned: 'The research was stopped without a verdict.'
};

/** Why research logs were not promoted, read after a count (#836). */
export const PROMOTE_SKIP_LABELS: Record<string, string> = {
	not_found: 'not found on this branch',
	not_created_on_branch: 'already mainline entries',
	already_promoted: 'already copied',
	subject_not_on_main: 'about someone not on the mainline'
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

/**
 * A sentence reporting a research-log promotion (#836): how many logs were
 * copied to the mainline, and why any were not.
 */
export function promotionSummary(result: PromoteResearchLogsResult): string {
	const copied = result.promoted.length;
	const parts = [
		copied === 0
			? 'No research logs were copied to the mainline'
			: `${copied} research log${copied === 1 ? ' was' : 's were'} copied to the mainline`
	];
	const reasons = new Map<string, number>();
	for (const skipped of result.skipped) {
		reasons.set(skipped.reason, (reasons.get(skipped.reason) ?? 0) + 1);
	}
	const skippedText = [...reasons]
		.map(([reason, count]) => `${count} ${PROMOTE_SKIP_LABELS[reason] ?? reason}`)
		.join('; ');
	return skippedText ? `${parts[0]} (not copied: ${skippedText}).` : `${parts[0]}.`;
}
