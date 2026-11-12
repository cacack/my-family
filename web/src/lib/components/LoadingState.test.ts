import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import LoadingState from './LoadingState.svelte';

describe('LoadingState', () => {
	it('announces its label politely as a status', () => {
		render(LoadingState, { props: { label: 'Loading people...' } });
		const status = screen.getByRole('status');
		expect(status.getAttribute('aria-live')).toBe('polite');
		expect(status.textContent).toBe('Loading people...');
	});

	it('defaults the label to "Loading..."', () => {
		render(LoadingState);
		expect(screen.getByRole('status').textContent).toBe('Loading...');
	});
});
