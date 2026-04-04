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
			getEvidenceConflict: vi.fn(),
			getEvidenceAnalysis: vi.fn(),
			resolveEvidenceConflict: vi.fn()
		}
	};
});

// Mock the page store
vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((callback: (value: unknown) => void) => {
			callback({ params: { id: 'ec-1' } });
			return () => {};
		})
	}
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

const mockOpenConflict: apiModule.EvidenceConflictResponse = {
	id: 'ec-1',
	fact_type: 'person_death',
	subject_id: 'p-1-abcd-efgh',
	analysis_ids: ['ea-1', 'ea-2'],
	description: 'Death date disputed between two sources',
	status: 'open',
	version: 1,
	created_at: '2024-03-15T10:00:00Z'
};

const mockResolvedConflict: apiModule.EvidenceConflictResponse = {
	...mockOpenConflict,
	status: 'resolved',
	resolution: 'The 1920 death certificate is the primary source.',
	version: 2
};

const mockLinkedAnalysis1: apiModule.EvidenceAnalysisResponse = {
	id: 'ea-1',
	fact_type: 'person_death',
	subject_id: 'p-1-abcd-efgh',
	conclusion: 'Died 1920 per death certificate',
	research_status: 'certain',
	citation_ids: ['c-1'],
	version: 1
};

const mockLinkedAnalysis2: apiModule.EvidenceAnalysisResponse = {
	id: 'ea-2',
	fact_type: 'person_death',
	subject_id: 'p-1-abcd-efgh',
	conclusion: 'Died 1918 per newspaper',
	research_status: 'possible',
	version: 1
};

describe('Conflict Detail Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getEvidenceConflict).mockResolvedValue(mockOpenConflict);
		vi.mocked(apiModule.api.getEvidenceAnalysis).mockImplementation((id: string) => {
			if (id === 'ea-1') return Promise.resolve(mockLinkedAnalysis1);
			if (id === 'ea-2') return Promise.resolve(mockLinkedAnalysis2);
			return Promise.reject(new Error('Not found'));
		});
	});

	it('renders loading state initially', () => {
		vi.mocked(apiModule.api.getEvidenceConflict).mockReturnValue(new Promise(() => {}));
		render(Page);
		expect(screen.getByText('Loading...')).toBeDefined();
	});

	it('loads and displays conflict description', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Evidence Conflict')).toBeDefined();
			expect(screen.getByText('Death date disputed between two sources')).toBeDefined();
		});
	});

	it('displays Open status badge for open conflicts', async () => {
		render(Page);
		await waitFor(() => {
			// "Open" appears in the header badge and in the details section
			expect(screen.getAllByText('Open').length).toBeGreaterThanOrEqual(1);
		});
	});

	it('shows resolution form for open conflicts', async () => {
		render(Page);
		await waitFor(() => {
			// "Resolve Conflict" appears as section heading and button text
			expect(screen.getAllByText('Resolve Conflict').length).toBeGreaterThanOrEqual(1);
			expect(screen.getByLabelText('Resolution text')).toBeDefined();
		});
	});

	it('displays linked analyses', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Died 1920 per death certificate')).toBeDefined();
			expect(screen.getByText('Died 1918 per newspaper')).toBeDefined();
		});
	});

	it('renders error state on API failure', async () => {
		vi.mocked(apiModule.api.getEvidenceConflict).mockRejectedValue(new Error('Not found'));
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

describe('Conflict Detail Page - Resolved', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getEvidenceConflict).mockResolvedValue(mockResolvedConflict);
		vi.mocked(apiModule.api.getEvidenceAnalysis).mockImplementation((id: string) => {
			if (id === 'ea-1') return Promise.resolve(mockLinkedAnalysis1);
			if (id === 'ea-2') return Promise.resolve(mockLinkedAnalysis2);
			return Promise.reject(new Error('Not found'));
		});
	});

	it('displays Resolved status badge', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getAllByText('Resolved').length).toBeGreaterThanOrEqual(1);
		});
	});

	it('shows resolution text', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('The 1920 death certificate is the primary source.')).toBeDefined();
		});
	});

	it('hides resolution form for resolved conflicts', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Evidence Conflict')).toBeDefined();
		});
		expect(screen.queryByText('Resolve Conflict')).toBeNull();
	});
});
