package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

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
	// refusing call. These shapes get here:
	//
	//   - main moved on the stream after the claim pinned it. The claim's
	//     conflict verdict no longer describes main there, so replaying over it
	//     would be the silent override #698 closed for the first attempt.
	//   - main removed the stream's entity after the claim — deleted it, or
	//     merged the person into another, which leaves the merged person's
	//     stream at its pin. Replaying the branch's edits restores nothing, so
	//     only "main" is accepted for it.
	//   - the claim predates #685 and recorded no replay plan, so neither the
	//     stream's resolution nor its pin is known.
	//   - the recorded plan would replay the stream, but one of its events
	//     references a person main will no longer have — main deleted or
	//     merged that person away after the claim, or this call resolves the
	//     person's own stream to main. Replaying it would be refused as a
	//     dangling reference (ErrMergeDanglingReference), so without a decision
	//     the merge could never finish; a "main" resolution rolls it forward
	//     without that stream.
	//   - the same for evidence (#758): the stream is a citation whose final
	//     source main will not have (main deleted it after the claim, or this
	//     call resolves it to main), or a source delete that would cascade onto
	//     a citation of it main still has (typically one main added after the
	//     claim).
	//   - the same for media (#759): the stream uploads a media item whose
	//     owner (person, family or source) main will not have when it lands —
	//     main deleted it after the claim, this call resolves the stream that
	//     creates it to main, or a stream already on main deleted it — or it
	//     deletes a media owner while main has an item of that owner it wrote
	//     to after the branch's delete (typically an upload made during the
	//     interruption), or changed after the branch's own edit of it landed.
	//   - the same for GPS artifacts (#760): the stream edits an evidence
	//     analysis, evidence conflict, research log or proof summary main no
	//     longer has, sets one's subject to a person or family main will not
	//     have when it lands, or deletes a person or family while main has GPS
	//     research about them that the branch never saw (added or changed on
	//     main after the fork, changed on main after the branch's own edit of
	//     it landed, or re-pointed back by main after the replay moved it
	//     away).
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
	//
	// A "main" decision is permanent. A "branch" decision re-pins the stream at
	// main's version when it was made, so it holds only while main leaves the
	// entity alone: a mainline write to it (or the removal of a person it
	// references) before its replay lands was never reviewed, and makes the
	// stream pending — and decidable — again.
	Resolutions map[uuid.UUID]MergeResolution

	// Rationales optionally says, per resolved stream, why that side won
	// (#828). Each key must also be in Resolutions. Recorded on the
	// BranchMergeResumed event with the decisions.
	Rationales map[uuid.UUID]string
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

	// Pending describes PendingStreamIDs, in the same order: what each entity
	// is, what it is called, why it needs a decision and which decisions it
	// accepts (#830).
	Pending []PendingMergeEntity
}

// resumeStep is what a resume will do with one stream of the replay set.
type resumeStep struct {
	group   streamGroup
	planned int64 // the main version the append asserts
}

