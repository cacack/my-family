import { describe, it, expect, vi, beforeEach } from 'vitest';
import { goto } from '$app/navigation';
import {
	clampPageToTotal,
	readChartOptions,
	readEnum,
	readFlag,
	readPositiveInt,
	setQuery,
	withQuery
} from './urlState';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

const url = (path: string) => new URL(path, 'http://localhost');

describe('withQuery', () => {
	it.each([
		['sets a new param', '/persons', { sort: 'birth_date' }, {}, '/persons?sort=birth_date'],
		['keeps params it does not touch', '/persons?sort=birth_date', { page: 3 }, {}, '/persons?sort=birth_date&page=3'],
		['drops a value equal to its default', '/persons?page=3', { page: 1 }, { page: 1 }, '/persons'],
		['drops empty strings, null and undefined', '/search?q=a&b=1&c=2', { q: '', b: null, c: undefined }, {}, '/search'],
		['writes true as 1 and drops false', '/search?soundex=1', { fuzzy: true, soundex: false }, {}, '/search?fuzzy=1'],
		['encodes values', '/search', { q: 'Smith & Co' }, {}, '/search?q=Smith+%26+Co']
	])('%s', (_name, from, updates, defaults, want) => {
		expect(withQuery(url(from), updates, defaults)).toBe(want);
	});
});

describe('setQuery', () => {
	beforeEach(() => vi.mocked(goto).mockClear());

	it('replaces by default, keeping focus and scroll', async () => {
		await setQuery(url('/persons'), { sort: 'birth_date' });
		expect(goto).toHaveBeenCalledWith('/persons?sort=birth_date', {
			replaceState: true,
			keepFocus: true,
			noScroll: true
		});
	});

	it('pushes when asked, applying defaults', async () => {
		await setQuery(url('/persons?page=2'), { page: 1 }, { push: true, defaults: { page: 1 } });
		expect(goto).toHaveBeenCalledWith('/persons', {
			replaceState: false,
			keepFocus: true,
			noScroll: true
		});
	});
});

describe('clampPageToTotal', () => {
	beforeEach(() => vi.mocked(goto).mockClear());

	it.each([
		['moves a page past the end to the last page', '/persons?sort=surname&page=9', 9, 45, '/persons?sort=surname&page=3'],
		['drops the page param when the last page is 1', '/persons?page=4', 4, 5, '/persons'],
		['drops the page param when there are no items', '/persons?page=2', 2, 0, '/persons']
	])('%s, replacing the URL', (_name, from, requested, total, want) => {
		expect(clampPageToTotal(url(from), requested, total, 20)).toBe(true);
		expect(goto).toHaveBeenCalledWith(want, { replaceState: true, keepFocus: true, noScroll: true });
	});

	it.each([
		['the last page', 3, 45],
		['page 1 of an empty list', 1, 0],
		['a page inside the list', 2, 100]
	])('leaves %s alone', (_name, requested, total) => {
		expect(clampPageToTotal(url('/persons'), requested, total, 20)).toBe(false);
		expect(goto).not.toHaveBeenCalled();
	});
});

describe('readers', () => {
	const params = new URLSearchParams(
		'sort=birth_date&bad=nope&page=3&zero=0&neg=-2&frac=1.5&on=1&off=0&no=false&gens=7&layout=wide'
	);
	const SORTS = ['surname', 'birth_date'] as const;

	it('readEnum accepts allowed values and falls back otherwise', () => {
		expect(readEnum(params, 'sort', SORTS, 'surname')).toBe('birth_date');
		expect(readEnum(params, 'bad', SORTS, 'surname')).toBe('surname');
		expect(readEnum(params, 'missing', SORTS, 'surname')).toBe('surname');
	});

	it.each([
		['page', 3],
		['zero', 1],
		['neg', 1],
		['frac', 1],
		['bad', 1],
		['missing', 1]
	])('readPositiveInt(%s) is %d', (key, want) => {
		expect(readPositiveInt(params, key, 1)).toBe(want);
	});

	it('readPositiveInt caps a value at max', () => {
		expect(readPositiveInt(params, 'page', 1, 2)).toBe(2);
	});

	it.each([
		['on', true],
		['off', false],
		['no', false],
		['missing', false]
	])('readFlag(%s) is %s', (key, want) => {
		expect(readFlag(params, key)).toBe(want);
	});

	it('readChartOptions reads generations and layout', () => {
		expect(readChartOptions(params)).toEqual({ generations: 7, layout: 'wide' });
	});

	it('readChartOptions falls back to 4 generations, compact, on bad input', () => {
		expect(readChartOptions(new URLSearchParams('gens=99&layout=huge'))).toEqual({
			generations: 4,
			layout: 'compact'
		});
	});
});
