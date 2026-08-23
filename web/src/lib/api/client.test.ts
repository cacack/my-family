import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
// Vite inlines the spec at transform time, so the drift test needs neither
// `node:fs` (the `web` package declares no Node type definitions) nor any
// assumption about the working directory.
import openapiSpec from '../../../../internal/api/openapi.yaml?raw';
import { NOTE_MAX_LENGTH } from '$lib/components/MergeConfirmDialog.svelte';
import {
	api,
	formatGenDate,
	isBranchMergeRefusal,
	isBranchScopedRequest,
	setClientBranch,
	getClientBranch,
	type BranchMergeConflictError,
	type BranchMergeResult,
	type GenDate
} from './client';

describe('formatGenDate', () => {
	it('returns the raw string verbatim when present', () => {
		const date: GenDate = {
			raw: 'INT 1850 (about eighteen fifty)',
			qualifier: 'int',
			year: 1850,
			interpreted_from: 'about eighteen fifty'
		};
		expect(formatGenDate(date)).toBe('INT 1850 (about eighteen fifty)');
	});

	it('formats an interpreted date with its original phrase when raw is absent', () => {
		const date: GenDate = {
			qualifier: 'int',
			year: 1850,
			interpreted_from: 'about eighteen fifty'
		};
		expect(formatGenDate(date)).toBe('INT 1850 (about eighteen fifty)');
	});

	it('formats an interpreted date without a phrase', () => {
		const date: GenDate = { qualifier: 'int', year: 1850 };
		expect(formatGenDate(date)).toBe('INT 1850');
	});
});

const PERSON_ID = '11111111-1111-1111-1111-111111111111';
const NAME_ID = '22222222-2222-2222-2222-222222222222';
const FAMILY_ID = '33333333-3333-3333-3333-333333333333';
const BRANCH_ID = '44444444-4444-4444-4444-444444444444';

describe('isBranchScopedRequest', () => {
	it.each([
		['GET', '/persons'],
		['POST', '/persons'],
		['GET', `/persons/${PERSON_ID}`],
		['PUT', `/persons/${PERSON_ID}`],
		['DELETE', `/persons/${PERSON_ID}`],
		['GET', `/persons/${PERSON_ID}/names`],
		['POST', `/persons/${PERSON_ID}/names`],
		['PUT', `/persons/${PERSON_ID}/names/${NAME_ID}`],
		['DELETE', `/persons/${PERSON_ID}/names/${NAME_ID}`],
		['POST', '/families'],
		['GET', `/families/${FAMILY_ID}`],
		['PUT', `/families/${FAMILY_ID}`],
		['DELETE', `/families/${FAMILY_ID}`],
		['POST', `/families/${FAMILY_ID}/children`],
		['DELETE', `/families/${FAMILY_ID}/children/${PERSON_ID}`],
		['GET', `/pedigree/${PERSON_ID}`],
		['GET', '/browse/surnames'],
		['GET', '/browse/surnames/Smith/persons'],
		['GET', '/browse/places'],
		['GET', '/browse/places/Ohio/persons'],
		['GET', '/browse/cemeteries/Oak%20Hill%20Cemetery/persons'],
		['GET', '/map/locations']
	])('allows %s %s', (method, path) => {
		expect(isBranchScopedRequest(method, path)).toBe(true);
	});

	it('matches free-text segments the way the client actually encodes them', () => {
		// The browse client percent-encodes the surname/place segment, so the
		// pattern has to survive `%20`, `%2C`, non-ASCII and an encoded slash
		// without either rejecting the path or spilling across a `/`.
		const place = 'Saint-Étienne, Loire, France / Cimetière';
		expect(encodeURIComponent(place)).not.toContain('/');
		expect(
			isBranchScopedRequest('GET', `/browse/places/${encodeURIComponent(place)}/persons`)
		).toBe(true);
		expect(
			isBranchScopedRequest('GET', `/browse/cemeteries/${encodeURIComponent(place)}/persons`)
		).toBe(true);
		expect(
			isBranchScopedRequest('GET', `/browse/surnames/${encodeURIComponent("O'Brien")}/persons`)
		).toBe(true);
	});

	it('does not let a free-text segment swallow a slash', () => {
		expect(isBranchScopedRequest('GET', '/browse/places/Ohio/Franklin/persons')).toBe(false);
		expect(isBranchScopedRequest('GET', '/browse/surnames//persons')).toBe(false);
	});

	it('leaves the main-only browse operations unscoped', () => {
		// The cemetery index aggregates `life_events`, which has no `branch_id`
		// yet (#757); brick walls are not event-sourced (#761). Sending
		// `?branch=` on these would imply a scoping the server does not apply.
		expect(isBranchScopedRequest('GET', '/browse/cemeteries')).toBe(false);
		expect(isBranchScopedRequest('GET', '/browse/brick-walls')).toBe(false);
		expect(isBranchScopedRequest('PUT', `/persons/${PERSON_ID}/brick-wall`)).toBe(false);
		expect(isBranchScopedRequest('DELETE', `/persons/${PERSON_ID}/brick-wall`)).toBe(false);
	});

	it('does not allow GET /families - listFamilies has no branch parameter', () => {
		expect(isBranchScopedRequest('GET', '/families')).toBe(false);
	});

	it('matches on method, not just path', () => {
		expect(isBranchScopedRequest('DELETE', '/persons')).toBe(false);
		expect(isBranchScopedRequest('GET', `/families/${FAMILY_ID}/children`)).toBe(false);
	});

	it('does not mistake literal person routes for /persons/{id}', () => {
		expect(isBranchScopedRequest('GET', '/persons/duplicates')).toBe(false);
		expect(isBranchScopedRequest('POST', '/persons/merge')).toBe(false);
	});

	it('leaves branch lifecycle and other mainline-only endpoints alone', () => {
		expect(isBranchScopedRequest('GET', '/branches')).toBe(false);
		expect(isBranchScopedRequest('GET', `/branches/${BRANCH_ID}/compare`)).toBe(false);
		expect(isBranchScopedRequest('GET', '/sources')).toBe(false);
		expect(isBranchScopedRequest('GET', `/persons/${PERSON_ID}/history`)).toBe(false);
		expect(isBranchScopedRequest('GET', `/families/${FAMILY_ID}/group-sheet`)).toBe(false);
	});

	it('ignores an existing query string when matching', () => {
		expect(isBranchScopedRequest('GET', '/persons?limit=20&offset=0')).toBe(true);
		expect(isBranchScopedRequest('GET', `/pedigree/${PERSON_ID}?generations=4`)).toBe(true);
	});
});

