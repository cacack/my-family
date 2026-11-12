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

	it('offers no Retry when the record is not found (#1096)', () => {
		render(ErrorState, { props: { message: 'Gone', notFound: true, onRetry: vi.fn() } });
		expect(screen.getByRole('alert').textContent).toContain('Gone');
		expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
	});

	it('shows its title as the page h1, and none without one (#1096)', () => {
		const { unmount } = render(ErrorState, {
			props: { title: 'Person not found', message: 'Gone', notFound: true, onRetry: vi.fn() }
		});
		expect(screen.getByRole('heading', { level: 1, name: 'Person not found' })).toBeTruthy();
		unmount();

		render(ErrorState, { props: { message: 'Failed', onRetry: vi.fn() } });
		expect(screen.queryByRole('heading')).toBeNull();
	});
});
