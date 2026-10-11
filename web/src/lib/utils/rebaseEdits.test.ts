import { describe, it, expect } from 'vitest';
import { rebaseEdits } from './rebaseEdits';

describe('rebaseEdits', () => {
	const base = { notes: 'old', place: 'London', date: '1815' };

	it('keeps the fields the user changed and takes the latest for the rest', () => {
		const form = { ...base, notes: 'mine' };
		const latest = { notes: 'old', place: 'Westminster', date: '1816' };
		expect(rebaseEdits(form, base, latest)).toEqual({ notes: 'mine', place: 'Westminster', date: '1816' });
	});

	it('keeps a field the user cleared', () => {
		const form = { ...base, place: '' };
		const latest = { ...base, place: 'Westminster' };
		expect(rebaseEdits(form, base, latest).place).toBe('');
	});
});
