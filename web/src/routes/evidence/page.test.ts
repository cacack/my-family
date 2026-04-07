import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';

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

describe('Evidence Hub Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
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
});
