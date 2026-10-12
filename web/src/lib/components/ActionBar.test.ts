import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import ActionBar from './ActionBar.svelte';

const snippet = (html: string) => createRawSnippet(() => ({ render: () => html }));

describe('ActionBar', () => {
	it('renders the back link', () => {
		render(ActionBar, { props: { back: { href: '/sources', label: 'Sources' } } });
		expect(screen.getByRole('link', { name: '← Sources' }).getAttribute('href')).toBe('/sources');
	});

	it('renders actions in their own wrapping group', () => {
		const { container } = render(ActionBar, {
			props: {
				back: { href: '/persons', label: 'People' },
				actions: snippet('<button>Edit</button>')
			}
		});
		const actions = container.querySelector('.action-bar-actions');
		expect(actions?.querySelector('button')?.textContent).toBe('Edit');
	});

	it('omits the actions group when there are no actions', () => {
		const { container } = render(ActionBar, { props: { back: { href: '/', label: 'Back' } } });
		expect(container.querySelector('.action-bar-actions')).toBeNull();
	});

	it('has no heading, since the page body carries it', () => {
		render(ActionBar, { props: { back: { href: '/', label: 'Back' } } });
		expect(screen.queryByRole('heading')).toBeNull();
	});
});
