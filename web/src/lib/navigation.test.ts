import { describe, it, expect } from 'vitest';
import { mainNav, isGroup, isEntryActive, isLinkActive, type NavEntry } from './navigation';

function entry(label: string): NavEntry {
	const found = mainNav.find((e) => e.label === label);
	if (!found) throw new Error(`no nav entry ${label}`);
	return found;
}

/** Every page in the app, mapped to the top-level entry it belongs to. */
const sections: [string, string][] = [
	['/persons', 'People'],
	['/persons/add', 'People'],
	['/persons/abc', 'People'],
	['/pedigree/abc', 'People'],
	['/descendancy/abc', 'People'],
	['/ahnentafel/abc', 'People'],
	['/families', 'Families'],
	['/families/abc/group-sheet', 'Families'],
	['/browse/surnames/Smith', 'Browse'],
	['/browse/places/Ohio', 'Browse'],
	['/browse/cemeteries/Oak%20Hill', 'Browse'],
	['/map', 'Browse'],
	['/relationship', 'Browse'],
	['/sources/abc', 'Research'],
	['/evidence/conflicts/abc', 'Research'],
	['/repositories/abc', 'Research'],
	['/browse/citation-templates', 'Research'],
	['/browse/brick-walls', 'Research'],
	['/quality/merge/a/b', 'Research'],
	['/analytics', 'Research'],
	['/history', 'History'],
	['/branches/abc/research', 'History'],
	['/snapshots/compare', 'History'],
	['/import', 'Import']
];

describe('main navigation', () => {
	it.each(sections)('%s highlights exactly %s', (pathname, label) => {
		const active = mainNav.filter((e) => isEntryActive(e, pathname)).map((e) => e.label);
		expect(active).toEqual([label]);
	});

	it.each(['/', '/search'])('%s highlights nothing', (pathname) => {
		expect(mainNav.filter((e) => isEntryActive(e, pathname))).toEqual([]);
	});

	it('matches whole path segments only', () => {
		expect(isLinkActive({ href: '/map', label: 'Map' }, '/mapping')).toBe(false);
	});

	it('marks only the current link inside an active group', () => {
		const research = entry('Research');
		if (!isGroup(research)) throw new Error('Research is a group');
		const active = research.links.filter((l) => isLinkActive(l, '/sources/abc'));
		expect(active.map((l) => l.label)).toEqual(['Sources']);
	});

	it('keeps the top level short enough to fit on one row', () => {
		expect(mainNav.length).toBeLessThanOrEqual(6);
	});

	it('offers each destination once', () => {
		const hrefs = mainNav.flatMap((e) => (isGroup(e) ? e.links : [e])).map((l) => l.href);
		expect(new Set(hrefs).size).toBe(hrefs.length);
	});
});
