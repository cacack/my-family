import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';

// Configurable page params
let mockPageParams = { id: 'ea-1' };
let mockPageUrl: URL | undefined;

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getEvidenceAnalysis: vi.fn(),
			createEvidenceAnalysis: vi.fn(),
			updateEvidenceAnalysis: vi.fn(),
			deleteEvidenceAnalysis: vi.fn()
		}
	};
});

// Mock the page store with configurable params
vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((callback: (value: unknown) => void) => {
			callback({ params: mockPageParams, url: mockPageUrl });
			return () => {};
		})
	}
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

const mockAnalysis: apiModule.EvidenceAnalysisResponse = {
	id: 'ea-1',
	fact_type: 'person_birth',
	subject_id: 'p-1-abcd-efgh',
	conclusion: 'Born circa 1850 in Ohio',
	research_status: 'probable',
	citation_ids: ['c-1'],
	notes: 'Based on census records',
	version: 1,
	created_at: '2024-03-15T10:00:00Z',
	updated_at: '2024-03-15T10:00:00Z'
};

describe('Analysis Detail Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockPageParams = { id: 'ea-1' };
		vi.mocked(apiModule.api.getEvidenceAnalysis).mockResolvedValue(mockAnalysis);
	});

	it('renders loading state initially', () => {
		vi.mocked(apiModule.api.getEvidenceAnalysis).mockReturnValue(new Promise(() => {}));
		render(Page);
		expect(screen.getByText('Loading...')).toBeDefined();
	});

	it('loads and displays analysis data', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Evidence Analysis')).toBeDefined();
			expect(screen.getByText('Born circa 1850 in Ohio')).toBeDefined();
		});
	});

	it('displays subject link', async () => {
		const { container } = render(Page);
		await waitFor(() => {
			const subjectLink = container.querySelector('a[href="/persons/p-1-abcd-efgh"]');
			expect(subjectLink).not.toBeNull();
		});
	});

	it('displays notes section', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Based on census records')).toBeDefined();
		});
	});

	it('displays citation count', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Citations (1)')).toBeDefined();
		});
	});

	it('displays version info', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Version: 1')).toBeDefined();
		});
	});

	it('renders error state on API failure', async () => {
		vi.mocked(apiModule.api.getEvidenceAnalysis).mockRejectedValue(new Error('Not found'));
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Not found')).toBeDefined();
			expect(screen.getByText('Retry')).toBeDefined();
		});
	});

	it('shows Edit and Delete buttons in view mode', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Edit')).toBeDefined();
			expect(screen.getByText('Delete')).toBeDefined();
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

describe('Analysis Detail Page - New Mode', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockPageParams = { id: 'new' };
		mockPageUrl = undefined;
	});

	it('prefills the subject and fact type from the URL (the merge review link)', async () => {
		mockPageUrl = new URL(
			'http://localhost/evidence/analyses/new?subjectId=p-9&factType=person_death'
		);
		render(Page);
		await waitFor(() => {
			expect((screen.getByLabelText('Fact Type') as HTMLSelectElement).value).toBe('person_death');
			expect((screen.getByLabelText('Subject ID') as HTMLInputElement).value).toBe('p-9');
		});
	});

	it('says so, and keeps the default, for a fact type the form does not offer', async () => {
		mockPageUrl = new URL('http://localhost/evidence/analyses/new?subjectId=p-9&factType=bogus');
		render(Page);
		await waitFor(() => {
			expect((screen.getByLabelText('Fact Type') as HTMLSelectElement).value).toBe('person_birth');
		});
		expect(
			screen.getByText(/The linked fact type "bogus" is not one an analysis can have/)
		).toBeDefined();
	});

	it('offers every fact type an analysis can have (the merge review links any of them)', async () => {
		for (const factType of [
			'person_cremation',
			'person_christening',
			'person_generic_event',
			'family_marriage_bann',
			'family_marriage_contract',
			'family_marriage_license',
			'family_marriage_settlement'
		]) {
			mockPageUrl = new URL(
				`http://localhost/evidence/analyses/new?subjectId=p-9&factType=${factType}`
			);
			const { unmount } = render(Page);
			await waitFor(() => {
				expect((screen.getByLabelText('Fact Type') as HTMLSelectElement).value).toBe(factType);
			});
			expect(screen.queryByText(/is not one an analysis can have/)).toBeNull();
			unmount();
		}
	});

	it('defaults a family subject with no fact type to its marriage', async () => {
		mockPageUrl = new URL(
			'http://localhost/evidence/analyses/new?subjectId=f-9&subjectType=family'
		);
		render(Page);
		await waitFor(() => {
			expect((screen.getByLabelText('Fact Type') as HTMLSelectElement).value).toBe(
				'family_marriage'
			);
			expect((screen.getByLabelText('Subject ID') as HTMLInputElement).value).toBe('f-9');
		});
	});

	it('shows create form when id is "new"', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('New Evidence Analysis')).toBeDefined();
		});
	});

	it('shows form fields in create mode', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByLabelText('Fact Type')).toBeDefined();
			expect(screen.getByLabelText('Subject ID')).toBeDefined();
			expect(screen.getByLabelText('Conclusion')).toBeDefined();
			expect(screen.getByLabelText('Research Status')).toBeDefined();
		});
	});

	it('shows Create Analysis submit button', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Create Analysis')).toBeDefined();
		});
	});

	it('does not fetch analysis data in create mode', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('New Evidence Analysis')).toBeDefined();
		});
		expect(apiModule.api.getEvidenceAnalysis).not.toHaveBeenCalled();
	});
});
