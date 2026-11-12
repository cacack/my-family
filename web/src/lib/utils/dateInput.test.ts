import { describe, it, expect } from 'vitest';
import {
	asDatePhrase,
	dateErrorAttrs,
	dateFieldError,
	savedAsNotes,
	savedAsText,
	stillStored,
	unreadableDateField
} from './dateInput';

describe('dateFieldError', () => {
	const fields = ['birth_date', 'death_date'] as const;

	it('names the field and drops the server prefix from the message', () => {
		const error = {
			message: 'invalid input: death_date: "31 FEB 1900" is not a real calendar date',
			details: { field: 'death_date', unreadable: false }
		};
		expect(dateFieldError(error, fields)).toEqual({
			field: 'death_date',
			message: '"31 FEB 1900" is not a real calendar date',
			unreadable: false
		});
	});

	it('keeps a message without the prefix as it is', () => {
		const error = { message: 'not a date', details: { field: 'birth_date', unreadable: true } };
		expect(dateFieldError(error, fields)).toEqual({ field: 'birth_date', message: 'not a date', unreadable: true });
	});

	it('is null for another field or another error', () => {
		expect(dateFieldError({ details: { field: 'marriage_date', unreadable: true } }, fields)).toBeNull();
		expect(dateFieldError({ message: 'x' }, fields)).toBeNull();
		expect(dateFieldError(undefined, fields)).toBeNull();
	});
});

describe('dateErrorAttrs', () => {
	const error = { field: 'birth_date', message: 'x', unreadable: false };

	it('marks only the refused field and points it at its message', () => {
		expect(dateErrorAttrs(error, 'birth_date')).toEqual({
			'aria-invalid': 'true',
			'aria-describedby': 'birth_date-error'
		});
		expect(dateErrorAttrs(error, 'death_date')).toEqual({});
		expect(dateErrorAttrs(null, 'birth_date')).toEqual({});
	});
});

describe('unreadableDateField', () => {
	const fields = ['birth_date', 'death_date'] as const;

	it('names the field of an unreadable date', () => {
		const error = { code: 'VALIDATION_ERROR', message: 'x', details: { field: 'death_date', unreadable: true } };
		expect(unreadableDateField(error, fields)).toBe('death_date');
	});

	it('is null for an impossible date, another field, or another error', () => {
		expect(unreadableDateField({ details: { field: 'birth_date', unreadable: false } }, fields)).toBeNull();
		expect(unreadableDateField({ details: { field: 'marriage_date', unreadable: true } }, fields)).toBeNull();
		expect(unreadableDateField({ code: 'VALIDATION_ERROR', message: 'x' }, fields)).toBeNull();
		expect(unreadableDateField(undefined, fields)).toBeNull();
	});
});

describe('asDatePhrase', () => {
	it('wraps the trimmed text in parentheses', () => {
		expect(asDatePhrase('  Sept 5, 1850 ')).toBe('(Sept 5, 1850)');
	});
});

describe('savedAsNotes', () => {
	it('notes the dates stored in another form', () => {
		expect(
			savedAsNotes(
				{ birth_date: 'about 1850', death_date: '1 JAN 1900' },
				{ birth_date: 'ABT 1850', death_date: '1 JAN 1900' }
			)
		).toEqual({ birth_date: { typed: 'about 1850', stored: 'ABT 1850' } });
	});

	it('ignores surrounding whitespace and empty dates', () => {
		expect(savedAsNotes({ birth_date: ' 1850 ', death_date: '' }, { birth_date: '1850', death_date: '' })).toEqual({});
	});
});

describe('stillStored', () => {
	const note = { typed: 'about 1850', stored: 'ABT 1850' };

	it('keeps a note that still matches the stored date', () => {
		expect(stillStored(note, 'ABT 1850')).toBe(note);
	});

	it('drops a note for a date edited since', () => {
		expect(stillStored(note, '1851')).toBeUndefined();
	});

	it('passes no note through as none', () => {
		expect(stillStored(undefined, 'ABT 1850')).toBeUndefined();
	});
});

describe('savedAsText', () => {
	it('says what a typed date was saved as', () => {
		expect(savedAsText({ typed: 'about 1850', stored: 'ABT 1850' })).toBe('Saved as ABT 1850 from “about 1850”');
	});

	it('is empty without a note', () => {
		expect(savedAsText(undefined)).toBeUndefined();
	});
});
