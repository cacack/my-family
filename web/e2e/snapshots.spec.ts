/**
 * Smoke: research snapshots can be created, deleted and compared against the
 * real binary.
 *
 * The component tests cover rendering against a mocked client. What only the
 * real stack can show is that the comparison the server computes between two
 * seeded snapshots reads correctly in the UI: the mainline edit made between
 * them appears as a field-level diff, and the branch edit made in the same
 * window does not. On a branch (#839) a snapshot is the branch's own, and
 * "compare to now" lists the branch edit made since it.
 */
import { expect, test } from '@playwright/test';
import { API_BASE, readSeed } from './seed';

// Read the seed inside each test, not at module scope: `playwright test --list`
// and editor test discovery load spec files WITHOUT running globalSetup, so a
// module-scope read fails collection with ENOENT instead of listing tests.

test('a snapshot can be created and then deleted after confirmation', async ({ page }) => {
	const name = 'E2E Courthouse Trip';

	await page.goto('/snapshots');
	await expect(page.getByRole('heading', { level: 1, name: 'Research Snapshots' })).toBeVisible();

	await page.getByRole('button', { name: 'New snapshot' }).click();
	const createDialog = page.getByRole('dialog');
	await createDialog.getByLabel('Name').fill(name);
	await createDialog.getByLabel('Description (optional)').fill('Probate files');
	await createDialog.getByRole('button', { name: 'Create snapshot' }).click();

	const card = page.getByRole('listitem').filter({ has: page.getByRole('heading', { name }) });
	await expect(card).toBeVisible();
	await expect(card.getByText('Probate files')).toBeVisible();

	await card.getByRole('button', { name: `Delete snapshot ${name}` }).click();
	const confirm = page.getByRole('alertdialog');
	await expect(confirm.getByText('Delete this snapshot?')).toBeVisible();
	await confirm.getByRole('button', { name: 'Delete snapshot' }).click();

	await expect(page.getByRole('heading', { name })).toHaveCount(0);
});

test('comparing two snapshots shows the mainline edit between them, and not the branch edit', async ({
	page
}) => {
	const { snapshots, merge } = readSeed();

	await page.goto('/snapshots');
	await page.getByLabel('From', { exact: true }).selectOption(snapshots.beforeId);
	await page.getByLabel('To', { exact: true }).selectOption(snapshots.afterId);
	await page.getByRole('button', { name: 'Compare', exact: true }).click();

	await expect(page).toHaveURL(/\/snapshots\/compare\?/);
	await expect(page.getByRole('heading', { level: 1, name: 'Snapshot comparison' })).toBeVisible();
	await expect(page.getByText(snapshots.beforeName)).toBeVisible();
	await expect(page.getByText(snapshots.afterName)).toBeVisible();

	const changes = page.getByRole('list', { name: 'Changes, oldest first' });
	const edit = changes
		.getByRole('listitem')
		.filter({ hasText: 'updated' })
		.filter({ has: page.getByRole('link', { name: merge.person.name, exact: true }) });
	// The mainline edit's new value, as a diff. Asserting the value, not just the
	// person's link, is what makes this non-vacuous: the create alone would
	// render the link too.
	await expect(edit.getByText(merge.person.mainBirthPlace)).toBeVisible();

	// The branch wrote into the same window of the shared log; it is not the
	// mainline's history.
	await expect(changes.getByText(merge.person.branchBirthPlace)).toHaveCount(0);
});

test('a snapshot taken on a branch compares to now and shows the branch edit made since', async ({
	page,
	request
}) => {
	const branchName = `Snapshot branch ${Date.now()}`;
	const snapshotName = `E2E Pre-DNA on branch ${Date.now()}`;
	const person = await request.post(`${API_BASE}/persons`, {
		data: { given_name: 'Snapshot', surname: 'Branchley', gender: 'female' }
	});
	expect(person.status()).toBe(201);
	const { id: personId } = (await person.json()) as { id: string };
	const branch = await request.post(`${API_BASE}/branches`, { data: { name: branchName } });
	expect(branch.status()).toBe(201);

	// Switch to the branch through the UI.
	await page.goto('/');
	await page.getByRole('button', { name: /switch research branch/i }).click();
	await page.getByRole('menuitem', { name: branchName }).click();
	await expect(page.getByText(`Working on ${branchName}`)).toBeVisible();

	// Snapshots follow the branch: no mainline notice, and the seeded mainline
	// snapshots are not this branch's.
	await page.goto('/snapshots');
	await expect(page.getByRole('heading', { level: 1, name: 'Research Snapshots' })).toBeVisible();
	await expect(page.getByText(/always shows mainline data/)).toHaveCount(0);
	await expect(page.getByText('No snapshots yet')).toBeVisible();

	await page.getByRole('button', { name: 'New snapshot' }).click();
	const createDialog = page.getByRole('dialog');
	await createDialog.getByLabel('Name').fill(snapshotName);
	await createDialog.getByRole('button', { name: 'Create snapshot' }).click();
	await expect(page.getByRole('heading', { name: snapshotName })).toBeVisible();

	// Edit the person on the branch.
	await page.goto(`/persons/${personId}`);
	await expect(page.getByRole('heading', { level: 1, name: 'Snapshot Branchley' })).toBeVisible();
	await page.locator('header.page-header').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByLabel('Birth Place').fill('Branchton Parish');
	await page.getByRole('button', { name: 'Save Changes' }).click();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toHaveCount(0);

	// Compare the snapshot to now: the branch edit is listed, labelled as the branch's own.
	await page.goto('/snapshots');
	await page.getByRole('link', { name: `Compare to now: ${snapshotName}` }).click();
	await expect(page).toHaveURL(/\/snapshots\/compare\?.*to=current/);
	await expect(page.getByText('Now', { exact: true })).toBeVisible();
	const changes = page.getByRole('list', { name: 'Changes, oldest first' });
	const edit = changes
		.getByRole('listitem')
		.filter({ hasText: 'updated' })
		.filter({ has: page.getByRole('link', { name: 'Snapshot Branchley', exact: true }) });
	await expect(edit.getByText('Branchton Parish')).toBeVisible();
	await expect(edit.getByText('This branch')).toBeVisible();

	// The mainline does not list the branch's snapshot.
	const mainList = await (await request.get(`${API_BASE}/snapshots`)).json();
	expect(mainList.items.map((s: { name: string }) => s.name)).not.toContain(snapshotName);
});
