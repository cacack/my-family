/**
 * A save refused because the record changed elsewhere keeps the user's edits
 * and offers to compare and re-apply them (#899). It spans the stack: the page
 * recognises a version conflict by the server's 409, which the family update
 * used to answer with a 400.
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

async function put(request: APIRequestContext, path: string, data: unknown): Promise<void> {
	const response = await request.put(`${API_BASE}${path}`, { data });
	expect(response.status(), `PUT ${path}: ${await response.text()}`).toBe(200);
}

test('a family save that lost a race keeps the edits and re-applies them on request', async ({
	page,
	request
}) => {
	const stamp = Date.now();
	const partner = await post<{ id: string }>(request, '/persons', {
		given_name: 'Quinn',
		surname: `Conflict${stamp}`,
		gender: 'unknown'
	});
	const family = await post<{ id: string; version: number }>(request, '/families', {
		partner1_id: partner.id,
		marriage_place: 'Springfield'
	});

	await page.goto(`/families/${family.id}`);
	await page.locator('header.page-header').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByLabel('Marriage Place').fill('Evanston');

	// Another tab saves first.
	await put(request, `/families/${family.id}`, { marriage_place: 'Peoria', version: family.version });

	await page.getByRole('button', { name: 'Save Changes' }).click();
	const row = page.getByRole('row', { name: /Marriage Place/ });
	await expect(row).toContainText('Evanston');
	await expect(row).toContainText('Peoria');
	await expect(page.getByLabel('Marriage Place')).toHaveValue('Evanston');

	await page.getByRole('button', { name: 'Save my edits' }).click();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toHaveCount(0);

	const saved = await (await request.get(`${API_BASE}/families/${family.id}`)).json();
	expect(saved.marriage_place).toBe('Evanston');
});

test('a malformed person id reads as not found and offers Retry', async ({ page }) => {
	await page.goto('/persons/not-a-uuid');
	const alert = page.getByRole('alert');
	await expect(alert).toContainText('This person could not be found');
	await expect(alert).not.toContainText('unmarshaling');
	await expect(alert.getByRole('button', { name: 'Retry' })).toBeVisible();
});
