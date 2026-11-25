import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import PlaceBrowser from './PlaceBrowser.svelte';
import * as apiModule from '#lib/api/client.js';
import { back, currentPath, historyLength, resetRouter } from '#lib/test/fakeRouter.svelte.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getPlaceHierarchy: vi.fn() } };
});

vi.mock('$app/state', async () => (await import('#lib/test/fakeRouter.svelte.js')).appState);
vi.mock('$app/navigation', async () => (await import('#lib/test/fakeRouter.svelte.js')).appNavigation);

const USA = '11111111-1111-5111-8111-111111111111';
const OHIO = '22222222-2222-5222-8222-222222222222';
const COLUMBUS = '33333333-3333-5333-8333-333333333333';
const usa = { id: USA, name: 'USA', full_name: 'USA', count: 5, has_children: true };
const ohio = { id: OHIO, name: 'Ohio', full_name: 'Ohio, USA', count: 5, has_children: true };
const columbus = { id: COLUMBUS, name: 'Columbus', full_name: 'Columbus, Ohio, USA', count: 2, has_children: false };

describe('PlaceBrowser', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/browse/places');
		vi.mocked(apiModule.api.getPlaceHierarchy).mockImplementation(async (parent) => {
			if (parent === OHIO) return { items: [columbus], total: 1, breadcrumb: [usa, ohio] };
			if (parent === USA) return { items: [ohio], total: 1, breadcrumb: [usa] };
			return { items: [usa], total: 1 };
		});
	});

	it('opens the place whose ID the URL names (#946, #1737)', async () => {
		resetRouter(`/browse/places?parent=${OHIO}`);
		render(PlaceBrowser);
		await screen.findByText('Columbus');
		expect(apiModule.api.getPlaceHierarchy).toHaveBeenCalledWith(OHIO);
		expect(screen.getByRole('link', { name: /Columbus/ }).getAttribute('href')).toBe(`/browse/places/${COLUMBUS}`);
		expect(screen.getByRole('link', { name: 'View all people from Ohio' }).getAttribute('href')).toBe(
			`/browse/places/${OHIO}`
		);
	});

	it('pushes a drill-down by ID, so Back returns to the parent level (#946)', async () => {
		render(PlaceBrowser);
		await fireEvent.click(await screen.findByText('USA'));
		await waitFor(() => expect(currentPath()).toBe(`/browse/places?parent=${USA}`));
		expect(historyLength()).toBe(2);
		await screen.findByText('Ohio');

		back();
		await screen.findByText('Click to view sub-locations');
		expect(currentPath()).toBe('/browse/places');
	});

	it('names each breadcrumb and browses to it by ID', async () => {
		resetRouter(`/browse/places?parent=${OHIO}`);
		render(PlaceBrowser);
		await screen.findByText('Columbus');
		await fireEvent.click(screen.getByRole('button', { name: 'USA' }));
		await waitFor(() => expect(currentPath()).toBe(`/browse/places?parent=${USA}`));
		await screen.findByText('Ohio');
	});

	it('shows a parent from before places had IDs as not found, with a way back (#1737)', async () => {
		resetRouter('/browse/places?parent=Ohio');
		vi.mocked(apiModule.api.getPlaceHierarchy).mockRejectedValue({
			status: 400,
			code: 'BAD_REQUEST',
			message: 'Invalid format for parameter parent'
		});
		render(PlaceBrowser);
		await screen.findByText('This place link is out of date or wrong.');
		expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
		expect(screen.getByRole('link', { name: 'Browse all places' }).getAttribute('href')).toBe('/browse/places');
	});

	it('offers Retry when the places fail to load', async () => {
		vi.mocked(apiModule.api.getPlaceHierarchy).mockRejectedValue({ status: 500, code: 'INTERNAL', message: 'boom' });
		render(PlaceBrowser);
		await screen.findByText('Failed to load the places. Please try again.');
		expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy();
	});
});
