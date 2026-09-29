import { describe, it, expect } from 'vitest';
import type { BranchChangedFact, BranchHealth, BranchValidationIssue } from '$lib/api/client';
import {
	addAnalysisHref,
	changedFactLabel,
	confidencePercent,
	coverageHeading,
	healthSummary,
	issueRecordHref,
	subjectHref
} from './branchReview';

const fact: BranchChangedFact = {
	kind: 'fact',
	fact_type: 'person_birth',
	subject_type: 'person',
	subject_id: 'p-1',
	subject_name: 'Ada Sample',
	change_count: 1
};
const relationship: BranchChangedFact = {
	kind: 'relationship',
	subject_type: 'family',
	subject_id: 'f-1',
	subject_name: 'Ada Sample & Bob Sample',
	change_count: 2
};

const noHealth: BranchHealth = {
	validation_issues: [],
	quality_issues: [],
	duplicates: [],
	error_count: 0,
	warning_count: 0,
	info_count: 0,
	resolved_count: 0
};

describe('branch review wording', () => {
	it('labels a fact by its type and a relationship as such', () => {
		expect(changedFactLabel(fact)).toBe('Birth');
		expect(changedFactLabel({ ...fact, fact_type: 'family_marriage' })).toBe('Marriage');
		expect(changedFactLabel(relationship)).toBe('Partners or children');
		expect(changedFactLabel({ ...relationship, kind: 'deletion' })).toBe('Family deleted');
		expect(changedFactLabel({ ...fact, kind: 'deletion', fact_type: undefined })).toBe(
			'Person deleted'
		);
	});

	it('links the subject and the prefilled analysis form', () => {
		expect(subjectHref('person', 'p-1')).toBe('/persons/p-1');
		expect(subjectHref('family', 'f-1')).toBe('/families/f-1');
		expect(addAnalysisHref(fact)).toBe(
			'/evidence/analyses/new?subjectId=p-1&factType=person_birth'
		);
		expect(addAnalysisHref(relationship)).toBe(
			'/evidence/analyses/new?subjectId=f-1&subjectType=family'
		);
		expect(addAnalysisHref({ ...fact, kind: 'deletion', fact_type: undefined })).toBe(
			'/evidence/analyses/new?subjectId=p-1&subjectType=person'
		);
	});

	it('counts the undocumented changes', () => {
		expect(coverageHeading(1)).toBe(
			'1 changed fact or relationship has no evidence analysis or proof summary on this branch'
		);
		expect(coverageHeading(3)).toBe(
			'3 changed facts or relationships have no evidence analysis or proof summary on this branch'
		);
	});

	it('summarises what the branch introduces', () => {
		expect(healthSummary(noHealth)).toBe(
			'This branch introduces no new validation issues, quality issues or possible duplicates.'
		);
		expect(healthSummary({ ...noHealth, error_count: 1 })).toBe(
			'This branch introduces 1 error that the mainline does not have.'
		);
		expect(
			healthSummary({
				...noHealth,
				error_count: 2,
				warning_count: 1,
				info_count: 3,
				quality_issues: [{ key: 'k', person_id: 'p', person_name: 'P', issue: 'i' }],
				duplicates: [
					{
						key: 'd',
						person1_id: 'a',
						person1_name: 'A',
						person2_id: 'b',
						person2_name: 'B',
						confidence: 0.9,
						match_reasons: []
					}
				]
			})
		).toBe(
			'This branch introduces 2 errors, 1 warning, 3 notices, 1 quality issue and 1 possible duplicate that the mainline does not have.'
		);
	});

	it('links an issue to its record', () => {
		const issue: BranchValidationIssue = {
			key: 'k',
			severity: 'warning',
			code: 'C',
			message: 'm',
			record_id: 'r-1',
			record_type: 'person'
		};
		expect(issueRecordHref(issue)).toBe('/persons/r-1');
		expect(issueRecordHref({ ...issue, record_type: 'family' })).toBe('/families/r-1');
		expect(issueRecordHref({ ...issue, record_type: 'source' })).toBe('/sources/r-1');
		expect(issueRecordHref({ ...issue, record_id: undefined })).toBeNull();
	});

	it('shows a confidence as a percentage', () => {
		expect(confidencePercent(0.874)).toBe('87%');
	});
});
