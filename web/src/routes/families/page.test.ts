import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import FamiliesPage from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, resetRouter } from '$lib/test/fakeRouter';

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

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

describe('Families list page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/families');
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

	it('shows a load failure as an error with Retry, not as an empty list', async () => {
		vi.mocked(apiModule.api.listFamilies).mockRejectedValueOnce(new Error('boom'));
		const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
		render(FamiliesPage);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('Failed to load families');
		expect(screen.queryByText('No families found.')).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
		await waitFor(() => expect(screen.getByText('No families found.')).toBeTruthy());
		spy.mockRestore();
	});

	it('keeps the page in the URL: Next pushes it, Back and reload restore it (#901)', async () => {
		vi.mocked(apiModule.api.listFamilies).mockResolvedValue({
			items: [{ id: 'f-1', version: 1, partner1_name: 'Ada Lovelace' }],
			total: 60
		} as unknown as Awaited<ReturnType<typeof apiModule.api.listFamilies>>);
		resetRouter('/families?page=2');
		render(FamiliesPage);
		await waitFor(() => expect(screen.getByText('Page 2 of 3')).toBeTruthy());
		expect(apiModule.api.listFamilies).toHaveBeenLastCalledWith({ limit: 20, offset: 20 });

		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		await waitFor(() => expect(screen.getByText('Page 3 of 3')).toBeTruthy());
		expect(currentPath()).toBe('/families?page=3');

		back();
		await waitFor(() => expect(screen.getByText('Page 2 of 3')).toBeTruthy());
		expect(apiModule.api.listFamilies).toHaveBeenLastCalledWith({ limit: 20, offset: 20 });
	});
});
