import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import MergedFromChip from './MergedFromChip.svelte';
import type { MergeOrigin } from '#lib/api/client.js';
import { auth } from '#lib/stores/access.svelte.js';
import { signInAs } from '#lib/test/roles.js';

const origin: MergeOrigin = {
	branch_id: '33333333-3333-3333-3333-333333333333',
	branch_name: 'Byron theory',
	note: 'Baptism record found',
	merged_at: '2026-02-01T12:00:00Z',
	original_timestamp: '2026-01-20T09:00:00Z'
};

describe('MergedFromChip', () => {
	it("links a signed-in user to the merged branch's page", () => {
		signInAs('viewer');
		render(MergedFromChip, { origin });

		const chip = screen.getByTestId('merged-from');
		expect(chip.tagName).toBe('A');
		expect(chip.getAttribute('href')).toBe(`/branches/${origin.branch_id}`);
	});

	it('shows a visitor the provenance without a link to the branch (#1535)', () => {
		auth.phase = 'visitor';
		render(MergedFromChip, { origin });

		const chip = screen.getByTestId('merged-from');
		expect(chip.tagName).toBe('SPAN');
		expect(chip.hasAttribute('href')).toBe(false);
		expect(chip.textContent).toContain('Byron theory');
		expect(screen.queryByRole('link')).toBeNull();
	});
});
