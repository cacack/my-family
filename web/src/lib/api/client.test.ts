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
	type BranchMergeResumeResult,
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
		['GET', '/browse/cemeteries'],
		['GET', '/browse/cemeteries/Oak%20Hill%20Cemetery/persons'],
		['GET', '/map/locations'],
		['GET', '/associations'],
		['POST', '/associations'],
		['GET', `/associations/${PERSON_ID}`],
		['PUT', `/associations/${PERSON_ID}`],
		['DELETE', `/associations/${PERSON_ID}`],
		['GET', `/persons/${PERSON_ID}/associations`],
		['GET', '/sources'],
		['POST', '/sources'],
		['GET', '/sources/search'],
		['GET', `/sources/${PERSON_ID}`],
		['PUT', `/sources/${PERSON_ID}`],
		['DELETE', `/sources/${PERSON_ID}`],
		['GET', `/sources/${PERSON_ID}/citations`],
		['POST', '/citations'],
		['GET', `/citations/${PERSON_ID}`],
		['PUT', `/citations/${PERSON_ID}`],
		['DELETE', `/citations/${PERSON_ID}`],
		['GET', `/citations/${PERSON_ID}/format`],
		['GET', `/persons/${PERSON_ID}/citations`],
		['GET', '/notes'],
		['POST', '/notes'],
		['GET', `/notes/${PERSON_ID}`],
		['PUT', `/notes/${PERSON_ID}`],
		['DELETE', `/notes/${PERSON_ID}`],
		['GET', `/persons/${PERSON_ID}/media`],
		['POST', `/persons/${PERSON_ID}/media`],
		['GET', `/media/${NAME_ID}`],
		['PUT', `/media/${NAME_ID}`],
		['DELETE', `/media/${NAME_ID}`],
		['GET', `/media/${NAME_ID}/content`],
		['GET', `/media/${NAME_ID}/thumbnail`],
		['GET', '/evidence-analyses'],
		['POST', '/evidence-analyses'],
		['GET', '/evidence-analyses/by-fact?factType=person_birth&subjectId=' + PERSON_ID],
		['GET', `/evidence-analyses/${NAME_ID}`],
		['PUT', `/evidence-analyses/${NAME_ID}`],
		['DELETE', `/evidence-analyses/${NAME_ID}?version=1`],
		['GET', '/evidence-conflicts?status=open'],
		['GET', `/evidence-conflicts/${NAME_ID}`],
		['POST', `/evidence-conflicts/${NAME_ID}/resolve`],
		['GET', `/evidence-conflicts/by-subject/${PERSON_ID}`],
		['GET', '/research-logs'],
		['POST', '/research-logs'],
		['GET', `/research-logs/${NAME_ID}`],
		['PUT', `/research-logs/${NAME_ID}`],
		['DELETE', `/research-logs/${NAME_ID}`],
		['GET', `/research-logs/by-subject/${PERSON_ID}`],
		['GET', '/proof-summaries'],
		['POST', '/proof-summaries'],
		['GET', '/proof-summaries/by-fact?factType=person_birth&subjectId=' + PERSON_ID],
		['GET', `/proof-summaries/${NAME_ID}`],
		['PUT', `/proof-summaries/${NAME_ID}`],
		['DELETE', `/proof-summaries/${NAME_ID}`]
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
		// Brick walls are not event-sourced (#761). Sending `?branch=` on these
		// would imply a scoping the server does not apply.
		expect(isBranchScopedRequest('GET', '/browse/brick-walls')).toBe(false);
		expect(isBranchScopedRequest('PUT', `/persons/${PERSON_ID}/brick-wall`)).toBe(false);
		expect(isBranchScopedRequest('DELETE', `/persons/${PERSON_ID}/brick-wall`)).toBe(false);
	});

	it('scopes search, the families list and the kinship reads (#829)', () => {
		expect(isBranchScopedRequest('GET', '/search?q=Ada&limit=10')).toBe(true);
		expect(isBranchScopedRequest('GET', '/families')).toBe(true);
		expect(isBranchScopedRequest('GET', '/families?limit=20&offset=40')).toBe(true);
		expect(isBranchScopedRequest('GET', `/families/${FAMILY_ID}/group-sheet`)).toBe(true);
		expect(isBranchScopedRequest('GET', `/ahnentafel/${PERSON_ID}?generations=5`)).toBe(true);
		expect(isBranchScopedRequest('GET', `/descendancy/${PERSON_ID}`)).toBe(true);
		expect(isBranchScopedRequest('GET', `/relationship/${PERSON_ID}/${NAME_ID}`)).toBe(true);
		// Only the two-person form exists.
		expect(isBranchScopedRequest('GET', `/relationship/${PERSON_ID}`)).toBe(false);
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
		expect(isBranchScopedRequest('GET', '/repositories')).toBe(false);
		// Source history stays mainline (#758 scopes the entity, not its audit trail).
		expect(isBranchScopedRequest('GET', `/sources/${PERSON_ID}/history`)).toBe(false);
		// GET /citations has no list operation, and /sources/search takes no writes.
		expect(isBranchScopedRequest('GET', '/citations')).toBe(false);
		expect(isBranchScopedRequest('POST', '/sources/search')).toBe(false);
		// Media history and rollback stay mainline (#759 scopes the metadata, not
		// its audit trail), and the content/thumbnail reads take no writes.
		expect(isBranchScopedRequest('GET', `/media/${NAME_ID}/history`)).toBe(false);
		expect(isBranchScopedRequest('POST', `/media/${NAME_ID}/rollback`)).toBe(false);
		expect(isBranchScopedRequest('PUT', `/media/${NAME_ID}/content`)).toBe(false);
	});

	it('scopes person and family history to the branch (#824)', () => {
		expect(isBranchScopedRequest('GET', `/persons/${PERSON_ID}/history`)).toBe(true);
		expect(isBranchScopedRequest('GET', `/families/${FAMILY_ID}/history?limit=1&offset=0`)).toBe(
			true
		);
	});

	it('forwards the scope to restore points and rollback so the server refuses them (#824)', () => {
		for (const kind of ['persons', 'families', 'sources', 'citations']) {
			expect(isBranchScopedRequest('GET', `/${kind}/${PERSON_ID}/restore-points`)).toBe(true);
			expect(isBranchScopedRequest('POST', `/${kind}/${PERSON_ID}/rollback`)).toBe(true);
			expect(isBranchScopedRequest('GET', `/${kind}/${PERSON_ID}/rollback`)).toBe(false);
		}
		expect(isBranchScopedRequest('POST', `/notes/${PERSON_ID}/rollback`)).toBe(false);
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
		await api.listRepositories();
		expect(requestedUrl()).toBe('/api/v1/repositories');
	});

	it('scopes search and the families list, which every search surface and list page share (#829)', async () => {
		setClientBranch(BRANCH_ID);
		await api.searchPersons({ q: 'Ada', limit: 10 });
		await api.listFamilies({ limit: 20 });
		expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
			`/api/v1/search?q=Ada&limit=10&branch=${BRANCH_ID}`,
			`/api/v1/families?limit=20&branch=${BRANCH_ID}`
		]);
	});

	it('scopes the text Ahnentafel, which bypasses request() (#829)', async () => {
		fetchMock.mockResolvedValueOnce(new Response('AHNENTAFEL REPORT', { status: 200 }));
		setClientBranch(BRANCH_ID);
		await expect(api.getAhnentafelText(PERSON_ID, 3)).resolves.toBe('AHNENTAFEL REPORT');
		expect(requestedUrl()).toBe(
			`/api/v1/ahnentafel/${PERSON_ID}?format=text&generations=3&branch=${BRANCH_ID}`
		);
	});

	it('scopes the media URL builders and the multipart upload, which bypass request()', async () => {
		expect(api.getMediaThumbnailUrl(NAME_ID)).toBe(`/api/v1/media/${NAME_ID}/thumbnail`);
		setClientBranch(BRANCH_ID);
		expect(api.getMediaContentUrl(NAME_ID)).toBe(`/api/v1/media/${NAME_ID}/content?branch=${BRANCH_ID}`);
		expect(api.getMediaThumbnailUrl(NAME_ID)).toBe(
			`/api/v1/media/${NAME_ID}/thumbnail?branch=${BRANCH_ID}`
		);
		await api.uploadPersonMedia(PERSON_ID, new File(['x'], 'x.jpg'), 'x');
		expect(requestedUrl()).toBe(`/api/v1/persons/${PERSON_ID}/media?branch=${BRANCH_ID}`);
	});

	it('never scopes the branch lifecycle endpoints themselves', async () => {
		setClientBranch(BRANCH_ID);
		await api.listBranches();
		expect(requestedUrl()).toBe('/api/v1/branches');
	});

	it('scopes the snapshot endpoints - a snapshot marks a position in the branch view', async () => {
		setClientBranch(BRANCH_ID);
		await api.listSnapshots();
		await api.createSnapshot({ name: 'On the branch' });
		await api.deleteSnapshot(NAME_ID);
		await api.compareSnapshots(NAME_ID, PERSON_ID);
		await api.compareSnapshotToCurrent(NAME_ID);
		await api.compareSnapshotToCurrent(NAME_ID, 7);
		// Ids are path-encoded, so a malformed one cannot reshape the route; it is
		// not a snapshot id either, so it is not scoped.
		await api.compareSnapshots('a/b', 'c d');
		expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
			`/api/v1/snapshots?branch=${BRANCH_ID}`,
			`/api/v1/snapshots?branch=${BRANCH_ID}`,
			`/api/v1/snapshots/${NAME_ID}?branch=${BRANCH_ID}`,
			`/api/v1/snapshots/${NAME_ID}/compare/${PERSON_ID}?branch=${BRANCH_ID}`,
			`/api/v1/snapshots/${NAME_ID}/compare-current?branch=${BRANCH_ID}`,
			`/api/v1/snapshots/${NAME_ID}/compare-current?until=7&branch=${BRANCH_ID}`,
			'/api/v1/snapshots/a%2Fb/compare/c%20d'
		]);
	});

	it('leaves the snapshot endpoints unscoped on the mainline', async () => {
		await api.listSnapshots();
		await api.compareSnapshotToCurrent(NAME_ID);
		await api.compareSnapshotToCurrent(NAME_ID, 42);
		expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
			'/api/v1/snapshots',
			`/api/v1/snapshots/${NAME_ID}/compare-current`,
			`/api/v1/snapshots/${NAME_ID}/compare-current?until=42`
		]);
	});
});

