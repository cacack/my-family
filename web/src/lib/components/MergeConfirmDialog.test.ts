import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import type { ComponentProps } from 'svelte';
import MergeConfirmDialog, {
	EXCLUDED_PREVIEW_LIMIT,
	MERGE_REFUSAL_COPY,
	GENERIC_MERGE_FAILURE,
	NOTE_MAX_LENGTH,
	type MergePlan
} from './MergeConfirmDialog.svelte';
import type { Branch, BranchMergeResult, MergeConflict } from '$lib/api/client';

const branch: Branch = {
	id: '11111111-1111-1111-1111-111111111111',
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'active',
	created_at: '2026-01-15T10:30:00Z'
};

const editEdit: MergeConflict = {
	stream_id: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
	entity_type: 'person',
	entity_name: 'Ada Lovelace',
	kind: 'edit_edit',
	fields: ['surname'],
	detail: 'Both sides changed surname to different values',
	supported_resolutions: ['branch', 'main']
};

// The schema documents `entity_name` as possibly empty - an entity that exists
// only on the branch, or only as a deletion.
const unnamed: MergeConflict = {
	stream_id: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
	entity_type: 'family',
	entity_name: '',
	kind: 'delete_edit',
	detail: 'Deleted on the mainline while the branch went on editing it',
	supported_resolutions: ['main']
};

const plan: MergePlan = {
	mergingCount: 5,
	excluded: [
		{
			streamId: 'cccccccc-cccc-cccc-cccc-cccccccccccc',
			entityType: 'family',
			entityName: 'Smith / Jones'
		}
	],
	decisions: [
		{ conflict: editEdit, resolution: 'branch' },
		{ conflict: unnamed, resolution: 'main' }
	],
	hasMore: false
};

const mergeResult: BranchMergeResult = {
	branch: { ...branch, status: 'merged', merged_at: '2026-02-01T09:00:00Z' },
	merged_at_position: 128,
	replayed_event_count: 7,
	skipped_stream_ids: ['cccccccc-cccc-cccc-cccc-cccccccccccc']
};

type Props = ComponentProps<typeof MergeConfirmDialog>;

const onclose = vi.fn();
const onrefused = vi.fn();
const onrecompare = vi.fn();
const onreturntomainline = vi.fn();

function renderDialog(overrides: Partial<Props> = {}) {
	const props: Props = {
		open: true,
		branch,
		plan,
		onconfirm: vi.fn().mockResolvedValue(mergeResult),
		onclose,
		onrefused,
		onrecompare,
		onreturntomainline,
		...overrides
	};
	render(MergeConfirmDialog, { props });
	return props;
}

/** Drives the dialog to a failure state and hands back the confirm spy. */
async function refuse(error: unknown) {
	const onconfirm = vi.fn().mockRejectedValue(error);
	renderDialog({ onconfirm });
	await fireEvent.click(screen.getByRole('button', { name: 'Merge branch' }));
	return onconfirm;
}

function noteField(): HTMLTextAreaElement {
	return screen.getByLabelText(/note/i) as HTMLTextAreaElement;
}

function mergeButton(): HTMLButtonElement {
	return screen.getByRole('button', { name: /^(Merge branch|Merging\.\.\.)$/ }) as HTMLButtonElement;
}

