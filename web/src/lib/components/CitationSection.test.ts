import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import CitationSection from './CitationSection.svelte';
import * as apiModule from '$lib/api/client';
import { PERSON_FACT_TYPES } from '$lib/utils/evidence';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getPersonCitations: vi.fn(),
			searchSources: vi.fn(),
			createCitation: vi.fn()
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
});
