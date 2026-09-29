package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// Branch merge command errors.
var (
	// ErrMergeConflicts is returned when the branch and main made incompatible
	// changes to the same aggregate and the caller did not say which side wins.
	// ADR-005 §Conflict definition: "any conflict requires review before the
	// merge can complete", so the command refuses rather than picking a side.
	// The conflicts are on the returned MergeBranchResult.
	ErrMergeConflicts = errors.New("branch has unresolved merge conflicts")

	// ErrMergeEmpty is returned when the branch has no changes of its own
	// since it forked. Merging it would record a "merged" that promoted
	// nothing (#828), so it is refused and the branch stays active.
	ErrMergeEmpty = errors.New("branch has no changes to merge")

	// ErrMergeAlreadyClaimed is returned when another request won the
	// active→merged compare-and-set for this branch while this one was
	// planning. The loser has written nothing; the merge it lost to is the
	// merge (ADR-005 requires exactly one).
	ErrMergeAlreadyClaimed = errors.New("branch merge was already claimed by a concurrent request")

	// ErrBranchTooLargeToMerge is returned when the BRANCH's own scan hit the
	// comparison cap, so its replay set is incomplete. Merging a partial plan
	// would silently promote half a branch, so refuse instead. This is a
	// property of the branch and does not resolve on its own.
	ErrBranchTooLargeToMerge = errors.New("branch is too large to merge: its replay set is incomplete")

	// ErrMainTooFarAheadToMerge is returned when a scan of MAIN hit the
	// comparison cap, so the conflict list is not known to be complete even
	// though the branch may be small. Kept distinct from
	// ErrBranchTooLargeToMerge because the cause and the remedy differ: this
	// one is driven by mainline activity since the fork, not by branch size,
	// and telling a user their three-event branch is "too large" sends them
	// after the wrong fix.
	ErrMainTooFarAheadToMerge = errors.New("main has moved too far since the fork to verify this merge: the conflict scan is incomplete")

	// ErrUnknownResolution is returned when a resolution names a stream the
	// branch never changed, or carries a value that is neither "branch" nor
	// "main". Both mean the caller and the server disagree about what is being
	// merged, which is not something to guess at.
	ErrUnknownResolution = errors.New("merge resolution is unknown")

	// ErrUnsupportedResolution is returned when a resolution is a legal value
	// but would not produce the outcome it names for that conflict — resolving
	// a main-deleted entity to "branch", or a create_create to "branch". The
	// alternative is a 200 reporting a merge that silently did the opposite of
	// what the caller chose, so refuse and say which resolutions are available
	// (query.MergeConflict.SupportedResolutions).
	ErrUnsupportedResolution = errors.New("merge resolution is not supported for this conflict")

	// ErrMergeDanglingReference is returned when the replay would break a
	// reference that crosses aggregates. Either main would end up holding a
	// reference to an entity it will not have (a family child whose person
	// was deleted on main, a citation whose source was), or a replayed delete
	// would take down main rows that still reference the deleted entity (a
	// branch SourceDeleted cascading onto a citation main added after the
	// fork). Refused rather than silently dropping or orphaning data (see
	// validateNoDanglingReferences and collectEvidenceBlockers). The refusal is a
	// *MergeBlockedError listing every breach (#831).
	ErrMergeDanglingReference = errors.New("merge would leave a reference pointing at an entity main does not have")

	// ErrMergePlanStale is returned when main moved on a stream this merge would
	// replay, after the conflict verdict was computed against it. The verdict
	// therefore describes a main that no longer exists: the mainline write was
	// never compared with the branch's events, and replaying over it would be
	// the silent override ADR-005 §Conflict definition exists to prevent.
	//
	// NOTHING HAS BEEN WRITTEN and the branch is still active. The remedy is to
	// re-plan — GET /branches/{id}/compare, which re-runs conflict detection
	// against main as it is now — and merge again against that fresh verdict.
	//
	// This is the exact opposite of ErrMergePartiallyApplied, and the two must
	// never be confused: there, the branch is terminal and main is half-updated,
	// so retrying is the wrong move. Here, retrying against a fresh plan IS the
	// fix. Past the claim this sentinel is deliberately wrapped in
	// ErrMergePartiallyApplied, because past the claim that is the true state.
	ErrMergePlanStale = errors.New("merge plan is stale: main moved after the conflict verdict was computed")

	// ErrMergePlanIncomplete is returned when the merge plan does not carry a
	// pinned main version for a stream the merge would replay, so the staleness
	// guard has nothing to compare against. It is an internal-invariant failure,
	// not a caller mistake: PlanMerge pins every replayed stream. It exists
	// because the alternative — defaulting the missing version to 0 — silently
	// disables the guard for precisely the streams main has never seen, which is
	// the one case Append's optimistic concurrency cannot catch either.
	//
	// Deliberately not mapped to a 4xx code: a caller cannot fix it, and the
	// generic 500 is the honest answer. ResumeMerge (#685) replays from the
	// plan recorded on the claim rather than from a MergePlan, and applies the
	// same never-default rule to it.
	ErrMergePlanIncomplete = errors.New("merge plan is incomplete: a replayed stream has no pinned main version")

	// ErrMergePartiallyApplied is returned when the branch was claimed (it is
	// now merged) but the replay onto main failed partway. This is the
	// non-transactional window ADR-005's merge implementation note documents:
	// the claim and the replay are not one transaction because there is no
	// cross-store transaction facility. It is distinct from every other merge
	// error because it is the only one where the caller must NOT retry blindly
	// — the branch is terminal and main may be partially updated.
	//
	// "May be": the replay can also fail on its FIRST stream, leaving the branch
	// merged and main completely untouched. That is the common shape for a
	// single-stream branch losing the residual staleness race, and it needs a
	// different response from a genuine half-application, so the message says
	// explicitly whether anything reached main (see replayOntoMain). Either way
	// the branch is terminal: the way forward is Handler.ResumeMerge (#685),
	// never a second MergeBranch.
	ErrMergePartiallyApplied = errors.New("branch was marked merged but the replay onto main did not finish")
)

