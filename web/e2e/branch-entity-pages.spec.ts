/**
 * A person and a family that exist ONLY on a research branch open, display
 * and save on that branch (#823), and the branch never offers Restore or
 * rollback — both mainline-only (#824, ADR-005).
 *
 * The fixture is made here over the API, on a branch of this spec's own, so
 * nothing any other spec does (merging, or editing the seeded branches) can
 * affect it. The pages themselves are driven through the UI.
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

/** Opens the History panel and checks it is the branch's view, with no rollback. */
async function expectBranchHistoryWithoutRollback(page: Page) {
	await page.getByRole('main').getByRole('button', { name: /History/ }).click();
	await expect(page.getByText(/Restore points and rollback work on the mainline only/)).toBeVisible();
	await expect(page.getByRole('button', { name: 'Restore', exact: true })).toHaveCount(0);
	// The change log is the branch's own: both the create and the edit were
	// made on the branch. Exact, so it finds the origin badge and not the
	// branch banner's prose, which also says "this branch".
	await expect(page.getByText('This branch', { exact: true }).first()).toBeVisible();
	await expect(page.getByText('updated', { exact: true }).first()).toBeVisible();
}

test('a person and a family created on a branch open, save, and offer no rollback', async ({
	page,
	request
}) => {
	const branchName = `Branch-only entities ${Date.now()}`;
	const branch = await post<{ id: string }>(request, '/branches', { name: branchName });
	const onBranch = `?branch=${branch.id}`;

	const person = await post<{ id: string }>(request, `/persons${onBranch}`, {
		given_name: 'Hypatia',
		surname: 'Branchwood',
		gender: 'female'
	});
	const partner = await post<{ id: string }>(request, `/persons${onBranch}`, {
		given_name: 'Theon',
		surname: 'Branchwood',
		gender: 'male'
	});
	const family = await post<{ id: string }>(request, `/families${onBranch}`, {
		partner1_id: partner.id,
		partner2_id: person.id,
		relationship_type: 'marriage'
	});

	// The mainline does not know them.
	expect((await request.get(`${API_BASE}/persons/${person.id}`)).status()).toBe(404);
	expect((await request.get(`${API_BASE}/families/${family.id}`)).status()).toBe(404);

	await switchTo(page, branchName);

	// Person: opens (this used to be "Failed to load"), edits and saves.
	await page.goto(`/persons/${person.id}`);
	await expect(page.getByRole('heading', { level: 1, name: 'Hypatia Branchwood' })).toBeVisible();
	await page.locator('header.page-header').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByLabel('Birth Place').fill('Alexandria');
	await page.getByRole('button', { name: 'Save Changes' }).click();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toHaveCount(0);
	await expect(page.getByText('Alexandria')).toBeVisible();
	await expectBranchHistoryWithoutRollback(page);

	// Family: the same.
	await page.goto(`/families/${family.id}`);
	await expect(page.getByRole('heading', { level: 1, name: /Theon Branchwood & Hypatia Branchwood/ })).toBeVisible();
	await page.locator('header.page-header').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByLabel('Marriage Place').fill('Library of Alexandria');
	await page.getByRole('button', { name: 'Save Changes' }).click();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toHaveCount(0);
	await expect(page.getByText('Library of Alexandria')).toBeVisible();
	await expectBranchHistoryWithoutRollback(page);

	// The saves landed on the branch only.
	const branchPerson = await (await request.get(`${API_BASE}/persons/${person.id}${onBranch}`)).json();
	expect(branchPerson.birth_place).toBe('Alexandria');
	expect((await request.get(`${API_BASE}/persons/${person.id}`)).status()).toBe(404);

	// The API backstop: a branch-scoped rollback is refused.
	const rollback = await request.post(`${API_BASE}/persons/${person.id}/rollback${onBranch}`, {
		data: { target_version: 1 }
	});
	expect(rollback.status()).toBe(409);
	expect((await rollback.json()).code).toBe('rollback_mainline_only');
});

test('a mainline person on a branch shows inherited history and no rollback', async ({
	page,
	request
}) => {
	const branchName = `Inherited history ${Date.now()}`;
	const person = await post<{ id: string }>(request, '/persons', {
		given_name: 'Mainline',
		surname: 'Elder',
		gender: 'unknown'
	});
	await post(request, '/branches', { name: branchName });

	// On the mainline, rollback is offered.
	await page.goto(`/persons/${person.id}`);
	await page.getByRole('main').getByRole('button', { name: /History/ }).click();
	await expect(page.getByRole('button', { name: 'Restore', exact: true })).toBeVisible();

	await switchTo(page, branchName);
	await page.goto(`/persons/${person.id}`);
	await expect(page.getByRole('heading', { level: 1, name: 'Mainline Elder' })).toBeVisible();
	await page.getByRole('main').getByRole('button', { name: /History/ }).click();
	await expect(page.getByText(/Restore points and rollback work on the mainline only/)).toBeVisible();
	await expect(page.getByRole('button', { name: 'Restore', exact: true })).toHaveCount(0);
	await expect(page.getByText('Mainline', { exact: true }).first()).toBeVisible();
});

test('a mainline person corrected after the fork saves on the branch (#844)', async ({
	page,
	request
}) => {
	const branchName = `Post-fork correction ${Date.now()}`;
	const person = await post<{ id: string }>(request, '/persons', {
		given_name: 'Corrected',
		surname: 'Elder',
		gender: 'unknown'
	});
	const personURL = `${API_BASE}/persons/${person.id}`;
	const putMain = async (data: Record<string, unknown>) => {
		const { version } = (await (await request.get(personURL)).json()) as { version: number };
		const response = await request.put(personURL, { data: { ...data, version } });
		expect(response.status(), `PUT main: ${await response.text()}`).toBe(200);
	};

	await putMain({ birth_place: 'Oldtown' });
	const branch = await post<{ id: string }>(request, '/branches', { name: branchName });
	// The mainline corrects the person AFTER the fork. The branch has not
	// touched it, so the branch shows, and must accept, the corrected version.
	await putMain({ surname: 'Elderly' });

	await switchTo(page, branchName);
	await page.goto(`/persons/${person.id}`);
	await expect(page.getByRole('heading', { level: 1, name: 'Corrected Elderly' })).toBeVisible();
	await page.locator('header.page-header').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByLabel('Birth Place').fill('Newtown');
	await page.getByRole('button', { name: 'Save Changes' }).click();
	// This save used to fail with "Version conflict".
	await expect(page.getByRole('button', { name: 'Save Changes' })).toHaveCount(0);
	await expect(page.getByText('Newtown')).toBeVisible();

	// The save landed on the branch, on top of the mainline's correction.
	const onBranch = await (await request.get(`${personURL}?branch=${branch.id}`)).json();
	expect(onBranch.birth_place).toBe('Newtown');
	expect(onBranch.surname).toBe('Elderly');
	const onMain = await (await request.get(personURL)).json();
	expect(onMain.birth_place).toBe('Oldtown');
});
