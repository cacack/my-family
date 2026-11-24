import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '#lib/api/client.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getMapLocations: vi.fn(() => new Promise(() => {})) } };
});

describe('Family Map page', () => {
	it('links to the places without coordinates (#1561)', () => {
		render(Page);
		expect(screen.getByRole('link', { name: 'Places without coordinates' }).getAttribute('href')).toBe(
			'/places/unmapped'
		);
	});
});
