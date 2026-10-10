import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/svelte';
import MainNav from './MainNav.svelte';
import { mainNav, isGroup } from '$lib/navigation';

// jsdom applies no media queries, so both the wide row and the narrow menu
// trigger are in the DOM; queries scope to one or the other.
function wideRow(container: HTMLElement): HTMLElement {
	return container.querySelector('.nav-wide') as HTMLElement;
}

async function open(trigger: HTMLElement) {
	// bits-ui opens on pointerdown, not click.
	await fireEvent.pointerDown(trigger, { button: 0, pointerType: 'mouse' });
	return screen.findByRole('menu');
}

describe('MainNav', () => {
	it('shows the top-level links and group menus on one row', () => {
		const { container } = render(MainNav, { pathname: '/' });
		const row = within(wideRow(container));
		expect(row.getByRole('link', { name: 'People' }).getAttribute('href')).toBe('/persons');
		expect(row.getByRole('link', { name: 'Families' })).toBeDefined();
		expect(row.getByRole('link', { name: 'Import' })).toBeDefined();
		for (const group of ['Browse', 'Research', 'History']) {
			expect(row.getByRole('button', { name: group })).toBeDefined();
		}
	});

	it('marks the current top-level link', () => {
		const { container } = render(MainNav, { pathname: '/pedigree/abc' });
		const people = within(wideRow(container)).getByRole('link', { name: 'People' });
		expect(people.getAttribute('aria-current')).toBe('page');
	});

	it('highlights a group when the page is nested under one of its links', () => {
		const { container } = render(MainNav, { pathname: '/evidence/conflicts/abc' });
		const row = within(wideRow(container));
		expect(row.getByRole('button', { name: 'Research' }).className).toContain('bg-[#eff6ff]');
		expect(row.getByRole('button', { name: 'Browse' }).className).not.toContain('bg-[#eff6ff]');
	});

	it('opens a group menu of links, marking the current one', async () => {
		const { container } = render(MainNav, { pathname: '/branches/abc' });
		const menu = await open(within(wideRow(container)).getByRole('button', { name: 'History' }));
		const items = within(menu).getAllByRole('menuitem');
		expect(items.map((i) => i.textContent?.trim())).toEqual(['Change History', 'Branches', 'Snapshots']);
		expect(within(menu).getByRole('menuitem', { name: 'Branches' }).getAttribute('href')).toBe('/branches');
		expect(within(menu).getByRole('menuitem', { name: 'Branches' }).getAttribute('aria-current')).toBe('page');
		expect(within(menu).getByRole('menuitem', { name: 'Snapshots' }).getAttribute('aria-current')).toBeNull();
	});

	it('offers every destination from the narrow-screen menu, grouped', async () => {
		render(MainNav, { pathname: '/sources/abc' });
		const menu = await open(screen.getByRole('button', { name: 'Menu' }));
		for (const heading of ['Browse', 'Research', 'History']) {
			expect(within(menu).getByText(heading)).toBeDefined();
		}
		const labels = within(menu).getAllByRole('menuitem').map((i) => i.textContent?.trim());
		const destinations = mainNav.flatMap((e) => (isGroup(e) ? e.links : [e])).map((l) => l.label);
		expect(labels).toEqual(destinations);
		expect(within(menu).getByRole('menuitem', { name: 'Sources' }).getAttribute('aria-current')).toBe('page');
	});
});
