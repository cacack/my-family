import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type {
	Branch,
	BranchChangeEntry,
	BranchComparisonResult,
	BranchMergePrecheckRequest,
	BranchMergeRequest,
	BranchMergeResult,
	MergeBlocker,
	MergeConflict
} from '$lib/api/client';

const BRANCH_ID = '11111111-1111-1111-1111-111111111111';
const PERSON_ID = '99999999-9999-9999-9999-999999999999';
const OTHER_ID = '88888888-8888-8888-8888-888888888888';
const THIRD_ID = '77777777-7777-7777-7777-777777777777';

// Hoisted so the module mocks below (which vitest lifts above the imports) can
// close over them.
const {
	mockState,
	compareBranch,
	mergeBranch,
	precheckBranchMerge,
	switchBranch,
	returnToMainline,
	routeState
} = vi.hoisted(() => ({
		mockState: {
			id: null as string | null,
			branch: null as Branch | null,
			revalidating: false,
			unconfirmed: false,
			notice: null as string | null
		},
		compareBranch: vi.fn(),
		mergeBranch: vi.fn(),
		precheckBranchMerge: vi.fn(),
		switchBranch: vi.fn().mockResolvedValue(undefined),
		returnToMainline: vi.fn(),
		// A soft navigation between two /branches/{id} entries reuses the component,
		// so the route store has to be drivable rather than fixed.
		routeState: {
			current: { params: { id: '' } } as { params: { id: string } },
			subscribers: new Set<(value: { params: { id: string } }) => void>()
		}
	}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			compareBranch: (id: string) => compareBranch(id),
			mergeBranch: (id: string, req: BranchMergeRequest) => mergeBranch(id, req),
			precheckBranchMerge: (id: string, req: BranchMergePrecheckRequest) =>
				precheckBranchMerge(id, req)
		}
	};
});

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: mockState,
	switchBranch: (branch: Branch | null) => switchBranch(branch),
	returnToMainline: () => returnToMainline()
}));

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (callback: (value: { params: { id: string } }) => void) => {
			routeState.subscribers.add(callback);
			callback(routeState.current);
			return () => routeState.subscribers.delete(callback);
		}
	}
}));

/** Navigate to another `/branches/{id}` without remounting the component. */
function navigateTo(id: string) {
	// A fresh object: Svelte's store bridge dedupes on identity, so mutating the
	// existing one would not re-run the effect.
	routeState.current = { params: { id } };
	for (const subscriber of routeState.subscribers) {
		subscriber(routeState.current);
	}
}

const branch: Branch = {
	id: BRANCH_ID,
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'active',
	created_at: '2026-01-15T10:30:00Z'
};

function comparison(overrides: Partial<BranchComparisonResult> = {}): BranchComparisonResult {
	return {
		branch,
		base_position: 42,
		branch_changes: [
			{
				id: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
				timestamp: '2026-01-16T10:30:00Z',
				entity_type: 'person',
				entity_id: PERSON_ID,
				entity_name: 'Ada Lovelace',
				action: 'updated',
				changes: { surname: { old_value: 'Byron', new_value: 'Lovelace' } }
			}
		],
		main_changes: [
			{
				id: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
				timestamp: '2026-01-17T10:30:00Z',
				entity_type: 'person',
				entity_id: PERSON_ID,
				entity_name: 'Ada Lovelace',
				action: 'updated',
				changes: { surname: { old_value: 'Byron', new_value: 'King' } }
			}
		],
		branch_change_count: 1,
		main_change_count: 1,
		has_more: false,
		overlapping_stream_ids: [PERSON_ID],
		conflicts: [
			{
				stream_id: PERSON_ID,
				supported_resolutions: ['branch', 'main'],
				entity_type: 'person',
				entity_name: 'Ada Lovelace',
				kind: 'edit_edit',
				fields: ['surname'],
				detail: 'Both sides changed surname to different values'
			}
		],
		...overrides
	};
}

/** A branch-side change entry, so a test can put several entities on the branch. */
function branchEntry(id: string, entityId: string, entityName: string): BranchChangeEntry {
	return {
		id,
		timestamp: '2026-01-16T10:30:00Z',
		entity_type: 'person',
		entity_id: entityId,
		entity_name: entityName,
		action: 'updated'
	};
}

function editEdit(streamId: string, entityName: string): MergeConflict {
	return {
		stream_id: streamId,
		supported_resolutions: ['branch', 'main'],
		entity_type: 'person',
		entity_name: entityName,
		kind: 'edit_edit',
		fields: ['surname'],
		detail: `Both sides changed ${entityName}`
	};
}

function mergeResult(overrides: Partial<BranchMergeResult> = {}): BranchMergeResult {
	return {
		branch: { ...branch, status: 'merged', merged_at: '2026-02-01T09:00:00Z' },
		merged_at_position: 128,
		replayed_event_count: 7,
		skipped_stream_ids: [],
		...overrides
	};
}

/**
 * The page's own radio for one conflict/resolution pair. Queried off `container`
 * on purpose: the confirm dialog portals to `document.body`, so a global query
 * cannot tell page content from dialog content once it is open.
 */
function radio(container: HTMLElement, streamId: string, resolution: string): HTMLElement {
	const el = container.querySelector<HTMLElement>(
		`#conflict-${streamId}-resolution-${resolution}`
	);
	if (!el) throw new Error(`no ${resolution} radio for ${streamId}`);
	return el;
}

