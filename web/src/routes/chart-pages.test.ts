/**
 * What the chart pages say around the chart: the Back link (#1026), the tab
 * title (#1119) and the keyboard hints (#1123). The behaviour is shared, so
 * one table covers the pages that have it.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import PedigreePage from './pedigree/[id]/+page.svelte';
import DescendancyPage from './descendancy/[id]/+page.svelte';
import AhnentafelPage from './ahnentafel/[id]/+page.svelte';
import * as apiModule from '#lib/api/client.js';
import { DEFAULT_SHORTCUTS, keyLabel, type ShortcutContext } from '#lib/keyboard/shortcuts.js';
import { appNavigation, back, resetRouter } from '#lib/test/fakeRouter.svelte.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: { getPedigree: vi.fn(), getDescendancy: vi.fn(), getAhnentafel: vi.fn() }
	};
});

vi.mock('$app/state', async () => (await import('#lib/test/fakeRouter.svelte.js')).appState);
vi.mock('$app/navigation', async () => (await import('#lib/test/fakeRouter.svelte.js')).appNavigation);

const ROOT = { id: 'p-1', given_name: 'Victoria', surname: 'Hanover', generation: 0 };
const CHILD = { id: 'p-2', given_name: 'Edward', surname: 'Wettin', generation: 1 };

beforeEach(() => {
	vi.clearAllMocks();
	vi.mocked(apiModule.api.getPedigree).mockResolvedValue({
		root: ROOT,
		total_ancestors: 0,
		max_generation: 0
	} as unknown as apiModule.Pedigree);
	vi.mocked(apiModule.api.getDescendancy).mockResolvedValue({
		root: { ...ROOT, children: [CHILD] },
		total_descendants: 1,
		max_generation: 1
	} as unknown as apiModule.Descendancy);
	vi.mocked(apiModule.api.getAhnentafel).mockResolvedValue({
		subject: { id: 'p-1', given_name: 'Victoria', surname: 'Hanover' },
		entries: [{ number: 1, generation: 0, id: 'p-1', given_name: 'Victoria', surname: 'Hanover' }],
		generations: 0,
		total_count: 1,
		known_count: 1,
		ancestor_slots: 0,
		known_ancestors: 0
	} as unknown as apiModule.AhnentafelResponse);
});

function backLink(): HTMLAnchorElement {
	return screen.getByRole('link', { name: /Back/ }) as HTMLAnchorElement;
}

describe.each([
	{ name: 'pedigree', Page: PedigreePage },
	{ name: 'descendancy', Page: DescendancyPage },
	{ name: 'ahnentafel', Page: AhnentafelPage }
])('$name Back link (#1026)', ({ name, Page }) => {
	it('returns to the page the user came from', async () => {
		resetRouter(`/${name}/p-1`, { id: 'p-1' }, { from: '/persons/p-1?tab=events' });
		render(Page);
		// Once loaded, the fallback would name the person: the origin wins.
		await waitFor(() => expect(document.title).toContain('Victoria Hanover'));
		expect(backLink().getAttribute('href')).toBe('/persons/p-1?tab=events');
		expect(backLink().textContent?.trim()).toBe('← Back');
	});

	it('keeps that origin when the chart moves on to another person', async () => {
		resetRouter(`/${name}/p-1`, { id: 'p-1' }, { from: '/families/f-9' });
		render(Page);
		await appNavigation.goto(`/${name}/p-2`);
		await appNavigation.goto(`/${name}/p-2?gens=6`, { replace: true });
		back();
		await waitFor(() => expect(backLink().getAttribute('href')).toBe('/families/f-9'));
	});

	it('falls back to the chart person on a deep link', async () => {
		resetRouter(`/${name}/p-1`, { id: 'p-1' });
		render(Page);
		await waitFor(() => expect(backLink().textContent).toContain('Back to Victoria Hanover'));
		expect(backLink().getAttribute('href')).toBe('/persons/p-1');
	});
});

describe.each([
	{ name: 'pedigree', Page: PedigreePage, title: 'Pedigree: Victoria Hanover | My Family' },
	{ name: 'descendancy', Page: DescendancyPage, title: 'Descendancy: Victoria Hanover | My Family' }
])('$name tab title (#1119)', ({ name, Page, title }) => {
	it('names the person', async () => {
		resetRouter(`/${name}/p-1`, { id: 'p-1' });
		render(Page);
		await waitFor(() => expect(document.title).toBe(title));
	});
});

describe.each([
	{ name: 'pedigree', Page: PedigreePage, chart: /Pedigree chart/ },
	{ name: 'descendancy', Page: DescendancyPage, chart: /Descendancy chart/ }
])('$name keyboard hints (#1123)', ({ name, Page, chart }) => {
	const keys = DEFAULT_SHORTCUTS.filter((s) => s.context === name).flatMap((s) => s.keys);

	it('the aria-label and the hint name every key in the ? dialog group', async () => {
		resetRouter(`/${name}/p-1`, { id: 'p-1' });
		const { container } = render(Page);
		const label = (await screen.findByRole('application', { name: chart })).getAttribute('aria-label') ?? '';
		const hint = container.querySelector('.hint')?.textContent ?? '';
		expect(keys.length).toBeGreaterThan(0);
		for (const key of keys) {
			expect(label).toContain(`${keyLabel(key)}`);
			expect(hint).toContain(`${keyLabel(key)}`);
		}
	});
});

describe('chart shortcut groups (#1123)', () => {
	const group = (context: ShortcutContext) => DEFAULT_SHORTCUTS.filter((s) => s.context === context);

	it('the pedigree offers no key without a working handler (ArrowRight had none)', () => {
		expect(group('pedigree').map((s) => s.action)).not.toContain('navigate-spouse');
		expect(group('pedigree').flatMap((s) => s.keys)).not.toContain('ArrowRight');
	});

	it('the descendancy has a key back to the root', () => {
		expect(group('descendancy').map((s) => s.action)).toContain('navigate-root');
	});

	it('Home returns the descendancy selection to the root', async () => {
		resetRouter('/descendancy/p-1', { id: 'p-1' });
		render(DescendancyPage);
		await screen.findByRole('application', { name: /Descendancy chart/ });
		const status = screen.getByRole('status');

		await fireEvent.keyDown(window, { key: 'ArrowDown' });
		await waitFor(() => expect(status.textContent).toContain('Navigated to child: Edward Wettin'));

		await fireEvent.keyDown(window, { key: 'Home' });
		await waitFor(() => expect(status.textContent).toContain('Navigated to root: Victoria Hanover'));
	});
});
