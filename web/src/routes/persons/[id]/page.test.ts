import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { PersonDetail } from '$lib/api/client';

const PERSON_ID = '11111111-1111-1111-1111-111111111111';
const BRANCH_ID = '44444444-4444-4444-4444-444444444444';

const { branchState, getPerson, setPersonBrickWall, resolvePersonBrickWall } = vi.hoisted(() => ({
	// The real store exposes a read-only view, so the active branch is injected.
	branchState: { id: null as string | null },
	getPerson: vi.fn(),
	setPersonBrickWall: vi.fn(),
	resolvePersonBrickWall: vi.fn()
}));

/**
 * This page mounts several self-fetching panels (media, citations, evidence,
 * names, history). None of them is under test, so every method other than the
 * three that matter answers with a shape that satisfies the list-ish responses
 * they all expect.
 */
const EMPTY_RESPONSE = {
	items: [],
	total: 0,
	citations: [],
	analyses: [],
	conflicts: [],
	logs: [],
	summaries: [],
	sources: [],
	names: []
};

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	const stubs: Record<string, unknown> = {
		getPerson,
		setPersonBrickWall,
		resolvePersonBrickWall,
		// These two answer with a bare array rather than a wrapper object.
		getConflictsBySubject: vi.fn(async () => []),
		getResearchLogsBySubject: vi.fn(async () => [])
	};
	const api = new Proxy(stubs, {
		get(target, prop: string) {
			if (!(prop in target)) {
				target[prop] = prop.endsWith('Url')
					? () => ''
					: vi.fn(async () => ({ ...EMPTY_RESPONSE }));
			}
			return target[prop];
		}
	});
	return { ...actual, api };
});

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState
}));

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (callback: (value: { params: { id: string } }) => void) => {
			callback({ params: { id: PERSON_ID } });
			return () => {};
		}
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

function person(overrides: Partial<PersonDetail> = {}): PersonDetail {
	return {
		id: PERSON_ID,
		given_name: 'Ada',
		surname: 'Lovelace',
		version: 3,
		...overrides
	};
}

describe('Person detail brick-wall controls', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		getPerson.mockResolvedValue(person());
	});

	it('offers the brick-wall control on the mainline', async () => {
		render(Page);
		expect(await screen.findByRole('button', { name: 'Mark as Brick Wall' })).toBeDefined();
	});

	// `PUT`/`DELETE /persons/{id}/brick-wall` declare no `branch` parameter, so
	// these writes would land on the mainline while the banner promises the
	// branch. The controls are withdrawn rather than silently lying.
	it('withdraws the brick-wall control while a research branch is active', async () => {
		branchState.id = BRANCH_ID;

		render(Page);

		expect(await screen.findByText(/recorded on the mainline only/)).toBeDefined();
		expect(screen.queryByRole('button', { name: 'Mark as Brick Wall' })).toBeNull();
		expect(setPersonBrickWall).not.toHaveBeenCalled();
	});

	it('withdraws the resolve control on a branch, keeping the brick wall visible', async () => {
		branchState.id = BRANCH_ID;
		getPerson.mockResolvedValue(
			person({ brick_wall_note: 'No baptism record found', brick_wall_since: '2026-01-15T10:30:00Z' })
		);

		render(Page);

		expect(await screen.findByText('No baptism record found')).toBeDefined();
		expect(screen.queryByRole('button', { name: /Resolve Brick Wall/ })).toBeNull();
		expect(screen.getByText(/recorded on the mainline only/)).toBeDefined();
		expect(resolvePersonBrickWall).not.toHaveBeenCalled();
	});
});
