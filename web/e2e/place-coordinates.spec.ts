/**
 * A person entered by hand can be put on the map (#1033): coordinates typed
 * into the Add Person form are saved in GEDCOM form and the map lists the
 * place. The place and surname are ones no other spec uses.
 */
import { expect, test } from '@playwright/test';

test('a birth place given coordinates in the form appears on the map', async ({ page }) => {
	const surname = `Surveyor${Date.now().toString(36)}`;
	const place = `Pinfold ${surname}`;

	await page.goto('/persons/add');
	await page.getByLabel('Given Name').fill('Ada');
	await page.getByLabel('Surname').fill(surname);
	await page.getByLabel('Birth Date').fill('1 JAN 1850');
	await page.getByLabel('Birth Place', { exact: true }).fill(place);
	const birth = page.getByRole('group', { name: 'Birth Place coordinates' });
	await birth.getByLabel('Latitude').fill('42.3601');
	await birth.getByLabel('Longitude').fill('-71.0589');
	await page.getByRole('button', { name: 'Create Person' }).click();
	await expect(page).toHaveURL(/\/persons\/[0-9a-f-]+$/);

	// The edit form shows the pair back in signed decimal degrees.
	await page.getByRole('button', { name: 'Edit', exact: true }).first().click();
	const edit = page.getByRole('group', { name: 'Birth Place coordinates' });
	await expect(edit.getByLabel('Latitude')).toHaveValue('42.3601');
	await expect(edit.getByLabel('Longitude')).toHaveValue('-71.0589');
	await expect(page.getByTestId('coordinates-origin').first()).toHaveText('Entered by hand');

	await page.goto('/map');
	const list = page.locator('details.location-list');
	await list.locator('summary').click();
	await expect(list.getByRole('button', { name: new RegExp(place) })).toBeVisible();
});
