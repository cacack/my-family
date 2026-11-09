import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
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
});
