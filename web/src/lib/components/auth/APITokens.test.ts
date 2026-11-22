import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import APITokens from './APITokens.svelte';
import * as apiModule from '#lib/api/client.js';
import type { APIToken } from '#lib/api/client.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: { listAPITokens: vi.fn(), createAPIToken: vi.fn(), revokeAPIToken: vi.fn() }
	};
});

const api = vi.mocked(apiModule.api);

const used: APIToken = {
	id: 'token-used',
	name: 'Nightly backup',
	created_at: '2026-01-01T00:00:00Z',
	last_used_at: '2026-01-02T00:00:00Z'
};
const unused: APIToken = { id: 'token-unused', name: 'Import script', created_at: '2026-01-03T00:00:00Z' };

function row(name: string): HTMLElement {
	return screen.getAllByTestId('api-token').find((r) => r.textContent?.includes(name)) as HTMLElement;
}

describe('APITokens', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		api.listAPITokens.mockResolvedValue([unused, used]);
	});

	it('lists each token with when it was created and last used, never a secret', async () => {
		render(APITokens);
		await waitFor(() => expect(screen.getAllByTestId('api-token')).toHaveLength(2));
		expect(row('Nightly backup').textContent).toContain('Last used');
		expect(row('Import script').textContent).toContain('Never used.');
		expect(screen.queryByLabelText(/New token/)).toBeNull();
	});

	it('says when there are none', async () => {
		api.listAPITokens.mockResolvedValue([]);
		render(APITokens);
		expect(await screen.findByText('You have no API tokens.')).toBeDefined();
	});

	it('creates a token and shows its secret once', async () => {
		const announce = vi.fn();
		api.createAPIToken.mockResolvedValue({ id: 'token-new', name: 'Report', created_at: '2026-01-04T00:00:00Z', token: 'mf_secret' });
		render(APITokens, { announce });
		await waitFor(() => expect(screen.getAllByTestId('api-token')).toHaveLength(2));
		const create = screen.getByRole('button', { name: 'Create token' });
		expect((create as HTMLButtonElement).disabled).toBe(true);

		api.listAPITokens.mockResolvedValue([{ id: 'token-new', name: 'Report', created_at: '2026-01-04T00:00:00Z' }, unused, used]);
		await fireEvent.input(screen.getByLabelText('Token name'), { target: { value: ' Report ' } });
		await fireEvent.click(create);
		expect(api.createAPIToken).toHaveBeenCalledWith('Report');
		const secret = (await screen.findByLabelText('New token: Report')) as HTMLInputElement;
		expect(secret.value).toBe('mf_secret');
		expect(screen.getByText(/not shown again/)).toBeDefined();
		await waitFor(() => expect(screen.getAllByTestId('api-token')).toHaveLength(3));
		expect((screen.getByLabelText('Token name') as HTMLInputElement).value).toBe('');
		expect(announce).toHaveBeenCalledWith(expect.stringContaining('Report created'));
	});

	it('shows why a token could not be created', async () => {
		api.createAPIToken.mockRejectedValue({ status: 400, message: 'token name must be 1 to 100 characters' });
		render(APITokens);
		await fireEvent.input(screen.getByLabelText('Token name'), { target: { value: 'x' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Create token' }));
		expect((await screen.findByRole('alert')).textContent).toContain('1 to 100 characters');
	});

	it('revokes a token, and drops one already revoked elsewhere', async () => {
		const announce = vi.fn();
		api.revokeAPIToken.mockResolvedValueOnce(undefined).mockRejectedValueOnce({ status: 404, message: 'gone' });
		render(APITokens, { announce });
		await waitFor(() => expect(screen.getAllByTestId('api-token')).toHaveLength(2));

		await fireEvent.click(screen.getByRole('button', { name: 'Revoke API token Nightly backup' }));
		expect(api.revokeAPIToken).toHaveBeenCalledWith('token-used');
		await waitFor(() => expect(screen.getAllByTestId('api-token')).toHaveLength(1));
		expect(announce).toHaveBeenCalledWith('API token Nightly backup revoked');

		await fireEvent.click(screen.getByRole('button', { name: 'Revoke API token Import script' }));
		expect(await screen.findByText('You have no API tokens.')).toBeDefined();
		expect(announce).toHaveBeenCalledWith('API token Import script was already revoked');
	});

	it('keeps a token whose revoke failed, with the reason', async () => {
		api.revokeAPIToken.mockRejectedValue({ status: 500, message: 'Server error' });
		render(APITokens);
		await waitFor(() => expect(screen.getAllByTestId('api-token')).toHaveLength(2));
		await fireEvent.click(screen.getByRole('button', { name: 'Revoke API token Nightly backup' }));
		await waitFor(() => expect(within(row('Nightly backup')).getByRole('alert').textContent).toContain('Server error'));
		expect(screen.getAllByTestId('api-token')).toHaveLength(2);
	});

	it('offers a retry when the list fails to load', async () => {
		api.listAPITokens.mockRejectedValueOnce({ status: 500, message: 'boom' });
		render(APITokens);
		expect(await screen.findByText('Failed to load your API tokens. Please try again.')).toBeDefined();
		await fireEvent.click(screen.getByRole('button', { name: /retry|try again/i }));
		await waitFor(() => expect(screen.getAllByTestId('api-token')).toHaveLength(2));
	});
});
