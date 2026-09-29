import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import type { ComponentProps } from 'svelte';
import FinishMergeDialog, {
	FINISH_MERGE_FAILURE_COPY,
	GENERIC_FINISH_FAILURE,
	READY_PREVIEW_LIMIT
} from './FinishMergeDialog.svelte';
import type { Branch, BranchMergeResumeResult, MergePendingEntity } from '$lib/api/client';

const ADA = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
const GRACE = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb';

function pending(streamId: string, name: string, reason: MergePendingEntity['reason']): MergePendingEntity {
	return {
		stream_id: streamId,
		entity_type: 'person',
		entity_name: name,
		reason,
		needs_resolution: reason !== 'ready' && reason !== 'needs_repair',
		supported_resolutions:
			reason === 'ready' || reason === 'needs_repair' ? [] : reason === 'main_removed' || reason === 'breaks_reference' ? ['main'] : ['branch', 'main']
	};
}

const branch: Branch = {
	id: '11111111-1111-1111-1111-111111111111',
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'merged',
	created_at: '2026-01-15T10:30:00Z',
	merged_at: '2026-02-01T09:00:00Z',
	outcome: 'open',
	subjects: [],
	proof_summary_ids: [],
	merge_state: 'incomplete',
	merge_pending: [pending(ADA, 'Ada Lovelace', 'breaks_reference'), pending(GRACE, 'Grace Hopper', 'ready')]
};

const done: BranchMergeResumeResult = {
	branch: { ...branch, merge_state: 'complete', merge_pending: undefined },
	merged_at_position: 128,
	replayed_event_count: 3,
	already_replayed_stream_ids: [],
	skipped_stream_ids: [ADA],
	reprojected_stream_ids: []
};

type Props = ComponentProps<typeof FinishMergeDialog>;
const onclose = vi.fn();
const onfinished = vi.fn();
const onrefresh = vi.fn();

function renderDialog(overrides: Partial<Props> = {}) {
	const props: Props = {
		open: true,
		branch,
		onresume: vi.fn().mockResolvedValue(done),
		onclose,
		onfinished,
		onrefresh,
		...overrides
	};
	render(FinishMergeDialog, { props });
	return props;
}

function finishButton(): HTMLButtonElement {
	return screen.getByRole('button', { name: /^(Finish merge|Finishing\.\.\.)$/ }) as HTMLButtonElement;
}

async function decideAdaMain() {
	await fireEvent.click(document.querySelector<HTMLElement>(`#conflict-${ADA}-resolution-main`)!);
}

