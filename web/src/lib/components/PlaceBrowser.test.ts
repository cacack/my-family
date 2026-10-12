import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import PlaceBrowser from './PlaceBrowser.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getPlaceHierarchy: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

describe('PlaceBrowser', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/browse/places');
		vi.mocked(apiModule.api.getPlaceHierarchy).mockImplementation(async (parent) =>
			parent === 'Ohio, USA'
				? { items: [{ name: 'Columbus', full_name: 'Columbus, Ohio, USA', count: 2 }], total: 1, breadcrumb: ['Ohio', 'USA'] }
				: { items: [{ name: 'Ohio', full_name: 'Ohio, USA', count: 5, has_children: true }], total: 1, breadcrumb: [] }
		);
	});

	it('opens the place the URL names (#946)', async () => {
		resetRouter('/browse/places?parent=Ohio%2C+USA');
		render(PlaceBrowser);
		await screen.findByText('Columbus');
		expect(apiModule.api.getPlaceHierarchy).toHaveBeenCalledWith('Ohio, USA');
	});

	it('pushes a drill-down, so Back returns to the parent level (#946)', async () => {
		render(PlaceBrowser);
		await fireEvent.click(await screen.findByText('Ohio'));
		await waitFor(() => expect(currentPath()).toBe('/browse/places?parent=Ohio%2C+USA'));
		expect(historyLength()).toBe(2);
		await screen.findByText('Columbus');

		back();
		await screen.findByText('Click to view sub-locations');
		expect(currentPath()).toBe('/browse/places');
	});
});
