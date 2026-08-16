/**
 * Shared wiring between the Playwright config, the global setup that seeds the
 * fixture over the real API, and the specs that read it back.
 *
 * The seeded ids travel through a JSON file rather than a module-level export:
 * global setup and every worker are separate processes, so anything assigned in
 * memory here would be `undefined` by the time a spec looked at it.
 */
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));

/**
 * Deliberately not 8080: a developer's `make run` or `npm run dev` lives there,
 * and a suite that quietly attached to it would seed a real database.
 */
export const E2E_PORT = Number(process.env.E2E_PORT ?? 8181);
export const BASE_URL = `http://127.0.0.1:${E2E_PORT}`;
export const API_BASE = `${BASE_URL}/api/v1`;

/** Playwright's `outputDir`. It is wiped once *before* global setup, never after. */
export const OUTPUT_DIR = resolve(here, '..', 'test-results');
const SEED_FILE = resolve(OUTPUT_DIR, 'seed.json');

/** A person seeded on main and then edited on a branch. */
export interface SeededPerson {
	id: string;
	/** As `formatPersonName` renders it, which is what the UI shows. */
	name: string;
	/** `birth_place` as the mainline has it. */
	mainBirthPlace: string;
	/** `birth_place` as the branch has it - the branch-only value. */
	branchBirthPlace: string;
}

export interface SeedData {
	/**
	 * A branch touched by nothing else, so the switcher smoke's assertions about
	 * its person hold no matter what order the specs run in.
	 */
	switcher: {
		branchId: string;
		branchName: string;
		person: SeededPerson;
	};
	/**
	 * A second branch, with its own person, carrying a real `edit_edit` conflict
	 * for the merge review to render and resolve. Merging it is terminal, which
	 * is precisely why it shares no entity with `switcher`.
	 */
	merge: {
		branchId: string;
		branchName: string;
		/** Edited on both sides, on the same field - the conflict. */
		person: SeededPerson;
		/** The contested field, as the conflict reports it. */
		conflictField: string;
		/** Edited on the branch only, so the diff has a second entity type. */
		familyId: string;
		familyName: string;
		familyBranchMarriagePlace: string;
	};
}

export function writeSeed(seed: SeedData): void {
	mkdirSync(OUTPUT_DIR, { recursive: true });
	writeFileSync(SEED_FILE, JSON.stringify(seed, null, 2), 'utf-8');
}

export function readSeed(): SeedData {
	return JSON.parse(readFileSync(SEED_FILE, 'utf-8')) as SeedData;
}
