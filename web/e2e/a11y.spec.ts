/**
 * Accessibility guard: axe finds nothing serious or critical on any page, at
 * phone and desktop widths.
 *
 * Contrast is the reason it exists (#906): muted and accent text drifted below
 * WCAG AA one hardcoded colour at a time, and nothing short of a real browser
 * computing real colours can see that. It checks every axe rule, not only
 * contrast, so other serious regressions fail here too; minor and moderate
 * findings are left to review.
 */
import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { DETAIL_ROUTES, STATIC_ROUTES } from './routes';
import { readSeed } from './seed';

const WIDTHS = [375, 1280];
const BLOCKING = new Set(['serious', 'critical']);

/** One line per failing element, so a report names what to fix. */
async function blockingViolations(page: Page, route: string): Promise<string[]> {
	await page.goto(route);
	await page.waitForLoadState('networkidle');

	const { violations } = await new AxeBuilder({ page }).analyze();
	return violations
		.filter((v) => BLOCKING.has(v.impact ?? ''))
		.flatMap((v) =>
			v.nodes.map((n) => `${route} [${v.id}] ${n.target.join(' ')}: ${n.failureSummary ?? v.help}`)
		);
}

for (const width of WIDTHS) {
	test.describe(`at ${width}px`, () => {
		test.use({ viewport: { width, height: 900 } });

		test('static routes have no serious axe violations', async ({ page }) => {
			const found: string[] = [];
			for (const route of STATIC_ROUTES) {
				found.push(...(await blockingViolations(page, route)));
			}
			expect(found).toEqual([]);
		});

		test('detail routes have no serious axe violations', async ({ page }) => {
			const seed = readSeed();
			const found: string[] = [];
			for (const route of DETAIL_ROUTES.map((build) => build(seed))) {
				found.push(...(await blockingViolations(page, route)));
			}
			expect(found).toEqual([]);
		});
	});
}
