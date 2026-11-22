import { describe, it, expect } from 'vitest';
import { auth } from '#lib/stores/access.svelte.js';
import { signInAs } from '#lib/test/roles.js';
import { branchPageHref } from './branchPages';

const ID = '33333333-3333-3333-3333-333333333333';

describe('branchPageHref', () => {
	it("gives a signed-in user a branch's comparison and research pages", () => {
		signInAs('viewer');
		expect(branchPageHref(ID)).toBe(`/branches/${ID}`);
		expect(branchPageHref(ID, 'research')).toBe(`/branches/${ID}/research`);
	});

	it('escapes the id', () => {
		expect(branchPageHref('a/b')).toBe('/branches/a%2Fb');
	});

	it('gives a visitor, or the owner previewing as one, no branch page (ADR-008 §4)', () => {
		auth.phase = 'visitor';
		expect(branchPageHref(ID)).toBeNull();
		expect(branchPageHref(ID, 'research')).toBeNull();

		signInAs('owner');
		auth.previewAs = 'visitor';
		expect(branchPageHref(ID)).toBeNull();
	});
});
