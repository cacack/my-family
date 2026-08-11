/**
 * Active Research Branch Store
 *
 * Holds the research branch every branch-scoped read and write is routed to
 * (null = mainline), persisted to localStorage so a reload keeps the user where
 * they were.
 *
 * Three things about the lifecycle are load-bearing:
 *
 * 1. The id is hydrated **synchronously** at module load and handed straight to
 *    the API client. Route pages fetch as soon as they mount, well before any
 *    async revalidation could finish, and a request sent in that window would
 *    silently read the mainline while the UI claimed to be on a branch.
 * 2. Revalidation then runs asynchronously. A branch that is gone, `merged`, or
 *    `archived` has no overlay rows left, so every scoped read against it would
 *    404 — a stale localStorage value would otherwise break every person and
 *    family page. Those fall back to the mainline with a visible notice.
 *    Only a 404 or a terminal status drops the branch: a 503 (no branch
 *    registry configured), a 500 or an offline blip say nothing about whether
 *    the branch still exists, so they leave the scope alone and surface a
 *    softer "couldn't confirm" state that a later call can clear.
 * 3. Every path that changes scope out from under a mounted page reloads, and a
 *    reload wipes in-memory state. The notice explaining an involuntary drop is
 *    therefore persisted to sessionStorage first and consumed on the next load,
 *    or the user would get a silent scope change.
 *
 * The exported `activeBranch` is a read-only view. The setters below are the
 * only mutators, so the store and the API client's own `activeBranchId` cannot
 * drift into disagreeing about what scope the app is in.
 *
 * Persistence happens imperatively in the setters rather than through an
 * `$effect`, so the store can be exercised without a reactive root.
 */

import { api, setClientBranch, type ApiError, type Branch } from '$lib/api/client';

const STORAGE_KEY = 'active-branch';
/**
 * Where an involuntary-drop notice waits out the reload. sessionStorage, not
 * localStorage: the message is about this tab's scope changing and must not
 * outlive the tab.
 */
const NOTICE_KEY = 'active-branch-notice';

interface ActiveBranchState {
	/** Branch id the API client is scoping to; null means the mainline. */
	id: string | null;
	/** The full branch record, once loaded. Null while revalidating or on mainline. */
	branch: Branch | null;
	/** True while startup revalidation is in flight. */
	revalidating: boolean;
	/**
	 * True when revalidation failed for a reason that says nothing about the
	 * branch (503/500/offline). The scope is unchanged; only our confidence in
	 * it is.
	 */
	unconfirmed: boolean;
	/** Why a persisted branch was dropped, for the banner to explain. */
	notice: string | null;
}

const state = $state<ActiveBranchState>({
	id: null,
	branch: null,
	revalidating: false,
	unconfirmed: false,
	notice: null
});

function readStoredBranchId(): string | null {
	if (typeof window === 'undefined') {
		return null;
	}
	try {
		const stored = localStorage.getItem(STORAGE_KEY);
		return stored ? stored : null;
	} catch {
		// Storage unavailable - mainline is the safe default.
		return null;
	}
}

/** True when the new value is what a reload would read back. */
function persistBranchId(id: string | null): boolean {
	if (typeof window === 'undefined') {
		return false;
	}
	try {
		if (id === null) {
			localStorage.removeItem(STORAGE_KEY);
		} else {
			localStorage.setItem(STORAGE_KEY, id);
		}
		return true;
	} catch {
		// Storage full or unavailable - the in-memory scope still applies.
		return false;
	}
}

function persistNotice(notice: string): void {
	if (typeof window === 'undefined') {
		return;
	}
	try {
		sessionStorage.setItem(NOTICE_KEY, notice);
	} catch {
		// Storage full or unavailable - the in-memory notice still shows if the
		// reload never happens.
	}
}

/** Read the pending notice and clear it, so it is shown exactly once. */
function takePersistedNotice(): string | null {
	if (typeof window === 'undefined') {
		return null;
	}
	try {
		const stored = sessionStorage.getItem(NOTICE_KEY);
		sessionStorage.removeItem(NOTICE_KEY);
		return stored ? stored : null;
	} catch {
		return null;
	}
}

// Hydrate before anything can issue a request (see note 1 above).
const storedBranchId = readStoredBranchId();
if (storedBranchId !== null) {
	state.id = storedBranchId;
	setClientBranch(storedBranchId);
}
// Pick up a notice left by the reload that dropped the branch (note 3).
state.notice = takePersistedNotice();