describe('branch scope threading', () => {
	let fetchMock: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			status: 200,
			json: async () => ({})
		});
		vi.stubGlobal('fetch', fetchMock);
	});

	afterEach(() => {
		setClientBranch(null);
		vi.unstubAllGlobals();
	});

	function requestedUrl(): string {
		return fetchMock.mock.calls[0][0] as string;
	}

	it('sends nothing extra when no branch is active', async () => {
		expect(getClientBranch()).toBeNull();
		await api.listPersons({ limit: 20 });
		expect(requestedUrl()).toBe('/api/v1/persons?limit=20');
	});

	it('appends ?branch= to an allowlisted path with no existing query', async () => {
		setClientBranch(BRANCH_ID);
		await api.getPerson(PERSON_ID);
		expect(requestedUrl()).toBe(`/api/v1/persons/${PERSON_ID}?branch=${BRANCH_ID}`);
	});

	it('joins with & when the path already carries a query string', async () => {
		setClientBranch(BRANCH_ID);
		await api.listPersons({ limit: 20, offset: 40 });
		expect(requestedUrl()).toBe(`/api/v1/persons?limit=20&offset=40&branch=${BRANCH_ID}`);
	});

	it('leaves non-allowlisted requests untouched while a branch is active', async () => {
		setClientBranch(BRANCH_ID);
		await api.listFamilies({ limit: 20 });
		expect(requestedUrl()).toBe('/api/v1/families?limit=20');
	});

	it('never scopes the branch lifecycle endpoints themselves', async () => {
		setClientBranch(BRANCH_ID);
		await api.listBranches();
		expect(requestedUrl()).toBe('/api/v1/branches');
	});
});

