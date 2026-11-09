/**
 * GEDCOM CONT/CONC lines fold into a source's title, author and publisher and a
 * citation's page. The views show those line breaks and an unchanged edit keeps
 * them (#1174).
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

async function post<T>(request: APIRequestContext, path: string, data: unknown): Promise<T> {
	const response = await request.post(`${API_BASE}${path}`, { data });
	expect(response.status(), `POST ${path}: ${await response.text()}`).toBe(201);
	return (await response.json()) as T;
}

test('multi-line source and citation fields keep their line breaks', async ({ page, request }) => {
	const stamp = Date.now();
	const fields = {
		title: `Parish register ${stamp}\nSt Mary, Kent`,
		author: 'Church of England\nDiocese of Canterbury',
		publisher: 'Kent Archives\nMaidstone'
	};
	const citationPage = 'Folio 12\nEntry 3';
	const source = await post<{ id: string }>(request, '/sources', { source_type: 'church_record', ...fields });
	const person = await post<{ id: string }>(request, '/persons', {
		given_name: 'Ada',
		surname: `Multiline${stamp}`,
		gender: 'female'
	});
	await post(request, '/citations', {
		source_id: source.id,
		fact_type: 'person_birth',
		fact_owner_id: person.id,
		page: citationPage
	});

	await page.goto(`/sources/${source.id}`);
	const title = page.getByRole('heading', { level: 1 });
	await expect(title).toHaveCSS('white-space', 'pre-line');
	await expect(page.locator('dd', { hasText: 'Church of England' })).toHaveCSS('white-space', 'pre-line');
	await expect(page.getByText(/^Page: Folio 12/)).toHaveCSS('white-space', 'pre-line');

	// Saving the form unchanged keeps every line break.
	await page.locator('.action-bar').getByRole('button', { name: 'Edit', exact: true }).click();
	await page.getByRole('button', { name: 'Save Changes' }).click();
	await expect(page.getByRole('button', { name: 'Save Changes' })).toBeHidden();
	const saved = await (await request.get(`${API_BASE}/sources/${source.id}`)).json();
	expect(saved).toMatchObject(fields);

	await page.goto(`/sources?q=${stamp}`);
	await expect(page.getByRole('heading', { level: 3, name: /Parish register/ })).toHaveCSS('white-space', 'pre-line');
	await expect(page.locator('.author', { hasText: 'Church of England' })).toHaveCSS('white-space', 'pre-line');

	await page.goto(`/persons/${person.id}`);
	await expect(page.getByText(/^Page: Folio 12/)).toHaveCSS('white-space', 'pre-line');
	await expect(page.getByRole('link', { name: /Parish register/ })).toHaveCSS('white-space', 'pre-line');
});
