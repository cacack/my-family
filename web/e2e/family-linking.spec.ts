/**
 * The milestone's own example, entirely in the UI (#826): on a research branch,
 * create two people and a child, create a family linking the two as partners,
 * add the child to it, see all of it in the branch compare, merge, and find the
 * family — partners and child — on the mainline.
 *
 * Only the branch itself is made over the API, so the spec owns a branch no
 * other spec can merge or edit. Everything the issue says could not be done in
 * the UI is done through the UI here.
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

/** Creates a person through the add form and returns its id. */
async function createPerson(page: Page, given: string, surname: string, gender: string): Promise<string> {
	await page.goto('/persons/add');
	await page.getByLabel('Given Name').fill(given);
	await page.getByLabel('Surname').fill(surname);
	await page.getByLabel('Gender').selectOption(gender);
	await page.getByRole('button', { name: 'Create Person' }).click();
	await expect(page.getByRole('heading', { level: 1, name: `${given} ${surname}` })).toBeVisible();
	return page.url().split('/persons/')[1];
}

/** Picks a person in a PersonSelector by searching for their full name. */
async function pick(page: Page, label: string, fullName: string) {
	await page.getByRole('combobox', { name: label }).fill(fullName);
	await page.getByRole('option', { name: new RegExp(fullName) }).click();
	await expect(page.getByRole('button', { name: `Clear ${label}: ${fullName}` })).toBeVisible();
}

/** A surname of letters only, unique to this run, so search matches it as one token. */
function uniqueSurname(): string {
	const letters = Date.now()
		.toString()
		.split('')
		.map((digit) => String.fromCharCode(97 + Number(digit)))
		.join('');
	return `Linkwood${letters}`;
}

