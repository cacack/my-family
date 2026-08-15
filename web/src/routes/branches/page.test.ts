import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch } from '$lib/api/client';

// Hoisted so the module mocks below (which vitest lifts above the imports) can
// close over them.
const { mockState, listBranches, createBranch, deleteBranch, switchBranch } = vi.hoisted(() => ({
	mockState: {
		id: null as string | null,
		branch: null as Branch | null,
		revalidating: false,
		notice: null as string | null
	},
	listBranches: vi.fn(),
	createBranch: vi.fn(),
	deleteBranch: vi.fn(),
	switchBranch: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listBranches: () => listBranches(),
			createBranch: (data: apiModule.BranchCreate) => createBranch(data),
			deleteBranch: (id: string) => deleteBranch(id)
		}
	};
});

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: mockState,
	switchBranch: (branch: Branch | null) => switchBranch(branch)
}));

const active: Branch = {
	id: '11111111-1111-1111-1111-111111111111',
	name: 'Maternal Smith line',
	description: 'Chasing the 1880 census gap',
	base_position: 42,
	status: 'active',
	created_at: '2026-01-15T10:30:00Z'
};

const merged: Branch = {
	id: '22222222-2222-2222-2222-222222222222',
	name: 'Jones cemetery sweep',
	base_position: 10,
	status: 'merged',
	created_at: '2026-01-02T09:00:00Z',
	merged_at: '2026-01-09T12:00:00Z',
	merge_note: 'Confirmed by headstone photos'
};

const archived: Branch = {
	id: '33333333-3333-3333-3333-333333333333',
	name: 'Discarded Miller theory',
	base_position: 5,
	status: 'archived',
	created_at: '2026-01-01T08:00:00Z'
};

describe('Branches page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockState.id = null;
		mockState.branch = null;
		listBranches.mockResolvedValue({ items: [active, merged, archived], total: 3 });
		createBranch.mockResolvedValue(active);
		deleteBranch.mockResolvedValue(undefined);
	});

	// This file opens bits-ui overlays (the create Dialog, the delete
	// AlertDialog), and bits-ui releases its body-scroll lock on a 24ms timer
	// (`actualDelay = delay === null ? 24 : delay` in body-scroll-lock.svelte.js).
	// If the environment tears down inside that window the callback runs against
	// a destroyed document and throws `ReferenceError: document is not defined`
	// — an unhandled error that fails the run even though every test passed.
	// It is a race, so it surfaces intermittently: it took a scheduling shift
	// from an unrelated commit to expose it. Draining past the delay here makes
	// the cleanup run while the DOM still exists.
	afterEach(async () => {
		await new Promise((resolve) => setTimeout(resolve, 30));
	});

	it('groups branches by lifecycle status', async () => {
		render(Page);

		await screen.findByText('Maternal Smith line');
		expect(screen.getByRole('heading', { name: 'Active' })).toBeDefined();
		expect(screen.getByRole('heading', { name: 'Merged' })).toBeDefined();
		expect(screen.getByRole('heading', { name: 'Archived' })).toBeDefined();
	});

	it('shows the fork position, description and merge note', async () => {
		render(Page);

		await screen.findByText('Maternal Smith line');
		expect(screen.getByText('Chasing the 1880 census gap')).toBeDefined();
		expect(screen.getByText('position 42')).toBeDefined();
		expect(screen.getByText(/Confirmed by headstone photos/)).toBeDefined();
	});

	it('offers switch and delete only for active branches', async () => {
		render(Page);

		await screen.findByText('Maternal Smith line');
		expect(screen.getAllByRole('button', { name: /^Switch to branch$/ })).toHaveLength(1);
		expect(screen.getAllByRole('button', { name: /^Delete$/ })).toHaveLength(1);
	});

	it('delegates switching to the store', async () => {
		render(Page);

		const switchButton = await screen.findByRole('button', { name: /^Switch to branch$/ });
		await fireEvent.click(switchButton);

		await waitFor(() => {
			expect(switchBranch).toHaveBeenCalledWith(expect.objectContaining({ id: active.id }));
		});
	});

	it('offers a return to mainline instead of a switch for the current branch', async () => {
		mockState.id = active.id;
		mockState.branch = active;

		render(Page);

		await screen.findByText('Maternal Smith line');
		expect(screen.queryByRole('button', { name: /^Switch to branch$/ })).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: /^Return to mainline$/ }));
		await waitFor(() => {
			expect(switchBranch).toHaveBeenCalledWith(null);
		});
	});

	it('explains that branches are unconfigured when the server answers 503', async () => {
		listBranches.mockRejectedValue({
			status: 503,
			code: 'branches_unavailable',
			message: 'Branch registry is not configured on this server'
		});

		render(Page);

		await screen.findByText('Branches are not configured');
		expect(screen.queryByRole('button', { name: /new branch/i })).toBeNull();
	});

	it('shows an empty state when no branches exist', async () => {
		listBranches.mockResolvedValue({ items: [], total: 0 });

		render(Page);

		await screen.findByText('No research branches yet');
	});

	it('creates a branch, omitting an empty description rather than sending ""', async () => {
		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('button', { name: /new branch/i }));

		const nameInput = await screen.findByLabelText('Name');
		await fireEvent.input(nameInput, { target: { value: 'Paternal Doe line' } });
		await fireEvent.click(screen.getByRole('button', { name: /^Create branch$/ }));

		await waitFor(() => {
			expect(createBranch).toHaveBeenCalledWith({ name: 'Paternal Doe line' });
		});
	});

	it('accepts a full-length name with trailing whitespace, since the name is trimmed', async () => {
		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('button', { name: /new branch/i }));

		// 100 significant characters is the server's limit; the trailing space is
		// stripped before sending, so it must not cost the user a character.
		const name = 'a'.repeat(100);
		const nameInput = await screen.findByLabelText('Name');
		await fireEvent.input(nameInput, { target: { value: `${name} ` } });
		await fireEvent.click(screen.getByRole('button', { name: /^Create branch$/ }));

		await waitFor(() => {
			expect(createBranch).toHaveBeenCalledWith({ name });
		});
	});

	it('deletes a branch and explains a 409 as already merged or archived', async () => {
		deleteBranch.mockRejectedValue({
			status: 409,
			code: 'branch_not_active',
			message: 'Branch is not active'
		});

		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('button', { name: /^Delete$/ }));
		await fireEvent.click(await screen.findByRole('button', { name: /^Delete branch$/ }));

		await waitFor(() => {
			expect(deleteBranch).toHaveBeenCalledWith(active.id);
		});
		expect(await screen.findByText(/already been merged or archived/)).toBeDefined();
	});

	it('surfaces a load failure', async () => {
		listBranches.mockRejectedValue({ status: 500, code: 'internal', message: 'boom' });

		render(Page);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('boom');
	});
});
