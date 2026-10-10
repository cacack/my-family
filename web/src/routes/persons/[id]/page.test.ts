import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import { buttonVariants } from '$lib/components/ui/button';
import { cn } from '$lib/utils.js';
import type * as apiModule from '$lib/api/client';
import type { PersonDetail } from '$lib/api/client';
import { goto } from '$app/navigation';

const PERSON_ID = '11111111-1111-1111-1111-111111111111';
const BRANCH_ID = '44444444-4444-4444-4444-444444444444';

const {
	branchState,
	getPerson,
	setPersonBrickWall,
	resolvePersonBrickWall,
	getPersonHistory,
	listPersonMedia,
	getPersonRestorePoints,
	updatePerson
} = vi.hoisted(() => ({
	// The real store exposes a read-only view, so the active branch is injected.
	branchState: { id: null as string | null },
	getPerson: vi.fn(),
	setPersonBrickWall: vi.fn(),
	resolvePersonBrickWall: vi.fn(),
	getPersonHistory: vi.fn(async () => ({ items: [], total: 0 })),
	listPersonMedia: vi.fn(async () => ({ items: [], total: 0 })),
	getPersonRestorePoints: vi.fn(async () => ({ items: [], total: 0, has_more: false })),
	updatePerson: vi.fn()
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
		getPersonHistory,
		listPersonMedia,
		getPersonRestorePoints,
		updatePerson,
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
		const button = await screen.findByRole('button', { name: 'Mark as Brick Wall' });
		// A bordered (outline) button, not a ghost one that reads as plain text (#897).
		expect(button.getAttribute('data-slot')).toBe('button');
		expect(button.className).toBe(cn(buttonVariants({ variant: 'outline' })));
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

describe('Person detail badge counts (#823)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		getPerson.mockResolvedValue(person());
		getPersonHistory.mockResolvedValue({ ...EMPTY_RESPONSE, total: 4 });
		listPersonMedia.mockResolvedValue({ ...EMPTY_RESPONSE, total: 2 });
		vi.spyOn(console, 'warn').mockImplementation(() => {});
	});

	it('shows the history count once the person has loaded', async () => {
		render(Page);
		const heading = await screen.findByRole('heading', { name: /History/ });
		await waitFor(() => expect(heading.textContent).toContain('4'));
	});

	// A person created on a branch used to fail here: the history lookup ran in
	// the same try as the person, so its failure blanked the whole page.
	it('still renders the person when the history count fails', async () => {
		branchState.id = BRANCH_ID;
		getPersonHistory.mockRejectedValue({ message: 'Person not found' });

		render(Page);

		expect(await screen.findByRole('heading', { name: 'Ada Lovelace' })).toBeDefined();
		expect(screen.queryByText('Person not found')).toBeNull();
		const heading = screen.getByRole('heading', { name: /History/ });
		await waitFor(() => expect(getPersonHistory).toHaveBeenCalled());
		expect(heading.textContent?.trim()).toBe('History');
	});

	it('still renders the person when the media count fails', async () => {
		listPersonMedia.mockRejectedValue({ message: 'boom' });

		render(Page);

		expect(await screen.findByRole('heading', { name: 'Ada Lovelace' })).toBeDefined();
		// The gallery reports its own failure; the page itself stays usable.
		await waitFor(() => expect(listPersonMedia).toHaveBeenCalled());
		expect(screen.getByRole('button', { name: 'Edit' })).toBeDefined();
	});
});

describe('Person detail rollback on a branch (#824)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		getPerson.mockResolvedValue(person());
		getPersonHistory.mockResolvedValue({ ...EMPTY_RESPONSE });
		listPersonMedia.mockResolvedValue({ ...EMPTY_RESPONSE });
		getPersonRestorePoints.mockResolvedValue({ items: [], total: 0, has_more: false });
	});

	async function openHistory() {
		await fireEvent.click(await screen.findByRole('button', { name: /History/ }));
	}

	it('offers the Restore tab on the mainline', async () => {
		render(Page);
		await openHistory();
		expect(screen.getByRole('button', { name: 'Restore' })).toBeDefined();
		expect(screen.queryByText(/work on the mainline only/)).toBeNull();
	});

	it('withdraws Restore and rollback on a branch, keeping the change log', async () => {
		branchState.id = BRANCH_ID;

		render(Page);
		await openHistory();

		expect(screen.getByText(/Restore points and rollback work on the mainline only/)).toBeDefined();
		expect(screen.queryByRole('button', { name: 'Restore' })).toBeNull();
		expect(screen.queryByRole('button', { name: 'Change Log' })).toBeNull();
		await waitFor(() => expect(getPersonHistory).toHaveBeenCalledWith(PERSON_ID, { limit: 20, offset: 0 }));
		expect(getPersonRestorePoints).not.toHaveBeenCalled();
	});
});

