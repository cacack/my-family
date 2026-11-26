import { describe, it, expect } from 'vitest';
import type { GeocodeCandidate } from '#lib/api/client.js';
import {
	candidateInput,
	candidatePick,
	geocodeErrorMessage,
	geocodedOrigin,
	matchPrecision,
	pickedOrigin
} from './geocode.js';

const boston: GeocodeCandidate = {
	display_name: 'Boston, Suffolk County, Massachusetts, United States',
	latitude: 42.3554334,
	longitude: -71.060511,
	query: 'Boston, Massachusetts',
	provider: 'Nominatim',
	attribution: 'Data © OpenStreetMap contributors, ODbL 1.0'
};

describe('candidateInput', () => {
	it.each([
		[42.3554334, -71.060511, '42.3554334', '-71.060511'],
		[51.5, 0, '51.5', '0'],
		[1e-7, -1e-8, '0.0000001', '-0'],
		[-33.8688, 151.2093, '-33.8688', '151.2093']
	])('shows %d, %d as %s, %s', (latitude, longitude, lat, lon) => {
		expect(candidateInput({ ...boston, latitude, longitude })).toEqual({ latitude: lat, longitude: lon });
	});
});

describe('matchPrecision', () => {
	it.each([
		['Boston, Massachusetts', 'Boston, Massachusetts', 'full'],
		['Boston, Massachusetts', '  boston,   Massachusetts ', 'full'],
		['Massachusetts', 'Boston, Massachusetts', 'partial'],
		['Boston', 'Boston, Massachusetts', 'partial'],
		['suffolk,  massachusetts', 'Boston, Suffolk, Massachusetts', 'partial'],
		['Boston, Massachusetts', 'Boston', undefined],
		['Boston', 'Salem, Essex, Massachusetts', undefined],
		['Boston, Massachusetts', 'Boston, Suffolk, Massachusetts', 'partial'],
		['Massachusetts, Boston', 'Boston, Suffolk, Massachusetts', undefined],
		['Bost', 'Boston, Massachusetts', undefined]
	] as const)('matched %j for place %j is %s', (query, place, want) => {
		expect(matchPrecision(query, place)).toBe(want);
	});
});

describe('geocodedOrigin', () => {
	it('names the provider and the precision against the place name', () => {
		expect(geocodedOrigin(boston, 'Boston, Massachusetts')).toEqual({
			source: 'geocoded',
			provider: 'Nominatim',
			precision: 'full'
		});
		expect(geocodedOrigin(boston, 'Boston, Suffolk, Massachusetts')).toEqual({
			source: 'geocoded',
			provider: 'Nominatim',
			precision: 'partial'
		});
		expect(geocodedOrigin(boston, 'Salem, Massachusetts')).toBeUndefined();
	});
});

describe('pickedOrigin', () => {
	const pick = candidatePick(boston);

	it('is geocoded while the inputs hold the pick, judged against the place name at save time', () => {
		expect(pickedOrigin({ latitude: '42.3554334', longitude: '-71.060511' }, pick, 'Boston, Massachusetts')).toEqual({
			source: 'geocoded',
			provider: 'Nominatim',
			precision: 'full'
		});
		expect(
			pickedOrigin({ latitude: ' 42.3554334', longitude: '-71.060511 ' }, pick, 'Back Bay, Boston, Massachusetts')
				?.precision
		).toBe('partial');
	});

	it('is undefined once the place name no longer holds the text matched', () => {
		expect(pickedOrigin({ latitude: '42.3554334', longitude: '-71.060511' }, pick, 'Salem, Essex, Massachusetts')).toBeUndefined();
	});

	it.each([
		['no pick', { latitude: '42.3554334', longitude: '-71.060511' }, null],
		['a typed-over latitude', { latitude: '42.36', longitude: '-71.060511' }, pick],
		['cleared inputs', { latitude: '', longitude: '' }, { ...pick, latitude: '', longitude: '' }]
	])('is undefined with %s', (_name, pair, p) => {
		expect(pickedOrigin(pair, p, 'Boston, Massachusetts')).toBeUndefined();
	});
});

describe('geocodeErrorMessage', () => {
	it.each([
		[{ status: 504, code: 'geocoder_timeout', message: 'x' }, /did not answer in time/],
		[{ status: 502, code: 'geocoder_error', message: 'x' }, /failed to answer/],
		[{ status: 409, code: 'geocoder_disabled', message: 'x' }, /not enabled/],
		[{ status: 400, code: 'VALIDATION_ERROR', message: 'q must be 1 to 255 characters' }, /^q must be 1 to 255/],
		[new TypeError('Failed to fetch'), /^The lookup failed/]
	])('says which way %j failed', (error, want) => {
		expect(geocodeErrorMessage(error)).toMatch(want);
	});
});
