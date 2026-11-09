import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import DiscoveryFeed from './DiscoveryFeed.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getDiscoveryFeed: vi.fn() } };
});

function suggestion(type: apiModule.DiscoverySuggestion['type'], title: string): apiModule.DiscoverySuggestion {
	return { type, title, description: '', action_url: '/persons', priority: 1 };
}

describe('DiscoveryFeed filter chips', () => {
	beforeEach(() => {
		vi.mocked(apiModule.api.getDiscoveryFeed).mockResolvedValue({
			items: [suggestion('missing_data', 'Add a birth date'), suggestion('orphan', 'Link a parent')],
			total: 2
		} as unknown as apiModule.DiscoveryFeedResponse);
	});

	// The active chip was shown only by a class (#1061).
	it('marks the active chip with aria-pressed', async () => {
		render(DiscoveryFeed);
		const missing = await screen.findByRole('button', { name: 'Missing Data', pressed: false });
		const orphans = screen.getByRole('button', { name: 'Orphans', pressed: false });

		await fireEvent.click(missing);
		expect(missing.getAttribute('aria-pressed')).toBe('true');
		expect(orphans.getAttribute('aria-pressed')).toBe('false');
		expect(missing.classList.contains('active')).toBe(false);
		expect(screen.queryByText('Link a parent')).toBeNull();

		await fireEvent.click(missing);
		expect(missing.getAttribute('aria-pressed')).toBe('false');
		expect(screen.getByText('Link a parent')).toBeDefined();
	});
});
