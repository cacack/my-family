import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import RoleNotice from './RoleNotice.svelte';
import { signInAs } from '#lib/test/roles.js';
import { auth } from '#lib/stores/access.svelte.js';

describe('RoleNotice', () => {
	it('names the role the action needs and the one the user has', () => {
		signInAs('viewer');
		render(RoleNotice, { action: 'Exporting the tree', required: 'editor' });
		expect(screen.getByText('Exporting the tree needs the editor role or above')).toBeDefined();
		expect(screen.getByRole('note').textContent).toMatch(/signed in as a viewer\.\s+Ask the\s+tree's owner/);
	});

	it('says the owner role for an owner-only action', () => {
		signInAs('editor');
		render(RoleNotice, { action: 'Importing GEDCOM', required: 'owner' });
		expect(screen.getByText('Importing GEDCOM needs the owner role')).toBeDefined();
		expect(screen.getByRole('note').textContent).toMatch(/signed in as an editor/);
	});

	it('tells a visitor to sign in rather than calling them signed in', () => {
		auth.phase = 'visitor';
		render(RoleNotice, { action: 'Adding people', required: 'editor' });
		const text = screen.getByRole('note').textContent ?? '';
		expect(text).toContain('browsing as a visitor. Sign in');
		expect(text).not.toContain('signed in as');
	});
});
