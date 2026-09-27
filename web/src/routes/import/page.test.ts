import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ImportPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			importGedcomStream: vi.fn(),
			getExportEstimate: vi.fn()
		}
	};
});

// The real store exposes a read-only view, so the active branch is injected here.
const { branchState, returnToMainline } = vi.hoisted(() => ({
	branchState: { id: null as string | null, branch: null as { name: string } | null },
	returnToMainline: vi.fn()
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState,
	returnToMainline
}));

describe('Import & Export page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		branchState.branch = null;
		vi.mocked(apiModule.api.getExportEstimate).mockResolvedValue({
			person_count: 0,
			family_count: 0,
			source_count: 0,
			citation_count: 0,
			event_count: 0,
			note_count: 0,
			total_records: 0,
			estimated_bytes: 0,
			is_large_export: false
		} as unknown as Awaited<ReturnType<typeof apiModule.api.getExportEstimate>>);
	});

	it('offers the GEDCOM upload on the mainline, with no branch notices', () => {
		const { container } = render(ImportPage);
		expect(screen.getByText('Browse Files')).toBeTruthy();
		expect(container.querySelector('input[type="file"]')).toBeTruthy();
		expect(screen.queryByText('Import is unavailable on a research branch')).toBeNull();
		expect(screen.queryByText(/Exports always cover the mainline/)).toBeNull();
	});

	it('withdraws the GEDCOM upload on a research branch (#825)', () => {
		branchState.id = 'b-1';
		branchState.branch = { name: 'Smith hypothesis' };
		const { container } = render(ImportPage);

		expect(screen.getByText('Import is unavailable on a research branch')).toBeTruthy();
		expect(screen.getByRole('button', { name: 'Switch to mainline' })).toBeTruthy();
		// No way to pick or submit a file.
		expect(container.querySelector('input[type="file"]')).toBeNull();
		expect(screen.queryByText('Browse Files')).toBeNull();
		expect(screen.queryByRole('button', { name: /Import File/ })).toBeNull();
		expect(apiModule.api.importGedcomStream).not.toHaveBeenCalled();
	});

	it('labels exports as mainline on a research branch', () => {
		branchState.id = 'b-1';
		render(ImportPage);
		expect(screen.getByText(/Exports always cover the mainline/)).toBeTruthy();
	});
});
