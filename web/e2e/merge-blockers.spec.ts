/**
 * Merge blockers (#831): a branch whose merge would break two cross-entity
 * references shows both by name before merging, highlights the rows involved,
 * and each is fixed from the review with its one-click fix - after which the
 * merge goes through.
 *
 * The fixture is this spec's own, built over the real API, so nothing another
 * spec does can change what it sees:
 *
 * - on the mainline, a source and a person, then the branch;
 * - on the branch, a new person, a family naming them, and a citation of the
 *   mainline source;
 * - on the mainline again, the source is deleted - so the branch's citation
 *   would cite nothing (blocker one).
 *
 * Leaving the new person out then strands the family that names them
 * (blocker two), whose fix is to include the person again.
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), await response.text()).toBe(201);
	return (await response.json()) as T;
}

test('a branch with two merge blockers shows both by name and each is fixed from the review', async ({
	page,
	request
}) => {
	const stamp = Date.now();
	const sourceTitle = `Blocker Register ${stamp}`;
	const citationName = `${sourceTitle} (Birth)`;

	// --- Mainline, then the branch ---------------------------------------
	const source = await post<{ id: string; version: number }>(request, '/sources', {
		source_type: 'census',
		title: sourceTitle
	});
	const anchorName = `Ada Anchor${stamp}`;
	const anchor = await post<{ id: string }>(request, '/persons', {
		given_name: 'Ada',
		surname: `Anchor${stamp}`
	});
	const branchName = `E2E Blocked ${stamp}`;
	const branch = await post<{ id: string }>(request, '/branches', { name: branchName });

	// --- The branch's research ---------------------------------------------
	const newcomerName = `Pat Newcomer${stamp}`;
	const newcomer = await post<{ id: string }>(request, `/persons?branch=${branch.id}`, {
		given_name: 'Pat',
		surname: `Newcomer${stamp}`
	});
	await post(request, `/families?branch=${branch.id}`, {
		partner1_id: newcomer.id,
		partner2_id: anchor.id,
		relationship_type: 'marriage'
	});
	const familyName = `${newcomerName} & ${anchorName}`;
	await post(request, `/citations?branch=${branch.id}`, {
		source_id: source.id,
		fact_type: 'person_birth',
		fact_owner_id: anchor.id,
		page: '7'
	});

	// --- The mainline deletes the cited source -----------------------------
	const deleted = await request.delete(`${API_BASE}/sources/${source.id}?version=${source.version}`);
	expect(deleted.status(), await deleted.text()).toBe(204);

	await page.goto(`/branches/${branch.id}`);
	await expect(page.getByRole('heading', { level: 1, name: branchName })).toBeVisible();

	const panel = page.getByTestId('merge-blockers');
	const reviewAndMerge = page.getByRole('button', { name: 'Review & merge' });

	// --- Blocker one, by name, before any merge attempt ---------------------
	await expect(panel.getByRole('heading', { name: '1 merge blocker' })).toBeVisible();
	await expect(
		panel.getByText(
			`Citation "${citationName}" cites source "${sourceTitle}", which will not exist on the mainline.`
		)
	).toBeVisible();
	await expect(reviewAndMerge).toBeDisabled();

	// --- Leaving the new person out adds blocker two ------------------------
	// Every change entry of an entity carries the same toggle, so any one will do.
	const leaveOutNewcomer = page
		.getByRole('checkbox', { name: `Leave out of the merge: ${newcomerName}`, exact: true })
		.first();
	await leaveOutNewcomer.click();
	await expect(panel.getByRole('heading', { name: '2 merge blockers' })).toBeVisible();
	await expect(
		panel.getByText(
			`Family "${familyName}" names person "${newcomerName}", who will not exist on the mainline.`
		)
	).toBeVisible();
	await expect(page.getByText('2 merge blockers to fix.')).toBeVisible();

	// The rows involved are marked on the branch side.
	const branchSide = page.getByTestId('branch-changes');
	await expect(branchSide.locator('li.blocked').filter({ hasText: familyName })).not.toHaveCount(0);
	await expect(branchSide.locator('li.blocked').filter({ hasText: citationName })).not.toHaveCount(0);

	// --- Each is fixed with one click, re-checked as it goes ----------------
	await panel.getByRole('button', { name: `Include ${newcomerName}` }).click();
	await expect(panel.getByRole('heading', { name: '1 merge blocker' })).toBeVisible();
	await expect(leaveOutNewcomer).not.toBeChecked();

	await panel.getByRole('button', { name: `Also leave out ${citationName}` }).click();
	await expect(panel).toHaveCount(0);
	await expect(
		page.getByRole('checkbox', { name: `Leave out of the merge: ${citationName}` }).first()
	).toBeChecked();
	await expect(reviewAndMerge).toBeEnabled();

	// --- And the merge goes through -----------------------------------------
	await reviewAndMerge.click();
	const dialog = page.getByRole('alertdialog');
	await expect(dialog.getByText(`Merge ${branchName} into the mainline?`)).toBeVisible();
	await dialog.getByRole('button', { name: 'Merge branch' }).click();
	await expect(dialog.getByText(`Merged ${branchName} into the mainline`)).toBeVisible();
	const leftBehind = dialog.locator('dl.summary > div').filter({ hasText: 'Entities left behind' });
	await expect(leftBehind).toContainText('1');
	await dialog.getByRole('button', { name: 'Done' }).click();

	// The included person reached the mainline.
	await page.goto(`/persons/${newcomer.id}`);
	await expect(page.getByText(newcomerName).first()).toBeVisible();
});
