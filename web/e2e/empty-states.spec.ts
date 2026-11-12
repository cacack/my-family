/**
 * Empty states on an empty database (#1115).
 *
 * The suite's store is seeded, so an empty database is stood in for here: every
 * API read is passed through to the real server and its answer emptied (every
 * list becomes `[]`, every count `0`). Each list page must then show the shared
 * `EmptyState` (`data-testid="empty-state"`) rather than a blank page, a
 * hand-built message or, on /analytics, 0% cards and charts. Tabbed pages keep
 * every panel in the DOM, so only the visible one counts.
 */
import { expect, test, type Page } from '@playwright/test';

/** Arrays become empty and numbers zero; paging parameters keep their values. */
function emptied(value: unknown, key = ''): unknown {
	if (Array.isArray(value)) return [];
	if (typeof value === 'number') return key === 'limit' || key === 'offset' ? value : 0;
	if (value && typeof value === 'object') {
		return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, emptied(v, k)]));
	}
	return value;
}

async function emptyDatabase(page: Page) {
	await page.route('**/api/v1/**', async (route) => {
		if (route.request().method() !== 'GET') return route.fallback();
		const response = await route.fetch();
		const type = response.headers()['content-type'] ?? '';
		if (!response.ok() || !type.includes('application/json')) return route.fulfill({ response });
		await route.fulfill({ response, json: emptied(await response.json()) });
	});
}

const VISIBLE_EMPTY_STATE = '[data-testid="empty-state"]:visible';

const LIST_PAGES = [
	'/persons',
	'/families',
	'/sources',
	'/repositories',
	'/branches',
	'/snapshots',
	'/evidence',
	'/evidence?tab=conflicts',
	'/evidence?tab=logs',
	'/evidence?tab=summaries',
	'/quality',
	'/quality?tab=duplicates',
	'/quality?tab=not-duplicates',
	'/browse/surnames',
	'/browse/places',
	'/browse/cemeteries',
	'/browse/brick-walls',
	'/browse/citation-templates',
	'/history',
	'/map',
	'/search?q=nobody'
];

test.describe('on an empty database', () => {
	test.beforeEach(async ({ page }) => {
		await emptyDatabase(page);
	});

	for (const path of LIST_PAGES) {
		test(`${path} shows the shared empty state`, async ({ page }) => {
			await page.goto(path);
			await expect(page.locator(VISIBLE_EMPTY_STATE)).toHaveCount(1);
		});
	}

	test('/analytics shows the empty state, not 0% cards and charts', async ({ page }) => {
		await page.goto('/analytics');
		const empty = page.getByTestId('empty-state');
		await expect(empty).toBeVisible();
		await expect(empty).toContainText('No people yet.');
		await expect(empty.getByRole('link', { name: 'Import GEDCOM' })).toHaveAttribute('href', '/import');
		await expect(page.getByText('Total Persons')).toHaveCount(0);
		await expect(page.getByRole('heading', { name: 'Research Confidence' })).toHaveCount(0);
	});
});
