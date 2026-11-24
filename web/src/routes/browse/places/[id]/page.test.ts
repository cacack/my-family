import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import { back, currentPath, historyLength, resetRouter } from '#lib/test/fakeRouter.svelte.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getPersonsByPlace: vi.fn(), getPlace: vi.fn() } };
});

vi.mock('$app/state', async () => (await import('#lib/test/fakeRouter.svelte.js')).appState);
vi.mock('$app/navigation', async () => (await import('#lib/test/fakeRouter.svelte.js')).appNavigation);

const OHIO = '22222222-2222-5222-8222-222222222222';
const person = { id: 'p-1', given_name: 'Ada', surname: 'Lovelace', version: 1 } as apiModule.Person;

describe('Browse place detail page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getPersonsByPlace).mockResolvedValue({ items: [person], total: 45 });
		vi.mocked(apiModule.api.getPlace).mockResolvedValue({ id: OHIO, key: 'ohio', name: 'Ohio', count: 45, version: 0 });
	});

	it('restores the page from the URL (#946)', async () => {
		resetRouter(`/browse/places/${OHIO}?page=2`, { id: OHIO });
		render(Page);
		await waitFor(() =>
			expect(apiModule.api.getPersonsByPlace).toHaveBeenCalledWith(OHIO, { limit: 20, offset: 20 })
		);
		await screen.findByText('Page 2 of 3');
	});

	it('pushes a page change, so Back returns to the previous page (#946)', async () => {
		resetRouter(`/browse/places/${OHIO}`, { id: OHIO });
		render(Page);
		await screen.findByText('Page 1 of 3');
		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		await waitFor(() => expect(currentPath()).toBe(`/browse/places/${OHIO}?page=2`));
		expect(historyLength()).toBe(2);
		await screen.findByText('Page 2 of 3');

		back();
		await screen.findByText('Page 1 of 3');
	});

	it('moves a page past the end to the last page, replacing the URL (#948)', async () => {
		resetRouter(`/browse/places/${OHIO}?page=9`, { id: OHIO });
		render(Page);
		await waitFor(() => expect(currentPath()).toBe(`/browse/places/${OHIO}?page=3`));
		expect(historyLength()).toBe(1);
		await screen.findByText('Page 3 of 3');
		expect(apiModule.api.getPersonsByPlace).toHaveBeenLastCalledWith(OHIO, { limit: 20, offset: 40 });
	});

	it('names the place it reads by ID (#1737)', async () => {
		resetRouter(`/browse/places/${OHIO}`, { id: OHIO });
		render(Page);
		await screen.findByRole('heading', { name: 'People from "Ohio"' });
		expect(apiModule.api.getPlace).toHaveBeenCalledWith(OHIO);
	});
});
