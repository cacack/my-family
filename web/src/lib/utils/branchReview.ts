/**
 * Wording and links for the merge review's two soft checks (#838): the
 * evidence-coverage warning and the branch health. Pure functions, so the
 * panels stay thin and the wording is tested once.
 */
import type { BranchChangedFact, BranchHealth, BranchValidationIssue } from '$lib/api/client';
import { formatFactTypeShort } from '$lib/utils/evidence';

function plural(count: number, one: string, many = `${one}s`): string {
	return `${count} ${count === 1 ? one : many}`;
}

/** What changed, in words: "Birth", "Occupation", the relationship, or a deletion. */
export function changedFactLabel(fact: BranchChangedFact): string {
	if (fact.kind === 'deletion') {
		return fact.subject_type === 'family' ? 'Family deleted' : 'Person deleted';
	}
	if (fact.kind === 'relationship' || !fact.fact_type) {
		return 'Partners or children';
	}
	return formatFactTypeShort(fact.fact_type);
}

/** The page that shows a subject: its person or family page. */
export function subjectHref(subjectType: string, subjectId: string): string {
	return subjectType === 'family' ? `/families/${subjectId}` : `/persons/${subjectId}`;
}

/**
 * The new-analysis form, prefilled with the subject and, for a fact, its
 * fact type. A relationship or a deletion has no fact type of its own - any
 * analysis about the subject documents it - so the form is told the subject's
 * type and picks a fitting default (a family's marriage, a person's birth).
 */
export function addAnalysisHref(fact: BranchChangedFact): string {
	const params = new URLSearchParams({ subjectId: fact.subject_id });
	if (fact.kind === 'fact' && fact.fact_type) {
		params.set('factType', fact.fact_type);
	} else {
		params.set('subjectType', fact.subject_type);
	}
	return `/evidence/analyses/new?${params.toString()}`;
}

/** "3 changed facts or relationships have no evidence ..." */
export function coverageHeading(uncovered: number): string {
	return `${plural(uncovered, 'changed fact or relationship', 'changed facts or relationships')} ${
		uncovered === 1 ? 'has' : 'have'
	} no evidence analysis or proof summary on this branch`;
}

/** Everything the branch introduces, counted, for the section's summary line. */
export function healthSummary(health: BranchHealth): string {
	const parts: string[] = [];
	if (health.error_count > 0) parts.push(plural(health.error_count, 'error'));
	if (health.warning_count > 0) parts.push(plural(health.warning_count, 'warning'));
	if (health.info_count > 0) parts.push(plural(health.info_count, 'notice'));
	if (health.quality_issues.length > 0) {
		parts.push(plural(health.quality_issues.length, 'quality issue'));
	}
	if (health.duplicates.length > 0) {
		parts.push(plural(health.duplicates.length, 'possible duplicate'));
	}
	if (parts.length === 0) {
		return 'This branch introduces no new validation issues, quality issues or possible duplicates.';
	}
	const list =
		parts.length === 1
			? parts[0]
			: `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`;
	return `This branch introduces ${list} that the mainline does not have.`;
}

/** The record an issue is about, as a link target; null when it names none. */
export function issueRecordHref(issue: BranchValidationIssue): string | null {
	if (!issue.record_id || !issue.record_type) return null;
	switch (issue.record_type) {
		case 'person':
			return `/persons/${issue.record_id}`;
		case 'family':
			return `/families/${issue.record_id}`;
		case 'source':
			return `/sources/${issue.record_id}`;
		default:
			return null;
	}
}

/** A confidence in [0, 1] as a whole percentage. */
export function confidencePercent(confidence: number): string {
	return `${Math.round(confidence * 100)}%`;
}
