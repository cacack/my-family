import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import OffscreenIndicator from './OffscreenIndicator.svelte';

describe('OffscreenIndicator', () => {
	it('names the count on each side, singular and plural', () => {
		const { getByRole } = render(OffscreenIndicator, {
			props: { left: 1, right: 3, noun: ['parent', 'parents'], onPan: () => {} }
		});
		expect(getByRole('button', { name: 'Show 1 parent off-screen to the left' }).textContent).toBe('‹ 1 parent');
		expect(getByRole('button', { name: 'Show 3 parents off-screen to the right' }).textContent).toBe('3 parents ›');
	});

	it('shows nothing for a side with nothing off-screen', () => {
		const { queryAllByRole } = render(OffscreenIndicator, {
			props: { left: 0, right: 0, noun: ['child', 'children'], onPan: () => {} }
		});
		expect(queryAllByRole('button')).toHaveLength(0);
	});

	it('pans toward the side clicked', async () => {
		const onPan = vi.fn();
		const { getByRole } = render(OffscreenIndicator, {
			props: { left: 2, right: 0, noun: ['child', 'children'], onPan }
		});
		await fireEvent.click(getByRole('button'));
		expect(onPan).toHaveBeenCalledWith('left');
	});

	describe('keeps focus when a side runs out (WCAG 2.4.3)', () => {
		it('moves focus to the other side', async () => {
			const chart = document.createElement('div');
			chart.tabIndex = 0;
			document.body.appendChild(chart);
			const { getByRole, rerender } = render(OffscreenIndicator, {
				props: { left: 1, right: 1, noun: ['parent', 'parents'], onPan: () => {}, focusFallback: chart }
			});
			const right = getByRole('button', { name: /right/ });
			right.focus();
			await fireEvent.click(right);
			await rerender({ left: 1, right: 0 });
			await tick();
			expect(document.activeElement).toBe(getByRole('button', { name: /left/ }));
			chart.remove();
		});

		it('falls back to the given element when neither side is left', async () => {
			const chart = document.createElement('div');
			chart.tabIndex = 0;
			document.body.appendChild(chart);
			const { getByRole, rerender } = render(OffscreenIndicator, {
				props: { left: 1, right: 0, noun: ['child', 'children'], onPan: () => {}, focusFallback: chart }
			});
			const left = getByRole('button');
			left.focus();
			await fireEvent.click(left);
			await rerender({ left: 0, right: 0 });
			await tick();
			expect(document.activeElement).toBe(chart);
			chart.remove();
		});

		it('leaves focus alone when the user has moved on', async () => {
			const elsewhere = document.createElement('input');
			document.body.appendChild(elsewhere);
			const { getByRole, rerender } = render(OffscreenIndicator, {
				props: { left: 1, right: 1, noun: ['parent', 'parents'], onPan: () => {} }
			});
			await fireEvent.click(getByRole('button', { name: /right/ }));
			elsewhere.focus();
			await rerender({ left: 1, right: 0 });
			await tick();
			expect(document.activeElement).toBe(elsewhere);
			elsewhere.remove();
		});
	});
});
