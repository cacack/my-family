import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import PageHeader from './PageHeader.svelte';

const snippet = (html: string) => createRawSnippet(() => ({ render: () => html }));

describe('PageHeader', () => {
	it('renders the title as the page h1', () => {
		render(PageHeader, { props: { title: 'Sources' } });
		expect(screen.getByRole('heading', { level: 1, name: 'Sources' })).toBeDefined();
	});

	it('renders the description only when given', () => {
		const { container, unmount } = render(PageHeader, { props: { title: 'Sources' } });
		expect(container.querySelector('.description')).toBeNull();
		unmount();

		render(PageHeader, { props: { title: 'Map', description: 'Where they lived.' } });
		expect(screen.getByText('Where they lived.')).toBeDefined();
	});

	it('renders a back link only when both href and label are given', () => {
		const { unmount } = render(PageHeader, { props: { title: 'Add', backHref: '/persons' } });
		expect(screen.queryByRole('link')).toBeNull();
		unmount();

		render(PageHeader, { props: { title: 'Add', backHref: '/persons', backLabel: 'People' } });
		expect(screen.getByRole('link', { name: '← People' }).getAttribute('href')).toBe('/persons');
	});

	it('renders actions in their own wrapping group, and extra content under the title', () => {
		const { container } = render(PageHeader, {
			props: {
				title: 'Branches',
				actions: snippet('<button>New branch</button>'),
				children: snippet('<p class="note">A branch is a live view.</p>')
			}
		});
		const actions = container.querySelector('.page-header-actions');
		expect(actions?.querySelector('button')?.textContent).toBe('New branch');
		expect(container.querySelector('.page-header-text .note')).not.toBeNull();
	});

	it('omits the actions group when there are no actions', () => {
		const { container } = render(PageHeader, { props: { title: 'Families' } });
		expect(container.querySelector('.page-header-actions')).toBeNull();
	});

	it('exposes the h1 so a page can move focus to it', () => {
		let heading: HTMLHeadingElement | null = null;
		render(PageHeader, {
			props: {
				title: 'Snapshots',
				get headingEl() {
					return heading;
				},
				set headingEl(el: HTMLHeadingElement | null) {
					heading = el;
				}
			}
		});
		expect(heading).toBe(screen.getByRole('heading', { level: 1 }));
	});
});
