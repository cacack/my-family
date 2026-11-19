import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import AccountMenu from './AccountMenu.svelte';

const { state, signOut, goto } = vi.hoisted(() => ({
	state: {
		phase: 'signed-in',
		account: null as null | { id: string; username: string; display_name: string; role: string }
	},
	signOut: vi.fn(),
	goto: vi.fn()
}));
vi.mock('#lib/stores/auth.svelte.js', () => ({
	auth: state,
	can: (required: string) => required !== 'owner' || state.account?.role === 'owner',
	signOut: () => signOut()
}));
vi.mock('$app/navigation', () => ({ goto: (href: string) => goto(href) }));

async function open() {
	const trigger = screen.getByRole('button', { name: 'Account: Ada Lovelace' });
	await fireEvent.pointerDown(trigger, { button: 0, pointerType: 'mouse' });
}

describe('AccountMenu', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		state.account = { id: '1', username: 'ada', display_name: 'Ada Lovelace', role: 'owner' };
	});

	it('names the signed-in user and signs out', async () => {
		render(AccountMenu);
		await open();
		const item = await screen.findByRole('menuitem', { name: 'Sign out' });
		expect(screen.getByText('ada')).toBeDefined();
		await fireEvent.click(item);
		expect(signOut).toHaveBeenCalledOnce();
	});

	it('opens the account page, and the users page for an owner', async () => {
		render(AccountMenu);
		await open();
		await fireEvent.click(await screen.findByRole('menuitem', { name: 'Users' }));
		expect(goto).toHaveBeenCalledWith('/users');
		await open();
		await fireEvent.click(await screen.findByRole('menuitem', { name: 'Account' }));
		expect(goto).toHaveBeenCalledWith('/account');
	});

	it('offers no users page below owner', async () => {
		state.account = { id: '1', username: 'ada', display_name: 'Ada Lovelace', role: 'editor' };
		render(AccountMenu);
		await open();
		expect(await screen.findByRole('menuitem', { name: 'Account' })).toBeDefined();
		expect(screen.queryByRole('menuitem', { name: 'Users' })).toBeNull();
	});

	it('renders nothing without an account (open mode)', () => {
		state.account = null;
		render(AccountMenu);
		expect(screen.queryByRole('button')).toBeNull();
	});
});