// MergeResolution names the side that wins for one aggregate. Resolving to
// "main" is also the only supported way to exclude an entity from a merge:
// partial merge / cherry-pick is deliberately deferred (ADR-005 §Merge).
type MergeResolution string

const (
	// ResolveBranch replays the branch's events for the stream onto main.
	ResolveBranch MergeResolution = "branch"

	// ResolveMain keeps main's version: the stream's branch events are not
	// replayed and the stream is reported in MergeBranchResult.SkippedStreamIDs.
	ResolveMain MergeResolution = "main"
)

// MergeBranchInput is a request to promote a branch's research onto main.
type MergeBranchInput struct {
	BranchID uuid.UUID

	// Note is the merge note recorded on the BranchMerged event and the branch
	// registry row. Optional; validated against the domain's length rule.
	Note string

	// Resolutions decides, per aggregate, which side wins. Every conflicting
	// stream must appear here or the merge is refused. A stream the branch
	// touched without conflict may also appear, which is how a caller excludes
	// it (ResolveMain).
	Resolutions map[uuid.UUID]MergeResolution // streamID → winning side

	// Rationales optionally says, per resolved stream, why that side won
	// (#828). Each key must also be in Resolutions. Recorded on the
	// BranchMerged claim; blank entries are dropped.
	Rationales map[uuid.UUID]string

	// SnapshotBefore asks the merge to mark the mainline with a snapshot
	// ("Before merging <branch>") just before it claims the branch (#833), and
	// to record its id on the claim. See takePreMergeSnapshot.
	SnapshotBefore bool
}

// MergeBranchResult reports what a merge did — or, alongside ErrMergeConflicts,
// what stood in its way.
type MergeBranchResult struct {
	// Branch is the branch re-read after the merge, so its Status, MergedAt and
	// MergeNote reflect the registry.
	Branch *domain.Branch

	// MergedAtPosition is main's head position the branch was merged onto.
	MergedAtPosition int64

	// ReplayedEventCount is how many branch events were re-appended to main.
	ReplayedEventCount int

	// SkippedStreamIDs are the streams resolved to main, whose branch events
	// were deliberately not replayed.
	SkippedStreamIDs []uuid.UUID

	// Conflicts is populated only alongside ErrMergeConflicts, where it carries
	// the whole conflict list so a caller can render the review. It is nil on a
	// successful merge.
	Conflicts []query.MergeConflict

	// PreMergeSnapshot is the snapshot taken before the claim when the input
	// asked for one (SnapshotBefore); nil otherwise.
	PreMergeSnapshot *domain.Snapshot
}

