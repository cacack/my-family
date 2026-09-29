package command

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
)

// MergeCompletenessState says whether a merged branch's changes have all
// reached main (#830).
type MergeCompletenessState string

const (
	// MergeStateComplete: every entity the merge set out to replay is on main,
	// or was deliberately left behind.
	MergeStateComplete MergeCompletenessState = "complete"
	// MergeStateIncomplete: the merge was claimed but its replay stopped
	// partway (ErrMergePartiallyApplied); ResumeMerge finishes it.
	MergeStateIncomplete MergeCompletenessState = "incomplete"
)

// MergePendingReason says why an entity of an incomplete merge has not reached
// main yet, and so whether finishing the merge needs a decision about it.
type MergePendingReason string

const (
	// PendingReady: the recorded plan still vouches for the entity; a resume
	// replays it without asking.
	PendingReady MergePendingReason = "ready"
	// PendingMainChanged: main changed the entity after the merge pinned it,
	// so the merge's conflict verdict no longer describes it.
	PendingMainChanged MergePendingReason = "main_changed"
	// PendingMainRemoved: main deleted the entity (or merged the person into
	// another) after the merge was claimed. Only "main" can resolve it.
	PendingMainRemoved MergePendingReason = "main_removed"
	// PendingNoPlan: the merge was claimed before claims recorded a replay
	// plan, so nothing vouches for the entity.
	PendingNoPlan MergePendingReason = "no_plan"
	// PendingBreaksReference: replaying the entity would leave main
	// referencing something it no longer has (a person, source, media owner
	// or GPS subject), or cascade onto data main still has. Only "main" can
	// resolve it.
	PendingBreaksReference MergePendingReason = "breaks_reference"
	// PendingNeedsRepair: the entity's events reached main's log, but main's
	// read model does not show them yet (the append landed and its
	// synchronous projection failed), or a source's citation count disagrees
	// with main's citations of it. A resume re-projects it from the log
	// without asking.
	PendingNeedsRepair MergePendingReason = "needs_repair"
)

// NeedsResolution reports whether a resume must be told what to do with an
// entity pending for this reason.
func (r MergePendingReason) NeedsResolution() bool {
	return r != PendingReady && r != PendingNeedsRepair
}

// SupportedResolutions are the decisions a resume accepts for an entity
// pending for this reason: none for one it replays without asking, only
// "main" where replaying the branch's changes is refused (a removed entity,
// or a replay that would break a reference), both otherwise.
func (r MergePendingReason) SupportedResolutions() []MergeResolution {
	switch r {
	case PendingReady, PendingNeedsRepair:
		return []MergeResolution{}
	case PendingMainRemoved, PendingBreaksReference:
		return []MergeResolution{ResolveMain}
	default:
		return []MergeResolution{ResolveBranch, ResolveMain}
	}
}

// PendingMergeEntity is one entity an incomplete merge has not replayed yet.
type PendingMergeEntity struct {
	StreamID uuid.UUID
	// EntityType is in the ChangeEntry vocabulary; EntityName is the display
	// name as the branch sees the entity ("" when nothing names it).
	EntityType string
	EntityName string
	Reason     MergePendingReason
}

// MergeCompleteness is a merged branch's merge state (#830): whether its
// replay onto main finished, and if not, what is left.
type MergeCompleteness struct {
	State MergeCompletenessState
	// Pending lists the entities still to replay, in replay order, each with
	// the reason it is pending. Empty (never nil) when State is complete.
	Pending []PendingMergeEntity
}

// resumeInspection is everything a resume reads before it writes anything:
// the replay set, what main holds of it, and the verdict per stream.
type resumeInspection struct {
	groups   []streamGroup
	view     resumeView
	decision resumeDecision
	result   *ResumeMergeResult
}

// inspectResume computes a resume's plan for a claimed merge WITHOUT WRITING
// ANYTHING: which streams already landed, which the recorded plan replays
// automatically, which were left behind, and which need a decision. It is
// the read half of ResumeMerge, shared with MergeCompleteness so the state GET
// /branches/{id} reports for the entities still to replay is exactly what a
// resume would act on (MergeCompleteness adds the landed entities a resume
// would repair).
func (h *Handler) inspectResume(
	ctx context.Context,
	branch *domain.Branch,
	record mergeRecord,
	resolutions map[uuid.UUID]MergeResolution,
) (*resumeInspection, error) {
	groups, err := h.resumeReplayGroups(ctx, branch, record.claim)
	if err != nil {
		return nil, err
	}
	return h.inspectResumeGroups(ctx, branch, record, groups, resolutions)
}