/**
 * Fall back to the mainline, explaining why, and reload.
 *
 * The reload is the same argument `switchBranch` makes: the mounted route is
 * still showing rows fetched under the old scope. Without it a branch merged or
 * archived in another tab leaves the user staring at a page of 404s. The notice
 * is persisted first so it survives the reload.
 *
 * The reload is conditional on the id actually having been cleared from
 * storage. If localStorage is unwritable the next load would re-hydrate the
 * same dead branch and drop it again — an endless reload. In that case the
 * in-memory fallback and the notice stand on their own.
 */
function dropStaleBranch(notice: string): void {
	state.id = null;
	state.branch = null;
	state.unconfirmed = false;
	setClientBranch(null);
	const cleared = persistBranchId(null);
	state.notice = notice;
	persistNotice(notice);
	if (cleared && typeof window !== 'undefined') {
		window.location.reload();
	}
}

// Plain boolean, not `$state`: it guards against re-entry when the layout's
// `$effect` re-runs, and making it reactive would feed that effect back into
// itself.
let revalidationStarted = false;

/**
 * Confirm the persisted branch still exists and still accepts writes. Safe to
 * call repeatedly; only the first call does work, except after an inconclusive
 * failure, which leaves the door open for a retry. No-op on the mainline.
 */
export async function revalidateActiveBranch(): Promise<void> {
	if (revalidationStarted) {
		return;
	}
	revalidationStarted = true;

	const id = state.id;
	if (id === null) {
		return;
	}

	state.revalidating = true;
	try {
		const branch = await api.getBranch(id);
		if (branch.status === 'active') {
			state.branch = branch;
			state.unconfirmed = false;
		} else {
			dropStaleBranch(
				`Research branch "${branch.name}" is ${branch.status} and accepts no further changes. You are back on the mainline.`
			);
		}
	} catch (e) {
		if ((e as ApiError)?.status === 404) {
			dropStaleBranch(
				'The research branch you were working on is no longer available. You are back on the mainline.'
			);
		} else {
			// A 503, a 500 or a dropped connection is evidence about the server,
			// not about the branch. Discarding the user's branch selection over
			// it would be data loss dressed up as safety.
			state.unconfirmed = true;
			revalidationStarted = false;
		}
	} finally {
		state.revalidating = false;
	}
}

/**
 * Scope subsequent reads and writes to `branch`, or to the mainline when null.
 *
 * The page is then reloaded, because the already-mounted route is still showing
 * rows fetched under the previous scope and leaving them there is the UI lying
 * about what the user is looking at.
 *
 * A reload rather than SvelteKit's `invalidateAll()`: this app has no `load`
 * functions at all (`src/routes/+layout.ts` turns off SSR and every route
 * fetches from a client-side `$effect`), so `invalidateAll()` would invalidate
 * nothing. The branch id is persisted before the reload, so it survives.
 */
export function switchBranch(branch: Branch | null): void {
	const id = branch?.id ?? null;
	state.id = id;
	state.branch = branch;
	state.notice = null;
	state.unconfirmed = false;
	// A branch chosen explicitly needs no startup revalidation.
	revalidationStarted = true;
	setClientBranch(id);
	// Only reload if the new id actually reached storage. If localStorage is
	// unwritable, the reload would re-hydrate the PREVIOUS id and silently undo
	// the switch the user just asked for — the same trap dropStaleBranch guards
	// against above. Skipping the reload leaves the in-memory scope in force, so
	// later requests still go to the chosen branch; only already-rendered rows
	// are stale, which is the lesser of the two wrongs.
	const persisted = persistBranchId(id);
	if (persisted && typeof window !== 'undefined') {
		window.location.reload();
	}
}

/** Return to the mainline. */
export function returnToMainline(): void {
	switchBranch(null);
}

/** Dismiss the stale-branch notice once the user has read it. */
export function dismissBranchNotice(): void {
	state.notice = null;
}

/**
 * Read-only view of the active branch.
 *
 * Getter-only on purpose: `activeBranch.id = x` from a consumer would leave the
 * API client still scoped to the old branch, so the banner would name one scope
 * while every request used another. Go through `switchBranch`,
 * `returnToMainline` or `dismissBranchNotice` instead.
 */
export const activeBranch: Readonly<ActiveBranchState> = {
	get id() {
		return state.id;
	},
	get branch() {
		return state.branch;
	},
	get revalidating() {
		return state.revalidating;
	},
	get unconfirmed() {
		return state.unconfirmed;
	},
	get notice() {
		return state.notice;
	}
};
