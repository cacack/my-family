/**
 * The pedigree and descendancy pages keep their toolbar options (generations,
 * layout) in the URL (#901). Both pages share the behaviour, so one table covers them.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import PedigreePage from './pedigree/[id]/+page.svelte';
import DescendancyPage from './descendancy/[id]/+page.svelte';
import * as apiModule from '$lib/api/client';
import { currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getPedigree: vi.fn(), getDescendancy: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

const ROOT = { id: 'p-1', given_name: 'Ada', surname: 'Lovelace', generation: 0 };

describe.each([
	{ name: 'pedigree', Page: PedigreePage, load: () => apiModule.api.getPedigree },
	{ name: 'descendancy', Page: DescendancyPage, load: () => apiModule.api.getDescendancy }
])('$name chart options in the URL', ({ name, Page, load }) => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getPedigree).mockResolvedValue({
			root: ROOT
		} as unknown as apiModule.Pedigree);
		vi.mocked(apiModule.api.getDescendancy).mockResolvedValue({
			root: ROOT
		} as unknown as apiModule.Descendancy);
	});

	const generations = () => screen.getByLabelText('Generations:') as HTMLSelectElement;
	const layoutButton = (label: string) => screen.getByTitle(`${label} layout`);

	it('reads generations and layout from the URL', async () => {
		resetRouter(`/${name}/p-1?gens=7&layout=wide`, { id: 'p-1' });
		render(Page);
		await waitFor(() => expect(load()).toHaveBeenCalledWith('p-1', 7));
		expect(generations().value).toBe('7');
		expect(layoutButton('Wide').classList.contains('active')).toBe(true);
	});

	it('defaults to 4 generations, compact, with a clean URL', async () => {
		resetRouter(`/${name}/p-1`, { id: 'p-1' });
		render(Page);
		await waitFor(() => expect(load()).toHaveBeenCalledWith('p-1', 4));
		expect(layoutButton('Compact').classList.contains('active')).toBe(true);
	});

	it('replaces the URL on option changes and reloads only for generations', async () => {
		resetRouter(`/${name}/p-1`, { id: 'p-1' });
		render(Page);
		await waitFor(() => expect(load()).toHaveBeenCalledTimes(1));

		await fireEvent.change(generations(), { target: { value: '6' } });
		await waitFor(() => expect(currentPath()).toBe(`/${name}/p-1?gens=6`));
		await waitFor(() => expect(load()).toHaveBeenLastCalledWith('p-1', 6));

		await fireEvent.click(layoutButton('Standard'));
		await waitFor(() => expect(currentPath()).toBe(`/${name}/p-1?gens=6&layout=standard`));
		expect(layoutButton('Standard').classList.contains('active')).toBe(true);
		expect(load()).toHaveBeenCalledTimes(2);
		expect(historyLength()).toBe(1);

		// Back to the default layout drops the param again.
		await fireEvent.click(layoutButton('Compact'));
		await waitFor(() => expect(currentPath()).toBe(`/${name}/p-1?gens=6`));
	});
});
