/**
 * Two persons merged on a research branch (#834): the merge page offers the
 * merge on a branch, says it lands there only, and the mainline keeps both
 * persons until the branch itself is merged.
 *
 * The fixture is made over the API on a branch of this spec's own, so nothing
 * another spec does can affect it; the merge itself is driven through the UI.
 */
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(
	request: APIRequestContext,
	path: string,
	data: unknown,
	status = 201
): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(status);
	return (await response.json()) as T;
}

async function switchTo(page: Page, branchName: string) {
	await page.goto('/');
	await page.getByRole('button', { name: /switch research branch/i }).click();
	await page.getByRole('menuitem', { name: branchName }).click();
	await expect(page.getByText(`Working on ${branchName}`)).toBeVisible();
}

test('two persons merge on a branch and the mainline is unaffected until the branch merges', async ({
	page,
	request
}) => {
	const stamp = Date.now();
	const surname = `Mergetest${stamp}`;
	const survivor = await post<{ id: string }>(request, '/persons', {
		given_name: 'Rowan',
		surname,
		gender: 'male'
	});
	const merged = await post<{ id: string }>(request, '/persons', {
		given_name: 'Rowan',
		surname,
		gender: 'male',
		birth_place: 'Riverton'
	});
	const branchName = `Same person ${stamp}`;
	const branch = await post<{ id: string }>(request, '/branches', { name: branchName });
	const onBranch = `?branch=${branch.id}`;

	await switchTo(page, branchName);
	await page.goto(`/quality/merge/${survivor.id}/${merged.id}`);
	await expect(page.getByText(`Merging on ${branchName}`)).toBeVisible();
	await expect(page.getByText(/The mainline keeps both persons/)).toBeVisible();

	// The survivor has no birth place, so the merged person's is preselected.
	await page.getByRole('button', { name: 'Merge persons' }).click();
	await expect(page.getByText('Merged successfully')).toBeVisible();
	await expect(page).toHaveURL(new RegExp(`/persons/${survivor.id}$`));
	await expect(page.getByText('Riverton')).toBeVisible();

	// The branch has one person; the mainline still has both.
	expect((await request.get(`${API_BASE}/persons/${merged.id}${onBranch}`)).status()).toBe(404);
	const onBranchSurvivor = await (
		await request.get(`${API_BASE}/persons/${survivor.id}${onBranch}`)
	).json();
	expect(onBranchSurvivor.birth_place).toBe('Riverton');
	expect((await request.get(`${API_BASE}/persons/${merged.id}`)).status()).toBe(200);
	const mainSurvivor = await (await request.get(`${API_BASE}/persons/${survivor.id}`)).json();
	expect(mainSurvivor.birth_place ?? '').toBe('');

	// Merging the branch carries the person merge onto the mainline.
	await post(request, `/branches/${branch.id}/merge`, { note: 'same person' }, 200);
	expect((await request.get(`${API_BASE}/persons/${merged.id}`)).status()).toBe(404);
	const afterMerge = await (await request.get(`${API_BASE}/persons/${survivor.id}`)).json();
	expect(afterMerge.birth_place).toBe('Riverton');
});
