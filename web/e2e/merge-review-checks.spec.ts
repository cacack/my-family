/**
 * Merge review checks (#838): a branch that introduces a validation issue
 * shows it in the merge review's branch health, and the fact it changed
 * without evidence in the evidence-coverage warning - whose "add analysis"
 * link, followed on the branch, documents the change and clears the warning.
 *
 * The fixture is this spec's own, built over the real API: on the mainline a
 * person born in 1850, then a branch on which she dies in 1995 - an impossible
 * age of 145 that the mainline does not have.
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), await response.text()).toBe(201);
	return (await response.json()) as T;
}

test('a branch introducing a validation issue shows it, and its undocumented change, in the merge review', async ({
	page,
	request
}) => {
	const stamp = Date.now();
	const surname = `Checked${stamp}`;
	const personName = `Ada ${surname}`;

	// --- Mainline, then the branch's research ---------------------------------
	const person = await post<{ id: string }>(request, '/persons', {
		given_name: 'Ada',
		surname,
		birth_date: '1 JAN 1850'
	});
	const branchName = `E2E Review Checks ${stamp}`;
	const branch = await post<{ id: string }>(request, '/branches', { name: branchName });
	const onBranch = await request.get(`${API_BASE}/persons/${person.id}?branch=${branch.id}`);
	const { version } = (await onBranch.json()) as { version: number };
	const updated = await request.put(`${API_BASE}/persons/${person.id}?branch=${branch.id}`, {
		data: { death_date: '1 JAN 1995', version }
	});
	expect(updated.status(), await updated.text()).toBe(200);

	await page.goto(`/branches/${branch.id}`);
	await expect(page.getByRole('heading', { level: 1, name: branchName })).toBeVisible();

	// --- Branch health: the introduced issue, by name ---------------------------
	const health = page.getByTestId('branch-health');
	await expect(health.getByRole('heading', { name: 'Branch health' })).toBeVisible();
	await expect(health.getByText('IMPOSSIBLE_AGE')).toBeVisible();
	await expect(health.getByRole('link', { name: personName })).toHaveAttribute(
		'href',
		`/persons/${person.id}`
	);
	await expect(health.getByText(/This branch introduces 1 warning/)).toBeVisible();

	// --- Evidence coverage: the death changed without evidence ------------------
	const coverage = page.getByTestId('evidence-coverage');
	await expect(
		coverage.getByRole('heading', {
			name: '1 changed fact or relationship has no evidence analysis or proof summary on this branch'
		})
	).toBeVisible();
	await expect(coverage.getByText('Death')).toBeVisible();
	await expect(coverage.getByRole('link', { name: personName })).toBeVisible();
	// Soft: neither check holds the merge.
	await expect(page.getByRole('button', { name: 'Review & merge' })).toBeEnabled();

	// An analysis must land on the branch, so the link is offered there only.
	await expect(coverage.getByRole('link', { name: /^Add analysis/ })).toHaveCount(0);
	await coverage.getByRole('button', { name: 'Switch to branch' }).click();
	const addAnalysis = page.getByTestId('evidence-coverage').getByRole('link', { name: /^Add analysis/ });
	await expect(addAnalysis).toBeVisible();

	// --- Document the change on the branch --------------------------------------
	await addAnalysis.click();
	await expect(page.getByRole('heading', { name: 'New Evidence Analysis' })).toBeVisible();
	await expect(page.getByLabel('Fact Type')).toHaveValue('person_death');
	await expect(page.getByLabel('Subject ID')).toHaveValue(person.id);
	await page.getByLabel('Conclusion').fill('Died 1995 per the burial register');
	await page.getByRole('button', { name: 'Create Analysis' }).click();
	await expect(page.getByText('Died 1995 per the burial register')).toBeVisible();

	await page.goto(`/branches/${branch.id}`);
	await expect(
		page
			.getByTestId('evidence-coverage')
			.getByText(/Every fact and relationship this branch changed has an evidence analysis/)
	).toBeVisible();
	// The health is about the data, not its documentation: still there.
	await expect(page.getByTestId('branch-health').getByText('IMPOSSIBLE_AGE')).toBeVisible();

	// The mainline has no analysis: it was written on the branch.
	const mainAnalyses = await request.get(`${API_BASE}/evidence-analyses?limit=100`);
	const { analyses } = (await mainAnalyses.json()) as { analyses: { subject_id: string }[] };
	expect(analyses.some((a) => a.subject_id === person.id)).toBe(false);
});