describe('MergeConfirmDialog', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	// This file opens a bits-ui AlertDialog, and bits-ui releases its body-scroll
	// lock on a 24ms timer. Tearing down inside that window runs the callback
	// against a destroyed document and fails the run even though every test
	// passed. Draining past the delay keeps the DOM alive for it. Same reason as
	// `web/src/routes/branches/page.test.ts`.
	afterEach(async () => {
		await new Promise((resolve) => setTimeout(resolve, 30));
	});

	describe('preview', () => {
		it('says in plain language what will merge and what will not', () => {
			renderDialog();

			expect(screen.getByText(/5 entities from this branch will be replayed/)).toBeDefined();
			expect(screen.getByText(/1 entity will be left behind/)).toBeDefined();
		});

		it('says nothing is left behind when nothing is excluded', () => {
			renderDialog({ plan: { ...plan, excluded: [] } });

			expect(screen.getByText(/Nothing is being left behind/)).toBeDefined();
		});

		it('lists each resolved conflict with the side that won', () => {
			renderDialog();

			expect(screen.getByText('Ada Lovelace')).toBeDefined();
			expect(screen.getByText('This branch wins')).toBeDefined();
			expect(screen.getByText(/branch's version is replayed onto the mainline/)).toBeDefined();
			// The `main` side of the second decision, and its empty-name fallback.
			expect(screen.getByText('Unnamed entity')).toBeDefined();
			expect(screen.getByText('The mainline wins')).toBeDefined();
			expect(screen.getByText(/mainline's version stands/)).toBeDefined();
		});

		it('lists the excluded entities, marked as not being promoted', () => {
			renderDialog();

			expect(screen.getByText('Smith / Jones')).toBeDefined();
			expect(screen.getByText('Not merging')).toBeDefined();
			expect(screen.getByText(/These are not being promoted/)).toBeDefined();
		});

		// The page's own change list stays mounted behind the overlay, so rendering
		// every exclusion here roughly doubles the live list nodes for a branch
		// that excludes most of itself. Only the rows are capped - every count the
		// dialog states is still the true total.
		it('caps the left-behind list and summarises the rest', () => {
			const excluded = Array.from({ length: EXCLUDED_PREVIEW_LIMIT + 5 }, (_, i) => ({
				streamId: `stream-${i}`,
				entityType: 'person',
				entityName: `Person ${i}`
			}));
			renderDialog({ plan: { ...plan, excluded } });

			expect(screen.getAllByText('Not merging')).toHaveLength(EXCLUDED_PREVIEW_LIMIT);
			expect(screen.getByText('Person 0')).toBeDefined();
			expect(screen.queryByText(`Person ${EXCLUDED_PREVIEW_LIMIT}`)).toBeNull();
			expect(screen.getByText(/\+5 more not listed here/)).toBeDefined();
			expect(screen.getByText(/All 25 are left behind/)).toBeDefined();
			expect(screen.getByRole('heading', { name: 'Left behind (25)' })).toBeDefined();
			expect(screen.getByText(/25 entities will be left behind/)).toBeDefined();
		});

		it('lists every exclusion when they fit under the cap', () => {
			renderDialog();

			expect(screen.queryByText(/more not listed here/)).toBeNull();
		});

		it('warns that the review is partial only when the comparison was truncated', () => {
			renderDialog();
			expect(screen.queryByText(/partial/)).toBeNull();
		});

		it('warns when the comparison hit the read cap', () => {
			renderDialog({ plan: { ...plan, hasMore: true } });

			expect(screen.getByRole('note').textContent).toMatch(/incomplete information/);
		});
	});

	describe('the note', () => {
		it('allows an empty note', async () => {
			const props = renderDialog();

			expect(mergeButton().disabled).toBe(false);
			await fireEvent.click(mergeButton());

			await waitFor(() => expect(props.onconfirm).toHaveBeenCalledWith(''));
		});

		it('measures the cap on the trimmed note, so trailing whitespace is free', async () => {
			const props = renderDialog();

			const note = 'a'.repeat(NOTE_MAX_LENGTH);
			await fireEvent.input(noteField(), { target: { value: `${note}   ` } });

			expect(mergeButton().disabled).toBe(false);
			await fireEvent.click(mergeButton());

			await waitFor(() => expect(props.onconfirm).toHaveBeenCalledWith(note));
		});

		it('refuses to send a note longer than the cap once trimmed', async () => {
			const props = renderDialog();

			await fireEvent.input(noteField(), { target: { value: 'a'.repeat(NOTE_MAX_LENGTH + 1) } });

			expect(mergeButton().disabled).toBe(true);
			expect(screen.getByText(/1 bytes over the limit/)).toBeDefined();
			expect(props.onconfirm).not.toHaveBeenCalled();
		});

		// The server counts bytes (`internal/domain/branch.go`), so a counter that
		// counted JS string length would read "334/1000" here and let through a
		// note the merge then refuses with `400 validation_error`.
		it('counts the note in bytes, as the server does', async () => {
			const props = renderDialog();

			// 334 CJK characters = 1002 UTF-8 bytes: two over, and only two.
			await fireEvent.input(noteField(), { target: { value: '家'.repeat(334) } });

			expect(screen.getByText(`1002/${NOTE_MAX_LENGTH}`)).toBeDefined();
			expect(mergeButton().disabled).toBe(true);
			expect(screen.getByText(/2 bytes over the limit/)).toBeDefined();
			expect(props.onconfirm).not.toHaveBeenCalled();
		});
	});

	describe('confirming', () => {
		it('keeps the dialog open and the action disabled while the request is in flight', async () => {
			let settle: (result: BranchMergeResult) => void = () => {};
			const onconfirm = vi.fn(
				() =>
					new Promise<BranchMergeResult>((resolve) => {
						settle = resolve;
					})
			);
			renderDialog({ onconfirm });

			await fireEvent.input(noteField(), { target: { value: 'Confirmed by the 1881 census' } });
			await fireEvent.click(mergeButton());

			await waitFor(() => expect(onconfirm).toHaveBeenCalledWith('Confirmed by the 1881 census'));
			expect(mergeButton().textContent).toContain('Merging...');
			expect(mergeButton().disabled).toBe(true);

			// bits-ui's Cancel swallows `disabled` rather than setting it on the
			// button, so `aria-disabled` is what says so - and clicking it while
			// pending must not dismiss.
			const cancel = screen.getByRole('button', { name: 'Cancel' });
			expect(cancel.getAttribute('aria-disabled')).toBe('true');
			await fireEvent.click(cancel);
			expect(onclose).not.toHaveBeenCalled();

			// Still on screen: the outcome has to land where the user is looking.
			expect(screen.getByText(/Merge Maternal Smith line into the mainline\?/)).toBeDefined();
			expect(onclose).not.toHaveBeenCalled();

			settle(mergeResult);
			await screen.findByText(/Merged Maternal Smith line into the mainline/);
		});
	});

	describe('refusals', () => {
		it.each(Object.entries(MERGE_REFUSAL_COPY))(
			'%s renders its own copy, the server message, and a retry only when retrying is the fix',
			async (code, copy) => {
				const message = `server detail for ${code}`;
				await refuse({ status: code === 'merge_partially_applied' ? 500 : 409, code, message });

				expect(await screen.findByText(copy.title)).toBeDefined();
				expect(screen.getByText(copy.body)).toBeDefined();
				// The static copy cannot carry stream ids, versions or caps - the
				// server's own message is the only place those appear.
				expect(screen.getByText(message)).toBeDefined();

				const retry = screen.queryByRole('button', { name: /try merging again/i });
				if (copy.recovery === 'retry') {
					expect(retry).not.toBeNull();
				} else {
					expect(retry).toBeNull();
				}
			}
		);

		it('offers a retry for merge_plan_stale that re-issues with the same note', async () => {
			const onconfirm = vi
				.fn()
				.mockRejectedValueOnce({
					status: 409,
					code: 'merge_plan_stale',
					message: 'stream aaaa moved from version 3 to 4'
				})
				.mockResolvedValueOnce(mergeResult);
			renderDialog({ onconfirm });

			await fireEvent.input(noteField(), { target: { value: 'Census confirmed' } });
			await fireEvent.click(mergeButton());

			const retry = await screen.findByRole('button', { name: /try merging again/i });
			await fireEvent.click(retry);

			await waitFor(() => expect(onconfirm).toHaveBeenCalledTimes(2));
			expect(onconfirm).toHaveBeenNthCalledWith(2, 'Census confirmed');
			await screen.findByText(/Merged Maternal Smith line into the mainline/);
		});

		it('does not offer a retry for branch_too_large, and links #684', async () => {
			await refuse({
				status: 409,
				code: 'branch_too_large',
				message: 'branch scan hit the cap of 1000 entities'
			});

			await screen.findByText(MERGE_REFUSAL_COPY.branch_too_large.title);
			expect(screen.queryByRole('button', { name: /try merging again/i })).toBeNull();
			expect(screen.queryByRole('button', { name: /compare again/i })).toBeNull();
			expect(screen.getByRole('link', { name: '#684' }).getAttribute('href')).toBe(
				'https://github.com/cacack/my-family/issues/684'
			);
		});

		it('does not offer a retry for merge_partially_applied, and links #685', async () => {
			await refuse({
				status: 500,
				code: 'merge_partially_applied',
				message: 'replay stopped after 3 of 9 streams; main was modified'
			});

			await screen.findByText(MERGE_REFUSAL_COPY.merge_partially_applied.title);
			expect(screen.queryByRole('button', { name: /try merging again/i })).toBeNull();
			expect(screen.getByRole('link', { name: '#685' }).getAttribute('href')).toBe(
				'https://github.com/cacack/my-family/issues/685'
			);
			// The only valid recovery: verify by comparing again.
			expect(screen.getByRole('button', { name: /compare again/i })).toBeDefined();
		});

		it('names the mainline, not the branch, for main_too_far_ahead', async () => {
			await refuse({
				status: 409,
				code: 'main_too_far_ahead',
				message: 'mainline scan hit the cap'
			});

			const copy = await screen.findByText(MERGE_REFUSAL_COPY.main_too_far_ahead.body);
			expect(copy.textContent).toMatch(/not the size of your branch/);
		});

		it('hands a merge_conflicts refusal to the page so it can re-render the pickers', async () => {
			const refusal = {
				status: 409,
				code: 'merge_conflicts',
				message: '1 of 1 conflicts have no resolution',
				conflicts: [editEdit]
			};
			await refuse(refusal);

			await screen.findByText(MERGE_REFUSAL_COPY.merge_conflicts.title);
			expect(onrefused).toHaveBeenCalledWith(expect.objectContaining({ conflicts: [editEdit] }));
			// Closing is the affordance - the page owns the fresh conflict list.
			expect(screen.queryByRole('button', { name: /try merging again/i })).toBeNull();
			expect(screen.getByRole('button', { name: 'Close' })).toBeDefined();
		});

		it('re-runs the comparison when the refusal means the review is out of date', async () => {
			await refuse({
				status: 409,
				code: 'branch_not_active',
				message: 'branch is already merged'
			});

			await fireEvent.click(await screen.findByRole('button', { name: /compare again/i }));

			expect(onrecompare).toHaveBeenCalled();
			expect(onclose).toHaveBeenCalled();
		});

		it('falls back to a generic message with no retry for an unrecognized failure', async () => {
			await refuse({ status: 500, code: 'internal_error', message: 'boom' });

			expect(await screen.findByText(GENERIC_MERGE_FAILURE.title)).toBeDefined();
			expect(screen.getByText('boom')).toBeDefined();
			expect(screen.queryByRole('button', { name: /try merging again/i })).toBeNull();
			expect(screen.queryByRole('button', { name: /compare again/i })).toBeNull();
			// Not a documented refusal, so the page is told nothing it could act on.
			expect(onrefused).not.toHaveBeenCalled();
		});

		// A bare `Promise.reject()` is legal JS and the default of several mocking
		// helpers. Reading `.message` off it would throw inside the catch block and
		// surface as an unhandled rejection instead of this dialog's own failure UI.
		it.each([[undefined], [null], ['boom'], [42]])(
			'shows the generic failure for a rejection value of %s',
			async (value) => {
				await refuse(value);

				expect(await screen.findByText(GENERIC_MERGE_FAILURE.title)).toBeDefined();
				expect(screen.getByText(GENERIC_MERGE_FAILURE.body)).toBeDefined();
				expect(screen.getByText('The merge could not be completed.')).toBeDefined();
				expect(onrefused).not.toHaveBeenCalled();
			}
		);

		it('offers a re-comparison for a resolution the merge no longer supports', async () => {
			await refuse({
				status: 400,
				code: 'invalid_resolution',
				message: "resolution 'branch' is not supported for stream aaaa"
			});

			await screen.findByText(MERGE_REFUSAL_COPY.invalid_resolution.title);
			// Re-comparing is the fix: it refetches supported_resolutions and clears
			// the decision that no longer applies.
			expect(screen.getByRole('button', { name: /compare again/i })).toBeDefined();
			expect(screen.queryByRole('button', { name: /try merging again/i })).toBeNull();
		});
	});

	describe('success', () => {
		it('reports what the merge did, in the endpoint\'s own terms', async () => {
			renderDialog();

			await fireEvent.click(mergeButton());

			await screen.findByText(/Merged Maternal Smith line into the mainline/);
			expect(screen.getByText('Events replayed').parentElement?.textContent).toContain('7');
			expect(screen.getByText('Entities left behind').parentElement?.textContent).toContain('1');
			expect(screen.getByText('Recorded at position').parentElement?.textContent).toContain('128');
			// The position is a global sequence, not a count of anything.
			expect(screen.getByText(/not a count of what was promoted/)).toBeDefined();
		});

		it('offers a return to mainline only when the merged branch is the active one', async () => {
			renderDialog();

			await fireEvent.click(mergeButton());
			await screen.findByText(/Merged Maternal Smith line into the mainline/);

			expect(screen.queryByRole('button', { name: /return to mainline/i })).toBeNull();
		});

		it('lets the page perform the return to mainline', async () => {
			renderDialog({ isActiveBranch: true });

			await fireEvent.click(mergeButton());
			await screen.findByText(/Merged Maternal Smith line into the mainline/);

			await fireEvent.click(screen.getByRole('button', { name: /return to mainline/i }));
			expect(onreturntomainline).toHaveBeenCalled();
		});
	});

	describe('dismissal', () => {
		it('dismisses on Escape while idle', async () => {
			renderDialog();

			await fireEvent.keyDown(document, { key: 'Escape' });

			await waitFor(() => expect(onclose).toHaveBeenCalled());
		});

		it('dismisses on Cancel while idle', async () => {
			renderDialog();

			await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

			await waitFor(() => expect(onclose).toHaveBeenCalled());
		});

		it('does not dismiss on Escape while a merge is in flight', async () => {
			const onconfirm = vi.fn(() => new Promise<BranchMergeResult>(() => {}));
			renderDialog({ onconfirm });

			await fireEvent.click(mergeButton());
			await waitFor(() => expect(onconfirm).toHaveBeenCalled());

			await fireEvent.keyDown(document, { key: 'Escape' });

			expect(onclose).not.toHaveBeenCalled();
			expect(screen.getByText(/Merge Maternal Smith line into the mainline\?/)).toBeDefined();
		});
	});
});
