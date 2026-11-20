/**
 * The media lightbox on a phone (#1550): an editor sees the Mark private button
 * without scrolling the details panel, and the image stays in view above it.
 *
 * The spec uploads its own tall portrait with a long title and description, the
 * case that pushed the button below the panel's scroll edge.
 */
import { expect, test } from '@playwright/test';
import { API_BASE } from './seed';

const TITLE = 'Wedding portrait of the whole family in the garden of the old house';

let personId: string;

test.beforeAll(async ({ browser, request }) => {
	// A real PNG, taller than wide, drawn by the browser rather than kept as a fixture.
	const canvas = await browser.newPage({ viewport: { width: 600, height: 1200 } });
	await canvas.setContent('<body style="margin:0;background:#4a7c59"></body>');
	const portrait = await canvas.screenshot();
	await canvas.close();

	const person = await request.post(`${API_BASE}/persons`, {
		data: { given_name: 'Rosalind', surname: 'Lightbox' }
	});
	expect(person.ok()).toBe(true);
	personId = (await person.json()).id as string;

	const media = await request.post(`${API_BASE}/persons/${personId}/media`, {
		multipart: {
			title: TITLE,
			description:
				'Taken in the garden after the ceremony, with the whole family gathered on the steps of the old house.',
			file: { name: 'portrait.png', mimeType: 'image/png', buffer: portrait }
		}
	});
	expect(media.ok(), await media.text()).toBe(true);
});

for (const width of [320, 375]) {
	test.describe(`at ${width}x800`, () => {
		test.use({ viewport: { width, height: 800 } });

		test('Mark private is in view without scrolling, below the image', async ({ page }, testInfo) => {
			await page.goto(`/persons/${personId}`);
			await page.getByRole('button', { name: `View ${TITLE}` }).click();

			const dialog = page.getByRole('dialog');
			const mark = dialog.getByRole('button', { name: 'Mark private', exact: true });
			await expect(mark).toBeVisible();

			const panel = await dialog.locator('.metadata-panel').boundingBox();
			const button = await mark.boundingBox();
			const image = await dialog.locator('.media-image').boundingBox();
			if (!panel || !button || !image) throw new Error('lightbox parts not laid out');

			expect(button.y, 'button starts inside the panel').toBeGreaterThanOrEqual(panel.y);
			expect(button.y + button.height, 'button ends inside the panel').toBeLessThanOrEqual(
				panel.y + panel.height
			);
			expect(button.y + button.height, 'button ends inside the viewport').toBeLessThanOrEqual(800);
			expect(image.height, 'image is shown').toBeGreaterThan(100);
			expect(image.y + image.height, 'image sits above the panel').toBeLessThanOrEqual(panel.y);

			await testInfo.attach(`lightbox-${width}x800`, {
				body: await page.screenshot(),
				contentType: 'image/png'
			});
		});
	});
}
