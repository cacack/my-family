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
		notice: null as string | null,
		noticeHref: null as string | null
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
	outcome: 'open',
	subjects: [],
	proof_summary_ids: [],
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
		mockState.noticeHref = null;
	});

	it('links an unfinished-merge notice to the branch page to finish it (#830)', async () => {
		mockState.notice = 'Research branch "X" is merged, but its merge did not finish.';
		mockState.noticeHref = `/branches/${branch.id}`;

		render(BranchBanner);

		const link = screen.getByRole('link', { name: 'Finish merge' });
		expect(link.getAttribute('href')).toBe(`/branches/${branch.id}`);
		// jsdom cannot navigate; keep the click to what the banner does with it.
		link.addEventListener('click', (event) => event.preventDefault());
		await fireEvent.click(link);
		expect(dismissBranchNotice).toHaveBeenCalled();
	});

	it('offers no follow-up link for a plain notice', () => {
		mockState.notice = 'The research branch you were working on is no longer available.';
		render(BranchBanner);
		expect(screen.queryByRole('link', { name: 'Finish merge' })).toBeNull();
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

	it("shows the branch's research question and outcome", () => {
		mockState.id = branch.id;
		mockState.branch = { ...branch, hypothesis: 'Was Mary the daughter of John?', outcome: 'proved' };

		render(BranchBanner);

		expect(screen.getByText('Was Mary the daughter of John?')).toBeDefined();
		expect(screen.getByTestId('branch-outcome').textContent).toContain('Proved');
	});

	it("names the branch's subjects, linked to their pages, and summarises the rest", () => {
		mockState.id = branch.id;
		mockState.branch = {
			...branch,
			subjects: [
				{ type: 'person', id: 'a1111111-1111-1111-1111-111111111111', name: 'Mary Smith' },
				{ type: 'family', id: 'b2222222-2222-2222-2222-222222222222', name: 'John Smith & Ann Doe' },
				{ type: 'person', id: 'c3333333-3333-3333-3333-333333333333', name: 'John Smith' },
				{ type: 'person', id: 'd4444444-4444-4444-4444-444444444444', name: 'Ann Doe' },
				{ type: 'person', id: 'e5555555-5555-5555-5555-555555555555' }
			]
		};

		render(BranchBanner);

		const subjects = screen.getByTestId('banner-subjects');
		expect(subjects.textContent).toContain('About');
		expect(screen.getByRole('link', { name: 'Mary Smith' }).getAttribute('href')).toBe(
			'/persons/a1111111-1111-1111-1111-111111111111'
		);
		expect(screen.getByRole('link', { name: 'John Smith & Ann Doe' }).getAttribute('href')).toBe(
			'/families/b2222222-2222-2222-2222-222222222222'
		);
		expect(screen.getByRole('link', { name: 'John Smith' })).toBeDefined();
		expect(screen.queryByRole('link', { name: 'Ann Doe' })).toBeNull();
		expect(screen.getByRole('link', { name: '+2 more' }).getAttribute('href')).toBe(`/branches/${branch.id}`);
	});

	it('shows no subjects line when none are linked', () => {
		mockState.id = branch.id;
		mockState.branch = branch;

		render(BranchBanner);

		expect(screen.queryByTestId('banner-subjects')).toBeNull();
	});

	it('shows the outcome alone when no question is recorded', () => {
		mockState.id = branch.id;
		mockState.branch = branch;

		const { container } = render(BranchBanner);

		expect(screen.getByTestId('branch-outcome').textContent).toContain('Open');
		expect(container.querySelector('.branch-hypothesis')).toBeNull();
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
