import { describe, it, expect } from 'vitest';
import type { AfterNavigate } from '$app/navigation';
import { cameFrom } from './backLink.svelte';

const ORIGIN = 'http://localhost';

function at(path: string, routeId: string) {
	return { params: {}, route: { id: routeId }, url: new URL(path, ORIGIN) };
}

function navigation(from: unknown): Pick<AfterNavigate, 'from' | 'to'> {
	return { from, to: at('/pedigree/p-1', '/pedigree/[id]') } as unknown as Pick<
		AfterNavigate,
		'from' | 'to'
	>;
}

describe('cameFrom (#1026)', () => {
	it('returns the in-app page the navigation came from, with its query and hash', () => {
		expect(cameFrom(navigation(at('/persons/p-1?tab=events#notes', '/persons/[id]')))).toBe(
			'/persons/p-1?tab=events#notes'
		);
	});

	it("is null on SvelteKit's SPA enter navigation, whose from has no URL", () => {
		expect(cameFrom(navigation({ params: {}, route: { id: null }, url: null }))).toBeNull();
	});

	it('is null with no from at all', () => {
		expect(cameFrom(navigation(null))).toBeNull();
	});

	it('is null when moving within the same route', () => {
		expect(cameFrom(navigation(at('/pedigree/p-2', '/pedigree/[id]')))).toBeNull();
	});
});
