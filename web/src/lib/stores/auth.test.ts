import { describe, it, expect, vi, beforeEach } from 'vitest';
import type * as apiModule from '#lib/api/client.js';

const { getAuthStatus, signInMock, setupOwner, signOutMock, handlers } = vi.hoisted(() => ({
	getAuthStatus: vi.fn(),
	signInMock: vi.fn(),
	setupOwner: vi.fn(),
	signOutMock: vi.fn(),
	handlers: [] as Array<(() => void) | null>
}));

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		setUnauthorizedHandler: (handler: (() => void) | null) => handlers.push(handler),
		api: {
			getAuthStatus: () => getAuthStatus(),
			signIn: (input: unknown) => signInMock(input),
			setupOwner: (input: unknown) => setupOwner(input),
			signOut: () => signOutMock()
		}
	};
});

import {
	auth,
	loadAuth,
	signIn,
	signOut,
	completeSetup,
	markSignedOut,
	can,
	currentRole,
	showSignIn,
	browseAsVisitor
} from './auth.svelte.js';

const ADA = { id: '11111111-1111-1111-1111-111111111111', username: 'ada', display_name: 'Ada', role: 'owner' as const };

describe('auth store', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		auth.phase = 'loading';
		auth.account = null;
		auth.visitorsWelcome = false;
	});

	it.each([
		[{ mode: 'open', setup_required: false }, 'open', null],
		[{ mode: 'accounts', setup_required: true }, 'setup', null],
		[{ mode: 'accounts', setup_required: false }, 'signed-out', null],
		[{ mode: 'accounts', setup_required: false, account: ADA }, 'signed-in', ADA],
		[{ mode: 'accounts', setup_required: false, anonymous_access: 'none' }, 'signed-out', null],
		[{ mode: 'accounts', setup_required: false, anonymous_access: 'viewer' }, 'visitor', null],
		[{ mode: 'accounts', setup_required: false, anonymous_access: 'viewer', account: ADA }, 'signed-in', ADA]
	])('reads status %o as %s', async (status, phase, account) => {
		getAuthStatus.mockResolvedValue(status);
		await loadAuth();
		expect(auth.phase).toBe(phase);
		expect(auth.account).toEqual(account);
	});

	it('lets a visitor browse as a viewer, sign in, and come back after signing out', async () => {
		getAuthStatus.mockResolvedValue({ mode: 'accounts', setup_required: false, anonymous_access: 'viewer' });
		await loadAuth();
		expect(currentRole()).toBe('viewer');
		expect(can('viewer')).toBe(true);
		expect(can('editor')).toBe(false);

		showSignIn();
		expect(auth.phase).toBe('signed-out');
		browseAsVisitor();
		expect(auth.phase).toBe('visitor');

		signInMock.mockResolvedValue(ADA);
		await signIn('ada', 'secret password');
		signOutMock.mockResolvedValue(undefined);
		await signOut();
		expect(auth.phase).toBe('visitor');
	});

	it('keeps the sign-in page when visitors must sign in', async () => {
		getAuthStatus.mockResolvedValue({ mode: 'accounts', setup_required: false, anonymous_access: 'none' });
		await loadAuth();
		browseAsVisitor();
		expect(auth.phase).toBe('signed-out');
		expect(currentRole()).toBeNull();
	});

	it('reports a status it could not read', async () => {
		getAuthStatus.mockRejectedValue(new TypeError('Failed to fetch'));
		await loadAuth();
		expect(auth.phase).toBe('error');
	});

	it('signs in, and out on the server', async () => {
		signInMock.mockResolvedValue(ADA);
		await signIn('ada', 'secret password');
		expect(signInMock).toHaveBeenCalledWith({ username: 'ada', password: 'secret password' });
		expect(auth.phase).toBe('signed-in');
		expect(auth.account).toEqual(ADA);

		signOutMock.mockResolvedValue(undefined);
		await signOut();
		expect(signOutMock).toHaveBeenCalledOnce();
		expect(auth.phase).toBe('signed-out');
		expect(auth.account).toBeNull();
	});

	it('shows the sign-in page even when sign-out fails', async () => {
		auth.phase = 'signed-in';
		auth.account = ADA;
		signOutMock.mockRejectedValue(new TypeError('Failed to fetch'));
		await expect(signOut()).rejects.toThrow();
		expect(auth.phase).toBe('signed-out');
	});

	it('keeps the phase when sign-in fails', async () => {
		auth.phase = 'signed-out';
		signInMock.mockRejectedValue({ status: 401, code: 'INVALID_CREDENTIALS', message: 'wrong' });
		await expect(signIn('ada', 'x')).rejects.toMatchObject({ code: 'INVALID_CREDENTIALS' });
		expect(auth.phase).toBe('signed-out');
	});

	it('signs the new owner in after setup', async () => {
		setupOwner.mockResolvedValue(ADA);
		await completeSetup({ token: 't', username: 'ada', display_name: 'Ada', password: 'secret password' });
		expect(auth.phase).toBe('signed-in');
	});

	it('turns a 401 mid-session into the sign-in page', () => {
		expect(handlers.at(-1)).toBe(markSignedOut);
		auth.phase = 'signed-in';
		auth.account = ADA;
		markSignedOut();
		expect(auth.phase).toBe('signed-out');
		expect(auth.account).toBeNull();
	});

	it('leaves open mode alone on a stray 401', () => {
		auth.phase = 'open';
		markSignedOut();
		expect(auth.phase).toBe('open');
	});

	it.each([
		['viewer', [true, false, false]],
		['editor', [true, true, false]],
		['owner', [true, true, true]]
	] as const)('lets a signed-in %s do what its role includes', (role, expected) => {
		auth.phase = 'signed-in';
		auth.account = { ...ADA, role };
		expect(currentRole()).toBe(role);
		expect([can('viewer'), can('editor'), can('owner')]).toEqual(expected);
	});

	it('treats open mode as the owner and no one signed in as no role', () => {
		auth.phase = 'open';
		expect(currentRole()).toBe('owner');
		expect(can('owner')).toBe(true);
		auth.phase = 'signed-out';
		expect(currentRole()).toBeNull();
		expect(can('viewer')).toBe(false);
	});
});
