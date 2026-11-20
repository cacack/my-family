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

function entry(action: AuthAuditEntry['action'], extra: Partial<AuthAuditEntry> = {}): AuthAuditEntry {
	return { at: '2026-01-01T12:00:00Z', action, username: 'grace', detail: '', client_ip: '192.0.2.1', ...extra };
}

describe('Audit log page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		signInAs('owner');
	});

	it('lists entries newest first and pages back', async () => {
		api.listAuthAudit
			.mockResolvedValueOnce({
				items: [
					entry('role_changed', { actor_id: 'a', actor_name: 'Ada', detail: 'editor -> viewer' }),
					entry('sign_in_failed')
				],
				total: 3
			})
			.mockResolvedValueOnce({ items: [entry('user_created', { client_ip: 'cli' })], total: 3 });
		render(AuditPage);
		await waitFor(() => expect(screen.getAllByTestId('audit-entry')).toHaveLength(2));
		const [changed, failed] = screen.getAllByTestId('audit-entry');
		expect(changed.textContent).toContain('Role changed');
		expect(changed.textContent).toContain('Ada');
		expect(changed.textContent).toContain('editor -> viewer');
		expect(failed.textContent).toContain('Sign-in failed');
		expect(failed.textContent).toContain('Not signed in');
		expect(screen.getByText('Showing 2 of 3')).toBeDefined();

		await fireEvent.click(screen.getByRole('button', { name: 'Show older entries' }));
		expect(api.listAuthAudit).toHaveBeenLastCalledWith({ limit: 50, offset: 2 });
		await waitFor(() => expect(screen.getAllByTestId('audit-entry')).toHaveLength(3));
		expect(screen.getAllByTestId('audit-entry')[2].textContent).toContain('Command line');
		expect(screen.queryByRole('button', { name: 'Show older entries' })).toBeNull();
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
});
