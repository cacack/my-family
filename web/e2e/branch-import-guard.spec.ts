/**
 * GEDCOM import always writes the mainline, so it is withdrawn while a research
 * branch is active (#825). The component tests pin the guard's rendering; this
 * smoke shows the wiring against the real binary: switching to a branch
 * removes the upload, the offered way back restores it, and the API refuses a
 * branch-scoped import even from a client that skips the UI.
 */
import { expect, test } from '@playwright/test';
import { API_BASE, readSeed } from './seed';

test('import is disabled on a research branch and returns on the mainline', async ({ page }) => {
	const { branchName } = readSeed().switcher;

	await page.goto('/import');
	await expect(page.getByText('Browse Files')).toBeVisible();

	await page.getByRole('button', { name: /switch research branch/i }).click();
	await page.getByRole('menuitem', { name: branchName }).click();

	// Switching reloads the page.
	await expect(page.getByText(`Working on ${branchName}`)).toBeVisible();
	await expect(page.getByText('Import is unavailable on a research branch')).toBeVisible();
	await expect(page.getByText('Browse Files')).toHaveCount(0);
	await expect(page.locator('input[type="file"]')).toHaveCount(0);
	await expect(page.getByRole('button', { name: /import file/i })).toHaveCount(0);
	// Exports stay available, but say what they cover.
	await expect(page.getByText(/Exports always cover the mainline/)).toBeVisible();

	await page.getByRole('button', { name: 'Switch to mainline' }).click();

	await expect(page.getByText(`Working on ${branchName}`)).toHaveCount(0);
	await expect(page.getByText('Browse Files')).toBeVisible();
	await expect(page.getByText('Import is unavailable on a research branch')).toHaveCount(0);
});

test('the API refuses a branch-scoped GEDCOM import', async ({ request }) => {
	const { branchId } = readSeed().switcher;

	const gedcom = '0 HEAD\n1 GEDC\n2 VERS 5.5\n1 CHAR UTF-8\n0 @I1@ INDI\n1 NAME Refused /Import/\n0 TRLR\n';
	for (const route of ['gedcom/import', 'gedcom/import/stream']) {
		const res = await request.post(`${API_BASE}/${route}?branch=${branchId}`, {
			multipart: {
				file: { name: 'refused.ged', mimeType: 'text/plain', buffer: Buffer.from(gedcom) }
			}
		});
		expect(res.status(), route).toBe(400);
		const body = await res.json();
		expect(body.message, route).toContain('mainline');
	}

	const search = await request.get(`${API_BASE}/persons?limit=1000`);
	expect(search.ok()).toBe(true);
	const persons = (await search.json()) as { items: { surname?: string }[] };
	expect(persons.items.some((p) => p.surname === 'Import')).toBe(false);
});
