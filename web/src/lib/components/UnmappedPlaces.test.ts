import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within, fireEvent } from '@testing-library/svelte';
import UnmappedPlaces from './UnmappedPlaces.svelte';
import * as apiModule from '#lib/api/client.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { listUnmappedPlaces: vi.fn() } };
});

const boston: apiModule.UnmappedPlace = {
	id: 'b0570000-0000-5000-8000-000000000000',
	key: 'boston,ma',
	name: 'Boston',
	spelling: 'Boston, MA',
	count: 3,
	suggestions: [
		{ latitude: 'N42.36', longitude: 'W71.06', count: 2 },
		{ latitude: 'N42.30', longitude: 'W71.00', count: 1 }
	]
};
const salem: apiModule.UnmappedPlace = {
	id: '5a1e0000-0000-5000-8000-000000000000',
	key: 'salem,ma',
	name: 'Salem, MA',
	spelling: 'Salem, MA',
	count: 1,
	suggestions: []
};

describe('UnmappedPlaces', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.listUnmappedPlaces).mockResolvedValue({ items: [boston, salem], total: 2 });
	});

	it('lists each place in order with its count and suggestions', async () => {
		render(UnmappedPlaces);
		await screen.findByText('Boston');
		const places = Array.from(document.querySelectorAll<HTMLElement>('.unmapped-item'));
		expect(places.map((li) => li.querySelector('.unmapped-name')?.textContent)).toEqual(['Boston', 'Salem, MA']);

		const first = within(places[0]);
		expect(first.getByText('3 records without coordinates')).toBeTruthy();
		expect(first.getByText('N42.36, W71.06').parentElement?.textContent).toBe('N42.36, W71.06 (2 records)');
		expect(first.getByText('N42.30, W71.00').parentElement?.textContent).toBe('N42.30, W71.00 (1 record)');

		const second = within(places[1]);
		expect(second.getByText('1 record without coordinates')).toBeTruthy();
		expect(second.getByText('No other record here has coordinates.')).toBeTruthy();
		expect(screen.getByText('2 places without coordinates')).toBeTruthy();
	});

	it('links each entry to the people born or died there by a recorded spelling, not its display name', async () => {
		render(UnmappedPlaces);
		const boston = await screen.findByRole('link', { name: 'People born or died in Boston' });
		expect(boston.getAttribute('href')).toBe('/browse/places/Boston%2C%20MA');
		expect(screen.getByRole('link', { name: 'People born or died in Salem, MA' }).getAttribute('href')).toBe(
			'/browse/places/Salem%2C%20MA'
		);
	});

	it('shows the empty state when every place has coordinates', async () => {
		vi.mocked(apiModule.api.listUnmappedPlaces).mockResolvedValue({ items: [], total: 0 });
		render(UnmappedPlaces);
		expect((await screen.findByTestId('empty-state')).textContent).toContain('Every place has coordinates.');
		expect(screen.queryByRole('link')).toBeNull();
	});

	it('shows a load error with Retry', async () => {
		vi.mocked(apiModule.api.listUnmappedPlaces).mockRejectedValueOnce(new Error('boom'));
		vi.spyOn(console, 'error').mockImplementation(() => {});
		render(UnmappedPlaces);
		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('Failed to load places without coordinates. Please try again.');
		expect(alert.textContent).not.toContain('boom');
		await fireEvent.click(within(alert).getByRole('button', { name: 'Retry' }));
		await screen.findByText('Boston');
	});
});