// inspectResumeGroups is inspectResume over a replay set already loaded with
// resumeReplayGroups.
func (h *Handler) inspectResumeGroups(
	ctx context.Context,
	branch *domain.Branch,
	record mergeRecord,
	groups []streamGroup,
	resolutions map[uuid.UUID]MergeResolution,
) (*resumeInspection, error) {
	claim := record.claim
	if err := validateResolutions(resolutions, groups); err != nil {
		return nil, err
	}

	// Main's versions are read BEFORE the landed scan, and the order is what
	// keeps a concurrent resume from landing a stream twice. A stream another
	// resume appends before the scan shows as landed and is left alone. One it
	// appends after the scan was read at its pre-landing version, so this
	// call's step for it asserts that stale version and replayStream refuses
	// it (ErrMergePlanStale) instead of appending over the other resume's
	// copy. Read the other way round, a landing between the two reads would
	// look unlanded AND already at main's new version — decidable, and a
	// "branch" resolution would then replay it a second time.
	mainVersions, landed, err := h.readMainState(ctx, groups, claim.MergedAtPosition)
	if err != nil {
		return nil, err
	}
	return h.inspectResumeOver(ctx, branch, record, groups, resolutions, mainVersions, landed)
}

// readMainState reads main's version of every stream of the replay set, THEN
// which of them already landed, in the order inspectResumeGroups explains.
// Every caller that inspects a resume reads them through here, once.
func (h *Handler) readMainState(ctx context.Context, groups []streamGroup, mergedAtPosition int64) (map[uuid.UUID]int64, map[uuid.UUID]bool, error) {
	mainVersions, err := h.mainStreamVersions(ctx, groups)
	if err != nil {
		return nil, nil, err
	}
	landed, err := h.streamsAlreadyOnMain(ctx, groups, mergedAtPosition)
	if err != nil {
		return nil, nil, err
	}
	return mainVersions, landed, nil
}

// inspectResumeOver is inspectResumeGroups over main's state already read with
// readMainState (and resolutions already validated).
func (h *Handler) inspectResumeOver(
	ctx context.Context,
	branch *domain.Branch,
	record mergeRecord,
	groups []streamGroup,
	resolutions map[uuid.UUID]MergeResolution,
	mainVersions map[uuid.UUID]int64,
	landed map[uuid.UUID]bool,
) (*resumeInspection, error) {
	claim := record.claim

	// What main removed since the claim, and so which persons the replay can
	// still vouch for.
	removed, relinked, err := h.streamsRemovedOnMain(ctx, groups, mainVersions)
	if err != nil {
		return nil, err
	}
	view := resumeView{
		landed:       landed,
		mainVersions: mainVersions,
		removed:      removed,
		relinked:     relinked,
		created:      personsCreatedByReplay(groups, removed),
		basePosition: branch.BasePosition,
	}

	// A stream the plan would replay automatically still needs a decision when
	// replaying it would leave main pointing at a person it no longer has.
	view.danglingAuto, err = h.danglingAutoPlannedStreams(ctx, record.plan, groups, view, resolutions)
	if err != nil {
		return nil, err
	}

	result := &ResumeMergeResult{MergedAtPosition: claim.MergedAtPosition}
	decision, err := planResume(claim.BranchID, record.plan, groups, view, resolutions, result)
	if err != nil {
		return nil, err
	}
	return &resumeInspection{groups: groups, view: view, decision: decision, result: result}, nil
}

// pendingReasonOf says why planResume could not replay a stream on the
// recorded plan. The order matters where several apply: a removed entity
// accepts only "main" whatever else is true of it.
func pendingReasonOf(plan map[uuid.UUID]int64, planned bool, pinned, current int64, view resumeView, streamID uuid.UUID) MergePendingReason {
	switch {
	case view.removed[streamID]:
		return PendingMainRemoved
	case plan == nil || !planned:
		return PendingNoPlan
	case current != pinned:
		return PendingMainChanged
	default:
		return PendingBreaksReference
	}
}

