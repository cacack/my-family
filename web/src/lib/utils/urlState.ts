/**
 * Page state kept in URL search params, so reload, Back and a copied link all
 * restore the view. The URL is the single source of truth: a page derives its
 * state from `$page.url` and changes it only through `setQuery`.
 *
 * History rule: `push` for navigation-level changes the user may want to step
 * back through (a list page, a tab, a submitted search); replace for refining
 * the current view (sort, filters, chart options).
 */
import { goto } from '$app/navigation';

export type QueryValue = string | number | boolean | null | undefined;
export type QueryValues = Record<string, QueryValue>;

/**
 * The `pathname?search` of `url` with `updates` applied. A value that is empty,
 * `false`, `null`/`undefined` or equal to its entry in `defaults` is removed,
 * so a view in its default state has a clean URL. `true` is written as `1`.
 */
export function withQuery(url: URL, updates: QueryValues, defaults: QueryValues = {}): string {
	const params = new URLSearchParams(url.search);
	for (const [key, value] of Object.entries(updates)) {
		if (value === null || value === undefined || value === '' || value === false || value === defaults[key]) {
			params.delete(key);
		} else {
			params.set(key, value === true ? '1' : String(value));
		}
	}
	const search = params.toString();
	return search ? `${url.pathname}?${search}` : url.pathname;
}

/** Applies `updates` to the current URL; see `withQuery` and the history rule above. */
export function setQuery(
	url: URL,
	updates: QueryValues,
	{ push = false, defaults = {} }: { push?: boolean; defaults?: QueryValues } = {}
): Promise<void> {
	return goto(withQuery(url, updates, defaults), {
		replaceState: !push,
		keepFocus: true,
		noScroll: true
	});
}

/** `params[key]` when it is one of `allowed`, else `fallback` (a hand-edited URL never breaks the page). */
export function readEnum<T extends string>(
	params: URLSearchParams,
	key: string,
	allowed: readonly T[],
	fallback: T
): T {
	const value = params.get(key);
	return value !== null && (allowed as readonly string[]).includes(value) ? (value as T) : fallback;
}

/** `params[key]` as a positive integer capped at `max`, else `fallback`. */
export function readPositiveInt(
	params: URLSearchParams,
	key: string,
	fallback: number,
	max = Number.MAX_SAFE_INTEGER
): number {
	const value = Number(params.get(key));
	return Number.isInteger(value) && value > 0 ? Math.min(value, max) : fallback;
}

/** `params[key]` as a flag: present and not `0`/`false` (`withQuery` writes `1` or omits it). */
export function readFlag(params: URLSearchParams, key: string): boolean {
	const value = params.get(key);
	return value !== null && value !== '0' && value !== 'false';
}

/** The pedigree and descendancy charts' toolbar options, shared by both pages. */
export const CHART_GENERATIONS = ['2', '3', '4', '5', '6', '7', '8', '9', '10'] as const;
export const CHART_LAYOUTS = ['compact', 'standard', 'wide'] as const;
export const CHART_DEFAULTS = { gens: 4, layout: 'compact' } as const satisfies {
	gens: number;
	layout: (typeof CHART_LAYOUTS)[number];
};

export function readChartOptions(params: URLSearchParams): {
	generations: number;
	layout: (typeof CHART_LAYOUTS)[number];
} {
	const defaultGens = String(CHART_DEFAULTS.gens) as (typeof CHART_GENERATIONS)[number];
	return {
		generations: Number(readEnum(params, 'gens', CHART_GENERATIONS, defaultGens)),
		layout: readEnum(params, 'layout', CHART_LAYOUTS, CHART_DEFAULTS.layout)
	};
}
