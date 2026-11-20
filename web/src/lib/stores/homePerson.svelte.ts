/**
 * Home Person Store
 *
 * The person marked as "me": person pages state their relationship to them and
 * charts start from them. The server keeps it (`/settings/home-person`) for
 * each signed-in account, or for the site in open mode; a visitor has none
 * (#1443).
 *
 * Loaded by the root layout, again whenever who is signed in changes, and
 * reloaded after a delete or merge that may have removed the home person,
 * since the server moves or drops them then (#1444). The setters are the only
 * other mutators.
 */

import { api, type Person } from '#lib/api/client.js';

interface HomePersonState {
	/** The home person, or null when none is set (or not loaded yet). */
	person: Person | null;
	/** Whether the first load has finished, so pages can tell "unset" from "not yet known". */
	loaded: boolean;
}

const state = $state<HomePersonState>({ person: null, loaded: false });

/** Read-only view of the home person. */
export const homePerson: Readonly<HomePersonState> = state;

/**
 * Loads, or reloads, the home person. A failure leaves it unset: a missing "me" only hides
 * the relationship line, so it is not worth an error on every page.
 */
export async function loadHomePerson(): Promise<void> {
	try {
		const res = await api.getHomePerson();
		state.person = res.person ?? null;
	} catch {
		state.person = null;
	} finally {
		state.loaded = true;
	}
}

/** Marks personId as the home person. Throws the API error so the caller can show it. */
export async function setHomePerson(personId: string): Promise<void> {
	const res = await api.setHomePerson(personId);
	state.person = res.person ?? null;
}

/** Clears the home person. Throws the API error so the caller can show it. */
export async function clearHomePerson(): Promise<void> {
	await api.clearHomePerson();
	state.person = null;
}
