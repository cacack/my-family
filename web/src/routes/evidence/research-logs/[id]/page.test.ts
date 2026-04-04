import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';

// Configurable page params
let mockPageParams = { id: 'rl-1' };

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getResearchLog: vi.fn(),
			createResearchLog: vi.fn(),
			updateResearchLog: vi.fn(),
			deleteResearchLog: vi.fn()
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

const mockLog: apiModule.ResearchLogResponse = {
	id: 'rl-1',
	subject_id: 'p-1-abcd-efgh',
	subject_type: 'person',
	repository: 'Ancestry.com',
	search_description: 'Census records 1850-1870',
	outcome: 'found',
	notes: 'Found in 1860 census',
	search_date: '2024-03-15',
	version: 1,
	created_at: '2024-03-15T10:00:00Z',
	updated_at: '2024-03-15T10:00:00Z'
};

describe('Research Log Detail Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockPageParams = { id: 'rl-1' };
		vi.mocked(apiModule.api.getResearchLog).mockResolvedValue(mockLog);
	});

	it('renders loading state initially', () => {
		vi.mocked(apiModule.api.getResearchLog).mockReturnValue(new Promise(() => {}));
		render(Page);
		expect(screen.getByText('Loading...')).toBeDefined();
	});

	it('loads and displays research log data', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Research Log')).toBeDefined();
			expect(screen.getByText('Ancestry.com')).toBeDefined();
			expect(screen.getByText('Census records 1850-1870')).toBeDefined();
		});
	});

	it('displays Found outcome badge', async () => {
		render(Page);
		await waitFor(() => {
			// "Found" appears in both the badge and the details section
			expect(screen.getAllByText('Found').length).toBeGreaterThanOrEqual(1);
		});
	});

	it('displays Not Found outcome badge', async () => {
		vi.mocked(apiModule.api.getResearchLog).mockResolvedValue({
			...mockLog,
			outcome: 'not_found'
		});
		render(Page);
		await waitFor(() => {
			expect(screen.getAllByText('Not Found').length).toBeGreaterThanOrEqual(1);
		});
	});

	it('displays Inconclusive outcome badge', async () => {
		vi.mocked(apiModule.api.getResearchLog).mockResolvedValue({
			...mockLog,
			outcome: 'inconclusive'
		});
		render(Page);
		await waitFor(() => {
			expect(screen.getAllByText('Inconclusive').length).toBeGreaterThanOrEqual(1);
		});
	});

	it('displays notes section', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Found in 1860 census')).toBeDefined();
		});
	});

	it('displays subject type', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Person')).toBeDefined();
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
		vi.mocked(apiModule.api.getResearchLog).mockRejectedValue(new Error('Not found'));
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

describe('Research Log Detail Page - New Mode', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockPageParams = { id: 'new' };
	});

	it('shows create form when id is "new"', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('New Research Log')).toBeDefined();
		});
	});

	it('shows form fields in create mode', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByLabelText('Subject ID')).toBeDefined();
			expect(screen.getByLabelText('Repository')).toBeDefined();
			expect(screen.getByLabelText('Search Description')).toBeDefined();
			expect(screen.getByLabelText('Outcome')).toBeDefined();
		});
	});

	it('shows Create Research Log submit button', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Create Research Log')).toBeDefined();
		});
	});
});
