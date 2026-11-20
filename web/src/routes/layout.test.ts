import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet, flushSync } from 'svelte';
import Layout from './+layout.svelte';
import { auth } from '#lib/stores/auth.svelte.js';
import { invalidateAll } from '$app/navigation';
import { loadHomePerson, resetHomePerson } from '#lib/stores/homePerson.svelte.js';
import type { Account } from '#lib/api/client.js';

vi.mock('$app/navigation', () => ({ goto: vi.fn(), invalidateAll: vi.fn() }));
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/'), route: { id: '/' } } }));

// The real auth state, so the layout reacts when a test changes who is viewing;
// only the server calls are stubbed.
vi.mock('#lib/stores/auth.svelte.js', async (importOriginal) => ({
	...(await importOriginal<typeof import('#lib/stores/auth.svelte.js')>()),
	loadAuth: vi.fn(),
	recheckSession: vi.fn()
}));

vi.mock('#lib/stores/homePerson.svelte.js', () => ({
	homePerson: { person: null, loaded: false },
	loadHomePerson: vi.fn(),
	resetHomePerson: vi.fn()
}));

vi.mock('#lib/stores/appConfig.svelte.js', () => ({
	loadAppConfig: vi.fn(),
	getAppConfig: () => ({ demo_mode: false })
}));

vi.mock('#lib/stores/activeBranch.svelte.js', async (importOriginal) => ({
	...(await importOriginal<typeof import('#lib/stores/activeBranch.svelte.js')>()),
	revalidateActiveBranch: vi.fn()
}));

const account = (id: string) => ({ id, username: id, role: 'owner' }) as unknown as Account;

/** A page that counts how often it is mounted. */
let mounts = 0;
const children = createRawSnippet(() => ({
	render: () => '<p>page</p>',
	setup: () => {
		mounts++;
	}
}));

describe('root layout: a new viewer', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mounts = 0;
		auth.phase = 'signed-in';
		auth.account = account('a-1');
		auth.visitorsWelcome = true;
	});

	it('forgets the home person, refetches, and rebuilds the page', () => {
		render(Layout, { children });
		flushSync();
		expect(mounts).toBe(1);
		expect(resetHomePerson).toHaveBeenCalledTimes(1);
		expect(loadHomePerson).toHaveBeenCalledTimes(1);
		expect(invalidateAll).toHaveBeenCalledTimes(1);

		// Signing out while visitors are let in keeps the app on screen.
		auth.phase = 'visitor';
		auth.account = null;
		flushSync();

		expect(mounts).toBe(2);
		expect(resetHomePerson).toHaveBeenCalledTimes(2);
		expect(loadHomePerson).toHaveBeenCalledTimes(2);
		expect(invalidateAll).toHaveBeenCalledTimes(2);
		// Reset runs before the load, so no page shows the last viewer's choice.
		expect(vi.mocked(resetHomePerson).mock.invocationCallOrder[1]).toBeLessThan(
			vi.mocked(loadHomePerson).mock.invocationCallOrder[1]
		);
	});

	it('keeps the page when the same account is refreshed', () => {
		render(Layout, { children });
		flushSync();

		auth.account = account('a-1'); // e.g. recheckSession or a profile edit
		flushSync();

		expect(mounts).toBe(1);
		expect(loadHomePerson).toHaveBeenCalledTimes(1);
		expect(invalidateAll).toHaveBeenCalledTimes(1);
	});

	it('rebuilds the page when the owner starts or stops a preview, under its banner (#1413)', () => {
		render(Layout, { children });
		flushSync();

		auth.previewAs = 'visitor';
		flushSync();
		expect(mounts).toBe(2);
		expect(loadHomePerson).toHaveBeenCalledTimes(2);
		expect(invalidateAll).toHaveBeenCalledTimes(2);
		expect(screen.getByText(/as a visitor who is not signed in does/)).toBeDefined();

		auth.previewAs = null;
		flushSync();
		expect(mounts).toBe(3);
		expect(invalidateAll).toHaveBeenCalledTimes(3);
		expect(screen.queryByText(/as a visitor who is not signed in does/)).toBeNull();
	});
});
