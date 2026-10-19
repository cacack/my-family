/**
 * Scrolling dialogs keep their content reachable by keyboard (#1104).
 *
 * jsdom does no layout, so whether focus scrolls a dialog, or whether a scroll
 * region can be scrolled at all, only shows in a real browser.
 */
import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Locator } from '@playwright/test';

/** True when `inner` lies wholly within the visible box of `outer`. */
async function fullyInside(inner: Locator, outer: Locator): Promise<boolean> {
	const a = await inner.boundingBox();
	const b = await outer.boundingBox();
	if (!a || !b) return false;
	return a.y >= b.y && a.y + a.height <= b.y + b.height && a.x >= b.x && a.x + a.width <= b.x + b.width;
}

test.describe('Accessibility settings dialog', () => {
	test.use({ viewport: { width: 375, height: 700 } });

	test('tabbing to each toggle at Larger font keeps it in view', async ({ page }) => {
		await page.addInitScript(() => {
			localStorage.setItem(
				'accessibility-settings',
				JSON.stringify({ fontSize: 'larger', highContrast: false, reducedMotion: false })
			);
		});
		await page.goto('/');
		await page.getByRole('button', { name: 'Accessibility settings' }).click();
		const dialog = page.getByRole('dialog', { name: 'Accessibility Settings' });
		const content = dialog.locator('.dialog-content');
		await expect(dialog.getByRole('button', { name: 'Close accessibility settings' })).toBeFocused();

		for (const name of ['High contrast', 'Reduced motion', 'Keyboard shortcuts']) {
			const toggle = dialog.getByRole('checkbox', { name });
			for (let i = 0; i < 10 && !(await toggle.evaluate((el) => el === document.activeElement)); i++) {
				await page.keyboard.press('Tab');
			}
			await expect(toggle).toBeFocused();
			const visibleSwitch = toggle.locator('xpath=following-sibling::*[contains(@class, "toggle-switch")]');
			expect(await fullyInside(visibleSwitch, content), `${name} switch in view`).toBe(true);
		}
	});
});

for (const width of [375, 1280]) {
	test.describe(`Keyboard Shortcuts dialog at ${width}px`, () => {
		test.use({ viewport: { width, height: 700 } });

		test.beforeEach(async ({ page }) => {
			await page.goto('/');
			await page.locator('body').press('?');
			await expect(page.getByRole('dialog', { name: 'Keyboard Shortcuts' })).toBeVisible();
		});

		test('has no scrollable-region-focusable violation', async ({ page }) => {
			const { violations } = await new AxeBuilder({ page })
				.include('[role="dialog"]')
				.withRules(['scrollable-region-focusable'])
				.analyze();
			expect(violations.flatMap((v) => v.nodes.map((n) => n.target.join(' ')))).toEqual([]);
		});

		test('scrolls to the last group by keyboard alone', async ({ page }) => {
			const list = page.getByRole('region', { name: 'Shortcut list' });
			for (let i = 0; i < 5 && !(await list.evaluate((el) => el === document.activeElement)); i++) {
				await page.keyboard.press('Tab');
			}
			await expect(list).toBeFocused();

			await page.keyboard.press('End');
			const last = list.getByRole('heading', { name: 'Family Detail' });
			await expect.poll(() => fullyInside(last, list)).toBe(true);
			await expect
				.poll(() => list.evaluate((el) => el.scrollTop + el.clientHeight >= el.scrollHeight - 1))
				.toBe(true);
		});
	});
}
