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

	// ErrMergeResumeConcurrent is returned when another resume of the same
	// merge recorded its decisions (domain.BranchMergeResumed) between this
	// call reading the plan and appending its own. Nothing has been replayed by
	// the refusing call. Resume again: the plan it then reads includes the
	// other request's decisions, and it will not re-decide them.
	ErrMergeResumeConcurrent = errors.New("another resume of this merge recorded its decisions first")
)

// resumeScanPage is the page size of the scan that finds which of the branch's
// events already reached main. Paged rather than capped: under-reading would
// report a landed stream as unreplayed and replay it twice, the one outcome a
// resume exists to prevent.
const resumeScanPage = 1000

// ResumeMergeInput asks ResumeMerge to finish an interrupted merge.
type ResumeMergeInput struct {
	BranchID uuid.UUID

	// Resolutions decides the streams the recorded plan cannot vouch for (see
	// ErrMergeResumeNeedsResolution). It may name ONLY those streams: every
	// other stream's fate was fixed when the merge was claimed or by an earlier
	// resume, and a resume that could re-decide it would let a second request
	// quietly rewrite what the first one reviewed. The decisions are recorded
	// in the log (domain.BranchMergeResumed) before anything is replayed.
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

	// SkippedStreamIDs are the streams resolved to main — at claim time, by an
	// earlier resume, or by this call's Resolutions — whose branch events are
	// deliberately not replayed.
	SkippedStreamIDs []uuid.UUID

	// ReprojectedStreamIDs are already-replayed streams whose main read model
	// was found behind main's event log — an earlier attempt's Append landed
	// but its synchronous projection failed — and which this call re-projected
	// from the log. Nothing is appended for them.
	ReprojectedStreamIDs []uuid.UUID

	// PendingStreamIDs is populated only alongside
	// ErrMergeResumeNeedsResolution: the streams that need a resolution.
	PendingStreamIDs []uuid.UUID
}

// resumeStep is what a resume will do with one stream of the replay set.
type resumeStep struct {
	group   streamGroup
	planned int64 // the main version the append asserts
}

// mergeRecord is what a branch's own stream says about its merge: the claim,
// the replay plan as it now stands, and the stream's version.
type mergeRecord struct {
	claim domain.BranchMerged

	// plan is the claim's ReplayStreamVersions, replaced by the latest
	// BranchMergeResumed's when a resume has recorded decisions. nil only for
	// a pre-#685 claim no resume has decided anything for.
	plan map[uuid.UUID]int64

	// branchVersion is the branch's own stream version, which recording a
	// resume's decisions asserts.
	branchVersion int64
}

// resumeDecision is planResume's verdict.
type resumeDecision struct {
	steps []resumeStep

	// nextPlan is the plan with this call's resolutions applied — what a
	// BranchMergeResumed records. Meaningful only when resolutions were given.
	nextPlan map[uuid.UUID]int64
}

