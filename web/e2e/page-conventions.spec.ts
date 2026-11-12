/**
 * Page conventions guard (#1114): the page container widths, the h1 style and
 * the back-link labels in docs/CONVENTIONS.md ("Page container", "Page
 * heading", "Back links") hold on every route.
 *
 * At 1280px every list page's h1 starts at one x, so moving between two of
 * them does not shift the heading, and every detail page's heading block (its
 * back link, else its h1) starts at one x. A record page (person, family,
 * source, evidence) sets its h1 inside the record's card, so its h1 sits the
 * card's padding further in; the block still starts at the container edge.
 * Every h1 has one computed size and weight, and every back link reads its
 * destination's h1. Pages are classified by path below, so a page that picks
 * a third width fails here.
 */
import { expect, test, type Page } from '@playwright/test';
import { DETAIL_ROUTES, STATIC_ROUTES } from './routes';
import { readSeed, type SeedData } from './seed';

// Routes the whole-app guards do not visit yet, but whose headings differed.
const EXTRA_ROUTES: ((s: SeedData) => string)[] = [
	(s) => `/quality/merge/${s.switcher.person.id}/${s.merge.person.id}`,
	(s) => `/branches/${s.switcher.branchId}/research`,
	() => '/evidence/analyses/new'
];

type Width = 'list' | 'detail' | 'chart';

// One record or a form; anything else not a chart is a list-width page.
const DETAIL_PATHS = [
	/^\/persons\/[^/]+$/,
	/^\/families\/[^/]+(\/group-sheet)?$/,
	/^\/(sources|repositories)\/[^/]+$/,
	/^\/branches\/[^/]+\/research$/,
	/^\/evidence\/[^/]+\/[^/]+$/,
	/^\/(import|relationship)$/
];
const CHART_PATHS = /^\/(pedigree|descendancy)\//;

function widthOf(route: string): Width {
	const path = route.split('?')[0];
	if (CHART_PATHS.test(path)) return 'chart';
	return DETAIL_PATHS.some((p) => p.test(path)) ? 'detail' : 'list';
}

// Each route is visited once to measure and once more through its back link.
const PER_ROUTE_MS = 10_000;

interface Heading {
	route: string;
	width: Width;
	left: number;
	/** Where the heading block starts: the back link, else the h1. */
	start: number;
	font: string;
	back: { href: string; label: string } | null;
}

async function measure(page: Page, route: string): Promise<Heading> {
	await page.goto(route);
	await page.waitForLoadState('networkidle');
	const h1 = page.locator('main h1');
	await expect(h1, `${route}: one h1`).toHaveCount(1);
	const m = await h1.evaluate((el) => {
		const style = getComputedStyle(el);
		const back = document.querySelector<HTMLAnchorElement>('main .back-link');
		const left = Math.round(el.getBoundingClientRect().left);
		return {
			left,
			start: back ? Math.round(back.getBoundingClientRect().left) : left,
			font: `${style.fontSize}/${style.fontWeight}`,
			back: back
				? {
						href: back.getAttribute('href') ?? '',
						label: (back.textContent ?? '').replace(/^\s*←\s*/, '').trim()
					}
				: null
		};
	});
	return { route, width: widthOf(route), ...m };
}

/** The values seen, each with the routes that had it - one entry when they all agree. */
function grouped(headings: Heading[], key: (h: Heading) => string | number) {
	const groups: Record<string, string[]> = {};
	for (const h of headings) (groups[String(key(h))] ??= []).push(h.route);
	return groups;
}

test.describe('at 1280px', () => {
	test.use({ viewport: { width: 1280, height: 900 } });

	test('headings line up, match in style, and back links name their destination', async ({
		page
	}) => {
		const seed = readSeed();
		const routes = [
			...STATIC_ROUTES,
			...DETAIL_ROUTES.map((build) => build(seed)),
			...EXTRA_ROUTES.map((build) => build(seed))
		];
		test.setTimeout(routes.length * PER_ROUTE_MS);

		const headings: Heading[] = [];
		for (const route of routes) headings.push(await measure(page, route));

		const listX = grouped(
			headings.filter((h) => h.width === 'list'),
			(h) => h.left
		);
		expect(Object.keys(listX), `list pages' h1 left x: ${JSON.stringify(listX)}`).toHaveLength(1);
		const detailX = grouped(
			headings.filter((h) => h.width === 'detail'),
			(h) => h.start
		);
		expect(
			Object.keys(detailX),
			`detail pages' heading left x: ${JSON.stringify(detailX)}`
		).toHaveLength(1);
		const fonts = grouped(headings, (h) => h.font);
		expect(Object.keys(fonts), `h1 font size/weight: ${JSON.stringify(fonts)}`).toHaveLength(1);

		// A Back link to the user's origin is the one label that names no page.
		const named = headings.filter((h) => h.back && h.back.label !== 'Back');
		expect(named.length, 'pages with a named back link').toBeGreaterThan(0);
		for (const { route, back } of named) {
			await page.goto(back!.href);
			await page.waitForLoadState('networkidle');
			await expect(page.locator('main h1'), `${route}: back link "${back!.label}"`).toHaveText(
				back!.label
			);
		}
	});
});
