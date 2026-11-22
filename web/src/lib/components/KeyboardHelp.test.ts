import { describe, it, expect, afterEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import KeyboardHelp from './KeyboardHelp.svelte';
import { auth } from '#lib/stores/access.svelte.js';

describe('KeyboardHelp', () => {
	// The list scrolls inside the dialog; without a Tab stop of its own a
	// keyboard user cannot reach the lower groups (#1104).
	it('makes the scrolling shortcut list a named Tab stop', () => {
		render(KeyboardHelp, { open: true, onClose: () => {} });

		const list = screen.getByRole('region', { name: 'Shortcut list' });
		expect(list.getAttribute('tabindex')).toBe('0');
		expect(list.textContent).toContain('Family Detail');
	});

	// A visitor is refused the change log, so the snapshots shortcut is not
	// listed for them (#1530).
	describe('the research snapshots shortcut', () => {
		afterEach(() => {
			auth.phase = 'loading';
		});

		it('is listed for a signed-in user', () => {
			auth.phase = 'signed-in';
			render(KeyboardHelp, { open: true, onClose: () => {} });
			expect(screen.getByText('Go to research snapshots')).toBeTruthy();
		});

		it('is not listed for a visitor', () => {
			auth.phase = 'visitor';
			render(KeyboardHelp, { open: true, onClose: () => {} });
			expect(screen.queryByText('Go to research snapshots')).toBeNull();
			expect(screen.getByText('Go to people list')).toBeTruthy();
		});
	});
});