test('on a branch, link two partners and a child in the UI, compare, and merge to main', async ({
	page,
	request
}) => {
	const surname = uniqueSurname();
	const branchName = `Mary is John's daughter ${surname}`;
	const branch = await post<{ id: string }>(request, '/branches', { name: branchName });
	const onBranch = `?branch=${branch.id}`;

	await switchTo(page, branchName);

	const johnId = await createPerson(page, 'John', surname, 'male');
	const annId = await createPerson(page, 'Ann', surname, 'female');
	const maryId = await createPerson(page, 'Mary', surname, 'female');

	// --- Create the family with both partners picked in the form. ---
	await page.goto('/families/add');
	await expect(page.getByText(/Partners can be added after creating the family/)).toHaveCount(0);
	await pick(page, 'Partner 1', `John ${surname}`);
	await pick(page, 'Partner 2', `Ann ${surname}`);
	await page.getByLabel('Relationship Type').selectOption('marriage');
	await page.getByRole('button', { name: 'Create Family' }).click();

	const familyHeading = page.getByRole('heading', {
		level: 1,
		name: `John ${surname} & Ann ${surname}`
	});
	await expect(familyHeading).toBeVisible();
	const familyId = page.url().split('/families/')[1];

	// --- Add Mary as a child through the dialog. ---
	await page.getByRole('button', { name: 'Add child' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByText(`as a child of John ${surname} & Ann ${surname}`)).toBeVisible();
	await dialog.getByRole('combobox', { name: 'Child' }).fill(`Mary ${surname}`);
	await dialog.getByRole('option', { name: new RegExp(`Mary ${surname}`) }).click();
	await dialog.getByLabel('Relationship to the parents').selectOption('biological');
	await dialog.getByRole('button', { name: `Add Mary ${surname}` }).click();
	await expect(dialog).toHaveCount(0);

	const children = page.locator('.children-list');
	await expect(children.getByRole('link', { name: `Mary ${surname}` })).toBeVisible();
	await expect(page.getByTestId('announcer')).toHaveText(`Mary ${surname} added as a child`);

	// Mary's own page now names her parents, and no longer offers Add parents.
	await page.goto(`/persons/${maryId}`);
	await expect(page.getByRole('link', { name: `John ${surname} & Ann ${surname}` })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Add parents' })).toHaveCount(0);

	// It is all on the branch and nothing is on the mainline yet.
	const onBranchFamily = await (await request.get(`${API_BASE}/families/${familyId}${onBranch}`)).json();
	expect(onBranchFamily.partner1_id).toBe(johnId);
	expect(onBranchFamily.partner2_id).toBe(annId);
	expect(onBranchFamily.children.map((c: { person_id: string }) => c.person_id)).toEqual([maryId]);
	expect((await request.get(`${API_BASE}/families/${familyId}`)).status()).toBe(404);

	// --- Compare: the family and all three people are branch changes. ---
	await page.goto(`/branches/${branch.id}`);
	await expect(page.getByRole('heading', { level: 1, name: branchName })).toBeVisible();
	const branchSide = page.getByTestId('branch-changes');
	await expect(branchSide.locator(`a[href="/families/${familyId}"]`).first()).toBeVisible();
	for (const id of [johnId, annId, maryId]) {
		await expect(branchSide.locator(`a[href="/persons/${id}"]`).first()).toBeVisible();
	}
	await expect(page.getByTestId('main-changes').locator(`a[href="/families/${familyId}"]`)).toHaveCount(0);

	// --- Merge. ---
	const reviewAndMerge = page.getByRole('button', { name: 'Review & merge' });
	await expect(reviewAndMerge).toBeEnabled();
	await reviewAndMerge.click();
	const confirm = page.getByRole('alertdialog');
	await confirm.getByRole('button', { name: 'Merge branch' }).click();
	await expect(confirm.getByText(`Merged ${branchName} into the mainline`)).toBeVisible();
	await confirm.getByRole('button', { name: 'Return to mainline' }).click();
	await expect(page.getByText(`Working on ${branchName}`)).toHaveCount(0);

	// --- The mainline has the family with its partners and child. ---
	await page.goto(`/families/${familyId}`);
	await expect(familyHeading).toBeVisible();
	await expect(page.locator('a.partner-card', { hasText: `John ${surname}` })).toHaveAttribute(
		'href',
		`/persons/${johnId}`
	);
	await expect(page.locator('a.partner-card', { hasText: `Ann ${surname}` })).toHaveAttribute(
		'href',
		`/persons/${annId}`
	);
	await expect(page.locator('.children-list').getByRole('link', { name: `Mary ${surname}` })).toBeVisible();

	const onMain = await (await request.get(`${API_BASE}/families/${familyId}`)).json();
	expect(onMain.partner1_id).toBe(johnId);
	expect(onMain.partner2_id).toBe(annId);
	expect(onMain.relationship_type).toBe('marriage');
	expect(onMain.children.map((c: { person_id: string }) => c.person_id)).toEqual([maryId]);
});

test('on the mainline, change and clear a partner and remove a child in the UI', async ({
	page,
	request
}) => {
	const surname = uniqueSurname();
	const make = (given: string) =>
		post<{ id: string }>(request, '/persons', { given_name: given, surname, gender: 'unknown' });
	const [rowan, sage, ash, wren] = await Promise.all([make('Rowan'), make('Sage'), make('Ash'), make('Wren')]);
	const family = await post<{ id: string }>(request, '/families', {
		partner1_id: rowan.id,
		partner2_id: sage.id,
		relationship_type: 'partnership'
	});
	await post(request, `/families/${family.id}/children`, { person_id: wren.id });

	await page.goto(`/families/${family.id}`);
	await expect(page.getByRole('heading', { level: 1, name: `Rowan ${surname} & Sage ${surname}` })).toBeVisible();

	// Swap partner 1 for Ash and remove partner 2. With both slots empty the
	// form will not save: a family must keep at least one partner.
	await page.locator('header.page-header').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByRole('button', { name: `Clear Partner 1: Rowan ${surname}` }).click();
	await page.getByRole('button', { name: `Clear Partner 2: Sage ${surname}` }).click();
	await expect(page.getByTestId('partner-required')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toBeDisabled();
	await pick(page, 'Partner 1', `Ash ${surname}`);
	await expect(page.getByTestId('partner-required')).toHaveCount(0);
	await page.getByRole('button', { name: 'Save Changes' }).click();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toHaveCount(0);
	await expect(page.getByRole('heading', { level: 1, name: `Ash ${surname}` })).toBeVisible();

	let saved = await (await request.get(`${API_BASE}/families/${family.id}`)).json();
	expect(saved.partner1_id).toBe(ash.id);
	expect(saved.partner2_id).toBeUndefined();

	// Remove the child, with confirmation; the person is kept.
	await page.getByRole('button', { name: `Remove Wren ${surname} from this family` }).click();
	const confirm = page.getByRole('alertdialog');
	await expect(confirm.getByText(`Wren ${surname} will no longer be recorded as a child`)).toBeVisible();
	await confirm.getByRole('button', { name: 'Remove child' }).click();
	await expect(confirm).toHaveCount(0);
	await expect(page.getByText('No children recorded')).toBeVisible();
	await expect(page.getByTestId('announcer')).toHaveText(`Wren ${surname} removed from this family`);

	saved = await (await request.get(`${API_BASE}/families/${family.id}`)).json();
	expect(saved.children ?? []).toHaveLength(0);
	expect((await request.get(`${API_BASE}/persons/${wren.id}`)).status()).toBe(200);

	// Wren's page offers Add parents, which opens the form with her as the child.
	await page.goto(`/persons/${wren.id}`);
	await page.getByRole('link', { name: 'Add parents' }).click();
	await expect(page.getByTestId('child-note')).toContainText(`Wren ${surname} will be added as a child`);
	await expect(page.getByRole('button', { name: 'Create Family' })).toBeDisabled();
	await pick(page, 'Partner 1', `Rowan ${surname}`);
	await page.getByRole('button', { name: 'Create Family' }).click();
	await expect(page.getByRole('heading', { level: 1, name: `Rowan ${surname}` })).toBeVisible();
	await expect(page.locator('.children-list').getByRole('link', { name: `Wren ${surname}` })).toBeVisible();
});
