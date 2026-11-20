import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import UsersPage from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import type { User } from '#lib/api/client.js';
import { auth } from '#lib/stores/access.svelte.js';
import { signInAs } from '#lib/test/roles.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listUsers: vi.fn(),
			updateUser: vi.fn(),
			createInvite: vi.fn(),
			createPasswordReset: vi.fn(),
			listInvites: vi.fn(),
			listPasswordResets: vi.fn(),
			revokeInvite: vi.fn(),
			revokePasswordReset: vi.fn()
		}
	};
});

const api = vi.mocked(apiModule.api);

const owner: User = {
	id: 'user-owner',
	username: 'owner',
	display_name: 'Ada',
	role: 'owner',
	disabled: false,
	created_at: '2026-01-01T00:00:00Z'
};
const grace: User = { ...owner, id: 'user-grace', username: 'grace', display_name: 'Grace', role: 'editor' };

function row(name: string): HTMLElement {
	return screen.getAllByTestId('user-row').find((r) => r.textContent?.includes(name)) as HTMLElement;
}

describe('Users page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		signInAs('owner');
		api.listUsers.mockResolvedValue([owner, grace]);
		api.listInvites.mockResolvedValue([]);
		api.listPasswordResets.mockResolvedValue([]);
	});

	it('lists the users with their roles, marking the signed-in one', async () => {
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		expect(row('Ada').textContent).toContain('(you)');
		expect((within(row('Grace')).getByLabelText('Role') as HTMLSelectElement).value).toBe('editor');
		expect(screen.getByRole('link', { name: 'Audit log' }).getAttribute('href')).toBe('/users/audit');
	});

	it('creates an invite link with the chosen role', async () => {
		api.createInvite.mockResolvedValue({ token: 'tok', expires_at: '2026-01-08T00:00:00Z' });
		render(UsersPage);
		await fireEvent.change(screen.getByLabelText('Role', { selector: '#invite-role' }), { target: { value: 'editor' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Create invite link' }));
		expect(api.createInvite).toHaveBeenCalledWith('editor');
		const link = (await screen.findByLabelText('Invite link (editor)')) as HTMLInputElement;
		expect(link.value).toBe(`${window.location.origin}/join#token=tok`);
		await waitFor(() => expect(api.listInvites).toHaveBeenCalledTimes(2));
	});

	it('lists the pending links', async () => {
		api.listInvites.mockResolvedValue([
			{
				id: 'h',
				role: 'viewer',
				created_by: 'user-owner',
				created_by_name: 'Ada',
				created_at: '2026-01-01T00:00:00Z',
				expires_at: '2026-01-08T00:00:00Z'
			}
		]);
		render(UsersPage);
		expect(await screen.findByText('Viewer invite')).toBeDefined();
		expect(screen.getByRole('heading', { name: 'Pending links' })).toBeDefined();
	});

	it('changes a role, and shows a refusal with the role put back', async () => {
		api.updateUser.mockResolvedValueOnce({ ...grace, role: 'viewer' });
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		const graceRole = within(row('Grace')).getByLabelText('Role') as HTMLSelectElement;
		await fireEvent.change(graceRole, { target: { value: 'viewer' } });
		expect(api.updateUser).toHaveBeenCalledWith('user-grace', { role: 'viewer' });
		await waitFor(() => expect(graceRole.value).toBe('viewer'));

		api.updateUser.mockRejectedValueOnce({ status: 409, code: 'LAST_OWNER', message: "This is the tree's last active owner." });
		const adaRole = within(row('Ada')).getByLabelText('Role') as HTMLSelectElement;
		await fireEvent.change(adaRole, { target: { value: 'viewer' } });
		expect((await within(row('Ada')).findByRole('alert')).textContent).toContain('last active owner');
		expect(adaRole.value).toBe('owner');
	});

	it('disables after confirming, and re-enables', async () => {
		api.updateUser.mockResolvedValueOnce({ ...grace, disabled: true });
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		await fireEvent.click(screen.getByRole('button', { name: 'Disable Grace' }));
		const dialog = await screen.findByRole('alertdialog');
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Disable' }));
		expect(api.updateUser).toHaveBeenCalledWith('user-grace', { disabled: true });
		await waitFor(() => expect(within(row('Grace')).getByText('Disabled')).toBeDefined());

		api.updateUser.mockResolvedValueOnce(grace);
		await fireEvent.click(screen.getByRole('button', { name: 'Re-enable Grace' }));
		expect(api.updateUser).toHaveBeenLastCalledWith('user-grace', { disabled: false });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Disable Grace' })).toBeDefined());
	});

	it('shows a password reset link for the user', async () => {
		api.createPasswordReset.mockResolvedValue({ token: 'reset-tok', expires_at: '2026-01-02T00:00:00Z' });
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		await fireEvent.click(screen.getByRole('button', { name: 'Reset password for Grace' }));
		const link = (await screen.findByLabelText('Password reset link for Grace')) as HTMLInputElement;
		expect(link.value).toBe(`${window.location.origin}/reset-password#token=reset-tok`);
		await waitFor(() => expect(api.listPasswordResets).toHaveBeenCalledTimes(2));
	});

	it('is for owners only', () => {
		signInAs('editor');
		render(UsersPage);
		expect(screen.getByTestId('role-notice').textContent).toContain('Managing users needs the owner role');
		expect(api.listUsers).not.toHaveBeenCalled();
		expect(api.listInvites).not.toHaveBeenCalled();
	});

	it('explains open mode instead of loading', () => {
		auth.phase = 'open';
		auth.account = null;
		render(UsersPage);
		expect(screen.getByRole('note').textContent).toContain('AUTH_MODE=open');
		expect(api.listUsers).not.toHaveBeenCalled();
	});
});
