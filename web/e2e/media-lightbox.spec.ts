/**
 * The media lightbox on a phone (#1550): an editor sees the Mark private button
 * without scrolling the details panel, and the image stays in view above it.
 *
 * The spec uploads its own tall portrait with a long title and description, the
 * case that pushed the button below the panel's scroll edge.
 *
 * It also covers the rest of the phone layout (#1555): the viewer hides the
 * page, an unbreakable title wraps, the close button clears the image, and a
 * document preview fits the screen.
 *
 * Editing an item's details (#1107) is saved through the API and survives a reload.
 */
import { expect, test, type Page } from '@playwright/test';
import { API_BASE } from './seed';

const TITLE = 'Wedding portrait of the whole family in the garden of the old house';
/** One 80-character word, which cannot wrap at a space. */
const UNBROKEN_TITLE = 'Grosselternhochzeitsfotografie'.padEnd(80, 'x');
const PDF_TITLE = 'Census page';
/** No hyphen or space to break at. */
const PDF_NAME = 'census_of_the_parish_of_saint_mary_the_virgin_1881_page_twelve.pdf';

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

	const uploads = [
		{
			title: TITLE,
			description:
				'Taken in the garden after the ceremony, with the whole family gathered on the steps of the old house.',
			file: { name: 'portrait.png', mimeType: 'image/png', buffer: portrait }
		},
		{
			title: UNBROKEN_TITLE,
			file: { name: 'unbroken.png', mimeType: 'image/png', buffer: portrait }
		},
		{
			title: PDF_TITLE,
			file: {
				name: PDF_NAME,
				mimeType: 'application/pdf',
				buffer: Buffer.from('%PDF-1.4\n%%EOF\n')
			}
		}
	];
	for (const multipart of uploads) {
		const media = await request.post(`${API_BASE}/persons/${personId}/media`, { multipart });
		expect(media.ok(), await media.text()).toBe(true);
	}
});

async function openLightbox(page: Page, title: string) {
	await page.goto(`/persons/${personId}`);
	await page.getByRole('button', { name: `View ${title}` }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog).toBeVisible();
	return dialog;
}

type Box = { x: number; y: number; width: number; height: number };

function intersects(a: Box, b: Box): boolean {
	return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
}

