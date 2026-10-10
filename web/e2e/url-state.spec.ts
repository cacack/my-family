/**
 * Page state lives in the URL, so Back from a detail page and a reload return
 * to the same view (#901). Unit tests cover each page against a fake router;
 * this checks the real SvelteKit history does the same.
 */
import { expect, test } from '@playwright/test';
import { API_BASE } from './seed';

// Enough people for a third page of 20, whatever else the suite has seeded.
const EXTRA_PEOPLE = 45;

test.beforeAll(async ({ request }) => {
	const stamp = Date.now();
	for (let i = 0; i < EXTRA_PEOPLE; i++) {
		const response = await request.post(`${API_BASE}/persons`, {
			data: {
				given_name: `Pager${String(i).padStart(2, '0')}`,
				surname: `Urlstate${stamp}`,
				birth_date: `1 JAN ${1700 + i}`
			}
		});
		expect(response.status(), await response.text()).toBe(201);
	}
});

test('Back from a person opened on /persons page 3, sorted by birth date, returns to that page and sort', async ({
	page
}) => {
	await page.goto('/persons');
	await page.getByLabel('Sort by:').selectOption('birth_date');
	await expect(page).toHaveURL(/\/persons\?sort=birth_date$/);
	await page.getByRole('button', { name: 'Next' }).click();
	await expect(page.getByText(/^Page 2 of \d+$/)).toBeVisible();
	await page.getByRole('button', { name: 'Next' }).click();
	await expect(page.getByText(/^Page 3 of \d+$/)).toBeVisible();
	await expect(page).toHaveURL(/\/persons\?sort=birth_date&page=3$/);

	const firstCard = page.locator('.persons-grid a').first();
	const firstName = await firstCard.innerText();
	await firstCard.click();
	await expect(page).toHaveURL(/\/persons\/[0-9a-f-]+$/);

	await page.goBack();
	await expect(page).toHaveURL(/\/persons\?sort=birth_date&page=3$/);
	await expect(page.getByText(/^Page 3 of \d+$/)).toBeVisible();
	await expect(page.getByLabel('Sort by:')).toHaveValue('birth_date');
	await expect(page.locator('.persons-grid a').first()).toHaveText(firstName);

	// Back again steps through the pages (pushes), not the sort (a replace).
	await page.goBack();
	await expect(page.getByText(/^Page 2 of \d+$/)).toBeVisible();
	await expect(page.getByLabel('Sort by:')).toHaveValue('birth_date');

	// A reload keeps it too.
	await page.reload();
	await expect(page.getByText(/^Page 2 of \d+$/)).toBeVisible();
	await expect(page.getByLabel('Sort by:')).toHaveValue('birth_date');
});

test('Back from a search result returns to the search and its results', async ({ page }) => {
	await page.goto('/search');
	await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Pager01');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	await expect(page).toHaveURL(/\/search\?q=Pager01$/);
	const result = page.locator('.results-table .person-link').first();
	await expect(result).toContainText('Pager01');

	await result.click();
	await expect(page).toHaveURL(/\/persons\/[0-9a-f-]+$/);

	await page.goBack();
	await expect(page.getByRole('textbox', { name: 'Name', exact: true })).toHaveValue('Pager01');
	await expect(page.locator('.results-table .person-link').first()).toContainText('Pager01');
});