describe('snapshot endpoints', () => {
	let fetchMock: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 204, json: async () => ({}) });
		vi.stubGlobal('fetch', fetchMock);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('creates with a JSON body', async () => {
		fetchMock.mockResolvedValue({ ok: true, status: 201, json: async () => ({ id: 'x' }) });
		await api.createSnapshot({ name: 'Pre-DNA results', description: 'Before the kit' });
		const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
		expect(url).toBe('/api/v1/snapshots');
		expect(init.method).toBe('POST');
		expect(JSON.parse(init.body as string)).toEqual({
			name: 'Pre-DNA results',
			description: 'Before the kit'
		});
	});

	it('deletes by encoded id', async () => {
		await api.deleteSnapshot('x/y');
		const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
		expect(url).toBe('/api/v1/snapshots/x%2Fy');
		expect(init.method).toBe('DELETE');
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

describe('resumeBranchMerge', () => {
	const resumed: BranchMergeResumeResult = {
		branch: {
			id: BRANCH_ID,
			name: 'census-1881',
			base_position: 12,
			status: 'merged',
			created_at: '2026-01-01T00:00:00Z'
		},
		merged_at_position: 128,
		replayed_event_count: 1,
		already_replayed_stream_ids: [],
		skipped_stream_ids: [],
		reprojected_stream_ids: []
	};

	let fetchMock: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			status: 200,
			statusText: '',
			json: async () => resumed
		});
		vi.stubGlobal('fetch', fetchMock);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('POSTs to /branches/{id}/merge/resume with the id URL-encoded', async () => {
		await expect(api.resumeBranchMerge('branch/with space')).resolves.toEqual(resumed);
		expect(fetchMock.mock.calls[0][0]).toBe(
			'/api/v1/branches/branch%2Fwith%20space/merge/resume'
		);
		expect(fetchMock.mock.calls[0][1].method).toBe('POST');
		expect(fetchMock.mock.calls[0][1].body).toBe('{}');
	});

	it('serializes the resolutions it is given', async () => {
		const resolutions = [
			{ stream_id: '55555555-5555-5555-5555-555555555555', resolution: 'main' as const }
		];
		await api.resumeBranchMerge(BRANCH_ID, { resolutions });
		expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ resolutions });
	});
});

describe('precheckBranchMerge', () => {
	let fetchMock: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			status: 200,
			statusText: '',
			json: async () => ({ blockers: [] })
		});
		vi.stubGlobal('fetch', fetchMock);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('POSTs the proposed resolutions to /branches/{id}/merge/precheck', async () => {
		const resolutions = [
			{ stream_id: '55555555-5555-5555-5555-555555555555', resolution: 'main' as const }
		];
		await expect(api.precheckBranchMerge('branch/x', { resolutions })).resolves.toEqual({
			blockers: []
		});
		expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/branches/branch%2Fx/merge/precheck');
		expect(fetchMock.mock.calls[0][1].method).toBe('POST');
		expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ resolutions });
	});

	it('sends an empty body when given nothing', async () => {
		await api.precheckBranchMerge(BRANCH_ID);
		expect(fetchMock.mock.calls[0][1].body).toBe('{}');
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
		'merge_empty',
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