// MergeBranch replays a branch's genealogy-mutation events onto main and marks
// the branch merged (issue #55, invariant BR-004).
//
// The order below is load-bearing:
//
//  1. Guards, then the merge plan. A truncated plan is refused outright.
//  2. Conflicts must all be resolved before anything is written.
//  3. When asked (SnapshotBefore), the PRE-MERGE SNAPSHOT (#833): a mainline
//     snapshot marking the log before anything is replayed. A refusal at
//     step 4 or 5 discards it again; see takePreMergeSnapshot for its races.
//  4. The STALENESS CHECK: main must still sit at the versions the plan was
//     computed against, or the verdict from step 1 no longer describes main and
//     the merge is refused (ErrMergePlanStale) with nothing written.
//  5. The CLAIM: BranchMerged is appended to the branch's OWN stream at the
//     version this call observed. Per-(stream, branch) optimistic concurrency
//     makes that append the atomic compare-and-set ADR-005 asks for — exactly
//     one of two concurrent merges wins it, and the loser has not yet touched
//     main.
//  6. Only then the replay onto main, which re-asserts the same planned
//     versions per stream as it goes.
//
// Step 4 MUST precede step 5. claimMerge marks the branch terminal, so a
// staleness refusal after it would leave the branch merged with nothing
// replayed and no way to retry — the retry would hit the status guard and get
// 409 branch_not_active, which is not evidence of anything. Checking first
// keeps "stale" in the same class as every other refusal: nothing written,
// branch still active, re-plan and try again.
//
// RESIDUAL WINDOW: steps 4 and 6 close the gap #698 described but cannot make it
// zero. Between the check in step 4 and each stream's append in step 6 there is
// still no lock, so a mainline write can still land — step 6's per-stream
// assertion catches it, but by then the branch is claimed, so it surfaces as the
// partially-applied state below rather than as a clean refusal. Shrinking that
// last window needs the transaction the codebase does not have; a merge-wide
// lock is not an option, as it contradicts ADR-005's per-(stream, branch)
// design.
//
// KNOWN LIMITATION: steps 5 and 6 are not one transaction. The codebase has no
// cross-store transaction facility and ADR-003's synchronous projections are
// per-append, so a failure mid-replay leaves the branch merged with main
// partially updated — or, when the FIRST stream fails, with main untouched. The
// returned error names the stream that failed, how many events had already been
// replayed, and which of those two states this is, so it is diagnosable without
// counting. The claim records the replay plan (the streams to replay and their
// pinned main versions), so ResumeMerge (#685) can finish the replay from the
// log alone. Replaying one Append per stream (rather than per event) keeps the
// failure granularity at whole-entity, since the SQL backends wrap an Append in
// a transaction — which is also what lets a resume classify each stream as
// wholly replayed or not replayed at all.
func (h *Handler) MergeBranch(ctx context.Context, input MergeBranchInput) (*MergeBranchResult, error) {
	if h.branchStore == nil {
		return nil, ErrBranchStoreRequired
	}
	if h.snapshots == nil {
		return nil, ErrPositionSourceRequired
	}

	branch, err := h.branchStore.Get(ctx, input.BranchID)
	if err != nil {
		return nil, err // includes repository.ErrBranchNotFound
	}
	if branch.Status != domain.BranchStatusActive {
		return nil, fmt.Errorf("%w: %s", ErrBranchNotActive, branch.Status)
	}

	// Validate the note against the domain rule before writing anything, by
	// asking the branch it will end up on. Cheaper than discovering it when
	// MarkMerged has already run.
	candidate := *branch
	candidate.MergeNote = input.Note
	if err := candidate.Validate(); err != nil {
		return nil, err
	}

	plan, err := h.branchService.PlanMerge(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("planning merge: %w", err)
	}
	if err := refuseUnmergeablePlan(branch, plan); err != nil {
		return nil, err
	}

	groups, err := orderEvidenceForReplay(groupEventsByStream(plan.ReplayEvents))
	if err != nil {
		return nil, err
	}
	if err := validateResolutions(input.Resolutions, groups); err != nil {
		return nil, err
	}
	rationales, err := validateRationales(input.Rationales, input.Resolutions)
	if err != nil {
		return nil, err
	}
	if err := validateConflictResolutions(plan.Conflicts, input.Resolutions); err != nil {
		return nil, err
	}
	if err := h.validateNoDanglingReferences(ctx, plan, groups, input.Resolutions); err != nil {
		return nil, err
	}
	if unresolved := unresolvedConflicts(plan.Conflicts, input.Resolutions); unresolved > 0 {
		// The refusal hands the conflicts back for review, so they carry what
		// each side says (#828); a merge that goes ahead never reads them.
		if err := h.branchService.DescribeConflictValues(ctx, plan); err != nil {
			return nil, err
		}
		return &MergeBranchResult{
				Branch:    branch,
				Conflicts: plan.Conflicts,
			}, fmt.Errorf("%w: %d of %d conflicts have no resolution",
				ErrMergeConflicts, unresolved, len(plan.Conflicts))
	}

	// main's head as the branch sees it — recorded on the marker, not used as an
	// expected version (versions are per stream).
	mergedAtPosition, err := h.snapshots.GetMaxPosition(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting max event position: %w", err)
	}

	// The claim, merge record included, is built before the staleness check:
	// naming the record's exclusions reads the read model, and that read must
	// not sit between the check and the claim.
	claim, err := h.buildMergeClaim(ctx, branch, mergedAtPosition, input, plan, groups, rationales)
	if err != nil {
		return nil, err
	}

	// The pre-merge snapshot (#833), when asked for, goes after every read
	// above and before the staleness check, so nothing sits between that check
	// and the claim.
	preMerge, err := h.snapshotAndClaim(ctx, branch, &claim, plan, groups, input)
	if err != nil {
		return nil, err
	}

	// Past this point the branch is already merged, so a replay failure is not
	// "nothing happened" — it is the partially-applied state, and the caller
	// has to be able to tell the two apart.
	replayed, skipped, err := h.replayOntoMain(ctx, branch, claim, groups, plan.MainStreamVersions, input.Resolutions)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMergePartiallyApplied, err)
	}

	merged, err := h.branchStore.Get(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("re-reading merged branch: %w", err)
	}

	return &MergeBranchResult{
		Branch:             merged,
		MergedAtPosition:   mergedAtPosition,
		ReplayedEventCount: replayed,
		SkippedStreamIDs:   skipped,
		PreMergeSnapshot:   preMerge,
	}, nil
}

// claimFreshMerge runs the staleness check and, only if main has not moved,
// claims the merge. The check is deliberately the LAST thing before the claim:
// every read before it can only widen the window between the check and the
// replay, so it goes as late as it can while still being a refusal rather than
// a half-merge. Nothing may read between the two.
func (h *Handler) claimFreshMerge(
	ctx context.Context,
	branch *domain.Branch,
	claim domain.BranchMerged,
	plan *query.MergePlan,
	groups []streamGroup,
	resolutions map[uuid.UUID]MergeResolution,
) error {
	if err := h.validatePlanNotStale(ctx, plan, groups, resolutions); err != nil {
		return err
	}
	return h.claimMerge(ctx, branch, claim)
}

// refuseUnmergeablePlan refuses a plan no resolution can make mergeable: one
// whose branch or main scan was truncated, or one with nothing to replay.
func refuseUnmergeablePlan(branch *domain.Branch, plan *query.MergePlan) error {
	// The two truncation sides are different problems and get different
	// answers. A branch bigger than the cap is permanently unmergeable as-is;
	// a main tail bigger than the cap says nothing about the branch, grows with
	// unrelated mainline activity, and is not the branch's fault.
	if plan.BranchTruncated {
		return fmt.Errorf(
			"%w: branch %s has more than %d events of its own, so its replay set is incomplete. "+
				"Retrying will not help — the cap is fixed and the branch does not shrink; "+
				"promoting a subset needs partial merge (#684)",
			ErrBranchTooLargeToMerge, branch.ID, plan.EventCap)
	}
	if plan.MainTruncated {
		return fmt.Errorf(
			"%w: more than %d events have landed on main for the streams branch %s touches since it forked, "+
				"so the conflict list is not known to be complete. The branch itself may be small — this is a "+
				"limit on how far back the comparison scans, not on the branch",
			ErrMainTooFarAheadToMerge, plan.EventCap, branch.ID)
	}

	if len(plan.ReplayEvents) == 0 {
		return fmt.Errorf("%w: branch %s has made no changes since it forked, so there is nothing to promote", ErrMergeEmpty, branch.ID)
	}
	return nil
}

