import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listEvidenceAnalyses: vi.fn(),
			listEvidenceConflicts: vi.fn(),
			listResearchLogs: vi.fn(),
			listProofSummaries: vi.fn()
		}
	};
});

const mockAnalysesList: apiModule.EvidenceAnalysisListResponse = {
	analyses: [
		{
			id: 'ea-1',
			fact_type: 'person_birth',
			subject_id: 'p-1-long-uuid',
			conclusion: 'Born circa 1850 in Ohio',
			research_status: 'probable',
			citation_ids: ['c-1'],
			notes: 'Based on census records',
			version: 1
		},
		{
			id: 'ea-2',
			fact_type: 'person_death',
			subject_id: 'p-2-long-uuid',
			conclusion: 'Died 1920 in Illinois',
			research_status: 'certain',
			citation_ids: ['c-2', 'c-3'],
			version: 1
		}
	],
	total: 2
};

const emptyAnalyses: apiModule.EvidenceAnalysisListResponse = { analyses: [], total: 0 };
const emptyConflicts: apiModule.EvidenceConflictListResponse = { conflicts: [], total: 0 };
const emptyLogs: apiModule.ResearchLogListResponse = { logs: [], total: 0 };
const emptySummaries: apiModule.ProofSummaryListResponse = { summaries: [], total: 0 };

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

