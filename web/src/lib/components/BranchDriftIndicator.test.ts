import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import BranchDriftIndicator from './BranchDriftIndicator.svelte';
import type { BranchDrift } from '$lib/api/client';

const BRANCH_ID = '44444444-4444-4444-4444-444444444444';

function drift(overrides: Partial<BranchDrift>): BranchDrift {
	return {
		branch_id: BRANCH_ID,
		base_position: 42,
		main_change_count: 0,
		main_change_count_on_branch_entities: 0,
		has_more: false,
		...overrides
	};
}

function text(): string {
	return (screen.getByTestId('branch-drift').textContent ?? '').replace(/\s+/g, ' ').trim();
}

describe('BranchDriftIndicator', () => {
	it('says the mainline is unchanged and links nowhere when nothing moved', () => {
		render(BranchDriftIndicator, { props: { drift: drift({}) } });

		expect(text()).toBe('Mainline unchanged since you branched.');
		expect(screen.queryByRole('link')).toBeNull();
	});

	it('counts main changes and those on entities the branch touched', () => {
		render(BranchDriftIndicator, {
			props: { drift: drift({ main_change_count: 12, main_change_count_on_branch_entities: 3 }) }
		});

		expect(text()).toContain(
			'Mainline changed 12 times since you branched, 3 on entities this branch touched.'
		);
		const link = screen.getByRole('link', { name: 'Review mainline changes' });
		expect(link.getAttribute('href')).toBe(`/branches/${BRANCH_ID}#main-changes`);
	});

	it('uses the singular for one change', () => {
		render(BranchDriftIndicator, {
			props: { drift: drift({ main_change_count: 1, main_change_count_on_branch_entities: 0 }) }
		});

		expect(text()).toContain('Mainline changed 1 time since you branched, 0 on entities');
	});

	it('offers compare, not a review, when nothing the branch touched moved', () => {
		render(BranchDriftIndicator, {
			props: { drift: drift({ main_change_count: 4, main_change_count_on_branch_entities: 0 }) }
		});

		expect(screen.getByRole('link', { name: 'Open compare' }).getAttribute('href')).toBe(
			`/branches/${BRANCH_ID}#main-changes`
		);
		expect(screen.getByTestId('branch-drift').classList.contains('drift-touched')).toBe(false);
	});

	it('marks capped counts as lower bounds', () => {
		render(BranchDriftIndicator, {
			props: {
				drift: drift({
					main_change_count: 10000,
					main_change_count_on_branch_entities: 25,
					has_more: true
				})
			}
		});

		expect(text()).toContain('Mainline changed 10,000+ times since you branched, 25 on entities');
	});

	it('marks the on-branch count as a lower bound when it reached the capped total', () => {
		render(BranchDriftIndicator, {
			props: {
				drift: drift({
					main_change_count: 10000,
					main_change_count_on_branch_entities: 10000,
					has_more: true
				})
			}
		});

		expect(text()).toContain('10,000+ on entities this branch touched');
	});
});
