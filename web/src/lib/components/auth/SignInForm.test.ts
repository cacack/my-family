import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SignInForm from './SignInForm.svelte';

const { signIn, browseAsVisitor, state } = vi.hoisted(() => ({
	signIn: vi.fn(),
	browseAsVisitor: vi.fn(),
	state: { visitorsWelcome: false }
}));
vi.mock('#lib/stores/auth.svelte.js', () => ({
	auth: state,
	signIn: (u: string, p: string) => signIn(u, p),
	browseAsVisitor: () => browseAsVisitor()
}));

async function submit(username: string, password: string) {
	await fireEvent.input(screen.getByLabelText('Username'), { target: { value: username } });
	await fireEvent.input(screen.getByLabelText('Password'), { target: { value: password } });
	await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
}

describe('SignInForm', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		state.visitorsWelcome = false;
	});

	it('offers no way back to browsing when visitors must sign in', () => {
		render(SignInForm);
		expect(screen.queryByRole('button', { name: 'Browse without signing in' })).toBeNull();
	});

	it('goes back to browsing when visitors are welcome', async () => {
		state.visitorsWelcome = true;
		render(SignInForm);
		await fireEvent.click(screen.getByRole('button', { name: 'Browse without signing in' }));
		expect(browseAsVisitor).toHaveBeenCalledOnce();
	});

	it('signs in with the username and password', async () => {
		signIn.mockResolvedValue(undefined);
		render(SignInForm);
		expect(screen.getByRole('heading', { name: 'Sign in' })).toBeDefined();
		await submit('ada', 'secret password');
		expect(signIn).toHaveBeenCalledWith('ada', 'secret password');
	});

	it('uses the browser password manager fields', () => {
		render(SignInForm);
		expect(screen.getByLabelText('Username').getAttribute('autocomplete')).toBe('username');
		const password = screen.getByLabelText('Password');
		expect(password.getAttribute('type')).toBe('password');
		expect(password.getAttribute('autocomplete')).toBe('current-password');
	});

	it('shows why sign-in failed and clears the password', async () => {
		signIn.mockRejectedValue({ status: 401, code: 'INVALID_CREDENTIALS', message: 'The username or password is wrong' });
		render(SignInForm);
		await submit('ada', 'wrong password');
		expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringContaining('The username or password is wrong'));
		await waitFor(() => expect((screen.getByLabelText('Password') as HTMLInputElement).value).toBe(''));
		expect(screen.getByLabelText('Password').getAttribute('aria-describedby')).toBe('signin-error');
	});
});
