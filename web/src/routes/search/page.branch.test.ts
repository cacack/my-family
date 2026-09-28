import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import SearchPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getPlaceHierarchy: vi.fn(),
			searchPersons: vi.fn()
		}
	};
});

const { branchState } = vi.hoisted(() => ({
	branchState: { id: null as string | null }
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState
}));

describe('Advanced Search page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(apiModule.api.getPlaceHierarchy).mockResolvedValue({
			items: []
		} as unknown as Awaited<ReturnType<typeof apiModule.api.getPlaceHierarchy>>);
	});

	it('labels nothing as mainline on a branch: results and place filter both follow it (#829)', async () => {
		branchState.id = 'b-1';
		render(SearchPage);
		await waitFor(() => expect(apiModule.api.getPlaceHierarchy).toHaveBeenCalled());
		expect(screen.getByRole('heading', { name: 'Advanced Search' })).toBeTruthy();
		expect(screen.queryByRole('note')).toBeNull();
	});
});
