package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Merge resume errors (#685).
var (
	// ErrMergeNotClaimed is returned when ResumeMerge is asked to finish a merge
	// that was never started: the branch's own stream holds no BranchMerged
	// claim, so there is nothing to resume. Merge it with MergeBranch instead.
	// Also returned for an archived branch, which is discarded and is not merged
	// whatever its log says.
	ErrMergeNotClaimed = errors.New("branch has no claimed merge to resume")

	// ErrMergeResumeNeedsResolution is returned when at least one stream still to
	// be replayed cannot be replayed on the strength of the claim alone, and the
	// caller has not said what to do with it. NOTHING HAS BEEN WRITTEN by the
	// refusing call. Two shapes get here:
	//
	//   - main moved on the stream after the claim pinned it. The claim's
	//     conflict verdict no longer describes main there, so replaying over it
	//     would be the silent override #698 closed for the first attempt.
	//   - the claim predates #685 and recorded no replay plan, so neither the
	//     stream's resolution nor its pin is known.
	//
	// The streams are listed on ResumeMergeResult.PendingStreamIDs. Inspect them
	// with GET /branches/{id}/compare, then resume again with a resolution for
	// each: "branch" replays the branch's events over main as it now stands,
	// "main" leaves the entity as main has it.
	ErrMergeResumeNeedsResolution = errors.New("merge resume needs a resolution for streams whose replay the claim cannot vouch for")
)

// resumeScanPage is the page size of the scan that finds which of the branch's
// events already reached main. Paged rather than capped: under-reading would
// report a landed stream as unreplayed and replay it twice, the one outcome a
// resume exists to prevent.
const resumeScanPage = 1000

// ResumeMergeInput asks ResumeMerge to finish an interrupted merge.
type ResumeMergeInput struct {
	BranchID uuid.UUID

	// Resolutions decides the streams the claim cannot vouch for (see
	// ErrMergeResumeNeedsResolution). It may name ONLY those streams: every
	// other stream's fate was fixed when the merge was claimed, and a resume
	// that could re-decide it would let a second request quietly rewrite what
	// the first one reviewed.
	Resolutions map[uuid.UUID]MergeResolution
}

// ResumeMergeResult reports what a resume did — or, alongside
// ErrMergeResumeNeedsResolution, which streams need a decision.
type ResumeMergeResult struct {
	// Branch is the branch re-read after the resume.
	Branch *domain.Branch

	// MergedAtPosition is the log head the original merge recorded on its claim.
	MergedAtPosition int64

	// ReplayedEventCount is how many branch events THIS call re-appended to
	// main. Zero on a resume of an already-complete merge, which is how a
	// caller can see it was a no-op.
	ReplayedEventCount int

	// AlreadyReplayedStreamIDs are the streams whose branch events were
	// already on main when this call started, so it left them alone.
	AlreadyReplayedStreamIDs []uuid.UUID

	// SkippedStreamIDs are the streams resolved to main — at claim time, or by
	// this call's Resolutions — whose branch events are deliberately not
	// replayed.
	SkippedStreamIDs []uuid.UUID

	// PendingStreamIDs is populated only alongside
	// ErrMergeResumeNeedsResolution: the streams that need a resolution.
	PendingStreamIDs []uuid.UUID
}

// resumeStep is what a resume will do with one stream of the replay set.
type resumeStep struct {
	group   streamGroup
	planned int64 // the main version the append asserts
}

// ResumeMerge finishes a merge whose replay onto main was interrupted after
// the branch was claimed (issue #685; ADR-005 §Merge implementation note).
//
// It never rewrites history (ES-002): it only appends the branch events that
// are not yet on main, exactly as MergeBranch would have.
//
// What already landed is DERIVED FROM MAIN, not remembered. Replay re-appends
// each branch event decoded, so the event's own payload id travels with it onto
// main; a branch event whose payload id is already among main's events on that
// stream after the claim position has been replayed. Because replay issues one
// Append per stream and every backend applies an Append atomically, a stream is
// either wholly on main or not there at all — a stream found half-there is an
// invariant breach and is refused rather than guessed at.
//
// What is still to do comes from the CLAIM. MergeBranch records its replay plan
// on the BranchMerged event (domain.BranchMerged.ReplayStreamVersions): the
// streams to replay and main's version of each when the conflict verdict was
// computed. A remaining stream is replayed automatically only when main still
// sits at that pinned version — the same staleness guarantee the first attempt
// ran under. Anything else needs the caller's resolution
// (ErrMergeResumeNeedsResolution), checked before anything is written.
//
// Resume is idempotent: once every stream is on main (or skipped), a further
// call appends nothing and reports ReplayedEventCount 0. Two concurrent resumes
// cannot both append a stream: each asserts the stream's main version on
// Append, so the loser gets a concurrency conflict (reported as
// ErrMergePartiallyApplied) and its retry finds the stream already replayed.
//
// A replay failure DURING a resume is reported exactly like one during a merge
// (ErrMergePartiallyApplied), and the remedy is the same: resume again.
//
// Read-model scope: resume completes the EVENT LOG, the source of truth. If an
// earlier attempt's Append landed but its synchronous projection then failed,
// the stream counts as replayed here and its read-model rows are repaired by a
// projection rebuild, as for any other ADR-003 projection failure — replaying
// the events a second time to re-drive the projection would duplicate them.
func (h *Handler) ResumeMerge(ctx context.Context, input ResumeMergeInput) (*ResumeMergeResult, error) {
	if h.branchStore == nil {
		return nil, ErrBranchStoreRequired
	}

	branch, err := h.branchStore.Get(ctx, input.BranchID)
	if err != nil {
		return nil, err // includes repository.ErrBranchNotFound
	}
	claim, err := h.resumableClaim(ctx, branch)
	if err != nil {
		return nil, err
	}

	replaySet, err := h.branchService.LoadMergeReplaySet(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("loading merge replay set: %w", err)
	}
	if replaySet.Truncated {
		// MergeBranch refuses a truncated branch before claiming it, so a claimed
		// branch cannot be over the cap unless the cap shrank. Resuming from a
		// partial set would report success over a half-promoted branch.
		return nil, fmt.Errorf("%w: branch %s has more than %d events of its own, so the replay set to resume is incomplete",
			ErrBranchTooLargeToMerge, branch.ID, replaySet.EventCap)
	}

	// The claim fixed the replay set. MergedAtPosition is the log head read just
	// before the claim was appended, so a branch event after it was written
	// after (or racing) the claim — possible only while a failed claim
	// projection left the registry reading "active". The claim's plan never
	// considered such an event, so replaying or skipping it would both be
	// guesses.
	for i := range replaySet.ReplayEvents {
		if evt := replaySet.ReplayEvents[i]; evt.Position > claim.MergedAtPosition {
			return nil, fmt.Errorf(
				"branch %s gained a %s event on stream %s (position %d) after its merge was claimed at position %d; "+
					"the claim's plan does not cover it, so the merge cannot be resumed automatically",
				branch.ID, evt.EventType, evt.StreamID, evt.Position, claim.MergedAtPosition)
		}
	}

	groups := groupEventsByStream(replaySet.ReplayEvents)
	if err := validateResolutions(input.Resolutions, groups); err != nil {
		return nil, err
	}

	landed, err := h.streamsAlreadyOnMain(ctx, groups, claim.MergedAtPosition)
	if err != nil {
		return nil, err
	}

	result := &ResumeMergeResult{MergedAtPosition: claim.MergedAtPosition}
	steps, err := h.planResume(ctx, claim, groups, landed, input.Resolutions, result)
	if err != nil {
		return nil, err
	}
	if len(result.PendingStreamIDs) > 0 {
		result.Branch = branch
		return result, fmt.Errorf(
			"%w: %d stream(s) still to replay for branch %s cannot be replayed on the claim's plan "+
				"(main moved on them since the claim, or the claim recorded no plan). Nothing has been written; "+
				"review them with GET /branches/{id}/compare and resume again with a resolution for each: %v",
			ErrMergeResumeNeedsResolution, len(result.PendingStreamIDs), branch.ID, result.PendingStreamIDs)
	}

	// Same cross-entity check the merge ran before its claim: a resolution
	// made here can exclude a person a replayed family event still links.
	if err := h.validateNoDanglingReferences(ctx, stepGroups(steps), nil); err != nil {
		return nil, err
	}

	for i, step := range steps {
		appended, err := h.replayStream(ctx, step.group, step.planned)
		result.ReplayedEventCount += appended
		if err != nil {
			return nil, fmt.Errorf(
				"%w: resuming merge of branch %s: stream %s failed after %d of %d remaining stream(s) "+
					"(%d event(s)) reached main in this attempt; resume again to finish: %w",
				ErrMergePartiallyApplied, branch.ID, step.group.streamID, i, len(steps), result.ReplayedEventCount, err)
		}
	}

	merged, err := h.branchStore.Get(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("re-reading merged branch: %w", err)
	}
	result.Branch = merged
	return result, nil
}

// resumableClaim returns the branch's BranchMerged claim, refusing a branch
// that has none. A claim whose registry projection never landed (the branch
// still reads "active") is repaired first, exactly as claimMerge repairs it —
// resuming is the natural next step from that state too.
func (h *Handler) resumableClaim(ctx context.Context, branch *domain.Branch) (domain.BranchMerged, error) {
	if branch.Status == domain.BranchStatusArchived {
		return domain.BranchMerged{}, fmt.Errorf("%w: branch %s is archived", ErrMergeNotClaimed, branch.ID)
	}

	claimed, err := h.branchAlreadyClaimed(ctx, branch)
	if err != nil {
		return domain.BranchMerged{}, err
	}
	if claimed == nil {
		return domain.BranchMerged{}, fmt.Errorf("%w: branch %s is %s and was never claimed for a merge; merge it instead",
			ErrMergeNotClaimed, branch.ID, branch.Status)
	}
	claim, ok := claimed.(domain.BranchMerged)
	if !ok {
		return domain.BranchMerged{}, fmt.Errorf("branch %s: merge claim decoded as %T, want BranchMerged", branch.ID, claimed)
	}

	if branch.Status != domain.BranchStatusMerged {
		scope := branchScope(branch)
		version, err := h.eventStore.GetStreamVersion(ctx, branch.ID, scope.BranchID)
		if err != nil {
			return domain.BranchMerged{}, fmt.Errorf("getting branch stream version: %w", err)
		}
		if err := h.projector.Project(ctx, claim, version, scope.BranchID); err != nil {
			return domain.BranchMerged{}, fmt.Errorf("repairing branch registry after an interrupted claim: %w", err)
		}
	}
	return claim, nil
}

// planResume decides, per stream, what a resume does, filling the result's
// already-replayed, skipped and pending lists. It writes nothing: every refusal
// it can produce happens before the first append.
func (h *Handler) planResume(
	ctx context.Context,
	claim domain.BranchMerged,
	groups []streamGroup,
	landed map[uuid.UUID]bool,
	resolutions map[uuid.UUID]MergeResolution,
	result *ResumeMergeResult,
) ([]resumeStep, error) {
	plan := claim.ReplayStreamVersions // nil: a pre-#685 claim with no recorded plan
	if plan != nil {
		inReplaySet := make(map[uuid.UUID]bool, len(groups))
		for _, group := range groups {
			inReplaySet[group.streamID] = true
		}
		for streamID := range plan {
			if !inReplaySet[streamID] {
				return nil, fmt.Errorf("branch %s: the merge claim plans a replay of stream %s, which the branch never changed",
					claim.BranchID, streamID)
			}
		}
	}

	var steps []resumeStep
	decidable := make(map[uuid.UUID]bool)
	for _, group := range groups {
		if landed[group.streamID] {
			result.AlreadyReplayedStreamIDs = append(result.AlreadyReplayedStreamIDs, group.streamID)
			continue
		}

		pinned, planned := plan[group.streamID]
		if plan != nil && !planned {
			// Resolved to main when the merge was claimed.
			result.SkippedStreamIDs = append(result.SkippedStreamIDs, group.streamID)
			continue
		}

		current, err := h.eventStore.GetStreamVersion(ctx, group.streamID, domain.MainBranchID)
		if err != nil {
			return nil, fmt.Errorf("getting main stream version for %s: %w", group.streamID, err)
		}
		if planned && current == pinned {
			steps = append(steps, resumeStep{group: group, planned: pinned})
			continue
		}

		// The claim cannot vouch for this stream; only the caller can.
		decidable[group.streamID] = true
		switch resolutions[group.streamID] {
		case ResolveBranch:
			steps = append(steps, resumeStep{group: group, planned: current})
		case ResolveMain:
			result.SkippedStreamIDs = append(result.SkippedStreamIDs, group.streamID)
		default:
			result.PendingStreamIDs = append(result.PendingStreamIDs, group.streamID)
		}
	}

	// A resolution for a stream the claim already decided is refused rather
	// than ignored: the caller believes they are choosing something they are
	// not. Sorted so a request with several bad entries reports the same one.
	undecidable := make([]uuid.UUID, 0, len(resolutions))
	for streamID := range resolutions {
		if !decidable[streamID] {
			undecidable = append(undecidable, streamID)
		}
	}
	if len(undecidable) > 0 {
		sort.Slice(undecidable, func(i, j int) bool { return undecidable[i].String() < undecidable[j].String() })
		return nil, fmt.Errorf(
			"%w: stream %s was already decided when the merge was claimed (or is already on main); "+
				"a resume may resolve only the streams it reports as pending",
			ErrUnknownResolution, undecidable[0])
	}
	return steps, nil
}

// streamsAlreadyOnMain reports, per stream of the replay set, whether its
// branch events are already on main — matched by the events' own payload ids,
// which a replay carries over unchanged. Only main's events after the claim
// position are read, in ONE set-based paged scan over the replay set's
// streams rather than a read per stream.
//
// A stream with some but not all of its branch events on main is an error: one
// Append per stream is atomic on every backend, so that state means something
// other than an interrupted merge wrote to main, and neither replaying nor
// skipping it is safe.
func (h *Handler) streamsAlreadyOnMain(ctx context.Context, groups []streamGroup, mergedAtPosition int64) (map[uuid.UUID]bool, error) {
	streamIDs := make([]uuid.UUID, 0, len(groups))
	for _, group := range groups {
		streamIDs = append(streamIDs, group.streamID)
	}

	onMain := make(map[uuid.UUID]bool)
	from := mergedAtPosition
	for len(streamIDs) > 0 {
		page, err := h.eventStore.ReadStreamsForBranch(ctx, streamIDs, domain.MainBranchID, from, resumeScanPage)
		if err != nil {
			return nil, fmt.Errorf("reading main events after the merge claim: %w", err)
		}
		for i := range page {
			id, err := eventPayloadID(page[i])
			if err != nil {
				return nil, err
			}
			onMain[id] = true
		}
		if len(page) < resumeScanPage {
			break
		}
		from = page[len(page)-1].Position
	}

	landed := make(map[uuid.UUID]bool, len(groups))
	for _, group := range groups {
		present := 0
		for i := range group.events {
			id, err := eventPayloadID(group.events[i])
			if err != nil {
				return nil, err
			}
			if onMain[id] {
				present++
			}
		}
		switch present {
		case 0:
		case len(group.events):
			landed[group.streamID] = true
		default:
			return nil, fmt.Errorf(
				"stream %s has %d of its %d branch events on main; a merge replays a stream in one atomic append, "+
					"so this is not an interrupted merge and resume will not guess at it",
				group.streamID, present, len(group.events))
		}
	}
	return landed, nil
}

// eventPayloadID extracts a stored event's domain event id (domain.BaseEvent's
// "id"), the identity a replay preserves. A missing id would make
// already-replayed detection silently fail open, so it is an error.
func eventPayloadID(evt repository.StoredEvent) (uuid.UUID, error) {
	var payload struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return uuid.Nil, fmt.Errorf("decoding %s event id on stream %s: %w", evt.EventType, evt.StreamID, err)
	}
	if payload.ID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%s event at position %d on stream %s carries no event id", evt.EventType, evt.Position, evt.StreamID)
	}
	return payload.ID, nil
}

// stepGroups returns the stream groups a resume will replay.
func stepGroups(steps []resumeStep) []streamGroup {
	groups := make([]streamGroup, 0, len(steps))
	for _, step := range steps {
		groups = append(groups, step.group)
	}
	return groups
}
