import { describe, it, expect } from 'vitest';
import {
	coordinateChange,
	coordinateInput,
	coordinatePairError,
	toDecimalDegrees,
	toGedcomCoordinate
} from './coordinates';

describe('toGedcomCoordinate', () => {
	it.each([
		['42.3601', 'latitude', 'N42.3601'],
		['-33.8688', 'latitude', 'S33.8688'],
		['151.2093', 'longitude', 'E151.2093'],
		['-71.0589', 'longitude', 'W71.0589'],
		['+12.5', 'latitude', 'N12.5'],
		[' 0 ', 'latitude', 'N0'],
		['-0', 'longitude', 'E0'],
		['-0.0', 'latitude', 'N0.0'],
		['.5', 'longitude', 'E0.5'],
		['-90', 'latitude', 'S90']
	] as const)('writes %s as a %s as %s', (value, axis, expected) => {
		expect(toGedcomCoordinate(value, axis)).toBe(expected);
	});

	it('refuses a value that is not a decimal', () => {
		expect(() => toGedcomCoordinate('1e5', 'latitude')).toThrow();
	});
});

describe('toDecimalDegrees', () => {
	it.each([
		['N42.3601', 'latitude', '42.3601'],
		['S33.8688', 'latitude', '-33.8688'],
		['E151.2093', 'longitude', '151.2093'],
		['W71.0589', 'longitude', '-71.0589'],
		['w71.0589', 'longitude', '-71.0589'],
		['N0', 'latitude', '0'],
		['W0', 'longitude', '0'],
		[' S0.0 ', 'latitude', '0.0']
	] as const)('reads %s as a %s as %s', (value, axis, expected) => {
		expect(toDecimalDegrees(value, axis)).toBe(expected);
	});

	it('reads nothing as empty', () => {
		expect(toDecimalDegrees(null, 'latitude')).toBe('');
		expect(toDecimalDegrees(undefined, 'longitude')).toBe('');
	});

	it('shows a value it cannot read as stored, for the user to correct', () => {
		expect(toDecimalDegrees('E42', 'latitude')).toBe('E42');
		expect(toDecimalDegrees('42 N', 'latitude')).toBe('42 N');
	});

	it('round-trips with toGedcomCoordinate', () => {
		for (const value of ['N42.3601', 'S33.8688']) {
			expect(toGedcomCoordinate(toDecimalDegrees(value, 'latitude'), 'latitude')).toBe(value);
		}
		for (const value of ['E151.2093', 'W71.0589']) {
			expect(toGedcomCoordinate(toDecimalDegrees(value, 'longitude'), 'longitude')).toBe(value);
		}
	});
});

describe('coordinatePairError', () => {
	const pair = (latitude: string, longitude: string) => ({ latitude, longitude });

	it('accepts an empty pair and a pair in range', () => {
		expect(coordinatePairError(pair('', ''), '')).toBeNull();
		expect(coordinatePairError(pair('  ', ''), 'Boston')).toBeNull();
		expect(coordinatePairError(pair('42.3601', '-71.0589'), 'Boston')).toBeNull();
		expect(coordinatePairError(pair('-90', '180'), 'Pole')).toBeNull();
	});

	it.each([
		[pair('north', '-71'), 'latitude', 'Latitude must be a number of degrees, such as 42.3601.'],
		[pair('42', '1e2'), 'longitude', 'Longitude must be a number of degrees, such as -71.0589.'],
		[pair('90.5', '0'), 'latitude', 'Latitude must be between -90 and 90.'],
		[pair('-91', '0'), 'latitude', 'Latitude must be between -90 and 90.'],
		[pair('0', '-180.01'), 'longitude', 'Longitude must be between -180 and 180.'],
		[pair('42', ''), 'longitude', 'Enter a longitude too, or clear the latitude.'],
		[pair('', '-71'), 'latitude', 'Enter a latitude too, or clear the longitude.']
	])('refuses %o on its %s', (typed, field, message) => {
		expect(coordinatePairError(typed, 'Boston')).toEqual({ field, message });
	});

	it('needs a place name for a pair', () => {
		expect(coordinatePairError(pair('42', '-71'), ' ')).toEqual({
			field: 'latitude',
			message: 'Enter a place name for these coordinates.'
		});
	});

	it('does not check a pair left as stored, which is not sent', () => {
		const saved = { latitude: 'N95', longitude: 'W71' };
		expect(coordinatePairError(coordinateInput(saved), '', saved)).toBeNull();
		expect(coordinatePairError(pair('96', '-71'), 'Boston', saved)).not.toBeNull();
	});
});

describe('coordinateChange', () => {
	const saved = { latitude: 'N42.3601', longitude: 'W71.0589' };

	it('leaves out a pair that is as stored', () => {
		expect(coordinateChange({ latitude: '42.3601', longitude: ' -71.0589 ' }, saved)).toBeUndefined();
		expect(coordinateChange({ latitude: '', longitude: '' })).toBeUndefined();
	});

	it('sends a cleared pair as null for both', () => {
		expect(coordinateChange({ latitude: '', longitude: '' }, saved)).toEqual({ latitude: null, longitude: null });
	});

	it('sends a changed pair in GEDCOM form', () => {
		expect(coordinateChange({ latitude: '-33.8688', longitude: '151.2093' }, saved)).toEqual({
			latitude: 'S33.8688',
			longitude: 'E151.2093'
		});
		expect(coordinateChange({ latitude: '42.3601', longitude: '-71.0589' })).toEqual(saved);
	});
});
