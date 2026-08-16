/**
 * Smoke: the merge review renders a real comparison and can actually promote a
 * branch onto the mainline.
 *
 * One test, not four, because the flow is one transaction against a stateful
 * server: the merge is terminal, so a "then merge" step cannot be re-entered by
 * a second test. Splitting it would only buy independent titles at the cost of
 * three more boots and three more seedings.
 *
 * The last two assertions are the ones that make the rest non-vacuous: the
 * mainline person page must show the value that only existed on the branch, and
 * the branch must have dropped out of the switcher's active list.
 */
import { expect, test } from '@playwright/test';
import { readSeed } from './seed';

test('review resolves the conflict, merges the branch, and the mainline takes the branch value', async ({
	page
}) => {
	// Read the seed inside the test, not at module scope: `playwright test --list`
	// and editor test discovery load spec files WITHOUT running globalSetup, so a
	// module-scope read fails collection with ENOENT instead of listing tests.
	const { branchId, branchName, person, conflictField, familyName, familyBranchMarriagePlace } =
		readSeed().merge;

	await page.goto(`/branches/${branchId}`);
	await expect(page.getByRole('heading', { level: 1, name: branchName })).toBeVisible();

	// --- The diff, per side ------------------------------------------------
	const branchSide = page.getByTestId('branch-changes');
	const mainSide = page.getByTestId('main-changes');

	await expect(branchSide.getByRole('link', { name: person.name, exact: true })).toBeVisible();
	await expect(branchSide.getByText(person.branchBirthPlace)).toBeVisible();
	// A second branch-aware entity type, so the diff is not a one-entity special
	// case. Assert the changed VALUE too, not just that the family is listed —
	// the link alone would show for any family change at all.
	await expect(branchSide.getByRole('link', { name: familyName })).toBeVisible();
	await expect(branchSide.getByText(familyBranchMarriagePlace)).toBeVisible();

	await expect(mainSide.getByRole('link', { name: person.name, exact: true })).toBeVisible();
	await expect(mainSide.getByText(person.mainBirthPlace)).toBeVisible();
	// The mainline never touched the family, so its side must not claim otherwise.
	await expect(mainSide.getByRole('link', { name: familyName })).toHaveCount(0);

	// --- The conflict, and its resolution controls -------------------------
	await expect(page.getByRole('heading', { level: 3, name: new RegExp(person.name) })).toBeVisible();
	await expect(page.getByText(`Contested fields: ${conflictField}`)).toBeVisible();

	const takeBranch = page.getByRole('radio', { name: /Take the branch's version/ });
	await expect(takeBranch).toBeVisible();
	await expect(page.getByRole('radio', { name: /Keep the mainline's version/ })).toBeVisible();

	const reviewAndMerge = page.getByRole('button', { name: 'Review & merge' });
	await expect(page.getByText('1 of 1 conflict still undecided.')).toBeVisible();
	await expect(reviewAndMerge).toBeDisabled();

	await takeBranch.click();
	await expect(page.getByText('All 1 conflict decided.')).toBeVisible();
	await expect(reviewAndMerge).toBeEnabled();

	// --- The confirm dialog -------------------------------------------------
	await reviewAndMerge.click();
	const dialog = page.getByRole('alertdialog');
	await expect(dialog.getByText(`Merge ${branchName} into the mainline?`)).toBeVisible();
	await expect(dialog.getByText('This branch wins')).toBeVisible();

	await dialog.getByRole('button', { name: 'Merge branch' }).click();

	await expect(dialog.getByText(`Merged ${branchName} into the mainline`)).toBeVisible();
	// Two events replayed: the person edit and the family edit. Assert the count
	// itself, not just the label — the label is present for any count, so
	// checking only its visibility would pass on a merge that replayed nothing.
	const replayed = dialog.locator('dl.summary > div').filter({ hasText: 'Events replayed' });
	await expect(replayed).toContainText('2');
	await dialog.getByRole('button', { name: 'Done' }).click();

	// --- The branch now reads as merged ------------------------------------
	await page.goto(`/branches/${branchId}`);
	await expect(page.getByText(/accepts no further changes/)).toBeVisible();
	await expect(page.getByRole('button', { name: 'Review & merge' })).toHaveCount(0);

	// --- The merge actually moved the mainline ------------------------------
	await page.goto(`/persons/${person.id}`);
	await expect(page.getByText(person.branchBirthPlace)).toBeVisible();
	await expect(page.getByText(person.mainBirthPlace)).toHaveCount(0);

	// A merged branch is terminal, so it is no longer a switch target.
	await page.goto('/');
	await page.getByRole('button', { name: /switch research branch/i }).click();
	await expect(page.getByRole('menuitem', { name: 'Mainline' })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: branchName })).toHaveCount(0);
});
