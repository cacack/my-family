/**
 * Auth Store
 *
 * Who is using the app (ADR-007). The root layout loads it first and shows the
 * setup page, the sign-in page or the app accordingly. `phase` is:
 *
 * - `loading`: the status has not arrived yet
 * - `open`: the server runs with AUTH_MODE=open; no one signs in
 * - `setup`: the tree has no owner yet
 * - `signed-out`: sign in to continue
 * - `visitor`: not signed in, browsing as a viewer because the owner lets
 *   visitors in (`auth.visitorsWelcome`)
 * - `signed-in`: `account` is who
 * - `error`: the status could not be read; `retry` with `loadAuth`
 */
import {
	api,
	setUnauthorizedHandler,
	type AcceptInviteRequest,
	type Account,
	type AccountUpdate,
	type SetupRequest
} from '#lib/api/client.js';
import { auth, type AuthPhase } from './access.svelte.js';

export { auth, can, canChooseHomePerson, currentRole, type AuthPhase, type Role } from './access.svelte.js';

function signedIn(account: Account): void {
	auth.account = account;
	auth.phase = 'signed-in';
}

/** The session ended on the server (expired or revoked): show the sign-in page. */
export function markSignedOut(): void {
	if (auth.phase === 'signed-in') {
		auth.account = null;
		auth.phase = 'signed-out';
	}
}

setUnauthorizedHandler(markSignedOut);

/**
 * Ask the server whether a signed-in user is still signed in, and show the
 * sign-in page if not. While visitors are let in, an expired session's reads
 * are served as a visitor's rather than refused, so no 401 would tell the app
 * until a write; the root layout calls this whenever the window regains focus.
 */
export async function recheckSession(): Promise<void> {
	if (auth.phase !== 'signed-in') return;
	try {
		const status = await api.getAuthStatus();
		auth.visitorsWelcome = status.anonymous_access === 'viewer';
		if (status.account) {
			auth.account = status.account;
		} else {
			markSignedOut();
		}
	} catch {
		// Unreachable for now: keep the session; the next request will tell.
	}
}

/** Where someone who is not signed in lands: browsing as a visitor, or the sign-in page. */
function signedOutPhase(): AuthPhase {
	return auth.visitorsWelcome ? 'visitor' : 'signed-out';
}

/** A visitor asks to sign in: show the sign-in page. */
export function showSignIn(): void {
	if (auth.phase === 'visitor') auth.phase = 'signed-out';
}

/** Back from the sign-in page to browsing, when visitors are welcome. */
export function browseAsVisitor(): void {
	if (auth.phase === 'signed-out' && auth.visitorsWelcome) auth.phase = 'visitor';
}

/** Read how this server signs people in and who is signed in. */
export async function loadAuth(): Promise<void> {
	try {
		const status = await api.getAuthStatus();
		if (status.mode === 'open') {
			auth.account = null;
			auth.phase = 'open';
		} else if (status.setup_required) {
			auth.account = null;
			auth.phase = 'setup';
		} else {
			auth.visitorsWelcome = status.anonymous_access === 'viewer';
			if (status.account) {
				signedIn(status.account);
			} else {
				auth.account = null;
				auth.phase = signedOutPhase();
			}
		}
	} catch {
		auth.phase = 'error';
	}
}

/** Sign in; throws the API error (wrong password, rate limited) for the form to show. */
export async function signIn(username: string, password: string): Promise<void> {
	signedIn(await api.signIn({ username, password }));
}

/** Create the first owner and sign them in; throws the API error for the form to show. */
export async function completeSetup(input: SetupRequest): Promise<void> {
	signedIn(await api.setupOwner(input));
}

/** Create an account from an invite link and sign it in; throws the API error for the form to show. */
export async function acceptInvite(input: AcceptInviteRequest): Promise<void> {
	signedIn(await api.acceptInvite(input));
}

/** Change the signed-in user's display name or password; throws the API error for the form to show. */
export async function updateOwnAccount(input: AccountUpdate): Promise<void> {
	signedIn(await api.updateCurrentAccount(input));
}

/**
 * Sign out on the server, then show the sign-in page (or the tree, to a
 * visitor) whatever it answered.
 */
export async function signOut(): Promise<void> {
	try {
		await api.signOut();
	} finally {
		auth.account = null;
		auth.phase = signedOutPhase();
	}
}
