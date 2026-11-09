import { describe, it, expect } from 'vitest';
import { formatGenDate, calendarLabel } from './client';

describe('calendarLabel', () => {
	it('labels historical calendars', () => {
		expect(calendarLabel('DJULIAN')).toBe('Julian');
		expect(calendarLabel('DHEBREW')).toBe('Hebrew');
		expect(calendarLabel('DFRENCH R')).toBe('French Republican');
	});

	it('returns empty for gregorian or unknown', () => {
		expect(calendarLabel('DGREGORIAN')).toBe('');
		expect(calendarLabel(undefined)).toBe('');
		expect(calendarLabel('DBOGUS')).toBe('');
	});
});

describe('formatGenDate with historical calendars', () => {
	it('strips the escape sequence and annotates the calendar from raw', () => {
		expect(
			formatGenDate({ raw: '@#DJULIAN@ 14 FEB 1689', calendar: 'DJULIAN' })
		).toBe('14 FEB 1689 (Julian)');
	});

	it('annotates Hebrew dates', () => {
		expect(
			formatGenDate({ raw: '@#DHEBREW@ 15 NSN 5785', calendar: 'DHEBREW' })
		).toBe('15 NSN 5785 (Hebrew)');
	});

	it('leaves Gregorian raw dates unchanged', () => {
		expect(formatGenDate({ raw: '25 DEC 2020' })).toBe('25 DEC 2020');
	});

	it('annotates the calendar on structured dates without raw', () => {
		expect(formatGenDate({ calendar: 'DJULIAN', year: 1689 })).toBe('1689 (Julian)');
	});

	it('derives the label from the raw escape when calendar is missing', () => {
		expect(formatGenDate({ raw: '@#DJULIAN@ 14 FEB 1689' })).toBe('14 FEB 1689 (Julian)');
	});

	it('labels a single-calendar range as a whole', () => {
		expect(
			formatGenDate({ raw: '@#DJULIAN@ BET 1690 AND 1700', calendar: 'DJULIAN' })
		).toBe('BET 1690 AND 1700 (Julian)');
		expect(formatGenDate({ raw: 'FROM @#DJULIAN@ 1690 TO @#DJULIAN@ 1700' })).toBe(
			'FROM 1690 TO 1700 (Julian)'
		);
	});

	it('labels each endpoint of a mixed-calendar range in its own calendar', () => {
		expect(
			formatGenDate({ raw: 'BET @#DJULIAN@ 1690 AND @#DGREGORIAN@ 1700', calendar: 'DJULIAN' })
		).toBe('BET 1690 (Julian) AND 1700');
		expect(formatGenDate({ raw: 'BET 1740 AND @#DHEBREW@ 5500' })).toBe(
			'BET 1740 AND 5500 (Hebrew)'
		);
		expect(
			formatGenDate({ raw: 'from @#DJULIAN@ 1700 to @#DFRENCH R@ 2', calendar: 'DJULIAN' })
		).toBe('from 1700 (Julian) to 2 (French Republican)');
	});

	it('labels the start of a mixed range by its own escape, not a stale calendar', () => {
		// A date stored before #1227 could carry the end's calendar as the whole
		// date's calendar.
		expect(formatGenDate({ raw: 'BET 1740 AND @#DHEBREW@ 5500', calendar: 'DHEBREW' })).toBe(
			'BET 1740 AND 5500 (Hebrew)'
		);
	});
});
