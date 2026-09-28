/**
 * Search and the kinship views follow the active branch (#829): a person
 * created on a branch is found by the header search and the relationship
 * calculator's person picker, and shows up in the descendancy chart and the
 * relationship result — none of which the mainline knows about.
 *
 * The parents are made on the mainline over the API; the child is created
 * through the UI on a branch of this spec's own, so nothing another spec does
 * can affect it.
 */
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

async function switchTo(page: Page, branchName: string) {
	await page.goto('/');
	await page.getByRole('button', { name: /switch research branch/i }).click();
	await page.getByRole('menuitem', { name: branchName }).click();
	await expect(page.getByText(`Working on ${branchName}`)).toBeVisible();
}

test('a person created on a branch is searchable and appears in descendancy and relationship', async ({
	page,
	request
}) => {
	const stamp = Date.now();
	const surname = `Kinshipe${stamp}`;
	const branchName = `Kinship reads ${stamp}`;

	const father = await post<{ id: string }>(request, '/persons', {
		given_name: 'Octavian',
		surname,
		gender: 'male'
	});
	const mother = await post<{ id: string }>(request, '/persons', {
		given_name: 'Livia',
		surname,
		gender: 'female'
	});
	const family = await post<{ id: string }>(request, '/families', {
		partner1_id: father.id,
		partner2_id: mother.id,
		relationship_type: 'marriage'
	});
	const branch = await post<{ id: string }>(request, '/branches', {
		name: branchName
	});
	const onBranch = `?branch=${branch.id}`;

	await switchTo(page, branchName);

	// Create the child through the UI: on a branch, it lands on the branch.
	await page.goto('/persons/add');
	await page.getByLabel('Given Name').fill('Zenobia');
	await page.getByLabel('Surname').fill(surname);
	await page.getByLabel('Gender').selectOption('female');
	await page.getByRole('button', { name: 'Create Person' }).click();
	await expect(page.getByRole('heading', { level: 1, name: `Zenobia ${surname}` })).toBeVisible();
	const childID = page.url().split('/persons/')[1];
	expect((await request.get(`${API_BASE}/persons/${childID}`)).status()).toBe(404);
	expect((await request.get(`${API_BASE}/persons/${childID}${onBranch}`)).status()).toBe(200);

	// Link her to the mainline couple, on the branch only.
	await post(request, `/families/${family.id}/children${onBranch}`, {
		person_id: childID
	});

	// The header search finds her on the branch; the mainline search does not.
	await page.goto('/');
	await page.getByPlaceholder('Search people...').fill(`Zenobia ${surname}`);
	await page.getByRole('option', { name: new RegExp(`Zenobia ${surname}`) }).click();
	await expect(page).toHaveURL(new RegExp(`/persons/${childID}$`));
	await expect(page.getByRole('heading', { level: 1, name: `Zenobia ${surname}` })).toBeVisible();
	const mainSearch = await (await request.get(`${API_BASE}/search?q=Zenobia`)).json();
	expect(mainSearch.items.map((item: { id: string }) => item.id)).not.toContain(childID);

	// The descendancy chart of her father shows her.
	await page.goto(`/descendancy/${father.id}`);
	await expect(page.getByText('Zenobia', { exact: false }).first()).toBeVisible();

	// The relationship calculator: pick her with the person picker (a search),
	// against her father, and calculate.
	await page.goto(`/relationship?personB=${father.id}`);
	await expect(page.getByText(`Octavian ${surname}`).first()).toBeVisible();
	await page.getByLabel('First Person').fill('Zenobia');
	await page.getByRole('option', { name: new RegExp(`Zenobia ${surname}`) }).click();
	await page.getByRole('button', { name: 'Calculate Relationship' }).click();
	const results = page.getByRole('region', { name: 'Relationship results' });
	await expect(results.getByText('parent', { exact: true }).first()).toBeVisible();
	await expect(results.getByRole('alert')).toHaveCount(0);

	// None of it exists on the mainline.
	expect((await request.get(`${API_BASE}/relationship/${childID}/${father.id}`)).status()).toBe(
		404
	);
	const mainDescendancy = await (await request.get(`${API_BASE}/descendancy/${father.id}`)).json();
	expect(mainDescendancy.root.children ?? []).toHaveLength(0);
});
