import { describe, it, expect } from 'vitest';
import type { BranchChangeEntry } from '$lib/api/client';
import {
	BRANCH_OUTCOMES,
	CLOSE_OUTCOMES,
	CLOSE_OUTCOME_HINTS,
	OUTCOME_LABELS,
	branchSubjectCandidates,
	promotionSummary
} from './branchResearch';

function change(
	entity_type: BranchChangeEntry['entity_type'],
	entity_id: string,
	action: BranchChangeEntry['action'],
	entity_name?: string
): BranchChangeEntry {
	return { id: crypto.randomUUID(), timestamp: '2026-01-15T10:30:00Z', entity_type, entity_id, action, entity_name };
}

describe('branchSubjectCandidates', () => {
	it('offers the persons and families the branch changed and still has', () => {
		const candidates = branchSubjectCandidates([
			change('person', 'p1', 'created', 'Mary Smith'),
			change('source', 's1', 'created', 'Census 1850'),
			change('family', 'f1', 'created', 'Mary Smith & John Doe'),
			change('person', 'p2', 'created', 'Gone Person'),
			change('person', 'p1', 'updated', 'Mary Smyth'),
			change('person', 'p2', 'deleted'),
			change('person', 'p3', 'updated')
		]);

		expect(candidates).toEqual([
			{ type: 'person', id: 'p1', name: 'Mary Smyth' },
			{ type: 'family', id: 'f1', name: 'Mary Smith & John Doe' },
			{ type: 'person', id: 'p3', name: undefined }
		]);
	});

	it('keeps an earlier name when a later change carries none', () => {
		expect(
			branchSubjectCandidates([change('person', 'p1', 'created', 'Mary'), change('person', 'p1', 'updated')])
		).toEqual([{ type: 'person', id: 'p1', name: 'Mary' }]);
	});
});

describe('close vocabulary (#836)', () => {
	it('labels every outcome and explains every close outcome', () => {
		for (const outcome of BRANCH_OUTCOMES) expect(OUTCOME_LABELS[outcome]).toBeTruthy();
		for (const outcome of CLOSE_OUTCOMES) expect(CLOSE_OUTCOME_HINTS[outcome]).toBeTruthy();
		expect(CLOSE_OUTCOMES).not.toContain('open');
		expect(CLOSE_OUTCOMES).not.toContain('proved');
		expect(BRANCH_OUTCOMES).toContain('abandoned');
	});
});

describe('promotionSummary', () => {
	it('counts what was copied and groups what was not by reason', () => {
		expect(
			promotionSummary({
				promoted: ['a', 'b'],
				skipped: [
					{ id: 'c', reason: 'subject_not_on_main' },
					{ id: 'd', reason: 'subject_not_on_main' },
					{ id: 'e', reason: 'not_created_on_branch' }
				],
				truncated: false
			})
		).toBe(
			'2 research logs were copied to the mainline (not copied: 2 about someone not on the mainline; 1 already mainline entries).'
		);
	});

	it('reads naturally for one and for none', () => {
		expect(promotionSummary({ promoted: ['a'], skipped: [], truncated: false })).toBe(
			'1 research log was copied to the mainline.'
		);
		expect(promotionSummary({ promoted: [], skipped: [], truncated: false })).toBe(
			'No research logs were copied to the mainline.'
		);
	});
});
