import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import RepositoryPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getRepository: vi.fn(),
			updateRepository: vi.fn(),
			deleteRepository: vi.fn()
		}
	};
});

vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((callback: (value: unknown) => void) => {
			callback({ params: { id: 'repo-1' } });
			return () => {};
		})
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

const { branchState } = vi.hoisted(() => ({
	branchState: { id: null as string | null }
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState
}));

const SHARED = /Repositories are shared across all branches/;

describe('Repository detail page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(apiModule.api.getRepository).mockResolvedValue({
			id: 'repo-1',
			name: 'National Archives',
			version: 1
		} as apiModule.RepositoryDetail);
	});

	it('shows no shared-repository notice on the mainline', async () => {
		render(RepositoryPage);
		await waitFor(() => expect(screen.getByText('National Archives')).toBeTruthy());
		expect(screen.queryByText(SHARED)).toBeNull();
	});

	it('says repositories are shared across branches, in view and edit mode (#825)', async () => {
		branchState.id = 'b-1';
		render(RepositoryPage);
		await waitFor(() => expect(screen.getByText('National Archives')).toBeTruthy());
		expect(screen.getByText(SHARED)).toBeTruthy();

		await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
		expect(screen.getByText(SHARED)).toBeTruthy();
	});

	it('sends cleared notes and address so the API clears them', async () => {
		vi.mocked(apiModule.api.getRepository).mockResolvedValue({
			id: 'repo-1',
			name: 'National Archives',
			notes: 'Old notes',
			address: { city: 'Washington' },
			version: 1
		} as apiModule.RepositoryDetail);
		vi.mocked(apiModule.api.updateRepository).mockResolvedValue({
			id: 'repo-1',
			name: 'National Archives',
			version: 2
		} as apiModule.Repository);
		render(RepositoryPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));

		await fireEvent.input(screen.getByLabelText('City'), { target: { value: '' } });
		await fireEvent.input(screen.getByLabelText('Notes'), { target: { value: '' } });
		await fireEvent.click(screen.getByRole('button', { name: /Save/ }));

		await waitFor(() => expect(apiModule.api.updateRepository).toHaveBeenCalled());
		const body = vi.mocked(apiModule.api.updateRepository).mock.calls[0][1];
		expect(body).toMatchObject({ notes: '', gedcom_xref: '', address: {} });
	});
});

describe('Repository detail page: failed actions (#899)', () => {
	const api = apiModule.api;

	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(api.getRepository).mockResolvedValue({
			id: 'repo-1',
			name: 'National Archives',
			version: 1
		} as apiModule.RepositoryDetail);
	});

	it('shows a refused delete beside the repository it kept', async () => {
		vi.mocked(api.deleteRepository).mockRejectedValueOnce({ status: 500, message: 'Database unavailable' });
		vi.spyOn(window, 'confirm').mockReturnValue(true);
		render(RepositoryPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));

		expect((await screen.findByRole('alert')).textContent).toContain('Database unavailable');
		expect(screen.getByText('National Archives')).toBeTruthy();
	});

	it('says a malformed id is not found, and offers Retry', async () => {
		vi.mocked(api.getRepository).mockRejectedValueOnce({
			status: 400,
			message: "Invalid format for parameter id: error unmarshaling 'xyz'"
		});
		render(RepositoryPage);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('This repository could not be found');
		expect(alert.textContent).not.toContain('unmarshaling');
		await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
		expect(await screen.findByText('National Archives')).toBeTruthy();
	});
});
