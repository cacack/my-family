/**
 * Home Person Store
 *
 * The person marked as "me": person pages state their relationship to them and
 * charts start from them. The server keeps it (`/settings/home-person`) for
 * each signed-in account, or for the site in open mode; a visitor has none
 * (#1443).
 *
 * Loaded by the root layout whenever who is viewing changes, and reloaded
 * after a delete or merge that may have removed the home person, since the
 * server moves or drops them then (#1444). The setters are the only other
 * mutators.
 *
 * Loads are not awaited, so they can overlap each other and the setters. Each
 * call takes a ticket, and a load's answer is kept only when nothing started
 * after it: a slow answer never replaces a newer choice, or reaches someone
 * who signed in after it was asked for.
 */

import { api, type Person } from '#lib/api/client.js';

interface HomePersonState {
	/** The home person, or null when none is set (or not loaded yet). */
	person: Person | null;
	/** Whether the first load has finished, so pages can tell "unset" from "not yet known". */
	loaded: boolean;
}

const state = $state<HomePersonState>({ person: null, loaded: false });

/** The latest load, set or clear; plain, so reading it in an effect tracks nothing. */
let latest = 0;

/** Read-only view of the home person. */
export const homePerson: Readonly<HomePersonState> = state;

/**
 * Loads, or reloads, the home person. A failure leaves it unset: a missing "me" only hides
 * the relationship line, so it is not worth an error on every page.
 */
export async function loadHomePerson(): Promise<void> {
	const ticket = ++latest;
	let person: Person | null = null;
	try {
		person = (await api.getHomePerson()).person ?? null;
	} catch {
		// Leave it unset.
	}
	if (ticket !== latest) return;
	state.person = person;
	state.loaded = true;
}

/**
 * Forgets the home person until the next load, for a new viewer: the last
 * one's choice may be a person this one may not see.
 */
export function resetHomePerson(): void {
	latest++;
	state.person = null;
	state.loaded = false;
}

/**
 * Marks personId as the home person. Throws the API error so the caller can show it.
 * Resolving means the server saved it, not that the store shows it: when a newer
 * load, set, clear or reset started meanwhile, that one's answer stands instead.
 */
export async function setHomePerson(personId: string): Promise<void> {
	const ticket = ++latest;
	const res = await api.setHomePerson(personId);
	if (ticket !== latest) return;
	state.person = res.person ?? null;
	state.loaded = true;
}

/**
 * Clears the home person. Throws the API error so the caller can show it. Like
 * setHomePerson, a newer call started meanwhile decides what the store shows.
 */
export async function clearHomePerson(): Promise<void> {
	const ticket = ++latest;
	await api.clearHomePerson();
	if (ticket !== latest) return;
	state.person = null;
	state.loaded = true;
}