// ResumeMerge finishes a merge whose replay onto main was interrupted after
// the branch was claimed (issue #685; ADR-005 §Merge implementation note).
//
// It never rewrites history (ES-002): it only appends the branch events that
// are not yet on main, exactly as MergeBranch would have, plus — when the
// caller had to decide something — one BranchMergeResumed decision record on
// the branch's own stream.
//
// What already landed is DERIVED FROM MAIN, not remembered. Replay re-appends
// each branch event decoded, so the event's own payload id travels with it onto
// main; a branch event whose payload id is already among main's events on that
// stream after the claim position has been replayed. Because replay issues one
// Append per stream and every backend applies an Append atomically, a stream is
// either wholly on main or not there at all — a stream found half-there is an
// invariant breach and is refused rather than guessed at.
//
// What is still to do comes from the RECORDED PLAN. MergeBranch records its
// replay plan on the BranchMerged event (domain.BranchMerged.ReplayStreamVersions):
// the streams to replay and main's version of each when the conflict verdict
// was computed. A remaining stream is replayed automatically only when main
// still sits at that pinned version — the same staleness guarantee the first
// attempt ran under. Anything else needs the caller's resolution
// (ErrMergeResumeNeedsResolution), checked before anything is written. The
// caller's resolutions are then appended as a domain.BranchMergeResumed carrying
// the updated plan BEFORE any replay, and every later resume reads that plan.
// So a stream a resume resolved to main stays skipped for good, and a stream it
// resolved to branch is re-pinned to the version the caller reviewed.
//
// Resume is idempotent: once every stream is on main (or skipped), a further
// call appends nothing and reports ReplayedEventCount 0. Two concurrent resumes
// cannot both append a stream: each asserts the stream's main version on
// Append, so the loser gets a concurrency conflict (reported as
// ErrMergePartiallyApplied) and its retry finds the stream already replayed.
// Two concurrent resumes cannot both record decisions either: the record
// asserts the branch stream's version (ErrMergeResumeConcurrent).
//
// A replay failure DURING a resume is reported exactly like one during a merge
// (ErrMergePartiallyApplied), and the remedy is the same: resume again.
//
// Read model: if an earlier attempt's Append landed but its synchronous
// projection then failed, the stream counts as replayed in the log, and its
// events are not appended again. Resume instead compares the stream's main
// read-model version with main's event-log version and, where the read model is
// behind, re-projects the missing events from the log (reprojectLandedStreams),
// reporting those streams in ReprojectedStreamIDs.
func (h *Handler) ResumeMerge(ctx context.Context, input ResumeMergeInput) (*ResumeMergeResult, error) {
	if h.branchStore == nil {
		return nil, ErrBranchStoreRequired
	}

	branch, err := h.branchStore.Get(ctx, input.BranchID)
	if err != nil {
		return nil, err // includes repository.ErrBranchNotFound
	}
	record, err := h.resumableMerge(ctx, branch)
	if err != nil {
		return nil, err
	}
	claim := record.claim

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
	mainVersions, err := h.mainStreamVersions(ctx, groups)
	if err != nil {
		return nil, err
	}

	result := &ResumeMergeResult{MergedAtPosition: claim.MergedAtPosition}
	decision, err := planResume(claim.BranchID, record.plan, groups, landed, mainVersions, input.Resolutions, result)
	if err != nil {
		return nil, err
	}
	if len(result.PendingStreamIDs) > 0 {
		result.Branch = branch
		return result, fmt.Errorf(
			"%w: %d stream(s) still to replay for branch %s cannot be replayed on the recorded plan "+
				"(main moved on them since it was recorded, or the claim recorded no plan). Nothing has been written; "+
				"review them with GET /branches/{id}/compare and resume again with a resolution for each: %v",
			ErrMergeResumeNeedsResolution, len(result.PendingStreamIDs), branch.ID, result.PendingStreamIDs)
	}

	// Bring main's read model level with the log for the streams already
	// there, before anything reads it on their behalf.
	result.ReprojectedStreamIDs, err = h.reprojectLandedStreams(ctx, groups, landed, mainVersions)
	if err != nil {
		return nil, err
	}

	// Same cross-entity check the merge ran before its claim: a resolution
	// made here can exclude a person a replayed family event still links. The
	// streams already on main count as present.
	if err := h.validateNoDanglingReferences(ctx, stepGroups(decision.steps), nil, landed); err != nil {
		return nil, err
	}

	if len(input.Resolutions) > 0 {
		if err := h.recordResumeDecisions(ctx, branch, record, decision.nextPlan, input.Resolutions); err != nil {
			return nil, err
		}
	}

	for i, step := range decision.steps {
		appended, err := h.replayStream(ctx, step.group, step.planned)
		result.ReplayedEventCount += appended
		if err != nil {
			return nil, fmt.Errorf(
				"%w: resuming merge of branch %s: stream %s failed after %d of %d remaining stream(s) "+
					"(%d event(s)) reached main in this attempt; resume again to finish: %w",
				ErrMergePartiallyApplied, branch.ID, step.group.streamID, i, len(decision.steps), result.ReplayedEventCount, err)
		}
	}

	merged, err := h.branchStore.Get(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("re-reading merged branch: %w", err)
	}
	result.Branch = merged
	return result, nil
}

