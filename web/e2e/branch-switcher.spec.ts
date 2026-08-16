/**
 * Smoke: choosing a research branch actually rescopes what the app reads.
 *
 * The component tests already cover the switcher's own rendering. What only a
 * browser against the real binary can show is the wiring: menu -> store ->
 * localStorage -> reload -> `?branch=` on every request -> a different value on
 * a page that knows nothing about branches.
 */
import { expect, test } from '@playwright/test';
import { readSeed } from './seed';

// Read the seed inside each test, not at module scope: `playwright test --list`
// and editor test discovery load spec files WITHOUT running globalSetup, so a
// module-scope read fails collection with ENOENT instead of listing tests.

test('the switcher lists the mainline and the seeded branch', async ({ page }) => {
	const { branchName } = readSeed().switcher;

	await page.goto('/');

	await page.getByRole('button', { name: /switch research branch/i }).click();

	await expect(page.getByRole('menuitem', { name: 'Mainline' })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: branchName })).toBeVisible();
});

test('scoping to a branch shows the branch-edited value, and returning restores the mainline', async ({
	page
}) => {
	const { branchName, person } = readSeed().switcher;

	await page.goto(`/persons/${person.id}`);

	// The mainline value, before any branch is chosen.
	await expect(page.getByRole('heading', { level: 1, name: person.name })).toBeVisible();
	await expect(page.getByText(person.mainBirthPlace)).toBeVisible();

	await page.getByRole('button', { name: /switch research branch/i }).click();
	await page.getByRole('menuitem', { name: branchName }).click();

	// Switching reloads the page, so this assertion also proves the choice
	// survived the reload rather than living in a store that was torn down.
	await expect(page.getByText(`Working on ${branchName}`)).toBeVisible();
	await expect(page.getByText(person.branchBirthPlace)).toBeVisible();
	await expect(page.getByText(person.mainBirthPlace)).toHaveCount(0);

	await page.getByRole('button', { name: /return to mainline/i }).click();

	await expect(page.getByText(`Working on ${branchName}`)).toHaveCount(0);
	await expect(page.getByText(person.mainBirthPlace)).toBeVisible();
	await expect(page.getByText(person.branchBirthPlace)).toHaveCount(0);
});
