import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SourcesPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listSources: vi.fn(),
			createSource: vi.fn()
		}
	};
});

// domain.SourceType: the API rejects every other value.
const API_SOURCE_TYPES = [
	'book', 'archive', 'webpage', 'census', 'vital_record', 'church_record',
	'newspaper', 'photograph', 'interview', 'correspondence', 'other'
];

describe('Sources page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.listSources).mockResolvedValue({ sources: [], total: 0, limit: 20, offset: 0 });
		vi.mocked(apiModule.api.createSource).mockResolvedValue({
			id: 's-1',
			source_type: 'other',
			title: 'Parish register',
			version: 1
		} as apiModule.Source);
	});

	it('offers exactly the source types the API accepts and creates with a valid default', async () => {
		render(SourcesPage);
		await screen.findByText('No sources found.');
		await fireEvent.click(screen.getAllByRole('button', { name: 'Add Source' })[0]);

		const select = screen.getByLabelText('Source Type') as HTMLSelectElement;
		expect(Array.from(select.options).map((o) => o.value).sort()).toEqual([...API_SOURCE_TYPES].sort());

		await fireEvent.input(screen.getByLabelText(/Title/), { target: { value: 'Parish register' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Create Source' }));

		await waitFor(() => expect(apiModule.api.createSource).toHaveBeenCalled());
		const body = vi.mocked(apiModule.api.createSource).mock.calls[0][0];
		expect(API_SOURCE_TYPES).toContain(body.source_type);
	});
});