// MergeCompleteness reports whether a merged branch's merge finished (#830),
// and, when it did not, the entities a resume would still act on, with their
// names and why each is pending. It returns nil for a branch that is not
// merged.
//
// It WRITES NOTHING — not even the registry repair a resume performs — so it
// is safe on every read of a branch. It shares every verdict with ResumeMerge:
//
//   - which entities are left to replay, and which of them need a decision,
//     come from the resume's read-only plan (inspectResume), so an entity is
//     reported as pending exactly when a resume would replay it or ask about
//     it;
//   - which entities already on main's LOG a resume would re-project, because
//     main's read model is behind the log for them (an append that landed
//     and whose synchronous projection then failed), come from the read half
//     of the resume's repair (planReadModelRepair), and are reported as
//     needs_repair;
//   - so do the entities whose sources' citation counts a resume would
//     recount (citationCountsOff).
//
// So a merge that stopped at a projection failure reads as incomplete even
// when its last stream reached main's log. The same checks also report a
// merged entity whose read model a LATER mainline write left behind: a resume
// repairs that too, so it is reported rather than hidden.
//
// Only the reads that can change the verdict cost anything: a finished merge
// is remembered against the event log's head (see mergeStateCache), so
// reading it again, as the branch list does on every load, costs one head
// read until something is appended anywhere. A read that does recompute costs
// a fixed number of statements per merged branch apart from the per-stream
// version reads it shares with a resume: main's log is scanned once for the
// whole replay set, and the cited sources and their citation counts are read
// in one batched statement each.
func (h *Handler) MergeCompleteness(ctx context.Context, branch *domain.Branch) (*MergeCompleteness, error) {
	if branch == nil || branch.Status != domain.BranchStatusMerged {
		return nil, nil
	}
	if h.branchStore == nil {
		return nil, ErrBranchStoreRequired
	}

	head, cacheable, err := h.logHead(ctx)
	if err != nil {
		return nil, err
	}
	if cacheable && h.mergeStates.completeAt(branch.ID, head) {
		return &MergeCompleteness{State: MergeStateComplete, Pending: []PendingMergeEntity{}}, nil
	}

	completeness, err := h.readMergeCompleteness(ctx, branch)
	if err != nil {
		return nil, err
	}
	if cacheable && completeness.State == MergeStateComplete {
		h.mergeStates.markComplete(branch.ID, head)
	}
	return completeness, nil
}

// readMergeCompleteness is MergeCompleteness without the cache.
func (h *Handler) readMergeCompleteness(ctx context.Context, branch *domain.Branch) (*MergeCompleteness, error) {
	record, _, err := h.readMergeRecord(ctx, branch)
	if err != nil {
		return nil, err
	}
	groups, err := h.resumeReplayGroups(ctx, branch, record.claim)
	if err != nil {
		return nil, err
	}

	// Main's state is read ONCE, in the order a resume reads it, and shared by
	// the replay check, the resume's plan and the repair check.
	mainVersions, landed, err := h.readMainState(ctx, groups, record.claim.MergedAtPosition)
	if err != nil {
		return nil, err
	}
	reasons := make(map[uuid.UUID]MergePendingReason)
	if anyStreamLeftToReplay(groups, record, landed) {
		inspection, err := h.inspectResumeOver(ctx, branch, record, groups, nil, mainVersions, landed)
		if err != nil {
			return nil, err
		}
		for _, step := range inspection.decision.steps {
			reasons[step.group.streamID] = PendingReady
		}
		for _, id := range inspection.result.PendingStreamIDs {
			reasons[id] = inspection.decision.reasons[id]
		}
	}

	if err := h.addRepairReasons(ctx, groups, landed, mainVersions, reasons); err != nil {
		return nil, err
	}
	if len(reasons) == 0 {
		return &MergeCompleteness{State: MergeStateComplete, Pending: []PendingMergeEntity{}}, nil
	}
	pending, err := h.describePendingStreams(ctx, branch.ID, groups, reasons)
	if err != nil {
		return nil, err
	}
	return &MergeCompleteness{State: MergeStateIncomplete, Pending: pending}, nil
}

