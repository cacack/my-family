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

/**
 * Ids and names in a seed come back from HTTP responses, and this file writes
 * them to disk. Each one is checked against the shape it is supposed to have,
 * and what gets written is the *matched* text rather than the response string.
 *
 * The point is a better failure: an API that starts returning a differently
 * shaped id fails here, naming the field, instead of surfacing three files away
 * as a locator that mysteriously matches nothing. It also keeps response text
 * from flowing directly into `writeFileSync`, which is what CodeQL's
 * `js/http-to-file-access` rule objects to.
 */
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
/** Human-readable text the UI renders: names, places, branch labels. */
const DISPLAY = /^[\p{L}\p{N} '&.,()-]{1,120}$/u;
/** A snake_case API field name, e.g. `birth_place`. */
const FIELD = /^[a-z][a-z0-9_]{0,39}$/;

function matched(pattern: RegExp, value: string, field: string): string {
	const found = pattern.exec(value);
	if (found === null) {
		throw new Error(
			`E2E seed: ${field} is not the shape this suite expects: ${JSON.stringify(value)}`
		);
	}
	return found[0];
}

const asId = (value: string, field: string) => matched(UUID, value, field);
const asText = (value: string, field: string) => matched(DISPLAY, value, field);

function checkPerson(person: SeededPerson, field: string): SeededPerson {
	return {
		id: asId(person.id, `${field}.id`),
		name: asText(person.name, `${field}.name`),
		mainBirthPlace: asText(person.mainBirthPlace, `${field}.mainBirthPlace`),
		branchBirthPlace: asText(person.branchBirthPlace, `${field}.branchBirthPlace`)
	};
}

export function writeSeed(seed: SeedData): void {
	const checked: SeedData = {
		switcher: {
			branchId: asId(seed.switcher.branchId, 'switcher.branchId'),
			branchName: asText(seed.switcher.branchName, 'switcher.branchName'),
			person: checkPerson(seed.switcher.person, 'switcher.person')
		},
		merge: {
			branchId: asId(seed.merge.branchId, 'merge.branchId'),
			branchName: asText(seed.merge.branchName, 'merge.branchName'),
			person: checkPerson(seed.merge.person, 'merge.person'),
			conflictField: matched(FIELD, seed.merge.conflictField, 'merge.conflictField'),
			familyId: asId(seed.merge.familyId, 'merge.familyId'),
			familyName: asText(seed.merge.familyName, 'merge.familyName'),
			familyBranchMarriagePlace: asText(
				seed.merge.familyBranchMarriagePlace,
				'merge.familyBranchMarriagePlace'
			)
		}
	};

	mkdirSync(OUTPUT_DIR, { recursive: true });
	writeFileSync(SEED_FILE, JSON.stringify(checked, null, 2), 'utf-8');
}

export function readSeed(): SeedData {
	return JSON.parse(readFileSync(SEED_FILE, 'utf-8')) as SeedData;
}
