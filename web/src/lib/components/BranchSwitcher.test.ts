import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import BranchSwitcher from './BranchSwitcher.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch } from '$lib/api/client';

// Hoisted so the module mocks below (which vitest lifts above the imports) can
// close over them.
const { mockState, listBranches, switchBranch } = vi.hoisted(() => ({
	mockState: {
		id: null as string | null,
		branch: null as Branch | null,
		revalidating: false,
		unconfirmed: false,
		notice: null as string | null
	},
	listBranches: vi.fn(),
	switchBranch: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: { listBranches: () => listBranches() }
	};
});

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: mockState,
	switchBranch: (branch: Branch | null) => switchBranch(branch)
}));

const branch: Branch = {
	id: '44444444-4444-4444-4444-444444444444',
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'active',
	created_at: '2026-01-15T10:30:00Z'
};

describe('BranchSwitcher', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockState.id = null;
		mockState.branch = null;
		mockState.notice = null;
		listBranches.mockResolvedValue({ items: [branch], total: 1 });
	});

	it('shows the mainline as the current scope by default', () => {
		render(BranchSwitcher);
		expect(screen.getByText('Mainline')).toBeDefined();
	});

	it('names the active branch on the trigger', () => {
		mockState.id = branch.id;
		mockState.branch = branch;

		render(BranchSwitcher);
		expect(screen.getByText('Maternal Smith line')).toBeDefined();
	});

	it('labels the trigger with the current scope for screen readers', () => {
		mockState.id = branch.id;
		mockState.branch = branch;

		render(BranchSwitcher);
		expect(
			screen.getByLabelText('Switch research branch. Currently on Maternal Smith line')
		).toBeDefined();
	});

	it('does not fetch the branch list until the menu is opened', () => {
		render(BranchSwitcher);
		expect(listBranches).not.toHaveBeenCalled();
	});

	it('lists only active branches once opened, and switches to the chosen one', async () => {
		listBranches.mockResolvedValue({
			items: [
				branch,
				{ ...branch, id: 'merged-id', name: 'Already merged', status: 'merged' },
				{ ...branch, id: 'archived-id', name: 'Already archived', status: 'archived' }
			],
			total: 3
		});

		render(BranchSwitcher);
		await fireEvent.pointerDown(screen.getByRole('button', { name: /switch research branch/i }));

		await waitFor(() => {
			expect(listBranches).toHaveBeenCalled();
		});

		const item = await screen.findByRole('menuitem', { name: /Maternal Smith line/ });
		expect(screen.queryByRole('menuitem', { name: /Already merged/ })).toBeNull();
		expect(screen.queryByRole('menuitem', { name: /Already archived/ })).toBeNull();

		await fireEvent.click(item);
		await waitFor(() => {
			expect(switchBranch).toHaveBeenCalledWith(expect.objectContaining({ id: branch.id }));
		});
	});

	it('applies only the newest list response when two opens overlap', async () => {
		const stale: Branch = { ...branch, id: 'stale-id', name: 'Stale branch' };
		let resolveFirst!: (value: { items: Branch[]; total: number }) => void;
		listBranches
			.mockImplementationOnce(
				() => new Promise<{ items: Branch[]; total: number }>((r) => (resolveFirst = r))
			)
			.mockResolvedValueOnce({ items: [branch], total: 1 });

		render(BranchSwitcher);
		const trigger = screen.getByRole('button', { name: /switch research branch/i });

		// Open, close, open again: two fetches in flight, the first slower.
		await fireEvent.pointerDown(trigger);
		await fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' });
		await fireEvent.pointerDown(trigger);

		await waitFor(() => {
			expect(listBranches).toHaveBeenCalledTimes(2);
		});
		await screen.findByRole('menuitem', { name: /Maternal Smith line/ });

		resolveFirst({ items: [stale], total: 1 });
		await waitFor(() => {
			expect(screen.queryByRole('menuitem', { name: /Stale branch/ })).toBeNull();
		});
		expect(screen.getByRole('menuitem', { name: /Maternal Smith line/ })).toBeDefined();
	});

	// The menuitem role must land on the anchor ITSELF, not on a wrapper around
	// it. bits-ui 2 dropped the `href` prop, so an anchor nested inside a
	// default-rendered Item leaves the Item as the interactive element: it
	// swallows the activation keypress and Enter never navigates. Asserting the
	// tag name is what catches that - the link still renders and still clicks
	// with a mouse either way, so a laxer assertion would pass on the broken form.
	it('exposes the manage-branches entry as the anchor itself, so Enter navigates', async () => {
		listBranches.mockResolvedValue({ items: [], total: 0 });

		render(BranchSwitcher);
		await fireEvent.pointerDown(screen.getByRole('button', { name: /switch research branch/i }));

		const link = await screen.findByRole('menuitem', { name: /manage branches/i });
		expect(link.tagName).toBe('A');
		expect(link.getAttribute('href')).toBe('/branches');
	});
});
