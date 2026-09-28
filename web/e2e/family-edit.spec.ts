/**
 * Editing a family's relationship type and marriage date saves, and the saved
 * values are what the page shows after a reload (#848). They used to be
 * dropped by the read model on the live save, so the page reverted to the old
 * values while the event log held the new ones.
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

test('a family edit of relationship type and marriage date is shown after saving', async ({
	page,
	request
}) => {
	const stamp = Date.now();
	const partner1 = await post<{ id: string }>(request, '/persons', {
		given_name: 'Rowan',
		surname: `Editfam${stamp}`,
		gender: 'unknown'
	});
	const partner2 = await post<{ id: string }>(request, '/persons', {
		given_name: 'Sage',
		surname: `Editfam${stamp}`,
		gender: 'unknown'
	});
	const family = await post<{ id: string }>(request, '/families', {
		partner1_id: partner1.id,
		partner2_id: partner2.id,
		relationship_type: 'marriage',
		marriage_date: '1 JUN 1875'
	});

	await page.goto(`/families/${family.id}`);
	await expect(page.locator('.relationship-badge')).toHaveText('marriage');

	await page.locator('header.page-header').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByLabel('Relationship Type').selectOption('partnership');
	await page.getByLabel('Marriage Date').fill('ABT 1880');
	await page.getByRole('button', { name: 'Save Changes' }).click();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toHaveCount(0);

	// The page re-reads the family after saving; the new values are there, and
	// still there on a fresh load.
	await expect(page.locator('.relationship-badge')).toHaveText('partnership');
	await page.reload();
	await expect(page.locator('.relationship-badge')).toHaveText('partnership');

	const saved = await (await request.get(`${API_BASE}/families/${family.id}`)).json();
	expect(saved.relationship_type).toBe('partnership');
	expect(saved.marriage_date?.raw).toBe('ABT 1880');
	expect(saved.partner1_id).toBe(partner1.id);
	expect(saved.partner2_id).toBe(partner2.id);
});