// resumeView is what a resume found on main, per stream of the replay set,
// before deciding anything.
type resumeView struct {
	// landed names the streams whose branch events are already on main.
	landed map[uuid.UUID]bool

	// mainVersions is main's current version of every stream.
	mainVersions map[uuid.UUID]int64

	// removed names the streams whose entity main had and has since removed,
	// for a reason main's log explains (see streamsRemovedOnMain).
	removed map[uuid.UUID]bool

	// relinked maps each landed media item (or GPS artifact) whose main row
	// is missing and whose person owner (or subject) main has since merged
	// into another person to the final survivor: the owner or subject the
	// read-model repair attaches it to (see mergeRelink).
	relinked map[uuid.UUID]uuid.UUID

	// created names the persons a replay group creates and main has not
	// removed since (see personsCreatedByReplay).
	created map[uuid.UUID]bool

	// danglingAuto names the streams the plan would replay automatically but
	// which need a decision anyway (see danglingAutoPlannedStreams).
	danglingAuto map[uuid.UUID]bool

	// basePosition is the branch's fork point, which the GPS subject-delete
	// rule (checkSubjectDeleteOrphansNoGPS) measures main's writes from.
	basePosition int64
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

	// reasons says, per pending stream, why the recorded plan cannot vouch
	// for it (#830).
	reasons map[uuid.UUID]MergePendingReason
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
// attempt ran under — and replaying it would not leave main referencing a
// person it no longer has (danglingAutoPlannedStreams). Anything else needs
// the caller's resolution
// (ErrMergeResumeNeedsResolution), checked before anything is written. The
// caller's resolutions are then appended as a domain.BranchMergeResumed carrying
// the updated plan BEFORE any replay, and every later resume reads that plan.
// So a stream a resume resolved to main stays skipped for good, and a stream it
// resolved to branch is re-pinned to the version the caller reviewed.
//
// Resume is idempotent: once every stream is on main (or skipped), a further
// call appends nothing and reports ReplayedEventCount 0. Two concurrent resumes
// cannot both append a stream: main's versions are read before the scan for
// landed streams, so a stream the other resume lands after that read is either
// seen as landed or planned at its pre-landing version; the append asserts that
// version, so the loser is refused (reported as ErrMergePartiallyApplied) and
// its retry finds the stream already replayed.
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
// reporting those streams in ReprojectedStreamIDs. The repair runs after every
// refusal, so a refused resume writes nothing at all, and it never rolls a row
// back past a version a concurrent write already projected. Once the replay is
// done, sources' citation counts — which citation projections step outside
// the version rule — are recounted from main's citations
// (reconcileCitationCounts), and the citation streams whose sources needed it
// are reported there too.
//
// Evidence (#758): the replay runs in the evidence order MergeBranch uses
// (orderEvidenceForReplay), and the evidence rules MergeBranch checks before
// its claim (checkEvidence: a citation must end up citing a source main will
// have; a source delete must not cascade onto a citation main keeps; and,
// since #759, a media upload must land on an owner main will have and an
// owner delete must not cascade onto media the branch never saw) are
// applied exactly as the person-reference rules are — an auto-planned stream
// that breaks one is pending, and the final decision is checked again.
//
// Media (#759): landed detection and the read-model repair treat a media
// stream like any other, and neither ever writes a branch row or copies file
// bytes — see branch_merge_resume_media.go for why the repair cannot lose
// shared bytes, and for the owner-merged case (an owner merged into a person
// main still has), which the repair handles by re-projecting the stream and
// re-linking the item to the final merge survivor (relinkMergedMedia).
//
// GPS artifacts (#760): the three GPS rules MergeBranch checks (an artifact
// must land on a subject main will have; an edit must land on an artifact main
// still has; a subject delete must not cascade onto research main added or
// changed after the fork) are part of checkEvidence, so they are applied with
// the same pending/decidable semantics. Landed detection and the read-model
// repair cover GPS streams too — see branch_merge_resume_gps.go for the
// subject-delete cascade it recognises and the subject-merged case, which the
// repair handles by re-projecting the stream and re-linking the row to the
// final merge survivor (relinkMergedGPS).
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
	rationales, err := validateRationales(input.Rationales, input.Resolutions)
	if err != nil {
		return nil, err
	}

	// Everything up to here, and everything inspectResume does, only reads:
	// the same plan GET /branches/{id} reports as the merge's completeness.
	inspection, err := h.inspectResume(ctx, branch, record, input.Resolutions)
	if err != nil {
		return nil, err
	}
	groups, view, decision, result := inspection.groups, inspection.view, inspection.decision, inspection.result
	landed := view.landed
	if len(result.PendingStreamIDs) > 0 {
		result.Branch = branch
		// Named here so a caller can put the decision in front of a person
		// without a second round trip (#830).
		reasons := make(map[uuid.UUID]MergePendingReason, len(result.PendingStreamIDs))
		for _, id := range result.PendingStreamIDs {
			reasons[id] = inspection.decision.reasons[id]
		}
		if result.Pending, err = h.describePendingStreams(ctx, branch.ID, groups, reasons); err != nil {
			return nil, err
		}
		return result, fmt.Errorf(
			"%w: %d stream(s) still to replay for branch %s cannot be replayed on the recorded plan "+
				"(main moved on them since it was recorded, main removed the entity, the claim recorded no plan, "+
				"or replaying them would leave main referencing a person, source, media owner or GPS artifact or subject it no longer has, "+
				"or cascade onto a citation, media item or GPS research it still has). Nothing has been written; "+
				"review them with GET /branches/{id}/compare and resume again with a resolution for each: %v",
			ErrMergeResumeNeedsResolution, len(result.PendingStreamIDs), branch.ID, result.PendingStreamIDs)
	}

	// Same cross-entity check the merge ran before its claim: a "branch"
	// resolution made here can replay a reference to a person main no longer
	// has, and a "main" one can exclude a person a stream already on main
	// references.
	if err := h.validateResumeReferences(ctx, branch.ID, groups, decision.steps, view, input.Resolutions); err != nil {
		return nil, err
	}

	if len(input.Resolutions) > 0 {
		if err := h.recordResumeDecisions(ctx, branch, record, decision.nextPlan, input.Resolutions, rationales); err != nil {
			return nil, err
		}
	}

	// Bring main's read model level with the log for the streams already
	// there, before the replay's projections read it on their behalf. It runs
	// after every refusal above, so a refused resume has written nothing at
	// all — not even a read-model repair.
	result.ReprojectedStreamIDs, err = h.reprojectLandedStreams(ctx, groups, landed, view.mainVersions)
	if err != nil {
		// Any decisions are already recorded, so this is reported like a
		// replay failure: the remedy is to resume again.
		return nil, fmt.Errorf("%w: resuming merge of branch %s: repairing main's read model before the replay: %w",
			ErrMergePartiallyApplied, branch.ID, err)
	}

	if err := h.finishResumeReplay(ctx, branch, claim, groups, landed, decision.steps, result); err != nil {
		return nil, err
	}

	merged, err := h.branchStore.Get(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("re-reading merged branch: %w", err)
	}
	result.Branch = merged
	return result, nil
}

