/**
 * The shared not-found and load-error states (#1096, docs/CONVENTIONS.md "Load
 * states"), on every detail page at once: each keeps exactly one h1, a missing
 * record offers no Retry, and a 500 or a network failure reads as a friendly
 * sentence with a Retry, never the raw server or browser text.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import type { Component } from 'svelte';
import type * as apiModule from '#lib/api/client.js';
import { resetRouter } from '#lib/test/fakeRouter.svelte.js';
import PersonPage from './persons/[id]/+page.svelte';
import FamilyPage from './families/[id]/+page.svelte';
import GroupSheetPage from './families/[id]/group-sheet/+page.svelte';
import SourcePage from './sources/[id]/+page.svelte';
import RepositoryPage from './repositories/[id]/+page.svelte';
import BranchPage from './branches/[id]/+page.svelte';
import BranchResearchPage from './branches/[id]/research/+page.svelte';
import AnalysisPage from './evidence/analyses/[id]/+page.svelte';
import ConflictPage from './evidence/conflicts/[id]/+page.svelte';
import ProofSummaryPage from './evidence/proof-summaries/[id]/+page.svelte';
import ResearchLogPage from './evidence/research-logs/[id]/+page.svelte';
import AhnentafelPage from './ahnentafel/[id]/+page.svelte';
import PedigreePage from './pedigree/[id]/+page.svelte';
import DescendancyPage from './descendancy/[id]/+page.svelte';
import MergePage from './quality/merge/[survivorId]/[mergedId]/+page.svelte';

/** What every API call rejects with; each test sets it before rendering. */
const failure = vi.hoisted(() => ({ error: undefined as unknown }));

// Every method fails the same way, so each page lands in its load state
// whichever calls it makes.
vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	const api = new Proxy({}, { get: () => () => Promise.reject(failure.error) });
	return { ...actual, api };
});
vi.mock('$app/state', async () => (await import('#lib/test/fakeRouter.svelte.js')).appState);
vi.mock('$app/navigation', async () => (await import('#lib/test/fakeRouter.svelte.js')).appNavigation);

const ID = '11111111-1111-1111-1111-111111111111';
const OTHER = '22222222-2222-2222-2222-222222222222';

interface Route {
	name: string;
	Page: Component;
	path: string;
	params: Record<string, string>;
	/** The h1 a not-found or failed load shows. */
	notFound: string;
	failed: string;
	/** The friendly sentence a 500 or network failure shows. */
	message: string;
}

/** A page whose heading lives in its loaded body, so the state brings it. */
function titled(name: string, Page: Component, path: string, noun: string): Route {
	const heading = noun.charAt(0).toUpperCase() + noun.slice(1);
	return {
		name,
		Page,
		path,
		params: { id: ID },
		notFound: `${heading} not found`,
		failed: `${heading} could not be loaded`,
		message: `Failed to load the ${noun}. Please try again.`
	};
}

/** A page whose own h1 stays above the content in every state. */
function headed(name: string, Page: Component, path: string, h1: string, message: string, params = { id: ID }): Route {
	return { name, Page, path, params, notFound: h1, failed: h1, message };
}

