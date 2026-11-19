import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import AccountMenu from './AccountMenu.svelte';

const { state, signOut } = vi.hoisted(() => ({
	state: { phase: 'signed-in', account: null as null | { id: string; username: string; display_name: string } },
	signOut: vi.fn()
}));
vi.mock('#lib/stores/auth.svelte.js', () => ({ auth: state, signOut: () => signOut() }));

describe('AccountMenu', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		state.account = { id: '1', username: 'ada', display_name: 'Ada Lovelace' };
	});

	it('names the signed-in user and signs out', async () => {
		render(AccountMenu);
		const trigger = screen.getByRole('button', { name: 'Account: Ada Lovelace' });
		await fireEvent.pointerDown(trigger, { button: 0, pointerType: 'mouse' });
		const item = await screen.findByRole('menuitem', { name: 'Sign out' });
		expect(screen.getByText('ada')).toBeDefined();
		await fireEvent.click(item);
		expect(signOut).toHaveBeenCalledOnce();
	});

	it('renders nothing without an account (open mode)', () => {
		state.account = null;
		render(AccountMenu);
		expect(screen.queryByRole('button')).toBeNull();
	});
});