for (const width of [320, 375]) {
	test.describe(`at ${width}x800`, () => {
		test.use({ viewport: { width, height: 800 } });

		test('Mark private is in view without scrolling, below the image', async ({ page }, testInfo) => {
			const dialog = await openLightbox(page, TITLE);
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

		test('the close button does not cover a tall image', async ({ page }) => {
			const dialog = await openLightbox(page, TITLE);
			await expect(dialog.locator('.media-image')).toBeVisible();

			const close = await dialog.getByRole('button', { name: 'Close lightbox' }).boundingBox();
			const image = await dialog.locator('.media-image').boundingBox();
			if (!close || !image) throw new Error('lightbox parts not laid out');

			expect(intersects(close, image), 'close button overlaps the image').toBe(false);
		});
	});
}

test.describe('at 375x800', () => {
	test.use({ viewport: { width: 375, height: 800 } });

	test('the viewer covers the screen with an opaque background', async ({ page }, testInfo) => {
		const dialog = await openLightbox(page, TITLE);
		await expect(dialog.locator('.media-image')).toBeVisible();

		// Polled: the dialog zooms in as it opens.
		await expect
			.poll(() => dialog.boundingBox())
			.toEqual({ x: 0, y: 0, width: 375, height: 800 });

		// Opaque colors compute to rgb(); any transparency computes to rgba() or a slash form.
		const background = await dialog.evaluate((el) => getComputedStyle(el).backgroundColor);
		expect(background).toMatch(/^rgb\(\d+, \d+, \d+\)$/);

		await testInfo.attach('lightbox-opaque-375x800', {
			body: await page.screenshot(),
			contentType: 'image/png'
		});
	});

	test('a document preview fits the screen', async ({ page }) => {
		const dialog = await openLightbox(page, PDF_TITLE);
		const name = dialog.locator('.document-name');
		const view = dialog.getByRole('link', { name: 'View PDF' });
		await expect(name).toHaveText(PDF_NAME);
		await expect(view).toBeVisible();

		for (const [label, locator] of [
			['name', name],
			['View PDF', view]
		] as const) {
			const box = await locator.boundingBox();
			if (!box) throw new Error(`${label} not laid out`);
			expect(box.x, `${label} starts inside the viewport`).toBeGreaterThanOrEqual(0);
			expect(box.y, `${label} starts inside the viewport`).toBeGreaterThanOrEqual(0);
			expect(box.x + box.width, `${label} ends inside the viewport`).toBeLessThanOrEqual(375);
			expect(box.y + box.height, `${label} ends inside the viewport`).toBeLessThanOrEqual(800);
		}

		const overflow = await dialog.evaluate((el) => ({
			page: document.documentElement.scrollWidth,
			dialog: el.scrollWidth - el.clientWidth,
			preview: (() => {
				const p = el.querySelector('.document-preview') as HTMLElement;
				return p.scrollWidth - p.clientWidth;
			})()
		}));
		expect(overflow).toEqual({ page: 375, dialog: 0, preview: 0 });
	});
});

test.describe('editing the details at 375x800 (#1107)', () => {
	test.use({ viewport: { width: 375, height: 800 } });

	test('saves a new title, description and type, which survive a reload', async ({ page, request }) => {
		// Its own item, so the other tests keep their titles.
		const media = await request.post(`${API_BASE}/persons/${personId}/media`, {
			multipart: {
				title: 'Scan 0042',
				media_type: 'photo',
				file: { name: 'scan.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4\n%%EOF\n') }
			}
		});
		expect(media.ok(), await media.text()).toBe(true);

		const dialog = await openLightbox(page, 'Scan 0042');
		await dialog.getByRole('button', { name: 'Edit details' }).click();
		await dialog.getByRole('textbox', { name: /^Title/ }).fill('Baptism record, 1881');
		await dialog.getByRole('textbox', { name: 'Description' }).fill('Parish register, page 12');
		await dialog.getByRole('combobox', { name: 'Type' }).selectOption('certificate');
		const save = dialog.getByRole('button', { name: 'Save' });
		const box = await save.boundingBox();
		if (!box) throw new Error('Save not laid out');
		expect(box.y + box.height, 'Save ends inside the viewport').toBeLessThanOrEqual(800);
		await save.click();

		await expect(dialog.getByRole('heading', { name: 'Baptism record, 1881', exact: true })).toBeVisible();
		await expect(dialog.getByRole('button', { name: 'Edit details' })).toBeFocused();

		const reopened = await openLightbox(page, 'Baptism record, 1881');
		await expect(reopened.getByText('Parish register, page 12')).toBeVisible();
		await expect(reopened.locator('.metadata-list')).toContainText('certificate');
	});
});

test.describe('at 320x800', () => {
	test.use({ viewport: { width: 320, height: 800 } });

	test('an unbroken title wraps and Mark private stays in the panel', async ({ page }) => {
		const dialog = await openLightbox(page, UNBROKEN_TITLE);
		// Polled: the dialog zooms in as it opens.
		await expect
			.poll(() => dialog.boundingBox())
			.toEqual({ x: 0, y: 0, width: 320, height: 800 });

		const panel = await dialog.locator('.metadata-panel').boundingBox();
		const heading = await dialog.locator('.metadata-panel h3').boundingBox();
		const button = await dialog
			.getByRole('button', { name: 'Mark private', exact: true })
			.boundingBox();
		if (!panel || !heading || !button) throw new Error('lightbox parts not laid out');

		// An unwrapped title widens the panel past the screen rather than overflowing it.
		expect(panel.x + panel.width, 'panel ends inside the viewport').toBeLessThanOrEqual(320);
		expect(heading.x + heading.width, 'title ends inside the panel').toBeLessThanOrEqual(
			panel.x + panel.width
		);
		expect(heading.height, 'title wraps onto several lines').toBeGreaterThan(2 * button.height);

		expect(button.x, 'button starts inside the panel').toBeGreaterThanOrEqual(panel.x);
		expect(button.y, 'button starts inside the panel').toBeGreaterThanOrEqual(panel.y);
		expect(button.x + button.width, 'button ends inside the panel').toBeLessThanOrEqual(
			panel.x + panel.width
		);
		expect(button.y + button.height, 'button ends inside the panel').toBeLessThanOrEqual(
			panel.y + panel.height
		);
		expect(button.x + button.width, 'button ends inside the viewport').toBeLessThanOrEqual(320);
		expect(button.y + button.height, 'button ends inside the viewport').toBeLessThanOrEqual(800);
	});
});
