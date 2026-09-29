import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import BranchBanner from './BranchBanner.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch, BranchDrift } from '$lib/api/client';

// Hoisted so the module mock below (which vitest lifts above the imports) can
// close over them.
const { mockState, returnToMainline, dismissBranchNotice, getBranchDrift, writeListeners } =
	vi.hoisted(() => ({
	mockState: {
		id: null as string | null,
		branch: null as Branch | null,
		revalidating: false,
		unconfirmed: false,
		notice: null as string | null,
		noticeHref: null as string | null
	},
	returnToMainline: vi.fn().mockResolvedValue(undefined),
	dismissBranchNotice: vi.fn(),
	getBranchDrift: vi.fn(),
	writeListeners: new Set<(branchId: string) => void>()
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: { getBranchDrift: (id: string) => getBranchDrift(id) },
		onBranchWrite: (listener: (branchId: string) => void) => {
			writeListeners.add(listener);
			return () => writeListeners.delete(listener);
		}
	};
});

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

function drift(overrides: Partial<BranchDrift>): BranchDrift {
	return {
		branch_id: branch.id,
		base_position: 42,
		main_change_count: 0,
		main_change_count_on_branch_entities: 0,
		has_more: false,
		...overrides
	};
}

describe('BranchBanner', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		writeListeners.clear();
		mockState.id = null;
		mockState.branch = null;
		mockState.revalidating = false;
		mockState.unconfirmed = false;
		mockState.notice = null;
		mockState.noticeHref = null;
		getBranchDrift.mockResolvedValue(drift({}));
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

	describe('live-overlay semantics (#837)', () => {
		it('explains that the branch is live, not frozen, and needs no rebase', () => {
			mockState.id = branch.id;
			mockState.branch = branch;

			render(BranchBanner);

			expect(screen.getByText('How branches work')).toBeDefined();
			const help = (
				screen.getByText(/live view over the mainline, not a frozen copy/).textContent ?? ''
			).replace(/\s+/g, ' ');
			expect(help).toMatch(/Records you haven't edited here always show the mainline's current data/);
			expect(help).toMatch(/nothing to rebase/);
			expect(help).toMatch(/Records you edit here are isolated/);
		});

		it('shows how far the mainline moved, linking to the compare section', async () => {
			mockState.id = branch.id;
			mockState.branch = branch;
			getBranchDrift.mockResolvedValue(
				drift({ main_change_count: 7, main_change_count_on_branch_entities: 2 })
			);

			render(BranchBanner);

			const indicator = await screen.findByTestId('branch-drift');
			expect(getBranchDrift).toHaveBeenCalledWith(branch.id);
			expect(indicator.textContent).toMatch(/Mainline changed 7\s+times since you branched/);
			expect(indicator.textContent).toMatch(/2 on entities this branch touched/);
			const link = screen.getByRole('link', { name: 'Review mainline changes' });
			expect(link.getAttribute('href')).toBe(`/branches/${branch.id}#main-changes`);
		});

		it('says the mainline is unchanged when nothing moved', async () => {
			mockState.id = branch.id;

			render(BranchBanner);

			expect(await screen.findByText('Mainline unchanged since you branched.')).toBeDefined();
		});

		it('says so when the counts could not be read', async () => {
			mockState.id = branch.id;
			getBranchDrift.mockRejectedValue(new Error('boom'));

			render(BranchBanner);

			expect(
				await screen.findByText("Couldn't check how far the mainline has moved.")
			).toBeDefined();
		});

		it('asks for nothing on the mainline', () => {
			render(BranchBanner);
			expect(getBranchDrift).not.toHaveBeenCalled();
		});

		it('re-reads the counts after a write lands on this branch', async () => {
			mockState.id = branch.id;
			getBranchDrift.mockResolvedValue(
				drift({ main_change_count: 3, main_change_count_on_branch_entities: 0 })
			);

			render(BranchBanner);
			await screen.findByTestId('branch-drift');
			expect(getBranchDrift).toHaveBeenCalledTimes(1);

			// The branch now touches an entity main also changed.
			getBranchDrift.mockResolvedValue(
				drift({ main_change_count: 3, main_change_count_on_branch_entities: 1 })
			);
			for (const listener of writeListeners) listener(branch.id);

			await waitFor(() => {
				expect(screen.getByTestId('branch-drift').textContent).toMatch(
					/1 on entities this branch touched/
				);
			});
			expect(getBranchDrift).toHaveBeenCalledTimes(2);
		});

		it('ignores writes reported for another branch', async () => {
			mockState.id = branch.id;

			render(BranchBanner);
			await screen.findByText('Mainline unchanged since you branched.');
			for (const listener of writeListeners) listener('some-other-branch');

			await Promise.resolve();
			expect(getBranchDrift).toHaveBeenCalledTimes(1);
		});

		it('re-reads the counts when the window regains focus', async () => {
			mockState.id = branch.id;

			render(BranchBanner);
			await screen.findByText('Mainline unchanged since you branched.');
			getBranchDrift.mockResolvedValue(drift({ main_change_count: 4 }));
			window.dispatchEvent(new Event('focus'));

			await waitFor(() => {
				expect(screen.getByTestId('branch-drift').textContent).toMatch(/Mainline changed 4/);
			});
		});

		it('re-reads the counts when the tab becomes visible again', async () => {
			mockState.id = branch.id;

			render(BranchBanner);
			await screen.findByText('Mainline unchanged since you branched.');
			document.dispatchEvent(new Event('visibilitychange'));

			await waitFor(() => {
				expect(getBranchDrift).toHaveBeenCalledTimes(2);
			});
		});

		it('keeps the last good counts when a refresh fails', async () => {
			mockState.id = branch.id;
			getBranchDrift.mockResolvedValue(drift({ main_change_count: 5 }));

			render(BranchBanner);
			await screen.findByTestId('branch-drift');
			getBranchDrift.mockRejectedValue(new Error('offline'));
			window.dispatchEvent(new Event('focus'));

			await waitFor(() => {
				expect(getBranchDrift).toHaveBeenCalledTimes(2);
			});
			expect(screen.getByTestId('branch-drift').textContent).toMatch(/Mainline changed 5/);
			expect(screen.queryByText("Couldn't check how far the mainline has moved.")).toBeNull();
		});

		it('stops listening once unmounted', async () => {
			mockState.id = branch.id;

			const { unmount } = render(BranchBanner);
			await screen.findByText('Mainline unchanged since you branched.');
			expect(writeListeners.size).toBe(1);
			unmount();
			expect(writeListeners.size).toBe(0);
		});
	});
});
