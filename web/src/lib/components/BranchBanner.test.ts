import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import BranchBanner from './BranchBanner.svelte';
import type { Branch } from '$lib/api/client';

// Hoisted so the module mock below (which vitest lifts above the imports) can
// close over them.
const { mockState, returnToMainline, dismissBranchNotice } = vi.hoisted(() => ({
	mockState: {
		id: null as string | null,
		branch: null as Branch | null,
		revalidating: false,
		unconfirmed: false,
		notice: null as string | null
	},
	returnToMainline: vi.fn().mockResolvedValue(undefined),
	dismissBranchNotice: vi.fn()
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: mockState,
	returnToMainline: () => returnToMainline(),
	dismissBranchNotice: () => dismissBranchNotice()
}));

const branch: Branch = {
	id: '44444444-4444-4444-4444-444444444444',
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'active',
	created_at: '2026-01-15T10:30:00Z'
};

describe('BranchBanner', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockState.id = null;
		mockState.branch = null;
		mockState.revalidating = false;
		mockState.unconfirmed = false;
		mockState.notice = null;
	});

	it('says so when the branch status could not be confirmed, without dropping it', () => {
		mockState.id = branch.id;
		mockState.branch = branch;
		mockState.unconfirmed = true;

		render(BranchBanner);

		expect(screen.getByText(/Couldn't confirm this branch's status/)).toBeDefined();
		// Still on the branch: an unreachable server is not evidence it is gone.
		expect(screen.getByText('Maternal Smith line')).toBeDefined();
	});

	it('renders nothing on the mainline', () => {
		const { container } = render(BranchBanner);
		expect(container.querySelector('.branch-banner')).toBeNull();
		expect(container.querySelector('.branch-notice')).toBeNull();
	});

	it('names the active branch and offers a one-click return to mainline', () => {
		mockState.id = branch.id;
		mockState.branch = branch;

		const { container } = render(BranchBanner);

		expect(container.querySelector('.branch-banner')).not.toBeNull();
		expect(screen.getByText('Maternal Smith line')).toBeDefined();
		expect(screen.getByRole('button', { name: /return to mainline/i })).toBeDefined();
	});

	it('still shows the banner before the branch record has loaded', () => {
		mockState.id = branch.id;

		const { container } = render(BranchBanner);

		expect(container.querySelector('.branch-banner')).not.toBeNull();
		expect(screen.getByRole('button', { name: /return to mainline/i })).toBeDefined();
	});

	it('returns to the mainline when the button is clicked', async () => {
		mockState.id = branch.id;
		mockState.branch = branch;

		render(BranchBanner);
		await fireEvent.click(screen.getByRole('button', { name: /return to mainline/i }));

		await waitFor(() => {
			expect(returnToMainline).toHaveBeenCalled();
		});
	});

	it('shows the stale-branch notice even though no branch is active', () => {
		mockState.notice = 'The research branch you were working on is no longer available.';

		const { container } = render(BranchBanner);

		expect(container.querySelector('.branch-notice')).not.toBeNull();
		expect(container.querySelector('.branch-banner')).toBeNull();
		expect(screen.getByText(/no longer available/i)).toBeDefined();
	});

	it('dismisses the stale-branch notice', async () => {
		mockState.notice = 'The research branch you were working on is no longer available.';

		render(BranchBanner);
		await fireEvent.click(screen.getByRole('button', { name: /dismiss/i }));

		expect(dismissBranchNotice).toHaveBeenCalled();
	});
});
