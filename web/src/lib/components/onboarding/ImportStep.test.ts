import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import ImportStep from './ImportStep.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			importGedcom: vi.fn()
		}
	};
});

const { branchState, returnToMainline } = vi.hoisted(() => ({
	branchState: { id: null as string | null, branch: null as { name: string } | null },
	returnToMainline: vi.fn()
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState,
	returnToMainline
}));

function gedFile(): File {
	return new File(['0 HEAD\n0 TRLR\n'], 'tree.ged', { type: 'text/plain' });
}

describe('Onboarding ImportStep', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		branchState.branch = null;
		vi.mocked(apiModule.api.importGedcom).mockResolvedValue({
			success: true,
			persons_imported: 2,
			families_imported: 1
		} as apiModule.ImportResult);
	});

	it('imports a selected file on the mainline', async () => {
		const { container } = render(ImportStep, {
			props: { onComplete: vi.fn(), onBack: vi.fn() }
		});
		const input = container.querySelector('input[type="file"]') as HTMLInputElement;
		expect(input).toBeTruthy();
		await fireEvent.change(input, { target: { files: [gedFile()] } });
		await fireEvent.click(screen.getByRole('button', { name: /Import File/ }));
		await waitFor(() => expect(screen.getByText('Import Successful!')).toBeTruthy());
		expect(apiModule.api.importGedcom).toHaveBeenCalledTimes(1);
	});

	it('withdraws the upload on a research branch and offers the mainline (#825)', async () => {
		branchState.id = 'b-1';
		const onBack = vi.fn();
		const { container } = render(ImportStep, {
			props: { onComplete: vi.fn(), onBack }
		});

		expect(screen.getByText('Import is unavailable on a research branch')).toBeTruthy();
		expect(container.querySelector('input[type="file"]')).toBeNull();
		expect(screen.queryByRole('button', { name: /Import File/ })).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: 'Switch to mainline' }));
		expect(returnToMainline).toHaveBeenCalledTimes(1);
		expect(apiModule.api.importGedcom).not.toHaveBeenCalled();

		// The way back out of the step still works.
		await fireEvent.click(screen.getByText(/Back/));
		expect(onBack).toHaveBeenCalledTimes(1);
	});
});
