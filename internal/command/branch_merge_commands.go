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

	// ErrMergeDanglingReference is returned when the replay would leave main
	// holding a relationship that points at a person main will not have —
	// typically because that person was deleted on main, which forces their
	// stream to a "main" resolution while the family event linking them lives
	// on a different stream and would still be replayed. Refused rather than
	// silently dropping the link (see validateNoDanglingReferences).
	ErrMergeDanglingReference = errors.New("merge would leave a relationship pointing at a person main does not have")

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
	// generic 500 is the honest answer. Anticipated trigger is a second plan
	// constructor, e.g. the stored/replayed plan #685 needs.
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
	// the branch is terminal and there is no resume path; resumable merge is
	// tracked as #685.
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
}

// MergeBranch replays a branch's genealogy-mutation events onto main and marks
// the branch merged (issue #55, invariant BR-004).
//
// The order below is load-bearing:
//
//  1. Guards, then the merge plan. A truncated plan is refused outright.
//  2. Conflicts must all be resolved before anything is written.
//  3. The STALENESS CHECK: main must still sit at the versions the plan was
//     computed against, or the verdict from step 1 no longer describes main and
//     the merge is refused (ErrMergePlanStale) with nothing written.
//  4. The CLAIM: BranchMerged is appended to the branch's OWN stream at the
//     version this call observed. Per-(stream, branch) optimistic concurrency
//     makes that append the atomic compare-and-set ADR-005 asks for — exactly
//     one of two concurrent merges wins it, and the loser has not yet touched
//     main.
//  5. Only then the replay onto main, which re-asserts the same planned
//     versions per stream as it goes.
//
// Step 3 MUST precede step 4. claimMerge marks the branch terminal, so a
// staleness refusal after it would leave the branch merged with nothing
// replayed and no way to retry — the retry would hit the status guard and get
// 409 branch_not_active, which is not evidence of anything. Checking first
// keeps "stale" in the same class as every other refusal: nothing written,
// branch still active, re-plan and try again.
//
// RESIDUAL WINDOW: steps 3 and 5 close the gap #698 described but cannot make it
// zero. Between the check in step 3 and each stream's append in step 5 there is
// still no lock, so a mainline write can still land — step 5's per-stream
// assertion catches it, but by then the branch is claimed, so it surfaces as the
// partially-applied state below rather than as a clean refusal. Shrinking that
// last window needs the transaction the codebase does not have; a merge-wide
// lock is not an option, as it contradicts ADR-005's per-(stream, branch)
// design.
//
// KNOWN LIMITATION: steps 4 and 5 are not one transaction. The codebase has no
// cross-store transaction facility and ADR-003's synchronous projections are
// per-append, so a failure mid-replay leaves the branch merged with main
// partially updated — or, when the FIRST stream fails, with main untouched. The
// returned error names the stream that failed, how many events had already been
// replayed, and which of those two states this is, so it is diagnosable without
// counting. Resumable merge is deliberate follow-up work (#685), not an
// oversight. Replaying one Append per
// stream (rather than per event) keeps the failure granularity at whole-entity,
// since the SQL backends wrap an Append in a transaction.
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
	// The two truncation sides are different problems and get different
	// answers. A branch bigger than the cap is permanently unmergeable as-is;
	// a main tail bigger than the cap says nothing about the branch, grows with
	// unrelated mainline activity, and is not the branch's fault.
	if plan.BranchTruncated {
		return nil, fmt.Errorf(
			"%w: branch %s has more than %d events of its own, so its replay set is incomplete. "+
				"Retrying will not help — the cap is fixed and the branch does not shrink; "+
				"promoting a subset needs partial merge (#684)",
			ErrBranchTooLargeToMerge, branch.ID, plan.EventCap)
	}
	if plan.MainTruncated {
		return nil, fmt.Errorf(
			"%w: more than %d events have landed on main for the streams branch %s touches since it forked, "+
				"so the conflict list is not known to be complete. The branch itself may be small — this is a "+
				"limit on how far back the comparison scans, not on the branch",
			ErrMainTooFarAheadToMerge, plan.EventCap, branch.ID)
	}

	groups := groupEventsByStream(plan.ReplayEvents)
	if err := validateResolutions(input.Resolutions, groups); err != nil {
		return nil, err
	}
	if err := validateConflictResolutions(plan.Conflicts, input.Resolutions); err != nil {
		return nil, err
	}
	if err := h.validateNoDanglingReferences(ctx, groups, input.Resolutions); err != nil {
		return nil, err
	}
	if unresolved := unresolvedConflicts(plan.Conflicts, input.Resolutions); unresolved > 0 {
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

	// Deliberately the LAST thing before the claim: every read above can only
	// widen the window between this check and the replay, so the check goes as
	// late as it can while still being a refusal rather than a half-merge.
	if err := h.validatePlanNotStale(ctx, plan, groups, input.Resolutions); err != nil {
		return nil, err
	}

	if err := h.claimMerge(ctx, branch, mergedAtPosition, input.Note); err != nil {
		return nil, err
	}

	// Past this point the branch is already merged, so a replay failure is not
	// "nothing happened" — it is the partially-applied state, and the caller
	// has to be able to tell the two apart.
	replayed, skipped, err := h.replayOntoMain(ctx, branch, groups, plan.MainStreamVersions, input.Resolutions)
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
	}, nil
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
// — so the -1 a branch-created stream is appended with turns the check OFF
// entirely rather than asserting "no prior events". Leaning on Append would
// therefore leave exactly the case where main GAINS a stream the branch also
// created — the create-vs-create shape — completely unguarded.
//
// Scoped to the streams that will actually be replayed. A stream resolved to
// main is not written, so main moving under it changes nothing this merge does,
// and refusing on it would fail merges for no reason. Streams the branch never
// touched are not in the plan at all, for the same reason ADR-005 scopes the
// whole comparison that way: a merge must not be hostage to unrelated mainline
// activity.
//
// A stream main has never seen is planned at 0 and still reads 0, so it does not
// trip the guard — that is the whole point of keeping the 0 → -1 translation out
// of the plan and down in replayStream.
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
		// stream; this refuses a plan from any future constructor that does not
		// (#685's stored plans).
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
func (h *Handler) claimMerge(ctx context.Context, branch *domain.Branch, mergedAtPosition int64, note string) error {
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
		// rather than continuing is deliberate: from here we cannot tell
		// whether the earlier attempt's replay ran, so resuming it risks
		// duplicating main's history. GET /branches/{id}/compare reports what
		// actually landed (#685 tracks resuming it properly).
		if err := h.projector.Project(ctx, claimed, currentVersion, scope.BranchID); err != nil {
			return fmt.Errorf("repairing branch registry after an interrupted claim: %w", err)
		}
		return fmt.Errorf("%w: %s was already claimed by an earlier attempt whose registry update did not land; "+
			"the registry has been repaired — verify with compare before retrying", ErrMergeAlreadyClaimed, branch.ID)
	}

	event := domain.NewBranchMerged(branch.ID, branch.BasePosition, mergedAtPosition, note)
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

		appended, err := h.replayStream(ctx, group, plannedVersion)
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
						"but the branch is already marked merged, so there is nothing to unwind on main "+
						"and no resume path yet (#685): %w",
					branch.ID, group.streamID, totalEvents, streamsToReplay, err)
			}
			return 0, nil, fmt.Errorf(
				"merging branch %s: stream %s failed after %d of %d events across %d of %d streams reached main — "+
					"MAIN IS PARTIALLY UPDATED: %w",
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
// The originals are re-appended DECODED, never rebuilt: EventStore.Append
// stamps the stored timestamp from event.OccurredAt(), so a decoded branch
// event lands on main with the branch's original payload and timestamp. That is
// ADR-005's provenance requirement, met with no special handling.
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
func (h *Handler) replayStream(ctx context.Context, group streamGroup, plannedVersion int64) (int, error) {
	events := make([]domain.Event, 0, len(group.events))
	for i := range group.events {
		decoded, err := group.events[i].DecodeEvent()
		if err != nil {
			return 0, fmt.Errorf("decoding %s event: %w", group.events[i].EventType, err)
		}
		events = append(events, decoded)
	}

	currentVersion, err := h.eventStore.GetStreamVersion(ctx, group.streamID, domain.MainBranchID)
	if err != nil {
		return 0, fmt.Errorf("getting main stream version: %w", err)
	}
	// Asserted BEFORE the 0 → -1 translation below, on the true version, so a
	// stream main gained since planning (0 → 1) is caught rather than erased by
	// the sentinel.
	if currentVersion != plannedVersion {
		return 0, fmt.Errorf(
			"%w: main reached version %d for stream %s between the pre-merge check and this append, "+
				"but the merge plan was computed against version %d",
			ErrMergePlanStale, currentVersion, group.streamID, plannedVersion)
	}
	// Same 0 → -1 convention as everywhere else: main has no events for a
	// stream the branch created, so the append must claim a new stream.
	expectedVersion := currentVersion
	if currentVersion == 0 {
		expectedVersion = -1
	}

	if err := h.eventStore.Append(ctx, group.streamID, group.streamType, events, expectedVersion, repository.MainScope); err != nil {
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
// a person on another. So excluding a person — by resolving their stream to
// main, which is the ONLY resolution offered when main is the deleter — does
// not exclude the family event that links them. Replayed on its own, that event
// writes a family_children row for a person main does not have: the projection
// saves the row unconditionally (internal/repository/projection.go,
// projectChildLinked reads the person only to denormalize a name), and the
// branch-scoping work dropped the FK cascade that would once have caught it.
// The result is a "successful" 200 leaving main with a blank-named phantom
// child, reported nowhere — skipped_stream_ids names the person, never the
// family still pointing at them.
//
// A person is fine if main already has them or the replay is about to create
// them. Anything else is refused, rather than silently dropping the link:
// dropping is the same silent-discard class of bug that per-conflict resolution
// exists to prevent.
//
// Only link events are checked. Unlinking a person main does not have removes
// nothing and is harmless.
func (h *Handler) validateNoDanglingReferences(ctx context.Context, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution) error {
	replayed := make(map[uuid.UUID]bool, len(groups))
	for _, group := range groups {
		if resolutions[group.streamID] != ResolveMain {
			replayed[group.streamID] = true
		}
	}

	checked := make(map[uuid.UUID]bool)
	for _, group := range groups {
		if resolutions[group.streamID] == ResolveMain {
			continue
		}
		for _, evt := range group.events {
			if evt.EventType != "ChildLinkedToFamily" {
				continue
			}
			var payload struct {
				PersonID uuid.UUID `json:"person_id"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("decoding child link on stream %s: %w", group.streamID, err)
			}
			// The replay will create or update this person, so the link lands
			// on something real.
			if replayed[payload.PersonID] || checked[payload.PersonID] {
				continue
			}
			person, err := h.readStore.GetPerson(ctx, domain.MainBranchID, payload.PersonID)
			if err != nil {
				return fmt.Errorf("checking person %s on main: %w", payload.PersonID, err)
			}
			if person == nil {
				return fmt.Errorf(
					"%w: the branch links person %s into family %s, but that person will not exist on main "+
						"(deleted there, or excluded by a \"main\" resolution)",
					ErrMergeDanglingReference, payload.PersonID, group.streamID)
			}
			checked[payload.PersonID] = true
		}
	}
	return nil
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
