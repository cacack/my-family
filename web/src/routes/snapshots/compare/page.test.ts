import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent, within } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { BranchChangeEntry, Snapshot, SnapshotComparisonResult } from '$lib/api/client';

const OLDER_ID = '11111111-1111-1111-1111-111111111111';
const NEWER_ID = '22222222-2222-2222-2222-222222222222';
const PERSON_ID = '99999999-9999-9999-9999-999999999999';
const FAMILY_ID = '88888888-8888-8888-8888-888888888888';

type RouteValue = { url: URL };

// Hoisted so the module mocks below (which vitest lifts above the imports) can
// close over them.
const { compareSnapshots, compareSnapshotToCurrent, routeState, branchState } = vi.hoisted(() => ({
	compareSnapshots: vi.fn(),
	compareSnapshotToCurrent: vi.fn(),
	branchState: {
		activeBranch: {
			id: null as string | null,
			branch: null as { name: string } | null,
			revalidating: false,
			notice: null
		}
	},
	// A soft navigation between two comparisons reuses the component, so the
	// route store has to be drivable rather than fixed.
	routeState: {
		current: { url: new URL('http://localhost/snapshots/compare') } as RouteValue,
		subscribers: new Set<(value: RouteValue) => void>()
	}
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			compareSnapshots: (a: string, b: string) => compareSnapshots(a, b),
			compareSnapshotToCurrent: (id: string) => compareSnapshotToCurrent(id)
		}
	};
});

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (callback: (value: RouteValue) => void) => {
			routeState.subscribers.add(callback);
			callback(routeState.current);
			return () => routeState.subscribers.delete(callback);
		}
	}
}));

vi.mock('$lib/stores/activeBranch.svelte', () => branchState);

function navigateTo(query: string) {
	// A fresh object: Svelte's store bridge dedupes on identity.
	routeState.current = { url: new URL(`http://localhost/snapshots/compare${query}`) };
	for (const subscriber of routeState.subscribers) {
		subscriber(routeState.current);
	}
}

const older: Snapshot = {
	id: OLDER_ID,
	name: 'Pre-DNA results',
	description: 'Before the kit came back',
	position: 42,
	created_at: '2026-01-15T10:30:00Z'
};

const newer: Snapshot = {
	id: NEWER_ID,
	name: 'Post-DNA results',
	position: 80,
	created_at: '2026-02-01T12:00:00Z'
};

const personUpdate: BranchChangeEntry = {
	id: 'e1',
	timestamp: '2026-01-20T09:00:00Z',
	entity_type: 'person',
	entity_id: PERSON_ID,
	entity_name: 'Mary Smith',
	action: 'updated',
	changes: { birth_place: { old_value: 'Ohio', new_value: 'Franklin County, Ohio' } }
};

const familyCreate: BranchChangeEntry = {
	id: 'e2',
	timestamp: '2026-01-21T09:00:00Z',
	entity_type: 'family',
	entity_id: FAMILY_ID,
	entity_name: 'John Smith & Mary Jones',
	action: 'created'
};

const personDelete: BranchChangeEntry = {
	id: 'e3',
	timestamp: '2026-01-22T09:00:00Z',
	entity_type: 'person',
	entity_id: '77777777-7777-7777-7777-777777777777',
	entity_name: 'Duplicate John',
	action: 'deleted'
};

// The server labels sub-record events it does not itemize as `unknown`, which
// is outside the generated enum - hence the cast.
const subRecord = {
	id: 'e4',
	timestamp: '2026-01-20T09:00:01Z',
	entity_type: 'unknown',
	entity_id: PERSON_ID,
	entity_name: PERSON_ID,
	action: 'unknown'
} as unknown as BranchChangeEntry;

function comparison(overrides: Partial<SnapshotComparisonResult> = {}): SnapshotComparisonResult {
	const changes = overrides.changes ?? [personUpdate, subRecord, familyCreate, personDelete];
	return {
		snapshot1: older,
		snapshot2: newer,
		changes,
		total_count: changes.length,
		has_more: false,
		older_first: true,
		...overrides
	};
}

