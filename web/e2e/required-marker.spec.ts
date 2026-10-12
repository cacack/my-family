/**
 * Required-field markers sit on the same line as their label text (#933).
 *
 * The entity forms lay each `<label>` out as a column (text above input), so a
 * bare `Title <span class="required">*</span>` became two flex items and the
 * `*` wrapped onto a line of its own. jsdom has no layout, hence a browser
 * check: the marker's top must match the top of the label's first text line.
 * Forms are only opened, never submitted, so the shared store is untouched.
 */
import { expect, test, type Page } from '@playwright/test';
import { readSeed } from './seed';

async function expectMarkersInline(page: Page, route: string, openButtons: string[]) {
	await page.goto(route);
	await page.waitForLoadState('networkidle');
	for (const name of openButtons) {
		await page.getByRole('button', { name, exact: true }).first().click();
	}

	const markers = page.locator('label .required');
	await expect(markers.first()).toBeVisible();

	const offsets = await markers.evaluateAll((els) =>
		els.map((marker) => {
			const label = marker.closest('label') as HTMLLabelElement;
			const walker = document.createTreeWalker(label, NodeFilter.SHOW_TEXT, {
				acceptNode: (n) =>
					n.textContent?.trim() ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_SKIP
			});
			const range = document.createRange();
			range.selectNodeContents(walker.nextNode() as Text);
			return {
				text: label.textContent?.trim().split('\n')[0] ?? '',
				textTop: range.getBoundingClientRect().top,
				markerTop: marker.getBoundingClientRect().top
			};
		})
	);

	for (const o of offsets) {
		expect(
			Math.abs(o.markerTop - o.textTop),
			`${route}: "*" after "${o.text}" is not on the label's line`
		).toBeLessThanOrEqual(2);
	}
}

for (const width of [375, 1280]) {
	test.describe(`at ${width}px`, () => {
		test.use({ viewport: { width, height: 900 } });

		test('Add Source form shows the Title marker inline', async ({ page }) => {
			await expectMarkersInline(page, '/sources', ['Add Source']);
		});

		test('Add Repository form shows the Name marker inline', async ({ page }) => {
			await expectMarkersInline(page, '/repositories', ['Add Repository']);
		});

		test('person name and citation forms show their markers inline', async ({ page }) => {
			const seed = readSeed();
			await expectMarkersInline(page, `/persons/${seed.switcher.person.id}`, [
				'Add Name',
				'Add Citation'
			]);
		});
	});
}