// validatePlanNotStale refuses a merge whose conflict verdict was computed
// against a main that has since moved on one of the streams this merge would
// replay.
//
// The check is an EXPLICIT version comparison rather than something delegated to
// Append's optimistic concurrency, because Append cannot express it. All three
// backends gate the check on `expectedVersion >= 0` — including PostgreSQL,
// which ADR-002 makes the primary production backend
// (internal/repository/postgres/eventstore.go:122,
// internal/repository/sqlite/eventstore.go, internal/repository/memory/eventstore.go)
// — so a -1 turns the check OFF entirely rather than asserting "no prior
// events", and Append in any case compares only against the version the caller
// just read, never against the plan's pin. Leaning on Append would therefore
// leave a mainline write landing between planning and the replay's own read
// completely unguarded.
//
// Scoped to the streams that will actually be replayed. A stream resolved to
// main is not written, so main moving under it changes nothing this merge does,
// and refusing on it would fail merges for no reason. Streams the branch never
// touched are not in the plan at all, for the same reason ADR-005 scopes the
// whole comparison that way: a merge must not be hostage to unrelated mainline
// activity.
//
// A stream main has never seen is planned at 0 and still reads 0, so it does not
// trip the guard. (replayStream passes that 0 to Append as-is, which asserts "no
// prior events" — unlike the -1 sentinel, which would switch the check off.)
func (h *Handler) validatePlanNotStale(
	ctx context.Context,
	plan *query.MergePlan,
	groups []streamGroup,
	resolutions map[uuid.UUID]MergeResolution,
) error {
	for _, group := range groups {
		if resolutions[group.streamID] == ResolveMain {
			continue
		}
		// A MISSING entry is refused, never defaulted. The zero value is not a
		// safe default here: a stream main has never seen is legitimately pinned
		// at 0, so an absent key would compare 0 == 0 and wave through exactly
		// the create-vs-create shape this guard exists for — the one case Append
		// provably cannot catch. PlanMerge always populates every replayed
		// stream; this refuses a plan from any future constructor that does not.
		planned, pinned := plan.MainStreamVersions[group.streamID]
		if !pinned {
			return fmt.Errorf(
				"%w: the merge plan for branch %s carries no pinned main version for stream %s, "+
					"so the mainline cannot be proven unchanged for it",
				ErrMergePlanIncomplete, plan.Branch.ID, group.streamID)
		}

		current, err := h.eventStore.GetStreamVersion(ctx, group.streamID, domain.MainBranchID)
		if err != nil {
			return fmt.Errorf("getting main stream version for %s: %w", group.streamID, err)
		}
		if current != planned {
			return fmt.Errorf(
				"%w: main is at version %d for stream %s but the conflict verdict was computed against version %d. "+
					"Nothing has been written and branch %s is still active — re-run GET /branches/{id}/compare "+
					"to get a fresh verdict, then merge again",
				ErrMergePlanStale, current, group.streamID, planned, plan.Branch.ID)
		}
	}
	return nil
}

// claimMerge performs the active→merged compare-and-set by appending
// BranchMerged to the branch's own stream at the version this call observed.
// The event store's per-(stream, branch) uniqueness makes that a CAS: a second
// concurrent merge observing the same version loses the append and gets
// ErrMergeAlreadyClaimed, having written nothing to main.
//
// The registry row is written by the projection, never by a direct
// BranchStore.MarkMerged call — the same rule CreateBranch and DeleteBranch
// follow, so a projection rebuild reconstructs the merge record.
//
// event is the claim itself, carrying the replay plan (see
// domain.BranchMerged.ReplayStreamVersions and ResumeMerge) and the merge
// record (#832).
func (h *Handler) claimMerge(ctx context.Context, branch *domain.Branch, event domain.BranchMerged) error {
	scope := branchScope(branch)

	currentVersion, err := h.eventStore.GetStreamVersion(ctx, branch.ID, scope.BranchID)
	if err != nil {
		return fmt.Errorf("getting branch stream version: %w", err)
	}
	// Refuse rather than fall back to -1 for an empty stream. Append skips the
	// optimistic-concurrency check entirely when expectedVersion is negative,
	// so the "new stream" sentinel would turn this CAS OFF — two concurrent
	// merges would both claim and both replay onto main. The case cannot arise:
	// the registry row we just read is written only by projectBranchCreated,
	// which projects a BranchCreated already appended to this same stream, so
	// the version is at least 1. If that ever stops holding, failing loudly
	// beats silently dropping the guarantee the whole merge rests on.
	if currentVersion == 0 {
		return fmt.Errorf(
			"branch %s has a registry row but no events on its own stream; refusing to merge without the concurrency guard",
			branch.ID)
	}

	// The registry status is NOT sufficient on its own to prove this branch is
	// unclaimed. The append below is durable before the projection that flips
	// the status runs, so a projection failure leaves a claimed branch reading
	// "active" — and a retry would then sail past MergeBranch's status guard,
	// observe the already-incremented version, append a SECOND BranchMerged,
	// and replay the whole branch onto main again. The log is the authority on
	// whether the claim landed, so ask it.
	claimed, err := h.branchAlreadyClaimed(ctx, branch)
	if err != nil {
		return err
	}
	if claimed != nil {
		// Heal the registry the failed attempt left behind — projecting the
		// event that already exists is idempotent — then refuse. Refusing
		// rather than continuing is deliberate: a fresh merge would re-plan
		// against a main the earlier attempt may already have written to.
		// Finishing that attempt is ResumeMerge's job (#685), which works from
		// the plan the claim recorded and detects what already landed.
		if err := h.projector.Project(ctx, claimed, currentVersion, scope.BranchID); err != nil {
			return fmt.Errorf("repairing branch registry after an interrupted claim: %w", err)
		}
		return fmt.Errorf("%w: %s was already claimed by an earlier attempt whose registry update did not land; "+
			"the registry has been repaired — finish that merge with POST /branches/{id}/merge/resume", ErrMergeAlreadyClaimed, branch.ID)
	}

	if err := h.eventStore.Append(ctx, branch.ID, branchStreamType, []domain.Event{event}, currentVersion, scope); err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return fmt.Errorf("%w: %s", ErrMergeAlreadyClaimed, branch.ID)
		}
		return fmt.Errorf("appending branch merged event: %w", err)
	}

	if err := h.projector.Project(ctx, event, currentVersion+1, scope.BranchID); err != nil {
		return fmt.Errorf("projecting branch merged event: %w", err)
	}
	return nil
}

