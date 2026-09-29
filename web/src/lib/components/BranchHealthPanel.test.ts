import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import BranchHealthPanel from './BranchHealthPanel.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getBranchHealth: vi.fn() } };
});

const BRANCH_ID = '11111111-1111-1111-1111-111111111111';
const ADA = '22222222-2222-2222-2222-222222222222';
const TWIN = '33333333-3333-3333-3333-333333333333';

const empty: apiModule.BranchHealth = {
	validation_issues: [],
	quality_issues: [],
	duplicates: [],
	error_count: 0,
	warning_count: 0,
	info_count: 0,
	resolved_count: 0
};

const health: apiModule.BranchHealth = {
	...empty,
	warning_count: 1,
	info_count: 1,
	resolved_count: 2,
	validation_issues: [
		{
			key: 'validation:IMPOSSIBLE_AGE:a::1',
			severity: 'warning',
			code: 'IMPOSSIBLE_AGE',
			message: 'age at death (140 years) exceeds maximum reasonable age',
			record_id: ADA,
			record_type: 'person',
			record_name: 'Ada Sample'
		},
		{
			key: 'validation:MISSING_DEATH_DATE:t::1',
			severity: 'info',
			code: 'MISSING_DEATH_DATE',
			message: 'no death date',
			record_id: TWIN,
			record_type: 'person',
			record_name: 'Ada Twin'
		}
	],
	quality_issues: [
		{
			key: 'q',
			person_id: TWIN,
			person_name: 'Ada Twin',
			issue: 'No family connections'
		}
	],
	duplicates: [
		{
			key: 'duplicate:a:t',
			person1_id: ADA,
			person1_name: 'Ada Sample',
			person2_id: TWIN,
			person2_name: 'Ada Twin',
			confidence: 0.92,
			match_reasons: ['same name', 'same birth date']
		}
	]
};

describe('BranchHealthPanel', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getBranchHealth).mockResolvedValue(health);
	});

	it('lists what the branch introduces, with notices and quality issues grouped', async () => {
		render(BranchHealthPanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});

		expect(await screen.findByText('IMPOSSIBLE_AGE')).toBeDefined();
		expect(apiModule.api.getBranchHealth).toHaveBeenCalledWith(BRANCH_ID);
		expect(
			screen.getByText(
				/This branch introduces 1 warning, 1 notice, 1 quality issue and 1 possible duplicate/
			)
		).toBeDefined();
		expect(screen.getByText(/It also resolves 2\s+findings the mainline has/)).toBeDefined();
		expect(screen.getAllByRole('link', { name: 'Ada Sample' })[0].getAttribute('href')).toBe(
			`/persons/${ADA}`
		);
		expect(screen.getByText('(92% match)')).toBeDefined();
		expect(screen.getByText('same name; same birth date')).toBeDefined();
		// Person merge is not branch-scoped: no merge link, a pointer instead.
		expect(screen.queryByRole('link', { name: /^Review merge/ })).toBeNull();
		expect(screen.getByText(/Person merge does not work on a research branch/)).toBeDefined();
		// On the branch, no switch is offered.
		expect(screen.queryByRole('button', { name: 'Switch to branch' })).toBeNull();

		const notices = screen.getByText('1 notice');
		await fireEvent.click(notices);
		expect(screen.getByText('MISSING_DEATH_DATE')).toBeDefined();
		expect(screen.getByText('1 quality issue')).toBeDefined();
		expect(screen.getByText('No family connections')).toBeDefined();
	});

	it('offers the switch while the reviewer is on another branch', async () => {
		const onswitch = vi.fn();
		render(BranchHealthPanel, {
			props: { branchId: BRANCH_ID, onBranch: false, onswitch }
		});
		await fireEvent.click(await screen.findByRole('button', { name: 'Switch to branch' }));
		expect(onswitch).toHaveBeenCalled();
	});

	it('offers no switch when the branch introduces nothing', async () => {
		vi.mocked(apiModule.api.getBranchHealth).mockResolvedValue(empty);
		render(BranchHealthPanel, {
			props: { branchId: BRANCH_ID, onBranch: false, onswitch: vi.fn() }
		});
		await screen.findByText(/introduces no new validation issues/);
		expect(screen.queryByRole('button', { name: 'Switch to branch' })).toBeNull();
	});

	it('says when the branch introduces nothing', async () => {
		vi.mocked(apiModule.api.getBranchHealth).mockResolvedValue(empty);
		render(BranchHealthPanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});
		expect(
			await screen.findByText(
				'This branch introduces no new validation issues, quality issues or possible duplicates.'
			)
		).toBeDefined();
		expect(screen.queryByText('Validation issues')).toBeNull();
	});

	it('reports a failed check as informational', async () => {
		vi.mocked(apiModule.api.getBranchHealth).mockRejectedValue({
			message: 'boom'
		});
		render(BranchHealthPanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});
		expect(await screen.findByText(/Branch health could not be checked \(boom\)/)).toBeDefined();
	});
});
