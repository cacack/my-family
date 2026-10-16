/**
 * The chart keyboard shortcuts the pedigree and descendancy pages advertise
 * (#1091): an arrow key moves the selection to a relative and Enter opens the
 * selected person. The unit tests cover every shortcut against the handler;
 * this checks the keys reach the real charts in a browser.
 */
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

/** The chart node carrying the selection ring. */
function selectedNode(page: Page) {
	return page.locator('.node', { has: page.locator('.selection-ring') });
}

let fatherId: string;
let childId: string;

test.beforeAll(async ({ request }) => {
	const surname = `Keyward${Date.now()}`;
	const father = await post<{ id: string }>(request, '/persons', { given_name: 'Abel', surname, gender: 'male' });
	const mother = await post<{ id: string }>(request, '/persons', { given_name: 'Bea', surname, gender: 'female' });
	const child = await post<{ id: string }>(request, '/persons', { given_name: 'Cal', surname, gender: 'male' });
	const family = await post<{ id: string }>(request, '/families', {
		partner1_id: father.id,
		partner2_id: mother.id,
		relationship_type: 'marriage'
	});
	await post(request, `/families/${family.id}/children`, { person_id: child.id });
	fatherId = father.id;
	childId = child.id;
});

test('on the pedigree, ArrowUp selects the father and Enter opens him', async ({ page }) => {
	await page.goto(`/pedigree/${childId}`);
	await expect(selectedNode(page)).toContainText('Cal');

	await page.getByRole('application', { name: /Pedigree chart/ }).focus();
	await page.keyboard.press('ArrowUp');
	await expect(selectedNode(page)).toContainText('Abel');
	await expect(page.getByRole('status').filter({ hasText: 'Navigated to father' })).toContainText('Abel');

	await page.keyboard.press('Enter');
	await expect(page).toHaveURL(new RegExp(`/persons/${fatherId}$`));
});

test('on the descendancy, ArrowDown selects the child and Enter opens them', async ({ page }) => {
	await page.goto(`/descendancy/${fatherId}`);
	await expect(selectedNode(page)).toContainText('Abel');

	await page.getByRole('application', { name: /Descendancy chart/ }).focus();
	await page.keyboard.press('ArrowDown');
	await expect(selectedNode(page)).toContainText('Cal');

	await page.keyboard.press('Enter');
	await expect(page).toHaveURL(new RegExp(`/persons/${childId}$`));
});

test('on the pedigree, Enter on a toolbar button activates it instead of opening the person', async ({ page }) => {
	await page.goto(`/pedigree/${childId}`);
	await expect(selectedNode(page)).toContainText('Cal');

	const wide = page.getByRole('button', { name: 'Wide' });
	await expect(wide).toHaveAttribute('aria-pressed', 'false');
	await wide.focus();
	await page.keyboard.press('Enter');

	await expect(wide).toHaveAttribute('aria-pressed', 'true');
	await expect(page).toHaveURL(new RegExp(`/pedigree/${childId}`));
});
