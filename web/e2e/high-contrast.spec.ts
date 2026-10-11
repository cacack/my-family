/**
 * High-contrast mode keeps every piece of text readable (#905).
 *
 * jsdom computes no colours, so only a real browser can tell whether the
 * high-contrast tokens actually reach the page. This turns the setting on the
 * way the Accessibility panel persists it and asks axe for colour-contrast
 * failures on the pages the original report covered.
 */
import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { readSeed } from './seed';

const PAGES: { name: string; path: (s: ReturnType<typeof readSeed>) => string }[] = [
	{ name: 'home', path: () => '/' },
	{ name: 'people', path: () => '/persons' },
	{ name: 'person', path: (s) => `/persons/${s.switcher.person.id}` },
	{ name: 'quality', path: () => '/quality' },
	{ name: 'evidence', path: () => '/evidence' }
];

test.beforeEach(async ({ page }) => {
	await page.addInitScript(() => {
		localStorage.setItem(
			'accessibility-settings',
			JSON.stringify({ fontSize: 'normal', highContrast: true, reducedMotion: true })
		);
	});
});

for (const { name, path } of PAGES) {
	test(`${name} has no colour-contrast failures in high-contrast mode`, async ({ page }) => {
		await page.goto(path(readSeed()));
		await page.waitForLoadState('networkidle');
		await expect(page.locator('body')).toHaveClass(/high-contrast/);

		const results = await new AxeBuilder({ page }).withRules(['color-contrast']).analyze();
		const failures = results.violations.flatMap((v) =>
			v.nodes.map((n) => `${n.target.join(' ')}: ${n.failureSummary ?? ''}`)
		);
		expect(failures).toEqual([]);
	});
}
