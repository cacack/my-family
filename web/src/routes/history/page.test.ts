import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '#lib/api/client.js';
import { auth } from '#lib/stores/access.svelte.js';
import { signInAs } from '#lib/test/roles.js';

const { getGlobalHistory } = vi.hoisted(() => ({ getGlobalHistory: vi.fn() }));

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: { getGlobalHistory: (params: unknown) => getGlobalHistory(params) }
	};
});

describe('Change History page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getGlobalHistory.mockResolvedValue({ items: [], total: 0, has_more: false });
	});

	it('loads the change history', async () => {
		render(Page);

		expect(screen.getByRole('heading', { level: 1, name: 'Change History' })).toBeTruthy();
		await vi.waitFor(() => expect(getGlobalHistory).toHaveBeenCalled());
		expect(screen.queryByRole('button', { name: 'Sign in' })).toBeNull();
	});

	it('asks a visitor to sign in instead of loading what the server refuses them', async () => {
		auth.phase = 'visitor';
		render(Page);

		expect(screen.getByRole('heading', { level: 1, name: 'Change History' })).toBeTruthy();
		expect(screen.getByTestId('empty-state').textContent).toContain('Sign in to see the change history');
		expect(getGlobalHistory).not.toHaveBeenCalled();
		await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
		expect(auth.phase).toBe('signed-out');
	});

	it('offers research snapshots to a signed-in user', () => {
		signInAs('viewer');
		render(Page);

		expect(screen.getByRole('link', { name: 'Research snapshots' }).getAttribute('href')).toBe(
			'/snapshots'
		);
	});

	it('does not send a visitor on to research snapshots, which also need signing in', () => {
		auth.phase = 'visitor';
		render(Page);

		expect(screen.queryByRole('link', { name: 'Research snapshots' })).toBeNull();
	});
});
