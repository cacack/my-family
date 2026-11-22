/**
 * The owner's pages during a preview (#1578): the server refuses the
 * previewed principal what only the owner may call (#1413), so each page the
 * owner alone may use says so, rather than failing on that refusal. Only an
 * owner who is signed in previews the way a relative would see it, so this
 * runs against an accounts-mode server (accounts-server.ts).
 */
import { expect, test, type Page } from '@playwright/test';
import { startAccountsServer, type AccountsServer } from './accounts-server';

/** The pages only the owner may use, and what each says the previewed principal may not do. */
const OWNER_PAGES = [
	{ path: '/settings', action: "Changing the tree's settings" },
	{ path: '/users', action: 'Managing users' },
	{ path: '/users/audit', action: 'Reading the audit log' },
	{ path: '/import', action: 'Importing GEDCOM' }
];

let accounts: AccountsServer;

test.beforeAll(async () => {
	accounts = await startAccountsServer();
});

test.afterAll(async () => {
	await accounts?.stop();
});

/** Collects the page's console errors and uncaught exceptions. */
function collectErrors(page: Page): string[] {
	const errors: string[] = [];
	page.on('console', (message) => {
		if (message.type() === 'error') errors.push(message.text());
	});
	page.on('pageerror', (error) => errors.push(error.message));
	return errors;
}

for (const as of ['viewer', 'visitor'] as const) {
	test(`each owner page previewed as a ${as} says why it cannot be used`, async ({ browser }) => {
		const context = await browser.newContext({
			baseURL: accounts.baseURL,
			storageState: await accounts.owner.storageState()
		});
		await context.addCookies([{ name: 'myfamily_preview', value: as, url: accounts.baseURL }]);
		const page = await context.newPage();
		const errors = collectErrors(page);

		for (const { path, action } of OWNER_PAGES) {
			await page.goto(path);
			const notice = page.getByTestId('role-notice').filter({ hasText: `${action} needs the owner role` });
			await expect(notice, path).toBeVisible();
			await expect(notice, path).toContainText(`You are previewing the tree as a ${as}`);
			await page.waitForLoadState('networkidle');
			expect(errors, path).toEqual([]);
		}
		await context.close();
	});
}

test('starting a preview from the settings page leaves no error behind', async ({ browser }) => {
	const context = await browser.newContext({
		baseURL: accounts.baseURL,
		storageState: await accounts.owner.storageState()
	});
	const page = await context.newPage();
	const errors = collectErrors(page);

	await page.goto('/settings');
	await page.getByRole('button', { name: 'Preview as a viewer' }).click();
	await expect(page.getByRole('status').filter({ hasText: 'You see the tree as a signed-in viewer does.' })).toBeVisible();
	// The banner shows as soon as the preview starts; wait for the home page
	// too, so going back returns to the settings page.
	await expect(page).toHaveURL(`${accounts.baseURL}/`);
	await page.goBack();
	await expect(page).toHaveURL(`${accounts.baseURL}/settings`);
	await expect(page.getByTestId('role-notice')).toContainText('You are previewing the tree as a viewer');
	await page.waitForLoadState('networkidle');
	expect(errors).toEqual([]);
	await context.close();
});
