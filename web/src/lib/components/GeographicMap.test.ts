import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import GeographicMap from './GeographicMap.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getMapLocations: vi.fn() } };
});

// The real bundled outline, unless a test makes it fail
const atlas = vi.hoisted(() => ({ fail: false }));
vi.mock('$lib/utils/worldAtlas', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/utils/worldAtlas')>();
	return {
		loadWorldAtlas: () =>
			atlas.fail ? Promise.reject(new Error('blocked')) : actual.loadWorldAtlas()
	};
});

const boston = { place: 'Boston, MA', latitude: 42.36, longitude: -71.06 };
const LOCATIONS = [
	{
		...boston,
		event_type: 'birth',
		count: 2,
		person_ids: ['p1', 'p2'],
		persons: [
			{ id: 'p1', given_name: 'John', surname: 'Smith' },
			{ id: 'p2', given_name: 'James', surname: 'Smith' }
		]
	},
	{
		...boston,
		event_type: 'death',
		count: 1,
		person_ids: ['p3'],
		persons: [{ id: 'p3', given_name: 'Mary', surname: 'Jones' }]
	},
	{
		place: 'Sydney, NSW',
		latitude: -33.87,
		longitude: 151.21,
		event_type: 'death',
		count: 1,
		person_ids: ['p2'],
		persons: [{ id: 'p2', given_name: 'James', surname: 'Smith' }]
	}
];

async function renderMap() {
	const result = render(GeographicMap);
	await waitFor(() =>
		expect(result.container.querySelectorAll('.marker').length).toBeGreaterThan(0)
	);
	return result;
}

describe('GeographicMap', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		atlas.fail = false;
		vi.mocked(apiModule.api.getMapLocations).mockResolvedValue({
			items: LOCATIONS,
			total: LOCATIONS.length
		} as unknown as Awaited<ReturnType<typeof apiModule.api.getMapLocations>>);
	});

	it('clusters co-located locations into one focusable, named marker', async () => {
		await renderMap();
		const markers = screen.getAllByRole('button', { name: /: \d/ });
		expect(markers.map((m) => m.getAttribute('aria-label'))).toEqual([
			'Boston, MA: 2 births, 1 death',
			'Sydney, NSW: 1 death'
		]);
		expect(markers.every((m) => m.getAttribute('tabindex') === '0')).toBe(true);
	});

	it('opens a panel naming each person at a marker from the keyboard', async () => {
		await renderMap();
		const marker = screen.getByRole('button', {
			name: 'Boston, MA: 2 births, 1 death'
		});
		await fireEvent.keyDown(marker, { key: 'Enter' });

		const panel = await screen.findByRole('heading', { name: '2 locations' });
		const details = panel.closest('.detail-panel') as HTMLElement;
		const links = within(details).getAllByRole('link');
		expect(links.map((a) => [a.textContent, a.getAttribute('href')])).toEqual([
			['John Smith', '/persons/p1'],
			['James Smith', '/persons/p2'],
			['Mary Jones', '/persons/p3']
		]);

		await fireEvent.click(within(details).getByRole('button', { name: 'Close details' }));
		expect(screen.queryByRole('heading', { name: '2 locations' })).toBeNull();
	});

	it('draws the bundled world outline under the markers', async () => {
		const { container } = await renderMap();
		await waitFor(() => expect(container.querySelectorAll('.country').length).toBeGreaterThan(100));
		expect(screen.queryByRole('status')).toBeNull();
	});

	it('keeps markers and the location list when the world outline fails', async () => {
		atlas.fail = true;
		const { container } = await renderMap();

		expect((await screen.findByRole('status')).textContent).toContain(
			'world outline could not be loaded'
		);
		expect(container.querySelectorAll('.country')).toHaveLength(0);
		expect(container.querySelectorAll('.marker')).toHaveLength(2);

		const list = container.querySelector('details.location-list') as HTMLDetailsElement;
		expect(list.open).toBe(true);
		const items = within(list).getAllByRole('button');
		expect(items).toHaveLength(LOCATIONS.length);

		await fireEvent.click(items[2]);
		const heading = await screen.findByRole('heading', { name: 'Sydney, NSW' });
		const details = heading.closest('.detail-panel') as HTMLElement;
		expect(within(details).getByRole('link', { name: 'James Smith' })).toBeTruthy();
	});
});
