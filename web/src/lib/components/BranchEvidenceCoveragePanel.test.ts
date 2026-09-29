import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import BranchEvidenceCoveragePanel from './BranchEvidenceCoveragePanel.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getBranchEvidenceCoverage: vi.fn() } };
});

const BRANCH_ID = '11111111-1111-1111-1111-111111111111';
const PERSON_ID = '22222222-2222-2222-2222-222222222222';
const FAMILY_ID = '33333333-3333-3333-3333-333333333333';

const coverage: apiModule.BranchEvidenceCoverage = {
	changed_fact_count: 3,
	has_more: false,
	uncovered: [
		{
			kind: 'fact',
			fact_type: 'person_death',
			subject_type: 'person',
			subject_id: PERSON_ID,
			subject_name: 'Ada Sample',
			change_count: 2
		},
		{
			kind: 'relationship',
			subject_type: 'family',
			subject_id: FAMILY_ID,
			subject_name: 'Ada Sample & Bob Sample',
			change_count: 1
		}
	]
};

describe('BranchEvidenceCoveragePanel', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getBranchEvidenceCoverage).mockResolvedValue(coverage);
	});

	it('lists each undocumented change with links to the fact and to "add analysis"', async () => {
		render(BranchEvidenceCoveragePanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});

		expect(
			await screen.findByRole('heading', {
				name: '2 changed facts or relationships have no evidence analysis or proof summary on this branch'
			})
		).toBeDefined();
		expect(apiModule.api.getBranchEvidenceCoverage).toHaveBeenCalledWith(BRANCH_ID);
		expect(screen.getByRole('link', { name: 'Ada Sample' }).getAttribute('href')).toBe(
			`/persons/${PERSON_ID}`
		);
		expect(screen.getByRole('link', { name: 'Ada Sample & Bob Sample' }).getAttribute('href')).toBe(
			`/families/${FAMILY_ID}`
		);
		expect(screen.getByText('Death')).toBeDefined();
		expect(screen.getByText('Partners or children')).toBeDefined();
		expect(screen.getByText('2 changes')).toBeDefined();
		const add = screen.getAllByRole('link', { name: /^Add analysis/ });
		expect(add.map((a) => a.getAttribute('href'))).toEqual([
			`/evidence/analyses/new?subjectId=${PERSON_ID}&factType=person_death`,
			`/evidence/analyses/new?subjectId=${FAMILY_ID}&subjectType=family`
		]);
		expect(screen.queryByRole('button', { name: 'Switch to branch' })).toBeNull();
	});

	it('lists a deleted record by name, without a link to its page', async () => {
		vi.mocked(apiModule.api.getBranchEvidenceCoverage).mockResolvedValue({
			changed_fact_count: 1,
			has_more: false,
			uncovered: [
				{
					kind: 'deletion',
					subject_type: 'family',
					subject_id: FAMILY_ID,
					subject_name: 'Ada Sample & Bob Sample',
					change_count: 1
				}
			]
		});
		render(BranchEvidenceCoveragePanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});
		expect(await screen.findByText('Family deleted')).toBeDefined();
		expect(screen.getByText('Ada Sample & Bob Sample')).toBeDefined();
		expect(screen.queryByRole('link', { name: 'Ada Sample & Bob Sample' })).toBeNull();
		expect(screen.getByRole('link', { name: /^Add analysis/ }).getAttribute('href')).toBe(
			`/evidence/analyses/new?subjectId=${FAMILY_ID}&subjectType=family`
		);
	});

	it('offers the switch instead of "add analysis" off the branch', async () => {
		const onswitch = vi.fn();
		render(BranchEvidenceCoveragePanel, {
			props: { branchId: BRANCH_ID, onBranch: false, onswitch }
		});

		await fireEvent.click(await screen.findByRole('button', { name: 'Switch to branch' }));
		expect(onswitch).toHaveBeenCalledOnce();
		expect(screen.queryByRole('link', { name: /^Add analysis/ })).toBeNull();
	});

	it('says so when every change is documented, and shows nothing for no changes', async () => {
		vi.mocked(apiModule.api.getBranchEvidenceCoverage).mockResolvedValue({
			changed_fact_count: 2,
			has_more: false,
			uncovered: []
		});
		const { unmount } = render(BranchEvidenceCoveragePanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});
		expect(
			await screen.findByText(/Every fact and relationship this branch changed/)
		).toBeDefined();
		unmount();

		vi.mocked(apiModule.api.getBranchEvidenceCoverage).mockResolvedValue({
			changed_fact_count: 0,
			has_more: false,
			uncovered: []
		});
		render(BranchEvidenceCoveragePanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});
		await vi.waitFor(() => expect(screen.queryByRole('status')).toBeNull());
		expect(screen.queryByTestId('evidence-coverage')).toBeNull();
	});

	it('notes a partial list and a failed check without holding anything', async () => {
		vi.mocked(apiModule.api.getBranchEvidenceCoverage).mockResolvedValue({
			...coverage,
			has_more: true
		});
		const { unmount } = render(BranchEvidenceCoveragePanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});
		expect(await screen.findByText(/this list may be incomplete/)).toBeDefined();
		unmount();

		vi.mocked(apiModule.api.getBranchEvidenceCoverage).mockRejectedValue({
			message: 'boom'
		});
		render(BranchEvidenceCoveragePanel, {
			props: { branchId: BRANCH_ID, onBranch: true, onswitch: vi.fn() }
		});
		expect(
			await screen.findByText(/Evidence coverage could not be checked \(boom\)/)
		).toBeDefined();
	});
});
