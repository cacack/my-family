/**
 * Custom Export as CSV downloads a CSV file with the columns chosen in the
 * field picker, not the JSON response (#1092).
 */
import { readFile } from 'node:fs/promises';
import { expect, test, type Page } from '@playwright/test';

async function downloadCustomExport(page: Page): Promise<{ filename: string; text: string }> {
	const [download] = await Promise.all([
		page.waitForEvent('download'),
		page.getByRole('button', { name: 'Export People as CSV' }).click()
	]);
	const path = await download.path();
	return { filename: download.suggestedFilename(), text: await readFile(path, 'utf8') };
}

test.beforeEach(async ({ page }) => {
	await page.goto('/import');
	await page.getByRole('radio', { name: 'People' }).check();
	await page.getByRole('radio', { name: 'CSV' }).check();
});

test('CSV export downloads a CSV file with a header row', async ({ page }) => {
	const { filename, text } = await downloadCustomExport(page);

	expect(filename).toBe('persons.csv');
	const [header] = text.split('\n');
	expect(header).toBe(
		'id,given_name,surname,full_name,gender,birth_date,birth_place,death_date,death_place,notes'
	);
});

test('CSV export includes only the chosen fields', async ({ page }) => {
	await page.getByRole('button', { name: 'Select None' }).click();
	await page.getByRole('checkbox', { name: 'ID', exact: true }).check();
	await page.getByRole('checkbox', { name: 'Given Name', exact: true }).check();

	const { text } = await downloadCustomExport(page);

	const lines = text.trim().split('\n');
	expect(lines[0]).toBe('id,given_name');
	expect(lines.length).toBeGreaterThan(1);
	for (const line of lines.slice(1)) {
		expect(line.split(',')).toHaveLength(2);
	}
});

test('CSV export is disabled when no fields are chosen', async ({ page }) => {
	const exportButton = page.getByRole('button', { name: 'Export People as CSV' });
	await page.getByRole('button', { name: 'Select None' }).click();

	await expect(exportButton).toBeDisabled();
	await expect(page.getByText('Select at least one field to export.')).toBeVisible();

	await page.getByRole('checkbox', { name: 'ID', exact: true }).check();
	await expect(exportButton).toBeEnabled();
});
