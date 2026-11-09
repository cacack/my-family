import { afterNavigate, type AfterNavigate } from '$app/navigation';

/** A page's back link, as PageHeader and ActionBar take it. */
export interface BackLink {
	href: string;
	label: string;
}

/**
 * The in-app page a navigation came from, or null when it came from outside
 * the app (a deep link, a new tab, a reload) or stayed on the same route (a
 * chart re-rooted on another person, or its options changed). In SPA mode an
 * `enter` navigation has a `from` with a null URL, so test the URL, not `from`.
 */
export function cameFrom(navigation: Pick<AfterNavigate, 'from' | 'to'>): string | null {
	const { from, to } = navigation;
	if (!from?.url || from.route.id === to?.route.id) return null;
	return `${from.url.pathname}${from.url.search}${from.url.hash}`;
}

/**
 * A back link to the page the user came from, or to `fallback` when the page
 * was opened directly (#1026). Call it while the page component initialises.
 * Moving within the page (re-rooting a chart) keeps the original origin.
 *
 * @param fallback - Where Back goes without an in-app origin, e.g. the chart's person
 */
export function useBackLink(fallback: () => BackLink): { readonly link: BackLink } {
	let origin: string | null = $state(null);
	afterNavigate((navigation) => {
		const href = cameFrom(navigation);
		if (href) origin = href;
	});
	return {
		get link() {
			return origin ? { href: origin, label: 'Back' } : fallback();
		}
	};
}