// branchAlreadyClaimed reports the BranchMerged event already on a branch's own
// stream, or nil when the branch has never been claimed. It reads the log
// rather than the registry because the log is written first and is therefore
// the only place a half-completed claim is visible.
//
// The read is cheap: a branch's own stream holds only its lifecycle events
// (created, merged, deleted), never the genealogy events it produced — those
// live on their aggregates' streams.
// A nil return means unclaimed — domain.Event is an interface, so its own nil
// carries that without a pointer.
func (h *Handler) branchAlreadyClaimed(ctx context.Context, branch *domain.Branch) (domain.Event, error) {
	events, err := h.eventStore.ReadStream(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("reading branch stream: %w", err)
	}
	for _, stored := range events {
		// ReadStream spans every branch, so filter to this branch's own scope
		// before trusting the event type.
		if stored.BranchID != domain.BranchID(branch.ID) || stored.EventType != "BranchMerged" {
			continue
		}
		decoded, err := stored.DecodeEvent()
		if err != nil {
			return nil, fmt.Errorf("decoding existing branch merged event: %w", err)
		}
		return decoded, nil
	}
	return nil, nil
}

// replayOntoMain re-appends the branch's events to main, one Append per stream,
// skipping the streams resolved to main. It returns the number of events
// replayed and the streams that were skipped.
//
// This deliberately does NOT route through execute. execute applies the
// handler's own branch scope and the BR-006 branch-aware allowlist; a merge
// writes to main on behalf of a branch, so it needs neither — its scope is
// always repository.MainScope regardless of the handler's, and the allowlist
// question was already settled when the branch accepted the event.
func (h *Handler) replayOntoMain(
	ctx context.Context,
	branch *domain.Branch,
	claim domain.BranchMerged,
	groups []streamGroup,
	plannedVersions map[uuid.UUID]int64,
	resolutions map[uuid.UUID]MergeResolution,
) (int, []uuid.UUID, error) {
	var (
		replayed        int
		streamsDone     int
		totalEvents     int
		streamsToReplay int
		skipped         []uuid.UUID
	)
	provenance := mergeProvenance(branch, claim)
	// Denominators first, so the failure message below compares like with like.
	for _, group := range groups {
		if resolutions[group.streamID] == ResolveMain {
			continue
		}
		streamsToReplay++
		totalEvents += len(group.events)
	}

	for _, group := range groups {
		if resolutions[group.streamID] == ResolveMain {
			skipped = append(skipped, group.streamID)
			continue
		}

		// Same two-value read as validatePlanNotStale, for the same reason: an
		// absent pin is not a zero pin. That guard runs over this same set of
		// streams before the claim, so reaching this branch means the plan was
		// mutated underneath us — refuse rather than replay unguarded.
		plannedVersion, pinned := plannedVersions[group.streamID]
		if !pinned {
			return 0, nil, fmt.Errorf(
				"merging branch %s: stream %s: %w", branch.ID, group.streamID, ErrMergePlanIncomplete)
		}

		appended, err := h.replayStream(ctx, group, plannedVersion, provenance)
		replayed += appended
		if err != nil {
			// Both units, each against its own total: an earlier form divided an
			// event count by a stream count ("17 of 4 events"). This message is
			// the only recovery aid for the partially-applied state, so it has to
			// be readable under incident conditions — and the first thing an
			// operator needs from it is whether main was touched at all.
			if replayed == 0 {
				return 0, nil, fmt.Errorf(
					"merging branch %s: stream %s failed before any event reached main — "+
						"MAIN WAS NOT MODIFIED (0 of %d events across 0 of %d streams replayed), "+
						"but the branch is already marked merged; finish it with POST /branches/{id}/merge/resume: %w",
					branch.ID, group.streamID, totalEvents, streamsToReplay, err)
			}
			return 0, nil, fmt.Errorf(
				"merging branch %s: stream %s failed after %d of %d events across %d of %d streams reached main — "+
					"MAIN IS PARTIALLY UPDATED; finish it with POST /branches/{id}/merge/resume: %w",
				branch.ID, group.streamID, replayed, totalEvents, streamsDone, streamsToReplay, err)
		}
		streamsDone++
	}
	return replayed, skipped, nil
}

