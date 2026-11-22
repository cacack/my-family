import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
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
	isOpenMode,
	showSignIn,
	browseAsVisitor,
	recheckSession,
	previewing,
	startPreview,
	stopPreview
} from './auth.svelte.js';
import { canChooseHomePerson, isVisitor } from './access.svelte.js';

const ADA = { id: '11111111-1111-1111-1111-111111111111', username: 'ada', display_name: 'Ada', role: 'owner' as const };

describe('auth store', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		auth.phase = 'loading';
		auth.account = null;
		auth.visitorsWelcome = false;
		auth.previewAs = null;
		document.cookie = 'myfamily_preview=; Path=/; Max-Age=0';
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
		expect(isOpenMode()).toBe(phase === 'open');
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

	it('shows a user whose session expired while visitors are let in as signed out', async () => {
		getAuthStatus.mockResolvedValue({ mode: 'accounts', setup_required: false, anonymous_access: 'viewer', account: ADA });
		await loadAuth();
		expect(auth.phase).toBe('signed-in');

		// Reads still succeed (as a visitor's), so only the status tells.
		getAuthStatus.mockResolvedValue({ mode: 'accounts', setup_required: false, anonymous_access: 'viewer' });
		await recheckSession();
		expect(auth.phase).toBe('signed-out');
		expect(auth.account).toBeNull();
		browseAsVisitor();
		expect(auth.phase).toBe('visitor');
	});

	it('keeps a live session, with its current role, on a recheck', async () => {
		auth.phase = 'signed-in';
		auth.account = ADA;
		getAuthStatus.mockResolvedValue({ mode: 'accounts', setup_required: false, account: { ...ADA, role: 'viewer' } });
		await recheckSession();
		expect(auth.phase).toBe('signed-in');
		expect(currentRole()).toBe('viewer');
	});

	it('keeps the session when the recheck cannot reach the server', async () => {
		auth.phase = 'signed-in';
		auth.account = ADA;
		getAuthStatus.mockRejectedValue(new TypeError('Failed to fetch'));
		await recheckSession();
		expect(auth.phase).toBe('signed-in');
		expect(auth.account).toEqual(ADA);
	});

	it.each(['visitor', 'signed-out', 'open'] as const)('does not recheck when %s', async (phase) => {
		auth.phase = phase;
		await recheckSession();
		expect(getAuthStatus).not.toHaveBeenCalled();
		expect(auth.phase).toBe(phase);
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

	describe('preview (#1413)', () => {
		const previewCookie = () =>
			document.cookie
				.split('; ')
				.find((c) => c.startsWith('myfamily_preview='))
				?.slice('myfamily_preview='.length);

		it('previews as a viewer: the cookie is set and the owner gets a viewer\'s role', () => {
			auth.phase = 'signed-in';
			auth.account = ADA;
			startPreview('viewer');
			expect(previewCookie()).toBe('viewer');
			expect(previewing()).toBe('viewer');
			expect(currentRole()).toBe('viewer');
			expect(can('editor')).toBe(false);
			expect(isVisitor()).toBe(false);
			expect(canChooseHomePerson()).toBe(true);

			stopPreview();
			expect(previewCookie()).toBeUndefined();
			expect(previewing()).toBeNull();
			expect(currentRole()).toBe('owner');
		});

		it('previews as a visitor, in open mode too', () => {
			auth.phase = 'open';
			startPreview('visitor');
			expect(currentRole()).toBe('viewer');
			expect(isVisitor()).toBe(true);
			expect(canChooseHomePerson()).toBe(false);
		});

		it.each(['editor', 'viewer'] as const)('ignores a preview for a signed-in %s, as the server does', (role) => {
			auth.phase = 'signed-in';
			auth.account = { ...ADA, role };
			auth.previewAs = 'visitor';
			expect(previewing()).toBeNull();
			expect(currentRole()).toBe(role);
			expect(isVisitor()).toBe(false);
		});

		it('resumes the preview the cookie holds when the app loads', async () => {
			document.cookie = 'myfamily_preview=visitor; Path=/';
			getAuthStatus.mockResolvedValue({ mode: 'accounts', setup_required: false, account: ADA });
			await loadAuth();
			expect(previewing()).toBe('visitor');
		});

		it('clears a cookie that names no principal', async () => {
			document.cookie = 'myfamily_preview=stranger; Path=/';
			getAuthStatus.mockResolvedValue({ mode: 'accounts', setup_required: false, account: ADA });
			await loadAuth();
			expect(previewing()).toBeNull();
			expect(previewCookie()).toBeUndefined();
		});

		describe('Secure attribute', () => {
			let written: string[];

			beforeEach(() => {
				written = [];
				vi.spyOn(Document.prototype, 'cookie', 'set').mockImplementation((value: string) => {
					written.push(value);
				});
			});

			afterEach(() => {
				vi.restoreAllMocks();
				vi.unstubAllGlobals();
			});

			it.each([
				['https:', true],
				['http:', false]
			])('over %s the cookie is Secure: %s', (protocol, secure) => {
				vi.stubGlobal('location', { ...window.location, protocol });
				startPreview('viewer');
				stopPreview();
				expect(written).toHaveLength(2);
				for (const cookie of written) {
					expect(cookie.split('; ').includes('Secure')).toBe(secure);
				}
			});
		});

		it.each([
			['signIn', () => signIn('ada', 'pw'), () => signInMock.mockResolvedValue(ADA)],
			['completeSetup', () => completeSetup({ token: 't', username: 'ada', display_name: 'Ada', password: 'pw' }), () => setupOwner.mockResolvedValue(ADA)]
		])('a session lost without a sign-out leaves no preview for the next %s', async (_name, signInAgain, answer) => {
			auth.phase = 'signed-in';
			auth.account = ADA;
			startPreview('visitor');
			markSignedOut();
			answer();
			await signInAgain();
			expect(previewCookie()).toBeUndefined();
			expect(previewing()).toBeNull();
			expect(currentRole()).toBe('owner');
		});

		it('a reload after the session expired, then a sign-in, starts with no preview', async () => {
			document.cookie = 'myfamily_preview=viewer; Path=/';
			getAuthStatus.mockResolvedValue({ mode: 'accounts', setup_required: false });
			await loadAuth();
			signInMock.mockResolvedValue(ADA);
			await signIn('ada', 'pw');
			expect(previewCookie()).toBeUndefined();
			expect(previewing()).toBeNull();
		});

		it('ends the preview on sign-out', async () => {
			auth.phase = 'signed-in';
			auth.account = ADA;
			startPreview('viewer');
			signOutMock.mockResolvedValue(undefined);
			await signOut();
			expect(auth.previewAs).toBeNull();
			expect(previewCookie()).toBeUndefined();
		});
	});
});
