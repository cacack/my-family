import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
// Vite inlines the spec at transform time, so the drift test needs neither
// `node:fs` (the `web` package declares no Node type definitions) nor any
// assumption about the working directory.
import openapiSpec from '../../../../internal/api/openapi.yaml?raw';
import {
	api,
	formatGenDate,
	isBranchScopedRequest,
	setClientBranch,
	getClientBranch,
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
		['GET', `/pedigree/${PERSON_ID}`]
	])('allows %s %s', (method, path) => {
		expect(isBranchScopedRequest(method, path)).toBe(true);
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
