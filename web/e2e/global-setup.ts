/**
 * Seeds the fixture the E2E specs run against, over the real HTTP API.
 *
 * Nothing here goes through the UI: the point of this suite is that the browser
 * finds a real stack already holding real data, so the setup uses the same
 * endpoints the app does and no others.
 *
 * The binary wires in-memory stores unconditionally (`cmd/myfamily/main.go`), so
 * every boot is an empty database. That is what makes this setup safe to write
 * unconditionally - there is nothing to clean up and nothing to collide with.
 *
 * Two ordering rules are load-bearing:
 *
 * 1. **Every mainline entity is created before either branch is.** A branch's
 *    first write to a stream continues main's version line *as of its base
 *    position* (`memory.EventStore.seedVersion`). An entity created after the
 *    fork seeds at version 0, so a branch-scoped update quoting the version the
 *    create returned is refused with a version conflict.
 * 2. **The branch-side edit precedes the conflicting mainline edit.** Both quote
 *    the version the create returned; doing main first would move main's line
 *    on and leave the branch write quoting a stale version.
 */
import { API_BASE, writeSeed, type SeedData } from './seed';

interface CreatedPerson {
	id: string;
	given_name?: string;
	surname?: string;
	version: number;
}

interface CreatedFamily {
	id: string;
	version: number;
}

interface CreatedBranch {
	id: string;
	name: string;
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
	const response = await fetch(`${API_BASE}${path}`, {
		method,
		headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
		body: body === undefined ? undefined : JSON.stringify(body)
	});
	if (!response.ok) {
		throw new Error(
			`E2E seed: ${method} ${path} responded ${response.status}: ${await response.text()}`
		);
	}
	return (await response.json()) as T;
}

/** The name the UI renders, mirroring `formatPersonName` in the API client. */
function displayName(person: CreatedPerson): string {
	return [person.given_name, person.surname]
		.map((part) => (part ?? '').trim())
		.filter((part) => part.length > 0)
		.join(' ');
}

export default async function globalSetup(): Promise<void> {
	// --- Mainline ---------------------------------------------------------
	const switcherPerson = await call<CreatedPerson>('POST', '/persons', {
		given_name: 'Wilhelmina',
		surname: 'Hartwell',
		birth_place: 'Springfield, Illinois'
	});
	const mergePerson = await call<CreatedPerson>('POST', '/persons', {
		given_name: 'Bartholomew',
		surname: 'Hartwell',
		birth_place: 'Old Harbour'
	});
	const family = await call<CreatedFamily>('POST', '/families', {
		partner1_id: switcherPerson.id,
		partner2_id: mergePerson.id,
		relationship_type: 'marriage',
		marriage_place: 'Old Chapel'
	});

	// --- Branches (after every mainline create - see rule 1 above) ---------
	const switcherBranch = await call<CreatedBranch>('POST', '/branches', {
		name: 'E2E Switcher Line',
		description: 'Branch the switcher smoke scopes to'
	});
	const mergeBranch = await call<CreatedBranch>('POST', '/branches', {
		name: 'E2E Merge Line',
		description: 'Branch the merge review smoke promotes'
	});

	// --- Branch-scoped edits ----------------------------------------------
	const switcherBranchBirthPlace = 'Branchside Cottage';
	await call('PUT', `/persons/${switcherPerson.id}?branch=${switcherBranch.id}`, {
		birth_place: switcherBranchBirthPlace,
		version: switcherPerson.version
	});

	const mergeBranchBirthPlace = 'Branch View';
	await call('PUT', `/persons/${mergePerson.id}?branch=${mergeBranch.id}`, {
		birth_place: mergeBranchBirthPlace,
		version: mergePerson.version
	});

	const familyBranchMarriagePlace = 'Branch Chapel';
	await call('PUT', `/families/${family.id}?branch=${mergeBranch.id}`, {
		marriage_place: familyBranchMarriagePlace,
		version: family.version
	});

	// --- The conflicting mainline edit (see rule 2 above) ------------------
	// Same entity, same field as the merge branch's edit, so the comparison
	// reports an `edit_edit` the review UI has to make the user decide.
	const mergeMainBirthPlace = 'Mainline Manor';
	await call('PUT', `/persons/${mergePerson.id}`, {
		birth_place: mergeMainBirthPlace,
		version: mergePerson.version
	});

	const seed: SeedData = {
		switcher: {
			branchId: switcherBranch.id,
			branchName: switcherBranch.name,
			person: {
				id: switcherPerson.id,
				name: displayName(switcherPerson),
				mainBirthPlace: 'Springfield, Illinois',
				branchBirthPlace: switcherBranchBirthPlace
			}
		},
		merge: {
			branchId: mergeBranch.id,
			branchName: mergeBranch.name,
			person: {
				id: mergePerson.id,
				name: displayName(mergePerson),
				mainBirthPlace: mergeMainBirthPlace,
				branchBirthPlace: mergeBranchBirthPlace
			},
			conflictField: 'birth_place',
			familyId: family.id,
			familyName: `${displayName(switcherPerson)} & ${displayName(mergePerson)}`,
			familyBranchMarriagePlace
		}
	};

	writeSeed(seed);
}
