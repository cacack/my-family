import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import SignInRequired from './SignInRequired.svelte';

const { showSignIn } = vi.hoisted(() => ({ showSignIn: vi.fn() }));
vi.mock('#lib/stores/auth.svelte.js', () => ({ showSignIn: () => showSignIn() }));

describe('SignInRequired', () => {
	it('says why and opens the sign-in page', async () => {
		render(SignInRequired, { subject: 'the change history' });
		const text = screen.getByTestId('empty-state').textContent;
		expect(text).toContain('Sign in to see the change history');
		expect(text).toContain('who made each change');
		await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
		expect(showSignIn).toHaveBeenCalledOnce();
	});
});
