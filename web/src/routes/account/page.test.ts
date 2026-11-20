import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import AccountPage from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import { auth } from '#lib/stores/access.svelte.js';
import { signInAs } from '#lib/test/roles.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { updateCurrentAccount: vi.fn() } };
});

const api = vi.mocked(apiModule.api);

describe('Account page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		signInAs('editor');
	});

	it('saves a new display name', async () => {
		api.updateCurrentAccount.mockResolvedValue({ id: 'user-editor', username: 'editor', display_name: 'Grace', role: 'editor' });
		render(AccountPage);
		await fireEvent.input(screen.getByLabelText('Display name'), { target: { value: ' Grace ' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save name' }));
		expect(api.updateCurrentAccount).toHaveBeenCalledWith({ display_name: 'Grace' });
		await waitFor(() => expect(auth.account?.display_name).toBe('Grace'));
	});

	it('changes the password with the current one', async () => {
		api.updateCurrentAccount.mockResolvedValue({ id: 'user-editor', username: 'editor', display_name: 'editor', role: 'editor' });
		render(AccountPage);
		await fireEvent.input(screen.getByLabelText('Current password'), { target: { value: 'old password' } });
		await fireEvent.input(screen.getByLabelText('New password'), { target: { value: 'new password!' } });
		await fireEvent.input(screen.getByLabelText('New password again'), { target: { value: 'new password!' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Change password' }));
		expect(api.updateCurrentAccount).toHaveBeenCalledWith({ current_password: 'old password', new_password: 'new password!' });
		await waitFor(() => expect((screen.getByLabelText('Current password') as HTMLInputElement).value).toBe(''));
	});

	it('refuses mismatched passwords and shows a wrong current password', async () => {
		render(AccountPage);
		await fireEvent.input(screen.getByLabelText('Current password'), { target: { value: 'wrong' } });
		await fireEvent.input(screen.getByLabelText('New password'), { target: { value: 'new password!' } });
		await fireEvent.input(screen.getByLabelText('New password again'), { target: { value: 'different!!' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Change password' }));
		expect(api.updateCurrentAccount).not.toHaveBeenCalled();
		expect((await screen.findByRole('alert')).textContent).toContain('do not match');

		api.updateCurrentAccount.mockRejectedValue({ status: 400, code: 'INVALID_CREDENTIALS', message: 'The current password is wrong' });
		await fireEvent.input(screen.getByLabelText('New password again'), { target: { value: 'new password!' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Change password' }));
		await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('current password is wrong'));
	});

	it('explains open mode', () => {
		auth.phase = 'open';
		auth.account = null;
		render(AccountPage);
		expect(screen.getByRole('note').textContent).toContain('AUTH_MODE=open');
	});

	it('offers a visitor the sign-in page, not the open-mode note', async () => {
		auth.phase = 'visitor';
		auth.visitorsWelcome = true;
		auth.account = null;
		render(AccountPage);
		expect(screen.getByRole('note').textContent).toContain('browsing as a visitor');
		expect(screen.queryByText(/AUTH_MODE=open/)).toBeNull();
		await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
		expect(auth.phase).toBe('signed-out');
	});
});
