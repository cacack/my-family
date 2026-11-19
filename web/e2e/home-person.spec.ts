/**
 * Smoke: choosing who you are in the tree (#1034) against the real binary.
 *
 * The component tests cover each piece against a mocked client. What only the
 * real stack shows is the round trip: the choice is kept by the server, the
 * relationship calculator words the line on another person's page, and the
 * chart routes without a person start from the choice.
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

async function createPerson(request: APIRequestContext, given: string, gender: string) {
	const res = await request.post(`${API_BASE}/persons`, {
		data: { given_name: given, surname: 'Homeward', gender }
	});
	expect(res.ok()).toBe(true);
	return (await res.json()).id as string;
}

test.afterEach(async ({ request }) => {
	// Leave no home person behind for the whole-app guards.
	await request.delete(`${API_BASE}/settings/home-person`);
});

test('choosing who you are words every relationship to you and starts the charts from you', async ({
	page,
	request
}) => {
	await request.delete(`${API_BASE}/settings/home-person`);
	const mother = await createPerson(request, 'Margit', 'female');
	const child = await createPerson(request, 'Ilse', 'female');
	const family = await request.post(`${API_BASE}/families`, { data: { partner1_id: mother } });
	expect(family.ok()).toBe(true);
	const familyId = (await family.json()).id as string;
	expect((await request.post(`${API_BASE}/families/${familyId}/children`, { data: { person_id: child } })).ok()).toBe(
		true
	);

	// With no one chosen, a chart route says how to choose.
	await page.goto('/pedigree');
	await expect(page.getByText('Choose who you are to start here')).toBeVisible();

	await page.goto(`/persons/${child}`);
	await page.getByRole('button', { name: 'This is me' }).click();
	await expect(page.getByText('This is you')).toBeVisible();

	await page.goto(`/persons/${mother}`);
	const relation = page.getByRole('link', { name: 'Your mother' });
	await expect(relation).toBeVisible();
	await relation.click();
	await expect(page).toHaveURL(new RegExp(`/relationship\\?personA=${child}&personB=${mother}`));

	await page.goto('/descendancy');
	await expect(page).toHaveURL(new RegExp(`/descendancy/${child}$`));

	// Cleared from the dashboard, the person page offers the choice again.
	await page.goto('/');
	await page.getByTestId('home-person-card').getByRole('button', { name: 'Not me' }).click();
	await expect(page.getByTestId('home-person-card')).toContainText('This is me');
	await page.goto(`/persons/${mother}`);
	await expect(page.getByRole('button', { name: 'This is me' })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Your mother' })).toHaveCount(0);
});
