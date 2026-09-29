/**
 * A branch's research record page (#836): read-only, rebuilt from events.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch, BranchResearchArchive } from '$lib/api/client';

const BRANCH_ID = '11111111-1111-1111-1111-111111111111';

const { getBranch, getBranchResearch, promoteBranchResearchLogs } = vi.hoisted(() => ({
	getBranch: vi.fn(),
	getBranchResearch: vi.fn(),
	promoteBranchResearchLogs: vi.fn()
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getBranch: (id: string) => getBranch(id),
			getBranchResearch: (id: string) => getBranchResearch(id),
			promoteBranchResearchLogs: (id: string, ids?: string[]) => promoteBranchResearchLogs(id, ids)
		}
	};
});

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (callback: (value: { params: { id: string } }) => void) => {
			callback({ params: { id: BRANCH_ID } });
			return () => {};
		}
	}
}));

const closed: Branch = {
	id: BRANCH_ID,
	name: 'Parent theory',
	base_position: 3,
	status: 'archived',
	outcome: 'disproved',
	hypothesis: 'Was Robin the child of the Example family?',
	subjects: [],
	proof_summary_ids: [],
	created_at: '2026-01-15T10:30:00Z',
	closed_at: '2026-02-01T10:00:00Z',
	close_reason: 'The register names other parents'
};

const archive: BranchResearchArchive = {
	branch_id: BRANCH_ID,
	research_logs: [
		{
			log: {
				id: 'log-1',
				subject_id: 'p1',
				subject_type: 'person',
				repository: 'Parish registers',
				search_description: 'Baptisms 1810-1820',
				outcome: 'not_found',
				notes: 'No entry for Robin',
				search_date: '2024-03-01T00:00:00Z',
				version: 1
			},
			subject_name: 'Robin Placeholder',
			created_on_branch: true
		},
		{
			log: {
				id: 'log-2',
				subject_id: 'p1',
				subject_type: 'person',
				repository: 'County archive',
				search_description: 'Land records',
				outcome: 'found',
				search_date: '2024-02-01T00:00:00Z',
				version: 2
			},
			created_on_branch: false
		}
	],
	evidence_analyses: [
		{
			analysis: {
				id: 'a1',
				fact_type: 'person_birth',
				subject_id: 'p1',
				conclusion: 'No baptism found',
				version: 1
			},
			subject_name: 'Robin Placeholder',
			created_on_branch: true
		}
	],
	proof_summaries: [
		{
			summary: {
				id: 's1',
				fact_type: 'person_birth',
				subject_id: 'p1',
				conclusion: 'Not the family child',
				argument: 'No record supports it',
				version: 1
			},
			created_on_branch: true
		}
	],
	deleted_count: 1,
	truncated: false
};

describe('Branch research page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getBranch.mockResolvedValue(closed);
		getBranchResearch.mockResolvedValue(archive);
	});

	it("lists the branch's research logs, including the searches that found nothing", async () => {
		render(Page);

		const logs = await screen.findAllByTestId('archived-research-log');
		expect(logs).toHaveLength(2);
		expect(logs[0].textContent).toContain('Baptisms 1810-1820');
		expect(logs[0].textContent).toContain('Robin Placeholder');
		expect(logs[0].textContent).toContain('No entry for Robin');
		expect(logs[0].textContent).toContain('Recorded on this branch');
		expect(logs[1].textContent).toContain('Mainline entry, edited');
		expect(screen.getAllByTestId('log-outcome').map((el) => el.textContent?.trim())).toEqual([
			'Not found',
			'Found'
		]);
		expect(screen.getAllByTestId('archived-analysis')[0].textContent).toContain('No baptism found');
		expect(screen.getAllByTestId('archived-proof-summary')[0].textContent).toContain(
			'No record supports it'
		);
		expect(screen.getByTestId('research-close-reason').textContent).toContain(
			'The register names other parents'
		);
		expect(screen.getByText('Was Robin the child of the Example family?')).toBeDefined();
		expect(screen.getByText(/also deleted 1 research entry/)).toBeDefined();
		expect(screen.getByTestId('branch-outcome').getAttribute('data-outcome')).toBe('disproved');
	});

	it('copies the research logs to the mainline and reports the result', async () => {
		promoteBranchResearchLogs.mockResolvedValue({
			promoted: ['log-1'],
			skipped: [{ id: 'log-2', reason: 'not_created_on_branch' }],
			truncated: false
		});
		render(Page);

		await fireEvent.click(
			await screen.findByRole('button', {
				name: 'Copy research logs to the mainline'
			})
		);
		await waitFor(() =>
			expect(promoteBranchResearchLogs).toHaveBeenCalledWith(BRANCH_ID, undefined)
		);
		expect(
			await screen.findByText(
				'1 research log was copied to the mainline (not copied: 1 already mainline entries).'
			)
		).toBeDefined();
	});

	it('reports a failed copy', async () => {
		promoteBranchResearchLogs.mockRejectedValue({
			status: 500,
			message: 'copy failed'
		});
		render(Page);

		await fireEvent.click(
			await screen.findByRole('button', {
				name: 'Copy research logs to the mainline'
			})
		);
		expect((await screen.findByRole('alert')).textContent).toContain('copy failed');
	});

	it('offers no copy for an active branch and says when nothing was recorded', async () => {
		getBranch.mockResolvedValue({
			...closed,
			status: 'active',
			outcome: 'open',
			closed_at: undefined,
			close_reason: undefined
		});
		getBranchResearch.mockResolvedValue({
			...archive,
			research_logs: [],
			evidence_analyses: [],
			proof_summaries: [],
			deleted_count: 0,
			truncated: true
		});
		render(Page);

		expect(await screen.findByText('This branch recorded no searches.')).toBeDefined();
		expect(screen.getByText('This branch recorded no evidence analyses.')).toBeDefined();
		expect(screen.getByText('This branch recorded no proof summaries.')).toBeDefined();
		expect(screen.getByRole('note').textContent).toContain('some research may be missing');
		expect(
			screen.queryByRole('button', {
				name: 'Copy research logs to the mainline'
			})
		).toBeNull();
	});

	it('says when the branch does not exist', async () => {
		getBranch.mockRejectedValue({ status: 404, message: 'Branch not found' });
		render(Page);
		expect(await screen.findByText('Branch not found')).toBeDefined();
	});

	it('surfaces a load failure', async () => {
		getBranchResearch.mockRejectedValue({ status: 500, message: 'boom' });
		render(Page);
		expect((await screen.findByRole('alert')).textContent).toContain('boom');
	});
});
