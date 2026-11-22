/**
 * A visitor reads the mainline only (ADR-008 §4, #1513): the app offers them
 * no branch and no link that leads only to a sign-in wall, and a branch left
 * in storage from before they became a visitor is dropped.
 *
 * The suite runs in open mode, where visitors cannot be let in, so the first
 * tests play the visitor through the owner's preview (#1413): the server
 * answers the preview exactly as it answers a visitor, and the app shows what
 * a visitor may do. The last describe block starts an accounts-mode server of
 * its own, lets visitors in as its owner, and checks a real one (#1558).
 */
import { expect, request as apiRequest, test, type APIRequestContext } from '@playwright/test';
import { startAccountsServer, type AccountsServer } from './accounts-server';
import { BASE_URL, readSeed } from './seed';

test('a visitor gets the mainline, no branch to switch to, and no compare links', async ({
	page,
	context
}) => {
	const { branchId, branchName } = readSeed().switcher;

	// On the branch as the owner first, as someone who then signs out would be.
	await page.goto('/');
	await page.evaluate((id) => localStorage.setItem('active-branch', id), branchId);
	await context.addCookies([{ name: 'myfamily_preview', value: 'visitor', url: BASE_URL }]);

	await page.goto('/branches');

	await expect(page.getByText('Visitors see the mainline only')).toBeVisible();
	await expect(page.getByText(`Working on ${branchName}`)).toHaveCount(0);
	expect(await page.evaluate(() => localStorage.getItem('active-branch'))).toBeNull();

	// The list names the branch but leads nowhere a visitor may not go.
	const card = page.locator('.branch-card', { hasText: branchName });
	await expect(card).toBeVisible();
	await expect(card.getByRole('link', { name: branchName })).toHaveCount(0);
	await expect(page.getByRole('link', { name: 'Compare' })).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Switch to branch' })).toHaveCount(0);

	await page.getByRole('button', { name: /switch research branch/i }).click();
	await expect(page.getByRole('menuitem', { name: 'Mainline' })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: 'Sign in to work in a research branch' })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: branchName })).toHaveCount(0);
});

test('the server refuses a visitor a branch scope', async ({ request }) => {
	const { branchId } = readSeed().switcher;
	const headers = { cookie: 'myfamily_preview=visitor' };

	expect((await request.get('/api/v1/persons', { headers })).status()).toBe(200);
	expect((await request.get(`/api/v1/persons?branch=${branchId}`, { headers })).status()).toBe(403);
	expect((await request.get(`/api/v1/branches/${branchId}/research`, { headers })).status()).toBe(403);
	expect((await request.get(`/api/v1/persons?branch=${branchId}`)).status()).toBe(200);
});

/**
 * A real visitor needs AUTH_MODE=accounts, which the suite's server does not
 * run, so this block starts one beside it (accounts-server.ts).
 */
test.describe('a real anonymous visitor', () => {
	let accounts: AccountsServer;
	let baseURL: string;
	let owner: APIRequestContext;
	let activeBranch: { id: string; name: string };
	let closedBranch: { id: string; name: string };

	test.beforeAll(async () => {
		accounts = await startAccountsServer();
		({ baseURL, owner } = accounts);

		const settings = await owner.get('/api/v1/settings/privacy');
		expect(settings.status()).toBe(200);
		const letIn = await owner.put('/api/v1/settings/privacy', {
			data: { ...(await settings.json()), anonymous_access: 'viewer' }
		});
		expect(letIn.status()).toBe(200);

		const create = async (name: string) => {
			const response = await owner.post('/api/v1/branches', { data: { name } });
			expect(response.status()).toBe(201);
			return (await response.json()) as { id: string; name: string };
		};
		activeBranch = await create('Visitor check open branch');
		closedBranch = await create('Visitor check closed branch');
		const close = await owner.post(`/api/v1/branches/${closedBranch.id}/close`, {
			data: { outcome: 'abandoned' }
		});
		expect(close.ok()).toBe(true);
	});

	test.afterAll(async () => {
		await accounts?.stop();
	});

	test('the branches page offers a visitor no branch, no compare and no research', async ({
		browser
	}) => {
		// The owner first, so each control checked absent below is one the page
		// really renders for someone allowed to use it.
		const ownerContext = await browser.newContext({ baseURL, storageState: await owner.storageState() });
		const ownerPage = await ownerContext.newPage();
		await ownerPage.goto('/branches');
		const ownerActive = ownerPage.locator('.branch-card', { hasText: activeBranch.name });
		await expect(ownerActive.getByRole('button', { name: 'Switch to branch' })).toBeVisible();
		await expect(ownerActive.getByRole('link', { name: 'Compare' })).toBeVisible();
		const ownerClosed = ownerPage.locator('.branch-card', { hasText: closedBranch.name });
		await expect(ownerClosed.getByRole('link', { name: 'Research' })).toBeVisible();
		await ownerContext.close();

		// No session at all: a visitor let in by the owner's setting.
		const visitorContext = await browser.newContext({ baseURL });
		const page = await visitorContext.newPage();
		await page.goto('/branches');

		await expect(page.locator('.branch-card', { hasText: activeBranch.name })).toBeVisible();
		await expect(page.locator('.branch-card', { hasText: closedBranch.name })).toBeVisible();
		await expect(page.getByRole('link', { name: activeBranch.name })).toHaveCount(0);
		await expect(page.getByRole('button', { name: 'Switch to branch' })).toHaveCount(0);
		await expect(page.getByRole('link', { name: 'Compare' })).toHaveCount(0);
		await expect(page.getByRole('link', { name: 'Research' })).toHaveCount(0);
		await expect(page.getByRole('link', { name: 'Finish merge' })).toHaveCount(0);

		await page.getByRole('button', { name: /switch research branch/i }).click();
		await expect(page.getByRole('menuitem', { name: 'Mainline' })).toBeVisible();
		await expect(page.getByRole('menuitem', { name: 'Sign in to work in a research branch' })).toBeVisible();
		await expect(page.getByRole('menuitem', { name: activeBranch.name })).toHaveCount(0);

		await visitorContext.close();
	});

	test('the server refuses a visitor a branch scope with 401', async () => {
		const visitor = await apiRequest.newContext({ baseURL });
		try {
			expect((await visitor.get('/api/v1/persons')).status()).toBe(200);
			expect((await visitor.get(`/api/v1/persons?branch=${activeBranch.id}`)).status()).toBe(401);
			expect((await owner.get(`/api/v1/persons?branch=${activeBranch.id}`)).status()).toBe(200);
		} finally {
			await visitor.dispose();
		}
	});
});
