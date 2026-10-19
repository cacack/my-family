import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import KeyboardHelp from './KeyboardHelp.svelte';

describe('KeyboardHelp', () => {
	// The list scrolls inside the dialog; without a Tab stop of its own a
	// keyboard user cannot reach the lower groups (#1104).
	it('makes the scrolling shortcut list a named Tab stop', () => {
		render(KeyboardHelp, { open: true, onClose: () => {} });

		const list = screen.getByRole('region', { name: 'Shortcut list' });
		expect(list.getAttribute('tabindex')).toBe('0');
		expect(list.textContent).toContain('Family Detail');
	});
});
