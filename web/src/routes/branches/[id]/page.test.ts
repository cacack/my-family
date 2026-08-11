import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch, BranchComparisonResult } from '$lib/api/client';

const BRANCH_ID = '11111111-1111-1111-1111-111111111111';
const PERSON_ID = '99999999-9999-9999-9999-999999999999';
const OTHER_ID = '88888888-8888-8888-8888-888888888888';

// Hoisted so the module mocks below (which vitest lifts above the imports) can
// close over them.
const { mockState, compareBranch, switchBranch, routeState } = vi.hoisted(() => ({
	mockState: {
		id: null as string | null,
		branch: null as Branch | null,
		revalidating: false,
		unconfirmed: false,
		notice: null as string | null
	},
	compareBranch: vi.fn(),
	switchBranch: vi.fn().mockResolvedValue(undefined),
	// A soft navigation between two /branches/{id} entries reuses the component,
	// so the route store has to be drivable rather than fixed.
	routeState: {
		current: { params: { id: '' } } as { params: { id: string } },
		subscribers: new Set<(value: { params: { id: string } }) => void>()
	}
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: { compareBranch: (id: string) => compareBranch(id) }
	};
});

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: mockState,
	switchBranch: (branch: Branch | null) => switchBranch(branch)
}));

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (callback: (value: { params: { id: string } }) => void) => {
			routeState.subscribers.add(callback);
			callback(routeState.current);
			return () => routeState.subscribers.delete(callback);
		}
	}
}));

/** Navigate to another `/branches/{id}` without remounting the component. */
function navigateTo(id: string) {
	// A fresh object: Svelte's store bridge dedupes on identity, so mutating the
	// existing one would not re-run the effect.
	routeState.current = { params: { id } };
	for (const subscriber of routeState.subscribers) {
		subscriber(routeState.current);
	}
}

const branch: Branch = {
	id: BRANCH_ID,
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'active',
	created_at: '2026-01-15T10:30:00Z'
};

function comparison(overrides: Partial<BranchComparisonResult> = {}): BranchComparisonResult {
	return {
		branch,
		base_position: 42,
		branch_changes: [
			{
				id: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
				timestamp: '2026-01-16T10:30:00Z',
				entity_type: 'person',
				entity_id: PERSON_ID,
				entity_name: 'Ada Lovelace',
				action: 'updated',
				changes: { surname: { old_value: 'Byron', new_value: 'Lovelace' } }
			}
		],
		main_changes: [
			{
				id: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
				timestamp: '2026-01-17T10:30:00Z',
				entity_type: 'person',
				entity_id: PERSON_ID,
				entity_name: 'Ada Lovelace',
				action: 'updated',
				changes: { surname: { old_value: 'Byron', new_value: 'King' } }
			}
		],
		branch_change_count: 1,
		main_change_count: 1,
		has_more: false,
		overlapping_stream_ids: [PERSON_ID],
		conflicts: [
			{
				stream_id: PERSON_ID,
				supported_resolutions: ['branch', 'main'],
				entity_type: 'person',
				entity_name: 'Ada Lovelace',
				kind: 'edit_edit',
				fields: ['surname'],
				detail: 'Both sides changed surname to different values'
			}
		],
		...overrides
	};
}

