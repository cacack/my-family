import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getBrickWalls: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

const active = { person_id: 'p-1', person_name: 'Ada Active', note: 'No parents', since: '2026-01-01T00:00:00Z' };
const resolved = { ...active, person_id: 'p-2', person_name: 'Rex Resolved', resolved_at: '2026-02-01T00:00:00Z' };

describe('Brick Walls page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/browse/brick-walls');
		vi.mocked(apiModule.api.getBrickWalls).mockImplementation(async (includeResolved) => ({
			items: includeResolved ? [active, resolved] : [active],
			active_count: 1,
			resolved_count: 1
		}));
	});

	it('restores the resolved filter from the URL (#946)', async () => {
		resetRouter('/browse/brick-walls?resolved=1');
		render(Page);
		await screen.findByText('Rex Resolved');
		expect(apiModule.api.getBrickWalls).toHaveBeenCalledWith(true);
		expect((screen.getByRole('checkbox', { name: 'Show resolved brick walls' }) as HTMLInputElement).checked).toBe(true);
	});

	it('replaces the URL when the resolved filter toggles (#946)', async () => {
		render(Page);
		await screen.findByText('Ada Active');
		expect(screen.queryByText('Rex Resolved')).toBeNull();

		await fireEvent.click(screen.getByRole('checkbox', { name: 'Show resolved brick walls' }));
		await waitFor(() => expect(currentPath()).toBe('/browse/brick-walls?resolved=1'));
		expect(historyLength()).toBe(1);
		await screen.findByText('Rex Resolved');

		await fireEvent.click(screen.getByRole('checkbox', { name: 'Show resolved brick walls' }));
		await waitFor(() => expect(currentPath()).toBe('/browse/brick-walls'));
	});
});
