/**
 * The map's layout at phone, tablet and desktop widths (#1101, #1102, #1117):
 * the first view fits every marker, no two drawn markers overlap, focus pans an
 * off-screen marker into view, and the details panel hands focus back on close.
 *
 * Coordinates only arrive through GEDCOM import, so the spec imports its own
 * people spread around the world, with a crowd of nearby European cities.
 */
import { expect, test, type Page } from '@playwright/test';
import { API_BASE } from './seed';

// Name, longitude, latitude, people born there (more people draw a bigger marker)
const PLACES: [string, number, number, number][] = [
	['Anchorage', -149.9, 61.22, 1],
	['Boston', -71.06, 42.36, 2],
	['Edinburgh', -3.19, 55.95, 4],
	['London', -0.13, 51.51, 6],
	['Paris', 2.35, 48.86, 6],
	['Lyon', 4.84, 45.76, 5],
	['Berlin', 13.4, 52.52, 6],
	['Rome', 12.5, 41.9, 5],
	['Madrid', -3.7, 40.42, 5],
	['Sydney', 151.21, -33.87, 1],
	['Suva', 178.44, -18.14, 1]
];

const suffix = Date.now().toString(36);
const place = (name: string) => `${name} Layout${suffix}`;

test.beforeAll(async ({ request }) => {
	const coord = (v: number, pos: string, neg: string) => `${v < 0 ? neg : pos}${Math.abs(v)}`;
	const lines = ['0 HEAD', '1 GEDC', '2 VERS 5.5.1', '1 CHAR UTF-8'];
	let id = 0;
	for (const [name, lon, lat, people] of PLACES) {
		for (let n = 0; n < people; n++) {
			lines.push(
				`0 @I${++id}@ INDI`,
				`1 NAME ${name}${n || ''} /Layout${suffix}/`,
				'1 BIRT',
				'2 DATE 1 JAN 1850',
				`2 PLAC ${place(name)}`,
				'3 MAP',
				`4 LATI ${coord(lat, 'N', 'S')}`,
				`4 LONG ${coord(lon, 'E', 'W')}`
			);
		}
	}
	lines.push('0 TRLR', '');
	const imported = await request.post(`${API_BASE}/gedcom/import`, {
		multipart: {
			file: {
				name: 'map-layout.ged',
				mimeType: 'text/plain',
				buffer: Buffer.from(lines.join('\n'))
			}
		}
	});
	expect(imported.ok(), await imported.text()).toBe(true);
});

interface Drawn {
	label: string;
	x: number;
	y: number;
	r: number;
}

/** Each marker's drawn circle in page pixels, and the map's box. */
function measure(page: Page) {
	return page.evaluate(() => {
		const map = document.querySelector('.map-container')!.getBoundingClientRect();
		const markers = [...document.querySelectorAll('.map-container .marker')].map((m) => {
			const box = m.querySelector('.dot')!.getBoundingClientRect();
			return {
				label: m.getAttribute('aria-label') ?? '',
				x: box.left + box.width / 2,
				y: box.top + box.height / 2,
				r: box.width / 2
			};
		});
		return {
			map: {
				left: map.left,
				right: map.right,
				top: map.top,
				bottom: map.bottom
			},
			markers
		};
	});
}

const inside = (m: Drawn, map: { left: number; right: number; top: number; bottom: number }) =>
	m.x >= map.left && m.x <= map.right && m.y >= map.top && m.y <= map.bottom;

async function openMap(page: Page) {
	await page.goto('/map');
	await expect(
		page.getByRole('button', { name: new RegExp(`Suva Layout${suffix}`) })
	).toBeVisible();
}

for (const width of [375, 768, 1280]) {
	test(`fits every marker without overlap at ${width}px`, async ({ page }) => {
		await page.setViewportSize({ width, height: 900 });
		await openMap(page);
		const { map, markers } = await measure(page);
		for (const m of markers) expect(inside(m, map), `${m.label} is in view`).toBe(true);
		for (const [i, a] of markers.entries()) {
			for (const b of markers.slice(i + 1)) {
				// Half a pixel for sub-pixel rounding of the measured boxes
				expect(Math.hypot(a.x - b.x, a.y - b.y), `${a.label} / ${b.label}`).toBeGreaterThanOrEqual(
					a.r + b.r - 0.5
				);
			}
		}
	});
}

test('focusing an off-screen marker pans it into view, and Reset view refits', async ({ page }) => {
	await page.setViewportSize({ width: 375, height: 800 });
	await openMap(page);
	const initial = await measure(page);

	const box = (await page.locator('.map-container').boundingBox())!;
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
	for (let i = 0; i < 6; i++) await page.mouse.wheel(0, -400);

	let off: Drawn | undefined;
	await expect
		.poll(async () => {
			const { map, markers } = await measure(page);
			off = markers.find((m) => !inside(m, map));
			return off?.label;
		})
		.toBeTruthy();

	const marker = page.locator(`.map-container .marker[aria-label="${off!.label}"]`);
	await marker.focus();
	await expect
		.poll(async () => {
			const { map, markers } = await measure(page);
			const now = markers.find((m) => m.label === off!.label);
			return now ? inside(now, map) : false;
		})
		.toBe(true);
	await expect(marker).toBeFocused();

	await page.getByRole('button', { name: 'Reset view' }).click();
	await expect.poll(async () => (await measure(page)).markers).toEqual(initial.markers);
});

test('the details panel closes on Escape or × and returns focus to its marker', async ({
	page
}) => {
	await page.setViewportSize({ width: 1280, height: 900 });
	await openMap(page);
	const marker = page.getByRole('button', {
		name: new RegExp(`Sydney Layout${suffix}`)
	});
	await expect(marker).toHaveAttribute('aria-expanded', 'false');

	await marker.click();
	await expect(page.getByRole('heading', { name: place('Sydney') })).toBeVisible();
	await expect(marker).toHaveAttribute('aria-expanded', 'true');
	await expect(marker).toHaveClass(/selected/);

	await page.getByRole('link', { name: `Sydney Layout${suffix}` }).focus();
	await page.keyboard.press('Escape');
	await expect(page.getByRole('heading', { name: place('Sydney') })).toHaveCount(0);
	await expect(marker).toBeFocused();
	await expect(marker).toHaveAttribute('aria-expanded', 'false');

	await page.keyboard.press('Enter');
	await page.getByRole('button', { name: 'Close details' }).click();
	await expect(page.getByRole('heading', { name: place('Sydney') })).toHaveCount(0);
	await expect(marker).toBeFocused();
});
