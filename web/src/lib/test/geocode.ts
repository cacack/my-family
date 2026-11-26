import { fireEvent, screen } from '@testing-library/svelte';
import type { GeocodeCandidate } from '#lib/api/client.js';
import { getAppConfig } from '#lib/stores/appConfig.svelte.js';

/** Turn the server's geocoder on or off, as `GET /config` reports it (#1607). */
export function setGeocoderEnabled(enabled: boolean): void {
	getAppConfig().geocoder_enabled = enabled;
}

/** A candidate as Nominatim answers it, for a lookup of "Boston". */
export const bostonCandidate: GeocodeCandidate = {
	display_name: 'Boston, Suffolk County, Massachusetts, United States',
	latitude: 42.3554334,
	longitude: -71.060511,
	query: 'Boston',
	provider: 'Nominatim',
	attribution: 'Data © OpenStreetMap contributors, ODbL 1.0, https://osm.org/copyright'
};

/** Look up the place labeled `place` and pick the first candidate. */
export async function pickFirstCandidate(place: string): Promise<void> {
	await fireEvent.click(screen.getByRole('button', { name: `Look up coordinates for ${place}` }));
	await fireEvent.click(await screen.findByRole('button', { name: new RegExp(`^Use this, .* for ${place}$`) }));
}
