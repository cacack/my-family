/**
 * Smoke: the branch UI explains live-overlay semantics and says how far the
 * mainline has moved under an open branch (#837).
 *
 * The component tests cover the wording and the link target. What only the
 * real binary can show is the wiring: the seeded mainline edit made after the
 * switcher branch forked comes back through `include_drift` on the list and
 * through `GET /branches/{id}/drift` on the banner, and the indicator's link
 * lands on the compare page's mainline section.
 */
import { expect, test } from '@playwright/test';
import { API_BASE, readSeed } from './seed';

// The seed edits the merge person on the mainline after the switcher branch
// forked, and nothing edits the switcher person on the mainline. Other specs
// may add mainline work (a merge replays onto it), so the total is matched
// loosely; the touched count is exact.
const DRIFT_TEXT =
	/Mainline changed [\d,]+\+?\s+times? since you branched,\s+0 on entities this branch touched/;

test('a branch card shows how far the mainline has moved', async ({ page }) => {
	const { branchId, branchName } = readSeed().switcher;

	await page.goto('/branches');

	const card = page.locator('article.branch-card', { hasText: branchName });
	await expect(card.getByTestId('branch-drift')).toHaveText(DRIFT_TEXT);
	await expect(card.getByRole('link', { name: 'Open compare' })).toHaveAttribute(
		'href',
		`/branches/${branchId}#main-changes`
	);
});

test('the create dialog explains that branches are live', async ({ page }) => {
	await page.goto('/branches');

	await page.getByRole('button', { name: 'New branch' }).click();

	await expect(page.getByTestId('live-overlay-note')).toContainText(
		'Branches are live, not frozen'
	);
	await expect(page.getByTestId('live-overlay-note')).toContainText('never need to rebase');
	await page.getByRole('button', { name: 'Cancel' }).click();
});

test('the banner explains live semantics and links the drift to compare', async ({ page }) => {
	const { branchId, branchName } = readSeed().switcher;

	await page.goto('/');
	await page.getByRole('button', { name: /switch research branch/i }).click();
	await page.getByRole('menuitem', { name: branchName }).click();

	const banner = page.locator('.branch-banner');
	await expect(banner.getByTestId('branch-drift')).toHaveText(DRIFT_TEXT);

	await banner.getByText('How branches work').click();
	await expect(banner.getByText(/live view over the mainline, not a frozen copy/)).toBeVisible();

	await banner.getByRole('link', { name: 'Open compare' }).click();
	await expect(page).toHaveURL(new RegExp(`/branches/${branchId}#main-changes$`));
	await expect(page.locator('#main-changes')).toBeInViewport();
});

test('the banner re-reads the counts when the tab regains focus and after a branch write', async ({
	page,
	request
}) => {
	// A fixture of this test's own, so nothing another spec does can move it.
	// Both people exist before the fork (rule 1 in global-setup).
	const created = await request.post(`${API_BASE}/persons`, {
		data: { given_name: 'Ottoline', surname: 'Driftwood', birth_place: 'Fork Town' }
	});
	expect(created.status(), await created.text()).toBe(201);
	const person = (await created.json()) as { id: string; version: number };
	const other = await request.post(`${API_BASE}/persons`, {
		data: { given_name: 'Percival', surname: 'Driftwood' }
	});
	expect(other.status(), await other.text()).toBe(201);
	const otherPerson = (await other.json()) as { id: string };

	const branchName = `Drift refresh ${Date.now()}`;
	const branchResponse = await request.post(`${API_BASE}/branches`, { data: { name: branchName } });
	expect(branchResponse.status(), await branchResponse.text()).toBe(201);
	const branch = (await branchResponse.json()) as { id: string };

	// The branch edits the person first (rule 2 in global-setup).
	const branchEdit = await request.put(`${API_BASE}/persons/${person.id}?branch=${branch.id}`, {
		data: { birth_place: 'Branch Town', version: person.version }
	});
	expect(branchEdit.status(), await branchEdit.text()).toBe(200);

	await page.goto('/');
	await page.getByRole('button', { name: /switch research branch/i }).click();
	await page.getByRole('menuitem', { name: branchName }).click();
	const banner = page.locator('.branch-banner');
	await expect(banner.getByText('Mainline unchanged since you branched.')).toBeVisible();

	// Main moves in "another tab": the banner catches up when this one regains focus.
	const mainEdit = await request.put(`${API_BASE}/persons/${person.id}`, {
		data: { birth_place: 'Main Town', version: person.version }
	});
	expect(mainEdit.status(), await mainEdit.text()).toBe(200);
	await page.evaluate(() => window.dispatchEvent(new Event('focus')));
	await expect(banner.getByTestId('branch-drift')).toHaveText(
		/Mainline changed 1\s+time since you branched,\s+1 on entities this branch touched/
	);

	// A save on the branch re-reads the counts with no reload or focus change.
	await page.goto(`/persons/${otherPerson.id}`);
	await expect(page.getByRole('heading', { level: 1, name: 'Percival Driftwood' })).toBeVisible();
	await expect(banner.getByTestId('branch-drift')).toBeVisible();
	await page.locator('header.page-header').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByLabel('Birth Place').fill('Branch Village');
	const reread = page.waitForRequest(
		(req) => req.method() === 'GET' && req.url().includes(`/api/v1/branches/${branch.id}/drift`)
	);
	await page.getByRole('button', { name: 'Save Changes' }).click();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toHaveCount(0);
	await reread;
	await expect(banner.getByTestId('branch-drift')).toHaveText(
		/Mainline changed 1\s+time since you branched,\s+1 on entities this branch touched/
	);
});
