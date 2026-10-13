/**
 * No fixed-column grids in components (#930).
 *
 * Grids use `repeat(auto-fit, minmax(min(<width>, 100%), 1fr))` so they reflow
 * at any width (CONVENTIONS.md, "Responsive"). A fixed `repeat(N, 1fr)` only
 * stacks where a media query says so. A grid whose column count truly must be
 * fixed says why in a comment containing `fixed columns:` on the same line or
 * the line above.
 */
import { describe, expect, it } from 'vitest';

const svelteSources = import.meta.glob('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

const FIXED_GRID = /repeat\(\s*\d+\s*,\s*1fr\s*\)/;
const EXEMPTION = /fixed columns:/i;

/** Fixed `repeat(N, 1fr)` grids in one Svelte source, as `line N: text` findings. */
export function fixedGrids(source: string): string[] {
	const lines = source.split('\n');
	return lines.flatMap((line, i) =>
		FIXED_GRID.test(line) && !EXEMPTION.test(line) && !EXEMPTION.test(lines[i - 1] ?? '')
			? [`line ${i + 1}: ${line.trim()}`]
			: []
	);
}

describe('fixedGrids', () => {
	it.each([
		['a fixed two-column grid', '.g {\n\tgrid-template-columns: repeat(2, 1fr);\n}'],
		['a fixed grid without spaces', '.g { grid-template-columns: repeat(3,1fr); }'],
		['a Tailwind arbitrary value', '<div class="grid grid-cols-[repeat(4, 1fr)]"></div>']
	])('flags %s', (_name, source) => {
		expect(fixedGrids(source)).toHaveLength(1);
	});

	it.each([
		['auto-fit', '.g { grid-template-columns: repeat(auto-fit, minmax(min(14rem, 100%), 1fr)); }'],
		['auto-fill', '.g { grid-template-columns: repeat(auto-fill, minmax(min(150px, 100%), 1fr)); }'],
		[
			'an explained fixed grid',
			'.g {\n\t/* fixed columns: one per weekday */\n\tgrid-template-columns: repeat(7, 1fr);\n}'
		]
	])('allows %s', (_name, source) => {
		expect(fixedGrids(source)).toEqual([]);
	});
});

describe('component grids', () => {
	it('use auto-fit/minmax rather than a fixed repeat(N, 1fr)', () => {
		const offenders = Object.entries(svelteSources).flatMap(([file, source]) =>
			fixedGrids(source).map((finding) => `${file}: ${finding}`)
		);
		expect(offenders).toEqual([]);
	});
});