// replayStream re-appends one aggregate's branch events onto main in a single
// Append, then projects them. It reports how many events were appended even
// when projection then fails, so the caller's error can say how far the merge
// got.
//
// The originals are re-appended DECODED, never rebuilt, so a branch event
// lands on main with its original payload — its id and OccurredAt included.
// That is ADR-005's provenance requirement. Each one is stamped (#832) with
// the merge's provenance: the envelope metadata names the branch, the claim
// and the note, and the store records the event at the merge's time, so the
// mainline's history shows it when it reached the mainline. The payload is
// untouched by the stamp.
//
// plannedVersion is main's version for this stream when the plan was built, and
// is asserted against the version read below. That read is one this function
// ALREADY performs to compute the expected version, so the last-moment check
// costs nothing extra — it just stops discarding the answer.
//
// ACCEPTED FALSE POSITIVE: this fires on ANY mainline write to a replayed
// stream, including one the classifier would have judged non-conflicting (say,
// main editing a field the branch never touched). That is deliberate. The guard
// has no verdict of its own to consult — re-running conflict detection here is
// the option the issue considered and rejected, since it doubles the scan cost
// and still leaves a window — so it fails safe on movement rather than guessing.
// The cost of a false positive is one re-plan, after which conflict detection
// has run against the new main and the retry succeeds. The cost of proceeding
// silently is the overwritten mainline edit that is the bug being fixed.
func (h *Handler) replayStream(ctx context.Context, group streamGroup, plannedVersion int64, provenance *domain.MergeProvenance) (int, error) {
	events := make([]domain.Event, 0, len(group.events))
	stamped := make([]domain.Event, 0, len(group.events))
	for i := range group.events {
		decoded, err := group.events[i].DecodeEvent()
		if err != nil {
			return 0, fmt.Errorf("decoding %s event: %w", group.events[i].EventType, err)
		}
		events = append(events, decoded)
		stamped = append(stamped, domain.Stamp(decoded,
			domain.EventMetadata{MergedFromBranch: provenance}, provenance.MergedAt))
	}

	currentVersion, err := h.eventStore.GetStreamVersion(ctx, group.streamID, domain.MainBranchID)
	if err != nil {
		return 0, fmt.Errorf("getting main stream version: %w", err)
	}
	// Asserted on the true version, so a stream main gained since planning
	// (0 → 1) is caught.
	if currentVersion != plannedVersion {
		return 0, fmt.Errorf(
			"%w: main reached version %d for stream %s between the pre-merge check and this append, "+
				"but the merge plan was computed against version %d",
			ErrMergePlanStale, currentVersion, group.streamID, plannedVersion)
	}
	// The version just read is passed as-is, 0 included, NOT translated to the
	// -1 "new stream" sentinel. Every backend gates its optimistic check on
	// expectedVersion >= 0, so -1 would turn the check off for exactly the
	// streams main has never seen — and two concurrent ResumeMerge calls (#685)
	// would then both append the branch's creation onto main. 0 asserts "main
	// has no events for this stream", which is the claim being made.
	if err := h.eventStore.Append(ctx, group.streamID, group.streamType, stamped, currentVersion, repository.MainScope); err != nil {
		return 0, fmt.Errorf("appending replayed events to main: %w", err)
	}

	version := currentVersion
	for _, event := range events {
		version++
		if err := h.projector.Project(ctx, event, version, domain.MainBranchID); err != nil {
			return len(events), fmt.Errorf("projecting replayed %s onto main: %w", event.EventType(), err)
		}
	}
	return len(events), nil
}

// replayPlan is the replay plan a claim records: every stream the merge will
// replay, mapped to its pinned main version. Streams resolved to main are left
// out, which is how a resume knows not to replay them. validatePlanNotStale has
// already refused a plan missing a pin for any of these streams, so every entry
// is a real pinned version, never a defaulted zero.
func replayPlan(groups []streamGroup, pinned map[uuid.UUID]int64, resolutions map[uuid.UUID]MergeResolution) map[uuid.UUID]int64 {
	plan := make(map[uuid.UUID]int64, len(groups))
	for _, group := range groups {
		if resolutions[group.streamID] == ResolveMain {
			continue
		}
		plan[group.streamID] = pinned[group.streamID]
	}
	return plan
}

// streamGroup is one aggregate's slice of the replay set, in position order.
type streamGroup struct {
	streamID   uuid.UUID
	streamType string
	events     []repository.StoredEvent
}

// groupEventsByStream partitions the replay set by aggregate, preserving
// position order within each stream and returning the streams in the order the
// branch first touched them. Grouping is what lets the replay issue one Append
// per aggregate instead of one per event.
func groupEventsByStream(events []repository.StoredEvent) []streamGroup {
	index := make(map[uuid.UUID]int, len(events))
	groups := make([]streamGroup, 0, len(events))
	for _, evt := range events {
		at, seen := index[evt.StreamID]
		if !seen {
			index[evt.StreamID] = len(groups)
			groups = append(groups, streamGroup{streamID: evt.StreamID, streamType: evt.StreamType})
			at = len(groups) - 1
		}
		groups[at].events = append(groups[at].events, evt)
	}
	return groups
}

// validateNoDanglingReferences refuses a merge whose replay would leave main
// holding a relationship pointing at a person that will not exist there.
//
// Resolutions are per-aggregate, but the branch's events reference each other
// ACROSS aggregates: ChildLinkedToFamily lives on the family's stream and names
// a person on another, and AssociationCreated (#757) lives on the
// association's stream and names two. So excluding a person — by resolving
// their stream to main, which is the ONLY resolution offered when main is the
// deleter — does not exclude the event that references them. Replayed on its
// own, that event writes a row for a person main does not have: the
// projections save the row unconditionally (internal/repository/projection.go,
// projectChildLinked and projectAssociationCreated read the person only to
// denormalize a name), and the branch-scoping work dropped the FK cascade that
// would once have caught it. The result is a "successful" 200 leaving main
// with a blank-named phantom child or association, reported nowhere —
// skipped_stream_ids names the person, never the stream still pointing at them.
//
// A person is fine if main already has them or the replay is about to create
// them — the replay carries a group that creates the person (see
// createsPerson). Merely replaying the person's stream is not enough: a
// stream of edits does not bring back a person main no longer has. main can
// remove a person without writing to their stream (PersonMerged lands on the
// survivor's), so an edit-only stream for a merged-away person is not even a
// conflict, yet replaying it restores nothing. Anything else is refused,
// rather than silently dropping the reference: dropping is the same
// silent-discard class of bug that per-conflict resolution exists to prevent.
//
// Only events that ADD a reference are checked. Unlinking a person main does
// not have removes nothing and is harmless.
//
// Every breach is reported, not just the first (#831): the refusal is a
// *MergeBlockedError carrying one named MergeBlocker per breach, each with the
// one-step fix that clears it.
func (h *Handler) validateNoDanglingReferences(ctx context.Context, plan *query.MergePlan, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution) error {
	blockers, err := h.collectMergeBlockers(ctx, plan, groups, resolutions)
	if err != nil {
		return err
	}
	if len(blockers) == 0 {
		return nil
	}
	return &MergeBlockedError{Blockers: blockers}
}

