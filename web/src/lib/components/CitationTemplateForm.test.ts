import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import CitationTemplateForm from './CitationTemplateForm.svelte';
import * as apiModule from '$lib/api/client';

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listCitationTemplates: vi.fn(),
			getCitationTemplate: vi.fn(),
			previewCitationTemplate: vi.fn()
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

const mockPreview = {
	full: 'U.S. Federal Census, 1900, Ohio, Hamilton County',
	short: 'Census, 1900',
	validation_issues: []
};

describe('CitationTemplateForm', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.clearAllMocks();
		vi.mocked(apiModule.api.listCitationTemplates).mockResolvedValue(mockTemplates);
		vi.mocked(apiModule.api.previewCitationTemplate).mockResolvedValue(mockPreview);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('renders loading state initially', () => {
		// Don't resolve the promise yet
		vi.mocked(apiModule.api.listCitationTemplates).mockReturnValue(new Promise(() => {}));
		render(CitationTemplateForm);
		expect(screen.getByText('Loading templates...')).toBeDefined();
	});

	it('renders template selector after load', async () => {
		render(CitationTemplateForm);
		await waitFor(() => {
			expect(screen.getByText('Citation Template')).toBeDefined();
			expect(screen.getByText('Select a template...')).toBeDefined();
		});
	});

	it('shows error state on API failure', async () => {
		vi.mocked(apiModule.api.listCitationTemplates).mockRejectedValue(new Error('Network error'));
		render(CitationTemplateForm);
		await waitFor(() => {
			expect(screen.getByText('Failed to load citation templates')).toBeDefined();
		});
	});

	// For tests that need a selected template, we use the templateId prop.
	// The component auto-selects the matching template when data loads.
	// This avoids interacting with the bits-ui Select dropdown which is
	// difficult to drive programmatically in jsdom.

	it('selecting a template renders its fields', async () => {
		render(CitationTemplateForm, { props: { templateId: 'census.us.federal' } });
		await waitFor(() => {
			expect(screen.getByText('Census Year')).toBeDefined();
			expect(screen.getByText('State')).toBeDefined();
			expect(screen.getByText('County')).toBeDefined();
			expect(screen.getByText('Notes')).toBeDefined();
		});
	});

	it('required fields have asterisk indicator', async () => {
		const { container } = render(CitationTemplateForm, {
			props: { templateId: 'census.us.federal' }
		});

		await waitFor(() => {
			const requiredMarkers = container.querySelectorAll(
				'span.text-destructive[aria-hidden="true"]'
			);
			// Census Year and State are required
			expect(requiredMarkers.length).toBe(2);
			for (const marker of requiredMarkers) {
				expect(marker.textContent).toBe('*');
			}
		});
	});

	it('notes field renders as textarea', async () => {
		const { container } = render(CitationTemplateForm, {
			props: { templateId: 'census.us.federal' }
		});

		await waitFor(() => {
			const notesField = container.querySelector('#template-field-notes');
			expect(notesField).not.toBeNull();
			expect(notesField?.tagName.toLowerCase()).toBe('textarea');
		});
	});

	it('non-notes fields render as input', async () => {
		const { container } = render(CitationTemplateForm, {
			props: { templateId: 'census.us.federal' }
		});

		await waitFor(() => {
			const yearField = container.querySelector('#template-field-year');
			expect(yearField).not.toBeNull();
			expect(yearField?.tagName.toLowerCase()).toBe('input');
		});
	});

	it('clear button removes template selection', async () => {
		const { container } = render(CitationTemplateForm, {
			props: { templateId: 'census.us.federal' }
		});

		await waitFor(() => {
			expect(screen.getByText('Census Year')).toBeDefined();
		});

		// Click clear button
		const clearButton = screen.getByLabelText('Clear template selection');
		await fireEvent.click(clearButton);

		// Fields should disappear
		await waitFor(() => {
			expect(container.querySelector('#template-field-year')).toBeNull();
		});
	});

	it('calls onchange when fields are updated', async () => {
		const changeHandler = vi.fn();
		const { container } = render(CitationTemplateForm, {
			props: { templateId: 'census.us.federal', onchange: changeHandler }
		});

		await waitFor(() => {
			expect(container.querySelector('#template-field-year')).not.toBeNull();
		});

		// Type in a field
		const yearField = container.querySelector('#template-field-year') as HTMLInputElement;
		await fireEvent.input(yearField, { target: { value: '1900' } });

		expect(changeHandler).toHaveBeenCalledWith(
			'census.us.federal',
			expect.objectContaining({ year: '1900' })
		);
	});

	it('debounces preview request', async () => {
		const { container } = render(CitationTemplateForm, {
			props: { templateId: 'census.us.federal' }
		});

		// Wait for template to load and initial preview debounce to settle
		await waitFor(() => {
			expect(container.querySelector('#template-field-year')).not.toBeNull();
		});
		await vi.advanceTimersByTimeAsync(300);
		vi.mocked(apiModule.api.previewCitationTemplate).mockClear();

		// Type in a field
		const yearField = container.querySelector('#template-field-year') as HTMLInputElement;
		await fireEvent.input(yearField, { target: { value: '1900' } });

		// Should not be called immediately
		expect(apiModule.api.previewCitationTemplate).not.toHaveBeenCalled();

		// Advance past debounce
		await vi.advanceTimersByTimeAsync(300);

		expect(apiModule.api.previewCitationTemplate).toHaveBeenCalledWith(
			'census.us.federal',
			expect.objectContaining({ year: '1900' })
		);
	});

	it('shows preview with full and short citations', async () => {
		const { container } = render(CitationTemplateForm, {
			props: { templateId: 'census.us.federal' }
		});

		// Wait for template fields to render
		await waitFor(() => {
			expect(container.querySelector('#template-field-year')).not.toBeNull();
		});

		// Type in a field to trigger preview via handleFieldChange -> schedulePreview
		const yearField = container.querySelector('#template-field-year') as HTMLInputElement;
		await fireEvent.input(yearField, { target: { value: '1900' } });

		// Advance past debounce
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			expect(screen.getByText('Full Citation')).toBeDefined();
			expect(screen.getByText('U.S. Federal Census, 1900, Ohio, Hamilton County')).toBeDefined();
			expect(screen.getByText('Short Citation')).toBeDefined();
			expect(screen.getByText('Census, 1900')).toBeDefined();
		});
	});

	it('shows validation issues from preview', async () => {
		const previewWithIssues = {
			full: 'Incomplete citation',
			short: 'Incomplete',
			validation_issues: [
				{ field: 'year', message: 'Year is required', level: 'error' as const },
				{ field: 'state', message: 'State recommended', level: 'warning' as const }
			]
		};
		vi.mocked(apiModule.api.previewCitationTemplate).mockResolvedValue(previewWithIssues);

		const { container } = render(CitationTemplateForm, {
			props: { templateId: 'census.us.federal' }
		});

		// Wait for template fields to render
		await waitFor(() => {
			expect(container.querySelector('#template-field-year')).not.toBeNull();
		});

		// Type in a field to trigger preview
		const yearField = container.querySelector('#template-field-year') as HTMLInputElement;
		await fireEvent.input(yearField, { target: { value: 'test' } });
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			expect(screen.getByText('Issues')).toBeDefined();
			expect(screen.getByText('year: Year is required')).toBeDefined();
			expect(screen.getByText('state: State recommended')).toBeDefined();
		});
	});
});
