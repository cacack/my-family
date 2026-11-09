import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import PersonsPage from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import { back, currentPath, historyLength, resetRouter } from '#lib/test/fakeRouter.svelte.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { listPersons: vi.fn() } };
});

vi.mock('$app/state', async () => (await import('#lib/test/fakeRouter.svelte.js')).appState);
vi.mock('$app/navigation', async () => (await import('#lib/test/fakeRouter.svelte.js')).appNavigation);

const persons = Array.from({ length: 20 }, (_, i) => ({
	id: `p-${i}`,
	given_name: `Given${i}`,
	surname: 'Smith',
	version: 1
}));

function lastListCall() {
	return vi.mocked(apiModule.api.listPersons).mock.lastCall?.[0];
}

describe('People list page: state in the URL (#901)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.listPersons).mockResolvedValue({
			items: persons,
			total: 100
		} as unknown as Awaited<ReturnType<typeof apiModule.api.listPersons>>);
	});

	it('loads the sort, order, filter and page the URL names', async () => {
		resetRouter('/persons?sort=birth_date&order=desc&status=possible&page=3');
		render(PersonsPage);
		await screen.findByText('Page 3 of 5');
		expect(lastListCall()).toEqual({
			limit: 20,
			offset: 40,
			sort: 'birth_date',
			order: 'desc',
			research_status: 'possible'
		});
		expect((screen.getByLabelText('Sort by:') as HTMLSelectElement).value).toBe('birth_date');
		expect((screen.getByLabelText('Confidence:') as HTMLSelectElement).value).toBe('possible');
	});

	it('falls back to defaults for values it does not know', async () => {
		resetRouter('/persons?sort=shoe_size&order=up&status=bogus&page=-1');
		render(PersonsPage);
		await screen.findByText('Page 1 of 5');
		expect(lastListCall()).toEqual({
			limit: 20,
			offset: 0,
			sort: 'surname',
			order: 'asc',
			research_status: undefined
		});
	});

	it('pushes page changes, so Back returns to the previous page', async () => {
		resetRouter('/persons');
		render(PersonsPage);
		await screen.findByText('Page 1 of 5');

		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		await screen.findByText('Page 2 of 5');
		expect(currentPath()).toBe('/persons?page=2');
		expect(historyLength()).toBe(2);
		expect(lastListCall()).toMatchObject({ offset: 20 });

		back();
		await screen.findByText('Page 1 of 5');
		expect(lastListCall()).toMatchObject({ offset: 0 });
	});

	it('ignores a load that resolves after a newer page has loaded', async () => {
		type ListResult = Awaited<ReturnType<typeof apiModule.api.listPersons>>;
		let resolveSlow: (r: ListResult) => void = () => {};
		resetRouter('/persons');
		render(PersonsPage);
		await screen.findByText('Page 1 of 5');

		vi.mocked(apiModule.api.listPersons).mockReturnValueOnce(
			new Promise<ListResult>((r) => (resolveSlow = r))
		);
		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		back();
		await screen.findByText('Page 1 of 5');

		resolveSlow({ items: [persons[0]], total: 21 } as unknown as ListResult);
		await new Promise((r) => setTimeout(r, 0));
		expect(screen.getByText('Page 1 of 5')).toBeDefined();
		expect(screen.getAllByText(/Given\d+/).length).toBe(20);
	});

	it('replaces on sort and filter changes, and returns to page 1', async () => {
		resetRouter('/persons?page=3');
		render(PersonsPage);
		await screen.findByText('Page 3 of 5');

		await fireEvent.change(screen.getByLabelText('Sort by:'), { target: { value: 'birth_date' } });
		await waitFor(() => expect(currentPath()).toBe('/persons?sort=birth_date'));
		await fireEvent.click(screen.getByRole('button', { name: /Sort order/ }));
		await waitFor(() => expect(currentPath()).toBe('/persons?sort=birth_date&order=desc'));
		await fireEvent.click(screen.getByRole('button', { name: 'Quick Captures' }));
		await waitFor(() =>
			expect(currentPath()).toBe('/persons?sort=birth_date&order=desc&status=possible')
		);

		expect(historyLength()).toBe(1);
		await waitFor(() =>
			expect(lastListCall()).toMatchObject({
				offset: 0,
				sort: 'birth_date',
				order: 'desc',
				research_status: 'possible'
			})
		);
	});

	// Unknown and no status listed as two buckets (#1110).
	it('filters on one Not assessed bucket covering unknown and unset', async () => {
		resetRouter('/persons');
		render(PersonsPage);
		await screen.findByText('Page 1 of 5');

		const filter = screen.getByLabelText('Confidence:') as HTMLSelectElement;
		const labels = Array.from(filter.options, (o) => o.textContent?.trim());
		expect(labels).toEqual(['All', 'Certain', 'Probable', 'Possible', 'Not assessed']);

		await fireEvent.change(filter, { target: { value: 'unassessed' } });
		await waitFor(() => expect(currentPath()).toBe('/persons?status=unassessed'));
		await waitFor(() => expect(lastListCall()).toMatchObject({ research_status: 'unassessed' }));
	});

	// Links made before #1110 named the two statuses the bucket now covers.
	it.each(['unset', 'unknown'])('reads an older status=%s link as Not assessed', async (old) => {
		resetRouter(`/persons?status=${old}`);
		render(PersonsPage);
		await screen.findByText('Page 1 of 5');
		expect(lastListCall()).toMatchObject({ research_status: 'unassessed' });
		expect((screen.getByLabelText('Confidence:') as HTMLSelectElement).value).toBe('unassessed');
	});

	it('marks the Quick Captures chip pressed while its filter is on', async () => {
		resetRouter('/persons');
		render(PersonsPage);
		await screen.findByText('Page 1 of 5');

		const chip = screen.getByRole('button', { name: 'Quick Captures', pressed: false });
		await fireEvent.click(chip);
		await waitFor(() => expect(chip.getAttribute('aria-pressed')).toBe('true'));
	});

	it('moves a page past the end to the last page, replacing the URL (#948)', async () => {
		resetRouter('/persons?sort=birth_date&page=9');
		render(PersonsPage);
		await waitFor(() => expect(currentPath()).toBe('/persons?sort=birth_date&page=5'));
		expect(historyLength()).toBe(1);
		await screen.findByText('Page 5 of 5');
		expect(lastListCall()).toMatchObject({ offset: 80, sort: 'birth_date' });
	});

	it('drops the page from the URL and shows the empty state when there are no people (#948)', async () => {
		vi.mocked(apiModule.api.listPersons).mockResolvedValue({
			items: [],
			total: 0
		} as unknown as Awaited<ReturnType<typeof apiModule.api.listPersons>>);
		resetRouter('/persons?page=4');
		render(PersonsPage);
		await waitFor(() => expect(currentPath()).toBe('/persons'));
		expect(historyLength()).toBe(1);
		await screen.findByText('No people found.');
		expect(lastListCall()).toMatchObject({ offset: 0 });
	});

	it('offers Add Person and Import GEDCOM in the empty state (#1156)', async () => {
		vi.mocked(apiModule.api.listPersons).mockResolvedValue({
			items: [],
			total: 0
		} as unknown as Awaited<ReturnType<typeof apiModule.api.listPersons>>);
		resetRouter('/persons');
		const { container } = render(PersonsPage);
		await screen.findByText('No people found.');
		const empty = container.querySelector('.empty') as HTMLElement;
		const add = within(empty).getByRole('link', { name: 'Add Person' });
		expect(add.getAttribute('href')).toBe('/persons/add');
		expect(add.className).not.toMatch(/border-border/);
		const importLink = within(empty).getByRole('link', { name: 'Import GEDCOM' });
		expect(importLink.getAttribute('href')).toBe('/import');
		expect(importLink.className).toMatch(/border-border/);
		expect(add.parentElement).toBe(importLink.parentElement);
	});

	it('sorts Birth Date oldest first, as /search does (#1120)', async () => {
		resetRouter('/persons');
		render(PersonsPage);
		await waitFor(() => expect(lastListCall()).toMatchObject({ sort: 'surname' }));
		await fireEvent.change(screen.getByLabelText(/Sort by/), { target: { value: 'birth_date' } });
		await waitFor(() => expect(lastListCall()).toMatchObject({ sort: 'birth_date', order: 'asc' }));
	});

	it('choosing a sort resets a descending order to ascending, as /search does (#1120)', async () => {
		resetRouter('/persons?order=desc');
		render(PersonsPage);
		await waitFor(() => expect(lastListCall()).toMatchObject({ sort: 'surname', order: 'desc' }));
		await fireEvent.change(screen.getByLabelText(/Sort by/), { target: { value: 'birth_date' } });
		await waitFor(() => expect(lastListCall()).toMatchObject({ sort: 'birth_date', order: 'asc' }));
		expect(currentPath()).toBe('/persons?sort=birth_date');
	});
});
