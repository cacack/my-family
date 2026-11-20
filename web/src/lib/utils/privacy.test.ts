import { describe, it, expect } from 'vitest';
import { personBadgeNames, privateBadgeName } from './privacy.js';

describe('privacy badge names', () => {
	it('names the badges a person carries, living first', () => {
		expect(personBadgeNames({ living: true, private: true })).toEqual([
			'Living person',
			'Private person'
		]);
		expect(personBadgeNames({ private: true })).toEqual(['Private person']);
		expect(personBadgeNames({ living: false })).toEqual([]);
	});

	it('names only the placeholder badge on a redacted person', () => {
		expect(personBadgeNames({ redacted: true, living: true, private: true })).toEqual(['Details hidden']);
	});

	it('names a private record by its noun', () => {
		expect(privateBadgeName('source')).toBe('Private source');
	});
});
