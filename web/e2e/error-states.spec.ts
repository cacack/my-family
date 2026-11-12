/**
 * The shared not-found and load-error states (#1096, docs/CONVENTIONS.md "Load
 * states") in a real browser: on every detail page a missing record keeps
 * exactly one h1 and offers no Retry, and a 500 or a dropped connection keeps
 * one h1, reads as a friendly sentence and offers Retry.
 *
 * The ids below exist nowhere, so the real server answers 404; the error cases
 * stub only the requests that name them.
 */
import { expect, test, type Page } from '@playwright/test';

const MISSING = '00000000-0000-4000-8000-000000000001';
const OTHER = '00000000-0000-4000-8000-000000000002';

const ROUTES = [
	`/persons/${MISSING}`,
	`/families/${MISSING}`,
	`/families/${MISSING}/group-sheet`,
	`/sources/${MISSING}`,
	`/repositories/${MISSING}`,
	`/branches/${MISSING}`,
	`/branches/${MISSING}/research`,
	`/evidence/analyses/${MISSING}`,
	`/evidence/conflicts/${MISSING}`,
	`/evidence/proof-summaries/${MISSING}`,
	`/evidence/research-logs/${MISSING}`,
	`/ahnentafel/${MISSING}`,
	`/pedigree/${MISSING}`,
	`/descendancy/${MISSING}`,
	`/quality/merge/${MISSING}/${OTHER}`,
	`/snapshots/compare?from=${MISSING}&to=${OTHER}`
];

/** Fails every API request naming one of the missing ids. */
async function failRecordRequests(page: Page, how: 'status500' | 'abort') {
	await page.route(
		(url) => url.pathname.startsWith('/api/') && (url.href.includes(MISSING) || url.href.includes(OTHER)),
		(route) =>
			how === 'abort'
				? route.abort('failed')
				: route.fulfill({
						status: 500,
						contentType: 'application/json',
						body: JSON.stringify({ code: 'INTERNAL_ERROR', message: 'boom' })
					})
	);
}

for (const route of ROUTES) {
	test.describe(route, () => {
		test('not found: one h1, no Retry', async ({ page }) => {
			await page.goto(route);
			await expect(page.getByRole('alert').filter({ hasText: /could not be found/ })).toBeVisible();
			await expect(page.locator('h1')).toHaveCount(1);
			await expect(page.getByRole('button', { name: 'Retry' })).toHaveCount(0);
		});

		for (const how of ['status500', 'abort'] as const) {
			test(`${how}: one h1, a friendly sentence, Retry`, async ({ page }) => {
				await failRecordRequests(page, how);
				await page.goto(route);
				const alert = page.getByRole('alert').filter({ hasText: /Please try again/ });
				await expect(alert).toBeVisible();
				await expect(alert).not.toContainText('boom');
				await expect(alert).not.toContainText('Failed to fetch');
				await expect(page.locator('h1')).toHaveCount(1);
				await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible();
			});
		}
	});
}
