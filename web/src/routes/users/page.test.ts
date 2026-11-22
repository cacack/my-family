import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import UsersPage from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import type { PendingInvite, PendingPasswordReset, User } from '#lib/api/client.js';
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

function pendingInvite(id: string, by: User = owner): PendingInvite {
	return {
		id,
		role: 'viewer',
		created_by: by.id,
		created_by_name: by.display_name,
		created_at: '2026-01-01T00:00:00Z',
		expires_at: '2026-01-08T00:00:00Z'
	};
}

function pendingReset(id: string): PendingPasswordReset {
	return {
		id,
		user_id: grace.id,
		user_name: grace.username,
		created_by: owner.id,
		created_by_name: owner.display_name,
		created_at: '2026-01-01T00:00:00Z',
		expires_at: '2026-01-02T00:00:00Z'
	};
}

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
		api.createInvite.mockResolvedValue({ id: 'tok-hash', token: 'tok', expires_at: '2026-01-08T00:00:00Z' });
		render(UsersPage);
		api.listInvites.mockResolvedValue([pendingInvite('tok-hash')]);
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
		api.createPasswordReset.mockResolvedValue({ id: 'reset-hash', token: 'reset-tok', expires_at: '2026-01-02T00:00:00Z' });
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		api.listPasswordResets.mockResolvedValue([pendingReset('reset-hash')]);
		await fireEvent.click(screen.getByRole('button', { name: 'Reset password for Grace' }));
		const link = (await screen.findByLabelText('Password reset link for Grace')) as HTMLInputElement;
		expect(link.value).toBe(`${window.location.origin}/reset-password#token=reset-tok`);
		await waitFor(() => expect(api.listPasswordResets).toHaveBeenCalledTimes(2));
	});

	it('clears an invite or reset link box once its link is revoked', async () => {
		api.createInvite.mockResolvedValue({ id: 'invite-hash', token: 'tok', expires_at: '2026-01-08T00:00:00Z' });
		api.createPasswordReset.mockResolvedValue({ id: 'reset-hash', token: 'reset-tok', expires_at: '2026-01-02T00:00:00Z' });
		api.revokeInvite.mockResolvedValue(undefined);
		api.revokePasswordReset.mockResolvedValue(undefined);
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		// Issuing each link reloads Pending links, which then lists both.
		api.listInvites.mockResolvedValue([pendingInvite('invite-hash')]);
		api.listPasswordResets.mockResolvedValue([pendingReset('reset-hash')]);
		await fireEvent.click(screen.getByRole('button', { name: 'Create invite link' }));
		await screen.findByLabelText('Invite link (viewer)');
		await fireEvent.click(screen.getByRole('button', { name: 'Reset password for Grace' }));
		await screen.findByLabelText('Password reset link for Grace');
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(2));

		await fireEvent.click(screen.getByRole('button', { name: /^Revoke viewer invite/ }));
		await waitFor(() => expect(screen.queryByLabelText('Invite link (viewer)')).toBeNull());
		expect(screen.getByLabelText('Password reset link for Grace')).toBeDefined();

		await fireEvent.click(screen.getByRole('button', { name: /^Revoke password reset for grace/i }));
		await waitFor(() => expect(screen.queryByLabelText('Password reset link for Grace')).toBeNull());
	});

	it('keeps a link box when a different link is revoked', async () => {
		api.createInvite.mockResolvedValue({ id: 'invite-hash', token: 'tok', expires_at: '2026-01-08T00:00:00Z' });
		api.listInvites.mockResolvedValue([pendingInvite('other-hash')]);
		api.revokeInvite.mockResolvedValue(undefined);
		render(UsersPage);
		await screen.findByRole('button', { name: /^Revoke viewer invite/ });
		api.listInvites.mockResolvedValue([pendingInvite('other-hash'), pendingInvite('invite-hash')]);
		await fireEvent.click(screen.getByRole('button', { name: 'Create invite link' }));
		await screen.findByLabelText('Invite link (viewer)');
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(2));
		await fireEvent.click(screen.getAllByRole('button', { name: /^Revoke viewer invite/ })[0]);
		await waitFor(() => expect(api.revokeInvite).toHaveBeenCalledWith('other-hash'));
		await waitFor(() => expect(screen.queryAllByTestId('pending-link')).toHaveLength(1));
		expect(screen.getByLabelText('Invite link (viewer)')).toBeDefined();
	});

	it('reloads the pending links after a role change or a disable', async () => {
		api.listInvites.mockResolvedValue([pendingInvite('grace-invite', grace)]);
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		expect(await screen.findByText(/Created by Grace/)).toBeDefined();

		// The server revokes the links Grace issued when she is demoted.
		api.listInvites.mockResolvedValue([]);
		api.updateUser.mockResolvedValueOnce({ ...grace, role: 'viewer' });
		await fireEvent.change(within(row('Grace')).getByLabelText('Role'), { target: { value: 'viewer' } });
		await waitFor(() => expect(screen.queryByText(/Created by Grace/)).toBeNull());
		expect(api.listInvites).toHaveBeenCalledTimes(2);

		api.updateUser.mockResolvedValueOnce({ ...grace, role: 'viewer', disabled: true });
		await fireEvent.click(screen.getByRole('button', { name: 'Disable Grace' }));
		await fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Disable' }));
		await waitFor(() => expect(api.listInvites).toHaveBeenCalledTimes(3));
	});

	it('clears the link boxes whose links a reload finds gone', async () => {
		api.createInvite.mockResolvedValue({ id: 'invite-hash', token: 'tok', expires_at: '2026-01-08T00:00:00Z' });
		api.createPasswordReset.mockResolvedValue({ id: 'reset-hash', token: 'reset-tok', expires_at: '2026-01-02T00:00:00Z' });
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		api.listInvites.mockResolvedValue([pendingInvite('invite-hash')]);
		api.listPasswordResets.mockResolvedValue([pendingReset('reset-hash')]);
		await fireEvent.click(screen.getByRole('button', { name: 'Create invite link' }));
		await screen.findByLabelText('Invite link (viewer)');
		await fireEvent.click(screen.getByRole('button', { name: 'Reset password for Grace' }));
		await screen.findByLabelText('Password reset link for Grace');
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(2));

		// Ada demotes herself while another owner exists: the server deletes the links she issued.
		api.listInvites.mockResolvedValue([]);
		api.listPasswordResets.mockResolvedValue([]);
		api.updateUser.mockResolvedValueOnce({ ...owner, role: 'editor' });
		await fireEvent.change(within(row('Ada')).getByLabelText('Role'), { target: { value: 'editor' } });
		await waitFor(() => expect(screen.queryByLabelText('Invite link (viewer)')).toBeNull());
		expect(screen.queryByLabelText('Password reset link for Grace')).toBeNull();
	});

	it('keeps a link box issued after a reload started', async () => {
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		await waitFor(() => expect(screen.getByText('No invite links are pending.')).toBeDefined());

		// A role change starts a reload that answers only after the invite is issued.
		let answerOlder!: (invites: PendingInvite[]) => void;
		api.listInvites.mockReturnValueOnce(new Promise((resolve) => (answerOlder = resolve)));
		api.updateUser.mockResolvedValueOnce({ ...grace, role: 'viewer' });
		await fireEvent.change(within(row('Grace')).getByLabelText('Role'), { target: { value: 'viewer' } });
		await waitFor(() => expect(api.listInvites).toHaveBeenCalledTimes(2));

		api.createInvite.mockResolvedValue({ id: 'invite-hash', token: 'tok', expires_at: '2026-01-08T00:00:00Z' });
		api.listInvites.mockResolvedValue([pendingInvite('invite-hash')]);
		await fireEvent.click(screen.getByRole('button', { name: 'Create invite link' }));
		await screen.findByLabelText('Invite link (viewer)');
		answerOlder([]);
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(1));
		expect(screen.getByLabelText('Invite link (viewer)')).toBeDefined();
	});

	it('keeps the pending links when a role change is refused', async () => {
		api.updateUser.mockRejectedValueOnce({ status: 409, code: 'LAST_OWNER', message: "This is the tree's last active owner." });
		render(UsersPage);
		await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
		await waitFor(() => expect(api.listInvites).toHaveBeenCalledTimes(1));
		await fireEvent.change(within(row('Ada')).getByLabelText('Role'), { target: { value: 'viewer' } });
		await within(row('Ada')).findByRole('alert');
		expect(api.listInvites).toHaveBeenCalledTimes(1);
	});

	it('is for owners only', () => {
		signInAs('editor');
		render(UsersPage);
		expect(screen.getByTestId('role-notice').textContent).toContain('Managing users needs the owner role');
		expect(api.listUsers).not.toHaveBeenCalled();
		expect(api.listInvites).not.toHaveBeenCalled();
	});

	it('shows a visitor the role notice, not the open-mode note', () => {
		auth.phase = 'visitor';
		auth.account = null;
		render(UsersPage);
		expect(screen.getByTestId('role-notice').textContent).toContain('browsing as a visitor');
		expect(screen.queryByText(/AUTH_MODE=open/)).toBeNull();
		expect(api.listUsers).not.toHaveBeenCalled();
	});

	it('explains open mode instead of loading', () => {
		auth.phase = 'open';
		auth.account = null;
		render(UsersPage);
		expect(screen.getByRole('note').textContent).toContain('AUTH_MODE=open');
		expect(api.listUsers).not.toHaveBeenCalled();
	});
});