// resumableMerge reads the branch's own stream for its merge claim and any
// decisions earlier resumes recorded, refusing a branch that has no claim. A
// claim whose registry projection never landed (the branch still reads
// "active") is repaired first, exactly as claimMerge repairs it — resuming is
// the natural next step from that state too.
func (h *Handler) resumableMerge(ctx context.Context, branch *domain.Branch) (mergeRecord, error) {
	if branch.Status == domain.BranchStatusArchived {
		return mergeRecord{}, fmt.Errorf("%w: branch %s is archived", ErrMergeNotClaimed, branch.ID)
	}

	stored, err := h.eventStore.ReadStream(ctx, branch.ID)
	if err != nil {
		return mergeRecord{}, fmt.Errorf("reading branch stream: %w", err)
	}
	// ReadStream spans every branch, so keep only this branch's own scope
	// before trusting an event type, and order by version so "latest" means
	// the last one appended.
	own := make([]repository.StoredEvent, 0, len(stored))
	for i := range stored {
		if stored[i].BranchID == domain.BranchID(branch.ID) {
			own = append(own, stored[i])
		}
	}
	sort.Slice(own, func(i, j int) bool { return own[i].Version < own[j].Version })

	var (
		record       mergeRecord
		claimed      bool
		claimVersion int64
	)
	for i := range own {
		evt := own[i]
		record.branchVersion = evt.Version
		switch evt.EventType {
		case "BranchMerged":
			if claimed {
				continue // the CAS admits one claim; the first is the claim
			}
			decoded, err := evt.DecodeEvent()
			if err != nil {
				return mergeRecord{}, fmt.Errorf("decoding branch merged event: %w", err)
			}
			claim, ok := decoded.(domain.BranchMerged)
			if !ok {
				return mergeRecord{}, fmt.Errorf("branch %s: merge claim decoded as %T, want BranchMerged", branch.ID, decoded)
			}
			record.claim, record.plan, claimed, claimVersion = claim, claim.ReplayStreamVersions, true, evt.Version
		case "BranchMergeResumed":
			if !claimed {
				return mergeRecord{}, fmt.Errorf("branch %s: resume record at version %d precedes any merge claim", branch.ID, evt.Version)
			}
			decoded, err := evt.DecodeEvent()
			if err != nil {
				return mergeRecord{}, fmt.Errorf("decoding branch merge resumed event: %w", err)
			}
			resumed, ok := decoded.(domain.BranchMergeResumed)
			if !ok {
				return mergeRecord{}, fmt.Errorf("branch %s: resume record decoded as %T, want BranchMergeResumed", branch.ID, decoded)
			}
			if resumed.ReplayStreamVersions == nil {
				// The constructor always stores {}; a missing plan here would
				// silently turn every stream into "resolved to main".
				return mergeRecord{}, fmt.Errorf("branch %s: resume record at version %d carries no replay plan", branch.ID, evt.Version)
			}
			record.plan = resumed.ReplayStreamVersions
		}
	}
	if !claimed {
		return mergeRecord{}, fmt.Errorf("%w: branch %s is %s and was never claimed for a merge; merge it instead",
			ErrMergeNotClaimed, branch.ID, branch.Status)
	}

	if branch.Status != domain.BranchStatusMerged {
		if err := h.projector.Project(ctx, record.claim, claimVersion, domain.BranchID(branch.ID)); err != nil {
			return mergeRecord{}, fmt.Errorf("repairing branch registry after an interrupted claim: %w", err)
		}
	}
	return record, nil
}

// recordResumeDecisions appends the caller's resolutions, and the plan they
// produce, to the branch's own stream at the version resumableMerge read. It
// runs before any replay, so a decision is durable before it has any effect on
// main: an interrupted resume leaves the decision recorded and the next resume
// carries it out rather than asking again.
func (h *Handler) recordResumeDecisions(
	ctx context.Context,
	branch *domain.Branch,
	record mergeRecord,
	nextPlan map[uuid.UUID]int64,
	resolutions map[uuid.UUID]MergeResolution,
) error {
	decided := make(map[uuid.UUID]string, len(resolutions))
	for streamID, side := range resolutions {
		decided[streamID] = string(side)
	}
	event := domain.NewBranchMergeResumed(branch.ID, record.claim.MergedAtPosition, nextPlan, decided)
	scope := branchScope(branch)
	if err := h.eventStore.Append(ctx, branch.ID, branchStreamType, []domain.Event{event}, record.branchVersion, scope); err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return fmt.Errorf("%w: branch %s; nothing has been replayed — resume again to see the plan as it now stands", ErrMergeResumeConcurrent, branch.ID)
		}
		return fmt.Errorf("recording resume decisions: %w", err)
	}
	if err := h.projector.Project(ctx, event, record.branchVersion+1, scope.BranchID); err != nil {
		return fmt.Errorf("projecting resume decisions: %w", err)
	}
	return nil
}

