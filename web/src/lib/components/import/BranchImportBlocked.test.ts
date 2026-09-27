import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import BranchImportBlocked from './BranchImportBlocked.svelte';

const { branchState, returnToMainline } = vi.hoisted(() => ({
	branchState: { id: 'b-1' as string | null, branch: null as { name: string } | null },
	returnToMainline: vi.fn()
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState,
	returnToMainline
}));

describe('BranchImportBlocked', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = 'b-1';
		branchState.branch = null;
	});

	it('explains that import writes the mainline', () => {
		render(BranchImportBlocked);
		expect(screen.getByText('Import is unavailable on a research branch')).toBeTruthy();
		expect(screen.getByRole('note').textContent).toContain('always writes to the mainline');
		expect(screen.getByRole('note').textContent).toContain('a research branch');
	});

	it('names the active branch once it is loaded', () => {
		branchState.branch = { name: 'Smith hypothesis' };
		render(BranchImportBlocked);
		expect(screen.getByRole('note').textContent).toContain('"Smith hypothesis"');
	});

	it('offers a switch back to the mainline', async () => {
		render(BranchImportBlocked);
		await fireEvent.click(screen.getByRole('button', { name: 'Switch to mainline' }));
		expect(returnToMainline).toHaveBeenCalledTimes(1);
	});
});
