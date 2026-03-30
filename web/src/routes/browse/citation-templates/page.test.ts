import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listCitationTemplates: vi.fn()
		}
	};
});

const mockTemplates = {
	templates: [
		{
			id: 'census.us.federal',
			name: 'U.S. Federal Census',
			category: 'Census Records',
			description: 'For U.S. federal census records',
			source_types: ['census'],
			fields: [
				{ key: 'year', label: 'Census Year', help_text: 'The year of the census', required: true },
				{ key: 'state', label: 'State', help_text: 'The state', required: true },
				{ key: 'county', label: 'County', help_text: 'The county', required: false },
				{ key: 'notes', label: 'Notes', help_text: 'Additional notes', required: false }
			]
		},
		{
			id: 'vital.birth',
			name: 'Birth Certificate',
			category: 'Vital Records',
			description: 'For birth certificates',
			source_types: ['vital_record'],
			fields: [
				{ key: 'registrant', label: 'Registrant', required: true },
				{ key: 'date', label: 'Date of Birth', required: true },
				{ key: 'place', label: 'Place', required: false }
			]
		}
	]
};

describe('Citation Templates Browse Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.listCitationTemplates).mockResolvedValue(mockTemplates);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('renders page title', async () => {
		render(Page);
		expect(screen.getByText('Citation Templates')).toBeDefined();
	});

	it('shows loading state', () => {
		vi.mocked(apiModule.api.listCitationTemplates).mockReturnValue(new Promise(() => {}));
		render(Page);
		expect(screen.getByText('Loading citation templates...')).toBeDefined();
	});

	it('renders all templates after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('U.S. Federal Census')).toBeDefined();
			expect(screen.getByText('Birth Certificate')).toBeDefined();
		});
	});

	it('category tabs are present', async () => {
		render(Page);
		await waitFor(() => {
			// "All" tab plus category tabs
			expect(screen.getByText(/All \(2\)/)).toBeDefined();
			expect(screen.getByText(/Census Records \(1\)/)).toBeDefined();
			expect(screen.getByText(/Vital Records \(1\)/)).toBeDefined();
		});
	});

	it('source type filter is present', async () => {
		const { container } = render(Page);
		await waitFor(() => {
			expect(screen.getByText('Filter by source type:')).toBeDefined();
			expect(container.querySelector('#source-type-filter')).not.toBeNull();
		});
	});

	it('template cards show name, description, and field count', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('U.S. Federal Census')).toBeDefined();
			expect(screen.getByText('For U.S. federal census records')).toBeDefined();
			expect(screen.getByText('4 fields, 2 required')).toBeDefined();
			expect(screen.getByText('Birth Certificate')).toBeDefined();
			expect(screen.getByText('For birth certificates')).toBeDefined();
			expect(screen.getByText('3 fields, 2 required')).toBeDefined();
		});
	});

	it('template cards show source type badges', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('census')).toBeDefined();
			expect(screen.getByText('vital_record')).toBeDefined();
		});
	});

	it('expand button shows field details', async () => {
		const { container } = render(Page);
		await waitFor(() => {
			expect(screen.getByText('U.S. Federal Census')).toBeDefined();
		});

		// Find and click the first "Show all fields" button
		const expandButtons = screen.getAllByText('Show all fields');
		await fireEvent.click(expandButtons[0]);

		// Verify field details appear
		await waitFor(() => {
			const fieldDetails = container.querySelector('#fields-census\\.us\\.federal');
			expect(fieldDetails).not.toBeNull();
			expect(screen.getByText('Census Year')).toBeDefined();
			// Check for Required/Optional badges
			const badges = fieldDetails?.querySelectorAll('.field-badge');
			expect(badges?.length).toBeGreaterThan(0);
		});

		// Button text should change
		expect(screen.getByText('Hide fields')).toBeDefined();
	});

	it('error state shows retry button', async () => {
		vi.mocked(apiModule.api.listCitationTemplates).mockRejectedValue(new Error('Network error'));
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Failed to load citation templates. Please try again.')).toBeDefined();
			expect(screen.getByText('Retry')).toBeDefined();
		});

		// Clicking retry should call the API again
		vi.mocked(apiModule.api.listCitationTemplates).mockResolvedValue(mockTemplates);
		await fireEvent.click(screen.getByText('Retry'));
		await waitFor(() => {
			expect(screen.getByText('U.S. Federal Census')).toBeDefined();
		});
	});
});