// finishResumeReplay replays a resume's remaining streams onto main, then
// recounts the citation counts of the sources they touch.
func (h *Handler) finishResumeReplay(
	ctx context.Context,
	branch *domain.Branch,
	claim domain.BranchMerged,
	groups []streamGroup,
	landed map[uuid.UUID]bool,
	steps []resumeStep,
	result *ResumeMergeResult,
) error {
	branchID := branch.ID
	// Stamped exactly as the merge stamped the streams it did replay (#832):
	// the provenance is built from the branch and the claim alone.
	provenance := mergeProvenance(branch, claim)
	onMain := make(map[uuid.UUID]bool, len(groups))
	for id, done := range landed {
		onMain[id] = done
	}
	for i, step := range steps {
		appended, err := h.replayStream(ctx, step.group, step.planned, provenance)
		result.ReplayedEventCount += appended
		if err != nil {
			return fmt.Errorf(
				"%w: resuming merge of branch %s: stream %s failed after %d of %d remaining stream(s) "+
					"(%d event(s)) reached main in this attempt; resume again to finish: %w",
				ErrMergePartiallyApplied, branchID, step.group.streamID, i, len(steps), result.ReplayedEventCount, err)
		}
		onMain[step.group.streamID] = true
	}

	// Source citation counts are stepped by citation projections outside the
	// citation row's version, so the version check reprojectLandedStreams
	// makes cannot see every half-projected citation. Recount them once
	// everything is on main (reconcileCitationCounts): the count is absolute,
	// so it is right whatever order the log's citations and sources landed in.
	recounted, err := h.reconcileCitationCounts(ctx, groups, onMain)
	if err != nil {
		return fmt.Errorf("%w: resuming merge of branch %s: every stream is on main, but recounting source citations failed; "+
			"resume again to finish: %w", ErrMergePartiallyApplied, branchID, err)
	}
	result.ReprojectedStreamIDs = appendUnique(result.ReprojectedStreamIDs, recounted...)
	return nil
}

// resumeReplayGroups loads the branch's replay set, grouped by stream, and
// refuses one the claim's plan cannot cover.
func (h *Handler) resumeReplayGroups(ctx context.Context, branch *domain.Branch, claim domain.BranchMerged) ([]streamGroup, error) {
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

	// The same order MergeBranch replays in (#758): sources that survive the
	// replay first, sources it deletes last. A resume of an interrupted merge
	// therefore continues the original order, and the remaining citations
	// find their sources on main and leave a doomed source before its delete
	// cascades.
	return orderEvidenceForReplay(groupEventsByStream(replaySet.ReplayEvents))
}

