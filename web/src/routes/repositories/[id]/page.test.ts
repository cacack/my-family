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
});