/** A two-entity branch: Ada is the conflict, Grace is a clean change. */
function twoEntityComparison(overrides: Partial<BranchComparisonResult> = {}) {
	return comparison({
		branch_changes: [
			branchEntry('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', PERSON_ID, 'Ada Lovelace'),
			branchEntry('cccccccc-cccc-cccc-cccc-cccccccccccc', OTHER_ID, 'Grace Hopper')
		],
		branch_change_count: 2,
		...overrides
	});
}

/** The dialog's "Left behind" section, so its rows are not confused with the page's. */
function leftBehindSection(dialog: HTMLElement): HTMLElement {
	const heading = within(dialog).getByRole('heading', { name: /^Left behind/ });
	const section = heading.closest('section');
	if (!section) throw new Error('the Left behind heading is not inside a section');
	return section as HTMLElement;
}

/** The resolutions payload of the nth `mergeBranch` call, order-independent. */
function sentResolutions(call = 0) {
	const req = mergeBranch.mock.calls[call][1] as BranchMergeRequest;
	return [...(req.resolutions ?? [])].sort((a, b) => a.stream_id.localeCompare(b.stream_id));
}

describe('Branch comparison page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockState.id = null;
		mockState.branch = null;
		routeState.current = { params: { id: BRANCH_ID } };
		routeState.subscribers.clear();
		compareBranch.mockResolvedValue(comparison());
		mergeBranch.mockResolvedValue(mergeResult());
		precheckBranchMerge.mockResolvedValue({ blockers: [] });
	});

	// The merge tests open a bits-ui AlertDialog, which releases its body-scroll
	// lock on a 24ms timer. Tearing down inside that window runs the callback
	// against a destroyed document and fails the run even though every test
	// passed. Draining past the delay keeps the DOM alive for it - the same
	// reason `web/src/routes/branches/page.test.ts` does it.
	afterEach(async () => {
		await new Promise((resolve) => setTimeout(resolve, 30));
	});

	it('loads the comparison for the routed branch', async () => {
		render(Page);
		await screen.findByRole('heading', { name: 'Maternal Smith line' });
		expect(compareBranch).toHaveBeenCalledWith(BRANCH_ID);
	});

	it('shows both sides of the divergence with their diffs', async () => {
		render(Page);

		await screen.findByRole('heading', { name: 'On this branch' });
		expect(screen.getByRole('heading', { name: 'On the mainline' })).toBeDefined();
		expect(screen.getByText('Lovelace')).toBeDefined();
		expect(screen.getByText('King')).toBeDefined();
	});

	it('renders every branch-aware entity type by label, name and page, never as a raw id', async () => {
		const NOTE_ID = '55555555-5555-5555-5555-555555555555';
		const EVENT_ID = '44444444-4444-4444-4444-444444444444';
		const LOG_ID = '33333333-3333-3333-3333-333333333333';
		compareBranch.mockResolvedValue(
			comparison({
				branch_changes: [
					{
						id: 'e1',
						timestamp: '2026-01-16T10:30:00Z',
						entity_type: 'person',
						entity_id: PERSON_ID,
						entity_name: 'Ada Lovelace',
						action: 'updated',
						changes: {
							name: { old_value: 'Ada Byron', new_value: 'Augusta Ada Byron' }
						}
					},
					{
						id: 'e2',
						timestamp: '2026-01-16T10:31:00Z',
						entity_type: 'note',
						entity_id: NOTE_ID,
						entity_name: 'Ada wrote the first algorithm',
						action: 'created'
					},
					{
						id: 'e3',
						timestamp: '2026-01-16T10:32:00Z',
						entity_type: 'life_event',
						entity_id: EVENT_ID,
						entity_name: 'Birth, 10 DEC 1815, London',
						action: 'updated',
						parent_entity_type: 'person',
						parent_entity_id: PERSON_ID,
						changes: {
							place: { old_value: 'Marylebone', new_value: 'London' }
						}
					},
					{
						id: 'e4',
						timestamp: '2026-01-16T10:33:00Z',
						entity_type: 'research_log',
						entity_id: LOG_ID,
						entity_name: 'Searched baptism registers (National Archives)',
						action: 'created'
					}
				],
				branch_change_count: 4,
				conflicts: [],
				overlapping_stream_ids: []
			})
		);

		render(Page);
		const side = await screen.findByTestId('branch-changes');
		const items = within(side).getAllByRole('listitem');
		expect(items).toHaveLength(4);

		// The person's name change shows what it replaced.
		expect(within(items[0]).getByText('Ada Byron')).toBeDefined();
		expect(within(items[0]).getByText('Augusta Ada Byron')).toBeDefined();

		// A note has no page, so it is named but not linked.
		expect(within(items[1]).getByText('Note')).toBeDefined();
		expect(within(items[1]).getAllByText('Ada wrote the first algorithm')).toHaveLength(2);
		expect(within(items[1]).queryByRole('link')).toBeNull();
		// ...and, being live, is not struck through as if deleted.
		expect(items[1].querySelector('.entity-name')?.classList.contains('deleted')).toBe(false);

		// A life event links to the person that presents it.
		expect(within(items[2]).getByText('Life event')).toBeDefined();
		expect(
			within(items[2])
				.getByRole('link', { name: 'Birth, 10 DEC 1815, London' })
				.getAttribute('href')
		).toBe(`/persons/${PERSON_ID}`);
		expect(within(items[2]).getByText('Marylebone')).toBeDefined();

		// A research log links to its own page.
		expect(
			within(items[3])
				.getByRole('link', {
					name: 'Searched baptism registers (National Archives)'
				})
				.getAttribute('href')
		).toBe(`/evidence/research-logs/${LOG_ID}`);

		// The leave-out control is labelled with the entity's name, visibly too.
		expect(
			within(items[1]).getByRole('checkbox', {
				name: 'Leave out of the merge: Ada wrote the first algorithm'
			})
		).toBeDefined();
		expect(within(items[1]).getByText(/Leave out of the merge:/).textContent).toContain(
			'Ada wrote the first algorithm'
		);

		expect(screen.queryByText(/unknown/i)).toBeNull();
		for (const id of [NOTE_ID, EVENT_ID, LOG_ID]) {
			expect(screen.queryByText(id)).toBeNull();
		}
	});

	it('reports conflicts as the verdict, with the contested fields', async () => {
		render(Page);

		await screen.findByText('Both sides changed surname to different values');
		expect(screen.getByText(/Contested fields: surname/)).toBeDefined();
	});

	it('does not repeat a conflicted entity as a clean overlap hint', async () => {
		render(Page);

		await screen.findByRole('heading', { name: 'Also changed on both sides' });
		expect(
			screen.getByText('Every entity changed on both sides is listed as a conflict above.')
		).toBeDefined();
	});

	it('distinguishes an overlap with no conflict from a conflict', async () => {
		compareBranch.mockResolvedValue(
			comparison({ conflicts: [], overlapping_stream_ids: [PERSON_ID, OTHER_ID] })
		);

		render(Page);

		await screen.findByText(
			"No conflicts. This branch's changes are compatible with the mainline."
		);
		expect(screen.getByText(PERSON_ID)).toBeDefined();
		expect(screen.getByText(OTHER_ID)).toBeDefined();
	});

	it('discloses a truncated diff', async () => {
		compareBranch.mockResolvedValue(comparison({ has_more: true }));

		render(Page);

		expect(await screen.findByText(/hit the read cap/)).toBeDefined();
	});

	it('says nothing about truncation when the diff is complete', async () => {
		const { container } = render(Page);

		await screen.findByRole('heading', { name: 'Maternal Smith line' });
		expect(container.querySelector('.truncation')).toBeNull();
	});

	describe('merge review', () => {
		it('gates "Review & merge" on every conflict carrying a decision', async () => {
			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			const before = screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement;
			expect(before.disabled).toBe(true);
			expect(screen.getByText('1 of 1 conflict still undecided.')).toBeDefined();

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));

			const after = screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement;
			expect(after.disabled).toBe(false);
			expect(screen.getByText('All 1 conflict decided.')).toBeDefined();
		});

		it('does not offer "Review & merge" for a branch with no changes (#828)', async () => {
			compareBranch.mockResolvedValue(
				comparison({
					branch_changes: [],
					main_changes: [],
					branch_change_count: 0,
					main_change_count: 0,
					overlapping_stream_ids: [],
					conflicts: []
				})
			);
			render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			const button = screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement;
			expect(button.disabled).toBe(true);
			expect(screen.getByText('This branch has no changes to merge yet.')).toBeDefined();
		});

		it('shows what each side says for a conflict, and sends its rationale (#828)', async () => {
			compareBranch.mockResolvedValue(
				comparison({
					conflicts: [
						{
							...editEdit(PERSON_ID, 'Ada Lovelace'),
							field_values: [
								{
									field: 'surname',
									label: 'Surname',
									base_value: 'Byron',
									branch_value: 'Lovelace',
									main_value: 'King'
								}
							]
						}
					]
				})
			);
			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			const table = within(container).getByRole('table');
			expect(within(table).getByRole('rowheader', { name: 'Surname' })).toBeDefined();
			expect(within(table).getByText('Byron')).toBeDefined();
			expect(within(table).getByText('Lovelace')).toBeDefined();
			expect(within(table).getByText('King')).toBeDefined();

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.input(within(container).getByLabelText(/Why this side/), {
				target: { value: '  Marriage register, 1835  ' }
			});

			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));
			expect(await screen.findByText('Why: Marriage register, 1835')).toBeDefined();
			await fireEvent.click(await screen.findByRole('button', { name: 'Merge branch' }));

			await waitFor(() => expect(mergeBranch).toHaveBeenCalledTimes(1));
			expect(sentResolutions()).toEqual([
				{ stream_id: PERSON_ID, resolution: 'branch', rationale: 'Marriage register, 1835' }
			]);
		});

		it('decides every conflict at once from the bulk control', async () => {
			compareBranch.mockResolvedValue(
				twoEntityComparison({
					conflicts: [editEdit(PERSON_ID, 'Ada Lovelace'), editEdit(OTHER_ID, 'Grace Hopper')]
				})
			);
			render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });
			expect(screen.getByText('2 of 2 conflicts still undecided.')).toBeDefined();

			await fireEvent.click(
				screen.getByRole('button', { name: "Keep the mainline's version for all" })
			);

			expect(screen.getByText('All 2 conflicts decided.')).toBeDefined();
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));
			await fireEvent.click(await screen.findByRole('button', { name: 'Merge branch' }));
			await waitFor(() => expect(mergeBranch).toHaveBeenCalledTimes(1));
			expect(sentResolutions()).toEqual([
				{ stream_id: OTHER_ID, resolution: 'main' },
				{ stream_id: PERSON_ID, resolution: 'main' }
			]);
		});

		it('announces the undecided count to assistive tech', async () => {
			render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			expect(screen.getByRole('status').textContent).toBe('1 of 1 conflict still undecided.');
		});

		for (const status of ['merged', 'archived'] as const) {
			it(`offers no merge affordances for a ${status} branch`, async () => {
				compareBranch.mockResolvedValue(comparison({ branch: { ...branch, status } }));

				const { container } = render(Page);
				await screen.findByRole('heading', { name: 'Maternal Smith line' });

				expect(screen.queryByRole('button', { name: /merge/i })).toBeNull();
				expect(container.querySelector('[role="radiogroup"]')).toBeNull();
				expect(screen.queryByRole('checkbox')).toBeNull();
				// The read-only conflict record survives - it is the history of what
				// this terminal branch diverged on.
				expect(screen.getByText('Both sides edited')).toBeDefined();
				expect(screen.getByText('Both sides changed surname to different values')).toBeDefined();
			});
		}

		it('sends one resolution per entity, with exclusion beating the conflict decision', async () => {
			compareBranch.mockResolvedValue(
				comparison({
					branch_changes: [
						branchEntry('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', PERSON_ID, 'Ada Lovelace'),
						branchEntry('cccccccc-cccc-cccc-cccc-cccccccccccc', OTHER_ID, 'Grace Hopper')
					],
					branch_change_count: 2
				})
			);

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			// Decide the conflict for Ada in the branch's favour, then exclude her
			// anyway - the exclusion is the stronger statement and must win.
			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.click(
				screen.getByRole('checkbox', { name: 'Leave out of the merge: Ada Lovelace' })
			);
			await fireEvent.click(
				screen.getByRole('checkbox', { name: 'Leave out of the merge: Grace Hopper' })
			);

			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));
			await fireEvent.click(await screen.findByRole('button', { name: 'Merge branch' }));

			await waitFor(() => expect(mergeBranch).toHaveBeenCalledTimes(1));
			expect(mergeBranch.mock.calls[0][0]).toBe(BRANCH_ID);
			expect(sentResolutions()).toEqual([
				{ stream_id: OTHER_ID, resolution: 'main' },
				{ stream_id: PERSON_ID, resolution: 'main' }
			]);
		});

		it('toggles exclusion per entity, not per change entry', async () => {
			compareBranch.mockResolvedValue(
				comparison({
					branch_changes: [
						branchEntry('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', PERSON_ID, 'Ada Lovelace'),
						branchEntry('dddddddd-dddd-dddd-dddd-dddddddddddd', PERSON_ID, 'Ada Lovelace')
					],
					branch_change_count: 2
				})
			);

			render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			const boxes = screen.getAllByRole('checkbox', {
				name: 'Leave out of the merge: Ada Lovelace'
			});
			expect(boxes).toHaveLength(2);

			await fireEvent.click(boxes[0]);

			for (const box of boxes) {
				expect(box.getAttribute('aria-checked')).toBe('true');
			}
			// Words, not colour: both entries say so.
			expect(screen.getAllByText('Not merging')).toHaveLength(2);
		});

		it('rebuilds the pickers from a 409 merge_conflicts and drops decisions it no longer covers', async () => {
			compareBranch.mockResolvedValue(
				comparison({
					branch_changes: [
						branchEntry('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', PERSON_ID, 'Ada Lovelace'),
						branchEntry('cccccccc-cccc-cccc-cccc-cccccccccccc', OTHER_ID, 'Grace Hopper')
					],
					branch_change_count: 2,
					overlapping_stream_ids: [PERSON_ID, OTHER_ID],
					conflicts: [editEdit(PERSON_ID, 'Ada Lovelace'), editEdit(OTHER_ID, 'Grace Hopper')]
				})
			);
			// The server's own verdict: Ada is no longer contested, Marie now is.
			mergeBranch.mockRejectedValueOnce({
				status: 409,
				code: 'merge_conflicts',
				message: 'one conflict has no resolution',
				conflicts: [editEdit(OTHER_ID, 'Grace Hopper'), editEdit(THIRD_ID, 'Marie Curie')]
			});

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.click(radio(container, OTHER_ID, 'main'));
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));
			await fireEvent.click(await screen.findByRole('button', { name: 'Merge branch' }));

			await waitFor(() =>
				expect(container.querySelector(`#conflict-${THIRD_ID}-resolution-branch`)).not.toBeNull()
			);
			// Ada's compare-time conflict, and the decision made about it, are gone.
			expect(container.querySelector(`#conflict-${PERSON_ID}-resolution-branch`)).toBeNull();
			expect(screen.getByText('1 of 2 conflicts still undecided.')).toBeDefined();

			// Deciding the new conflict and merging again must not resurrect Ada's.
			await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
			await fireEvent.click(radio(container, THIRD_ID, 'branch'));
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));
			await fireEvent.click(await screen.findByRole('button', { name: 'Merge branch' }));

			await waitFor(() => expect(mergeBranch).toHaveBeenCalledTimes(2));
			expect(sentResolutions(1)).toEqual([
				{ stream_id: THIRD_ID, resolution: 'branch' },
				{ stream_id: OTHER_ID, resolution: 'main' }
			]);
		});

		it('re-issues a merge_plan_stale retry with the same resolutions', async () => {
			mergeBranch.mockRejectedValueOnce({
				status: 409,
				code: 'merge_plan_stale',
				message: 'stream 9999 moved from version 3 to 4'
			});

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));
			await fireEvent.click(await screen.findByRole('button', { name: 'Merge branch' }));

			await fireEvent.click(await screen.findByRole('button', { name: 'Try merging again' }));

			await waitFor(() => expect(mergeBranch).toHaveBeenCalledTimes(2));
			expect(sentResolutions(1)).toEqual(sentResolutions(0));
			expect(sentResolutions(1)).toEqual([{ stream_id: PERSON_ID, resolution: 'branch' }]);
		});

		it('renders the success summary and leaves the return to the mainline to the user', async () => {
			mockState.id = BRANCH_ID;
			mergeBranch.mockResolvedValue(
				mergeResult({ replayed_event_count: 12, skipped_stream_ids: [OTHER_ID] })
			);

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));
			await fireEvent.click(await screen.findByRole('button', { name: 'Merge branch' }));

			await screen.findByText('Merged Maternal Smith line into the mainline');
			expect(screen.getByText('12')).toBeDefined();
			expect(screen.getByText('128')).toBeDefined();

			// A reload would wipe the summary the instant it rendered, so the page
			// must not navigate on its own.
			expect(returnToMainline).not.toHaveBeenCalled();
			expect(switchBranch).not.toHaveBeenCalled();

			await fireEvent.click(screen.getByRole('button', { name: 'Return to mainline' }));
			expect(returnToMainline).toHaveBeenCalledTimes(1);
		});

		it('clears decisions and exclusions when the route moves to another branch', async () => {
			const SECOND_ID = '22222222-2222-2222-2222-222222222222';
			const secondBranch: Branch = { ...branch, id: SECOND_ID, name: 'Paternal Jones line' };
			compareBranch
				.mockResolvedValueOnce(comparison())
				.mockResolvedValueOnce(comparison({ branch: secondBranch }));

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.click(
				screen.getByRole('checkbox', { name: 'Leave out of the merge: Ada Lovelace' })
			);
			expect(screen.getByText('All 1 conflict decided.')).toBeDefined();

			navigateTo(SECOND_ID);
			await screen.findByRole('heading', { name: 'Paternal Jones line' });

			expect(screen.getByText('1 of 1 conflict still undecided.')).toBeDefined();
			expect(
				(screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement).disabled
			).toBe(true);
			expect(screen.queryByText('Not merging')).toBeNull();
			expect(
				screen
					.getByRole('checkbox', { name: 'Leave out of the merge: Ada Lovelace' })
					.getAttribute('aria-checked')
			).toBe('false');
		});

		// Excluding a conflicted entity *is* a resolution the server honours - the
		// request carries `main` for it. Counting only the radio would leave the
		// merge button disabled with nothing left to click that would help.
		it('counts an exclusion as deciding the conflict it covers', async () => {
			render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			expect(
				(screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement).disabled
			).toBe(true);

			await fireEvent.click(
				screen.getByRole('checkbox', { name: 'Leave out of the merge: Ada Lovelace' })
			);

			expect(screen.getByText('All 1 conflict decided.')).toBeDefined();
			expect(
				(screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement).disabled
			).toBe(false);
		});

		// The picker must read back what the request will send, never a decision
		// the exclusion overrides - and unticking must restore the original.
		it('shows the resolution the exclusion forces, and restores the original when untoggled', async () => {
			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			expect(radio(container, PERSON_ID, 'branch').getAttribute('aria-checked')).toBe('true');

			const box = screen.getByRole('checkbox', { name: 'Leave out of the merge: Ada Lovelace' });
			await fireEvent.click(box);
			expect(radio(container, PERSON_ID, 'main').getAttribute('aria-checked')).toBe('true');
			expect(radio(container, PERSON_ID, 'branch').getAttribute('aria-checked')).toBe('false');

			await fireEvent.click(box);
			expect(radio(container, PERSON_ID, 'branch').getAttribute('aria-checked')).toBe('true');
		});

		// `merging` disables this page's whole resolver. It belongs to the
		// comparison the merge was issued against, so navigating away must clear it
		// even though that merge is still in flight.
		it('does not carry an in-flight merge over to another branch', async () => {
			const SECOND_ID = '22222222-2222-2222-2222-222222222222';
			const secondBranch: Branch = { ...branch, id: SECOND_ID, name: 'Paternal Jones line' };
			compareBranch
				.mockResolvedValueOnce(comparison())
				.mockResolvedValueOnce(comparison({ branch: secondBranch }));
			// Never settles: the merge is still in flight when the route moves.
			mergeBranch.mockImplementation(() => new Promise<BranchMergeResult>(() => {}));

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));
			await fireEvent.click(await screen.findByRole('button', { name: 'Merge branch' }));
			await waitFor(() => expect(mergeBranch).toHaveBeenCalledTimes(1));

			navigateTo(SECOND_ID);
			await screen.findByRole('heading', { name: 'Paternal Jones line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			expect(
				(screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement).disabled
			).toBe(false);
			expect(
				screen
					.getByRole('checkbox', { name: 'Leave out of the merge: Ada Lovelace' })
					.getAttribute('aria-disabled')
			).not.toBe('true');
		});

		// The plan is derived here, not in the dialog - it folds exclusions over the
		// decisions, subtracts the listed ones from the branch's entity count and
		// resolves the display names. Asserted on the rendered preview so the
		// computation is covered where it is actually built.
		it('previews the plan it computed, counting a main resolution as left behind', async () => {
			compareBranch.mockResolvedValue(twoEntityComparison());

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'main'));
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));

			const dialog = await screen.findByRole('alertdialog');
			expect(within(dialog).getByText(/1 entity from this branch will be replayed/)).toBeDefined();
			expect(within(dialog).getByText(/1 entity will be left behind/)).toBeDefined();
			expect(within(leftBehindSection(dialog)).getByText('Ada Lovelace')).toBeDefined();
		});

		it('previews an entity excluded by its checkbox as left behind', async () => {
			compareBranch.mockResolvedValue(twoEntityComparison());

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.click(
				screen.getByRole('checkbox', { name: 'Leave out of the merge: Grace Hopper' })
			);
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));

			const dialog = await screen.findByRole('alertdialog');
			expect(within(dialog).getByText(/1 entity from this branch will be replayed/)).toBeDefined();
			expect(within(dialog).getByText(/1 entity will be left behind/)).toBeDefined();
			expect(within(leftBehindSection(dialog)).getByText('Grace Hopper')).toBeDefined();
			// Ada's decision is a decision, not an exclusion.
			expect(within(leftBehindSection(dialog)).queryByText('Ada Lovelace')).toBeNull();
		});

		it('leaves nothing behind when every conflict goes the branch\'s way', async () => {
			compareBranch.mockResolvedValue(twoEntityComparison());

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });

			await fireEvent.click(radio(container, PERSON_ID, 'branch'));
			await fireEvent.click(screen.getByRole('button', { name: 'Review & merge' }));

			const dialog = await screen.findByRole('alertdialog');
			expect(within(dialog).getByText(/2 entities from this branch will be replayed/)).toBeDefined();
			expect(within(dialog).getByText(/Nothing is being left behind/)).toBeDefined();
		});
	});

	it('ignores a slow response for a branch the route has already left', async () => {
		const SECOND_ID = '22222222-2222-2222-2222-222222222222';
		const secondBranch: Branch = { ...branch, id: SECOND_ID, name: 'Paternal Jones line' };

		let resolveFirst!: (value: BranchComparisonResult) => void;
		compareBranch
			.mockImplementationOnce(
				() => new Promise<BranchComparisonResult>((resolve) => (resolveFirst = resolve))
			)
			.mockImplementationOnce(async () =>
				comparison({ branch: secondBranch, conflicts: [], overlapping_stream_ids: [] })
			);

		render(Page);
		navigateTo(SECOND_ID);
		await screen.findByRole('heading', { name: 'Paternal Jones line' });

		// The first branch's response lands last. It must not repaint the page
		// with one branch's changes under the other's identity.
		resolveFirst(comparison());
		await waitFor(() => {
			expect(compareBranch).toHaveBeenCalledTimes(2);
		});
		await tick();

		expect(screen.getByRole('heading', { name: 'Paternal Jones line' })).toBeDefined();
		expect(screen.queryByRole('heading', { name: 'Maternal Smith line' })).toBeNull();
		expect(
			screen.getByText("No conflicts. This branch's changes are compatible with the mainline.")
		).toBeDefined();
	});

	// A -> B -> A: two requests for the SAME id, so a routed-id check cannot
	// separate them. Only request ordering can, which is why the loader carries
	// a monotonic token rather than comparing ids.
	it('ignores an older response for the branch it has navigated back to', async () => {
		const SECOND_ID = '22222222-2222-2222-2222-222222222222';
		const secondBranch: Branch = { ...branch, id: SECOND_ID, name: 'Paternal Jones line' };
		const staleName = 'Maternal Smith line (stale)';

		let resolveFirstA!: (value: BranchComparisonResult) => void;
		compareBranch
			// First visit to A: slow, resolves last.
			.mockImplementationOnce(
				() => new Promise<BranchComparisonResult>((resolve) => (resolveFirstA = resolve))
			)
			// Visit to B.
			.mockImplementationOnce(async () =>
				comparison({ branch: secondBranch, conflicts: [], overlapping_stream_ids: [] })
			)
			// Back to A: the response the page must keep.
			.mockImplementationOnce(async () => comparison());

		render(Page);
		navigateTo(SECOND_ID);
		await screen.findByRole('heading', { name: 'Paternal Jones line' });

		navigateTo(BRANCH_ID);
		await screen.findByRole('heading', { name: 'Maternal Smith line' });

		// The first A request finally lands, carrying older data for the same id.
		resolveFirstA(comparison({ branch: { ...branch, name: staleName } }));
		await waitFor(() => {
			expect(compareBranch).toHaveBeenCalledTimes(3);
		});
		await tick();

		expect(screen.queryByRole('heading', { name: staleName })).toBeNull();
		expect(screen.getByRole('heading', { name: 'Maternal Smith line' })).toBeDefined();
	});

	it('handles a missing branch', async () => {
		compareBranch.mockRejectedValue({ status: 404, code: 'not_found', message: 'not found' });

		render(Page);

		expect(await screen.findByText('Branch not found')).toBeDefined();
	});

	describe('merge blockers (#831)', () => {
		const FAMILY_ID = '66666666-6666-6666-6666-666666666666';
		const CITATION_ID = '55555555-5555-5555-5555-555555555555';
		const SOURCE_ID = '44444444-4444-4444-4444-444444444444';

		/** The family names the excluded Grace; the citation cites a deleted source. */
		function familyBlocker(): MergeBlocker {
			return {
				stream_id: FAMILY_ID,
				entity_type: 'family',
				entity_name: 'Grace Hopper & Howard Aiken',
				referenced_id: OTHER_ID,
				referenced_type: 'person',
				referenced_name: 'Grace Hopper',
				kind: 'missing_person',
				suggested_resolution: 'include_referenced',
				message: 'family references person'
			};
		}
		function citationBlocker(): MergeBlocker {
			return {
				stream_id: CITATION_ID,
				entity_type: 'citation',
				entity_name: '1880 Census (Birth)',
				referenced_id: SOURCE_ID,
				referenced_type: 'source',
				referenced_name: '1880 Census',
				kind: 'missing_source',
				suggested_resolution: 'leave_out',
				message: 'citation cites source'
			};
		}

		function blockedComparison() {
			return comparison({
				branch_changes: [
					branchEntry('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', OTHER_ID, 'Grace Hopper'),
					{
						...branchEntry('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', FAMILY_ID, 'Grace Hopper & Howard Aiken'),
						entity_type: 'family'
					},
					{
						...branchEntry('cccccccc-cccc-cccc-cccc-cccccccccccc', CITATION_ID, '1880 Census (Birth)'),
						entity_type: 'citation'
					}
				],
				branch_change_count: 3,
				conflicts: [],
				overlapping_stream_ids: [],
				main_changes: [],
				main_change_count: 0
			});
		}

		/** The resolutions sent with the latest precheck, sorted. */
		function lastPrecheck() {
			const calls = precheckBranchMerge.mock.calls;
			const req = calls[calls.length - 1][1] as BranchMergePrecheckRequest;
			return [...(req.resolutions ?? [])].sort((a, b) => a.stream_id.localeCompare(b.stream_id));
		}

		it('checks the decisions as they change, lists every blocker by name and holds the merge', async () => {
			compareBranch.mockResolvedValue(blockedComparison());
			precheckBranchMerge.mockImplementation(async (_id: string, req: BranchMergePrecheckRequest) => {
				const out: MergeBlocker[] = [citationBlocker()];
				if (req.resolutions?.some((r) => r.stream_id === OTHER_ID && r.resolution === 'main')) {
					out.unshift(familyBlocker());
				}
				if (req.resolutions?.some((r) => r.stream_id === CITATION_ID)) {
					out.pop();
				}
				return { blockers: out };
			});

			const { container } = render(Page);
			await screen.findByRole('heading', { name: '1 merge blocker' });
			expect(precheckBranchMerge).toHaveBeenCalledWith(BRANCH_ID, { resolutions: [] });
			expect(
				screen.getByText('Citation "1880 Census (Birth)" cites source "1880 Census", which will not exist on the mainline.')
			).toBeDefined();
			expect(screen.getByText('1 merge blocker to fix.')).toBeDefined();
			expect(
				(screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement).disabled
			).toBe(true);

			// Leaving Grace out strands the family that names her: re-checked, both listed.
			await fireEvent.click(screen.getByRole('checkbox', { name: 'Leave out of the merge: Grace Hopper' }));
			await screen.findByRole('heading', { name: '2 merge blockers' });
			expect(lastPrecheck()).toEqual([{ stream_id: OTHER_ID, resolution: 'main' }]);
			expect(
				screen.getByText('Family "Grace Hopper & Howard Aiken" names person "Grace Hopper", who will not exist on the mainline.')
			).toBeDefined();

			// Every affected row - Grace, her family, the citation - is highlighted,
			// in words as well as colour.
			const side = screen.getByTestId('branch-changes');
			const blockedRows = container.querySelectorAll('[data-testid="branch-changes"] li.blocked');
			expect(blockedRows).toHaveLength(3);
			expect(within(side).getAllByText('Merge blocker')).toHaveLength(3);

			// One-click fixes: include Grace again, and leave the citation out.
			await fireEvent.click(screen.getByRole('button', { name: 'Include Grace Hopper' }));
			await screen.findByRole('heading', { name: '1 merge blocker' });
			expect(lastPrecheck()).toEqual([]);
			await fireEvent.click(
				screen.getByRole('button', { name: 'Also leave out 1880 Census (Birth)' })
			);
			await waitFor(() =>
				expect(screen.queryByRole('heading', { name: /merge blocker/ })).toBeNull()
			);
			expect(lastPrecheck()).toEqual([{ stream_id: CITATION_ID, resolution: 'main' }]);
			expect(
				(screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement).disabled
			).toBe(false);
		});

		it('includes a person left out by a conflict decision by taking the branch side', async () => {
			compareBranch.mockResolvedValue(
				comparison({ conflicts: [editEdit(PERSON_ID, 'Ada Lovelace')] })
			);
			precheckBranchMerge.mockImplementation(async (_id: string, req: BranchMergePrecheckRequest) => ({
				blockers: req.resolutions?.some((r) => r.stream_id === PERSON_ID && r.resolution === 'main')
					? [{ ...familyBlocker(), referenced_id: PERSON_ID, referenced_name: 'Ada Lovelace' }]
					: []
			}));

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });
			await fireEvent.click(radio(container, PERSON_ID, 'main'));
			await screen.findByRole('heading', { name: '1 merge blocker' });
			// The conflict card carries the highlight too.
			expect(container.querySelector('.verdict li.conflict.blocked')).not.toBeNull();

			await fireEvent.click(screen.getByRole('button', { name: 'Include Ada Lovelace' }));
			await waitFor(() =>
				expect(screen.queryByRole('heading', { name: /merge blocker/ })).toBeNull()
			);
			expect(lastPrecheck()).toEqual([{ stream_id: PERSON_ID, resolution: 'branch' }]);
		});

		it('includes a conflicted person left out by the checkbox alone by deciding for the branch', async () => {
			compareBranch.mockResolvedValue(
				comparison({ conflicts: [editEdit(PERSON_ID, 'Ada Lovelace')] })
			);
			precheckBranchMerge.mockImplementation(async (_id: string, req: BranchMergePrecheckRequest) => ({
				blockers: req.resolutions?.some((r) => r.stream_id === PERSON_ID && r.resolution === 'main')
					? [{ ...familyBlocker(), referenced_id: PERSON_ID, referenced_name: 'Ada Lovelace' }]
					: []
			}));

			const { container } = render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });
			// No radio decision: the exclusion alone folds Ada to the mainline.
			await fireEvent.click(screen.getByRole('checkbox', { name: 'Leave out of the merge: Ada Lovelace' }));
			await screen.findByRole('heading', { name: '1 merge blocker' });
			expect(lastPrecheck()).toEqual([{ stream_id: PERSON_ID, resolution: 'main' }]);

			await fireEvent.click(screen.getByRole('button', { name: 'Include Ada Lovelace' }));
			await waitFor(() =>
				expect(screen.queryByRole('heading', { name: /merge blocker/ })).toBeNull()
			);
			// Included for real: the branch side is decided, not left undecided.
			expect(lastPrecheck()).toEqual([{ stream_id: PERSON_ID, resolution: 'branch' }]);
			expect(radio(container, PERSON_ID, 'branch').getAttribute('aria-checked')).toBe('true');
			expect(screen.getByText('All 1 conflict decided.')).toBeDefined();
			expect(
				(screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement).disabled
			).toBe(false);
		});

		it('sends one precheck for a burst of changes', async () => {
			compareBranch.mockResolvedValue(twoEntityComparison({ conflicts: [] }));

			render(Page);
			await waitFor(() => expect(precheckBranchMerge).toHaveBeenCalledTimes(1));
			await fireEvent.click(screen.getByRole('checkbox', { name: 'Leave out of the merge: Ada Lovelace' }));
			await fireEvent.click(screen.getByRole('checkbox', { name: 'Leave out of the merge: Grace Hopper' }));
			await waitFor(() => expect(precheckBranchMerge).toHaveBeenCalledTimes(2));
			await new Promise((resolve) => setTimeout(resolve, 400));
			expect(precheckBranchMerge).toHaveBeenCalledTimes(2);
			expect(lastPrecheck()).toEqual([
				{ stream_id: PERSON_ID, resolution: 'main' },
				{ stream_id: OTHER_ID, resolution: 'main' }
			].sort((a, b) => a.stream_id.localeCompare(b.stream_id)));
		});

		it('shows the blockers a dangling-reference refusal carries', async () => {
			compareBranch.mockResolvedValue(twoEntityComparison({ conflicts: [] }));
			mergeBranch.mockRejectedValue({
				code: 'merge_dangling_reference',
				message: 'merge would leave a reference pointing at an entity main does not have: ...',
				blockers: [citationBlocker()],
				status: 409
			});

			render(Page);
			const reviewAndMerge = await screen.findByRole('button', { name: 'Review & merge' });
			await waitFor(() => expect((reviewAndMerge as HTMLButtonElement).disabled).toBe(false));
			await fireEvent.click(reviewAndMerge);
			const dialog = await screen.findByRole('alertdialog');
			await fireEvent.click(within(dialog).getByRole('button', { name: 'Merge branch' }));

			const list = await within(dialog).findByRole('list', { name: 'Merge blockers' });
			expect(within(list).getByText(/Citation "1880 Census \(Birth\)" cites source/)).toBeDefined();
			// The panel behind the dialog adopts the merge's own verdict.
			await screen.findByRole('heading', { name: '1 merge blocker' });
		});

		it('does not hold the merge when the check itself fails', async () => {
			compareBranch.mockResolvedValue(twoEntityComparison({ conflicts: [] }));
			precheckBranchMerge.mockRejectedValue({ code: 'internal_error', message: 'boom' });

			render(Page);
			await screen.findByText(/The merge blockers could not be checked \(boom\)/);
			expect(
				(screen.getByRole('button', { name: 'Review & merge' }) as HTMLButtonElement).disabled
			).toBe(false);
		});

		it('does not check a branch that cannot be merged', async () => {
			compareBranch.mockResolvedValue(
				comparison({ branch: { ...branch, status: 'merged' }, conflicts: [] })
			);
			render(Page);
			await screen.findByRole('heading', { name: 'Maternal Smith line' });
			// Past the precheck's debounce, so a scheduled check would have run.
			await new Promise((resolve) => setTimeout(resolve, 400));
			expect(precheckBranchMerge).not.toHaveBeenCalled();
		});
	});
});
