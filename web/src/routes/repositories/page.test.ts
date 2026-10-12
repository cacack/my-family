import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import RepositoriesPage from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

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

// The real store exposes a read-only view, so the active branch is injected here.
const { branchState } = vi.hoisted(() => ({
	branchState: { id: null as string | null }
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState
}));

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
		resetRouter('/repositories');
		branchState.id = null;
		vi.mocked(apiModule.api.listRepositories).mockResolvedValue(mockList);
		vi.mocked(apiModule.api.createRepository).mockResolvedValue({
			id: 'repo-3',
			name: 'New Repo',
			version: 1
		});
	});

	it('has no shared-repository notice on the mainline', async () => {
		render(RepositoriesPage);
		await waitFor(() => expect(screen.getByText('National Archives')).toBeTruthy());
		expect(screen.queryByText(/shared across all branches/)).toBeNull();
	});

	it('says repositories are shared across all branches while a branch is active (#825)', async () => {
		branchState.id = 'b-1';
		render(RepositoriesPage);
		await waitFor(() => expect(screen.getByText('National Archives')).toBeTruthy());
		expect(
			screen.getByText(/Repositories are shared across all branches/).textContent
		).toContain('changes the mainline');
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

	const manyRepos: apiModule.RepositoryList = { repositories: mockList.repositories, total: 45 };

	it('restores sort and page from the URL (#946)', async () => {
		vi.mocked(apiModule.api.listRepositories).mockResolvedValue(manyRepos);
		resetRouter('/repositories?sort=updated_at&order=desc&page=2');
		render(RepositoriesPage);
		await waitFor(() =>
			expect(apiModule.api.listRepositories).toHaveBeenCalledWith({
				limit: 20,
				offset: 20,
				sort: 'updated_at',
				order: 'desc'
			})
		);
		await screen.findByText('Page 2 of 3');
	});

	it('pushes page changes and replaces sort changes (#946)', async () => {
		vi.mocked(apiModule.api.listRepositories).mockResolvedValue(manyRepos);
		render(RepositoriesPage);
		await screen.findByText('Page 1 of 3');

		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		await waitFor(() => expect(currentPath()).toBe('/repositories?page=2'));
		expect(historyLength()).toBe(2);

		await fireEvent.change(screen.getByLabelText('Sort by:'), { target: { value: 'updated_at' } });
		await waitFor(() => expect(currentPath()).toBe('/repositories?sort=updated_at'));
		expect(historyLength()).toBe(2);

		back();
		await screen.findByText('Page 1 of 3');
		expect(currentPath()).toBe('/repositories');
	});
});
