/**
 * A stand-in for SvelteKit's router in component tests of pages that keep
 * state in the URL (see `#lib/utils/urlState.js`). `goto` moves a fake history
 * and updates the reactive `page`, so a test can drive a page, read its URL and
 * step Back. Wire it up with:
 *
 *   vi.mock('$app/state', async () => (await import('#lib/test/fakeRouter.svelte.js')).appState);
 *   vi.mock('$app/navigation', async () => (await import('#lib/test/fakeRouter.svelte.js')).appNavigation);
 *
 * and call `resetRouter(path)` before each render. `afterNavigate` callbacks
 * run on mount and after each `goto` or `back`.
 */
import { vi } from 'vitest';
import { onMount } from 'svelte';
import type { AfterNavigate } from '$app/navigation';

interface FakePage {
	url: URL;
	params: Record<string, string>;
	state: Record<string, unknown>;
}

type AfterNavigateCallback = (navigation: AfterNavigate) => void;

const ORIGIN = 'http://localhost';

let entries: string[] = ['/'];
let index = 0;
let params: Record<string, string> = {};
let afterNavigateCallbacks: AfterNavigateCallback[] = [];

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

/**
 * A fake route id: the first path segment, with any further segments as one
 * `[id]` param, so `/pedigree/p-1` and `/pedigree/p-2` share a route.
 */
function routeIdOf(path: string): string {
	const segments = new URL(path, ORIGIN).pathname.split('/').filter(Boolean);
	if (segments.length === 0) return '/';
	return segments.length === 1 ? `/${segments[0]}` : `/${segments[0]}/[id]`;
}

function target(path: string) {
	return { url: new URL(path, ORIGIN), params, route: { id: routeIdOf(path) } };
}

/**
 * Where an `enter` navigation comes from. In SPA mode (`ssr = false`) SvelteKit
 * passes a `from` with no route id and no URL, not `from: null`.
 */
const OUTSIDE_THE_APP = { params: {}, route: { id: null }, url: null };

/** A navigation to the current entry from `fromPath` (null: from outside the app). */
function navigationFrom(fromPath: string | null, type: 'enter' | 'goto' | 'popstate'): AfterNavigate {
	return {
		from: fromPath === null ? OUTSIDE_THE_APP : target(fromPath),
		to: target(entries[index]),
		type,
		willUnload: false,
		complete: Promise.resolve()
	} as unknown as AfterNavigate;
}

function navigated(fromPath: string, type: 'goto' | 'popstate') {
	const navigation = navigationFrom(fromPath, type);
	for (const callback of afterNavigateCallbacks) callback(navigation);
}

/**
 * Starts a fresh history at `path`, with the route `routeParams`. With
 * `from`, the history holds that page first, as if the user came from it.
 */
export function resetRouter(
	path: string,
	routeParams: Record<string, string> = {},
	{ from }: { from?: string } = {}
) {
	entries = from ? [from, path] : [path];
	index = entries.length - 1;
	params = routeParams;
	afterNavigateCallbacks = [];
	show();
}

/** Like SvelteKit's: runs on mount (from the previous entry, if any) and after each navigation. */
function afterNavigate(callback: AfterNavigateCallback) {
	onMount(() => {
		afterNavigateCallbacks.push(callback);
		callback(index > 0 ? navigationFrom(entries[index - 1], 'goto') : navigationFrom(null, 'enter'));
		return () => {
			afterNavigateCallbacks = afterNavigateCallbacks.filter((c) => c !== callback);
		};
	});
}

const goto = vi.fn(async (href: string, opts: { replace?: boolean } = {}) => {
	const fromPath = entries[index];
	if (opts.replace) {
		entries[index] = href;
	} else {
		entries = [...entries.slice(0, index + 1), href];
		index++;
	}
	show();
	navigated(fromPath, 'goto');
});

/** The browser's Back button. */
export function back() {
	if (index === 0) throw new Error('fakeRouter: no history entry to go back to');
	const fromPath = entries[index];
	index--;
	show();
	navigated(fromPath, 'popstate');
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
export const appNavigation = { goto, afterNavigate };
