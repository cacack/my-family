import { describe, it, expect } from 'vitest';
import { additionalRelationships, genderedRelationship, relationshipChain } from './relationship';

describe('relationshipChain', () => {
	it('lists each person once, from A through the common ancestor to B', () => {
		const chain = relationshipChain({
			name: '1st cousin',
			commonAncestorId: 'g',
			pathFromA: [
				{ id: 'a', name: 'Ann' },
				{ id: 'p', name: 'Pat' },
				{ id: 'g', name: 'Gran' }
			],
			pathFromB: [
				{ id: 'b', name: 'Bob' },
				{ id: 'q', name: 'Quinn' },
				{ id: 'g', name: 'Gran' }
			]
		});
		expect(chain.map((n) => n.name)).toEqual(['Ann', 'Pat', 'Gran', 'Quinn', 'Bob']);
		expect(chain.filter((n) => n.isCommonAncestor).map((n) => n.name)).toEqual(['Gran']);
	});

	it('handles B being the common ancestor', () => {
		const chain = relationshipChain({
			name: 'parent',
			commonAncestorId: 'v',
			pathFromA: [
				{ id: 'b', name: 'Beatrice' },
				{ id: 'v', name: 'Victoria' }
			],
			pathFromB: [{ id: 'v', name: 'Victoria' }]
		});
		expect(chain.map((n) => n.name)).toEqual(['Beatrice', 'Victoria']);
		expect(chain[1].isCommonAncestor).toBe(true);
	});

	it('returns an empty chain when there are no paths', () => {
		expect(relationshipChain({ name: 'self' })).toEqual([]);
	});
});

describe('genderedRelationship', () => {
	it.each([
		['parent', 'female', 'mother'],
		['great-grandparent', 'male', 'great-grandfather'],
		['grandchild', 'female', 'granddaughter'],
		['child', 'male', 'son'],
		['sibling', 'female', 'sister'],
		['uncle/aunt', 'female', 'aunt'],
		['great-grand-nephew/niece', 'male', 'great-grand-nephew'],
		['1st cousin once removed', 'female', '1st cousin once removed'],
		['parent', 'unknown', 'parent'],
		['uncle/aunt', undefined, 'uncle/aunt']
	])('%s for %s is %s', (name, gender, expected) => {
		expect(genderedRelationship(name, gender)).toBe(expected);
	});
});

describe('additionalRelationships', () => {
	it('drops duplicates and repeats of the primary relationship', () => {
		expect(additionalRelationships(['mother', 'aunt', 'aunt', 'mother', 'cousin'])).toEqual([
			'aunt',
			'cousin'
		]);
	});

	it('is empty for a single relationship', () => {
		expect(additionalRelationships(['mother'])).toEqual([]);
	});
});
