/**
 * Access Store
 *
 * Who is using the app and what their role lets them do (ADR-007). The auth
 * store (`auth.svelte.ts`) drives `auth`; this module imports nothing at
 * runtime, so any component can ask `can(...)` without pulling in the API
 * client.
 */
import type { Account } from '#lib/api/client.js';

export type AuthPhase = 'loading' | 'open' | 'setup' | 'signed-out' | 'visitor' | 'signed-in' | 'error';

/** What a user may do in the tree; each role includes the ones before it. */
export type Role = Account['role'];

/** Who the owner may preview the tree as (#1413). */
export type PreviewAs = 'viewer' | 'visitor';

const ROLE_RANK: Record<Role, number> = { viewer: 1, editor: 2, owner: 3 };

interface AuthState {
	phase: AuthPhase;
	account: Account | null;
	/** The owner lets visitors browse without signing in (anonymous access `viewer`). */
	visitorsWelcome: boolean;
	/**
	 * Who the owner previews the tree as, or null. The auth store keeps it in
	 * step with the preview cookie, which makes the server answer as them.
	 */
	previewAs: PreviewAs | null;
}

export const auth = $state<AuthState>({ phase: 'loading', account: null, visitorsWelcome: false, previewAs: null });

/**
 * Whether sign-in is switched off on this instance (open mode, ADR-007):
 * whoever uses the app acts as the owner and has no account of their own.
 * Reactive.
 */
export function isOpenMode(): boolean {
	return auth.phase === 'open';
}

/** The role the account holds: the owner's in open mode, a viewer's for a visitor. */
function accountRole(): Role | null {
	if (isOpenMode()) return 'owner';
	if (auth.phase === 'visitor') return 'viewer';
	return auth.phase === 'signed-in' ? (auth.account?.role ?? null) : null;
}

/**
 * Who the owner previews the tree as, or null when they are not previewing.
 * The server honours the preview for the owner only, so it is null for
 * anyone else. Reactive.
 */
export function previewing(): PreviewAs | null {
	return accountRole() === 'owner' ? auth.previewAs : null;
}

/**
 * The role of whoever is using the app: the signed-in user's, the owner's in
 * open mode, a viewer's for a visitor or while the owner previews the tree,
 * or null before anyone is signed in.
 */
export function currentRole(): Role | null {
	return previewing() ? 'viewer' : accountRole();
}

/**
 * Whether a visitor is browsing without signing in. The server refuses a
 * visitor the change log (history, restore points, snapshots and
 * comparisons), whose records name who made each change (ADR-008 §4), so the
 * app hides it from them, and from the owner previewing as a visitor.
 * Reactive, like `can`.
 */
export function isVisitor(): boolean {
	return auth.phase === 'visitor' || previewing() === 'visitor';
}

/**
 * Whether the current role includes `required`. Controls the server would
 * refuse are hidden with it, so it mirrors each operation's `x-min-role`:
 * `editor` changes data, works in branches and exports; `owner` imports and
 * rolls back. Reactive: read it in a `$derived` or the markup.
 */
export function can(required: Role): boolean {
	const role = currentRole();
	return role !== null && ROLE_RANK[role] >= ROLE_RANK[required];
}

/**
 * Whether whoever is using the app has a home person ("me") of their own to
 * choose: each signed-in account does, and the site does in open mode. A
 * visitor has none, so the server refuses them the control (#1443). Reactive.
 */
export function canChooseHomePerson(): boolean {
	return (isOpenMode() || auth.phase === 'signed-in') && previewing() !== 'visitor';
}