describe('Person detail family shortcuts (#826)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
	});

	it('offers Add family and Add parents when the person has neither', async () => {
		getPerson.mockResolvedValue(person());
		render(Page);

		const addFamily = await screen.findByRole('link', { name: 'Add family' });
		expect(addFamily.getAttribute('href')).toBe(`/families/add?partner1=${PERSON_ID}`);
		const addParents = screen.getByRole('link', { name: 'Add parents' });
		expect(addParents.getAttribute('href')).toBe(`/families/add?child=${PERSON_ID}`);
		expect(screen.getByText('No parents recorded.')).toBeDefined();
	});

	it('keeps Add family but withdraws Add parents once parents are recorded', async () => {
		getPerson.mockResolvedValue(
			person({
				family_as_child: { id: 'fam-parents', partner1_name: 'John Smith', partner2_name: 'Ann Jones' },
				families_as_partner: [{ id: 'fam-own', partner1_name: 'Ada Lovelace' }]
			})
		);
		render(Page);

		expect(await screen.findByRole('link', { name: 'Add family' })).toBeDefined();
		expect(screen.queryByRole('link', { name: 'Add parents' })).toBeNull();
		expect(screen.getByText(/John Smith/).closest('a')?.getAttribute('href')).toBe('/families/fam-parents');
	});

	it('offers the shortcuts on a branch too, since family writes follow the branch', async () => {
		branchState.id = BRANCH_ID;
		getPerson.mockResolvedValue(person());
		render(Page);

		expect(await screen.findByRole('link', { name: 'Add family' })).toBeDefined();
		expect(screen.getByRole('link', { name: 'Add parents' })).toBeDefined();
	});
});

describe('Person detail edit form', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		updatePerson.mockResolvedValue({ id: PERSON_ID, version: 4 });
	});

	async function openEdit() {
		render(Page);
		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
	}

	async function save() {
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
		await waitFor(() => expect(updatePerson).toHaveBeenCalled());
		return updatePerson.mock.calls[0][1];
	}

	// An omitted field is left unchanged by the API, so a cleared field must be
	// sent as an empty string or it can never be removed.
	it('sends a cleared field as an empty string', async () => {
		getPerson.mockResolvedValue(person({ birth_place: 'London', notes: 'A note' }));
		await openEdit();

		await fireEvent.input(screen.getByLabelText('Birth Place'), { target: { value: '' } });
		await fireEvent.input(screen.getByLabelText('Notes'), { target: { value: '' } });
		const body = await save();

		expect(body).toMatchObject({ birth_place: '', notes: '', given_name: 'Ada', surname: 'Lovelace' });
	});

	it('sends gender unknown when a known gender is changed to Unknown', async () => {
		getPerson.mockResolvedValue(person({ gender: 'female' }));
		await openEdit();

		await fireEvent.change(screen.getByLabelText('Gender'), { target: { value: 'unknown' } });
		const body = await save();

		expect(body.gender).toBe('unknown');
	});

	it('moves focus into the form on Edit and back to Edit on Cancel', async () => {
		getPerson.mockResolvedValue(person());
		await openEdit();
		await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText('Given Name')));

		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Edit' })));
	});

	it('returns focus to Edit after a save', async () => {
		getPerson.mockResolvedValue(person());
		await openEdit();
		await save();
		await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Edit' })));
	});

	it('sends no gender when it is untouched', async () => {
		getPerson.mockResolvedValue(person());
		await openEdit();

		const body = await save();

		expect(body.gender).toBeUndefined();
	});
});

describe('Person detail delete', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		getPerson.mockResolvedValue(person());
	});

	it('confirms a delete on the people list it returns to', async () => {
		vi.spyOn(window, 'confirm').mockReturnValue(true);
		render(Page);
		await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));

		await waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/persons', { state: { notice: 'Ada Lovelace was deleted.' } })
		);
	});
});
