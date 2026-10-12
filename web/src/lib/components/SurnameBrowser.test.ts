import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SurnameBrowser from './SurnameBrowser.svelte';
import * as apiModule from '$lib/api/client';
import { currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getSurnameIndex: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

describe('SurnameBrowser', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/browse/surnames');
		vi.mocked(apiModule.api.getSurnameIndex).mockImplementation(async (letter) =>
			letter === 'S'
				? { items: [{ surname: 'Smith', count: 3 }], total: 1 }
				: {
						items: [
							{ surname: 'Adams', count: 1 },
							{ surname: 'Smith', count: 3 }
						],
						total: 2,
						letter_counts: [
							{ letter: 'A', count: 1 },
							{ letter: 'S', count: 1 }
						]
					}
		);
	});

	it('restores the selected letter from the URL (#946)', async () => {
		resetRouter('/browse/surnames?letter=S');
		render(SurnameBrowser);
		await waitFor(() => expect(apiModule.api.getSurnameIndex).toHaveBeenCalledWith('S'));
		await screen.findByText('Smith');
		expect(screen.queryByText('Adams')).toBeNull();
	});

	it('replaces the URL when a letter is selected and cleared (#946)', async () => {
		render(SurnameBrowser);
		await screen.findByText('Adams');

		await fireEvent.click(screen.getByRole('button', { name: /^S/ }));
		await waitFor(() => expect(currentPath()).toBe('/browse/surnames?letter=S'));
		expect(historyLength()).toBe(1);
		await waitFor(() => expect(screen.queryByText('Adams')).toBeNull());

		await fireEvent.click(screen.getByRole('button', { name: /^S/ }));
		await waitFor(() => expect(currentPath()).toBe('/browse/surnames'));
		await screen.findByText('Adams');
	});

	it('clears a failed letter load when the letter is cleared (#946)', async () => {
		resetRouter('/browse/surnames?letter=S');
		vi.mocked(apiModule.api.getSurnameIndex).mockImplementation(async (letter) => {
			if (letter === 'S') throw new Error('boom');
			return {
				items: [{ surname: 'Adams', count: 1 }],
				total: 1,
				letter_counts: [{ letter: 'A', count: 1 }, { letter: 'S', count: 1 }]
			};
		});
		render(SurnameBrowser);
		await screen.findByText('Failed to load surnames. Please try again.');

		await fireEvent.click(screen.getByRole('button', { name: /^S/ }));
		await waitFor(() => expect(currentPath()).toBe('/browse/surnames'));
		await screen.findByText('Adams');
		expect(screen.queryByText('Failed to load surnames. Please try again.')).toBeNull();
	});
});
