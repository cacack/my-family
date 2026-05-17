import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';

// Configurable page params
let mockPageParams: Record<string, string> = {
	survivorId: 'p-survivor',
	mergedId: 'p-merged'
};

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getPerson: vi.fn(),
			mergePersons: vi.fn()
		}
	};
});

// Mock the page store with configurable params
vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((callback: (value: unknown) => void) => {
			callback({ params: mockPageParams });
			return () => {};
		})
	}
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
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

	it('rejects self-merge before fetching', async () => {
		mockPageParams = { survivorId: 'p-same', mergedId: 'p-same' };
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Cannot merge a person with themselves.')).toBeDefined();
		});
		expect(apiModule.api.getPerson).not.toHaveBeenCalled();
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

	it('shows error when a person fetch fails', async () => {
		vi.mocked(apiModule.api.getPerson).mockImplementation(async (id: string) => {
			if (id === 'p-survivor') return survivorPerson;
			throw { message: 'Person not found' };
		});

		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Person not found')).toBeDefined();
		});
	});
});
