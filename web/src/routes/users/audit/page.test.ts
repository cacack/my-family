import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import AuditPage from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import type { AuthAuditEntry } from '#lib/api/client.js';
import { signInAs } from '#lib/test/roles.js';
import { auth } from '#lib/stores/access.svelte.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { listAuthAudit: vi.fn() } };
});

const api = vi.mocked(apiModule.api);

function entry(
	action: AuthAuditEntry['action'],
	extra: Partial<AuthAuditEntry> = {}
): AuthAuditEntry {
	return {
		at: '2026-01-01T12:00:00Z',
		action,
		username: 'grace',
		detail: '',
		client_ip: '192.0.2.1',
		...extra
	};
}

describe('Audit log page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		signInAs('owner');
	});

	it('lists entries newest first', async () => {
		api.listAuthAudit.mockResolvedValueOnce({
			items: [
				entry('role_changed', {
					actor_id: 'a',
					actor_name: 'Ada',
					detail: 'editor -> viewer'
				}),
				entry('sign_in_failed')
			]
		});
		render(AuditPage);
		await waitFor(() => expect(screen.getAllByTestId('audit-entry')).toHaveLength(2));
		expect(api.listAuthAudit).toHaveBeenCalledWith({ limit: 50 });
		const [changed, failed] = screen.getAllByTestId('audit-entry');
		expect(changed.textContent).toContain('Role changed');
		expect(changed.textContent).toContain('Ada');
		expect(changed.textContent).toContain('editor -> viewer');
		expect(failed.textContent).toContain('Sign-in failed');
		expect(failed.textContent).toContain('Not signed in');
		// One page in all: nothing to page through.
		expect(screen.queryByRole('navigation', { name: 'Audit log pages' })).toBeNull();
	});

	it('pages older and newer by cursor', async () => {
		api.listAuthAudit
			.mockResolvedValueOnce({
				items: [entry('role_changed')],
				next_cursor: 'c1'
			})
			.mockResolvedValueOnce({ items: [entry('sign_in')], next_cursor: 'c2' })
			.mockResolvedValueOnce({
				items: [entry('user_created', { client_ip: 'cli' })]
			})
			.mockResolvedValueOnce({ items: [entry('sign_in')], next_cursor: 'c2' });
		render(AuditPage);
		const newer = await screen.findByRole('button', { name: 'Newer' });
		const older = screen.getByRole('button', { name: 'Older' });
		expect(screen.getByText('Page 1')).toBeDefined();
		expect((newer as HTMLButtonElement).disabled).toBe(true);

		await fireEvent.click(older);
		await waitFor(() => expect(screen.getByText('Page 2')).toBeDefined());
		expect(api.listAuthAudit).toHaveBeenLastCalledWith({
			limit: 50,
			cursor: 'c1'
		});
		expect(screen.getByTestId('audit-entry').textContent).toContain('Signed in');

		await fireEvent.click(older);
		await waitFor(() => expect(screen.getByText('Page 3')).toBeDefined());
		expect(api.listAuthAudit).toHaveBeenLastCalledWith({
			limit: 50,
			cursor: 'c2'
		});
		expect(screen.getByTestId('audit-entry').textContent).toContain('Command line');
		expect((older as HTMLButtonElement).disabled).toBe(true);

		await fireEvent.click(newer);
		await waitFor(() => expect(screen.getByText('Page 2')).toBeDefined());
		expect(api.listAuthAudit).toHaveBeenLastCalledWith({
			limit: 50,
			cursor: 'c1'
		});
		expect((older as HTMLButtonElement).disabled).toBe(false);
	});

	it('keeps the page on screen when another fails to load', async () => {
		api.listAuthAudit
			.mockResolvedValueOnce({
				items: [entry('role_changed')],
				next_cursor: 'c1'
			})
			.mockRejectedValueOnce({ message: 'Server unavailable' });
		render(AuditPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Older' }));
		await waitFor(() => expect(screen.getByText('Server unavailable')).toBeDefined());
		expect(screen.getByText('Page 1')).toBeDefined();
		expect(screen.getByTestId('audit-entry').textContent).toContain('Role changed');
	});

	it('is for owners only', () => {
		signInAs('viewer');
		render(AuditPage);
		expect(screen.getByTestId('role-notice')).toBeDefined();
		expect(api.listAuthAudit).not.toHaveBeenCalled();
	});

	it('shows a visitor the role notice, not the open-mode note', () => {
		auth.phase = 'visitor';
		auth.account = null;
		render(AuditPage);
		expect(screen.getByTestId('role-notice').textContent).toContain('browsing as a visitor');
		expect(screen.queryByText(/AUTH_MODE=open/)).toBeNull();
		expect(api.listAuthAudit).not.toHaveBeenCalled();
	});

	it('explains open mode instead of loading', () => {
		auth.phase = 'open';
		auth.account = null;
		render(AuditPage);
		expect(screen.getByRole('note').textContent).toContain('AUTH_MODE=open');
		expect(api.listAuthAudit).not.toHaveBeenCalled();
	});
});
