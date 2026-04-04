import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import EvidencePanel from './EvidencePanel.svelte';
import * as apiModule from '$lib/api/client';

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listEvidenceAnalyses: vi.fn(),
			getConflictsBySubject: vi.fn(),
			getResearchLogsBySubject: vi.fn()
		}
	};
});

const mockAnalyses: apiModule.EvidenceAnalysisListResponse = {
	analyses: [
		{
			id: 'ea-1',
			fact_type: 'person_birth',
			subject_id: 'p-1',
			conclusion: 'Born circa 1850',
			research_status: 'probable',
			citation_ids: ['c-1'],
			version: 1
		},
		{
			id: 'ea-2',
			fact_type: 'person_death',
			subject_id: 'p-1',
			conclusion: 'Died 1920',
			research_status: 'certain',
			version: 1
		}
	],
	total: 2
};

const mockConflicts: apiModule.EvidenceConflictResponse[] = [
	{
		id: 'ec-1',
		fact_type: 'person_death',
		subject_id: 'p-1',
		analysis_ids: ['ea-1', 'ea-2'],
		description: 'Death date disputed',
		status: 'open',
		version: 1
	}
];

const mockLogs: apiModule.ResearchLogResponse[] = [
	{
		id: 'rl-1',
		subject_id: 'p-1',
		subject_type: 'person',
		repository: 'Ancestry.com',
		search_description: 'Census records search',
		outcome: 'found',
		search_date: '2024-03-15',
		version: 1
	}
];

describe('EvidencePanel', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockResolvedValue(mockAnalyses);
		vi.mocked(apiModule.api.getConflictsBySubject).mockResolvedValue(mockConflicts);
		vi.mocked(apiModule.api.getResearchLogsBySubject).mockResolvedValue(mockLogs);
	});

	it('renders panel header', async () => {
		render(EvidencePanel, { props: { subjectId: 'p-1' } });
		expect(screen.getByText('Evidence & Research')).toBeDefined();
	});

	it('shows summary counts in header after load', async () => {
		render(EvidencePanel, { props: { subjectId: 'p-1' } });
		await waitFor(() => {
			expect(screen.getByText(/2 analyses/)).toBeDefined();
			expect(screen.getByText(/1 conflict/)).toBeDefined();
			expect(screen.getByText(/1 research log/)).toBeDefined();
		});
	});

	it('shows conflict badge when open conflicts exist', async () => {
		const { container } = render(EvidencePanel, { props: { subjectId: 'p-1' } });
		await waitFor(() => {
			// The open conflict count badge in the header
			expect(screen.getByText('1', { selector: '.ml-1' }) || container.querySelector('[data-slot="badge"]')).toBeDefined();
		});
	});

	it('shows expanded content when clicked', async () => {
		render(EvidencePanel, { props: { subjectId: 'p-1' } });
		await waitFor(() => {
			expect(screen.getByText(/2 analyses/)).toBeDefined();
		});

		// Click to expand
		const header = screen.getByText('Evidence & Research').closest('button')!;
		await fireEvent.click(header);

		await waitFor(() => {
			expect(screen.getByText('Born circa 1850')).toBeDefined();
			expect(screen.getByText('Died 1920')).toBeDefined();
		});
	});

	it('shows empty state when no evidence data', async () => {
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockResolvedValue({ analyses: [], total: 0 });
		vi.mocked(apiModule.api.getConflictsBySubject).mockResolvedValue([]);
		vi.mocked(apiModule.api.getResearchLogsBySubject).mockResolvedValue([]);

		render(EvidencePanel, { props: { subjectId: 'p-1' } });

		// Expand the panel
		await waitFor(() => {
			expect(screen.getByText('Evidence & Research')).toBeDefined();
		});
		const header = screen.getByText('Evidence & Research').closest('button')!;
		await fireEvent.click(header);

		await waitFor(() => {
			expect(screen.getByText(/No evidence data yet/)).toBeDefined();
		});
	});

	it('shows analysis links to detail pages when expanded', async () => {
		const { container } = render(EvidencePanel, { props: { subjectId: 'p-1' } });
		await waitFor(() => {
			expect(screen.getByText(/2 analyses/)).toBeDefined();
		});

		const header = screen.getByText('Evidence & Research').closest('button')!;
		await fireEvent.click(header);

		await waitFor(() => {
			const analysisLink = container.querySelector('a[href="/evidence/analyses/ea-1"]');
			expect(analysisLink).not.toBeNull();
		});
	});

	it('shows conflict links when expanded', async () => {
		const { container } = render(EvidencePanel, { props: { subjectId: 'p-1' } });
		await waitFor(() => {
			expect(screen.getByText(/1 conflict/)).toBeDefined();
		});

		const header = screen.getByText('Evidence & Research').closest('button')!;
		await fireEvent.click(header);

		await waitFor(() => {
			const conflictLink = container.querySelector('a[href="/evidence/conflicts/ec-1"]');
			expect(conflictLink).not.toBeNull();
		});
	});

	it('shows loading state when expanding before data loads', async () => {
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockReturnValue(new Promise(() => {}));
		vi.mocked(apiModule.api.getConflictsBySubject).mockReturnValue(new Promise(() => {}));
		vi.mocked(apiModule.api.getResearchLogsBySubject).mockReturnValue(new Promise(() => {}));

		render(EvidencePanel, { props: { subjectId: 'p-1' } });

		const header = screen.getByText('Evidence & Research').closest('button')!;
		await fireEvent.click(header);

		expect(screen.getByText('Loading evidence data...')).toBeDefined();
	});

	it('shows error state when API fails', async () => {
		vi.mocked(apiModule.api.listEvidenceAnalyses).mockRejectedValue(new Error('Network error'));

		render(EvidencePanel, { props: { subjectId: 'p-1' } });

		const header = screen.getByText('Evidence & Research').closest('button')!;
		await fireEvent.click(header);

		await waitFor(() => {
			expect(screen.getByText('Network error')).toBeDefined();
		});
	});
});
