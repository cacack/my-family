/**
 * Closing a branch keeps its negative research reachable (#836): a branch
 * whose search found nothing is closed as disproved with a reason, its
 * research log is still readable from the closed branch, and the close copies
 * the log to the mainline with the outcome recorded.
 *
 * The person, the branch and the research log are this spec's own, created
 * over the API with a name no other spec uses.
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

test('close a branch as disproved, then read its research log', async ({ page, request }) => {
	const surname = `Clausura${Date.now().toString(36)}`;
	const person = await post<{ id: string }>(request, '/persons', {
		given_name: 'Robin',
		surname,
		gender: 'unknown'
	});
	const branchName = `Parent theory ${surname}`;
	const branch = await post<{ id: string }>(request, '/branches', {
		name: branchName,
		hypothesis: `Was Robin ${surname} the child of the ferry keeper?`
	});
	const log = await post<{ id: string }>(request, `/research-logs?branch=${branch.id}`, {
		subject_id: person.id,
		subject_type: 'person',
		repository: 'Parish registers',
		search_description: `Baptisms for ${surname}, 1810-1820`,
		outcome: 'not_found',
		search_date: '2024-03-01T00:00:00Z'
	});
	const reason = 'The baptism register names other parents.';

	// Close it from the list, as disproved, keeping the copy to the mainline.
	await page.goto('/branches');
	const card = page.locator('article.branch-card', { hasText: branchName });
	await card.getByRole('button', { name: 'Close' }).click();
	const dialog = page.getByRole('dialog');
	await dialog.getByLabel('Outcome').selectOption('disproved');
	await dialog.getByLabel('Reason (optional)').fill(reason);
	await expect(dialog.getByLabel(/Copy this branch's research logs/)).toBeChecked();
	await dialog.getByRole('button', { name: 'Close branch' }).click();

	const notice = page
		.getByRole('status')
		.filter({ hasText: `Closed "${branchName}" as disproved` });
	await expect(notice).toContainText('1 research log was copied to the mainline.');

	// The list shows it closed, with its outcome and reason.
	await page.getByRole('radio', { name: 'Closed' }).click();
	const closedCard = page.locator('article.branch-card', {
		hasText: branchName
	});
	await expect(closedCard.getByTestId('branch-outcome')).toHaveText(/Disproved/);
	await expect(closedCard.getByTestId('close-reason')).toContainText(reason);

	// Its research log - the search that found nothing - is still readable.
	await closedCard.getByRole('link', { name: 'Research' }).click();
	await expect(page).toHaveURL(new RegExp(`/branches/${branch.id}/research$`));
	const entry = page
		.getByTestId('archived-research-log')
		.filter({ hasText: `Baptisms for ${surname}` });
	await expect(entry).toBeVisible();
	await expect(entry.getByTestId('log-outcome')).toHaveText('Not found');
	await expect(entry).toContainText(`Robin ${surname}`);
	await expect(page.getByTestId('research-close-reason')).toContainText(reason);

	// The branch page links to the same record.
	await page.goto(`/branches/${branch.id}`);
	await expect(page.getByTestId('closed-record')).toContainText(reason);
	await page.getByRole('link', { name: "View this branch's research" }).click();
	await expect(page.getByTestId('archived-research-log')).toHaveCount(1);

	// And the copy on the mainline records why the branch was closed.
	const onMain = await request.get(`${API_BASE}/research-logs/${log.id}`);
	expect(onMain.status()).toBe(200);
	const promoted = (await onMain.json()) as { outcome: string; notes?: string };
	expect(promoted.outcome).toBe('not_found');
	expect(promoted.notes).toContain('closed as disproved');
	expect(promoted.notes).toContain(reason);
});
