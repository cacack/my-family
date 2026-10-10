/**
 * Layout guard: no page scrolls sideways, at phone through desktop widths.
 *
 * Unit tests can't see layout - jsdom has none. This visits every top-level
 * route plus the seeded detail pages and checks the one symptom that matters
 * most: content wider than the viewport. It also checks the header controls
 * stay on-screen, which is how the original defect showed (the nav pushed the
 * search box out of view even at 1440px).
 *
 * Deliberately not a screenshot diff: those break on every intended change.
 */
import { expect, test } from '@playwright/test';
import { readSeed } from './seed';

const WIDTHS = [375, 768, 1280, 1440];

const STATIC_ROUTES = [
	'/',
	'/persons',
	'/persons/add',
	'/persons/quick',
	'/families',
	'/families/add',
	'/browse/surnames',
	'/browse/places',
	'/browse/cemeteries',
	'/browse/brick-walls',
	'/browse/citation-templates',
	'/repositories',
	'/sources',
	'/evidence',
	'/history',
	'/branches',
	'/snapshots',
	'/map',
	'/analytics',
	'/quality',
	'/relationship',
	'/import',
	'/search'
];

// Detail routes need seeded ids, and the seed can only be read once global
// setup has run - so they are built inside the test, not at collection time.
const DETAIL_ROUTES = [
	(s: ReturnType<typeof readSeed>) => `/persons/${s.switcher.person.id}`,
	(s: ReturnType<typeof readSeed>) => `/pedigree/${s.switcher.person.id}`,
	(s: ReturnType<typeof readSeed>) => `/descendancy/${s.switcher.person.id}`,
	(s: ReturnType<typeof readSeed>) => `/families/${s.merge.familyId}`,
	(s: ReturnType<typeof readSeed>) => `/families/${s.merge.familyId}/group-sheet`
];

async function expectNoHorizontalOverflow(page: import('@playwright/test').Page, route: string) {
	const errors: string[] = [];
	page.on('pageerror', (e) => errors.push(e.message));

	await page.goto(route);
	await page.waitForLoadState('networkidle');

	const m = await page.evaluate(() => {
		const main = document.querySelector('main');
		const search = document.querySelector('.header-controls input');
		return {
			doc: document.documentElement.scrollWidth - window.innerWidth,
			main: main ? main.scrollWidth - main.clientWidth : 0,
			searchRight: search ? search.getBoundingClientRect().right - window.innerWidth : 0
		};
	});

	expect(m.doc, `${route}: page overflows the viewport by ${m.doc}px`).toBeLessThanOrEqual(0);
	expect(m.main, `${route}: content overflows <main> by ${m.main}px`).toBeLessThanOrEqual(0);
	expect(m.searchRight, `${route}: header search box is off-screen`).toBeLessThanOrEqual(0);
	expect(errors, `${route}: uncaught page errors`).toEqual([]);
}

for (const width of WIDTHS) {
	test.describe(`at ${width}px`, () => {
		test.use({ viewport: { width, height: 900 } });

		test('static routes do not overflow horizontally', async ({ page }) => {
			for (const route of STATIC_ROUTES) {
				await expectNoHorizontalOverflow(page, route);
			}
		});

		test('detail routes do not overflow horizontally', async ({ page }) => {
			const seed = readSeed();
			for (const route of DETAIL_ROUTES.map((build) => build(seed))) {
				await expectNoHorizontalOverflow(page, route);
			}
		});
	});
}

// The grouped nav exists so the header fits on one row from 1024px up; a new
// top-level link that breaks that should fail here, not in a screenshot review.
for (const width of [1024, 1280, 1440]) {
	test(`header is a single row at ${width}px`, async ({ page }) => {
		await page.setViewportSize({ width, height: 900 });
		await page.goto('/');
		await page.waitForLoadState('networkidle');

		const rows = await page.evaluate(() =>
			[...document.querySelectorAll('.app-header > *')].map((el) => {
				const r = el.getBoundingClientRect();
				return { top: r.top, bottom: r.bottom };
			})
		);
		const lowestTop = Math.max(...rows.map((r) => r.top));
		const highestBottom = Math.min(...rows.map((r) => r.bottom));
		expect(lowestTop, 'header items wrapped onto a second row').toBeLessThan(highestBottom);
	});
}

test('every nav destination is two taps away on a phone', async ({ page }) => {
	await page.setViewportSize({ width: 375, height: 900 });
	await page.goto('/');
	await page.waitForLoadState('networkidle');

	await page.getByRole('button', { name: 'Menu' }).click();
	await page.getByRole('menuitem', { name: 'Citation Templates' }).click();
	await expect(page).toHaveURL(/\/browse\/citation-templates$/);
});
