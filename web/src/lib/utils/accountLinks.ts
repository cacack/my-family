/**
 * The links an owner passes on by hand (ADR-007: no email is sent): an invite
 * opens `/join`, a password reset `/reset-password`. The token rides in the
 * URL fragment, which the browser never sends to the server, so it stays out
 * of request logs. `myfamily user reset-password` prints the same reset link.
 */

/** The pages that take a link's token, shown whether or not anyone is signed in. */
export const LINK_PAGES = new Set(['/join', '/reset-password']);

function link(path: string, token: string, origin: string): string {
	return `${origin}${path}#token=${encodeURIComponent(token)}`;
}

/** The invite link for `token`, on this server. */
export function inviteLink(token: string, origin = window.location.origin): string {
	return link('/join', token, origin);
}

/** The password reset link for `token`, on this server. */
export function resetLink(token: string, origin = window.location.origin): string {
	return link('/reset-password', token, origin);
}

/** The token in a link's fragment (`#token=...`), or '' when there is none. */
export function tokenFromHash(hash: string): string {
	return new URLSearchParams(hash.replace(/^#/, '')).get('token') ?? '';
}
