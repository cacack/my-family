import { describe, it, expect } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import PreviewBanner from './PreviewBanner.svelte';
import { auth } from '#lib/stores/access.svelte.js';
import { signInAs } from '#lib/test/roles.js';

describe('PreviewBanner', () => {
	it('shows nothing when no one is previewing', () => {
		render(PreviewBanner);
		expect(screen.queryByRole('status')).toBeNull();
	});

	it.each([
		['viewer', 'as a signed-in viewer does'],
		['visitor', 'as a visitor who is not signed in does']
	] as const)('says the owner previews as a %s, and stops the preview', async (as, text) => {
		signInAs('owner');
		auth.previewAs = as;
		render(PreviewBanner);
		expect(screen.getByRole('status').textContent).toContain(text);

		await fireEvent.click(screen.getByRole('button', { name: 'Stop preview' }));
		expect(auth.previewAs).toBeNull();
		expect(screen.queryByRole('status')).toBeNull();
	});

	it('shows nothing to an editor, whose preview the server ignores', () => {
		signInAs('editor');
		auth.previewAs = 'viewer';
		render(PreviewBanner);
		expect(screen.queryByRole('status')).toBeNull();
	});
});