// collectMergeBlockers runs every cross-entity reference rule over the replay
// the resolutions describe and returns the named blockers, each with its
// suggested fix. It writes nothing, which is what lets PrecheckMerge run it
// ahead of the merge and get the answer the merge itself would.
//
// The scan and every trial run suggestMergeFixes makes share one memo of
// main-side lookups (forBlockerScan), so a lookup is paid once per scan
// however many candidate fixes re-run the rules.
func (h *Handler) collectMergeBlockers(ctx context.Context, plan *query.MergePlan, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution) ([]MergeBlocker, error) {
	scan := h.forBlockerScan()
	blockers, err := scan.findMergeBlockers(ctx, plan, groups, resolutions)
	if err != nil || len(blockers) == 0 {
		return nil, err
	}
	if err := scan.suggestMergeFixes(ctx, plan, groups, resolutions, blockers); err != nil {
		return nil, err
	}
	if err := h.nameBlockers(ctx, plan.Branch.ID, blockers); err != nil {
		return nil, err
	}
	return blockers, nil
}

// findMergeBlockers runs every cross-entity reference rule over the replay the
// resolutions describe and returns the blockers, unnamed and with the default
// fix.
func (h *Handler) findMergeBlockers(ctx context.Context, plan *query.MergePlan, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution) ([]MergeBlocker, error) {
	created := make(map[uuid.UUID]bool, len(groups))
	checkedGroups := make([]streamGroup, 0, len(groups))
	for _, group := range groups {
		if resolutions[group.streamID] == ResolveMain {
			continue
		}
		checkedGroups = append(checkedGroups, group)
		if createsPerson(group) {
			created[group.streamID] = true
		}
	}

	dangling, err := h.findDanglingReferences(ctx, checkedGroups, func(personID uuid.UUID) bool { return created[personID] })
	if err != nil {
		return nil, err
	}
	var list blockerList
	for _, d := range dangling {
		list.add(d.blocker())
	}
	if err := h.collectEvidenceBlockers(ctx, plan, groups, resolutions, &list); err != nil {
		return nil, err
	}
	return list.items, nil
}

// createsPerson reports whether a replay group leaves its person in existence
// on main by itself: it is a person stream that creates the person and does
// not end by deleting them.
func createsPerson(group streamGroup) bool {
	if !isPersonStream(group.streamType) || endsInDelete(group.events) {
		return false
	}
	for i := range group.events {
		if group.events[i].EventType == "PersonCreated" {
			return true
		}
	}
	return false
}

// danglingReference is one branch stream whose replay would point main at a
// person main will not have.
type danglingReference struct {
	group    streamGroup
	personID uuid.UUID
}

// blocker is the reference as a merge blocker.
func (d danglingReference) blocker() MergeBlocker {
	return streamBlocker(d.group, BlockerMissingPerson, d.personID, "person",
		"the branch's stream %s (a family partner or child link, or an association) references person %s, "+
			"but that person will not exist on main (deleted or merged away there, or excluded by a \"main\" resolution)",
		d.group.streamID, d.personID)
}

// findDanglingReferences reports every (group, person) pair where one of the
// group's events references (see personReferences) a person who is neither
// vouched for by present nor on main's read model. Pairs are reported in group
// order, each at most once. Each person is looked up on main at most once.
func (h *Handler) findDanglingReferences(ctx context.Context, groups []streamGroup, present func(uuid.UUID) bool) ([]danglingReference, error) {
	onMain := make(map[uuid.UUID]bool)
	var dangling []danglingReference
	for _, group := range groups {
		reported := make(map[uuid.UUID]bool)
		for i := range group.events {
			personIDs, err := personReferences(group.events[i])
			if err != nil {
				return nil, err
			}
			for _, personID := range personIDs {
				if present(personID) {
					continue
				}
				found, looked := onMain[personID]
				if !looked {
					person, err := h.readStore.GetPerson(ctx, domain.MainBranchID, personID)
					if err != nil {
						return nil, fmt.Errorf("checking person %s on main: %w", personID, err)
					}
					found = person != nil
					onMain[personID] = found
				}
				if !found && !reported[personID] {
					reported[personID] = true
					dangling = append(dangling, danglingReference{group: group, personID: personID})
				}
			}
		}
	}
	return dangling, nil
}

