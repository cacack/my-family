import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import CitationSection from './CitationSection.svelte';
import * as apiModule from '#lib/api/client.js';
import { PERSON_FACT_TYPES } from '#lib/utils/evidence.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getPersonCitations: vi.fn(),
			searchSources: vi.fn(),
			createCitation: vi.fn(),
			deleteCitation: vi.fn()
		}
	};
});

describe('CitationSection', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getPersonCitations).mockResolvedValue({ citations: [], total: 0 });
	});

	// The API rejects any fact type outside domain.FactType, and a person's
	// citation must name a person fact.
	it('offers only person fact types the API accepts, defaulting to birth', async () => {
		render(CitationSection, { props: { personId: 'p-1' } });
		await fireEvent.click(await screen.findByRole('button', { name: 'Add Citation' }));

		const select = screen.getByLabelText('Fact Type') as HTMLSelectElement;
		const values = Array.from(select.options).map((o) => o.value);
		expect(values).toEqual(PERSON_FACT_TYPES);
		expect(values).not.toContain('general');
		expect(values.every((v) => v.startsWith('person_'))).toBe(true);
		expect(select.value).toBe('person_birth');
	});

	// A GEDCOM PAGE can span CONT lines; a text input would strip the breaks (#1174).
	it('edits the page in a textarea that keeps line breaks', async () => {
		render(CitationSection, { props: { personId: 'p-1' } });
		await fireEvent.click(await screen.findByRole('button', { name: 'Add Citation' }));

		const page = screen.getByLabelText('Page') as HTMLTextAreaElement;
		expect(page.tagName).toBe('TEXTAREA');
		await fireEvent.input(page, { target: { value: 'Folio 12\nEntry 3' } });
		expect(page.value).toBe('Folio 12\nEntry 3');
	});

	// Only a stale version reads as "updated elsewhere"; a refusal keeps its message (#988).
	describe('a 409 on delete', () => {
		const citation = {
			id: 'c-1',
			source_id: 's-1',
			source_title: 'Parish register',
			fact_type: 'person_birth',
			fact_owner_id: 'p-1',
			version: 1
		} as apiModule.Citation;

		async function deleteIt() {
			vi.mocked(apiModule.api.getPersonCitations).mockResolvedValue({ citations: [citation], total: 1 });
			render(CitationSection, { props: { personId: 'p-1' } });
			await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));
			await fireEvent.click(screen.getByRole('button', { name: 'Yes, Delete' }));
		}

		it('shows the server message for a refusal, with no retry', async () => {
			const message = 'The citation is referenced by a proof summary';
			vi.mocked(apiModule.api.deleteCitation).mockRejectedValue({ status: 409, code: 'CONFLICT', message });
			await deleteIt();

			expect(await screen.findByRole('alert')).toHaveProperty('textContent', message);
			expect(screen.queryByRole('button', { name: 'Try Again' })).toBeNull();
		});

		it('shows the stale-version notice and a retry for a version conflict', async () => {
			vi.mocked(apiModule.api.deleteCitation).mockRejectedValue({
				status: 409,
				code: 'VERSION_CONFLICT',
				message: 'version conflict'
			});
			await deleteIt();

			expect((await screen.findByRole('alert')).textContent).toContain(
				'This record was updated by another operation'
			);
			expect(screen.getByRole('button', { name: 'Try Again' })).toBeTruthy();
		});
	});
});
