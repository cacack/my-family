import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import FamiliesPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listFamilies: vi.fn()
		}
	};
});

const { branchState } = vi.hoisted(() => ({
	branchState: { id: null as string | null }
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState
}));

describe('Families list page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(apiModule.api.listFamilies).mockResolvedValue({
			items: [],
			total: 0
		} as unknown as Awaited<ReturnType<typeof apiModule.api.listFamilies>>);
	});

	it('labels nothing as mainline on a branch: the list follows the branch (#829)', async () => {
		branchState.id = 'b-1';
		render(FamiliesPage);
		await waitFor(() => expect(screen.getByText('No families found.')).toBeTruthy());
		expect(apiModule.api.listFamilies).toHaveBeenCalledWith({
			limit: 20,
			offset: 0
		});
		expect(screen.queryByRole('note')).toBeNull();
	});
});
