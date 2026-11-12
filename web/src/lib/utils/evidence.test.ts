import { describe, it, expect } from 'vitest';
import { readableConflictDescription, subjectTypeOf } from './evidence.js';

describe('readableConflictDescription', () => {
	const description = 'Conflicting conclusions for person_birth on subject 8ac089f9-0000-4000-8000-000000000001';
	const id = '8ac089f9-0000-4000-8000-000000000001';

	it('names the subject and shows the fact type readably', () => {
		expect(readableConflictDescription(description, 'person_birth', id, 'Ada Lovelace')).toBe(
			'Conflicting conclusions for Person Birth on Ada Lovelace'
		);
	});

	it('never shows the id while the name is unknown', () => {
		expect(readableConflictDescription(description, 'person_birth', id, undefined)).toBe(
			'Conflicting conclusions for Person Birth on this subject'
		);
	});

	it('leaves a description written by hand alone', () => {
		expect(readableConflictDescription('Two census years disagree', 'person_birth', id, 'Ada')).toBe(
			'Two census years disagree'
		);
	});
});

describe('subjectTypeOf', () => {
	it('reads the subject type from the fact type', () => {
		expect(subjectTypeOf('family_marriage')).toBe('family');
		expect(subjectTypeOf('person_death')).toBe('person');
	});
});
