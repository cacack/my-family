import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import MainlineNotice from './MainlineNotice.svelte';

const { branchState } = vi.hoisted(() => ({
	branchState: { id: null as string | null }
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState
}));

describe('MainlineNotice', () => {
	beforeEach(() => {
		branchState.id = null;
	});

	it('renders nothing on the mainline', () => {
		render(MainlineNotice, { props: { surface: 'Quality' } });
		expect(screen.queryByRole('note')).toBeNull();
	});

	it('names the surface and the default explanation on a branch', () => {
		branchState.id = 'b-1';
		render(MainlineNotice, { props: { surface: 'Quality' } });
		const note = screen.getByRole('note');
		expect(note.textContent).toContain('Quality always shows mainline data');
		expect(note.textContent).toContain('A branch covers people, families, sources');
	});

	it('uses a custom message verbatim, replacing the default sentence', () => {
		branchState.id = 'b-1';
		render(MainlineNotice, {
			props: { surface: 'Ignored', message: 'Repositories are shared across all branches.' }
		});
		const note = screen.getByRole('note');
		expect(note.textContent?.trim()).toBe('Repositories are shared across all branches.');
		expect(note.textContent).not.toContain('always shows mainline data');
	});
});
