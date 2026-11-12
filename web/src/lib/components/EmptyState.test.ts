import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import EmptyState from './EmptyState.svelte';

const actions = createRawSnippet(() => ({
	render: () => '<a href="/persons/add">Add Person</a>'
}));

describe('EmptyState', () => {
	it('shows only the message when given nothing else', () => {
		render(EmptyState, { props: { message: 'No people found.' } });
		const empty = screen.getByTestId('empty-state');
		expect(empty.textContent?.trim()).toBe('No people found.');
		expect(empty.querySelector('.description')).toBeNull();
		expect(empty.querySelector('.actions')).toBeNull();
	});

	it('shows the description and the actions', () => {
		render(EmptyState, {
			props: {
				message: 'No research logs yet.',
				description: 'Log your research activities.',
				actions
			}
		});
		expect(screen.getByText('Log your research activities.')).toBeTruthy();
		expect(screen.getByRole('link', { name: 'Add Person' }).getAttribute('href')).toBe(
			'/persons/add'
		);
	});
});
