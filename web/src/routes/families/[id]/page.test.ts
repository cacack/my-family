import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import FamilyPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getFamily: vi.fn(),
			deleteFamily: vi.fn()
		}
	};
});

// Mock the page store
vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((callback) => {
			callback({ params: { id: 'test-family-id' } });
			return () => {};
		})
	}
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

const mockFamilyWithChildren: apiModule.FamilyDetail = {
	id: 'test-family-id',
	partner1_id: 'partner1-id',
	partner1_name: 'John Smith',
	partner2_id: 'partner2-id',
	partner2_name: 'Jane Smith',
	relationship_type: 'marriage',
	marriage_place: 'Chicago, IL',
	child_count: 2,
	version: 1,
	partner1: {
		id: 'partner1-id',
		given_name: 'John',
		surname: 'Smith'
	},
	partner2: {
		id: 'partner2-id',
		given_name: 'Jane',
		surname: 'Smith'
	},
	children: [
		{
			id: 'child1-id',
			name: 'Alice Smith',
			relationship_type: 'biological'
		},
		{
			id: 'child2-id',
			name: 'Bob Smith',
			relationship_type: 'adopted'
		}
	]
};

const mockFamilyNoChildren: apiModule.FamilyDetail = {
	id: 'test-family-id',
	partner1_name: 'John Smith',
	partner2_name: 'Jane Smith',
	child_count: 0,
	version: 1,
	children: []
};

describe('Family Detail Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('renders loading state initially', () => {
		vi.mocked(apiModule.api.getFamily).mockReturnValue(new Promise(() => {}));

		render(FamilyPage);
		expect(screen.getByText('Loading...')).toBeDefined();
	});

	it('renders family with partners', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('John Smith & Jane Smith')).toBeDefined();
		});
	});

	it('renders marriage badge', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('marriage')).toBeDefined();
		});
	});

	it('renders children list with names', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('Children (2)')).toBeDefined();
			expect(screen.getByText('Alice Smith')).toBeDefined();
			expect(screen.getByText('Bob Smith')).toBeDefined();
		});
	});

	it('shows adopted badge for adopted children', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('(adopted)')).toBeDefined();
		});
	});

	it('does not show biological badge for biological children', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		const { container } = render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('Alice Smith')).toBeDefined();
		});

		// Biological children should not have a type badge
		const childTypes = container.querySelectorAll('.child-type');
		expect(childTypes.length).toBe(1); // Only adopted child has badge
	});

	it('renders empty children message when no children', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyNoChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('No children recorded')).toBeDefined();
		});
	});

	it('renders marriage place', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('Chicago, IL')).toBeDefined();
		});
	});

	it('renders partner links to person pages', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		const { container } = render(FamilyPage);

		await waitFor(() => {
			const partnerLinks = container.querySelectorAll('a.partner-card');
			expect(partnerLinks.length).toBe(2);
			expect(partnerLinks[0].getAttribute('href')).toBe('/persons/partner1-id');
			expect(partnerLinks[1].getAttribute('href')).toBe('/persons/partner2-id');
		});
	});

	it('renders child links to person pages', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		const { container } = render(FamilyPage);

		await waitFor(() => {
			const childLinks = container.querySelectorAll('.children-list a');
			expect(childLinks.length).toBe(2);
			expect(childLinks[0].getAttribute('href')).toBe('/persons/child1-id');
			expect(childLinks[1].getAttribute('href')).toBe('/persons/child2-id');
		});
	});

	it('renders error state on API failure', async () => {
		vi.mocked(apiModule.api.getFamily).mockRejectedValue({ message: 'Family not found' });

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('Family not found')).toBeDefined();
		});
	});

	it('renders back link to families list', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		const { container } = render(FamilyPage);

		await waitFor(() => {
			const backLink = container.querySelector('.back-link');
			expect(backLink).not.toBeNull();
			expect(backLink?.getAttribute('href')).toBe('/families');
		});
	});
});