describe('mergeBranch', () => {
	const STREAM_ID = '55555555-5555-5555-5555-555555555555';
	// The id is deliberately not URL-safe, so the encodeURIComponent test bites.
	const AWKWARD_ID = 'branch/with space';

	const mergedBranch: BranchMergeResult = {
		branch: {
			id: BRANCH_ID,
			name: 'census-1881',
			base_position: 12,
			status: 'merged',
			created_at: '2026-01-01T00:00:00Z'
		},
		merged_at_position: 128,
		replayed_event_count: 7,
		skipped_stream_ids: []
	};

	let fetchMock: ReturnType<typeof vi.fn>;

	function mockResponse(status: number, body: unknown) {
		fetchMock.mockResolvedValue({
			ok: status >= 200 && status < 300,
			status,
			statusText: '',
			json: async () => body
		});
	}

	beforeEach(() => {
		fetchMock = vi.fn();
		vi.stubGlobal('fetch', fetchMock);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('POSTs to /branches/{id}/merge with the id URL-encoded', async () => {
		mockResponse(200, mergedBranch);
		await api.mergeBranch(AWKWARD_ID);
		expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/branches/branch%2Fwith%20space/merge');
		expect(fetchMock.mock.calls[0][1].method).toBe('POST');
	});

	it('sends an empty body when no request is given', async () => {
		mockResponse(200, mergedBranch);
		await api.mergeBranch(BRANCH_ID);
		expect(fetchMock.mock.calls[0][1].body).toBe('{}');
	});

	it('serializes the note and resolutions it is given', async () => {
		mockResponse(200, mergedBranch);
		await api.mergeBranch(BRANCH_ID, {
			note: 'Confirmed by the 1881 census',
			resolutions: [{ stream_id: STREAM_ID, resolution: 'branch' }]
		});
		expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
			note: 'Confirmed by the 1881 census',
			resolutions: [{ stream_id: STREAM_ID, resolution: 'branch' }]
		});
	});

	it('returns the merge result untouched', async () => {
		mockResponse(200, mergedBranch);
		await expect(api.mergeBranch(BRANCH_ID)).resolves.toEqual(mergedBranch);
	});

	it('throws a 409 merge_conflicts refusal with its conflicts intact', async () => {
		const refusal: BranchMergeConflictError = {
			code: 'merge_conflicts',
			message: '1 of 1 conflicts have no resolution',
			conflicts: [
				{
					stream_id: STREAM_ID,
					supported_resolutions: ['branch', 'main'],
					entity_type: 'person',
					entity_name: 'Ada Lovelace',
					kind: 'edit_edit',
					fields: ['surname'],
					detail: 'Both sides changed surname to different values'
				}
			]
		};
		mockResponse(409, refusal);

		// `request()` rethrows the parsed body as-is with `status` stamped on, so
		// the extra `conflicts` field survives with no extra plumbing.
		await expect(api.mergeBranch(BRANCH_ID)).rejects.toEqual({ ...refusal, status: 409 });
	});
});

describe('isBranchMergeRefusal', () => {
	it.each([
		'merge_conflicts',
		'branch_not_active',
		'merge_already_claimed',
		'branch_too_large',
		'main_too_far_ahead',
		'merge_plan_stale',
		'merge_dangling_reference',
		'merge_partially_applied',
		// The two 400s. Reachable when the merge's own conflict re-detection no
		// longer supports a resolution compare offered, or when the request body
		// itself fails validation - both refuse before anything is written.
		'invalid_resolution',
		'validation_error'
	])('recognises %s', (code) => {
		expect(isBranchMergeRefusal({ code, message: 'refused', status: 409 })).toBe(true);
	});

	it('does not require the optional conflicts array', () => {
		expect(isBranchMergeRefusal({ code: 'merge_plan_stale', message: 'main moved' })).toBe(true);
	});

	it('rejects an unrelated 409 from another endpoint', () => {
		expect(isBranchMergeRefusal({ code: 'CONFLICT', message: 'stale version', status: 409 })).toBe(
			false
		);
		expect(isBranchMergeRefusal({ code: 'CONFLICT_RETRY_FAILED', message: 'retry failed' })).toBe(
			false
		);
	});

	it('rejects a refusal-shaped value with no message', () => {
		expect(isBranchMergeRefusal({ code: 'merge_conflicts' })).toBe(false);
	});

	it('rejects null and non-objects', () => {
		expect(isBranchMergeRefusal(null)).toBe(false);
		expect(isBranchMergeRefusal(undefined)).toBe(false);
		expect(isBranchMergeRefusal('merge_conflicts')).toBe(false);
		expect(isBranchMergeRefusal(409)).toBe(false);
	});
});

/**
 * The allowlist in client.ts is hand-maintained; `internal/api/openapi.yaml` is
 * the source of truth. #676 will add `branchScope` to more operations, and
 * without this test that addition is invisible here — the UI would keep reading
 * the mainline for an operation its author believes is scoped.
 *
 * The spec is parsed by line rather than with a YAML library because `web`
 * declares no YAML parser among its dependencies. The parse is deliberately
 * brittle: anything it does not recognise throws instead of quietly matching
 * nothing.
 */
