/**
 * High contrast is defined once, in app.css, and covers both token sets (#905).
 *
 * Two contradictory definitions (a dark one in the layout, a light one in
 * app.css) once left text invisible, and the shadcn tokens were never switched
 * at all. These checks keep it to a single definition that switches every
 * colour token the light theme defines.
 */
import postcss, { type Declaration, type Rule } from 'postcss';
import { describe, expect, it } from 'vitest';
// Needs the `test.css.include` entry in vite.config.ts, or Vitest blanks it.
import appCss from '../app.css?raw';

const svelteSources = import.meta.glob('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

/**
 * Custom properties declared directly in the first top-level rule whose
 * selector is `selector`. Parsed, so a nested block cannot cut the list short.
 */
function declaredTokens(css: string, selector: string): string[] {
	const rule = postcss
		.parse(css)
		.nodes.find((node): node is Rule => node.type === 'rule' && node.selector === selector);
	expect(rule, `no "${selector}" rule in app.css`).toBeDefined();
	return (rule?.nodes ?? [])
		.filter((node): node is Declaration => node.type === 'decl' && node.prop.startsWith('--'))
		.map((decl) => decl.prop);
}

/** Whether `source` sets a custom property in a rule whose selector names `body.high-contrast`. */
function redefinesHighContrast(source: string): boolean {
	return [...source.matchAll(/body\.high-contrast\)?[^{]*\{([^}]*)\}/g)].some((m) =>
		/--[\w-]+\s*:/.test(m[1])
	);
}

// Tokens that are not colours, or are data-series colours charts own.
const NOT_THEME_COLOURS = /^--(font-size-|transition-|radius$|chart-)/;

describe('high-contrast definition', () => {
	it('overrides every colour token the light theme defines, shadcn ones included', () => {
		const themed = declaredTokens(appCss, ':root').filter((t) => !NOT_THEME_COLOURS.test(t));
		const highContrast = new Set(declaredTokens(appCss, 'body.high-contrast'));

		expect(themed).toEqual(
			expect.arrayContaining(['--background', '--foreground', '--primary', '--border'])
		);
		expect(themed.filter((t) => !highContrast.has(t))).toEqual([]);
	});

	it('reads every top-level declaration, past nested blocks', () => {
		const css = `:root {
	--a: 1;
	.x { --nested: 2; }
	--b: 3;
}
@media (min-width: 1px) {
	:root { --in-media: 4; }
}`;
		expect(declaredTokens(css, ':root')).toEqual(['--a', '--b']);
	});

	it('styles links outside any cascade layer, so component-scoped link rules cannot beat it', () => {
		// Unlayered styles beat layered ones whatever their specificity (#961).
		const selector = 'body.high-contrast a:not([data-slot]) {';
		const at = appCss.indexOf(selector);
		expect(at, `no "${selector}" rule in app.css`).toBeGreaterThanOrEqual(0);
		const before = appCss.slice(0, at).replace(/\/\*[\s\S]*?\*\//g, '');
		const depth = (before.match(/\{/g) ?? []).length - (before.match(/\}/g) ?? []).length;
		expect(depth).toBe(0);
	});

	it('exempts component links only through data-slot, from the documented set', () => {
		// app.css reads only the presence of `data-slot`; these values say why a
		// link outside shadcn's own parts (src/lib/components/ui) is exempt.
		const allowed = new Set(['card-link', 'action-link', 'nav-link', 'chip']);
		// A class naming a card, button, chip or tile marks a component link.
		const componentClass = /(^|[\s-])(card|btn|button|chip|tile)([\s-]|$)/;
		const openTag = /<a(?=[\s>])((?:[^>{]|\{[^}]*\})*)>/g;
		const offenders = Object.entries(svelteSources)
			.filter(([file]) => !file.includes('/lib/components/ui/'))
			.flatMap(([file, source]) =>
				[...source.matchAll(openTag)].flatMap(([tag, attrs]) => {
					const slot = attrs.match(/data-slot="([^"]*)"/)?.[1];
					const cls = attrs.match(/class=["{]([^"}]*)/)?.[1] ?? '';
					const untagged = slot === undefined && componentClass.test(cls);
					const unknown = slot !== undefined && !allowed.has(slot);
					return untagged || unknown ? [`${file}: ${tag}`] : [];
				})
			);
		expect(offenders).toEqual([]);
	});

	it('reports a component that sets a token under body.high-contrast', () => {
		const offending = `<style>
	:global(body.high-contrast) .card { color: var(--color-text); }
	:global(body.high-contrast) {
		--color-text: #000;
	}
</style>`;
		const harmless = `<style>
	:global(body.high-contrast) .card { color: var(--color-text); }
</style>`;
		expect(redefinesHighContrast(offending)).toBe(true);
		expect(redefinesHighContrast(harmless)).toBe(false);
	});

	it('is not redefined by any component', () => {
		const offenders = Object.entries(svelteSources)
			.filter(([, source]) => redefinesHighContrast(source))
			.map(([file]) => file);
		expect(offenders).toEqual([]);
	});
});
