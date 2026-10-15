/**
 * Layout guard: no page scrolls sideways, at phone through desktop widths.
 *
 * Unit tests can't see layout - jsdom has none. This visits every top-level
 * route plus the seeded detail pages and checks the one symptom that matters
 * most: content wider than the viewport. It also checks the header controls
 * stay on-screen, which is how the original defect showed (the nav pushed the
 * search box out of view even at 1440px). The phone width is swept a second
 * time with the Larger font setting, where rem-sized layouts run out of room.
 *
 * Deliberately not a screenshot diff: those break on every intended change.
 */
import { expect, test } from '@playwright/test';
import { API_BASE, readSeed } from './seed';
import { DETAIL_ROUTES, STATIC_ROUTES } from './routes';

const WIDTHS = [375, 768, 1280, 1440];

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

// The Larger font setting (150% root size) is the narrowest a phone layout gets:
// every rem-sized width, padding and grid floor grows with it. Seeding the
// stored preference before the app boots is how a returning user arrives.
test.describe('at 375px with the Larger font setting', () => {
	test.use({ viewport: { width: 375, height: 900 } });

	test.beforeEach(async ({ page }) => {
		await page.addInitScript(() => {
			localStorage.setItem('accessibility-settings', JSON.stringify({ fontSize: 'larger' }));
		});
	});

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

// A flex item shrinks by default, so a long name next to the avatar squeezed
// the circle into an oval once the font grew. The seeded name is short enough
// to wrap, so the test swaps in an unbroken one to force the squeeze.
test('the person avatar stays a circle at every font size', async ({ page }) => {
	await page.setViewportSize({ width: 375, height: 900 });
	await page.goto(`/persons/${readSeed().switcher.person.id}`);
	for (const fontSize of ['normal', 'large', 'larger']) {
		await page.evaluate((size) => {
			localStorage.setItem('accessibility-settings', JSON.stringify({ fontSize: size }));
		}, fontSize);
		await page.reload();
		const avatar = page.locator('.person-header .avatar');
		await expect(avatar).toBeVisible();
		if (fontSize === 'normal') {
			await expect(page.locator('html')).not.toHaveClass(/font-large/);
		} else {
			await expect(page.locator('html')).toHaveClass(new RegExp(`font-${fontSize}\\b`));
		}
		await page.locator('.person-title h1').evaluate((h1) => {
			h1.textContent = 'Wilhelmina'.repeat(6);
		});
		const box = await avatar.boundingBox();
		expect(box, `${fontSize}: person avatar is not rendered`).not.toBeNull();
		expect(box!.width, `${fontSize}: avatar is not square`).toBeCloseTo(box!.height, 0);
	}
});

async function expectHeaderOneRow(page: import('@playwright/test').Page) {
	const rows = await page.evaluate(() =>
		[...document.querySelectorAll('.app-header > *')].map((el) => {
			const r = el.getBoundingClientRect();
			return { top: r.top, bottom: r.bottom };
		})
	);
	const lowestTop = Math.max(...rows.map((r) => r.top));
	const highestBottom = Math.min(...rows.map((r) => r.bottom));
	expect(lowestTop, 'header items wrapped onto a second row').toBeLessThan(highestBottom);
}

// The grouped nav exists so the header fits on one row from 1024px up; a new
// top-level link that breaks that should fail here, not in a screenshot review.
// 1024 and 1279/1280 mirror the breakpoints in MainNav.svelte and +layout.svelte.
for (const width of [1024, 1280, 1440]) {
	test(`header is a single row at ${width}px`, async ({ page }) => {
		await page.setViewportSize({ width, height: 900 });
		await page.goto('/');
		await page.waitForLoadState('networkidle');
		await expectHeaderOneRow(page);
	});
}

// Every nav entry must be the topmost element at its own centre: header
// controls that run out of room must wrap, not lie on top of the nav (#1094).
// The search box must stay wide enough to show its whole placeholder.
async function expectHeaderUnobstructed(page: import('@playwright/test').Page, label: string) {
	const m = await page.evaluate(() => {
		const covered = [...document.querySelectorAll('.app-header .logo, .app-header .nav-wide > *')]
			.filter((el) => {
				const r = el.getBoundingClientRect();
				const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
				return !hit || !el.contains(hit);
			})
			.map((el) => el.textContent?.trim() || el.className);
		const input = document.querySelector<HTMLInputElement>('.header-controls input')!;
		const style = getComputedStyle(input);
		const ctx = document.createElement('canvas').getContext('2d')!;
		ctx.font = style.font;
		return {
			covered,
			placeholder: ctx.measureText(input.placeholder).width,
			room: input.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
		};
	});
	expect(m.covered, `${label}: nav entries covered by another element`).toEqual([]);
	expect(m.room, `${label}: search placeholder is cut off`).toBeGreaterThanOrEqual(m.placeholder);
}

const FONT_SIZES = ['normal', 'large', 'larger'] as const;
const HEADER_WIDTHS = [1024, 1280, 1440];

// The breakpoints are in px but the header is sized in rem, so a larger font
// size shrinks the room at every width. Only the default size must fit on one row.
async function sweepHeader(page: import('@playwright/test').Page) {
	for (const fontSize of FONT_SIZES) {
		// Init scripts run in the order added, so the latest setting wins.
		await page.addInitScript((size) => {
			localStorage.setItem('accessibility-settings', JSON.stringify({ fontSize: size }));
		}, fontSize);
		for (const width of HEADER_WIDTHS) {
			await page.setViewportSize({ width, height: 900 });
			await expectNoHorizontalOverflow(page, '/');
			await expectHeaderUnobstructed(page, `${fontSize} at ${width}px`);
			if (fontSize === 'normal') await expectHeaderOneRow(page);
		}
	}
}

test('the header controls never cover the nav, at any font size', async ({ page }) => {
	await sweepHeader(page);
});

// An active branch adds its switcher label to the same row. A long name must
// truncate rather than wrap the header, and must leave the search box room to
// type in at 1024px, where the row is tightest (#919).
test.describe('with a long research branch name active', () => {
	let name = '';

	test.beforeEach(async ({ page, request }) => {
		const suffix = Date.now().toString(36);
		name = `${suffix} Was the ferry keeper at Lindholm the father of Robin Ashby?`.slice(0, 60);
		const response = await request.post(`${API_BASE}/branches`, { data: { name } });
		expect(response.status(), await response.text()).toBe(201);
		const { id } = (await response.json()) as { id: string };
		await page.addInitScript((branchId) => {
			localStorage.setItem('active-branch', branchId);
		}, id);
	});

	for (const width of [1024, 1280]) {
		test(`header is a single row at ${width}px`, async ({ page }) => {
			await page.setViewportSize({ width, height: 900 });
			await expectNoHorizontalOverflow(page, '/');
			const switcher = page.getByRole('button', { name: `Switch research branch. Currently on ${name}` });
			await expect(switcher).toBeVisible();
			await expectHeaderOneRow(page);

			const searchRem = await page.evaluate(() => {
				const rem = parseFloat(getComputedStyle(document.documentElement).fontSize);
				return document.querySelector('.search-wrapper')!.getBoundingClientRect().width / rem;
			});
			expect(searchRem, 'header search box is narrower than 8rem').toBeGreaterThanOrEqual(8);
		});
	}

	test('the header controls never cover the nav, at any font size', async ({ page }) => {
		await sweepHeader(page);
	});
});

test('every nav destination is two taps away on a phone', async ({ page }) => {
	await page.setViewportSize({ width: 375, height: 900 });
	await page.goto('/');
	await page.waitForLoadState('networkidle');

	await page.getByRole('button', { name: 'Menu' }).click();
	await page.getByRole('menuitem', { name: 'Citation Templates' }).click();
	await expect(page).toHaveURL(/\/browse\/citation-templates$/);
});

// A chart sized from its own SVG could grow with a taller window but never
// shrink again, leaving the page scrolling after a rotation or resize.
for (const chart of ['pedigree', 'descendancy']) {
	test(`the ${chart} chart shrinks back when the window gets shorter`, async ({ page }) => {
		const box = page.locator(`.${chart}-chart`);
		const height = async () => (await box.boundingBox())!.height;
		await page.setViewportSize({ width: 1280, height: 900 });
		await page.goto(`/${chart}/${readSeed().switcher.person.id}`);
		await expect(box.locator('svg g').first()).toBeAttached();
		const initial = await height();

		await page.setViewportSize({ width: 1280, height: 1300 });
		const grown = initial + 400;
		await expect.poll(height).toBeCloseTo(grown, 0);
		// Wait for the chart to redraw at the new size: the redrawn SVG is what could pin it
		const viewBoxHeight = () => box.locator('svg').evaluate((svg) => svg.viewBox.baseVal.height);
		await expect.poll(viewBoxHeight).toBeCloseTo(grown, 0);
		await page.setViewportSize({ width: 1280, height: 900 });
		await expect.poll(height).toBeCloseTo(initial, 0);
		const overflow = await page.locator('main').evaluate((m) => m.scrollHeight - m.clientHeight);
		expect(overflow, 'page scrolls after shrinking the window').toBeLessThanOrEqual(0);
	});
}
