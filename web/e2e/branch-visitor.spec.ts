/**
 * A visitor reads the mainline only (ADR-008 §4, #1513): the app offers them
 * no branch and no link that leads only to a sign-in wall, and a branch left
 * in storage from before they became a visitor is dropped.
 *
 * The suite runs in open mode, where visitors cannot be let in, so it plays
 * the visitor through the owner's preview (#1413): the server answers the
 * preview exactly as it answers a visitor, and the app shows what a visitor
 * may do. Once anonymous viewing can be turned on (#1441), a real visitor
 * belongs here too.
 */
import { expect, test } from '@playwright/test';
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
	expect((await request.get(`/api/v1/persons?branch=${branchId}`)).status()).toBe(200);
});
