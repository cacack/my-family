import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import AnalyticsPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listPersons: vi.fn(),
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

describe('Data Quality (analytics) page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(apiModule.api.listPersons).mockResolvedValue({
			items: [],
			total: 0
		} as unknown as apiModule.PersonList);
		vi.mocked(apiModule.api.listFamilies).mockResolvedValue({
			items: [],
			total: 0
		} as unknown as Awaited<ReturnType<typeof apiModule.api.listFamilies>>);
	});

	it('shows no mainline notice on the mainline', async () => {
		render(AnalyticsPage);
		await waitFor(() => expect(screen.getByText('Total Persons')).toBeTruthy());
		expect(screen.queryByRole('note')).toBeNull();
	});

	it('says exactly which parts are mainline on a branch (#825)', async () => {
		branchState.id = 'b-1';
		render(AnalyticsPage);
		await waitFor(() => expect(screen.getByText('Total Persons')).toBeTruthy());
		const note = screen.getByRole('note');
		// People follow the branch; only the family-derived figures are mainline.
		expect(note.textContent).toContain('people and their quality scores follow your branch');
		expect(note.textContent).toContain('families still come from the mainline');
		expect(note.textContent).not.toContain('always shows mainline data');
	});
});
