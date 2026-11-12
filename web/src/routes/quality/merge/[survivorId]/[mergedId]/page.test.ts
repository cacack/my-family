import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '#lib/api/client.js';

// Configurable page params
let mockPageParams: Record<string, string> = {
	survivorId: 'p-survivor',
	mergedId: 'p-merged'
};

// Mock the API module
vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getPerson: vi.fn(),
			mergePersons: vi.fn(),
			dismissDuplicate: vi.fn()
		}
	};
});

// Mock the page store with configurable params
vi.mock('$app/state', () => ({
	get page() {
		return { params: mockPageParams };
	}
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

// The real store exposes a read-only view, so the active branch is injected here.
const { branchState } = vi.hoisted(() => ({
	branchState: { id: null as string | null, branch: null as { name: string } | null }
}));

vi.mock('#lib/stores/activeBranch.svelte.js', () => ({
	activeBranch: branchState
}));

const survivorPerson: apiModule.PersonDetail = {
	id: 'p-survivor',
	given_name: 'Jane',
	surname: 'Smith',
	gender: 'female',
	// Survivor has no birth_date — initial resolution should pick 'merged'
	birth_date: undefined,
	birth_place: 'Cleveland',
	death_date: undefined,
	death_place: undefined,
	notes: 'Survivor notes',
	research_status: 'probable',
	version: 7
};

const mergedPerson: apiModule.PersonDetail = {
	id: 'p-merged',
	given_name: 'Jane',
	surname: 'Smith',
	gender: 'female',
	birth_date: { raw: '12 MAR 1850' },
	birth_place: 'Cleveland',
	death_date: { raw: '4 JUL 1920' },
	death_place: 'Chicago',
	notes: '',
	research_status: 'possible',
	version: 3
};

describe('Merge Picker Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		branchState.branch = null;
		mockPageParams = { survivorId: 'p-survivor', mergedId: 'p-merged' };
		vi.mocked(apiModule.api.getPerson).mockImplementation(async (id: string) => {
			if (id === 'p-survivor') return survivorPerson;
			if (id === 'p-merged') return mergedPerson;
			throw new Error(`Unexpected person id ${id}`);
		});
		vi.mocked(apiModule.api.mergePersons).mockResolvedValue({
			person: { ...mergedPerson, id: 'p-survivor' },
			merge_summary: {
				merged_person_name: 'Jane Smith',
				fields_updated: ['birth_date'],
				families_updated: 0,
				citations_transferred: 1,
				names_transferred: 0,
				events_transferred: 0,
				media_transferred: 0
			}
		});
	});

	it('renders both persons side-by-side after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Survivor (keeps existing ID)')).toBeDefined();
			expect(screen.getByText('Will be merged & deleted')).toBeDefined();
		});
	});

	it('shows the shared header and an empty field as — (#1108)', async () => {
		const { container } = render(Page);
		await screen.findByText('Survivor (keeps existing ID)');
		expect(screen.getByRole('heading', { level: 1, name: 'Merge Persons' })).toBeDefined();
		expect(screen.getByRole('link', { name: /Quality/ }).getAttribute('href')).toBe('/quality');
		expect(container.textContent).not.toContain('(empty)');
		const survivorDeathPlace = [...container.querySelectorAll('dt')].find(
			(dt) => dt.textContent === 'Death Place'
		);
		expect(survivorDeathPlace?.nextElementSibling?.textContent?.trim()).toBe('—');
	});

	it('rejects self-merge before fetching, showing only the error (#1096)', async () => {
		mockPageParams = { survivorId: 'p-same', mergedId: 'p-same' };
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Cannot merge a person with themselves.')).toBeDefined();
		});
		expect(apiModule.api.getPerson).not.toHaveBeenCalled();
		// No retry can make the pair two people, and there is nothing to suggest.
		expect(screen.queryByText('Suggested merge')).toBeNull();
		expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
	});

	it('defaults birth_date to merged when survivor is empty and merged has a value', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Merge persons')).toBeDefined();
		});

		// Click submit and inspect the payload — easiest way to assert the initial
		// resolution state without poking at internal state.
		await fireEvent.click(screen.getByText('Merge persons'));

		await waitFor(() => {
			expect(apiModule.api.mergePersons).toHaveBeenCalledTimes(1);
		});
		const req = vi.mocked(apiModule.api.mergePersons).mock.calls[0][0];
		expect(req.field_resolution?.birth_date).toBe('merged');
	});

	it('builds submit payload with both versions and the resolution map', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Merge persons')).toBeDefined();
		});
		await fireEvent.click(screen.getByText('Merge persons'));

		await waitFor(() => {
			expect(apiModule.api.mergePersons).toHaveBeenCalledTimes(1);
		});
		const req = vi.mocked(apiModule.api.mergePersons).mock.calls[0][0];
		expect(req.survivor_id).toBe('p-survivor');
		expect(req.merged_id).toBe('p-merged');
		expect(req.survivor_version).toBe(7);
		expect(req.merged_version).toBe(3);
		// Resolution should include all 9 mergeable fields
		const keys = Object.keys(req.field_resolution ?? {}).sort();
		expect(keys).toEqual(
			[
				'birth_date',
				'birth_place',
				'death_date',
				'death_place',
				'gender',
				'given_name',
				'notes',
				'research_status',
				'surname'
			].sort()
		);
		// Survivor has non-empty notes → defaults to 'survivor'
		expect(req.field_resolution?.notes).toBe('survivor');
	});

	it('surfaces API error message on merge failure', async () => {
		const apiErr = {
			code: 'CONFLICT_RETRY_FAILED',
			message: 'This record was modified by another operation. Please try again.',
			status: 409
		};
		vi.mocked(apiModule.api.mergePersons).mockRejectedValue(apiErr);

		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Merge persons')).toBeDefined();
		});
		await fireEvent.click(screen.getByText('Merge persons'));

		await waitFor(() => {
			expect(
				screen.getByText('This record was modified by another operation. Please try again.')
			).toBeDefined();
		});
	});

	// Both the reads and the merge honor the active branch (#834): the merge is
	// offered on a branch, and the page says it lands there only.
	it('merges on a research branch and says the mainline is unaffected', async () => {
		branchState.id = '44444444-4444-4444-4444-444444444444';
		branchState.branch = { name: 'Same-person hypothesis' };

		render(Page);

		const button = await screen.findByRole('button', { name: 'Merge persons' });
		expect((button as HTMLButtonElement).disabled).toBe(false);
		expect(screen.getByText('Merging on Same-person hypothesis')).toBeDefined();
		expect(screen.getByText(/The mainline keeps both persons/)).toBeDefined();

		await fireEvent.click(button);
		await waitFor(() => {
			expect(apiModule.api.mergePersons).toHaveBeenCalledTimes(1);
		});
		await waitFor(() => {
			expect(screen.getByText('Merged successfully')).toBeDefined();
		});
		expect(screen.queryByText('Merging on Same-person hypothesis')).toBeNull();
	});

	it('names the branch generically until its record has loaded', async () => {
		branchState.id = '44444444-4444-4444-4444-444444444444';

		render(Page);

		await screen.findByRole('button', { name: 'Merge persons' });
		expect(screen.getByText('Merging on the active research branch')).toBeDefined();
	});

	it('shows no branch note on the mainline', async () => {
		render(Page);

		const button = await screen.findByRole('button', { name: 'Merge persons' });
		expect((button as HTMLButtonElement).disabled).toBe(false);
		expect(screen.queryByText(/^Merging on /)).toBeNull();
	});

	it('shows error when a person fetch fails', async () => {
		vi.mocked(apiModule.api.getPerson).mockImplementation(async (id: string) => {
			if (id === 'p-survivor') return survivorPerson;
			throw { status: 500, message: 'boom' };
		});

		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Failed to load one or both persons. Please try again.')).toBeDefined();
		});
		expect(screen.queryByText('boom')).toBeNull();
		expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy();
		expect(screen.queryByText('Suggested merge')).toBeNull();
	});

	// #1036: the pair can be marked "not a duplicate" from here as well.
	it('marks the pair not a duplicate and returns to the duplicates list', async () => {
		const { goto } = await import('$app/navigation');
		vi.mocked(apiModule.api.dismissDuplicate).mockResolvedValue(undefined);

		render(Page);
		await fireEvent.click(await screen.findByRole('button', { name: 'Not a duplicate' }));

		await waitFor(() => {
			expect(apiModule.api.dismissDuplicate).toHaveBeenCalledWith('p-survivor', 'p-merged');
		});
		await waitFor(() => expect(goto).toHaveBeenCalledWith('/quality?tab=duplicates'));
		expect(apiModule.api.mergePersons).not.toHaveBeenCalled();
	});

	it('stays on the page with the error when marking fails', async () => {
		const { goto } = await import('$app/navigation');
		vi.mocked(apiModule.api.dismissDuplicate).mockRejectedValue({ message: 'Person 2 not found' });

		render(Page);
		await fireEvent.click(await screen.findByRole('button', { name: 'Not a duplicate' }));

		await screen.findByText('Person 2 not found');
		expect(goto).not.toHaveBeenCalled();
	});
});
