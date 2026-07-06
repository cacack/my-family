import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import * as apiModule from '$lib/api/client';
import ExportVersionSelect from './ExportVersionSelect.svelte';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			previewGedcomExport: vi.fn()
		}
	};
});

describe('ExportVersionSelect', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('does not call preview and shows no warning for the default (auto) version', async () => {
		render(ExportVersionSelect, { props: { value: 'auto' } });
		expect(apiModule.api.previewGedcomExport).not.toHaveBeenCalled();
		expect(screen.queryByRole('alert')).toBeNull();
	});

	it('fetches a preview and renders a data-loss warning for a downgrade version', async () => {
		vi.mocked(apiModule.api.previewGedcomExport).mockResolvedValue({
			sourceVersion: '7.0',
			targetVersion: '5.5.1',
			hasDataLoss: true,
			dataLoss: [
				{ feature: 'EXID tags', reason: 'Tag not supported in GEDCOM 5.5.1', affectedRecords: ['@I1@'] }
			]
		});

		render(ExportVersionSelect, { props: { value: '5.5.1' } });

		await waitFor(() => {
			expect(apiModule.api.previewGedcomExport).toHaveBeenCalledWith('5.5.1');
			expect(screen.getByText('EXID tags')).toBeDefined();
			expect(screen.getByText(/Tag not supported in GEDCOM 5\.5\.1/)).toBeDefined();
		});
	});

	it('shows no warning when the target version loses no data', async () => {
		vi.mocked(apiModule.api.previewGedcomExport).mockResolvedValue({
			sourceVersion: '7.0',
			targetVersion: '7.0',
			hasDataLoss: false,
			dataLoss: []
		});

		render(ExportVersionSelect, { props: { value: '7.0' } });

		await waitFor(() => {
			expect(apiModule.api.previewGedcomExport).toHaveBeenCalledWith('7.0');
		});
		expect(screen.queryByRole('alert')).toBeNull();
	});

	it('surfaces an error when the preview request fails', async () => {
		vi.mocked(apiModule.api.previewGedcomExport).mockRejectedValue(new Error('Network error'));

		render(ExportVersionSelect, { props: { value: '5.5' } });

		await waitFor(() => {
			expect(screen.getByText('Network error')).toBeDefined();
		});
	});
});
