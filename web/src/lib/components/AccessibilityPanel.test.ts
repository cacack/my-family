import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { flushSync } from 'svelte';
import AccessibilityPanel from './AccessibilityPanel.svelte';

// jsdom has no matchMedia; the settings store reads it at import time.
vi.hoisted(() => {
	window.matchMedia = ((query: string) => ({ matches: false, media: query })) as typeof window.matchMedia;
});

describe('AccessibilityPanel', () => {
	let opener: HTMLButtonElement;

	beforeEach(() => {
		opener = document.createElement('button');
		opener.textContent = 'Accessibility settings';
		document.body.appendChild(opener);
	});

	afterEach(() => {
		opener.remove();
	});

	it('names each toggle by its purpose, not its state', () => {
		render(AccessibilityPanel, { open: true, onClose: () => {} });

		for (const name of ['High contrast', 'Reduced motion', 'Keyboard shortcuts']) {
			expect(screen.getByRole('checkbox', { name })).toBeTruthy();
		}
	});

	it('returns focus to the opener when closed', async () => {
		opener.focus();
		const { rerender } = render(AccessibilityPanel, { open: true, onClose: () => {} });

		const close = screen.getByRole('button', { name: 'Close accessibility settings' });
		close.focus();
		expect(document.activeElement).toBe(close);

		await rerender({ open: false });
		flushSync();

		expect(document.activeElement).toBe(opener);
	});

	it('calls onClose on Escape', async () => {
		let closed = false;
		render(AccessibilityPanel, { open: true, onClose: () => (closed = true) });

		await fireEvent.keyDown(window, { key: 'Escape' });

		expect(closed).toBe(true);
	});
});