// resumableMerge reads the branch's own stream for its merge claim and any
// decisions earlier resumes recorded, refusing a branch that has no claim. A
// claim whose registry projection never landed (the branch still reads
// "active") is repaired first, exactly as claimMerge repairs it — resuming is
// the natural next step from that state too.
func (h *Handler) resumableMerge(ctx context.Context, branch *domain.Branch) (mergeRecord, error) {
	record, claimVersion, err := h.readMergeRecord(ctx, branch)
	if err != nil {
		return mergeRecord{}, err
	}
	if branch.Status != domain.BranchStatusMerged {
		if err := h.projector.Project(ctx, record.claim, claimVersion, domain.BranchID(branch.ID)); err != nil {
			return mergeRecord{}, fmt.Errorf("repairing branch registry after an interrupted claim: %w", err)
		}
	}
	return record, nil
}

// readMergeRecord is resumableMerge without the registry repair: it reads the
// branch's own stream and writes nothing, so the merge-completeness read
// (MergeCompleteness) can share it. claimVersion is the claim's version on
// that stream.
func (h *Handler) readMergeRecord(ctx context.Context, branch *domain.Branch) (mergeRecord, int64, error) {
	if branch.Status == domain.BranchStatusArchived {
		return mergeRecord{}, 0, fmt.Errorf("%w: branch %s is archived", ErrMergeNotClaimed, branch.ID)
	}

	stored, err := h.eventStore.ReadStream(ctx, branch.ID)
	if err != nil {
		return mergeRecord{}, 0, fmt.Errorf("reading branch stream: %w", err)
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
				return mergeRecord{}, 0, fmt.Errorf("decoding branch merged event: %w", err)
			}
			claim, ok := decoded.(domain.BranchMerged)
			if !ok {
				return mergeRecord{}, 0, fmt.Errorf("branch %s: merge claim decoded as %T, want BranchMerged", branch.ID, decoded)
			}
			record.claim, record.plan, claimed, claimVersion = claim, claim.ReplayStreamVersions, true, evt.Version
		case "BranchMergeResumed":
			if !claimed {
				return mergeRecord{}, 0, fmt.Errorf("branch %s: resume record at version %d precedes any merge claim", branch.ID, evt.Version)
			}
			decoded, err := evt.DecodeEvent()
			if err != nil {
				return mergeRecord{}, 0, fmt.Errorf("decoding branch merge resumed event: %w", err)
			}
			resumed, ok := decoded.(domain.BranchMergeResumed)
			if !ok {
				return mergeRecord{}, 0, fmt.Errorf("branch %s: resume record decoded as %T, want BranchMergeResumed", branch.ID, decoded)
			}
			if resumed.ReplayStreamVersions == nil {
				// The constructor always stores {}; a missing plan here would
				// silently turn every stream into "resolved to main".
				return mergeRecord{}, 0, fmt.Errorf("branch %s: resume record at version %d carries no replay plan", branch.ID, evt.Version)
			}
			record.plan = resumed.ReplayStreamVersions
		}
	}
	if !claimed {
		return mergeRecord{}, 0, fmt.Errorf("%w: branch %s is %s and was never claimed for a merge; merge it instead",
			ErrMergeNotClaimed, branch.ID, branch.Status)
	}
	return record, claimVersion, nil
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
	rationales map[uuid.UUID]string,
) error {
	decided := make(map[uuid.UUID]string, len(resolutions))
	for streamID, side := range resolutions {
		decided[streamID] = string(side)
	}
	event := domain.NewBranchMergeResumed(branch.ID, record.claim.MergedAtPosition, nextPlan, decided)
	event.Rationales = rationales
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
//
// A stream whose entity main has removed since the claim (view.removed) is
// never replayed on the strength of the plan, and cannot be resolved to
// "branch": replaying edits onto an entity main deleted or merged away writes
// them after its removal and restores nothing — the same reason MergeBranch
// offers only "main" for a main-side delete.
func planResume(
	branchID uuid.UUID,
	plan map[uuid.UUID]int64,
	groups []streamGroup,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
	result *ResumeMergeResult,
) (resumeDecision, error) {
	landed, mainVersions := view.landed, view.mainVersions
	if err := validatePlanCoversReplaySet(branchID, plan, groups); err != nil {
		return resumeDecision{}, err
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
		if planned && current == pinned && !view.danglingAuto[group.streamID] && !view.removed[group.streamID] {
			decision.steps = append(decision.steps, resumeStep{group: group, planned: pinned})
			continue
		}

		// The recorded plan cannot vouch for this stream; only the caller can.
		decidable[group.streamID] = true
		switch resolutions[group.streamID] {
		case ResolveBranch:
			if view.removed[group.streamID] {
				return resumeDecision{}, fmt.Errorf(
					"%w: main has removed %s (deleted, or merged into another person) since the merge was claimed; "+
						"replaying the branch's changes onto it would restore nothing, so only \"main\" can resolve it",
					ErrUnknownResolution, group.streamID)
			}
			decision.steps = append(decision.steps, resumeStep{group: group, planned: current})
			nextPlan[group.streamID] = current
		case ResolveMain:
			result.SkippedStreamIDs = append(result.SkippedStreamIDs, group.streamID)
			delete(nextPlan, group.streamID)
		default:
			result.PendingStreamIDs = append(result.PendingStreamIDs, group.streamID)
			if decision.reasons == nil {
				decision.reasons = make(map[uuid.UUID]MergePendingReason)
			}
			decision.reasons[group.streamID] = pendingReasonOf(plan, planned, pinned, current, view, group.streamID)
		}
	}

	if err := refuseUndecidableResolutions(resolutions, decidable); err != nil {
		return resumeDecision{}, err
	}
	decision.nextPlan = nextPlan
	return decision, nil
}

// validatePlanCoversReplaySet refuses a recorded plan naming a stream the
// branch never changed. A nil plan (a pre-#685 claim) names nothing.
func validatePlanCoversReplaySet(branchID uuid.UUID, plan map[uuid.UUID]int64, groups []streamGroup) error {
	inReplaySet := make(map[uuid.UUID]bool, len(groups))
	for _, group := range groups {
		inReplaySet[group.streamID] = true
	}
	for streamID := range plan {
		if !inReplaySet[streamID] {
			return fmt.Errorf("branch %s: the merge's recorded plan replays stream %s, which the branch never changed",
				branchID, streamID)
		}
	}
	return nil
}

// refuseUndecidableResolutions refuses a resolution for a stream already
// decided rather than ignoring it: the caller believes they are choosing
// something they are not. Sorted so a request with several bad entries
// reports the same one.
func refuseUndecidableResolutions(resolutions map[uuid.UUID]MergeResolution, decidable map[uuid.UUID]bool) error {
	undecidable := make([]uuid.UUID, 0, len(resolutions))
	for streamID := range resolutions {
		if !decidable[streamID] {
			undecidable = append(undecidable, streamID)
		}
	}
	if len(undecidable) > 0 {
		sort.Slice(undecidable, func(i, j int) bool { return undecidable[i].String() < undecidable[j].String() })
		return fmt.Errorf(
			"%w: stream %s was already decided — when the merge was claimed or by an earlier resume — or is already on main; "+
				"a resume may resolve only the streams it reports as pending",
			ErrUnknownResolution, undecidable[0])
	}
	return nil
}

// danglingAutoPlannedStreams returns the streams the recorded plan would
// replay automatically — planned, not yet on main, main still at the pinned
// version, entity not removed — whose events reference a person main will not
// have once the resume is done. Without this, such a stream could never
// finish: the dangling-reference check refuses its replay, and because main
// never wrote to the stream itself nothing else would ever make it decidable.
// Reporting it as pending lets the caller roll the merge forward without it
// ("main"), and records that choice like any other.
//
// The evidence rules (checkEvidence, #758) are applied the same way: an
// auto-planned citation whose final source main will not have, or an
// auto-planned source delete that would cascade onto a citation main still
// has, is flagged too, as is an auto-planned media upload whose owner main
// will not have when it lands, or an auto-planned owner delete that would
// cascade onto an item main wrote to after the branch's delete (#759), and an
// auto-planned GPS artifact stream or subject delete that breaks a GPS rule
// (#760). For them a stream counts as replayed on
// the same terms — already on main, or planned and not resolved to main by
// this call — and a source or media owner main removed since the claim does
// not count as one main will have. An owner-deleting stream already on main
// has deleted the owner whatever its place in the replay order.
//
// A person counts as present if main's read model has them, or if a group the
// resume has replayed or may still replay (already on main, or planned and not
// resolved to main by this call) creates them and main has not removed them
// since (view.created). Replaying a person's stream of EDITS does not count:
// it does not bring back a person main deleted or merged away. Pending streams
// count as replayable, so a stream is not flagged merely because a creating
// person's own decision is outstanding; if that decision turns out to be
// "main", the next call flags it.
func (h *Handler) danglingAutoPlannedStreams(
	ctx context.Context,
	plan map[uuid.UUID]int64,
	groups []streamGroup,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
) (map[uuid.UUID]bool, error) {
	if plan == nil {
		return nil, nil // nothing replays automatically without a plan
	}
	present := make(map[uuid.UUID]bool, len(groups))
	evidence := evidencePlan{
		replayed:     make(map[uuid.UUID]streamGroup, len(groups)),
		removed:      view.removed,
		order:        replayOrder(groups),
		landed:       view.landed,
		basePosition: view.basePosition,
	}
	var auto []streamGroup
	for _, group := range groups {
		id := group.streamID
		pinned, planned := plan[id]
		switch {
		case view.landed[id]:
			present[id] = view.created[id]
			evidence.replayed[id] = group
		case !planned:
			// skipped by the recorded plan
		default:
			if resolutions[id] != ResolveMain {
				present[id] = view.created[id]
				evidence.replayed[id] = group
			}
			if view.mainVersions[id] == pinned && !view.removed[id] {
				auto = append(auto, group)
			}
		}
	}
	mergedInto, err := pendingPersonMerges(evidence.replayed, view.landed)
	if err != nil {
		return nil, err
	}
	evidence.mergedInto = mergedInto
	dangling, err := h.findDanglingReferences(ctx, auto,
		func(personID uuid.UUID) bool { return present[personID] }, survivorLookup(mergedInto))
	if err != nil {
		return nil, err
	}
	flagged := make(map[uuid.UUID]bool, len(dangling))
	for _, d := range dangling {
		flagged[d.group.streamID] = true
	}
	for _, group := range auto {
		var list blockerList
		if err := h.checkEvidence(ctx, group, evidence, &list); err != nil {
			return nil, err
		}
		if list.len() > 0 {
			flagged[group.streamID] = true
		}
	}
	if len(flagged) == 0 {
		return nil, nil
	}
	return flagged, nil
}

// validateResumeReferences is the resume's dangling-reference check, run once
// every stream is decided and before anything is written.
//
//   - A stream about to be replayed may reference only a person main's read
//     model has, or one a group already on main or about to be replayed
//     creates and main has not removed since (view.created). Every
//     auto-planned stream that fails this was made decidable
//     (danglingAutoPlannedStreams), so a refusal here always has a way out:
//     resolve that stream to main.
//   - A stream ALREADY on main can no longer be excluded, so a person it
//     references may not be excluded by this call: a "main" resolution for a
//     person main does not have, while a landed stream still references them,
//     would leave main with the phantom row the check exists to prevent (a
//     pre-#685 claim can reach this: the branch created the person, and the
//     family linking them landed before the interruption). Only this call's
//     own "main" resolutions of a person the replay creates are checked here — decisions recorded earlier
//     were checked when they were made, and a person main itself removed
//     later is main's own change, not the resume's to refuse.
//   - The evidence rules (#758) hold on the same terms: every stream about to
//     be replayed passes checkEvidence against the landed and replayed
//     streams, and a landed citation may not lose its source to this call's
//     "main" resolution of the stream that creates it.
//
// Every breach is reported as a named blocker on one *MergeBlockedError
// (#831). A stream about to be replayed is fixed by resolving it to main; a
// landed stream cannot be, so its blocker suggests resolving the referenced
// entity to branch instead.
func (h *Handler) validateResumeReferences(
	ctx context.Context,
	branchID uuid.UUID,
	groups []streamGroup,
	steps []resumeStep,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
) error {
	var list blockerList
	if err := h.collectResumeBlockers(ctx, groups, steps, view, resolutions, &list); err != nil {
		return err
	}
	return h.refusal(ctx, branchID, &list)
}

// collectResumeBlockers adds every breach validateResumeReferences refuses to
// list.
func (h *Handler) collectResumeBlockers(
	ctx context.Context,
	groups []streamGroup,
	steps []resumeStep,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
	list *blockerList,
) error {
	replayed := make(map[uuid.UUID]bool, len(groups))
	for id, onMain := range view.landed {
		replayed[id] = onMain
	}
	for _, step := range steps {
		replayed[step.group.streamID] = true
	}
	stepMerges, err := branchPersonMerges(stepGroups(steps))
	if err != nil {
		return err
	}
	dangling, err := h.findDanglingReferences(ctx, stepGroups(steps), func(personID uuid.UUID) bool {
		return replayed[personID] && view.created[personID]
	}, survivorLookup(stepMerges))
	if err != nil {
		return err
	}
	for _, d := range dangling {
		list.add(d.blocker())
	}

	var landedGroups []streamGroup
	for _, group := range groups {
		if view.landed[group.streamID] {
			landedGroups = append(landedGroups, group)
		}
	}
	// Deliberately no merge chain here (#834): this check only guards this
	// call's own "main" resolutions of a person the replay would create, and
	// refusing such a resolution — the caller can resolve that person to
	// branch instead — is the conservative answer.
	dangling, err = h.findDanglingReferences(ctx, landedGroups, func(personID uuid.UUID) bool {
		// Only excluding a person the replay would CREATE takes them away;
		// excluding a stream of edits leaves the person as main has them.
		return resolutions[personID] != ResolveMain || !view.created[personID]
	}, nil)
	if err != nil {
		return err
	}
	for _, d := range dangling {
		b := streamBlocker(d.group, BlockerMissingPerson, d.personID, "person",
			"stream %s is already on main and references person %s, whom main does not have; "+
				"resolving that person to main would leave the reference dangling — resolve them to branch instead",
			d.group.streamID, d.personID)
		b.SuggestedResolution = FixIncludeReferenced
		list.add(b)
	}
	return h.validateResumeEvidence(ctx, groups, steps, view, resolutions, list)
}

// validateResumeEvidence is validateResumeReferences' evidence half: the
// streams about to be replayed must pass checkEvidence (the citation, source,
// media-owner and GPS rules), and this call's own "main" resolution may not
// exclude a source the replay creates while a citation already on main cites
// it, nor a media owner main does not have while a media upload already on
// main is attached to it (checkLandedMediaOwners), nor a GPS subject main does
// not have while a GPS artifact already on main is about it
// (checkLandedGPSSubjects). Every breach is added to list.
func (h *Handler) validateResumeEvidence(
	ctx context.Context,
	groups []streamGroup,
	steps []resumeStep,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
	list *blockerList,
) error {
	evidence := evidencePlan{
		replayed:     make(map[uuid.UUID]streamGroup, len(groups)),
		removed:      view.removed,
		order:        replayOrder(groups),
		landed:       view.landed,
		basePosition: view.basePosition,
	}
	byID := make(map[uuid.UUID]streamGroup, len(groups))
	for _, group := range groups {
		byID[group.streamID] = group
		if view.landed[group.streamID] {
			evidence.replayed[group.streamID] = group
		}
	}
	for _, step := range steps {
		evidence.replayed[step.group.streamID] = step.group
	}
	mergedInto, err := pendingPersonMerges(evidence.replayed, view.landed)
	if err != nil {
		return err
	}
	evidence.mergedInto = mergedInto
	for _, step := range steps {
		if err := h.checkEvidence(ctx, step.group, evidence, list); err != nil {
			return err
		}
	}

	for _, group := range groups {
		if !view.landed[group.streamID] {
			continue
		}
		outcome, err := citationOutcomeOf(group)
		if err != nil {
			return err
		}
		if outcome.deleted || !outcome.repointed || resolutions[outcome.sourceID] != ResolveMain ||
			!createsSource(byID[outcome.sourceID]) {
			continue
		}
		source, err := h.readStore.GetSource(ctx, domain.MainBranchID, outcome.sourceID)
		if err != nil {
			return fmt.Errorf("checking source %s on main: %w", outcome.sourceID, err)
		}
		if source == nil {
			b := streamBlocker(group, BlockerMissingSource, outcome.sourceID, "source",
				"citation %s is already on main and cites source %s, which main does not have; "+
					"resolving that source to main would leave the citation orphaned — resolve it to branch instead",
				group.streamID, outcome.sourceID)
			b.SuggestedResolution = FixIncludeReferenced
			list.add(b)
		}
	}
	if err := h.checkLandedMediaOwners(ctx, groups, view, resolutions, list); err != nil {
		return err
	}
	return h.checkLandedGPSSubjects(ctx, groups, view, resolutions, list)
}

// personsCreatedByReplay returns the persons whose replay group creates them
// (createsPerson) and whose entity main has not removed since.
func personsCreatedByReplay(groups []streamGroup, removed map[uuid.UUID]bool) map[uuid.UUID]bool {
	created := make(map[uuid.UUID]bool)
	for _, group := range groups {
		if createsPerson(group) && !removed[group.streamID] {
			created[group.streamID] = true
		}
	}
	return created
}

// streamsRemovedOnMain returns the streams of the replay set whose entity main
// had and no longer has, for a reason main's log explains: the stream ends in
// a delete; a person was merged into another (PersonMerged, which does not
// write to the merged person's stream); an association lost one of its
// persons to a delete cascade (which does not write to the association's
// stream either); a citation lost its source, a media item its owner, or a
// GPS artifact its subject, the same way. Such a stream's replay restores nothing, whatever the plan
// pinned, and the person it names is not present for reference checks.
//
// A stream main never had (version 0: the branch created the entity and it
// has not landed) cannot have been removed. A row missing for any other
// reason — a failed projection of an already-replayed stream — is not a
// removal; reprojectLandedStreams repairs it. For a missing media row whose
// person owner, or a missing GPS row whose subject person, main merged into
// another person, it also reports the final survivor that repair re-links the
// row to.
func (h *Handler) streamsRemovedOnMain(ctx context.Context, groups []streamGroup, mainVersions map[uuid.UUID]int64) (map[uuid.UUID]bool, map[uuid.UUID]uuid.UUID, error) {
	var missing []streamGroup
	states := make(map[uuid.UUID]readModelState)
	for _, group := range groups {
		if mainVersions[group.streamID] == 0 || !isReadModelStream(group.streamType) {
			continue
		}
		state, err := h.mainReadModelState(ctx, group)
		if err != nil {
			return nil, nil, err
		}
		if state.present {
			continue
		}
		states[group.streamID] = state
		missing = append(missing, group)
	}
	if len(missing) == 0 {
		return nil, nil, nil
	}

	streamIDs := make([]uuid.UUID, 0, len(missing))
	for _, group := range missing {
		streamIDs = append(streamIDs, group.streamID)
	}
	mainEvents, err := h.readMainStreams(ctx, streamIDs)
	if err != nil {
		return nil, nil, err
	}
	removedElsewhere, relink, err := h.missingRowsRemovedElsewhere(ctx, missing, states, mainEvents)
	if err != nil {
		return nil, nil, err
	}
	removed := make(map[uuid.UUID]bool, len(missing))
	for _, group := range missing {
		gone, err := h.goneForLoggedReason(ctx, group, mainEvents[group.streamID], removedElsewhere[group.streamID])
		if err != nil {
			return nil, nil, err
		}
		if gone {
			removed[group.streamID] = true
		}
	}
	var relinked map[uuid.UUID]uuid.UUID
	for id, move := range relink {
		if relinked == nil {
			relinked = make(map[uuid.UUID]uuid.UUID, len(relink))
		}
		relinked[id] = move.target
	}
	return removed, relinked, nil
}

// isReadModelStream reports whether mainReadModelState can read a stream
// type's main row.
func isReadModelStream(streamType string) bool {
	return isPersonStream(streamType) || strings.EqualFold(streamType, familyStreamType) || isAssociationStream(streamType) ||
		isSourceStream(streamType) || isCitationStream(streamType) || isNoteStream(streamType) || isMediaStream(streamType) ||
		isGPSStream(streamType)
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