// personReferences returns the persons a branch event makes main point at: the
// partners a FamilyCreated names, a partner a FamilyUpdated sets, the child of
// a ChildLinkedToFamily, and both sides of an AssociationCreated (an
// association's persons are fixed at creation; AssociationUpdated cannot
// change them). A FamilyUpdated that clears a partner references no one.
func personReferences(evt repository.StoredEvent) ([]uuid.UUID, error) {
	switch evt.EventType {
	case "FamilyCreated":
		var payload struct {
			Partner1ID *uuid.UUID `json:"partner1_id"`
			Partner2ID *uuid.UUID `json:"partner2_id"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return nil, fmt.Errorf("decoding family on stream %s: %w", evt.StreamID, err)
		}
		return nonNilPersons(payload.Partner1ID, payload.Partner2ID), nil
	case "FamilyUpdated":
		var payload struct {
			Changes map[string]json.RawMessage `json:"changes"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return nil, fmt.Errorf("decoding family update on stream %s: %w", evt.StreamID, err)
		}
		var ids []uuid.UUID
		for _, key := range []string{"partner1_id", "partner2_id"} {
			if id := partnerChange(payload.Changes[key]); id != nil {
				ids = append(ids, *id)
			}
		}
		return ids, nil
	case "ChildLinkedToFamily":
		var payload struct {
			PersonID uuid.UUID `json:"person_id"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return nil, fmt.Errorf("decoding child link on stream %s: %w", evt.StreamID, err)
		}
		return []uuid.UUID{payload.PersonID}, nil
	case "AssociationCreated":
		var payload struct {
			PersonID    uuid.UUID `json:"person_id"`
			AssociateID uuid.UUID `json:"associate_id"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return nil, fmt.Errorf("decoding association on stream %s: %w", evt.StreamID, err)
		}
		return []uuid.UUID{payload.PersonID, payload.AssociateID}, nil
	}
	return nil, nil
}

// partnerChange decodes one partner entry of a FamilyUpdated's changes the
// way the projection reads it (resolvePartnerChange in
// internal/repository/projection.go): a UUID string sets that person; an
// absent entry, null, or any value the projection would store as "no partner"
// references no one.
func partnerChange(raw json.RawMessage) *uuid.UUID {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil || s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil // the projection clears the partner for this value too
	}
	return &id
}

// nonNilPersons returns the set ids among ids, in order.
func nonNilPersons(ids ...*uuid.UUID) []uuid.UUID {
	var out []uuid.UUID
	for _, id := range ids {
		if id != nil {
			out = append(out, *id)
		}
	}
	return out
}

// validateResolutions rejects resolutions the merge cannot honor: one naming a
// stream the branch never changed (the caller is talking about a different
// merge than the server is), and one carrying an unrecognized value.
//
// Keys are checked in sorted order so a request with several bad entries always
// reports the same one.
func validateResolutions(resolutions map[uuid.UUID]MergeResolution, groups []streamGroup) error {
	if len(resolutions) == 0 {
		return nil
	}

	touched := make(map[uuid.UUID]bool, len(groups))
	for _, group := range groups {
		touched[group.streamID] = true
	}

	streamIDs := make([]uuid.UUID, 0, len(resolutions))
	for streamID := range resolutions {
		streamIDs = append(streamIDs, streamID)
	}
	sort.Slice(streamIDs, func(i, j int) bool { return streamIDs[i].String() < streamIDs[j].String() })

	for _, streamID := range streamIDs {
		if !touched[streamID] {
			return fmt.Errorf("%w: the branch never changed stream %s", ErrUnknownResolution, streamID)
		}
		switch resolutions[streamID] {
		case ResolveBranch, ResolveMain:
		default:
			return fmt.Errorf("%w: %q for stream %s", ErrUnknownResolution, resolutions[streamID], streamID)
		}
	}
	return nil
}

// validateRationales checks the optional per-resolution rationales (#828): each
// must accompany a resolution for the same stream and fit the domain's length
// rule. It returns them trimmed, blank ones dropped, nil when none remain.
func validateRationales(rationales map[uuid.UUID]string, resolutions map[uuid.UUID]MergeResolution) (map[uuid.UUID]string, error) {
	for streamID := range rationales {
		if _, ok := resolutions[streamID]; !ok {
			return nil, fmt.Errorf("%w: a rationale was given for stream %s, which has no resolution", ErrUnknownResolution, streamID)
		}
	}
	return domain.NormalizeResolutionRationales(rationales)
}

// validateConflictResolutions rejects a resolution that is a legal value but
// would not do what it says for that particular conflict.
//
// Two shapes cannot honor "branch" (see
// query.MergeConflict.SupportedResolutions): a main-side delete, where
// replaying the branch's edits onto an absent read-model row is a no-op, and a
// create_create, where the two sides are different streams so promoting the
// branch's adds a duplicate rather than resolving anything. Both would
// otherwise return 200 having produced the opposite of the caller's decision.
//
// Conflicts are checked in the order the classifier reported them, which is the
// branch's first-touch stream order, so the message is stable across runs.
func validateConflictResolutions(conflicts []query.MergeConflict, resolutions map[uuid.UUID]MergeResolution) error {
	for _, conflict := range conflicts {
		chosen, decided := resolutions[conflict.StreamID]
		if !decided {
			// Undecided is unresolvedConflicts' business, not this check's.
			continue
		}
		if slices.Contains(conflict.SupportedResolutions, string(chosen)) {
			continue
		}
		return fmt.Errorf("%w: %q for the %s conflict on stream %s; available: %s",
			ErrUnsupportedResolution, chosen, conflict.Kind, conflict.StreamID,
			strings.Join(conflict.SupportedResolutions, ", "))
	}
	return nil
}

// unresolvedConflicts counts the conflicts the caller has not decided. Presence
// is the test, not the value — validateResolutions has already rejected values
// that are not "branch" or "main", and validateConflictResolutions has rejected
// values a given conflict cannot honor.
func unresolvedConflicts(conflicts []query.MergeConflict, resolutions map[uuid.UUID]MergeResolution) int {
	var unresolved int
	for _, conflict := range conflicts {
		if _, decided := resolutions[conflict.StreamID]; !decided {
			unresolved++
		}
	}
	return unresolved
}
