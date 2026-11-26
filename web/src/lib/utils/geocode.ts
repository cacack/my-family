/**
 * Geocoder candidates as the place forms and the worklist take them (#1607):
 * a candidate fills a pair in signed decimal degrees, and the pair is saved
 * with a geocoded origin naming the provider and how much of the place name
 * it matched (#1563).
 */
import type { CoordinateOrigin, GeocodeCandidate, MatchPrecision } from '#lib/api/client.js';
import type { CoordinateInput } from './coordinates.js';

/** A candidate picked into a form's inputs, kept until the form is saved. */
export interface CoordinatePick extends CoordinateInput {
	/** The text the geocoder matched. */
	query: string;
	provider: string;
}

/** Spaces trimmed and collapsed, as the server normalizes a lookup. */
function normalized(name: string): string {
	return name.trim().split(/\s+/).join(' ');
}

/** A coordinate in signed decimal degrees, without an exponent or trailing zeros. */
function decimal(value: number): string {
	return value.toFixed(7).replace(/\.?0+$/, '');
}

/** A candidate's pair as a form's inputs show it. */
export function candidateInput(candidate: GeocodeCandidate): CoordinateInput {
	return { latitude: decimal(candidate.latitude), longitude: decimal(candidate.longitude) };
}

/**
 * Full when the geocoder matched the whole place name, ignoring case and
 * runs of spaces as its cache does; partial when it matched other text.
 */
export function matchPrecision(query: string, place: string): MatchPrecision {
	return normalized(query).toLowerCase() === normalized(place).toLowerCase() ? 'full' : 'partial';
}

/** The origin a candidate's pair is saved with, for a place named `place`. */
export function geocodedOrigin(pick: Pick<CoordinatePick, 'query' | 'provider'>, place: string): CoordinateOrigin {
	return { source: 'geocoded', provider: pick.provider, precision: matchPrecision(pick.query, place) };
}

/** A candidate as a form keeps it until saved. */
export function candidatePick(candidate: GeocodeCandidate): CoordinatePick {
	return { ...candidateInput(candidate), query: candidate.query, provider: candidate.provider };
}

/**
 * The origin to send with a form's pair: geocoded while the pair is still the
 * one picked, for the place name as it is at save time; undefined once it was
 * typed over, so the server records it as entered by hand.
 */
export function pickedOrigin(
	pair: CoordinateInput,
	pick: CoordinatePick | null | undefined,
	place: string
): CoordinateOrigin | undefined {
	if (!pick) return undefined;
	const same = pair.latitude.trim() === pick.latitude && pair.longitude.trim() === pick.longitude;
	return same && pair.latitude.trim() ? geocodedOrigin(pick, place) : undefined;
}

/** Why a lookup failed, in words that say which way it failed. */
export function geocodeErrorMessage(error: unknown): string {
	const { code, status, message } = (error ?? {}) as { code?: string; status?: number; message?: string };
	switch (code) {
		case 'geocoder_timeout':
			return 'The geocoder did not answer in time. Try again in a moment, or enter the coordinates by hand.';
		case 'geocoder_error':
			return 'The geocoder failed to answer. Try again later, or enter the coordinates by hand.';
		case 'geocoder_disabled':
			return 'The geocoder is not enabled on this server. Enter the coordinates by hand.';
		default:
			// No status: the request never reached the server, and its message is the browser's.
			return status && message ? message : 'The lookup failed. Enter the coordinates by hand.';
	}
}
