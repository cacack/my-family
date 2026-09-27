/**
 * Branch compare renders every kind of change readably (#827, #739): a
 * person's name edit shows what it replaced, and a note written on the branch
 * shows as a note with its text - not as `unknown` with a raw id - including
 * on the "leave out of the merge" control.
 *
 * The fixture is made here over the API, on a branch of this spec's own, so
 * nothing any other spec does can affect it.
 */
import { expect, test, type APIRequestContext } from '@playwright/test';
import { API_BASE } from './seed';

const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

async function send<T>(
	request: APIRequestContext,
	method: 'post' | 'put',
	path: string,
	data: unknown,
	status: number
): Promise<T> {
	const response = await request[method](`${API_BASE}${path}`, { data });
	expect(response.status(), `${method.toUpperCase()} ${path}: ${await response.text()}`).toBe(
		status
	);
	return (await response.json()) as T;
}

test('a branch that renames a person and adds a note compares as readable entries', async ({
	page,
	request
}) => {
	const stamp = Date.now();
	const person = await send<{ id: string; version: number }>(
		request,
		'post',
		'/persons',
		{ given_name: 'Lucia', surname: `Readable${stamp}` },
		201
	);
	const branch = await send<{ id: string }>(
		request,
		'post',
		'/branches',
		{ name: `Readable compare ${stamp}` },
		201
	);
	const onBranch = `?branch=${branch.id}`;

	await send(
		request,
		'put',
		`/persons/${person.id}${onBranch}`,
		{ given_name: 'Lucy', version: person.version },
		200
	);
	const noteText = `Parish register lists Lucy as a sponsor ${stamp}`;
	await send(request, 'post', `/notes${onBranch}`, { text: noteText }, 201);

	await page.goto(`/branches/${branch.id}`);
	const branchSide = page.getByTestId('branch-changes');

	// The person edit: named by the branch's name for them, linked to their
	// page, and showing the value the edit replaced as well as the new one.
	const personEntry = branchSide
		.getByRole('listitem')
		.filter({ has: page.getByRole('link', { name: `Lucy Readable${stamp}`, exact: true }) });
	await expect(personEntry).toBeVisible();
	await expect(personEntry.getByText('Given Name')).toBeVisible();
	await expect(personEntry.getByText('Lucia', { exact: true })).toBeVisible();
	await expect(personEntry.getByText('Lucy', { exact: true })).toBeVisible();

	// The note: a Note badge and its text, never "unknown" and an id.
	const noteEntry = branchSide.getByRole('listitem').filter({ hasText: noteText });
	await expect(noteEntry.getByText('Note', { exact: true })).toBeVisible();
	await expect(noteEntry.getByText('created', { exact: true })).toBeVisible();
	await expect(
		noteEntry.getByRole('checkbox', { name: `Leave out of the merge: ${noteText}` })
	).toBeVisible();

	await expect(branchSide.getByText(/unknown/i)).toHaveCount(0);
	await expect(branchSide.getByText(UUID)).toHaveCount(0);
});
