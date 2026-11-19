import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ResetPasswordForm from './ResetPasswordForm.svelte';
import * as apiModule from '#lib/api/client.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { resetPassword: vi.fn() } };
});
const { loadAuth } = vi.hoisted(() => ({ loadAuth: vi.fn() }));
vi.mock('#lib/stores/auth.svelte.js', () => ({ loadAuth: () => loadAuth() }));

const api = vi.mocked(apiModule.api);

async function fill(password: string, again = password) {
	await fireEvent.input(screen.getByLabelText('New password'), { target: { value: password } });
	await fireEvent.input(screen.getByLabelText('New password again'), { target: { value: again } });
	await fireEvent.click(screen.getByRole('button', { name: 'Set password' }));
}

describe('ResetPasswordForm', () => {
	beforeEach(() => vi.clearAllMocks());

	it('sets the password and offers sign-in', async () => {
		api.resetPassword.mockResolvedValue(undefined);
		render(ResetPasswordForm, { token: 'tok' });
		await fill('a new password');
		expect(api.resetPassword).toHaveBeenCalledWith({ token: 'tok', password: 'a new password' });
		expect((await screen.findByRole('status')).textContent).toContain('Your password is set');
		expect(screen.getByRole('link', { name: 'Sign in' }).getAttribute('href')).toBe('/');
		expect(loadAuth).toHaveBeenCalled();
	});

	it('shows a used or expired link', async () => {
		api.resetPassword.mockRejectedValue({ status: 400, code: 'INVALID_LINK', message: 'This reset link is wrong, expired or already used.' });
		render(ResetPasswordForm, { token: 'tok' });
		await fill('a new password');
		expect((await screen.findByRole('alert')).textContent).toContain('already used');
	});

	it('refuses passwords that do not match', async () => {
		render(ResetPasswordForm, { token: 'tok' });
		await fill('a new password', 'another one');
		expect(api.resetPassword).not.toHaveBeenCalled();
		expect((await screen.findByRole('alert')).textContent).toContain('do not match');
	});

	it('explains a link without a token', () => {
		render(ResetPasswordForm, { token: '' });
		expect(screen.getByRole('note').textContent).toContain('incomplete');
	});
});
