/**
 * The front door of a secured instance: setup, sign-in, sign-out and an ended
 * session (ADR-007, #1424). The suite's server runs open, so each block starts
 * an accounts-mode server beside it (accounts-server.ts).
 */
import { expect, request as apiRequest, test, type Page } from '@playwright/test';
import {
	newPassword,
	startAccountsServer,
	startFreshAccountsServer,
	type AccountsServer,
	type FreshAccountsServer
} from './accounts-server';
import { ACCOUNTS_BASE_URL } from './seed';

const signInHeading = (page: Page) => page.getByRole('heading', { name: 'Sign in', level: 1 });

async function signIn(page: Page, username: string, password: string) {
	await page.getByLabel('Username').fill(username);
	await page.getByLabel('Password').fill(password);
	await page.getByRole('button', { name: 'Sign in' }).click();
}

// One page walks the whole first run, so these depend on each other.
test.describe('a fresh server', () => {
	test.describe.configure({ mode: 'serial' });

	let server: FreshAccountsServer;
	let page: Page;
	const owner = { username: 'first-owner', displayName: 'First Owner', password: newPassword() };

	test.beforeAll(async ({ browser }) => {
		server = await startFreshAccountsServer();
		page = await (await browser.newContext({ baseURL: ACCOUNTS_BASE_URL })).newPage();
	});

	test.afterAll(async () => {
		await page?.context().close();
		await server?.stop();
	});

	test('shows the setup page', async () => {
		await page.goto('/persons');
		await expect(page.getByRole('heading', { name: 'Set up My Family', level: 1 })).toBeVisible();
		await expect(page.locator('nav')).toHaveCount(0);
	});

	test('setup with the logged token signs the owner in', async () => {
		await page.getByLabel('Setup token').fill(server.setupToken);
		await page.getByLabel('Username').fill(owner.username);
		await page.getByLabel('Display name').fill(owner.displayName);
		await page.getByLabel('Password', { exact: true }).fill(owner.password);
		await page.getByLabel('Password again').fill(owner.password);
		await page.getByRole('button', { name: 'Create owner and sign in' }).click();

		await expect(page.getByRole('button', { name: `Account: ${owner.displayName}` })).toBeVisible();
		await expect(page.getByRole('heading', { name: 'Set up My Family' })).toHaveCount(0);
		// Signed in on the server too, not only in the page.
		await page.reload();
		await expect(page.getByRole('button', { name: `Account: ${owner.displayName}` })).toBeVisible();
	});

	test('sign-out shows the sign-in page', async () => {
		await page.getByRole('button', { name: `Account: ${owner.displayName}` }).click();
		await page.getByRole('menuitem', { name: 'Sign out' }).click();
		await expect(signInHeading(page)).toBeVisible();
		await page.reload();
		await expect(signInHeading(page)).toBeVisible();
	});
});

test.describe('with an owner', () => {
	let server: AccountsServer;

	test.beforeAll(async () => {
		server = await startAccountsServer();
	});

	test.afterAll(async () => {
		await server?.stop();
	});

	test.use({ baseURL: ACCOUNTS_BASE_URL });

	test('a wrong password shows an error and stays on the sign-in page', async ({ page }) => {
		await page.goto('/');
		await signIn(page, server.username, `${server.password}-wrong`);
		await expect(page.getByText('The username or password is wrong')).toBeVisible();
		await expect(signInHeading(page)).toBeVisible();
		await expect(page.getByLabel('Password')).toHaveValue('');
	});

	test('a request after the session is revoked returns to the sign-in page', async ({ page }) => {
		await page.goto('/');
		await signIn(page, server.username, server.password);
		const people = page.getByRole('link', { name: 'People', exact: true }).first();
		await expect(people).toBeVisible();

		// End this browser's session on the server, behind the page's back.
		const session = await apiRequest.newContext({
			baseURL: server.baseURL,
			storageState: await page.context().storageState()
		});
		expect((await session.post('/api/v1/auth/sign-out')).ok()).toBe(true);
		await session.dispose();

		await people.click();
		await expect(signInHeading(page)).toBeVisible();
	});
});