// mainStreamVersions reads main's current version of every stream in the
// replay set.
func (h *Handler) mainStreamVersions(ctx context.Context, groups []streamGroup) (map[uuid.UUID]int64, error) {
	versions := make(map[uuid.UUID]int64, len(groups))
	for _, group := range groups {
		version, err := h.eventStore.GetStreamVersion(ctx, group.streamID, domain.MainBranchID)
		if err != nil {
			return nil, fmt.Errorf("getting main stream version for %s: %w", group.streamID, err)
		}
		versions[group.streamID] = version
	}
	return versions, nil
}

// planResume decides, per stream, what a resume does, filling the result's
// already-replayed, skipped and pending lists. It writes nothing: every refusal
// it can produce happens before the first append.
//
// plan is the recorded plan (claim, or latest resume record); nil means a
// pre-#685 claim nothing has been decided for yet.
func planResume(
	branchID uuid.UUID,
	plan map[uuid.UUID]int64,
	groups []streamGroup,
	landed map[uuid.UUID]bool,
	mainVersions map[uuid.UUID]int64,
	resolutions map[uuid.UUID]MergeResolution,
	result *ResumeMergeResult,
) (resumeDecision, error) {
	if plan != nil {
		inReplaySet := make(map[uuid.UUID]bool, len(groups))
		for _, group := range groups {
			inReplaySet[group.streamID] = true
		}
		for streamID := range plan {
			if !inReplaySet[streamID] {
				return resumeDecision{}, fmt.Errorf("branch %s: the merge's recorded plan replays stream %s, which the branch never changed",
					branchID, streamID)
			}
		}
	}

	// nextPlan starts as the recorded plan. A pre-#685 claim has none, so it
	// starts from the streams already on main — they were replayed, so they
	// belong in "every stream the merge replays" — pinned at main's version.
	nextPlan := make(map[uuid.UUID]int64, len(groups))
	for streamID, version := range plan {
		nextPlan[streamID] = version
	}
	if plan == nil {
		for _, group := range groups {
			if landed[group.streamID] {
				nextPlan[group.streamID] = mainVersions[group.streamID]
			}
		}
	}

	decision := resumeDecision{}
	decidable := make(map[uuid.UUID]bool)
	for _, group := range groups {
		if landed[group.streamID] {
			result.AlreadyReplayedStreamIDs = append(result.AlreadyReplayedStreamIDs, group.streamID)
			continue
		}

		pinned, planned := plan[group.streamID]
		if plan != nil && !planned {
			// Resolved to main when the merge was claimed, or by an earlier
			// resume's recorded decision.
			result.SkippedStreamIDs = append(result.SkippedStreamIDs, group.streamID)
			continue
		}

		current := mainVersions[group.streamID]
		if planned && current == pinned {
			decision.steps = append(decision.steps, resumeStep{group: group, planned: pinned})
			continue
		}

		// The recorded plan cannot vouch for this stream; only the caller can.
		decidable[group.streamID] = true
		switch resolutions[group.streamID] {
		case ResolveBranch:
			decision.steps = append(decision.steps, resumeStep{group: group, planned: current})
			nextPlan[group.streamID] = current
		case ResolveMain:
			result.SkippedStreamIDs = append(result.SkippedStreamIDs, group.streamID)
			delete(nextPlan, group.streamID)
		default:
			result.PendingStreamIDs = append(result.PendingStreamIDs, group.streamID)
		}
	}

	// A resolution for a stream already decided is refused rather than
	// ignored: the caller believes they are choosing something they are not.
	// Sorted so a request with several bad entries reports the same one.
	undecidable := make([]uuid.UUID, 0, len(resolutions))
	for streamID := range resolutions {
		if !decidable[streamID] {
			undecidable = append(undecidable, streamID)
		}
	}
	if len(undecidable) > 0 {
		sort.Slice(undecidable, func(i, j int) bool { return undecidable[i].String() < undecidable[j].String() })
		return resumeDecision{}, fmt.Errorf(
			"%w: stream %s was already decided — when the merge was claimed or by an earlier resume — or is already on main; "+
				"a resume may resolve only the streams it reports as pending",
			ErrUnknownResolution, undecidable[0])
	}
	decision.nextPlan = nextPlan
	return decision, nil
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
