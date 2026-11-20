import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PersonCard from './PersonCard.svelte';

describe('PersonCard', () => {
	it('shows the new person when its person prop changes in place', async () => {
		const { container, rerender } = render(PersonCard, {
			person: { id: 'a', given_name: 'John', surname: 'Smith', gender: 'male' },
			href: '/persons/a'
		});
		expect(screen.getByText('John Smith')).toBeDefined();
		expect(container.querySelector('.avatar')?.getAttribute('data-gender')).toBe('male');

		await rerender({
			person: { id: 'b', given_name: 'Mary', surname: 'Jones', gender: 'female' },
			href: '/persons/b'
		});
		expect(screen.getByText('Mary Jones')).toBeDefined();
		expect(screen.queryByText('John Smith')).toBeNull();
		expect(container.querySelector('.avatar')?.getAttribute('data-gender')).toBe('female');
	});

	it('marks a living or private person, in the link name too', () => {
		render(PersonCard, {
			person: { id: 'a', given_name: 'Edith', surname: 'Holt', living: true, private: true, version: 1 },
			href: '/persons/a'
		});
		expect(screen.getByRole('img', { name: 'Living person' })).toBeDefined();
		expect(screen.getByRole('img', { name: 'Private person' })).toBeDefined();
		expect(screen.getByRole('link').textContent).toContain('Edith Holt');
		expect(screen.getByRole('link', { name: /Edith Holt.*Living person/ })).toBeDefined();
	});

	it('shows a placeholder by name and mark only, with no dates or research status (#1466)', () => {
		const { container } = render(PersonCard, {
			person: {
				id: 'a',
				given_name: 'Living',
				surname: '',
				redacted: true,
				living: true,
				research_status: 'unknown',
				birth_date: { year: 1990 },
				version: 1
			},
			href: '/persons/a'
		});
		expect(screen.getByRole('link', { name: /Living.*Details hidden/ })).toBeDefined();
		expect(screen.queryByRole('img', { name: 'Living person' })).toBeNull();
		expect(container.querySelector('.detail')).toBeNull();
		expect(screen.queryByText('Not assessed')).toBeNull();
	});

	it('shows no privacy badge for a deceased public person', () => {
		render(PersonCard, {
			person: { id: 'a', given_name: 'Old', surname: 'Holt', living: false, version: 1 },
			href: '/persons/a'
		});
		expect(screen.queryByRole('img', { name: 'Living person' })).toBeNull();
	});
});
