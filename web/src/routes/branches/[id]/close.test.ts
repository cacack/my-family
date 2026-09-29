/**
 * Closing a branch from its page (#836), and how a closed branch reads there.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch, BranchComparisonResult } from '$lib/api/client';

const BRANCH_ID = '11111111-1111-1111-1111-111111111111';

const { mockState, compareBranch, closeBranch, promoteBranchResearchLogs, switchBranch } =
	vi.hoisted(() => ({
		mockState: {
			id: null as string | null,
			branch: null as Branch | null,
			revalidating: false,
			unconfirmed: false,
			notice: null as string | null
		},
		compareBranch: vi.fn(),
		closeBranch: vi.fn(),
		promoteBranchResearchLogs: vi.fn(),
		switchBranch: vi.fn()
	}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			compareBranch: (id: string) => compareBranch(id),
			closeBranch: (id: string, req: apiModule.BranchCloseRequest) => closeBranch(id, req),
			promoteBranchResearchLogs: (id: string, ids?: string[]) => promoteBranchResearchLogs(id, ids)
		}
	};
});

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: mockState,
	switchBranch: (branch: Branch | null) => switchBranch(branch),
	returnToMainline: vi.fn(),
	refreshActiveBranch: vi.fn()
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
	outcome: 'open',
	subjects: [],
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

describe('Branch page close', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockState.id = null;
	});

	// bits-ui releases its body-scroll lock on a timer; drain it (see the
	// branches list page test).
	afterEach(async () => {
		await new Promise((resolve) => setTimeout(resolve, 30));
	});

	it('closes the active branch, copies its logs and returns to the mainline', async () => {
		compareBranch.mockResolvedValue(comparison(branch));
		const closed: Branch = {
			...branch,
			status: 'archived',
			outcome: 'disproved',
			closed_at: '2026-02-01T10:00:00Z',
			close_reason: 'Other parents named'
		};
		closeBranch.mockResolvedValue(closed);
		promoteBranchResearchLogs.mockResolvedValue({
			promoted: [],
			skipped: [{ id: 'x', reason: 'subject_not_on_main' }],
			truncated: false
		});
		mockState.id = BRANCH_ID;
		render(Page);

		await fireEvent.click(await screen.findByRole('button', { name: 'Close branch' }));
		await fireEvent.change(await screen.findByLabelText('Outcome'), {
			target: { value: 'disproved' }
		});
		await fireEvent.input(screen.getByLabelText('Reason (optional)'), {
			target: { value: 'Other parents named' }
		});
		const buttons = screen.getAllByRole('button', { name: 'Close branch' });
		await fireEvent.click(buttons[buttons.length - 1]);

		await waitFor(() =>
			expect(closeBranch).toHaveBeenCalledWith(BRANCH_ID, {
				outcome: 'disproved',
				reason: 'Other parents named'
			})
		);
		expect(await screen.findByTestId('closed-record')).toBeDefined();
		expect(screen.getByText(/Closed as disproved\. No research logs were copied/)).toBeDefined();
		expect(switchBranch).toHaveBeenCalledWith(null);
		expect(screen.queryByRole('button', { name: 'Review & merge' })).toBeNull();
	});

	it('shows a closed branch with its reason and a link to its research', async () => {
		compareBranch.mockResolvedValue(
			comparison({
				...branch,
				status: 'archived',
				outcome: 'disproved',
				closed_at: '2026-02-01T10:00:00Z',
				close_reason: 'Other parents named'
			})
		);
		render(Page);

		const record = await screen.findByTestId('closed-record');
		expect(record.textContent).toContain('Other parents named');
		expect(
			screen.getByRole('link', { name: "View this branch's research" }).getAttribute('href')
		).toBe(`/branches/${BRANCH_ID}/research`);
		expect(screen.queryByRole('button', { name: 'Close branch' })).toBeNull();
		expect(screen.getByText('closed')).toBeDefined();
	});
});
