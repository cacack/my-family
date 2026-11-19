import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import JoinForm from './JoinForm.svelte';

const { acceptInvite, goto, state } = vi.hoisted(() => ({
	acceptInvite: vi.fn(),
	goto: vi.fn(),
	state: { phase: 'signed-out' }
}));
vi.mock('#lib/stores/auth.svelte.js', () => ({ acceptInvite: (input: unknown) => acceptInvite(input), auth: state }));
vi.mock('$app/navigation', () => ({ goto: (href: string) => goto(href) }));

async function fill(fields: Record<string, string>) {
	for (const [label, value] of Object.entries(fields)) {
		await fireEvent.input(screen.getByLabelText(label), { target: { value } });
	}
	await fireEvent.click(screen.getByRole('button', { name: 'Create account and sign in' }));
}

const valid = { Username: 'grace', 'Display name': 'Grace Hopper', Password: 'secret password', 'Password again': 'secret password' };

describe('JoinForm', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		state.phase = 'signed-out';
	});

	it('accepts the invite and opens the tree', async () => {
		acceptInvite.mockResolvedValue(undefined);
		render(JoinForm, { token: 'tok' });
		await fill(valid);
		expect(acceptInvite).toHaveBeenCalledWith({ token: 'tok', username: 'grace', display_name: 'Grace Hopper', password: 'secret password' });
		expect(goto).toHaveBeenCalledWith('/');
	});

	it('shows a used or expired invite', async () => {
		acceptInvite.mockRejectedValue({ status: 400, code: 'INVALID_LINK', message: 'This invite link is wrong, expired or already used.' });
		render(JoinForm, { token: 'tok' });
		await fill(valid);
		expect((await screen.findByRole('alert')).textContent).toContain('already used');
		expect(goto).not.toHaveBeenCalled();
	});

	it('refuses passwords that do not match', async () => {
		render(JoinForm, { token: 'tok' });
		await fill({ ...valid, 'Password again': 'other' });
		expect(acceptInvite).not.toHaveBeenCalled();
	});

	it('explains a link without a token', () => {
		render(JoinForm, { token: '' });
		expect(screen.getByRole('note').textContent).toContain('incomplete');
		expect(screen.queryByRole('button')).toBeNull();
	});
});