const ROUTES: Route[] = [
	titled('person', PersonPage, `/persons/${ID}`, 'person'),
	titled('family', FamilyPage, `/families/${ID}`, 'family'),
	titled('group sheet', GroupSheetPage, `/families/${ID}/group-sheet`, 'family'),
	titled('source', SourcePage, `/sources/${ID}`, 'source'),
	titled('repository', RepositoryPage, `/repositories/${ID}`, 'repository'),
	titled('branch', BranchPage, `/branches/${ID}`, 'branch'),
	titled('branch research', BranchResearchPage, `/branches/${ID}/research`, 'branch'),
	titled('analysis', AnalysisPage, `/evidence/analyses/${ID}`, 'analysis'),
	titled('conflict', ConflictPage, `/evidence/conflicts/${ID}`, 'conflict'),
	titled('proof summary', ProofSummaryPage, `/evidence/proof-summaries/${ID}`, 'proof summary'),
	titled('research log', ResearchLogPage, `/evidence/research-logs/${ID}`, 'research log'),
	titled('ahnentafel', AhnentafelPage, `/ahnentafel/${ID}`, 'person'),
	headed('pedigree', PedigreePage, `/pedigree/${ID}`, 'Pedigree Chart', 'Failed to load the person. Please try again.'),
	headed(
		'descendancy',
		DescendancyPage,
		`/descendancy/${ID}`,
		'Descendancy Chart',
		'Failed to load the person. Please try again.'
	),
	headed(
		'merge',
		MergePage,
		`/quality/merge/${ID}/${OTHER}`,
		'Merge Persons',
		'Failed to load one or both persons. Please try again.',
		{ survivorId: ID, mergedId: OTHER } as unknown as { id: string }
	)
];

const NOT_FOUND = { status: 404, code: 'NOT_FOUND', message: 'Resource not found' };

beforeEach(() => {
	vi.clearAllMocks();
});

function renderRoute(route: Route, error: unknown) {
	failure.error = error;
	resetRouter(route.path, route.params);
	render(route.Page);
}

describe.each(ROUTES)('$name load states (#1096)', (route) => {
	it('shows one h1 and no Retry when the record is not found', async () => {
		renderRoute(route, NOT_FOUND);
		await screen.findByRole('alert');
		const headings = screen.getAllByRole('heading', { level: 1 });
		expect(headings.map((h) => h.textContent?.trim())).toEqual([route.notFound]);
		expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
	});

	it.each([
		['a 500', { status: 500, code: 'INTERNAL_ERROR', message: 'boom' }, 'boom'],
		['a network failure', new TypeError('Failed to fetch'), 'Failed to fetch']
	])('shows one h1, a friendly sentence and Retry on %s', async (_kind, error, raw) => {
		renderRoute(route, error);
		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain(route.message);
		expect(alert.textContent).not.toContain(raw);
		const headings = screen.getAllByRole('heading', { level: 1 });
		expect(headings.map((h) => h.textContent?.trim())).toEqual([route.failed]);
		expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy();
	});
});

describe('controls with nothing loaded (#1096)', () => {
	it.each([
		{ name: 'pedigree', route: ROUTES.find((r) => r.name === 'pedigree')! },
		{ name: 'descendancy', route: ROUTES.find((r) => r.name === 'descendancy')! }
	])('disables the $name chart controls', async ({ route }) => {
		renderRoute(route, { status: 500, message: 'boom' });
		await screen.findByRole('alert');
		// Disabled through their fieldset, which the `disabled` property doesn't reflect.
		expect(screen.getByRole('combobox').matches(':disabled')).toBe(true);
		for (const name of ['Compact', 'Standard', 'Wide', '+', '-', 'Reset']) {
			expect(screen.getByRole('button', { name }).matches(':disabled'), name).toBe(true);
		}
	});

	it('disables the Ahnentafel generations, Print and Export', async () => {
		renderRoute(ROUTES.find((r) => r.name === 'ahnentafel')!, NOT_FOUND);
		await screen.findByRole('alert');
		expect((screen.getByRole('combobox') as HTMLSelectElement).disabled).toBe(true);
		expect((screen.getByRole('button', { name: 'Print' }) as HTMLButtonElement).disabled).toBe(true);
		expect((screen.getByRole('button', { name: 'Export Text' }) as HTMLButtonElement).disabled).toBe(true);
	});

	it('disables the group sheet Print', async () => {
		renderRoute(ROUTES.find((r) => r.name === 'group sheet')!, { status: 500, message: 'boom' });
		await screen.findByRole('alert');
		expect((screen.getByRole('button', { name: 'Print' }) as HTMLButtonElement).disabled).toBe(true);
	});
});
