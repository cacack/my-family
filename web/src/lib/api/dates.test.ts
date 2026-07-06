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
});
