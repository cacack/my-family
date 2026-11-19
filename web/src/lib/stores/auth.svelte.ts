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
 * - `signed-in`: `account` is who
 * - `error`: the status could not be read; `retry` with `loadAuth`
 */
import { api, setUnauthorizedHandler, type Account, type SetupRequest } from '#lib/api/client.js';

export type AuthPhase = 'loading' | 'open' | 'setup' | 'signed-out' | 'signed-in' | 'error';

interface AuthState {
	phase: AuthPhase;
	account: Account | null;
}

export const auth = $state<AuthState>({ phase: 'loading', account: null });

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
		} else if (status.account) {
			signedIn(status.account);
		} else {
			auth.account = null;
			auth.phase = 'signed-out';
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

/** Sign out on the server, then show the sign-in page whatever it answered. */
export async function signOut(): Promise<void> {
	try {
		await api.signOut();
	} finally {
		auth.account = null;
		auth.phase = 'signed-out';
	}
}
