/**
 * A stand-in for SvelteKit's router in component tests of pages that keep
 * state in the URL (see `#lib/utils/urlState.js`). `goto` moves a fake history
 * and updates the reactive `page`, so a test can drive a page, read its URL and
 * step Back. Wire it up with:
 *
 *   vi.mock('$app/state', async () => (await import('#lib/test/fakeRouter.svelte.js')).appState);
 *   vi.mock('$app/navigation', async () => (await import('#lib/test/fakeRouter.svelte.js')).appNavigation);
 *
 * and call `resetRouter(path)` before each render.
 */
import { vi } from 'vitest';

interface FakePage {
	url: URL;
	params: Record<string, string>;
	state: Record<string, unknown>;
}

const ORIGIN = 'http://localhost';

let entries: string[] = ['/'];
let index = 0;
let params: Record<string, string> = {};

let current = $state.raw<FakePage>({ url: new URL('/', ORIGIN), params, state: {} });

/** Read-only and reactive, like `page` from `$app/state`. */
const page = {
	get url() {
		return current.url;
	},
	get params() {
		return current.params;
	},
	get state() {
		return current.state;
	}
};

function show() {
	current = { url: new URL(entries[index], ORIGIN), params, state: {} };
}

/** Starts a fresh history at `path`, with the route `routeParams`. */
export function resetRouter(path: string, routeParams: Record<string, string> = {}) {
	entries = [path];
	index = 0;
	params = routeParams;
	show();
}

const goto = vi.fn(async (href: string, opts: { replace?: boolean } = {}) => {
	if (opts.replace) {
		entries[index] = href;
	} else {
		entries = [...entries.slice(0, index + 1), href];
		index++;
	}
	show();
});

/** The browser's Back button. */
export function back() {
	if (index === 0) throw new Error('fakeRouter: no history entry to go back to');
	index--;
	show();
}

/** The current entry's `pathname?search`. */
export function currentPath(): string {
	return entries[index];
}

/** How many history entries exist (pushes add one, replaces don't). */
export function historyLength(): number {
	return entries.length;
}

export const appState = { page };
export const appNavigation = { goto };
