import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import CemeteryBrowser from './CemeteryBrowser.svelte';
import * as apiModule from '$lib/api/client';
import { currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getCemeteryIndex: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

describe('CemeteryBrowser', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/browse/cemeteries');
		vi.mocked(apiModule.api.getCemeteryIndex).mockResolvedValue({
			items: [
				{ place: 'Green Lawn Cemetery', count: 2 },
				{ place: 'Oak Hill', count: 1 }
			],
			total: 2
		});
	});

	it('restores the filter from the URL (#946)', async () => {
		resetRouter('/browse/cemeteries?q=oak');
		render(CemeteryBrowser);
		await screen.findByText('Oak Hill');
		expect(screen.queryByText('Green Lawn Cemetery')).toBeNull();
		expect((screen.getByRole('textbox') as HTMLInputElement).value).toBe('oak');
	});

	it('replaces the URL as the filter changes (#946)', async () => {
		render(CemeteryBrowser);
		const box = (await screen.findByRole('textbox')) as HTMLInputElement;
		await fireEvent.input(box, { target: { value: 'green' } });
		await waitFor(() => expect(currentPath()).toBe('/browse/cemeteries?q=green'));
		expect(historyLength()).toBe(1);
		expect(screen.queryByText('Oak Hill')).toBeNull();

		await fireEvent.input(box, { target: { value: '' } });
		await waitFor(() => expect(currentPath()).toBe('/browse/cemeteries'));
	});
});