describe('BRANCH_SCOPED_OPERATIONS vs openapi.yaml', () => {
	const SPEC_PATH = 'internal/api/openapi.yaml';
	const BRANCH_SCOPE_REF = "- $ref: '#/components/parameters/branchScope'";
	const TEMPLATE_ID = '11111111-1111-1111-1111-111111111111';

	interface SpecOperation {
		method: string;
		/** The templated path as written in the spec, e.g. `/persons/{id}`. */
		path: string;
		/** True when the operation declares the `branchScope` parameter. */
		scoped: boolean;
	}

	function parseOperations(spec: string): SpecOperation[] {
		const lines = spec.split('\n');
		const start = lines.indexOf('paths:');
		if (start === -1) {
			throw new Error(`${SPEC_PATH} has no top-level \`paths:\` key`);
		}

		const operations: SpecOperation[] = [];
		let path: string | null = null;
		let current: SpecOperation | null = null;

		for (let i = start + 1; i < lines.length; i++) {
			const line = lines[i];
			// A non-indented, non-blank line ends the paths block.
			if (/^\S/.test(line)) break;

			const pathMatch = /^ {2}(\/\S*):\s*$/.exec(line);
			if (pathMatch) {
				path = pathMatch[1];
				current = null;
				continue;
			}

			const methodMatch = /^ {4}(get|put|post|patch|delete|head|options):\s*$/.exec(line);
			if (methodMatch) {
				if (path === null) {
					throw new Error(`openapi.yaml:${i + 1}: operation outside any path`);
				}
				current = { method: methodMatch[1].toUpperCase(), path, scoped: false };
				operations.push(current);
				continue;
			}

			if (!line.includes('branchScope')) continue;

			// Operation-level parameter: eight spaces of indent, inside an
			// operation's own `parameters:` list. Path-level (six spaces) would
			// apply to every method on the path and is not modelled here.
			if (line === `        ${BRANCH_SCOPE_REF}`) {
				if (current === null) {
					throw new Error(`openapi.yaml:${i + 1}: branchScope outside any operation`);
				}
				current.scoped = true;
				continue;
			}

			throw new Error(
				`openapi.yaml:${i + 1}: unrecognised branchScope reference "${line.trim()}". ` +
					'If it is now declared at path level, teach this test to fan it out across ' +
					"that path's operations before trusting it again."
			);
		}

		return operations;
	}

	const operations = parseOperations(openapiSpec);
	const concrete = (path: string) => path.replace(/\{[^}]+\}/g, TEMPLATE_ID);

	it('parsed a plausible spec', () => {
		// Guards against a parser that silently matches nothing and passes.
		expect(operations.length).toBeGreaterThan(50);
		expect(operations.filter((op) => op.scoped).length).toBeGreaterThan(0);
	});

	it('accepts exactly the operations that declare branchScope', () => {
		const drift = operations
			.filter((op) => isBranchScopedRequest(op.method, concrete(op.path)) !== op.scoped)
			.map((op) =>
				op.scoped
					? `  MISSING: ${op.method} ${op.path} declares branchScope but the table rejects it`
					: `  EXTRA:   ${op.method} ${op.path} has no branchScope but the table accepts it`
			);

		expect(
			drift,
			'BRANCH_SCOPED_OPERATIONS in client.ts has drifted from internal/api/openapi.yaml.\n' +
				'Update the table (and the MainlineNotice coverage that depends on it):\n' +
				drift.join('\n')
		).toEqual([]);
	});
});

/**
 * `NOTE_MAX_LENGTH` is hand-copied from the spec, so it can drift silently: the
 * dialog would keep letting a note through that the server then refuses with
 * `400 validation_error`. Pinned here rather than in the component's own tests
 * because this is where the raw-spec import already lives (see above).
 *
 * Parsed by line for the same reason the branch-scope test is - `web` declares
 * no YAML parser - and just as brittle: a spec it cannot read throws rather than
 * quietly passing.
 */
describe('NOTE_MAX_LENGTH vs openapi.yaml', () => {
	function specNoteMaxLength(spec: string): number {
		const lines = spec.split('\n');
		const schema = lines.indexOf('    BranchMergeRequest:');
		if (schema === -1) {
			throw new Error('openapi.yaml has no `BranchMergeRequest` schema at the expected indent');
		}

		for (let i = schema + 1; i < lines.length; i++) {
			// A sibling schema at the same indent ends this one.
			if (/^ {4}\S/.test(lines[i])) break;
			if (lines[i] !== '        note:') continue;

			for (let j = i + 1; j < lines.length; j++) {
				// A sibling property at the same indent ends the `note` block.
				if (/^ {8}\S/.test(lines[j])) break;
				const match = /^ {10}maxLength: (\d+)$/.exec(lines[j]);
				if (match) return Number(match[1]);
			}
			throw new Error('openapi.yaml: `BranchMergeRequest.note` declares no maxLength');
		}

		throw new Error('openapi.yaml: `BranchMergeRequest` declares no `note` property');
	}

	it('matches the spec cap the server enforces', () => {
		expect(NOTE_MAX_LENGTH).toBe(specNoteMaxLength(openapiSpec));
	});
});
