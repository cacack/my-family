import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch } from '$lib/api/client';

// Hoisted so the module mocks below (which vitest lifts above the imports) can
// close over them.
const { mockState, listBranches, createBranch, closeBranch, promoteBranchResearchLogs, switchBranch } = vi.hoisted(() => ({
	mockState: {
		id: null as string | null,
		branch: null as Branch | null,
		revalidating: false,
		notice: null as string | null
	},
	listBranches: vi.fn(),
	createBranch: vi.fn(),
	closeBranch: vi.fn(),
	promoteBranchResearchLogs: vi.fn(),
	switchBranch: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listBranches: (options?: { includeDrift?: boolean }) => listBranches(options),
			createBranch: (data: apiModule.BranchCreate) => createBranch(data),
			closeBranch: (id: string, req: apiModule.BranchCloseRequest) => closeBranch(id, req),
			promoteBranchResearchLogs: (id: string, ids?: string[]) => promoteBranchResearchLogs(id, ids)
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
	outcome: 'open',
	subjects: [],
	proof_summary_ids: [],
	created_at: '2026-01-15T10:30:00Z'
};

const merged: Branch = {
	id: '22222222-2222-2222-2222-222222222222',
	name: 'Jones cemetery sweep',
	base_position: 10,
	status: 'merged',
	outcome: 'open',
	subjects: [],
	proof_summary_ids: [],
	created_at: '2026-01-02T09:00:00Z',
	merged_at: '2026-01-09T12:00:00Z',
	merge_note: 'Confirmed by headstone photos'
};

const archived: Branch = {
	id: '33333333-3333-3333-3333-333333333333',
	name: 'Discarded Miller theory',
	base_position: 5,
	status: 'archived',
	outcome: 'disproved',
	subjects: [],
	proof_summary_ids: [],
	created_at: '2026-01-01T08:00:00Z',
	closed_at: '2026-01-03T08:00:00Z',
	close_reason: 'The will names other heirs'
};

describe('Branches page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockState.id = null;
		mockState.branch = null;
		listBranches.mockResolvedValue({ items: [active, merged, archived], total: 3 });
		createBranch.mockResolvedValue(active);
		closeBranch.mockResolvedValue({ ...active, status: 'archived', outcome: 'disproved' });
		promoteBranchResearchLogs.mockResolvedValue({ promoted: ['l1'], skipped: [], truncated: false });
	});

	// This file opens bits-ui overlays (the create Dialog, the close
	// Dialog), and bits-ui releases its body-scroll lock on a 24ms timer
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
		expect(screen.getByRole('heading', { name: 'Closed' })).toBeDefined();
	});

	it('shows the fork position, description and merge note', async () => {
		render(Page);

		await screen.findByText('Maternal Smith line');
		expect(screen.getByText('Chasing the 1880 census gap')).toBeDefined();
		expect(screen.getByText('position 42')).toBeDefined();
		expect(screen.getByText(/Confirmed by headstone photos/)).toBeDefined();
	});

	it('offers switch and close only for active branches', async () => {
		render(Page);

		await screen.findByText('Maternal Smith line');
		expect(screen.getAllByRole('button', { name: /^Switch to branch$/ })).toHaveLength(1);
		expect(screen.getAllByRole('button', { name: /^Close$/ })).toHaveLength(1);
		expect(screen.queryByRole('button', { name: /delete/i })).toBeNull();
	});

	it("shows a closed branch's outcome, reason and a link to its research", async () => {
		render(Page);

		await screen.findByText('Discarded Miller theory');
		expect(screen.getByTestId('close-reason').textContent).toContain('The will names other heirs');
		const research = screen.getByRole('link', { name: /^Research$/ });
		expect(research.getAttribute('href')).toBe(`/branches/${archived.id}/research`);
		const outcomes = screen.getAllByTestId('branch-outcome').map((el) => el.getAttribute('data-outcome'));
		expect(outcomes).toContain('disproved');
	});

	it('filters by status and by outcome', async () => {
		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('radio', { name: 'Closed' }));
		expect(screen.queryByText('Maternal Smith line')).toBeNull();
		expect(screen.getByText('Discarded Miller theory')).toBeDefined();

		await fireEvent.click(screen.getByRole('radio', { name: 'All' }));
		await fireEvent.change(screen.getByLabelText('Filter by outcome'), { target: { value: 'open' } });
		expect(screen.getByText('Maternal Smith line')).toBeDefined();
		expect(screen.queryByText('Discarded Miller theory')).toBeNull();

		await fireEvent.change(screen.getByLabelText('Filter by outcome'), { target: { value: 'superseded' } });
		expect(screen.getByText('No branches match these filters.')).toBeDefined();
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

	it('creates a branch with its research question and a non-default outcome', async () => {
		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('button', { name: /new branch/i }));
		await fireEvent.input(await screen.findByLabelText('Name'), { target: { value: 'Mary line' } });
		await fireEvent.input(screen.getByLabelText('Research question (optional)'), {
			target: { value: '  Was Mary the daughter of John?  ' }
		});
		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'inconclusive' } });
		await fireEvent.click(screen.getByRole('button', { name: /^Create branch$/ }));

		await waitFor(() => {
			expect(createBranch).toHaveBeenCalledWith({
				name: 'Mary line',
				hypothesis: 'Was Mary the daughter of John?',
				outcome: 'inconclusive'
			});
		});
	});

	it("shows each branch's outcome, research question and subject count", async () => {
		listBranches.mockResolvedValue({
			items: [
				{
					...active,
					hypothesis: 'Was Mary the daughter of John?',
					subjects: [{ type: 'person', id: '99999999-9999-9999-9999-999999999999' }]
				},
				{ ...merged, outcome: 'proved' }
			],
			total: 2
		});
		render(Page);

		expect(await screen.findByTestId('card-hypothesis')).toBeDefined();
		expect(screen.getByText('Was Mary the daughter of John?')).toBeDefined();
		const outcomes = screen.getAllByTestId('branch-outcome').map((el) => el.getAttribute('data-outcome'));
		expect(outcomes).toEqual(['open', 'proved']);
		expect(screen.getByText('Subjects')).toBeDefined();
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

	it('closes a branch with an outcome and reason, then copies its research logs', async () => {
		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('button', { name: /^Close$/ }));
		const submit = await screen.findByRole('button', { name: /^Close branch$/ });
		expect((submit as HTMLButtonElement).disabled).toBe(true);

		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'disproved' } });
		await fireEvent.input(screen.getByLabelText('Reason (optional)'), {
			target: { value: '  The will names other heirs  ' }
		});
		await fireEvent.click(submit);

		await waitFor(() => {
			expect(closeBranch).toHaveBeenCalledWith(active.id, {
				outcome: 'disproved',
				reason: 'The will names other heirs'
			});
		});
		await waitFor(() => expect(promoteBranchResearchLogs).toHaveBeenCalledWith(active.id, undefined));
		const notice = await screen.findByText(/Closed "Maternal Smith line" as disproved/);
		expect(notice.textContent).toContain('1 research log was copied to the mainline.');
		expect(listBranches).toHaveBeenCalledTimes(2);
	});

	it('returns to the mainline when the current branch is closed', async () => {
		mockState.id = active.id;
		mockState.branch = active;
		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('button', { name: /^Close$/ }));
		await fireEvent.change(await screen.findByLabelText('Outcome'), { target: { value: 'abandoned' } });
		await fireEvent.click(screen.getByLabelText(/Copy this branch's research logs/));
		await fireEvent.click(screen.getByRole('button', { name: /^Close branch$/ }));

		await waitFor(() => expect(switchBranch).toHaveBeenCalledWith(null));
		expect(closeBranch).toHaveBeenCalledWith(active.id, { outcome: 'abandoned' });
		expect(promoteBranchResearchLogs).not.toHaveBeenCalled();
	});

	it('explains a 409 close as already merged or closed', async () => {
		closeBranch.mockRejectedValue({ status: 409, code: 'branch_not_active', message: 'Branch is not active' });

		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('button', { name: /^Close$/ }));
		await fireEvent.change(await screen.findByLabelText('Outcome'), { target: { value: 'inconclusive' } });
		await fireEvent.click(screen.getByRole('button', { name: /^Close branch$/ }));

		expect(await screen.findByText(/already been merged or closed/)).toBeDefined();
		expect(promoteBranchResearchLogs).not.toHaveBeenCalled();
	});

	it('reports a failed copy without undoing the close', async () => {
		promoteBranchResearchLogs.mockRejectedValue({ status: 500, message: 'copy failed' });
		closeBranch.mockResolvedValue({ ...active, status: 'archived', outcome: 'superseded' });

		render(Page);
		await screen.findByText('Maternal Smith line');

		await fireEvent.click(screen.getByRole('button', { name: /^Close$/ }));
		await fireEvent.change(await screen.findByLabelText('Outcome'), { target: { value: 'superseded' } });
		await fireEvent.click(screen.getByRole('button', { name: /^Close branch$/ }));

		expect(await screen.findByText(/copy failed/)).toBeDefined();
		expect(screen.getByText(/Closed "Maternal Smith line" as superseded/)).toBeDefined();
	});

	it('surfaces a load failure', async () => {
		listBranches.mockRejectedValue({ status: 500, code: 'internal', message: 'boom' });

		render(Page);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('boom');
	});

	it('flags a merged branch whose merge did not finish, and links to finishing it (#830)', async () => {
		const unfinished: Branch = {
			...merged,
			merge_state: 'incomplete',
			merge_pending: [
				{
					stream_id: '55555555-5555-5555-5555-555555555555',
					entity_type: 'person',
					entity_name: 'Ada Lovelace',
					reason: 'ready',
					needs_resolution: false,
					supported_resolutions: []
				}
			]
		};
		const finished: Branch = { ...archived, id: '66666666-6666-6666-6666-666666666666', status: 'merged', merge_state: 'complete' };
		listBranches.mockResolvedValue({ items: [unfinished, finished], total: 2 });

		render(Page);

		expect(await screen.findByText('Merge unfinished')).toBeDefined();
		expect(screen.getAllByText('Merge unfinished')).toHaveLength(1);
		expect(
			screen.getByText(/Its merge did not finish. 1 entity has not reached the mainline yet/)
		).toBeDefined();
		const finish = screen.getByRole('link', { name: 'Finish merge' });
		expect(finish.getAttribute('href')).toBe(`/branches/${unfinished.id}`);
	});

	it('flags a merged branch whose merge state could not be read (#830)', async () => {
		const unreadable: Branch = { ...merged, merge_state: 'unknown' };
		listBranches.mockResolvedValue({ items: [unreadable], total: 1 });

		render(Page);

		expect(await screen.findByText('Merge unfinished')).toBeDefined();
		expect(screen.getByText(/could not work out how far it got/)).toBeDefined();
		expect(screen.getByRole('link', { name: 'Finish merge' }).getAttribute('href')).toBe(`/branches/${unreadable.id}`);
	});

	describe('live-overlay semantics and drift (#837)', () => {
		it('asks the list for drift, so one request covers every card', async () => {
			render(Page);

			await screen.findByText('Maternal Smith line');
			expect(listBranches).toHaveBeenCalledTimes(1);
			expect(listBranches).toHaveBeenCalledWith({ includeDrift: true });
		});

		it('shows the main-moved counts on an active card, linking to compare', async () => {
			listBranches.mockResolvedValue({
				items: [
					{
						...active,
						drift: {
							branch_id: active.id,
							base_position: 42,
							main_change_count: 5,
							main_change_count_on_branch_entities: 1,
							has_more: false
						}
					},
					merged
				],
				total: 2
			});

			render(Page);

			const indicators = await screen.findAllByTestId('branch-drift');
			expect(indicators).toHaveLength(1);
			const text = (indicators[0].textContent ?? '').replace(/\s+/g, ' ');
			expect(text).toContain(
				'Mainline changed 5 times since you branched, 1 on entities this branch touched.'
			);
			expect(
				screen.getByRole('link', { name: 'Review mainline changes' }).getAttribute('href')
			).toBe(`/branches/${active.id}#main-changes`);
		});

		it('shows no indicator on a terminal branch, even if drift came back', async () => {
			listBranches.mockResolvedValue({
				items: [
					{
						...merged,
						drift: {
							branch_id: merged.id,
							base_position: 10,
							main_change_count: 9,
							main_change_count_on_branch_entities: 9,
							has_more: false
						}
					}
				],
				total: 1
			});

			render(Page);

			await screen.findByText('Jones cemetery sweep');
			expect(screen.queryByTestId('branch-drift')).toBeNull();
		});

		it('explains live semantics in the create dialog', async () => {
			render(Page);
			await screen.findByText('Maternal Smith line');

			await fireEvent.click(screen.getByRole('button', { name: /new branch/i }));

			const note = await screen.findByTestId('live-overlay-note');
			const text = (note.textContent ?? '').replace(/\s+/g, ' ');
			expect(text).toContain('Branches are live, not frozen');
			expect(text).toContain("records you don't edit on the branch keep showing the mainline's current data");
			expect(text).toContain('you never need to rebase');
			expect(text).toContain('Only the records you edit on the branch are held apart');
		});
	});
});
