<script lang="ts">
	/**
	 * Compare a research branch against the mainline, and merge it.
	 *
	 * The single `compareBranch` call behind this page returns the diff and the
	 * conflict verdict together, so the merge review lives inline here rather
	 * than on a route of its own. What the comparison means:
	 *
	 * - `overlapping_stream_ids` is a *hint*: entities both sides touched. An
	 *   entity can overlap without conflicting.
	 * - `conflicts` is the compare-time *verdict*: the changes that are actually
	 *   incompatible. It is **advisory only** - `POST /branches/{id}/merge`
	 *   re-runs detection itself and ignores whatever compare said, so a merge
	 *   can come back with a different list (see `handleRefused`).
	 * - `has_more` means one side hit the read cap, so what is rendered below is
	 *   only part of the diff.
	 *
	 * Merge affordances appear only while the branch is `active`. A `merged` or
	 * `archived` branch accepts no further writes, so it keeps the read-only
	 * conflict rendering below.
	 *
	 * Merge blockers (#831): the decisions and exclusions below are re-checked
	 * with `POST /branches/{id}/merge/precheck` whenever they change, and every
	 * cross-entity reference the merge would break is listed by name, with a
	 * one-click fix, and highlighted on the rows involved. "Review & merge" is
	 * held while any remain.
	 *
	 * Merge record (#832): a merged branch shows what its merge decided, read
	 * from the merge's own record (`merge_record`) rather than a verdict
	 * recomputed against a mainline the merge itself changed - the note, when,
	 * the counts, each decision by name, and what was left behind. The
	 * mainline column leaves out the merge's copies of this branch's changes
	 * (`replayed_change_count` says how many).
	 *
	 * Review checks (#838): while an active branch has changes, two soft,
	 * non-blocking panels follow the blockers - the facts and relationships it
	 * changed without evidence on the branch, and the validation issues, quality
	 * issues and possible duplicates it introduces over the mainline. Each loads
	 * its own data and never holds the merge.
	 */
	import { page } from '$app/stores';
	import {
		api,
		type ApiError,
		type Branch,
		type BranchChangeEntry,
		type BranchComparisonResult,
		type BranchMergeRefusal,
		type BranchMergeResult,
		type BranchMergeResumeRequest,
		type BranchMergeResumeResult,
		type MergeBlocker,
		type MergeConflict,
		type MergeRecord,
		type MergeRecordDecision,
		type MergeResolution
	} from '$lib/api/client';
	import {
		activeBranch,
		refreshActiveBranch,
		returnToMainline,
		switchBranch
	} from '$lib/stores/activeBranch.svelte';
	import ConflictValues from '$lib/components/ConflictValues.svelte';
	import BranchResearchSummary from '$lib/components/branch/BranchResearchSummary.svelte';
	import BranchResearchEditor from '$lib/components/branch/BranchResearchEditor.svelte';
	import CloseBranchDialog, {
		type BranchCloseResult
	} from '$lib/components/branch/CloseBranchDialog.svelte';
	import {
		OUTCOME_LABELS,
		branchOutcome,
		branchSubjectCandidates,
		promotionSummary
	} from '$lib/utils/branchResearch';
	import DiffView from '$lib/components/DiffView.svelte';
	import FinishMergeDialog from '$lib/components/FinishMergeDialog.svelte';
	import IncompleteMergeCallout from '$lib/components/IncompleteMergeCallout.svelte';
	import MergeBlockersPanel from '$lib/components/MergeBlockersPanel.svelte';
	import BranchEvidenceCoveragePanel from '$lib/components/BranchEvidenceCoveragePanel.svelte';
	import BranchHealthPanel from '$lib/components/BranchHealthPanel.svelte';
	import MergeConflictResolver from '$lib/components/MergeConflictResolver.svelte';
	import MergeEffectLink from '$lib/components/MergeEffectLink.svelte';
	import MergeConfirmDialog, {
		type MergeOptions,
		type MergePlan,
		type MergePlanEntity
	} from '$lib/components/MergeConfirmDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import {
		changeEntryLink,
		entityTypeLabel,
		unnamedEntityLabel
	} from '$lib/utils/changeEntries';
	import { blockedEntityIds, blockerFix } from '$lib/utils/mergeBlockers';
	import { isIncompleteMerge, isUnreadableMerge, pendingEntities } from '$lib/utils/mergeState';

	let comparison: BranchComparisonResult | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let notFound = $state(false);

	/**
	 * The conflict list a `409 merge_conflicts` refusal carried, which supersedes
	 * the comparison's advisory one. Null until the server has re-verdicted.
	 */
	let serverConflicts: MergeConflict[] | null = $state(null);
	/**
	 * The decision taken for each conflicting entity, keyed by `stream_id`.
	 *
	 * Reassigned, never mutated: a plain `Map` is not a `SvelteMap`, so an
	 * in-place `.set()` would leave every reader of this state unrepainted.
	 */
	let resolutions: Map<string, MergeResolution> = $state(new Map());
	/**
	 * Optional reasoning per conflict decision, keyed by `stream_id` (#828).
	 * Reassigned, never mutated, for the same reason as `resolutions`.
	 */
	let rationales: Map<string, string> = $state(new Map());
	/**
	 * Entities the user opted out of the merge, by `stream_id`. Excluding is
	 * keyed by *entity*, not by change entry: one entity can appear in several
	 * entries and all of them must show and toggle the same decision.
	 */
	let excluded: Set<string> = $state(new Set());
	let merging = $state(false);
	let confirmOpen = $state(false);
	/** The finish-merge dialog for an interrupted merge (#830). */
	let finishOpen = $state(false);
	/** Set once a finish succeeded, so closing the dialog refreshes the page. */
	let finished = $state(false);

	/**
	 * What the last precheck (or a `409 merge_dangling_reference`) said the
	 * current decisions would break. Replaced wholesale on every check.
	 */
	let blockers: MergeBlocker[] = $state([]);
	let checkingBlockers = $state(false);
	let blockerCheckError: string | null = $state(null);
	/** Orders precheck responses: only the latest request may write. */
	let precheckRequest = 0;
	/** How long the decisions must hold still before they are prechecked. */
	const PRECHECK_DEBOUNCE_MS = 250;
	const blockedIds = $derived(blockedEntityIds(blockers));
	/** The research-record editor (#835) is open. */
	let editingResearch = $state(false);
	/** The close dialog (#836) is open for this branch. */
	let closeOpen = $state(false);
	/** What the close did, until the page loads another branch. */
	let closeNotice: { text: string; warning: string | null } | null = $state(null);

	// `?? ''` so the id is a plain string everywhere below; the `$effect` already
	// treats an absent id as "nothing to load", and empty is absent.
	const branchId = $derived($page.params.id ?? '');
	const conflicts: MergeConflict[] = $derived.by(
		() => serverConflicts ?? comparison?.conflicts ?? []
	);
	const conflictedStreamIds = $derived.by(() => new Set(conflicts.map((c) => c.stream_id)));
	/**
	 * Overlaps not listed above - the interesting part of the hint. Normally
	 * that is the overlaps the conflict detector cleared. A merged branch shows
	 * its merge record instead of the recomputed conflicts, so there only the
	 * record's decisions are taken out: an entity the mainline changed again
	 * after the merge can be a recomputed conflict the record never decided,
	 * and it must still be listed somewhere.
	 */
	const cleanOverlaps = $derived.by(() => {
		const listedAbove = mergeRecord
			? new Set(mergeRecord.decisions.map((d) => d.stream_id))
			: conflictedStreamIds;
		return (comparison?.overlapping_stream_ids ?? []).filter((id) => !listedAbove.has(id));
	});

	/**
	 * Only an active branch can be merged; terminal ones are a read-only record.
	 *
	 * `$derived.by` rather than `$derived`: read inline, TypeScript narrows
	 * `comparison` back to its `null` initialiser and `.branch` collapses.
	 */
	const mergeable = $derived.by(() => comparison?.branch.status === 'active');

	/** A merged branch's record of its merge (#832); null for any other branch. */
	const mergeRecord: MergeRecord | null = $derived.by(() => comparison?.merge_record ?? null);

	/** How many of the mainline's changes are this branch's own, copied by its merge. */
	const replayedCopies = $derived.by(() => comparison?.replayed_change_count ?? 0);

	/**
	 * Names for the "changed on both sides" hint, from the entries that list
	 * each entity, so the hint reads as names rather than ids.
	 */
	const overlapNames: Map<string, { type: string; name: string }> = $derived.by(() => {
		const names = new Map<string, { type: string; name: string }>();
		for (const entry of [...(comparison?.branch_changes ?? []), ...(comparison?.main_changes ?? [])]) {
			const known = names.get(entry.entity_id);
			if (!known || (!known.name && entry.entity_name)) {
				names.set(entry.entity_id, { type: entry.entity_type, name: entry.entity_name ?? '' });
			}
		}
		return names;
	});

	/**
	 * A branch with no changes of its own has nothing to promote, and the server
	 * refuses to merge it (`409 merge_empty`) rather than record a merge that
	 * did nothing (#828) - so the button is not offered as if it could.
	 */
	const hasChanges = $derived.by(() => (comparison?.branch_change_count ?? 0) > 0);

	/**
	 * The research record can be edited while the branch is active, and a merged
	 * branch can still record its outcome. An archived branch is a closed record.
	 */
	const researchEditable = $derived.by(() => {
		const status = comparison?.branch.status;
		return status === 'active' || status === 'merged';
	});

	/** Persons and families this branch changed, offered as research subjects. */
	const subjectCandidates = $derived.by(() => branchSubjectCandidates(comparison?.branch_changes ?? []));

	/**
	 * Subject and proof summary pages read the active scope. For an active
	 * branch that is not the active one, say so: what the links open is not
	 * this branch's view.
	 */
	const researchScopeNote = $derived.by(() => {
		const shown = comparison?.branch;
		if (!shown || shown.status !== 'active' || activeBranch.id === shown.id) return null;
		const where = activeBranch.id ? 'the active branch' : 'the mainline';
		return `These links open in ${where}. Switch to this branch to see them as it has them.`;
	});

	/**
	 * Adopt the branch as the edit left it. The comparison's diff is unchanged -
	 * a research edit is metadata, never a change to the tree - so only the
	 * branch record is swapped, and the banner follows when this is the active
	 * branch.
	 */
	function handleResearchSaved(saved: Branch) {
		if (comparison && comparison.branch.id === saved.id) {
			comparison = { ...comparison, branch: saved };
		}
		refreshActiveBranch(saved);
		editingResearch = false;
	}

	/**
	 * Adopt the branch as the close left it (#836). A closed branch has no
	 * view of its own any more, so standing on it would leave every scoped
	 * read pointing at purged rows: return to the mainline (which reloads).
	 */
	function handleClosed(result: BranchCloseResult) {
		closeOpen = false;
		if (comparison && comparison.branch.id === result.branch.id) {
			comparison = { ...comparison, branch: result.branch };
		}
		const outcome = OUTCOME_LABELS[branchOutcome(result.branch.outcome)].toLowerCase();
		const copied = result.promotion ? ` ${promotionSummary(result.promotion)}` : '';
		closeNotice = { text: `Closed as ${outcome}.${copied}`, warning: result.promotionError };
		if (activeBranch.id === result.branch.id) {
			switchBranch(null);
		}
	}

	/**
	 * Every entity this branch changed, first entry wins for the name. The merge
	 * request may only name entities from here (plus the server's own conflicts):
	 * an id the branch never touched is a `400`, not a no-op.
	 */
	const branchEntities: Map<string, MergePlanEntity> = $derived.by(() => {
		const seen = new Map<string, MergePlanEntity>();
		for (const entry of comparison?.branch_changes ?? []) {
			if (!seen.has(entry.entity_id)) {
				seen.set(entry.entity_id, {
					streamId: entry.entity_id,
					entityType: entry.entity_type,
					entityName: entry.entity_name ?? ''
				});
			}
		}
		return seen;
	});

	/**
	 * Exactly what goes on the wire: conflict decisions and exclusions folded
	 * into one entry per `stream_id`.
	 *
	 * Exclusion is applied last and wins outright, because "leave this entity
	 * behind" *is* a `main` resolution - the only partial-merge shape the API
	 * supports (ADR-005, Merge). Folding them into one map is what stops a
	 * conflicted-and-excluded entity being sent twice, which would be the client
	 * contradicting itself.
	 */
	const mergeResolutions: Map<string, MergeResolution> = $derived.by(() => {
		const folded = new Map<string, MergeResolution>();
		for (const conflict of conflicts) {
			const decision = resolutions.get(conflict.stream_id);
			if (decision) folded.set(conflict.stream_id, decision);
		}
		for (const streamId of excluded) folded.set(streamId, 'main');
		return folded;
	});

	/**
	 * Counted against `mergeResolutions`, not the raw `resolutions` map, because
	 * that is what actually goes on the wire. Ticking "leave out of the merge" on
	 * a conflicted entity *is* a decision the server will honour - reading the raw
	 * map would call it undecided and leave "Review & merge" disabled with no way
	 * out but the radio the user has already made moot.
	 */
	const undecidedCount = $derived(
		conflicts.filter((conflict) => !mergeResolutions.has(conflict.stream_id)).length
	);
	const blockerLabel = $derived(
		blockers.length === 0
			? ''
			: `${blockers.length} merge blocker${blockers.length === 1 ? '' : 's'} to fix.`
	);
	const undecidedLabel = $derived.by(() => {
		if (!hasChanges) return 'This branch has no changes to merge yet.';
		if (conflicts.length === 0) return 'No conflicts to resolve.';
		const total = `${conflicts.length} conflict${conflicts.length === 1 ? '' : 's'}`;
		return undecidedCount === 0
			? `All ${total} decided.`
			: `${undecidedCount} of ${total} still undecided.`;
	});

	const mergePlan: MergePlan = $derived.by(() => {
		const leftBehind = [...mergeResolutions]
			.filter(([, resolution]) => resolution === 'main')
			.map(([streamId]) => planEntity(streamId));
		// Only entities this comparison actually listed can be counted, so a
		// truncated comparison undercounts - which is precisely what `hasMore`
		// discloses in the dialog.
		const listedExclusions = leftBehind.filter((e) => branchEntities.has(e.streamId)).length;
		return {
			mergingCount: branchEntities.size - listedExclusions,
			excluded: leftBehind,
			decisions: conflicts.flatMap((conflict) => {
				const resolution = mergeResolutions.get(conflict.stream_id);
				const rationale = rationales.get(conflict.stream_id)?.trim();
				return resolution ? [{ conflict, resolution, ...(rationale ? { rationale } : {}) }] : [];
			}),
			hasMore: comparison?.has_more ?? false
		};
	});

	/** Names an entity for the plan, falling back to the conflict's own labels. */
	function planEntity(streamId: string): MergePlanEntity {
		const known = branchEntities.get(streamId);
		if (known) return known;
		const conflict = conflicts.find((c) => c.stream_id === streamId);
		return {
			streamId,
			entityType: conflict?.entity_type ?? 'entity',
			entityName: conflict?.entity_name ?? ''
		};
	}

	function resolveConflict(streamId: string, resolution: MergeResolution) {
		resolutions = new Map(resolutions).set(streamId, resolution);
	}

	/** A bulk decision from the resolver: one reassignment for all of them. */
	function resolveConflicts(decisions: Array<[string, MergeResolution]>) {
		resolutions = new Map([...resolutions, ...decisions]);
	}

	function setRationale(streamId: string, rationale: string) {
		rationales = new Map(rationales).set(streamId, rationale);
	}

	function toggleExclusion(streamId: string) {
		const next = new Set(excluded);
		if (!next.delete(streamId)) next.add(streamId);
		excluded = next;
	}

	/**
	 * Applies a blocker's suggested fix. "Leave out" excludes the entity at
	 * fault; "include" undoes whatever left the referenced entity out - its
	 * exclusion - and, for a conflicted entity, records the branch decision
	 * that brings it along, whatever was decided before. The precheck effect
	 * then re-checks, since a fix can surface another blocker.
	 */
	function applyBlockerFix(blocker: MergeBlocker) {
		const { streamId, resolution } = blockerFix(blocker);
		const next = new Set(excluded);
		if (resolution === 'main') {
			next.add(streamId);
		} else {
			next.delete(streamId);
			// A conflicted entity is only included once it is decided for the
			// branch. It may have been left out by a mainline decision or by its
			// checkbox alone, with no decision at all; either way the fix records
			// "branch", or the conflict would go back to needing a decision.
			if (conflictedStreamIds.has(streamId) && resolutions.get(streamId) !== 'branch') {
				resolveConflict(streamId, 'branch');
			}
		}
		excluded = next;
	}

	/**
	 * Re-checks the merge blockers whenever the decisions that would be sent
	 * change. Only an active branch with changes can be merged, so only that is
	 * checked. Writes nothing on the server; a failed check is shown but does
	 * not hold the merge, which checks for itself.
	 */
	async function checkBlockers(id: string, entries: Array<[string, MergeResolution]>) {
		const request = ++precheckRequest;
		checkingBlockers = true;
		try {
			const result = await api.precheckBranchMerge(id, {
				resolutions: entries.map(([stream_id, resolution]) => ({ stream_id, resolution }))
			});
			if (request !== precheckRequest) return;
			blockers = result.blockers ?? [];
			blockerCheckError = null;
		} catch (e) {
			if (request !== precheckRequest) return;
			blockers = [];
			blockerCheckError = (e as ApiError)?.message || 'the check failed';
		} finally {
			if (request === precheckRequest) checkingBlockers = false;
		}
	}

	/**
	 * Debounced, so a burst of clicks (ticking several exclusions, a bulk
	 * decision) sends one precheck for the decisions it settles on rather
	 * than one per click. A pending check is dropped when the decisions change
	 * again or the page goes away.
	 */
	$effect(() => {
		const entries = [...mergeResolutions];
		const id = comparison?.branch.id;
		if (!id || !mergeable || !hasChanges) return;
		const timer = setTimeout(() => checkBlockers(id, entries), PRECHECK_DEBOUNCE_MS);
		return () => clearTimeout(timer);
	});

	function formatTimestamp(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	/** Every branch-aware entity links to its page, or to the page that presents it. */
	function entityLink(entry: BranchChangeEntry): string | null {
		return changeEntryLink(entry);
	}

	/** Which side a recorded decision kept, in words. */
	function decisionLabel(decision: MergeRecordDecision): string {
		return decision.resolution === 'branch'
			? "Kept this branch's version"
			: "Kept the mainline's version";
	}

	function plural(count: number, one: string, many = `${one}s`): string {
		return `${count} ${count === 1 ? one : many}`;
	}

	function conflictLabel(kind: MergeConflict['kind']): string {
		switch (kind) {
			case 'edit_edit':
				return 'Both sides edited';
			case 'delete_edit':
				return 'Deleted on one side';
			case 'create_create':
				return 'Created on both sides';
			default:
				return kind;
		}
	}

	/**
	 * A soft navigation between two `/branches/{id}` entries reuses this
	 * component rather than recreating it, so a slow first response can land
	 * after the second branch's. Every assignment below is therefore gated on
	 * the requested id still being the routed one — otherwise the page would
	 * show one branch's changes and conflicts under another's name.
	 */
	// Comparing the routed id is not enough on its own: A -> B -> A navigation
	// issues two requests for the SAME id, and the first can land after the
	// second and overwrite newer data with older. A monotonic token is what
	// actually orders them, so every state write is gated on it — the same
	// pattern BranchSwitcher uses for its list fetches.
	let comparisonRequest = 0;

	async function loadComparison(id: string) {
		const request = ++comparisonRequest;
		loading = true;
		error = null;
		notFound = false;
		// Decisions belong to the comparison they were made against. Clearing them
		// here, under the same token that gates every other write, is what stops a
		// late response from repopulating one branch's conflicts while another
		// branch's decisions are still held.
		serverConflicts = null;
		resolutions = new Map();
		rationales = new Map();
		excluded = new Set();
		confirmOpen = false;
		finishOpen = false;
		finished = false;
		// Blockers describe one comparison's decisions, like the decisions do;
		// bumping the token drops any check still in flight for the old one.
		precheckRequest++;
		blockers = [];
		checkingBlockers = false;
		blockerCheckError = null;
		editingResearch = false;
		closeOpen = false;
		closeNotice = null;
		// `merging` is per-comparison too: it disables this page's resolver,
		// exclusion checkboxes and merge button, and a merge issued for the branch
		// we just navigated away from must not disable the new one's. It is cleared
		// here *as well as* in `performMerge`'s `finally` - not moved. The `finally`
		// stays ungated on the token on purpose: gating it would leave `merging`
		// stuck true forever whenever a navigation raced the merge, which is worse
		// than the bug being fixed.
		merging = false;
		try {
			const result = await api.compareBranch(id);
			if (request !== comparisonRequest) return;
			comparison = result;
		} catch (e) {
			if (request !== comparisonRequest) return;
			const apiError = e as ApiError;
			if (apiError.status === 404) {
				notFound = true;
			} else {
				error = apiError.message || 'Failed to load branch comparison';
			}
			comparison = null;
		} finally {
			if (request === comparisonRequest) {
				loading = false;
			}
		}
	}

	$effect(() => {
		const id = branchId;
		if (id) {
			loadComparison(id);
		}
	});

	/**
	 * The comparison token the in-flight merge was issued under, so an outcome
	 * that lands after a soft navigation cannot rewrite the new branch's state.
	 */
	let mergeComparisonRequest = 0;

	/**
	 * Issues the merge. Resolves with the result and *throws* the refusal, which
	 * is the contract `MergeConfirmDialog` renders its outcome from.
	 */
	async function performMerge(note: string, options?: MergeOptions): Promise<BranchMergeResult> {
		const request = (mergeComparisonRequest = comparisonRequest);
		const id = branchId;
		merging = true;
		try {
			const result = await api.mergeBranch(id, {
				...(note ? { note } : {}),
				...(options?.snapshotBefore ? { snapshot_before: true } : {}),
				resolutions: [...mergeResolutions].map(([stream_id, resolution]) => {
					// Only a conflict decision carries reasoning; an exclusion is its own.
					const rationale = conflictedStreamIds.has(stream_id)
						? rationales.get(stream_id)?.trim()
						: undefined;
					return { stream_id, resolution, ...(rationale ? { rationale } : {}) };
				})
			});
			// Adopt the branch as the merge left it, so the page stops offering
			// merge affordances behind the still-open success summary. Gated on the
			// token like every other write: the user may have navigated away while
			// the merge was in flight.
			if (request === comparisonRequest && comparison) {
				comparison = { ...comparison, branch: result.branch };
			}
			return result;
		} finally {
			merging = false;
		}
	}

	/**
	 * Finishes an interrupted merge (#830). Resolves with the result and throws
	 * the refusal, the contract `FinishMergeDialog` renders its outcome from.
	 */
	async function performResume(request: BranchMergeResumeRequest): Promise<BranchMergeResumeResult> {
		const token = comparisonRequest;
		const result = await api.resumeBranchMerge(branchId, request);
		if (token === comparisonRequest && comparison) {
			// The result's branch carries merge_state: complete, so the flag and
			// the action go away behind the still-open summary.
			comparison = { ...comparison, branch: result.branch };
			finished = true;
		}
		return result;
	}

	function closeFinish() {
		finishOpen = false;
		// The merge record gained the resume's decisions; read it again.
		if (finished) loadComparison(branchId);
	}

	/**
	 * The merge dialog's "Finish merge" after a partial failure: the branch is
	 * merged now, so read it again for what is pending, then open the flow.
	 * It opens for any merged branch, not only one that reads as unfinished:
	 * the resume is safe to run on a finished merge (it replays nothing and
	 * repairs only what is behind), so the button the user pressed always
	 * leads somewhere, even if another tab finished the merge meanwhile.
	 */
	async function startFinishMerge() {
		await loadComparison(branchId);
		if (comparison?.branch.status === 'merged') finishOpen = true;
	}

	/**
	 * The merge endpoint's verdict overrides the comparison's. On
	 * `merge_conflicts` it carries the *whole* conflict list as the server now
	 * sees it, so the pickers are rebuilt from that and every decision the server
	 * no longer reports a conflict for is dropped - carrying one forward would
	 * silently re-send a choice made about a conflict that no longer exists.
	 */
	function handleRefused(refusal: BranchMergeRefusal) {
		if (mergeComparisonRequest !== comparisonRequest) return;
		if (refusal.code === 'merge_dangling_reference' && refusal.blockers) {
			// The merge's own verdict, which the panel shows until the next change.
			precheckRequest++;
			checkingBlockers = false;
			blockers = refusal.blockers;
			return;
		}
		if (refusal.code !== 'merge_conflicts') return;
		const fresh = refusal.conflicts ?? [];
		serverConflicts = fresh;
		const live = new Set(fresh.map((conflict) => conflict.stream_id));
		resolutions = new Map([...resolutions].filter(([streamId]) => live.has(streamId)));
		rationales = new Map([...rationales].filter(([streamId]) => live.has(streamId)));
	}
</script>

<svelte:head>
	<title>{comparison ? `${comparison.branch.name} | Branches` : 'Branch Comparison'} | My Family</title>
</svelte:head>

{#snippet changeList(entries: BranchChangeEntry[], emptyText: string, excludable: boolean)}
	{#if entries.length === 0}
		<p class="side-empty">{emptyText}</p>
	{:else}
		<ol class="change-list">
			{#each entries as entry (entry.id)}
				{@const link = entityLink(entry)}
				<!-- Branch side only: the mainline's own changes are never "being merged",
				     so marking them left behind would be meaningless. -->
				{@const isExcluded = excludable && excluded.has(entry.entity_id)}
				{@const isBlocked = excludable && mergeable && blockedIds.has(entry.entity_id)}
				<li
					class="change-entry"
					class:contested={conflictedStreamIds.has(entry.entity_id)}
					class:excluded={isExcluded}
					class:blocked={isBlocked}
				>
					<div class="change-head">
						<span class="change-time">{formatTimestamp(entry.timestamp)}</span>
						<Badge
							variant={entry.action === 'deleted' ? 'destructive' : 'secondary'}
							class="capitalize"
						>
							{entry.action}
						</Badge>
						{#if isExcluded}
							<!-- Words, not just the dashed border: the state must not depend on sight. -->
							<Badge variant="outline">Not merging</Badge>
						{/if}
						{#if isBlocked}
							<Badge variant="outline" class="border-orange-400 text-orange-800">Merge blocker</Badge>
						{/if}
					</div>
					<div class="change-body">
						<span class="entity-type">{entityTypeLabel(entry.entity_type)}</span>
						{#if link}
							<a href={link} class="entity-name"
								>{entry.entity_name || unnamedEntityLabel(entry.entity_type)}</a
							>
						{:else}
							<span class="entity-name" class:deleted={entry.action === 'deleted'}
								>{entry.entity_name || unnamedEntityLabel(entry.entity_type)}</span
							>
						{/if}
					</div>
					{#if entry.changes && Object.keys(entry.changes).length > 0}
						<div class="change-diff">
							<DiffView changes={entry.changes} />
						</div>
					{/if}
					{#if excludable && mergeable}
						<!--
							Keyed by `entity_id`, so an entity changed several times on this
							branch toggles and reads back as one decision across all of its
							entries. The visible text opens the accessible name so the two
							cannot disagree (WCAG 2.5.3).
						-->
						<div class="exclude-row">
							<Checkbox
								checked={isExcluded}
								onCheckedChange={() => toggleExclusion(entry.entity_id)}
								disabled={merging}
								aria-label="Leave out of the merge: {entry.entity_name ||
									unnamedEntityLabel(entry.entity_type)}"
							/>
							<span class="exclude-text">
								Leave out of the merge: <span class="exclude-name"
									>{entry.entity_name || unnamedEntityLabel(entry.entity_type)}</span
								>
							</span>
						</div>
					{/if}
				</li>
			{/each}
		</ol>
	{/if}
{/snippet}

{#snippet mergeRecordSection(record: MergeRecord)}
	<section class="merge-record" aria-labelledby="merge-record-heading" data-testid="merge-record">
		<h2 id="merge-record-heading">Merge record</h2>
		<p class="section-hint">
			What was decided when this branch was merged, as it was recorded then.
			The branch accepts no further changes.
		</p>
		<dl class="record-facts">
			<div>
				<dt>Merged</dt>
				<dd>{formatTimestamp(record.merged_at)}</dd>
			</div>
			<div>
				<dt>Note</dt>
				<dd>
					{#if record.note}
						<span class="record-note">{record.note}</span>
					{:else}
						<span class="muted">No merge note was recorded.</span>
					{/if}
				</dd>
			</div>
			<div>
				<dt>Promoted</dt>
				<dd>
					{#if record.replayed_event_count !== undefined}
						{plural(record.replayed_event_count, 'change')} replayed onto the mainline
					{:else}
						{plural(replayedCopies, 'change')} found on the mainline
					{/if}
				</dd>
			</div>
			<div>
				<dt>Left behind</dt>
				<dd>{plural(record.skipped_stream_ids.length, 'entity', 'entities')}</dd>
			</div>
			{#if record.resume_count > 0}
				<div>
					<dt>Resumed</dt>
					<dd>
						The merge was interrupted and finished later ({plural(record.resume_count, 'resume')}
						recorded decisions).
					</dd>
				</div>
			{/if}
		</dl>
		<MergeEffectLink {record} onBranch={activeBranch.id !== null} />
		{#if !record.recorded}
			<p class="record-legacy" role="note">
				This merge was made before decisions were recorded. What it left behind is known from
				its plan, but not which of those were conflicts or why.
			</p>
		{/if}

		<h3>Decisions</h3>
		{#if record.decisions.length === 0}
			<p class="muted">No conflicts needed a decision.</p>
		{:else}
			<ul class="record-list">
				{#each record.decisions as decision (decision.stream_id)}
					<li class="record-item">
						<div class="conflict-head">
							<span class="entity-type">{entityTypeLabel(decision.entity_type)}</span>
							<span class="conflict-name"
								>{decision.entity_name || unnamedEntityLabel(decision.entity_type)}</span
							>
							{#if decision.kind}
								<Badge variant="secondary">{conflictLabel(decision.kind)}</Badge>
							{/if}
							<Badge variant={decision.resolution === 'branch' ? 'default' : 'outline'}
								>{decisionLabel(decision)}</Badge
							>
						</div>
						{#if decision.fields && decision.fields.length > 0}
							<p class="conflict-fields record-fields">
								Contested fields: {decision.fields.join(', ')}
							</p>
						{/if}
						{#if decision.rationale}
							<p class="record-rationale">Why: {decision.rationale}</p>
						{/if}
						{#if decision.decided_at === 'resume'}
							<p class="muted record-when">Decided when the interrupted merge was resumed.</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}

		<h3>Left behind</h3>
		{#if record.exclusions.length === 0}
			<p class="muted">Nothing else was left out of the merge.</p>
		{:else}
			<ul class="record-list">
				{#each record.exclusions as exclusion (exclusion.stream_id)}
					<li class="record-item">
						<div class="conflict-head">
							<span class="entity-type">{entityTypeLabel(exclusion.entity_type)}</span>
							<span class="conflict-name"
								>{exclusion.entity_name || unnamedEntityLabel(exclusion.entity_type)}</span
							>
							<Badge variant="outline">Not merged</Badge>
						</div>
						{#if exclusion.rationale}
							<p class="record-rationale">Why: {exclusion.rationale}</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/snippet}

<div class="compare-page">
	<a href="/branches" class="back-link">&larr; All branches</a>

	{#if loading}
		<div class="state" role="status" aria-live="polite">Loading comparison...</div>
	{:else if notFound}
		<div class="state empty">
			<h2>Branch not found</h2>
			<p>It may have been deleted, or the branch registry is not configured on this server.</p>
		</div>
	{:else if error}
		<div class="state error" role="alert">{error}</div>
	{:else if comparison}
		<header class="page-header">
			<div>
				<div class="title-row">
					<h1>{comparison.branch.name}</h1>
					<Badge variant={comparison.branch.status === 'active' ? 'default' : 'secondary'} class="capitalize">
						{comparison.branch.status === 'archived' ? 'closed' : comparison.branch.status}
					</Badge>
					{#if isIncompleteMerge(comparison.branch)}
						<Badge variant="outline" class="border-orange-500 text-orange-800">Merge unfinished</Badge>
					{/if}
				</div>
				{#if comparison.branch.status === 'archived'}
					<div class="closed-record" data-testid="closed-record">
						<p>
							Closed{comparison.branch.closed_at
								? ` ${formatTimestamp(comparison.branch.closed_at)}`
								: ''} without merging.
							{#if comparison.branch.close_reason}
								<span class="closed-reason">{comparison.branch.close_reason}</span>
							{/if}
						</p>
						<a href="/branches/{comparison.branch.id}/research" class="research-link">
							View this branch's research
						</a>
					</div>
				{/if}
				{#if closeNotice}
					<div class="close-notice" role="status">
						<p>{closeNotice.text}</p>
						{#if closeNotice.warning}
							<p class="close-warning">
								{closeNotice.warning} You can copy them from the branch's research page.
							</p>
						{/if}
					</div>
				{/if}
				{#if comparison.branch.description}
					<p class="description">{comparison.branch.description}</p>
				{/if}
				{#if comparison.branch.status === 'merged' && comparison.branch.merged_at}
					<p class="merged-line" data-testid="merged-at">
						Merged into the mainline {formatTimestamp(comparison.branch.merged_at)}{comparison
							.branch.merge_note
							? ':'
							: '.'}
						{#if comparison.branch.merge_note}
							<span class="merged-note">{comparison.branch.merge_note}</span>
						{/if}
					</p>
				{/if}
				<p class="anchor">Compared against the mainline from position {comparison.base_position}.</p>
				{#if editingResearch}
					<BranchResearchEditor
						branch={comparison.branch}
						candidates={subjectCandidates}
						onsaved={handleResearchSaved}
						oncancel={() => (editingResearch = false)}
					/>
				{:else}
					<BranchResearchSummary branch={comparison.branch} scopeNote={researchScopeNote} />
					{#if researchEditable}
						<Button
							variant="outline"
							size="sm"
							class="research-edit"
							onclick={() => (editingResearch = true)}
						>
							{mergeable ? 'Edit research record' : 'Record outcome'}
						</Button>
					{/if}
				{/if}
			</div>
			<div class="header-actions">
				{#if mergeable && activeBranch.id !== comparison.branch.id}
					<Button variant="outline" onclick={() => switchBranch(comparison?.branch ?? null)}>
						Switch to branch
					</Button>
				{/if}
				{#if comparison.branch.status !== 'archived'}
					<Button variant="ghost" href="/branches/{comparison.branch.id}/research">Research</Button>
				{/if}
				{#if mergeable}
					<Button variant="outline" onclick={() => (closeOpen = true)} disabled={merging}>
						Close branch
					</Button>
				{/if}
				{#if mergeable}
					<div class="merge-bar">
						<p class="undecided" role="status">{undecidedLabel}</p>
						{#if blockerLabel}
							<p class="undecided blocker-count" role="status">{blockerLabel}</p>
						{/if}
						<Button
							onclick={() => (confirmOpen = true)}
							disabled={!hasChanges || undecidedCount > 0 || blockers.length > 0 || merging}
						>
							Review &amp; merge
						</Button>
					</div>
				{/if}
			</div>
		</header>

		{#if isIncompleteMerge(comparison.branch)}
			<IncompleteMergeCallout
				pending={pendingEntities(comparison.branch)}
				unreadable={isUnreadableMerge(comparison.branch)}
				onfinish={() => (finishOpen = true)}
			/>
		{/if}

		{#if comparison.has_more}
			<div class="truncation" role="note">
				One or both sides hit the read cap, so this comparison is <strong>partial</strong>. More
				changes exist than are shown below.
			</div>
		{/if}

		{#if mergeable}
			<MergeBlockersPanel
				{blockers}
				checking={checkingBlockers}
				error={blockerCheckError}
				onfix={applyBlockerFix}
				disabled={merging}
			/>
			{#if hasChanges}
				<BranchEvidenceCoveragePanel
					branchId={comparison.branch.id}
					onBranch={activeBranch.id === comparison.branch.id}
					onswitch={() => switchBranch(comparison?.branch ?? null)}
				/>
				<BranchHealthPanel
					branchId={comparison.branch.id}
					onBranch={activeBranch.id === comparison.branch.id}
					onswitch={() => switchBranch(comparison?.branch ?? null)}
				/>
			{/if}
		{/if}

		{#if mergeRecord}
			{@render mergeRecordSection(mergeRecord)}
		{/if}

		{#if !mergeRecord}
			<section class="verdict">
				<h2>Conflicts</h2>
				<p class="section-hint">
					{#if mergeable}
						Entities whose branch and mainline changes are actually incompatible. Every one needs a
						decision before this branch can be merged.
					{:else}
						Entities whose branch and mainline changes were incompatible. This branch is
						{comparison.branch.status} and accepts no further changes, so this is a record rather
						than a decision.
					{/if}
				</p>
				{#if conflicts.length === 0}
					<p class="clean">No conflicts. This branch's changes are compatible with the mainline.</p>
				{:else if mergeable}
					<!--
						Shown from `mergeResolutions`, not `resolutions`, so the picker always
						reads back what will actually be sent: an excluded entity displays the
						`main` its exclusion folds on top, rather than the branch's-version
						choice the payload overrides. Nothing is lost by this - the fold is
						one-way and `resolutions` still holds the original decision, so
						unticking the exclusion restores it. `onresolve` writes to
						`resolutions` (never the derived map), which is what keeps that true.
					-->
					<MergeConflictResolver
						{conflicts}
						resolutions={mergeResolutions}
						onresolve={resolveConflict}
						onresolveall={resolveConflicts}
						{rationales}
						onrationale={setRationale}
						blocked={blockedIds}
						disabled={merging}
					/>
				{:else}
					<!--
						A terminal branch gets the read-only list, not a disabled resolver: it
						can never take another write, so offering pickers at all - even inert
						ones - would suggest a decision is still outstanding.
					-->
					<ul class="conflict-list">
						{#each conflicts as conflict (conflict.stream_id)}
							<li class="conflict">
								<div class="conflict-head">
									<span class="entity-type">{entityTypeLabel(conflict.entity_type)}</span>
									<span class="conflict-name"
										>{conflict.entity_name || unnamedEntityLabel(conflict.entity_type)}</span
									>
									<Badge variant="destructive">{conflictLabel(conflict.kind)}</Badge>
								</div>
								<p class="conflict-detail">{conflict.detail}</p>
								{#if conflict.field_values && conflict.field_values.length > 0}
									<ConflictValues {conflict} />
								{:else if conflict.fields && conflict.fields.length > 0}
									<p class="conflict-fields">
										Contested fields: {conflict.fields.join(', ')}
									</p>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{/if}

		<section class="hint">
			<h2>Also changed on both sides</h2>
			<p class="section-hint">
				{#if mergeRecord}
					A divergence hint for human review, not a verdict - these entities were touched by both
					the branch and the mainline and are not among the merge's decisions above. That includes
					entities the mainline changed again after the merge.
				{:else}
					A divergence hint for human review, not a verdict - these entities were touched by both
					the branch and the mainline but their changes do not conflict.
				{/if}
			</p>
			{#if cleanOverlaps.length === 0}
				<p class="clean">
					{comparison.overlapping_stream_ids.length === 0
						? 'No entities were changed on both sides.'
						: mergeRecord
							? 'Every entity changed on both sides is listed in the merge record above.'
							: 'Every entity changed on both sides is listed as a conflict above.'}
				</p>
			{:else}
				<ul class="overlap-list">
					{#each cleanOverlaps as streamId (streamId)}
						{@const known = overlapNames.get(streamId)}
						<li>
							{#if known}
								<span class="entity-type">{entityTypeLabel(known.type)}</span>
								<span class="overlap-name">{known.name || unnamedEntityLabel(known.type)}</span>
							{:else}
								<code>{streamId}</code>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="changes">
			<!-- The two sides render the same entities in the same shape, so only the
			     container tells them apart. The E2E suite needs a handle that survives
			     a CSS rename to assert which side a change landed on. -->
			<div class="side" data-testid="branch-changes">
				<h2>On this branch</h2>
				<p class="side-count">{comparison.branch_change_count} change{comparison.branch_change_count === 1 ? '' : 's'} since the fork</p>
				{@render changeList(comparison.branch_changes, 'No changes on this branch yet.', true)}
			</div>
			<div class="side" data-testid="main-changes">
				<h2>On the mainline</h2>
				<p class="side-count">
					{comparison.main_change_count} change{comparison.main_change_count === 1 ? '' : 's'} to the
					same entities since the fork
				</p>
				{#if replayedCopies > 0}
					<p class="side-count replayed-note" data-testid="replayed-note">
						{plural(replayedCopies, 'change')} the merge copied from this branch
						{replayedCopies === 1 ? 'is' : 'are'} not listed here: {replayedCopies === 1
							? "it is this branch's own change"
							: "they are this branch's own changes"}, shown alongside.
					</p>
				{/if}
				{@render changeList(
					comparison.main_changes,
					'The mainline has not touched any of the entities this branch changed.',
					false
				)}
			</div>
		</section>

		<!--
			The dialog owns preview -> confirm -> outcome, but issues nothing and
			navigates nowhere. Both belong here: this page holds the resolution state
			a `merge_conflicts` refusal forces it to rebuild, and `returnToMainline()`
			reloads the page - calling it automatically on success would destroy the
			summary the instant it rendered, so the user has to ask for it.
		-->
		<MergeConfirmDialog
			open={confirmOpen}
			branch={comparison.branch}
			plan={mergePlan}
			isActiveBranch={activeBranch.id === comparison.branch.id}
			onconfirm={performMerge}
			onclose={() => (confirmOpen = false)}
			onrefused={handleRefused}
			onrecompare={() => loadComparison(branchId)}
			onreturntomainline={returnToMainline}
			onfinishmerge={startFinishMerge}
		/>

		{#if comparison.branch.status === 'merged'}
			<FinishMergeDialog
				open={finishOpen}
				branch={comparison.branch}
				onresume={performResume}
				onclose={closeFinish}
				onrefresh={() => loadComparison(branchId)}
			/>
		{/if}
	{/if}
</div>

<CloseBranchDialog
	branch={closeOpen ? (comparison?.branch ?? null) : null}
	onclosed={handleClosed}
	oncancel={() => (closeOpen = false)}
/>

<style>
	.closed-record {
		margin: 0.5rem 0 0;
		padding: 0.625rem 0.75rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		color: #334155;
	}

	.closed-record p {
		margin: 0;
	}

	.closed-reason {
		display: block;
		margin-top: 0.25rem;
		color: #1e293b;
	}

	.research-link {
		display: inline-block;
		margin-top: 0.375rem;
		font-weight: 500;
		color: #2563eb;
	}

	.close-notice {
		margin: 0.5rem 0 0;
		padding: 0.625rem 0.75rem;
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		border-radius: 6px;
		font-size: 0.875rem;
		color: #166534;
	}

	.close-notice p {
		margin: 0;
	}

	.close-warning {
		margin-top: 0.25rem !important;
		color: #92400e;
	}

	.compare-page {
		max-width: 1100px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.back-link {
		display: inline-block;
		margin-bottom: 1rem;
		font-size: 0.875rem;
		color: #64748b;
		text-decoration: none;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.page-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1.5rem;
	}

	.page-header > div:first-child {
		flex: 1 1 28rem;
		min-width: 0;
	}

	.page-header :global(.research-edit) {
		margin-top: 0.5rem;
	}

	.title-row {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		flex-wrap: wrap;
	}

	.title-row h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.description {
		margin: 0.375rem 0 0;
		font-size: 0.875rem;
		color: #475569;
	}

	.anchor {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #94a3b8;
	}

	/* Wraps under the heading at narrow widths rather than squeezing beside it. */
	.header-actions {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.merge-bar {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.undecided {
		margin: 0;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.truncation {
		margin-bottom: 1.5rem;
		padding: 0.75rem 1rem;
		background: #fef3c7;
		border: 1px solid #f59e0b;
		border-radius: 6px;
		font-size: 0.8125rem;
		color: #92400e;
	}

	section {
		margin-bottom: 2rem;
	}

	section h2 {
		margin: 0 0 0.25rem;
		font-size: 1rem;
		color: #1e293b;
	}

	.section-hint {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		color: #64748b;
		max-width: 52rem;
	}

	.clean {
		margin: 0;
		padding: 0.75rem 1rem;
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		border-radius: 6px;
		font-size: 0.875rem;
		color: #166534;
	}

	.conflict-list,
	.overlap-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.conflict {
		padding: 0.75rem 1rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
	}

	.conflict-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.conflict-name {
		font-weight: 600;
		color: #1e293b;
	}

	.conflict-detail {
		margin: 0.375rem 0 0;
		font-size: 0.875rem;
		color: #7f1d1d;
	}

	.conflict-fields {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #b91c1c;
	}

	.merged-line {
		margin: 0.375rem 0 0;
		font-size: 0.875rem;
		color: #475569;
	}

	.merged-note {
		font-style: italic;
		color: #1e293b;
		overflow-wrap: anywhere;
	}

	.merge-record h3 {
		margin: 1rem 0 0.5rem;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.record-facts {
		display: grid;
		grid-template-columns: 1fr;
		gap: 0.5rem;
		margin: 0;
		padding: 0.75rem 1rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
	}

	@media (min-width: 640px) {
		.record-facts {
			grid-template-columns: 1fr 1fr;
		}
	}

	.record-facts dt {
		font-size: 0.75rem;
		color: #64748b;
	}

	.record-facts dd {
		margin: 0;
		font-size: 0.875rem;
		color: #1e293b;
		overflow-wrap: anywhere;
	}

	.record-note {
		white-space: pre-line;
	}

	.record-legacy {
		margin: 0.75rem 0 0;
		padding: 0.5rem 0.75rem;
		background: #fef3c7;
		border: 1px solid #f59e0b;
		border-radius: 6px;
		font-size: 0.8125rem;
		color: #92400e;
	}

	.record-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.record-item {
		padding: 0.75rem 1rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
	}

	.record-fields {
		color: #475569;
	}

	.record-rationale {
		margin: 0.375rem 0 0;
		font-size: 0.875rem;
		color: #334155;
		overflow-wrap: anywhere;
	}

	.record-when,
	.muted {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.overlap-name {
		font-weight: 500;
		color: #1e293b;
	}

	.replayed-note {
		font-style: italic;
	}

	.overlap-list li {
		font-size: 0.8125rem;
		color: #475569;
	}

	.overlap-list code {
		font-size: 0.75rem;
		background: #f1f5f9;
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
	}

	/* Two sides at desktop width; stacked on narrow screens so the diff never
	   forces horizontal scrolling. */
	.changes {
		display: grid;
		grid-template-columns: 1fr;
		gap: 1.5rem;
	}

	@media (min-width: 768px) {
		.changes {
			grid-template-columns: 1fr 1fr;
		}
	}

	.side-count {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.side-empty {
		margin: 0;
		font-size: 0.875rem;
		color: #94a3b8;
	}

	.change-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.change-entry {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.875rem;
	}

	.change-entry.contested {
		border-color: #fca5a5;
	}

	/* Alongside the "Merge blocker" badge, so it never rests on colour alone. */
	.change-entry.blocked {
		border-color: #fb923c;
		box-shadow: 0 0 0 1px #fb923c;
	}

	.blocker-count {
		color: #9a3412;
		font-weight: 500;
	}

	/* Dashed and dimmed, alongside the "Not merging" badge - shape and words, so
	   the exclusion never rests on colour alone. */
	.change-entry.excluded {
		border-style: dashed;
		border-color: #94a3b8;
		background: #f8fafc;
	}

	.exclude-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.625rem;
		padding-top: 0.625rem;
		border-top: 1px solid #e2e8f0;
	}

	.exclude-text {
		font-size: 0.8125rem;
		color: #475569;
	}

	.exclude-name {
		font-weight: 500;
		color: #1e293b;
	}

	.change-head {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		margin-bottom: 0.375rem;
		flex-wrap: wrap;
	}

	.change-time {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.change-body {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.entity-type {
		font-size: 0.75rem;
		color: #94a3b8;
		text-transform: capitalize;
		padding: 0.125rem 0.375rem;
		background: #f1f5f9;
		border-radius: 4px;
	}

	.entity-name {
		font-weight: 500;
		color: #1e293b;
		text-decoration: none;
	}

	a.entity-name:hover {
		color: #3b82f6;
	}

	.entity-name.deleted {
		color: #94a3b8;
		text-decoration: line-through;
	}

	.change-diff {
		margin-top: 0.625rem;
		padding-top: 0.625rem;
		border-top: 1px solid #e2e8f0;
	}

	.state {
		padding: 2rem;
		text-align: center;
		color: #64748b;
	}

	.state.error {
		color: #dc2626;
	}

	.state.empty {
		background: white;
		border: 1px dashed #cbd5e1;
		border-radius: 8px;
	}

	.state.empty h2 {
		margin: 0 0 0.375rem;
		font-size: 1rem;
		color: #1e293b;
	}
</style>
