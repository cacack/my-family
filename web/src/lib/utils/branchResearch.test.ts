import { describe, it, expect } from 'vitest';
import type { BranchChangeEntry } from '$lib/api/client';
import { branchSubjectCandidates } from './branchResearch';

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
