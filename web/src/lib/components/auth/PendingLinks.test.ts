import { tick } from 'svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import PendingLinks from './PendingLinks.svelte';
import * as apiModule from '#lib/api/client.js';
import type { PendingInvite, PendingPasswordReset } from '#lib/api/client.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listInvites: vi.fn(),
			listPasswordResets: vi.fn(),
			revokeInvite: vi.fn(),
			revokePasswordReset: vi.fn()
		}
	};
});

const api = vi.mocked(apiModule.api);

const invite: PendingInvite = {
	id: 'invite-hash',
	role: 'editor',
	created_by: 'user-owner',
	created_by_name: 'Ada',
	created_at: '2026-01-01T00:00:00Z',
	expires_at: '2026-01-08T00:00:00Z'
};
const reset: PendingPasswordReset = {
	id: 'reset-hash',
	user_id: 'user-grace',
	user_name: 'Grace',
	created_by: 'user-owner',
	created_by_name: 'Ada',
	created_at: '2026-01-01T00:00:00Z',
	expires_at: '2026-01-02T00:00:00Z'
};
const cliReset: PendingPasswordReset = {
	...reset,
	id: 'cli-hash',
	created_by: undefined,
	created_by_name: undefined
};

/** A promise the test resolves when it chooses. */
function deferred<T>() {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((r) => (resolve = r));
	return { promise, resolve };
}

function item(text: string): HTMLElement {
	return screen.getAllByTestId('pending-link').find((r) => r.textContent?.includes(text)) as HTMLElement;
}

describe('PendingLinks', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		api.listInvites.mockResolvedValue([invite]);
		api.listPasswordResets.mockResolvedValue([reset, cliReset]);
	});

	it('lists pending invites and resets with their issuer, never a link', async () => {
		render(PendingLinks);
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(3));
		const invites = screen.getByRole('list', { name: 'Invites' });
		expect(invites.textContent).toContain('Editor invite');
		expect(invites.textContent).toContain('Created by Ada');
		const resets = screen.getByRole('list', { name: 'Password resets' });
		expect(within(resets).getAllByText('Password reset for Grace')).toHaveLength(2);
		expect(resets.textContent).toContain('Created by the command line');
		expect(document.body.textContent).not.toContain('invite-hash');
		expect(screen.queryByRole('textbox')).toBeNull();
	});

	it('says when nothing is pending', async () => {
		api.listInvites.mockResolvedValue([]);
		api.listPasswordResets.mockResolvedValue([]);
		render(PendingLinks);
		expect(await screen.findByText('No invite links are pending.')).toBeDefined();
		expect(screen.getByText('No password reset links are pending.')).toBeDefined();
	});

	it('revokes an invite and a reset, and announces each', async () => {
		const announce = vi.fn();
		const onrevoke = vi.fn();
		api.revokeInvite.mockResolvedValue(undefined);
		api.revokePasswordReset.mockResolvedValue(undefined);
		render(PendingLinks, { announce, onrevoke });
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(3));

		await fireEvent.click(
			within(item('Editor invite')).getByRole('button', { name: /^Revoke editor invite/ })
		);
		expect(api.revokeInvite).toHaveBeenCalledWith('invite-hash');
		await waitFor(() => expect(screen.getByText('No invite links are pending.')).toBeDefined());
		expect(announce).toHaveBeenCalledWith('Editor invite revoked');
		expect(onrevoke).toHaveBeenCalledWith('invite-hash');

		await fireEvent.click(
			within(item('Created by Ada')).getByRole('button', { name: /^Revoke password reset for grace/ })
		);
		expect(api.revokePasswordReset).toHaveBeenCalledWith('reset-hash');
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(1));
		expect(announce).toHaveBeenCalledWith('Password reset for Grace revoked');
		expect(onrevoke).toHaveBeenLastCalledWith('reset-hash');
	});

	it('reloads when a link was no longer pending', async () => {
		api.revokeInvite.mockRejectedValue({
			status: 404,
			code: 'NOT_FOUND',
			message: 'No such invite is pending'
		});
		const onrevoke = vi.fn();
		render(PendingLinks, { onrevoke });
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(3));
		api.listInvites.mockResolvedValue([]);
		await fireEvent.click(within(item('Editor invite')).getByRole('button', { name: /^Revoke/ }));
		expect(await screen.findByText('No invite links are pending.')).toBeDefined();
		expect(screen.queryByRole('alert')).toBeNull();
		await waitFor(() => expect(onrevoke).toHaveBeenCalledWith('invite-hash'));
	});

	it('shows a failed revoke beside the link', async () => {
		const onrevoke = vi.fn();
		api.revokeInvite.mockRejectedValue({ status: 500, message: '' });
		render(PendingLinks, { onrevoke });
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(3));
		await fireEvent.click(within(item('Editor invite')).getByRole('button', { name: /^Revoke/ }));
		const alert = await within(item('Editor invite')).findByRole('alert');
		expect(alert.textContent).toContain('Could not revoke the link');
		expect(onrevoke).not.toHaveBeenCalled();
	});

	it('offers a retry when the links fail to load', async () => {
		api.listInvites.mockRejectedValueOnce({ status: 500, message: 'boom' });
		render(PendingLinks);
		await fireEvent.click(await screen.findByRole('button', { name: /retry/i }));
		await waitFor(() => expect(screen.getAllByTestId('pending-link')).toHaveLength(3));
	});

	it('shows the later reload when two overlap and resolve out of order', async () => {
		const older = deferred<PendingInvite[]>();
		const newer = deferred<PendingInvite[]>();
		api.listInvites.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
		const { component } = render(PendingLinks);
		component.reload();
		newer.resolve([]);
		await waitFor(() => expect(screen.getByText('No invite links are pending.')).toBeDefined());
		older.resolve([invite]);
		// Let the older reload settle and render, if it were to.
		await new Promise((r) => setTimeout(r, 0));
		await tick();
		expect(screen.getByText('No invite links are pending.')).toBeDefined();
		expect(screen.queryByText('Editor invite')).toBeNull();
	});

	it('passes on the shown links a reload finds gone, as read when it started', async () => {
		const pending = deferred<PendingInvite[]>();
		api.listInvites.mockReturnValueOnce(pending.promise);
		const onrevoke = vi.fn();
		let shown = ['gone-hash', 'invite-hash'];
		render(PendingLinks, { onrevoke, shown: () => shown });
		// Issued after the reload started: not judged by it.
		shown = [...shown, 'later-hash'];
		pending.resolve([invite]);
		await waitFor(() => expect(onrevoke).toHaveBeenCalledWith('gone-hash'));
		expect(onrevoke).toHaveBeenCalledTimes(1);
	});
});