describe('Branch comparison page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockState.id = null;
		mockState.branch = null;
		routeState.current = { params: { id: BRANCH_ID } };
		routeState.subscribers.clear();
		compareBranch.mockResolvedValue(comparison());
	});

	it('loads the comparison for the routed branch', async () => {
		render(Page);
		await screen.findByRole('heading', { name: 'Maternal Smith line' });
		expect(compareBranch).toHaveBeenCalledWith(BRANCH_ID);
	});

	it('shows both sides of the divergence with their diffs', async () => {
		render(Page);

		await screen.findByRole('heading', { name: 'On this branch' });
		expect(screen.getByRole('heading', { name: 'On the mainline' })).toBeDefined();
		expect(screen.getByText('Lovelace')).toBeDefined();
		expect(screen.getByText('King')).toBeDefined();
	});

	it('reports conflicts as the verdict, with the contested fields', async () => {
		render(Page);

		await screen.findByText('Both sides changed surname to different values');
		expect(screen.getByText(/Contested fields: surname/)).toBeDefined();
	});

	it('does not repeat a conflicted entity as a clean overlap hint', async () => {
		render(Page);

		await screen.findByRole('heading', { name: 'Also changed on both sides' });
		expect(
			screen.getByText('Every entity changed on both sides is listed as a conflict above.')
		).toBeDefined();
	});

	it('distinguishes an overlap with no conflict from a conflict', async () => {
		compareBranch.mockResolvedValue(
			comparison({ conflicts: [], overlapping_stream_ids: [PERSON_ID, OTHER_ID] })
		);

		render(Page);

		await screen.findByText(
			"No conflicts. This branch's changes are compatible with the mainline."
		);
		expect(screen.getByText(PERSON_ID)).toBeDefined();
		expect(screen.getByText(OTHER_ID)).toBeDefined();
	});

	it('discloses a truncated diff', async () => {
		compareBranch.mockResolvedValue(comparison({ has_more: true }));

		render(Page);

		expect(await screen.findByText(/hit the read cap/)).toBeDefined();
	});

	it('says nothing about truncation when the diff is complete', async () => {
		const { container } = render(Page);

		await screen.findByRole('heading', { name: 'Maternal Smith line' });
		expect(container.querySelector('.truncation')).toBeNull();
	});

	it('offers no merge action - merging is a separate feature', async () => {
		render(Page);

		await screen.findByRole('heading', { name: 'Maternal Smith line' });
		expect(screen.queryByRole('button', { name: /merge/i })).toBeNull();
	});

	it('ignores a slow response for a branch the route has already left', async () => {
		const SECOND_ID = '22222222-2222-2222-2222-222222222222';
		const secondBranch: Branch = { ...branch, id: SECOND_ID, name: 'Paternal Jones line' };

		let resolveFirst!: (value: BranchComparisonResult) => void;
		compareBranch
			.mockImplementationOnce(
				() => new Promise<BranchComparisonResult>((resolve) => (resolveFirst = resolve))
			)
			.mockImplementationOnce(async () =>
				comparison({ branch: secondBranch, conflicts: [], overlapping_stream_ids: [] })
			);

		render(Page);
		navigateTo(SECOND_ID);
		await screen.findByRole('heading', { name: 'Paternal Jones line' });

		// The first branch's response lands last. It must not repaint the page
		// with one branch's changes under the other's identity.
		resolveFirst(comparison());
		await waitFor(() => {
			expect(compareBranch).toHaveBeenCalledTimes(2);
		});
		await tick();

		expect(screen.getByRole('heading', { name: 'Paternal Jones line' })).toBeDefined();
		expect(screen.queryByRole('heading', { name: 'Maternal Smith line' })).toBeNull();
		expect(
			screen.getByText("No conflicts. This branch's changes are compatible with the mainline.")
		).toBeDefined();
	});

	// A -> B -> A: two requests for the SAME id, so a routed-id check cannot
	// separate them. Only request ordering can, which is why the loader carries
	// a monotonic token rather than comparing ids.
	it('ignores an older response for the branch it has navigated back to', async () => {
		const SECOND_ID = '22222222-2222-2222-2222-222222222222';
		const secondBranch: Branch = { ...branch, id: SECOND_ID, name: 'Paternal Jones line' };
		const staleName = 'Maternal Smith line (stale)';

		let resolveFirstA!: (value: BranchComparisonResult) => void;
		compareBranch
			// First visit to A: slow, resolves last.
			.mockImplementationOnce(
				() => new Promise<BranchComparisonResult>((resolve) => (resolveFirstA = resolve))
			)
			// Visit to B.
			.mockImplementationOnce(async () =>
				comparison({ branch: secondBranch, conflicts: [], overlapping_stream_ids: [] })
			)
			// Back to A: the response the page must keep.
			.mockImplementationOnce(async () => comparison());

		render(Page);
		navigateTo(SECOND_ID);
		await screen.findByRole('heading', { name: 'Paternal Jones line' });

		navigateTo(BRANCH_ID);
		await screen.findByRole('heading', { name: 'Maternal Smith line' });

		// The first A request finally lands, carrying older data for the same id.
		resolveFirstA(comparison({ branch: { ...branch, name: staleName } }));
		await waitFor(() => {
			expect(compareBranch).toHaveBeenCalledTimes(3);
		});
		await tick();

		expect(screen.queryByRole('heading', { name: staleName })).toBeNull();
		expect(screen.getByRole('heading', { name: 'Maternal Smith line' })).toBeDefined();
	});

	it('handles a missing branch', async () => {
		compareBranch.mockRejectedValue({ status: 404, code: 'not_found', message: 'not found' });

		render(Page);

		expect(await screen.findByText('Branch not found')).toBeDefined();
	});
});
