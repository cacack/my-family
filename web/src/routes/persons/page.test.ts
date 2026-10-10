import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import PersonsPage from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { listPersons: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

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
});
