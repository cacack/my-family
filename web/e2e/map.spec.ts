/**
 * The map works with no internet (#909): the world outline is embedded in the
 * binary, so with every request off the server blocked the map still draws its
 * countries and markers, and a marker opens a panel naming its people.
 *
 * Coordinates only arrive through GEDCOM import, so the spec imports its own
 * person with a place and surname no other spec uses.
 */
import { expect, test } from '@playwright/test';
import { API_BASE, BASE_URL } from './seed';

test('the map draws countries and named markers with external network blocked', async ({
	page,
	request
}) => {
	const surname = `Cartographer${Date.now().toString(36)}`;
	const place = `Mapleton ${surname}`;
	const gedcom = [
		'0 HEAD',
		'1 GEDC',
		'2 VERS 5.5.1',
		'1 CHAR UTF-8',
		'0 @I1@ INDI',
		`1 NAME Ada /${surname}/`,
		'1 BIRT',
		'2 DATE 1 JAN 1850',
		`2 PLAC ${place}`,
		'3 MAP',
		'4 LATI S41.2865',
		'4 LONG E174.7762',
		'0 TRLR',
		''
	].join('\n');
	const imported = await request.post(`${API_BASE}/gedcom/import`, {
		multipart: { file: { name: 'map.ged', mimeType: 'text/plain', buffer: Buffer.from(gedcom) } }
	});
	expect(imported.ok(), await imported.text()).toBe(true);

	const external: string[] = [];
	await page.route('**/*', (route) => {
		const url = route.request().url();
		if (url.startsWith(BASE_URL)) return route.continue();
		external.push(url);
		return route.abort();
	});

	await page.goto('/map');
	const marker = page.getByRole('button', { name: `${place}: 1 birth` });
	await expect(marker).toBeVisible();
	await expect.poll(() => page.locator('.country').count()).toBeGreaterThan(100);
	await expect(page.getByText('world outline could not be loaded')).toHaveCount(0);

	await marker.focus();
	await page.keyboard.press('Enter');
	await expect(page.getByRole('heading', { name: place })).toBeVisible();
	await expect(page.getByRole('link', { name: `Ada ${surname}` })).toHaveAttribute(
		'href',
		/^\/persons\/[0-9a-f-]+$/
	);
	expect(external).toEqual([]);
});
