/**
 * Non-text contrast (WCAG 1.4.11, #965).
 *
 * axe's color-contrast rule only checks text, so icons, chevrons and focus
 * outlines need their own guard. Two checks: the non-text tokens in app.css
 * reach 3:1 on every light surface, and no component colours an icon or a
 * focus outline with a literal below 3:1 on white, the lightest surface
 * (CONVENTIONS.md, "Non-text colour"). Only component `<style>` blocks are
 * read; Tailwind utility classes and inline `style=` attributes are not.
 *
 * Empty-state illustrations are decorative - the message beside them carries
 * the meaning - so rules whose selector names `empty-state` are exempt. A
 * colour on a dark surface is listed in DARK_SURFACE with its measurement.
 */
import { describe, expect, it } from 'vitest';
// Needs the `test.css.include` entry in vite.config.ts, or Vitest blanks it.
import appCss from '../app.css?raw';

const svelteSources = import.meta.glob('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

/** WCAG contrast ratio of two #rrggbb colours. */
export function contrastRatio(a: string, b: string): number {
	const luminance = (hex: string) => {
		const full = hex.length === 4 ? hex.replace(/[0-9a-f]/gi, (c) => c + c) : hex;
		const [r, g, b] = [1, 3, 5].map((i) => {
			const v = parseInt(full.slice(i, i + 2), 16) / 255;
			return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
		});
		return 0.2126 * r + 0.7152 * g + 0.0722 * b;
	};
	const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (hi + 0.05) / (lo + 0.05);
}

/** The value of `token` in the first rule whose selector is `selector`. */
function tokenValue(selector: string, token: string): string {
	const start = appCss.indexOf(`\n${selector} {`);
	const body = appCss.slice(start, appCss.indexOf('}', start));
	const value = body.match(new RegExp(`${token}:\\s*(#[0-9a-f]{3,6})\\b`, 'i'))?.[1];
	expect(value, `${selector} declares no hex ${token}`).toBeDefined();
	return value!;
}

// The light surfaces the app.css token comment lists; #e2e8f0 and #dbeafe are the darkest.
const LIGHT_SURFACES = ['#ffffff', '#f8fafc', '#f1f5f9', '#e2e8f0', '#dbeafe'];
const HIGH_CONTRAST_SURFACES = ['#ffffff', '#f0f0f0'];
const NON_TEXT_TOKENS = ['--color-focus-ring', '--color-success', '--color-text-muted'];

describe('non-text tokens', () => {
	it.each(NON_TEXT_TOKENS)('%s reaches 3:1 on every light surface', (token) => {
		const colour = tokenValue(':root', token);
		for (const surface of LIGHT_SURFACES) {
			expect(contrastRatio(colour, surface), `${colour} on ${surface}`).toBeGreaterThanOrEqual(3);
		}
	});

	it.each(NON_TEXT_TOKENS)('%s reaches 3:1 in high contrast', (token) => {
		const colour = tokenValue('body.high-contrast', token);
		for (const surface of HIGH_CONTRAST_SURFACES) {
			expect(contrastRatio(colour, surface), `${colour} on ${surface}`).toBeGreaterThanOrEqual(3);
		}
	});
});

const ICON_SELECTOR = /\bsvg\b|icon|chevron/;
const FOCUS_SELECTOR = /focus/;

/** Icon and focus-outline colours in one Svelte source that fall below 3:1 on white. */
export function lowContrastNonText(source: string): string[] {
	const styles = [...source.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]);
	const rules = styles.flatMap((css) =>
		[...css.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)].map(
			([, selector, body]) => ({ selector: selector.trim().replace(/\s+/g, ' '), body })
		)
	);
	return rules
		.filter(({ selector }) => !selector.includes('empty-state'))
		.flatMap(({ selector, body }) => {
			const properties = [
				...(ICON_SELECTOR.test(selector) ? ['color', 'fill', 'stroke'] : []),
				...(FOCUS_SELECTOR.test(selector) ? ['outline', 'outline-color'] : [])
			];
			if (properties.length === 0) return [];
			return [...body.matchAll(/(?<![\w-])([\w-]+)\s*:\s*([^;]+)/g)]
				.filter(([, property]) => properties.includes(property))
				.filter(([, , value]) => {
					// An hsl() wrapper around an oklch shadcn token is invalid, so the outline vanishes.
					if (/hsl\(\s*var\(/.test(value)) return true;
					const hex = value.match(/^[^(]*?(#[0-9a-f]{3,6})\b/i)?.[1];
					return hex !== undefined && contrastRatio(hex, '#ffffff') < 3;
				})
				.map(([, property, value]) => `${selector} { ${property}: ${value.trim()} }`);
		});
}

describe('lowContrastNonText', () => {
	it.each([
		['a pale icon', '<style>.search-icon { color: #94a3b8; }</style>'],
		['a pale svg stroke', '<style>.row svg { stroke: #cbd5e1; }</style>'],
		['a pale chevron', '<style>.chevron { color: #22c55e; }</style>'],
		['a pale focus outline', '<style>.x:focus-visible { outline: 2px solid #93c5fd; }</style>'],
		['an hsl-wrapped shadcn token', '<style>a:focus-visible { outline: 2px solid hsl(var(--primary)); }</style>']
	])('flags %s', (_name, source) => {
		expect(lowContrastNonText(source)).toHaveLength(1);
	});

	it.each([
		['a token', '<style>.search-icon { color: var(--color-text-muted); }</style>'],
		['a dark enough icon', '<style>.order-btn svg { color: #64748b; }</style>'],
		['the focus-ring token', '<style>.x:focus { outline: 2px solid var(--color-focus-ring); }</style>'],
		['a decorative empty state', '<style>.empty-state svg { color: #cbd5e1; }</style>'],
		['a non-icon rule', '<style>.card { color: #94a3b8; background: #cbd5e1; }</style>']
	])('allows %s', (_name, source) => {
		expect(lowContrastNonText(source)).toEqual([]);
	});
});

// Findings that sit on a dark surface, where a light colour is the one that passes.
const DARK_SURFACE = new Set([
	// The lightbox is bg-black/95: #94a3b8 measures 7.6:1 there.
	'/src/lib/components/MediaLightbox.svelte: .document-icon { color: #94a3b8 }'
]);

describe('component icons and focus outlines', () => {
	it('reach 3:1 on white', () => {
		const offenders = Object.entries(svelteSources)
			.flatMap(([file, source]) => lowContrastNonText(source).map((finding) => `${file}: ${finding}`))
			.filter((finding) => !DARK_SURFACE.has(finding));
		expect(offenders).toEqual([]);
	});
});