describe('Evidence Hub Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/evidence');
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockResolvedValue(mockAnalysesList);
		vi.mocked(apiModule.api.listEvidenceConflicts).mockResolvedValue(emptyConflicts);
		vi.mocked(apiModule.api.listResearchLogs).mockResolvedValue(emptyLogs);
		vi.mocked(apiModule.api.listProofSummaries).mockResolvedValue(emptySummaries);
	});

	it('renders page title and subtitle', async () => {
		render(Page);
		expect(screen.getByText('Evidence Analysis')).toBeDefined();
		expect(screen.getByText('GPS-compliant research tracking and proof management')).toBeDefined();
	});

	it('renders all 4 tab triggers', async () => {
		render(Page);
		expect(screen.getByText('Analyses')).toBeDefined();
		// "Conflicts" appears multiple times (tab trigger + possible badge), use getAllByText
		expect(screen.getAllByText('Conflicts').length).toBeGreaterThanOrEqual(1);
		expect(screen.getByText('Research Logs')).toBeDefined();
		expect(screen.getByText('Proof Summaries')).toBeDefined();
	});

	it('shows loading state for analyses tab', () => {
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockReturnValue(new Promise(() => {}));
		render(Page);
		expect(screen.getByText('Loading analyses...')).toBeDefined();
	});

	it('renders analyses data after load', async () => {
		render(Page);
		await waitFor(() => {
			// "Person Birth" appears in both desktop table and mobile cards
			expect(screen.getAllByText('Person Birth').length).toBeGreaterThanOrEqual(1);
			expect(screen.getAllByText('Person Death').length).toBeGreaterThanOrEqual(1);
		});
	});

	it('shows analyses count', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('2 analyses')).toBeDefined();
		});
	});

	it('shows empty state when no analyses', async () => {
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockResolvedValue(emptyAnalyses);
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No evidence analyses yet.')).toBeDefined();
		});
	});

	it('shows error state with retry button when API fails', async () => {
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockRejectedValue(new Error('Network error'));
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Network error')).toBeDefined();
			expect(screen.getByText('Retry')).toBeDefined();
		});
	});

	it('opens the tab the URL names (#901)', async () => {
		resetRouter('/evidence?tab=logs');
		render(Page);
		await waitFor(() => expect(apiModule.api.listResearchLogs).toHaveBeenCalled());
		expect(screen.getByRole('tab', { name: 'Research Logs' }).getAttribute('aria-selected')).toBe('true');
		expect(apiModule.api.listEvidenceAnalyses).not.toHaveBeenCalled();
	});

	it('pushes a tab switch, so Back returns to the previous tab (#901)', async () => {
		render(Page);
		await waitFor(() => expect(apiModule.api.listEvidenceAnalyses).toHaveBeenCalled());

		const summaries = screen.getByRole('tab', { name: 'Proof Summaries' });
		await fireEvent.click(summaries);
		await waitFor(() => expect(currentPath()).toBe('/evidence?tab=summaries'));
		expect(historyLength()).toBe(2);
		await waitFor(() => expect(apiModule.api.listProofSummaries).toHaveBeenCalled());

		back();
		await waitFor(() =>
			expect(screen.getByRole('tab', { name: 'Analyses' }).getAttribute('aria-selected')).toBe('true')
		);
	});
	const manyConflicts = (status: apiModule.ConflictStatus): apiModule.EvidenceConflictListResponse => ({
		conflicts: [
			{
				id: `ec-${status}`,
				fact_type: 'person_birth',
				subject_id: 'p-1-long-uuid',
				analysis_ids: ['ea-1', 'ea-2'],
				description: `A ${status} conflict`,
				status,
				version: 1
			}
		],
		total: 45
	});

	it('restores the Conflicts page and status filter from the URL (#946)', async () => {
		vi.mocked(apiModule.api.listEvidenceConflicts).mockResolvedValue(manyConflicts('open'));
		resetRouter('/evidence?tab=conflicts&status=open&page=2');
		render(Page);
		await waitFor(() =>
			expect(apiModule.api.listEvidenceConflicts).toHaveBeenCalledWith({
				limit: 20,
				offset: 20,
				status: 'open'
			})
		);
		await waitFor(() => expect(screen.getByText('Page 2 of 3')).toBeDefined());
		expect(screen.getByRole('button', { name: 'Open' }).getAttribute('aria-pressed')).toBe('true');
	});

	it('pushes a page change, so Back returns to the previous page (#946)', async () => {
		vi.mocked(apiModule.api.listEvidenceConflicts).mockResolvedValue(manyConflicts('open'));
		resetRouter('/evidence?tab=conflicts');
		render(Page);
		await waitFor(() => expect(screen.getByText('Page 1 of 3')).toBeDefined());

		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		await waitFor(() => expect(currentPath()).toBe('/evidence?tab=conflicts&page=2'));
		expect(historyLength()).toBe(2);
		await waitFor(() => expect(screen.getByText('Page 2 of 3')).toBeDefined());
		expect(apiModule.api.listEvidenceConflicts).toHaveBeenCalledWith(
			expect.objectContaining({ offset: 20 })
		);

		back();
		await waitFor(() => expect(screen.getByText('Page 1 of 3')).toBeDefined());
	});

	it('replaces the URL on a filter change and starts at page 1 (#946)', async () => {
		vi.mocked(apiModule.api.listEvidenceConflicts).mockResolvedValue(manyConflicts('resolved'));
		resetRouter('/evidence?tab=conflicts&page=3');
		render(Page);
		await waitFor(() => expect(screen.getByText('Page 3 of 3')).toBeDefined());

		await fireEvent.click(screen.getByRole('button', { name: 'Resolved' }));
		await waitFor(() => expect(currentPath()).toBe('/evidence?tab=conflicts&status=resolved'));
		expect(historyLength()).toBe(1);
		await waitFor(() =>
			expect(apiModule.api.listEvidenceConflicts).toHaveBeenCalledWith({
				limit: 20,
				offset: 0,
				status: 'resolved'
			})
		);

		await fireEvent.click(screen.getByRole('button', { name: 'All' }));
		await waitFor(() => expect(currentPath()).toBe('/evidence?tab=conflicts'));
	});

	it('keeps defaults out of the URL when switching tabs (#946)', async () => {
		resetRouter('/evidence?tab=conflicts&status=open&page=2');
		render(Page);
		await fireEvent.click(screen.getByRole('tab', { name: 'Analyses' }));
		await waitFor(() => expect(currentPath()).toBe('/evidence'));
		await waitFor(() =>
			expect(apiModule.api.listEvidenceAnalyses).toHaveBeenCalledWith({ limit: 20, offset: 0 })
		);
	});

	it('ignores a load that resolves after a newer one (#947)', async () => {
		let resolveSlow: (r: apiModule.EvidenceAnalysisListResponse) => void = () => {};
		const stale: apiModule.EvidenceAnalysisListResponse = {
			analyses: [{ ...mockAnalysesList.analyses[0], id: 'ea-old', conclusion: 'Stale conclusion' }],
			total: 1
		};
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockReturnValueOnce(
			new Promise((r) => (resolveSlow = r))
		);
		render(Page);
		await fireEvent.click(screen.getByRole('tab', { name: 'Proof Summaries' }));
		await waitFor(() => expect(currentPath()).toBe('/evidence?tab=summaries'));
		back();
		await waitFor(() => expect(screen.getAllByText('Born circa 1850 in Ohio').length).toBeGreaterThan(0));

		resolveSlow(stale);
		await new Promise((r) => setTimeout(r, 0));
		expect(screen.getAllByText('Born circa 1850 in Ohio').length).toBeGreaterThan(0);
		expect(screen.queryByText('Stale conclusion')).toBeNull();
	});
});
