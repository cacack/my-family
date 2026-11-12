/**
 * No hand-built loading markup (#1115).
 *
 * A page shows its load in progress with `LoadingState` (CONVENTIONS.md, "UI
 * Building Blocks"), which fixes the spacing and type and announces itself via
 * `aria-live`. This fails a `div`, `p` or `span` whose text starts with
 * "Loading", and a `loading`, `loading-*` class, in any route or component
 * other than `LoadingState` itself.
 *
 * A busy button's own label (`{saving ? 'Saving...' : 'Save'}`) is not loading
 * markup and is not matched. The components in `NOT_PAGE_LOADERS` load one
 * section inside a page that is already showing, or spin inside a search
 * input, rather than load the page itself.
 */
import { describe, expect, it } from 'vitest';

const svelteSources = import.meta.glob('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

const SHARED = '/src/lib/components/LoadingState.svelte';

const NOT_PAGE_LOADERS = new Set([
	// A section of a page.
	'/src/lib/components/CitationSection.svelte',
	'/src/lib/components/CitationTemplateForm.svelte',
	'/src/lib/components/DiscoveryFeed.svelte',
	'/src/lib/components/EvidencePanel.svelte',
	'/src/lib/components/MediaGallery.svelte',
	'/src/lib/components/NameSection.svelte',
	'/src/lib/components/RestorePointBrowser.svelte',
	'/src/lib/components/branch/BranchResearchEditor.svelte',
	// A spinner inside a search input.
	'/src/lib/components/PersonSelector.svelte',
	'/src/lib/components/SearchBox.svelte'
]);

const LOADING_TEXT = /<(?:div|p|span)\b[^>]*>\s*Loading\b/g;
const LOADING_CLASS = /<[a-z][\w-]*\b[^>]*\bclass="(?:[^"]*\s)?loading[\w-]*(?:\s[^"]*)?"/g;

/** Ad hoc loading markup in one component's template, as `line N: text` findings. */
export function adHocLoading(source: string): string[] {
	// Blank out script, style and comments, keeping newlines so line numbers hold.
	const markup = source.replace(
		/<script\b[\s\S]*?<\/script\s*>|<style\b[\s\S]*?<\/style\s*>|<!--[\s\S]*?-->/gi,
		(block) => block.replace(/[^\n]/g, ' ')
	);
	const lines = markup.split('\n');
	return [...markup.matchAll(LOADING_TEXT), ...markup.matchAll(LOADING_CLASS)].map((m) => {
		const line = markup.slice(0, m.index).split('\n').length;
		return `line ${line}: ${lines[line - 1].trim()}`;
	});
}

describe('adHocLoading', () => {
	it.each([
		['a loading div', '<div class="p-12 text-center">Loading...</div>'],
		['a loading paragraph', '<p class="hint" role="status">Loading proof summaries...</p>'],
		['text on its own line', '<div class="state" role="status">\n\tLoading media...\n</div>'],
		['a loading class', '<div class="loading"></div>'],
		['a loading-* class', '<div class="loading-container"></div>'],
		['a loading class among others', '<div class="card loading big"></div>']
	])('flags %s', (_name, source) => {
		expect(adHocLoading(source)).toHaveLength(1);
	});

	it.each([
		['the shared component', '<LoadingState label="Loading people..." />'],
		['a busy button label', "<Button>{saving ? 'Saving...' : 'Save'}</Button>"],
		['a load-more label', "<button>{loadingMore ? 'Loading...' : 'Load more'}</button>"],
		['a class that only contains the word', '<div class="preloading"></div>'],
		['a comment', '<!-- <div>Loading...</div> -->'],
		['a style block', '<style>\n\t.loading { color: red; }\n</style>']
	])('allows %s', (_name, source) => {
		expect(adHocLoading(source)).toEqual([]);
	});
});

describe('loading states', () => {
	it('use LoadingState rather than hand-built markup', () => {
		const offenders = Object.entries(svelteSources)
			.filter(([file]) => file !== SHARED && !NOT_PAGE_LOADERS.has(file))
			.flatMap(([file, source]) => adHocLoading(source).map((finding) => `${file}: ${finding}`));
		expect(offenders).toEqual([]);
	});

	it('exempt only components that still have loading markup', () => {
		const stale = [...NOT_PAGE_LOADERS].filter(
			(file) => !svelteSources[file] || adHocLoading(svelteSources[file]).length === 0
		);
		expect(stale).toEqual([]);
	});
});
