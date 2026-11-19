import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import SetupForm from './SetupForm.svelte';

const { completeSetup } = vi.hoisted(() => ({ completeSetup: vi.fn() }));
vi.mock('#lib/stores/auth.svelte.js', () => ({ completeSetup: (input: unknown) => completeSetup(input) }));

async function fill(fields: Record<string, string>) {
	for (const [label, value] of Object.entries(fields)) {
		await fireEvent.input(screen.getByLabelText(label), { target: { value } });
	}
	await fireEvent.click(screen.getByRole('button', { name: 'Create owner and sign in' }));
}

const valid = {
	'Setup token': '  0123abcd  ',
	Username: 'ada',
	'Display name': 'Ada Lovelace',
	Password: 'secret password',
	'Password again': 'secret password'
};

describe('SetupForm', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates the owner with the token from the server log', async () => {
		completeSetup.mockResolvedValue(undefined);
		render(SetupForm);
		expect(screen.getByRole('heading', { name: 'Set up My Family' })).toBeDefined();
		await fill(valid);
		expect(completeSetup).toHaveBeenCalledWith({
			token: '0123abcd',
			username: 'ada',
			display_name: 'Ada Lovelace',
			password: 'secret password'
		});
	});

	it('defaults the display name to the username', async () => {
		completeSetup.mockResolvedValue(undefined);
		render(SetupForm);
		await fill({ ...valid, 'Display name': ' ' });
		expect(completeSetup).toHaveBeenCalledWith(expect.objectContaining({ display_name: 'ada' }));
	});

	it('refuses passwords that do not match', async () => {
		render(SetupForm);
		await fill({ ...valid, 'Password again': 'something else' });
		expect(completeSetup).not.toHaveBeenCalled();
		expect((await screen.findByRole('alert')).textContent).toContain('do not match');
	});

	it('shows a refused token', async () => {
		completeSetup.mockRejectedValue({ status: 403, code: 'INVALID_SETUP_TOKEN', message: 'The setup token is wrong. Copy it from the server log.' });
		render(SetupForm);
		await fill(valid);
		expect((await screen.findByRole('alert')).textContent).toContain('The setup token is wrong');
	});

	it('says where the token is', () => {
		render(SetupForm);
		expect(screen.getByLabelText('Setup token').getAttribute('aria-describedby')).toBe('setup-token-hint');
		expect(screen.getByText(/myfamily user create/)).toBeDefined();
	});
});