describe('Snapshot comparison page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.activeBranch.id = null;
		branchState.activeBranch.branch = null;
		navigateTo(`?from=${OLDER_ID}&to=${NEWER_ID}`);
		compareSnapshots.mockResolvedValue(comparison());
	});

	it('requests the pair from the query string', async () => {
		render(Page);

		await screen.findByText('Pre-DNA results');
		expect(compareSnapshots).toHaveBeenCalledWith(OLDER_ID, NEWER_ID);
	});

	it('renders both snapshots, oldest as the starting point', async () => {
		render(Page);

		await screen.findByText('Pre-DNA results');
		const labels = screen.getAllByText(/^(From|To) \((older|newer)\)$/).map((el) => el.textContent);
		expect(labels).toEqual(['From (older)', 'To (newer)']);
		expect(screen.getByText('Before the kit came back')).toBeDefined();
		expect(screen.queryByText(/Listed oldest first/)).toBeNull();
	});

	it('presents older -> newer even when the ids arrive newest first', async () => {
		navigateTo(`?from=${NEWER_ID}&to=${OLDER_ID}`);
		compareSnapshots.mockResolvedValue(
			comparison({ snapshot1: newer, snapshot2: older, older_first: false })
		);

		const { container } = render(Page);

		await screen.findByText(/Listed oldest first/);
		const names = [...container.querySelectorAll('.endpoint-name')].map((el) => el.textContent);
		expect(names).toEqual(['Pre-DNA results', 'Post-DNA results']);
		expect(screen.getByRole('link', { name: 'Link to this order' }).getAttribute('href')).toBe(
			`/snapshots/compare?from=${OLDER_ID}&to=${NEWER_ID}`
		);
	});

	it('summarizes and itemizes the changes, with a field-level diff for updates', async () => {
		render(Page);

		expect(await screen.findByText('3 changes: 1 created, 1 updated, 1 deleted.')).toBeDefined();

		const list = screen.getByRole('list', { name: 'Changes, oldest first' });
		const items = within(list).getAllByRole('listitem');
		expect(items).toHaveLength(3);

		const person = within(items[0]).getByRole('link', { name: 'Mary Smith' });
		expect(person.getAttribute('href')).toBe(`/persons/${PERSON_ID}`);
		expect(within(items[0]).getByText('Ohio')).toBeDefined();
		expect(within(items[0]).getByText('Franklin County, Ohio')).toBeDefined();

		expect(
			within(items[1]).getByRole('link', { name: 'John Smith & Mary Jones' }).getAttribute('href')
		).toBe(`/families/${FAMILY_ID}`);

		// A deleted entity has no page to link to.
		expect(within(items[2]).queryByRole('link')).toBeNull();
		expect(within(items[2]).getByText('Duplicate John')).toBeDefined();
	});

	it('counts sub-record changes instead of listing them by raw id', async () => {
		render(Page);

		await screen.findByText(/1 related record change \(/);
		expect(screen.queryByText(PERSON_ID)).toBeNull();
	});

	it('filters the list by entity type', async () => {
		render(Page);
		await screen.findByText('Mary Smith');

		await fireEvent.change(screen.getByLabelText('Show'), { target: { value: 'family' } });

		const list = screen.getByRole('list', { name: 'Changes, oldest first' });
		expect(within(list).getAllByRole('listitem')).toHaveLength(1);
		expect(within(list).getByText('John Smith & Mary Jones')).toBeDefined();
	});

	it('warns when the range was truncated', async () => {
		compareSnapshots.mockResolvedValue(comparison({ has_more: true }));

		render(Page);

		expect(await screen.findByText(/more changes than can be compared at once/)).toBeDefined();
	});

	it('says so when nothing changed', async () => {
		compareSnapshots.mockResolvedValue(comparison({ changes: [] }));

		render(Page);

		expect(
			await screen.findByText(
				'No changes to people, families, sources or citations between these snapshots.'
			)
		).toBeDefined();
		expect(screen.queryByRole('list', { name: 'Changes, oldest first' })).toBeNull();
	});

	it('announces the loaded comparison to assistive tech', async () => {
		render(Page);

		await waitFor(() => {
			expect(screen.getByTestId('announcer').textContent).toContain(
				'Comparison loaded. 3 changes: 1 created, 1 updated, 1 deleted.'
			);
		});
	});

	it('does not call the server without two snapshot ids', async () => {
		navigateTo(`?from=${OLDER_ID}`);

		render(Page);

		expect(await screen.findByText('Choose two snapshots to compare.')).toBeDefined();
		expect(compareSnapshots).not.toHaveBeenCalled();
	});

	it('does not compare a snapshot with itself', async () => {
		navigateTo(`?from=${OLDER_ID}&to=${OLDER_ID}`);

		render(Page);

		expect(await screen.findByText(/Choose two different snapshots/)).toBeDefined();
		expect(compareSnapshots).not.toHaveBeenCalled();
	});

	it('explains a deleted snapshot on 404', async () => {
		compareSnapshots.mockRejectedValue({ status: 404, code: 'not_found', message: 'Snapshot not found' });

		render(Page);

		expect(await screen.findByText('Snapshot not found')).toBeDefined();
		expect(screen.getByText(/may have been deleted/)).toBeDefined();
	});

	it('surfaces other failures', async () => {
		compareSnapshots.mockRejectedValue({ status: 500, code: 'internal', message: 'boom' });

		render(Page);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('boom');
	});

	it('ignores a slow response for a pair the user has navigated away from', async () => {
		let resolveFirst: (value: SnapshotComparisonResult) => void = () => {};
		compareSnapshots.mockImplementationOnce(
			() => new Promise<SnapshotComparisonResult>((resolve) => (resolveFirst = resolve))
		);
		const third: Snapshot = { ...newer, id: '33333333-3333-3333-3333-333333333333', name: 'Third' };
		compareSnapshots.mockResolvedValueOnce(comparison({ snapshot2: third }));

		render(Page);
		navigateTo(`?from=${OLDER_ID}&to=${third.id}`);
		await screen.findByText('Third');

		resolveFirst(comparison());
		await new Promise((resolve) => setTimeout(resolve, 0));

		expect(screen.getByText('Third')).toBeDefined();
		expect(screen.queryByText('Post-DNA results')).toBeNull();
	});

	describe('compare to now', () => {
		const branchEdit = { ...personUpdate, id: 'e5', origin: 'branch' } as BranchChangeEntry;
		const inherited = { ...familyCreate, id: 'e6', origin: 'main' } as BranchChangeEntry;

		beforeEach(() => {
			navigateTo(`?from=${OLDER_ID}&to=current`);
			compareSnapshotToCurrent.mockResolvedValue({
				snapshot: older,
				head_position: 120,
				changes: [personUpdate],
				total_count: 1,
				has_more: false
			});
		});

		it('asks for the changes since the snapshot, not for a pair', async () => {
			render(Page);

			await screen.findByText('Pre-DNA results');
			expect(compareSnapshotToCurrent).toHaveBeenCalledWith(OLDER_ID);
			expect(compareSnapshots).not.toHaveBeenCalled();
			expect(screen.getByText('Now')).toBeDefined();
			expect(screen.getByText('The current state of your research')).toBeDefined();
			expect(screen.getByText('1 change: 0 created, 1 updated, 0 deleted.')).toBeDefined();
			expect(screen.getByText('Franklin County, Ohio')).toBeDefined();
		});

		it('says so when nothing changed since the snapshot', async () => {
			compareSnapshotToCurrent.mockResolvedValue({
				snapshot: older,
				head_position: 42,
				changes: [],
				total_count: 0,
				has_more: false
			});

			render(Page);

			expect(
				await screen.findByText(
					'No changes to people, families, sources or citations since this snapshot.'
				)
			).toBeDefined();
		});

		it("labels a branch's own changes and the mainline changes it inherits", async () => {
			branchState.activeBranch.id = '44444444-4444-4444-4444-444444444444';
			branchState.activeBranch.branch = { name: 'Maternal line' };
			compareSnapshotToCurrent.mockResolvedValue({
				snapshot: older,
				head_position: 120,
				changes: [branchEdit, inherited],
				total_count: 2,
				has_more: false
			});

			render(Page);

			const list = await screen.findByRole('list', { name: 'Changes, oldest first' });
			const items = within(list).getAllByRole('listitem');
			expect(within(items[0]).getByText('This branch')).toBeDefined();
			expect(within(items[1]).getByText('Mainline')).toBeDefined();
			expect(screen.queryByText(/always shows mainline data/)).toBeNull();
			expect(screen.getByRole('note').textContent).toMatch(/Maternal line/);
		});

		it('explains snapshots from another branch', async () => {
			compareSnapshotToCurrent.mockRejectedValue({
				status: 409,
				code: 'snapshot_branch_mismatch',
				message: 'Snapshots can only be compared within the branch they were taken on'
			});

			render(Page);

			expect(await screen.findByText('Snapshots from another branch')).toBeDefined();
			expect(screen.getByRole('link', { name: 'choose snapshots from this one' })).toBeDefined();
		});
	});
});
