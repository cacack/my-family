/**
 * A branch carries its research record (#835): created with a question,
 * edited in place on the branch page (a subject added, the outcome recorded),
 * and shown on the branch list and the banner (question, verdict and subjects).
 *
 * The subject is created here over the API with a name no other spec uses, so
 * the search that picks it is unambiguous. The branch is this spec's own.
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

test('create a branch with a hypothesis, then add a subject and record its outcome', async ({
	page,
	request
}) => {
	const surname = `Quaestio${Date.now().toString(36)}`;
	const person = await post<{ id: string }>(request, '/persons', {
		given_name: 'Hypatia',
		surname,
		gender: 'female'
	});
	const branchName = `Research record ${surname}`;
	const hypothesis = `Was Hypatia ${surname} the daughter of the ferry keeper?`;

	// Create through the dialog, with the research question.
	await page.goto('/branches');
	await page.getByRole('button', { name: 'New branch' }).click();
	await page.getByLabel('Name').fill(branchName);
	await page.getByLabel('Research question (optional)').fill(hypothesis);
	await page.getByRole('button', { name: /^Create branch$/ }).click();

	// The card shows the question and the default outcome.
	const card = page.locator('article.branch-card', { hasText: branchName });
	await expect(card.getByText(hypothesis)).toBeVisible();
	await expect(card.getByTestId('branch-outcome')).toHaveText(/Open/);

	// The branch page shows the record; edit it in place.
	await card.getByRole('link', { name: branchName }).click();
	const research = page.getByTestId('branch-research');
	await expect(research.getByTestId('branch-hypothesis')).toHaveText(hypothesis);
	await page.getByRole('button', { name: 'Edit research record' }).click();

	const editor = page.getByTestId('branch-research-editor');
	await editor.getByLabel('Add a person').fill(surname);
	await editor.getByRole('option', { name: new RegExp(surname) }).click();
	await expect(editor.getByText(`Hypatia ${surname}`)).toBeVisible();
	await editor.getByLabel('Outcome').selectOption('proved');
	await editor.getByRole('button', { name: 'Save research record' }).click();

	// Saved: the summary is back, with the verdict and a link to the subject.
	await expect(page.getByTestId('branch-research-editor')).toHaveCount(0);
	await expect(research.getByTestId('branch-outcome')).toHaveText(/Proved/);
	const subjectLink = research.getByRole('link', { name: `Hypatia ${surname}` });
	await expect(subjectLink).toHaveAttribute('href', `/persons/${person.id}`);

	// It survives a reload - it was persisted, not just held in the page.
	await page.reload();
	await expect(page.getByTestId('branch-research').getByTestId('branch-outcome')).toHaveText(/Proved/);
	await expect(page.getByTestId('branch-research').getByRole('link', { name: `Hypatia ${surname}` })).toBeVisible();

	// On the branch, the banner carries the question and the verdict.
	await page.getByRole('button', { name: 'Switch to branch' }).click();
	const banner = page.getByTestId('banner-research');
	await expect(banner.getByText(hypothesis)).toBeVisible();
	await expect(banner.getByTestId('branch-outcome')).toHaveText(/Proved/);
	await expect(
		page.getByTestId('banner-subjects').getByRole('link', { name: `Hypatia ${surname}` })
	).toHaveAttribute('href', `/persons/${person.id}`);

	// The subject link opens the person.
	await page.getByTestId('branch-research').getByRole('link', { name: `Hypatia ${surname}` }).click();
	await expect(page.getByRole('heading', { level: 1, name: `Hypatia ${surname}` })).toBeVisible();
});
