import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getPersonsByPlace: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

const person = { id: 'p-1', given_name: 'Ada', surname: 'Lovelace', version: 1 } as apiModule.Person;

describe('Browse place detail page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getPersonsByPlace).mockResolvedValue({ items: [person], total: 45 });
	});

	it('restores the page from the URL (#946)', async () => {
		resetRouter('/browse/places/Ohio?page=2', { place: 'Ohio' });
		render(Page);
		await waitFor(() =>
			expect(apiModule.api.getPersonsByPlace).toHaveBeenCalledWith('Ohio', { limit: 20, offset: 20 })
		);
		await screen.findByText('Page 2 of 3');
	});

	it('pushes a page change, so Back returns to the previous page (#946)', async () => {
		resetRouter('/browse/places/Ohio', { place: 'Ohio' });
		render(Page);
		await screen.findByText('Page 1 of 3');
		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		await waitFor(() => expect(currentPath()).toBe('/browse/places/Ohio?page=2'));
		expect(historyLength()).toBe(2);
		await screen.findByText('Page 2 of 3');

		back();
		await screen.findByText('Page 1 of 3');
	});

	it('moves a page past the end to the last page, replacing the URL (#948)', async () => {
		resetRouter('/browse/places/Ohio?page=9', { place: 'Ohio' });
		render(Page);
		await waitFor(() => expect(currentPath()).toBe('/browse/places/Ohio?page=3'));
		expect(historyLength()).toBe(1);
		await screen.findByText('Page 3 of 3');
		expect(apiModule.api.getPersonsByPlace).toHaveBeenLastCalledWith('Ohio', { limit: 20, offset: 40 });
	});
});
