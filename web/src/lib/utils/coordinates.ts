/**
 * A place's coordinate pair as the forms take it (#1033): signed decimal
 * degrees typed in ("42.3601", "-71.0589"), stored and sent in GEDCOM form
 * ("N42.3601", "W71.0589"), the form the API and the map read.
 */

export type CoordinateAxis = 'latitude' | 'longitude';

/** A pair as typed into a form, in signed decimal degrees; '' is empty. */
export interface CoordinateInput {
	latitude: string;
	longitude: string;
}

/** A pair as the API carries it, in GEDCOM form. */
export interface GedcomCoordinates {
	latitude?: string | null;
	longitude?: string | null;
}

/** Why a typed pair can't be saved, and which input to show it on. */
export interface CoordinateError {
	field: CoordinateAxis;
	message: string;
}

const AXES = {
	latitude: { limit: 90, positive: 'N', negative: 'S' },
	longitude: { limit: 180, positive: 'E', negative: 'W' }
} as const;

// Plain decimals only, as gedcom.ParseLatitude takes after its letter: no
// exponent, no thousands separator.
const SIGNED_DECIMAL = /^([+-]?)(\d+\.?\d*|\.\d+)$/;
const GEDCOM_COORDINATE = /^([NSEW])(\d+\.?\d*|\.\d+)$/i;

/** Why `value` is not a coordinate on `axis`, or null when it is one. */
function axisError(value: string, axis: CoordinateAxis): string | null {
	const { limit } = AXES[axis];
	const match = SIGNED_DECIMAL.exec(value.trim());
	const name = axis === 'latitude' ? 'Latitude' : 'Longitude';
	if (!match) return `${name} must be a number of degrees, such as ${axis === 'latitude' ? '42.3601' : '-71.0589'}.`;
	if (Number(match[2]) > limit) return `${name} must be between -${limit} and ${limit}.`;
	return null;
}

/**
 * A signed decimal coordinate in GEDCOM form: "42.3601" is "N42.3601" as a
 * latitude, "-71.0589" is "W71.0589" as a longitude, and zero takes the
 * positive letter. The digits are kept as typed. `value` must be valid.
 */
export function toGedcomCoordinate(value: string, axis: CoordinateAxis): string {
	const match = SIGNED_DECIMAL.exec(value.trim());
	if (!match) throw new Error(`not a decimal coordinate: ${value}`);
	const [, sign, digits] = match;
	const { positive, negative } = AXES[axis];
	const letter = sign === '-' && Number(digits) !== 0 ? negative : positive;
	return letter + (digits.startsWith('.') ? `0${digits}` : digits);
}

/**
 * A GEDCOM coordinate in signed decimal degrees, as a form shows it:
 * "S33.8688" is "-33.8688". Empty is ''; a value that isn't a coordinate on
 * `axis` is shown as stored, for the user to see and correct.
 */
export function toDecimalDegrees(value: string | null | undefined, axis: CoordinateAxis): string {
	const trimmed = (value ?? '').trim();
	const match = GEDCOM_COORDINATE.exec(trimmed);
	const { positive, negative } = AXES[axis];
	const letter = match?.[1].toUpperCase();
	if (!match || (letter !== positive && letter !== negative)) return trimmed;
	const digits = match[2];
	return letter === negative && Number(digits) !== 0 ? `-${digits}` : digits;
}

/** A stored pair as a form shows it. */
export function coordinateInput(saved?: GedcomCoordinates): CoordinateInput {
	return {
		latitude: toDecimalDegrees(saved?.latitude, 'latitude'),
		longitude: toDecimalDegrees(saved?.longitude, 'longitude')
	};
}

/**
 * Why a typed pair can't be saved for a place named `place`, or null: each
 * half must be a number in range, both or neither must be given, and a pair
 * needs a place name, which the API keys coordinates by. A pair left as
 * `saved` is not checked, as it is not sent.
 */
export function coordinatePairError(
	pair: CoordinateInput,
	place: string,
	saved?: GedcomCoordinates
): CoordinateError | null {
	const latitude = pair.latitude.trim();
	const longitude = pair.longitude.trim();
	if ((!latitude && !longitude) || !coordinatesChanged(pair, saved)) return null;
	for (const field of ['latitude', 'longitude'] as const) {
		const value = field === 'latitude' ? latitude : longitude;
		const message = value ? axisError(value, field) : null;
		if (message) return { field, message };
	}
	if (!longitude) return { field: 'longitude', message: 'Enter a longitude too, or clear the latitude.' };
	if (!latitude) return { field: 'latitude', message: 'Enter a latitude too, or clear the longitude.' };
	if (!place.trim()) return { field: 'latitude', message: 'Enter a place name for these coordinates.' };
	return null;
}

/** Whether a typed pair differs from the stored one. */
export function coordinatesChanged(pair: CoordinateInput, saved?: GedcomCoordinates): boolean {
	const stored = coordinateInput(saved);
	return pair.latitude.trim() !== stored.latitude || pair.longitude.trim() !== stored.longitude;
}

/**
 * What to send for a typed pair: undefined when it is as stored, so the API
 * leaves it (and its origin) alone; both null when it was cleared; else both
 * in GEDCOM form. The pair must have passed `coordinatePairError`.
 */
export function coordinateChange(
	pair: CoordinateInput,
	saved?: GedcomCoordinates
): { latitude: string | null; longitude: string | null } | undefined {
	if (!coordinatesChanged(pair, saved)) return undefined;
	if (!pair.latitude.trim()) return { latitude: null, longitude: null };
	return {
		latitude: toGedcomCoordinate(pair.latitude, 'latitude'),
		longitude: toGedcomCoordinate(pair.longitude, 'longitude')
	};
}
