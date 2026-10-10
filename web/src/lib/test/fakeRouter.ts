/**
 * A stand-in for SvelteKit's router in component tests of pages that keep
 * state in the URL (see `$lib/utils/urlState.ts`). `goto` moves a fake history
 * and updates the `page` store, so a test can drive a page, read its URL and
 * step Back. Wire it up with:
 *
 *   vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
 *   vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);
 *
 * and call `resetRouter(path)` before each render.
 */
import { writable } from 'svelte/store';
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

const page = writable<FakePage>({ url: new URL('/', ORIGIN), params, state: {} });

function show() {
	// A fresh object each time: Svelte's store bridge dedupes on identity.
	page.set({ url: new URL(entries[index], ORIGIN), params, state: {} });
}

/** Starts a fresh history at `path`, with the route `routeParams`. */
export function resetRouter(path: string, routeParams: Record<string, string> = {}) {
	entries = [path];
	index = 0;
	params = routeParams;
	show();
}

const goto = vi.fn(async (href: string, opts: { replaceState?: boolean } = {}) => {
	if (opts.replaceState) {
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

export const appStores = { page };
export const appNavigation = { goto };
