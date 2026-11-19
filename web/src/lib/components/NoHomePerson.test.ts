import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import NoHomePerson from './NoHomePerson.svelte';

describe('NoHomePerson', () => {
	it('names the chart and says how to choose who you are', () => {
		render(NoHomePerson, { title: 'Pedigree Chart' });
		expect(screen.getByRole('heading', { name: 'Pedigree Chart' })).toBeTruthy();
		expect(screen.getByTestId('empty-state').textContent).toContain('This is me');
		expect(screen.getByRole('link', { name: 'Browse people' }).getAttribute('href')).toBe('/persons');
	});
});
