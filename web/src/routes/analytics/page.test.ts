import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import AnalyticsPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getQualityOverview: vi.fn(),
			listPersons: vi.fn(),
			listFamilies: vi.fn()
		}
	};
});

const { branchState } = vi.hoisted(() => ({
	branchState: { id: null as string | null }
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState
}));

function overview(overrides: Partial<apiModule.QualityOverview> = {}): apiModule.QualityOverview {
	return {
		total_persons: 0,
		average_completeness: 0,
		records_with_issues: 0,
		top_issues: [],
		research_status_counts: { certain: 0, probable: 0, possible: 0, unknown: 0, unset: 0 },
		lowest_scoring: [],
		...overrides
	};
}

function mockFamilies(total: number) {
	vi.mocked(apiModule.api.listFamilies).mockResolvedValue({
		items: [],
		total
	} as unknown as Awaited<ReturnType<typeof apiModule.api.listFamilies>>);
}

describe('Completeness (analytics) page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(apiModule.api.getQualityOverview).mockResolvedValue(overview());
		mockFamilies(0);
	});

	it('shows the server totals, not a count of a fetched page (#894)', async () => {
		vi.mocked(apiModule.api.getQualityOverview).mockResolvedValue(
			overview({ total_persons: 3013, average_completeness: 61.6, records_with_issues: 2500 })
		);
		mockFamilies(1423);

		render(AnalyticsPage);

		await waitFor(() => expect(screen.getByText((3013).toLocaleString())).toBeTruthy());
		expect(screen.getByText((1423).toLocaleString())).toBeTruthy();
		expect(screen.getByText((2500).toLocaleString())).toBeTruthy();
		// No person list is fetched to recompute anything client-side.
		expect(apiModule.api.listPersons).not.toHaveBeenCalled();
		expect(apiModule.api.listFamilies).toHaveBeenCalledWith({ limit: 1 });
	});

	it('renders the research status counts and lowest-scoring records from the overview', async () => {
		vi.mocked(apiModule.api.getQualityOverview).mockResolvedValue(
			overview({
				total_persons: 10,
				records_with_issues: 1,
				top_issues: [{ issue: 'Missing birth place', count: 7 }],
				research_status_counts: { certain: 4, probable: 3, possible: 0, unknown: 0, unset: 3 },
				lowest_scoring: [
					{
						person_id: '11111111-1111-4111-8111-111111111111',
						given_name: 'Ada',
						surname: 'Lovelace',
						completeness_score: 21.4,
						issues: ['Missing birth date', 'Missing birth place']
					}
				]
			})
		);

		render(AnalyticsPage);

		const link = await screen.findByRole('link', { name: 'Ada Lovelace' });
		expect(link.getAttribute('href')).toBe('/persons/11111111-1111-4111-8111-111111111111');
		expect(screen.getByText('Missing birth date')).toBeTruthy();
		expect(screen.getByText('4')).toBeTruthy();
		expect(screen.getByRole('heading', { name: 'Most Common Issues' })).toBeTruthy();
	});

	it('names itself Completeness and points to Quality for validation and duplicates', async () => {
		render(AnalyticsPage);
		expect(await screen.findByRole('heading', { level: 1, name: 'Completeness' })).toBeTruthy();
		expect(screen.getByRole('link', { name: 'Quality' }).getAttribute('href')).toBe('/quality');
	});

	it('offers import on an empty tree', async () => {
		render(AnalyticsPage);
		expect(await screen.findByRole('link', { name: 'Import a GEDCOM file' })).toBeTruthy();
	});

	it('shows an error when the overview fails to load', async () => {
		vi.mocked(apiModule.api.getQualityOverview).mockRejectedValue(new Error('boom'));
		vi.spyOn(console, 'error').mockImplementation(() => {});
		render(AnalyticsPage);
		expect(await screen.findByText('Failed to load data. Please try again.')).toBeTruthy();
	});

	it('shows no mainline notice on the mainline', async () => {
		render(AnalyticsPage);
		await waitFor(() => expect(screen.getByText('Total Persons')).toBeTruthy());
		expect(screen.queryByRole('note')).toBeNull();
	});

	it('shows no mainline notice on a branch either: the overview and families both follow it (#829, #894)', async () => {
		branchState.id = 'b-1';
		render(AnalyticsPage);
		await waitFor(() => expect(screen.getByText('Total Persons')).toBeTruthy());
		expect(screen.queryByRole('note')).toBeNull();
	});
});