// addRepairReasons marks, as needs_repair, the landed streams a resume would
// re-project or whose sources' citation counts it would recount.
func (h *Handler) addRepairReasons(
	ctx context.Context,
	groups []streamGroup,
	landed map[uuid.UUID]bool,
	mainVersions map[uuid.UUID]int64,
	reasons map[uuid.UUID]MergePendingReason,
) error {
	repairs, err := h.planReadModelRepair(ctx, groups, landed, mainVersions)
	if err != nil {
		return err
	}
	for _, repair := range repairs {
		reasons[repair.group.streamID] = PendingNeedsRepair
	}
	recount, err := h.citationCountsOff(ctx, groups, landed)
	if err != nil {
		return err
	}
	for _, id := range recount {
		if _, listed := reasons[id]; !listed {
			reasons[id] = PendingNeedsRepair
		}
	}
	return nil
}

// logHead reads the event log's head position, reporting whether there is a
// head reader to read it with (without one, nothing is cached).
func (h *Handler) logHead(ctx context.Context) (int64, bool, error) {
	if h.snapshots == nil || h.mergeStates == nil {
		return 0, false, nil
	}
	head, err := h.snapshots.GetMaxPosition(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("reading the event log's head: %w", err)
	}
	return head, true, nil
}

// anyStreamLeftToReplay reports whether any stream of the replay set is
// neither on main (landed) nor left behind by the recorded plan.
func anyStreamLeftToReplay(groups []streamGroup, record mergeRecord, landed map[uuid.UUID]bool) bool {
	for _, group := range groups {
		if landed[group.streamID] {
			continue
		}
		if _, planned := record.plan[group.streamID]; record.plan == nil || planned {
			return true
		}
	}
	return false
}

// describePendingStreams names the streams in reasons, in one batched lookup,
// as the branch sees them. The order is the replay order.
func (h *Handler) describePendingStreams(ctx context.Context, branchID uuid.UUID, groups []streamGroup, reasons map[uuid.UUID]MergePendingReason) ([]PendingMergeEntity, error) {
	pending := make([]PendingMergeEntity, 0, len(reasons))
	refs := make([]query.EntityRef, 0, len(reasons))
	for _, group := range groups {
		reason, ok := reasons[group.streamID]
		if !ok {
			continue
		}
		entity := PendingMergeEntity{
			StreamID:   group.streamID,
			EntityType: entityTypeOfStream(group.streamType),
			Reason:     reason,
		}
		pending = append(pending, entity)
		refs = append(refs, query.EntityRef{EntityType: entity.EntityType, ID: entity.StreamID})
	}
	if len(refs) == 0 {
		return pending, nil
	}
	names, err := h.branchService.NameEntities(ctx, domain.BranchID(branchID), refs)
	if err != nil {
		return nil, fmt.Errorf("naming the merge's pending entities: %w", err)
	}
	for i := range pending {
		pending[i].EntityName = names[refs[i]]
	}
	return pending, nil
}

// mergeStateCache remembers, per merged branch, the event log head at which
// its merge was last found complete. A finished merge's verdict depends only
// on the event log and on main's read model, and main's read model only moves
// behind the log when something is appended (a projection that fails follows
// an append), so while the head has not moved the verdict still holds. Any
// append anywhere moves the head and the next read recomputes. Only
// "complete" is remembered: an incomplete merge is always read afresh, so a
// resume's read-model repair, which appends nothing, is seen at once.
//
// The key is the global head, not the positions of the branch's own streams,
// because the verdict also depends on writes outside the replay set: any
// citation of a source the merge cites moves that source's count, and a
// person merge or delete elsewhere changes which rows a repair would
// resurrect. Keying on the replay set's streams would keep a stale "complete"
// across those.
//
// It is held by pointer so the handler's branch-scoped copies (WithBranch)
// share it.
type mergeStateCache struct {
	mu       sync.Mutex
	complete map[uuid.UUID]int64
}

func newMergeStateCache() *mergeStateCache {
	return &mergeStateCache{complete: make(map[uuid.UUID]int64)}
}

func (c *mergeStateCache) completeAt(branchID uuid.UUID, head int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.complete[branchID]
	return ok && at == head
}

func (c *mergeStateCache) markComplete(branchID uuid.UUID, head int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.complete[branchID] = head
}
