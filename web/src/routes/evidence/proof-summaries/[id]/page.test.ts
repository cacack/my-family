import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';

// Configurable page params
let mockPageParams = { id: 'ps-1' };

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getProofSummary: vi.fn(),
			getEvidenceAnalysis: vi.fn(),
			createProofSummary: vi.fn(),
			updateProofSummary: vi.fn(),
			deleteProofSummary: vi.fn()
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

const mockLinkedAnalysis: apiModule.EvidenceAnalysisResponse = {
	id: 'ea-1',
	fact_type: 'person_birth',
	subject_id: 'p-1-abcd-efgh',
	conclusion: 'Born 1850 per census',
	research_status: 'probable',
	citation_ids: ['c-1'],
	version: 1
};

const mockSummary: apiModule.ProofSummaryResponse = {
	id: 'ps-1',
	fact_type: 'person_birth',
	subject_id: 'p-1-abcd-efgh',
	conclusion: 'Born 1850 in Ohio',
	argument: 'Based on three independent sources: the 1860 census, a family bible, and a church record, we can conclude...',
	analysis_ids: ['ea-1'],
	research_status: 'certain',
	version: 1,
	created_at: '2024-03-15T10:00:00Z',
	updated_at: '2024-03-15T10:00:00Z'
};

describe('Proof Summary Detail Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockPageParams = { id: 'ps-1' };
		vi.mocked(apiModule.api.getProofSummary).mockResolvedValue(mockSummary);
		vi.mocked(apiModule.api.getEvidenceAnalysis).mockResolvedValue(mockLinkedAnalysis);
	});

	it('renders loading state initially', () => {
		vi.mocked(apiModule.api.getProofSummary).mockReturnValue(new Promise(() => {}));
		render(Page);
		expect(screen.getByText('Loading...')).toBeDefined();
	});

	it('loads and displays proof summary data', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Proof Summary')).toBeDefined();
			expect(screen.getByText('Born 1850 in Ohio')).toBeDefined();
		});
	});

	it('displays argument text prominently', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText(/Based on three independent sources/)).toBeDefined();
		});
	});

	it('displays linked analyses', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Born 1850 per census')).toBeDefined();
		});
	});

	it('displays supporting analyses count', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Supporting Analyses (1)')).toBeDefined();
		});
	});

	it('shows Edit and Delete buttons', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Edit')).toBeDefined();
			expect(screen.getByText('Delete')).toBeDefined();
		});
	});

	it('displays version info', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Version: 1')).toBeDefined();
		});
	});

	it('renders error state on API failure', async () => {
		vi.mocked(apiModule.api.getProofSummary).mockRejectedValue(new Error('Not found'));
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Not found')).toBeDefined();
			expect(screen.getByText('Retry')).toBeDefined();
		});
	});

	it('has back link to evidence page', async () => {
		const { container } = render(Page);
		await waitFor(() => {
			const backLink = container.querySelector('a[href="/evidence"]');
			expect(backLink).not.toBeNull();
		});
	});
});

describe('Proof Summary Detail Page - New Mode', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockPageParams = { id: 'new' };
	});

	it('shows create form when id is "new"', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('New Proof Summary')).toBeDefined();
		});
	});

	it('shows form fields including argument textarea', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByLabelText('Fact Type')).toBeDefined();
			expect(screen.getByLabelText('Subject ID')).toBeDefined();
			expect(screen.getByLabelText('Conclusion')).toBeDefined();
			expect(screen.getByLabelText('Argument')).toBeDefined();
			expect(screen.getByLabelText('Research Status')).toBeDefined();
		});
	});

	it('shows Create Proof Summary submit button', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Create Proof Summary')).toBeDefined();
		});
	});
});

describe('Proof Summary Detail Page - save conflict (#939)', () => {
	const stale = { status: 409, code: 'VERSION_CONFLICT', message: 'Conflict' };

	beforeEach(() => {
		vi.clearAllMocks();
		mockPageParams = { id: 'ps-1' };
		vi.mocked(apiModule.api.getEvidenceAnalysis).mockResolvedValue(mockLinkedAnalysis);
	});

	async function editArgument(value: string) {
		render(Page);
		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
		await fireEvent.input(screen.getByLabelText('Argument'), { target: { value } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
	}

	it('compares the edits with the latest version and keeps them in the form', async () => {
		vi.mocked(apiModule.api.getProofSummary)
			.mockResolvedValueOnce(mockSummary)
			.mockResolvedValue({ ...mockSummary, argument: 'Their argument', version: 2 });
		vi.mocked(apiModule.api.updateProofSummary).mockRejectedValueOnce(stale);
		await editArgument('My argument');

		expect(await screen.findByRole('rowheader', { name: 'Argument' })).toBeTruthy();
		expect(screen.getByTestId('latest').textContent).toBe('Their argument');
		expect((screen.getByLabelText('Argument') as HTMLTextAreaElement).value).toBe('My argument');
		expect(screen.queryByText(/reload and try again/i)).toBeNull();
	});

	it("keeps another writer's changes to the fields the user did not edit", async () => {
		const latest = { ...mockSummary, conclusion: 'Born 1851 in Ohio', analysis_ids: ['ea-1', 'ea-2'], version: 2 };
		vi.mocked(apiModule.api.getProofSummary).mockResolvedValueOnce(mockSummary).mockResolvedValue(latest);
		vi.mocked(apiModule.api.updateProofSummary)
			.mockRejectedValueOnce(stale)
			.mockResolvedValueOnce({ ...latest, argument: 'My argument', version: 3 });
		await editArgument('My argument');

		expect(await screen.findByRole('rowheader', { name: 'Argument' })).toBeTruthy();
		expect(screen.queryByRole('rowheader', { name: 'Conclusion' })).toBeNull();
		expect(screen.queryByRole('rowheader', { name: 'Linked Analysis IDs' })).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: 'Save my edits' }));

		await waitFor(() => expect(apiModule.api.updateProofSummary).toHaveBeenCalledTimes(2));
		expect(vi.mocked(apiModule.api.updateProofSummary).mock.calls[1][1]).toMatchObject({
			argument: 'My argument',
			conclusion: 'Born 1851 in Ohio',
			analysis_ids: ['ea-1', 'ea-2'],
			version: 2
		});
	});
});
