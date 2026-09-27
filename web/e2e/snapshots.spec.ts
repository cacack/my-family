/**
 * Smoke: research snapshots can be created, deleted and compared against the
 * real binary.
 *
 * The component tests cover rendering against a mocked client. What only the
 * real stack can show is that the comparison the server computes between two
 * seeded snapshots reads correctly in the UI: the mainline edit made between
 * them appears as a field-level diff, and the branch edit made in the same
 * window does not.
 */
import { expect, test } from '@playwright/test';
import { readSeed } from './seed';

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
	await page.getByLabel('From').selectOption(snapshots.beforeId);
	await page.getByLabel('To').selectOption(snapshots.afterId);
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
