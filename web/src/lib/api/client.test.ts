import { describe, it, expect } from 'vitest';
import { formatGenDate, type GenDate } from './client';

describe('formatGenDate', () => {
	it('returns the raw string verbatim when present', () => {
		const date: GenDate = {
			raw: 'INT 1850 (about eighteen fifty)',
			qualifier: 'int',
			year: 1850,
			interpreted_from: 'about eighteen fifty'
		};
		expect(formatGenDate(date)).toBe('INT 1850 (about eighteen fifty)');
	});

	it('formats an interpreted date with its original phrase when raw is absent', () => {
		const date: GenDate = {
			qualifier: 'int',
			year: 1850,
			interpreted_from: 'about eighteen fifty'
		};
		expect(formatGenDate(date)).toBe('INT 1850 (about eighteen fifty)');
	});

	it('formats an interpreted date without a phrase', () => {
		const date: GenDate = { qualifier: 'int', year: 1850 };
		expect(formatGenDate(date)).toBe('INT 1850');
	});
});
