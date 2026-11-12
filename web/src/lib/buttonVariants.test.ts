/**
 * Button variants follow the documented set (#1114).
 *
 * `default` is a page's main action, `outline` a secondary action, and a
 * Cancel button is `outline` (CONVENTIONS.md, "Buttons"). shadcn's `secondary`
 * variant is not in that set, so no `<Button>` outside `ui/` may use it, and a
 * Cancel must say `variant="outline"`.
 */
import { describe, expect, it } from 'vitest';

const svelteSources = import.meta.glob('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

interface ButtonTag {
	line: number;
	attrs: string;
	label: string;
}

/**
 * Every `<Button ...>label</Button>` in a source. The opening tag ends at the
 * first `>` outside braces and quotes, since handlers such as
 * `onclick={() => (open = false)}` contain one.
 */
export function buttonTags(source: string): ButtonTag[] {
	const tags: ButtonTag[] = [];
	const open = /<Button\b/g;
	let match: RegExpExecArray | null;
	while ((match = open.exec(source))) {
		let depth = 0;
		let quote: string | null = null;
		let i = match.index + match[0].length;
		for (; i < source.length; i++) {
			const c = source[i];
			if (quote) {
				if (c === quote) quote = null;
			} else if (c === '"' || c === "'" || c === '`') quote = c;
			else if (c === '{') depth++;
			else if (c === '}') depth--;
			else if (c === '>' && depth === 0) break;
		}
		const attrs = source.slice(match.index + match[0].length, i);
		const close = source.indexOf('</Button>', i);
		const label =
			attrs.trimEnd().endsWith('/') || close < 0 ? '' : source.slice(i + 1, close).trim();
		tags.push({ line: source.slice(0, match.index).split('\n').length, attrs, label });
	}
	return tags;
}

/** Buttons that break the variant rules, as `line N: reason` findings. */
export function variantFindings(source: string): string[] {
	return buttonTags(source).flatMap(({ line, attrs, label }) => {
		if (/variant="secondary"/.test(attrs)) return [`line ${line}: variant="secondary"`];
		if (label === 'Cancel' && !/variant="outline"/.test(attrs))
			return [`line ${line}: Cancel is not variant="outline"`];
		return [];
	});
}

describe('variantFindings', () => {
	it.each([
		['a secondary button', '<Button variant="secondary" onclick={go}>Clear</Button>'],
		[
			'a secondary Cancel over several lines',
			'<Button\n\ttype="button"\n\tvariant="secondary"\n\tonclick={() => (open = false)}\n>\n\tCancel\n</Button>'
		],
		['a default Cancel', '<Button onclick={() => (open = false)}>Cancel</Button>'],
		['a ghost Cancel', '<Button variant="ghost" onclick={close}>Cancel</Button>']
	])('flags %s', (_name, source) => {
		expect(variantFindings(source)).toHaveLength(1);
	});

	it.each([
		[
			'an outline Cancel',
			'<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>'
		],
		['a default main action', '<Button href="/persons/add">Add Person</Button>'],
		['a secondary Badge', '<Badge variant="secondary">Current</Badge>']
	])('allows %s', (_name, source) => {
		expect(variantFindings(source)).toEqual([]);
	});
});

describe('component buttons', () => {
	it('use the documented variants, with Cancel as outline', () => {
		const offenders = Object.entries(svelteSources)
			.filter(([file]) => !file.includes('/lib/components/ui/'))
			.flatMap(([file, source]) =>
				variantFindings(source).map((finding) => `${file}: ${finding}`)
			);
		expect(offenders).toEqual([]);
	});
});
