import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import ErrorState from './ErrorState.svelte';

describe('ErrorState', () => {
	it('announces the message as an alert', () => {
		render(ErrorState, { props: { message: 'Failed to load people.', onRetry: vi.fn() } });
		expect(screen.getByRole('alert').textContent).toContain('Failed to load people.');
	});

	it('calls onRetry from its Retry button', async () => {
		const onRetry = vi.fn();
		render(ErrorState, { props: { message: 'Failed', onRetry } });
		await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
		expect(onRetry).toHaveBeenCalledOnce();
	});
});
