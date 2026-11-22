import { describe, it, expect } from 'vitest';
import { parseName } from './nameParse';

describe('parseName', () => {
	it('splits first and last name', () => {
		expect(parseName('John Smith')).toEqual({ givenName: 'John', surname: 'Smith' });
	});

	it('handles multiple given names', () => {
		expect(parseName('Mary Jane Watson')).toEqual({ givenName: 'Mary Jane', surname: 'Watson' });
	});

	it('handles single name', () => {
		expect(parseName('Madonna')).toEqual({ givenName: 'Madonna', surname: '' });
	});

	it('trims whitespace', () => {
		expect(parseName('  John  Smith  ')).toEqual({ givenName: 'John', surname: 'Smith' });
	});

	it('handles empty string', () => {
		expect(parseName('')).toEqual({ givenName: '', surname: '' });
	});

	// A GEDCOM-style slashed surname is taken whole, and may stand alone (#1709).
	it('takes a slashed surname whole', () => {
		expect(parseName('Mary /van Buren/')).toEqual({ givenName: 'Mary', surname: 'van Buren' });
	});

	it('reads a slashed surname alone as a surname-only name', () => {
		expect(parseName('/Tudor/')).toEqual({ givenName: '', surname: 'Tudor' });
		expect(parseName(' /Tudor ')).toEqual({ givenName: '', surname: 'Tudor' });
	});

	it('reads a lone slash as no name', () => {
		expect(parseName('/')).toEqual({ givenName: '', surname: '' });
	});
});