describe('FinishMergeDialog', () => {
	beforeEach(() => vi.clearAllMocks());
	// bits-ui releases its body-scroll lock on a short timer; let it run.
	afterEach(async () => {
		await new Promise((resolve) => setTimeout(resolve, 30));
	});

	it('lists what is left, by name and why, and holds the finish until every decision is made', async () => {
		const props = renderDialog();

		expect(screen.getByText('Finish merging Maternal Smith line?')).toBeDefined();
		expect(screen.getByText(/2 entities have not reached the mainline yet, and one needs your decision first/)).toBeDefined();
		expect(screen.getByText('Ada Lovelace')).toBeDefined();
		expect(screen.getByText('Would break a reference')).toBeDefined();
		expect(screen.getByText(/Taking the branch's version would be refused/)).toBeDefined();
		expect(screen.getByText('Will be replayed as planned (1)')).toBeDefined();
		expect(screen.getByText('Grace Hopper')).toBeDefined();
		// Only the side the server accepts is offered.
		expect(document.querySelector(`#conflict-${ADA}-resolution-branch`)).toBeNull();

		expect(finishButton().disabled).toBe(true);
		expect(screen.getByText('1 entity still needs a decision.')).toBeDefined();
		await decideAdaMain();
		expect(finishButton().disabled).toBe(false);
		await fireEvent.click(finishButton());

		await screen.findByText('Finished merging Maternal Smith line');
		expect(props.onresume).toHaveBeenCalledWith({ resolutions: [{ stream_id: ADA, resolution: 'main' }] });
		expect(onfinished).toHaveBeenCalledWith(done);
		expect(screen.getByText('3')).toBeDefined();
	});

	it('sends no resolutions when nothing needs a decision', async () => {
		const onresume = vi.fn().mockResolvedValue(done);
		renderDialog({
			onresume,
			branch: { ...branch, merge_pending: [pending(GRACE, 'Grace Hopper', 'ready')] }
		});
		expect(screen.queryByText(/Needs your decision/)).toBeNull();
		await fireEvent.click(finishButton());
		await waitFor(() => expect(onresume).toHaveBeenCalledWith({}));
	});

	it('summarises a long ready list rather than rendering all of it', () => {
		const many = Array.from({ length: READY_PREVIEW_LIMIT + 3 }, (_, i) =>
			pending(`00000000-0000-0000-0000-${String(i).padStart(12, '0')}`, `Person ${i}`, 'ready')
		);
		renderDialog({ branch: { ...branch, merge_pending: many } });
		expect(screen.getByText('+3 more not listed here.')).toBeDefined();
	});

	it('shows the blockers a dangling-reference refusal carries and keeps the decisions', async () => {
		const onresume = vi.fn().mockRejectedValue({
			status: 409,
			code: 'merge_dangling_reference',
			message: 'would dangle',
			blockers: [
				{
					stream_id: ADA,
					entity_type: 'person',
					entity_name: 'Ada Lovelace',
					referenced_id: GRACE,
					referenced_type: 'person',
					referenced_name: 'Grace Hopper',
					kind: 'missing_person',
					suggested_resolution: 'leave_out',
					message: 'would dangle'
				}
			]
		});
		renderDialog({ onresume });
		await decideAdaMain();
		await fireEvent.click(finishButton());

		expect(await screen.findByText(/would break a reference between entities/)).toBeDefined();
		expect(screen.getByRole('list', { name: 'Merge blockers' }).textContent).toMatch(/Grace Hopper/);
		expect(screen.getByText('Merge blocker')).toBeDefined();
		// Still deciding: the finish button is back.
		expect(finishButton().disabled).toBe(false);
	});

	it('keeps a decision already made when the refusal names a different entity', async () => {
		const onresume = vi
			.fn()
			.mockRejectedValueOnce({
				status: 409,
				code: 'merge_resume_needs_resolution',
				message: 'needs resolution',
				pending_stream_ids: [GRACE],
				pending: [pending(GRACE, 'Grace Hopper', 'main_changed')]
			})
			.mockResolvedValueOnce(done);
		renderDialog({ onresume });
		await decideAdaMain();
		const rationale = document.querySelector<HTMLTextAreaElement>(`#conflict-${ADA}-rationale`)!;
		await fireEvent.input(rationale, { target: { value: 'The mainline record is sourced' } });
		await fireEvent.click(finishButton());

		// Grace, ready before, now needs a decision; Ada keeps hers.
		expect(await screen.findByText(/more entities need a decision/)).toBeDefined();
		expect(screen.getByText('Needs your decision (2)')).toBeDefined();
		expect(screen.queryByText(/Will be replayed as planned/)).toBeNull();
		expect(screen.getByText('1 entity still needs a decision.')).toBeDefined();

		await fireEvent.click(document.querySelector<HTMLElement>(`#conflict-${GRACE}-resolution-branch`)!);
		await fireEvent.click(finishButton());
		await screen.findByText('Finished merging Maternal Smith line');
		expect(onresume.mock.calls[1][0]).toEqual({
			resolutions: [
				{ stream_id: ADA, resolution: 'main', rationale: 'The mainline record is sourced' },
				{ stream_id: GRACE, resolution: 'branch' }
			]
		});
	});

	it('lists an entity needing repair apart, without asking for a decision', async () => {
		const onresume = vi.fn().mockResolvedValue(done);
		renderDialog({
			onresume,
			branch: { ...branch, merge_pending: [pending(GRACE, 'Grace Hopper', 'needs_repair')] }
		});
		expect(screen.getByText("Will be repaired from the mainline's history (1)")).toBeDefined();
		expect(screen.getByText(/1 entity reached the mainline's history but not its data/)).toBeDefined();
		expect(screen.queryByText(/Needs your decision/)).toBeNull();
		expect(screen.queryByText(/Will be replayed as planned/)).toBeNull();
		await fireEvent.click(finishButton());
		await waitFor(() => expect(onresume).toHaveBeenCalledWith({}));
	});

	it('offers to finish a merge whose state could not be read', async () => {
		const onresume = vi.fn().mockResolvedValue(done);
		renderDialog({ onresume, branch: { ...branch, merge_state: 'unknown', merge_pending: undefined } });
		expect(screen.getByText(/could not work out how far the merge got/)).toBeDefined();
		await fireEvent.click(finishButton());
		await waitFor(() => expect(onresume).toHaveBeenCalledWith({}));
	});

	it('shows the server message for a validation refusal and keeps deciding', async () => {
		renderDialog({
			onresume: vi.fn().mockRejectedValue({ status: 400, code: 'validation_error', message: 'rationale too long' })
		});
		await decideAdaMain();
		await fireEvent.click(finishButton());
		expect(await screen.findByText('rationale too long')).toBeDefined();
	});

	it('offers a retry after the resume itself stopped partway', async () => {
		const onresume = vi
			.fn()
			.mockRejectedValueOnce({ status: 500, code: 'merge_partially_applied', message: 'stopped after 1 of 2' })
			.mockResolvedValueOnce(done);
		renderDialog({ onresume });
		await decideAdaMain();
		await fireEvent.click(finishButton());

		expect(await screen.findByText(FINISH_MERGE_FAILURE_COPY.merge_partially_applied!.title)).toBeDefined();
		expect(screen.getByText('stopped after 1 of 2')).toBeDefined();
		await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
		await screen.findByText('Finished merging Maternal Smith line');
		expect(onresume).toHaveBeenCalledTimes(2);
		expect(onresume.mock.calls[1][0]).toEqual(onresume.mock.calls[0][0]);
	});

	it('asks for a refresh after a concurrent resume', async () => {
		renderDialog({
			onresume: vi.fn().mockRejectedValue({ status: 409, code: 'merge_resume_concurrent', message: 'raced' })
		});
		await decideAdaMain();
		await fireEvent.click(finishButton());
		await screen.findByText(FINISH_MERGE_FAILURE_COPY.merge_resume_concurrent!.title);
		await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
		expect(onrefresh).toHaveBeenCalledTimes(1);
		expect(onclose).toHaveBeenCalled();
	});

	it('closes without a retry for a branch too large to finish', async () => {
		renderDialog({
			onresume: vi.fn().mockRejectedValue({ status: 409, code: 'branch_too_large', message: 'cap' })
		});
		await decideAdaMain();
		await fireEvent.click(finishButton());
		await screen.findByText(FINISH_MERGE_FAILURE_COPY.branch_too_large!.title);
		expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
		expect(screen.queryByRole('button', { name: 'Refresh' })).toBeNull();
	});

	it('treats anything unrecognised as a generic, non-retryable failure', async () => {
		renderDialog({ onresume: vi.fn().mockRejectedValue(undefined) });
		await decideAdaMain();
		await fireEvent.click(finishButton());
		expect(await screen.findByText(GENERIC_FINISH_FAILURE.title)).toBeDefined();
		expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
		expect(screen.getByRole('button', { name: 'Refresh' })).toBeDefined();
	});

	it('explains an empty pending list', () => {
		renderDialog({ branch: { ...branch, merge_pending: [] } });
		expect(screen.getByText(/Nothing is left to replay/)).toBeDefined();
	});
});
