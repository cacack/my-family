import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import RepositoriesPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listRepositories: vi.fn(),
			createRepository: vi.fn()
		}
	};
});

const mockList: apiModule.RepositoryList = {
	repositories: [
		{
			id: 'repo-1',
			name: 'National Archives',
			address: { city: 'Washington', state: 'DC', country: 'USA' },
			version: 1
		},
		{
			id: 'repo-2',
			name: 'Local Library',
			version: 1
		}
	],
	total: 2
};

describe('Repositories list page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.listRepositories).mockResolvedValue(mockList);
		vi.mocked(apiModule.api.createRepository).mockResolvedValue({
			id: 'repo-3',
			name: 'New Repo',
			version: 1
		});
	});

	it('loads and renders repositories in a table with an address summary', async () => {
		render(RepositoriesPage);

		await waitFor(() => {
			expect(screen.getByText('National Archives')).toBeTruthy();
		});
		expect(screen.getByText('Local Library')).toBeTruthy();
		// Address summary composed from city/state/country
		expect(screen.getByText('Washington, DC, USA')).toBeTruthy();
		// Detail link uses the repository id
		const link = screen.getByText('National Archives').closest('a');
		expect(link?.getAttribute('href')).toBe('/repositories/repo-1');
	});

	it('rejects a blank (whitespace-only) name before creating', async () => {
		render(RepositoriesPage);
		await waitFor(() => expect(screen.getByText('Local Library')).toBeTruthy());

		await fireEvent.click(screen.getByRole('button', { name: 'Add Repository' }));
		// A whitespace-only name satisfies the native `required` attribute but must
		// still be rejected by the trim()-based guard.
		await fireEvent.input(screen.getByLabelText(/Name/), { target: { value: '   ' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Create Repository' }));

		await waitFor(() => expect(screen.getByText('Name is required')).toBeTruthy());
		expect(apiModule.api.createRepository).not.toHaveBeenCalled();
	});

	it('creates a repository with a nested address and reloads', async () => {
		render(RepositoriesPage);
		await waitFor(() => expect(screen.getByText('Local Library')).toBeTruthy());

		await fireEvent.click(screen.getByRole('button', { name: 'Add Repository' }));

		await fireEvent.input(screen.getByLabelText(/Name/), { target: { value: 'New Repo' } });
		await fireEvent.input(screen.getByLabelText('City'), { target: { value: 'Boston' } });

		await fireEvent.click(screen.getByRole('button', { name: 'Create Repository' }));

		await waitFor(() => {
			expect(apiModule.api.createRepository).toHaveBeenCalledWith({
				name: 'New Repo',
				address: { city: 'Boston' },
				notes: undefined,
				gedcom_xref: undefined
			});
		});
		// Reloaded the list after create (initial load + post-create reload)
		expect(apiModule.api.listRepositories).toHaveBeenCalledTimes(2);
	});
});
