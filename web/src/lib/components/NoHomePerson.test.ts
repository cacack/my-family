import { describe, it, expect, afterEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import NoHomePerson from './NoHomePerson.svelte';
import { auth } from '#lib/stores/access.svelte.js';

describe('NoHomePerson', () => {
	afterEach(() => {
		auth.phase = 'loading';
	});

	it('names the chart and says how to choose who you are', () => {
		auth.phase = 'open';
		render(NoHomePerson, { title: 'Pedigree Chart' });
		expect(screen.getByRole('heading', { name: 'Pedigree Chart' })).toBeTruthy();
		expect(screen.getByTestId('empty-state').textContent).toContain('This is me');
		expect(screen.getByRole('link', { name: 'Browse people' }).getAttribute('href')).toBe('/persons');
	});

	it('asks a visitor, who has no home person, to pick a person instead', () => {
		auth.phase = 'visitor';
		render(NoHomePerson, { title: 'Pedigree Chart' });
		const text = screen.getByTestId('empty-state').textContent;
		expect(text).not.toContain('This is me');
		expect(text).toContain("any person's page");
		expect(screen.getByRole('link', { name: 'Browse people' }).getAttribute('href')).toBe('/persons');
	});
});
