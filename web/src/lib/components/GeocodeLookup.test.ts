import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import GeocodeLookup from './GeocodeLookup.svelte';
import * as apiModule from '#lib/api/client.js';
import { getAppConfig } from '#lib/stores/appConfig.svelte.js';
import { signInAs } from '#lib/test/roles.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { geocodePlace: vi.fn() } };
});

const attribution = 'Data © OpenStreetMap contributors, ODbL 1.0';
const candidates: apiModule.GeocodeCandidate[] = [
	{
		display_name: 'Boston, Suffolk County, Massachusetts, United States',
		latitude: 42.3554334,
		longitude: -71.060511,
		query: 'Boston',
		provider: 'Nominatim',
		attribution
	},
	{
		display_name: 'Boston, Lincolnshire, England, United Kingdom',
		latitude: 52.9776,
		longitude: -0.0266,
		query: 'Boston',
		provider: 'Nominatim',
		attribution
	}
];

function renderLookup(place = 'Boston') {
	const onPick = vi.fn();
	render(GeocodeLookup, { id: 'birth-place', place, label: 'Birth Place', onPick });
	return onPick;
}

const lookUp = () => screen.getByRole('button', { name: 'Look up coordinates for Birth Place' });

describe('GeocodeLookup', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getAppConfig().geocoder_enabled = true;
		vi.mocked(apiModule.api.geocodePlace).mockResolvedValue({ items: candidates, total: 2 });
	});

	it('renders nothing when the server has the geocoder disabled', () => {
		getAppConfig().geocoder_enabled = false;
		const { container } = render(GeocodeLookup, { id: 'x', place: 'Boston', label: 'Birth Place', onPick: vi.fn() });
		expect(container.textContent?.trim()).toBe('');
		expect(screen.queryByRole('button')).toBeNull();
	});

	it('renders nothing for a viewer', () => {
		signInAs('viewer');
		renderLookup();
		expect(screen.queryByRole('button')).toBeNull();
	});

	it('is disabled while the place name is empty', () => {
		renderLookup('   ');
		expect(lookUp().hasAttribute('disabled')).toBe(true);
	});

	it('lists candidates with name, coordinates, a map and the attribution, before anything is picked', async () => {
		const onPick = renderLookup('  Boston ');
		await fireEvent.click(lookUp());

		expect(apiModule.api.geocodePlace).toHaveBeenCalledWith('Boston');
		const list = await screen.findByRole('region', { name: 'Locations matching “Boston”' });
		const items = within(list).getAllByRole('listitem');
		expect(items.map((li) => li.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
			'1 Boston, Suffolk County, Massachusetts, United States 42.3554334, -71.060511 Use this',
			'2 Boston, Lincolnshire, England, United Kingdom 52.9776, -0.0266 Use this'
		]);
		const map = within(list).getByRole('img', { name: 'Map of the 2 candidates' });
		expect(map.querySelectorAll('.marker')).toHaveLength(2);
		expect(within(list).getByTestId('geocode-attribution').textContent).toBe(attribution);
		expect(onPick).not.toHaveBeenCalled();
	});

	it('hands the picked candidate to the caller and closes the list', async () => {
		const onPick = renderLookup();
		await fireEvent.click(lookUp());
		await fireEvent.click(
			await screen.findByRole('button', {
				name: 'Use this, Boston, Lincolnshire, England, United Kingdom, 52.9776, -0.0266, for Birth Place'
			})
		);
		expect(onPick).toHaveBeenCalledWith(candidates[1]);
		expect(screen.queryByRole('region')).toBeNull();
		expect(document.activeElement).toBe(lookUp());
	});

	it.each([
		['the Close button', () => fireEvent.click(screen.getByRole('button', { name: 'Close' }))],
		['Escape', () => fireEvent.keyDown(screen.getByRole('region'), { key: 'Escape' })]
	])('closes with %s without picking', async (_name, close) => {
		const onPick = renderLookup();
		await fireEvent.click(lookUp());
		await screen.findByRole('region');
		await close();
		expect(screen.queryByRole('region')).toBeNull();
		expect(onPick).not.toHaveBeenCalled();
	});

	it('says when nothing matched', async () => {
		vi.mocked(apiModule.api.geocodePlace).mockResolvedValue({ items: [], total: 0 });
		renderLookup();
		await fireEvent.click(lookUp());
		const region = await screen.findByRole('region', { name: 'No locations matching “Boston”' });
		expect(region.textContent).toContain('The geocoder found no location for this name.');
		expect(within(region).queryByRole('img')).toBeNull();
	});

	it.each([
		[{ status: 502, code: 'geocoder_error', message: 'The geocoder failed' }, 'The geocoder failed to answer.'],
		[{ status: 504, code: 'geocoder_timeout', message: 'busy' }, 'The geocoder did not answer in time.']
	])('says which way a lookup failed: %j', async (error, want) => {
		vi.mocked(apiModule.api.geocodePlace).mockRejectedValue(error);
		renderLookup();
		await fireEvent.click(lookUp());
		const region = await screen.findByRole('region', { name: 'Lookup of “Boston” failed' });
		expect(within(region).getByRole('alert').textContent).toContain(want);
		// The lookup can be tried again.
		expect(lookUp().hasAttribute('disabled')).toBe(false);
	});

	it('drops the answer to an earlier lookup closed before it arrived', async () => {
		let answer: (v: apiModule.GeocodeCandidateList) => void = () => {};
		vi.mocked(apiModule.api.geocodePlace).mockReturnValue(new Promise((r) => (answer = r)));
		renderLookup();
		await fireEvent.click(lookUp());
		// Its name says what its label shows while the lookup runs.
		const looking = screen.getByRole('button', { name: 'Looking up coordinates for Birth Place' });
		expect(looking.textContent?.trim()).toBe('Looking up...');
		await fireEvent.keyDown(looking, { key: 'Escape' });
		answer({ items: candidates, total: 2 });
		await waitFor(() => expect(lookUp().textContent?.trim()).toBe('Look up'));
		expect(screen.queryByRole('region')).toBeNull();
	});
});
