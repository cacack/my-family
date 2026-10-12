/**
 * No sub-AA text colours in components (#964).
 *
 * The axe guard (e2e/a11y.spec.ts) only sees routes as they first load, so
 * low-contrast text in an interaction state slips past it. This static check
 * fails the Tailwind classes and slate hex values that fall below 4.5:1 on the
 * app's light surfaces when they colour text. Use `text-muted-foreground` or
 * `var(--color-text-muted)` instead (CONVENTIONS.md, "Muted and accent colour").
 *
 * Icons stay allowed: a CSS rule whose selector names an `svg` or an `*icon*`
 * class may use the hex values. `dark:` variants are allowed too; they apply on
 * dark surfaces, where these colours pass.
 */
import { describe, expect, it } from 'vitest';

const svelteSources = import.meta.glob('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

const BANNED_CLASS =
	/(?<![\w-])((?:[\w-]+:)*)text-(?:slate-(?:300|400|500)|gray-(?:400|500))(?![\w-])/g;
const BANNED_HEX_COLOR = /(?<![\w-])color\s*:\s*(#94a3b8|#cbd5e1)\b/gi;
const ICON_SELECTOR = /\bsvg\b|icon/;

/** Banned text colours in one Svelte source, as human-readable findings. */
export function bannedTextColours(source: string): string[] {
	const classes = [...source.matchAll(BANNED_CLASS)]
		.filter(([, variants]) => !variants.split(':').includes('dark'))
		.map(([match]) => `class ${match}`);

	const styles = [...source.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]);
	const rules = styles.flatMap((css) =>
		[...css.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)].map(
			([, selector, body]) => ({ selector: selector.trim(), body })
		)
	);
	const declarations = rules
		.filter(({ selector }) => !ICON_SELECTOR.test(selector))
		.flatMap(({ selector, body }) =>
			[...body.matchAll(BANNED_HEX_COLOR)].map(([, hex]) => `${selector} { color: ${hex} }`)
		);

	return [...classes, ...declarations];
}

describe('bannedTextColours', () => {
	it.each([
		['a Tailwind text class', '<p class="text-slate-400">x</p>'],
		['a variant-prefixed text class', '<a class="hover:text-gray-500">x</a>'],
		['a class built in script', "<script>const c = 'bg-white text-slate-300';</script>"],
		['a hex text colour', '<style>\n.hint {\n\tcolor: #94a3b8;\n}\n</style>'],
		['a hex text colour inside @media', '<style>@media (min-width: 1px) { .sep { color: #CBD5E1; } }</style>']
	])('flags %s', (_name, source) => {
		expect(bannedTextColours(source)).toHaveLength(1);
	});

	it.each([
		['an icon class', '<style>.search-icon { color: #94a3b8; }</style>'],
		['an svg', '<style>.empty svg { color: #cbd5e1; }</style>'],
		['a border or background', '<style>.x { border-color: #cbd5e1; background-color: #94a3b8; }</style>'],
		['non-text utilities', '<div class="bg-slate-400 border-gray-500 text-slate-600"></div>'],
		['a dark variant', '<span class="dark:text-gray-400"></span>'],
		['the muted token', '<style>.hint { color: var(--color-text-muted); }</style>']
	])('allows %s', (_name, source) => {
		expect(bannedTextColours(source)).toEqual([]);
	});
});

describe('component text colours', () => {
	it('use no sub-AA slate or gray text colour', () => {
		const offenders = Object.entries(svelteSources).flatMap(([file, source]) =>
			bannedTextColours(source).map((finding) => `${file}: ${finding}`)
		);
		expect(offenders).toEqual([]);
	});
});
