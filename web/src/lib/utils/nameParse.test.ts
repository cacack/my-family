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
});
