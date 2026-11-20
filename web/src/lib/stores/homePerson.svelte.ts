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
 * Loads are not awaited, so they can overlap each other and the setters. A
 * load's answer is kept only when no newer load, reset or saved choice came
 * after it was asked for; a setter's answer only when no newer set, clear or
 * reset was started. So a slow answer never replaces a newer choice, or reaches
 * someone who signed in after it was asked for, while a set or clear that
 * fails, or is still in flight, never throws away a load's answer.
 */

import { api, type Person } from '#lib/api/client.js';

interface HomePersonState {
	/** The home person, or null when none is set (or not loaded yet). */
	person: Person | null;
	/** Whether the first load has finished, so pages can tell "unset" from "not yet known". */
	loaded: boolean;
}

const state = $state<HomePersonState>({ person: null, loaded: false });

// Tickets are plain, so reading them in an effect tracks nothing.
/** Bumped by each load, reset and saved set or clear: a load's answer needs the latest. */
let latestLoad = 0;
/** Bumped by each set, clear and reset as it starts: a setter's answer needs the latest. */
let latestWrite = 0;

/** Read-only view of the home person. */
export const homePerson: Readonly<HomePersonState> = state;

/**
 * Loads, or reloads, the home person. A failure leaves it unset: a missing "me" only hides
 * the relationship line, so it is not worth an error on every page.
 */
export async function loadHomePerson(): Promise<void> {
	const ticket = ++latestLoad;
	let person: Person | null = null;
	try {
		person = (await api.getHomePerson()).person ?? null;
	} catch {
		// Leave it unset.
	}
	if (ticket !== latestLoad) return;
	state.person = person;
	state.loaded = true;
}

/**
 * Forgets the home person until the next load, for a new viewer: the last
 * one's choice may be a person this one may not see. Answers to calls started
 * before it are dropped, and `loaded` stays false, so call loadHomePerson()
 * right after it.
 */
export function resetHomePerson(): void {
	latestLoad++;
	latestWrite++;
	state.person = null;
	state.loaded = false;
}

/**
 * Marks personId as the home person. Throws the API error so the caller can show it.
 * Resolving means the server saved it, not that the store shows it: when a newer
 * set, clear or reset started meanwhile, that one's answer stands instead. Once
 * saved, it replaces the answer of any load still in flight.
 */
export async function setHomePerson(personId: string): Promise<void> {
	const ticket = ++latestWrite;
	const res = await api.setHomePerson(personId);
	if (ticket !== latestWrite) return;
	latestLoad++;
	state.person = res.person ?? null;
	state.loaded = true;
}

/**
 * Clears the home person. Throws the API error so the caller can show it. Like
 * setHomePerson, a newer set, clear or reset started meanwhile decides what the
 * store shows, and once saved it replaces the answer of any load still in flight.
 */
export async function clearHomePerson(): Promise<void> {
	const ticket = ++latestWrite;
	await api.clearHomePerson();
	if (ticket !== latestWrite) return;
	latestLoad++;
	state.person = null;
	state.loaded = true;
}
