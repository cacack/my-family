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
const { compareSnapshots, routeState } = vi.hoisted(() => ({
	compareSnapshots: vi.fn(),
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
			compareSnapshots: (a: string, b: string) => compareSnapshots(a, b)
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

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: { id: null, branch: null, revalidating: false, notice: null }
}));

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

// A sub-record: no page of its own, so it links to the person that presents it.
const LIFE_EVENT_ID = '88888888-8888-8888-8888-888888888888';
const subRecord: BranchChangeEntry = {
	id: 'e4',
	timestamp: '2026-01-20T09:00:01Z',
	entity_type: 'life_event',
	entity_id: LIFE_EVENT_ID,
	entity_name: 'Birth, 1850, Ohio',
	action: 'updated',
	parent_entity_type: 'person',
	parent_entity_id: PERSON_ID,
	changes: { place: { old_value: 'Kentucky', new_value: 'Ohio' } }
};

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

		expect(await screen.findByText('4 changes: 1 created, 2 updated, 1 deleted.')).toBeDefined();

		const list = screen.getByRole('list', { name: 'Changes, oldest first' });
		const items = within(list).getAllByRole('listitem');
		expect(items).toHaveLength(4);

		const person = within(items[0]).getByRole('link', { name: 'Mary Smith' });
		expect(person.getAttribute('href')).toBe(`/persons/${PERSON_ID}`);
		expect(within(items[0]).getByText('Ohio')).toBeDefined();
		expect(within(items[0]).getByText('Franklin County, Ohio')).toBeDefined();

		expect(
			within(items[2]).getByRole('link', { name: 'John Smith & Mary Jones' }).getAttribute('href')
		).toBe(`/families/${FAMILY_ID}`);

		// A deleted entity has no page to link to.
		expect(within(items[3]).queryByRole('link')).toBeNull();
		expect(within(items[3]).getByText('Duplicate John')).toBeDefined();
	});

	it('lists sub-records by name, with their before/after values, linked to their owner', async () => {
		render(Page);

		const list = await screen.findByRole('list', { name: 'Changes, oldest first' });
		const item = within(list).getAllByRole('listitem')[1];
		expect(within(item).getByText('Life event')).toBeDefined();
		const link = within(item).getByRole('link', { name: 'Birth, 1850, Ohio' });
		expect(link.getAttribute('href')).toBe(`/persons/${PERSON_ID}`);
		expect(within(item).getByText('Kentucky')).toBeDefined();
		expect(screen.queryByText(LIFE_EVENT_ID)).toBeNull();
		expect(screen.queryByText(/unknown/i)).toBeNull();
	});

	it('filters the list by entity type', async () => {
		render(Page);
		await screen.findByText('Mary Smith');

		await fireEvent.change(screen.getByLabelText('Show'), { target: { value: 'family' } });

		const list = screen.getByRole('list', { name: 'Changes, oldest first' });
		expect(within(list).getAllByRole('listitem')).toHaveLength(1);
		expect(within(list).getByText('John Smith & Mary Jones')).toBeDefined();

		await fireEvent.change(screen.getByLabelText('Show'), { target: { value: 'life_event' } });
		expect(within(list).getAllByRole('listitem')).toHaveLength(1);
		expect(within(list).getByText('Birth, 1850, Ohio')).toBeDefined();
	});

	it('warns when the range was truncated', async () => {
		compareSnapshots.mockResolvedValue(comparison({ has_more: true }));

		render(Page);

		expect(await screen.findByText(/more changes than can be compared at once/)).toBeDefined();
	});

	it('says so when nothing changed', async () => {
		compareSnapshots.mockResolvedValue(comparison({ changes: [] }));

		render(Page);

		expect(await screen.findByText('No changes between these snapshots.')).toBeDefined();
		expect(screen.queryByRole('list', { name: 'Changes, oldest first' })).toBeNull();
	});

	it('announces the loaded comparison to assistive tech', async () => {
		render(Page);

		await waitFor(() => {
			expect(screen.getByTestId('announcer').textContent).toContain(
				'Comparison loaded. 4 changes: 1 created, 2 updated, 1 deleted.'
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
});
