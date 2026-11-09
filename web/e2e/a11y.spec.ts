/**
 * Accessibility guard: axe finds nothing serious or critical on any page as it
 * first loads, at phone and desktop widths. States reached only by interaction
 * (open menus, search results) are not visited.
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
// Each test sweeps a whole route list, so the default 30s would scale with the
// list rather than with any one page; budget per route instead.
const PER_ROUTE_MS = 5_000;

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
			test.setTimeout(STATIC_ROUTES.length * PER_ROUTE_MS);
			const found: string[] = [];
			for (const route of STATIC_ROUTES) {
				found.push(...(await blockingViolations(page, route)));
			}
			expect(found).toEqual([]);
		});

		test('detail routes have no serious axe violations', async ({ page }) => {
			test.setTimeout(DETAIL_ROUTES.length * PER_ROUTE_MS);
			const seed = readSeed();
			const found: string[] = [];
			for (const route of DETAIL_ROUTES.map((build) => build(seed))) {
				found.push(...(await blockingViolations(page, route)));
			}
			expect(found).toEqual([]);
		});
	});
}

// Interaction state the route sweep never opens: the selected font-size
// option's faded description failed contrast on its blue fill (#1116).
test('the open Accessibility panel has no colour-contrast failures', async ({ page }) => {
	await page.setViewportSize({ width: 1280, height: 900 });
	await page.goto('/');
	await page.getByRole('button', { name: 'Accessibility settings' }).click();
	const dialog = page.getByRole('dialog', { name: 'Accessibility Settings' });
	await expect(dialog.getByRole('button', { name: /Normal/, pressed: true })).toBeVisible();

	const { violations } = await new AxeBuilder({ page })
		.include('.accessibility-dialog')
		.withRules(['color-contrast'])
		.analyze();
	expect(violations.flatMap((v) => v.nodes.map((n) => n.target.join(' ')))).toEqual([]);
});
