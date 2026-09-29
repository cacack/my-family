/**
 * The branch page's research record (#835): shown in the header, edited in
 * place, and handed to the store so the banner follows.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch, BranchComparisonResult, BranchUpdate } from '$lib/api/client';

const BRANCH_ID = '11111111-1111-1111-1111-111111111111';
const PERSON_ID = '99999999-9999-9999-9999-999999999999';

const { mockState, compareBranch, updateBranch, listProofSummaries, refreshActiveBranch } = vi.hoisted(
	() => ({
		mockState: {
			id: null as string | null,
			branch: null as Branch | null,
			revalidating: false,
			unconfirmed: false,
			notice: null as string | null
		},
		compareBranch: vi.fn(),
		updateBranch: vi.fn(),
		listProofSummaries: vi.fn(),
		refreshActiveBranch: vi.fn()
	})
);

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			compareBranch: (id: string) => compareBranch(id),
			updateBranch: (id: string, data: BranchUpdate) => updateBranch(id, data),
			listProofSummaries: (params: unknown) => listProofSummaries(params)
		}
	};
});

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: mockState,
	switchBranch: vi.fn(),
	returnToMainline: vi.fn(),
	refreshActiveBranch: (branch: Branch) => refreshActiveBranch(branch)
}));

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (callback: (value: { params: { id: string } }) => void) => {
			callback({ params: { id: BRANCH_ID } });
			return () => {};
		}
	}
}));

const branch: Branch = {
	id: BRANCH_ID,
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'active',
	hypothesis: 'Was Mary the daughter of John?',
	outcome: 'open',
	subjects: [{ type: 'person', id: PERSON_ID, name: 'Mary Smith' }],
	proof_summary_ids: [],
	proof_summaries: [],
	created_at: '2026-01-15T10:30:00Z'
};

function comparison(b: Branch): BranchComparisonResult {
	return {
		branch: b,
		base_position: 42,
		branch_changes: [],
		main_changes: [],
		branch_change_count: 0,
		main_change_count: 0,
		has_more: false,
		overlapping_stream_ids: [],
		conflicts: []
	};
}

describe('Branch page research record', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockState.id = null;
		listProofSummaries.mockResolvedValue({ summaries: [], total: 0 });
	});

	it('shows the question, outcome and linked subjects in the header', async () => {
		compareBranch.mockResolvedValue(comparison(branch));
		render(Page);

		expect(await screen.findByTestId('branch-hypothesis')).toBeDefined();
		expect(screen.getByText('Was Mary the daughter of John?')).toBeDefined();
		expect(screen.getByTestId('branch-outcome').textContent).toContain('Open');
		expect(screen.getByRole('link', { name: 'Mary Smith' }).getAttribute('href')).toBe(`/persons/${PERSON_ID}`);
	});

	it('edits the outcome in place and hands the saved branch to the store', async () => {
		compareBranch.mockResolvedValue(comparison(branch));
		const saved = { ...branch, outcome: 'proved' as const };
		updateBranch.mockResolvedValue(saved);
		mockState.id = BRANCH_ID;
		render(Page);

		await fireEvent.click(await screen.findByRole('button', { name: 'Edit research record' }));
		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'proved' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));

		await waitFor(() => expect(screen.getByTestId('branch-outcome').textContent).toContain('Proved'));
		expect(updateBranch).toHaveBeenCalledWith(BRANCH_ID, { outcome: 'proved' });
		expect(refreshActiveBranch).toHaveBeenCalledWith(saved);
		expect(screen.queryByTestId('branch-research-editor')).toBeNull();
	});

	it('says the subject links open elsewhere unless this branch is the active one', async () => {
		compareBranch.mockResolvedValue(comparison(branch));
		const { unmount } = render(Page);
		expect((await screen.findByTestId('branch-research-scope-note')).textContent).toContain('the mainline');
		unmount();

		mockState.id = '22222222-2222-2222-2222-222222222222';
		const second = render(Page);
		expect((await screen.findByTestId('branch-research-scope-note')).textContent).toContain('the active branch');
		second.unmount();

		mockState.id = BRANCH_ID;
		render(Page);
		await screen.findByTestId('branch-research');
		expect(screen.queryByTestId('branch-research-scope-note')).toBeNull();
	});

	it("offers the persons the branch changed as subjects in the editor", async () => {
		const NEW_ID = '44444444-4444-4444-4444-444444444444';
		compareBranch.mockResolvedValue({
			...comparison(branch),
			branch_changes: [
				{
					id: 'c1',
					timestamp: '2026-01-15T10:30:00Z',
					entity_type: 'person',
					entity_id: NEW_ID,
					entity_name: 'Branch Only',
					action: 'created'
				}
			],
			branch_change_count: 1
		});
		render(Page);

		await fireEvent.click(await screen.findByRole('button', { name: 'Edit research record' }));
		expect(screen.getByRole('button', { name: 'Add person: Branch Only' })).toBeDefined();
	});

	it('offers to record the outcome of a merged branch, and nothing on an archived one', async () => {
		compareBranch.mockResolvedValue(comparison({ ...branch, status: 'merged' }));
		const { unmount } = render(Page);
		await fireEvent.click(await screen.findByRole('button', { name: 'Record outcome' }));
		expect(screen.queryByLabelText('Research question')).toBeNull();
		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		expect(screen.getByTestId('branch-research')).toBeDefined();
		unmount();

		compareBranch.mockResolvedValue(comparison({ ...branch, status: 'archived' }));
		render(Page);
		await screen.findByTestId('branch-research');
		expect(screen.queryByRole('button', { name: /Edit research record|Record outcome/ })).toBeNull();
	});
});
