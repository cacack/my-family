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

describe('rebaseEdits with lists', () => {
	it('treats a list with the same items as not edited', () => {
		const base = { ids: ['a', 'b'], notes: '' };
		const latest = { ids: ['a', 'b', 'c'], notes: '' };
		expect(rebaseEdits({ ids: ['a', 'b'], notes: 'mine' }, base, latest)).toEqual({
			ids: ['a', 'b', 'c'],
			notes: 'mine'
		});
	});

	it('keeps a list the user changed', () => {
		const base = { ids: ['a', 'b'] };
		expect(rebaseEdits({ ids: ['b', 'a'] }, base, { ids: ['c'] }).ids).toEqual(['b', 'a']);
	});
});

describe('rebaseEdits with keys that go together', () => {
	const base = { lat: '42.3601', long: '-71.0589', notes: '' };
	const latest = { lat: '-33.8688', long: '151.2093', notes: 'theirs' };
	const pair: (keyof typeof base)[][] = [['lat', 'long']];

	it('keeps the whole pair when one half was edited', () => {
		expect(rebaseEdits({ ...base, lat: '42.3602' }, base, latest, pair)).toEqual({
			lat: '42.3602',
			long: '-71.0589',
			notes: 'theirs'
		});
	});

	it('takes the whole latest pair when neither half was edited', () => {
		expect(rebaseEdits({ ...base, notes: 'mine' }, base, latest, pair)).toEqual({
			lat: '-33.8688',
			long: '151.2093',
			notes: 'mine'
		});
	});
});
