package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// GPS artifacts on resume (#760 on top of #685).
//
// An evidence analysis, evidence conflict, research log or proof summary
// stream is resumed like any other branch-aware stream. Its events are found
// on main by payload id; its main row is read with the artifact's Get (every
// GPS projection writes the row, version included, in one save, or deletes
// it), and a row behind main's log is re-projected from main's own events.
//
// Before a resume replays anything it applies the merge's three GPS rules
// (checkGPSSubjectSurvives, checkSubjectDeleteOrphansNoGPS, both through
// checkEvidence) with the resume's pending/decidable semantics: an
// auto-planned stream that breaks one is reported pending, and a "branch"
// decision for it is refused by the final check, so "main" rolls the merge
// forward without it.
//
// As with media (branch_merge_resume_media.go), the GPS-specific question is a
// MISSING main row. A GPS row can be removed by a write to ANOTHER stream: a
// PersonDeleted or FamilyDeleted of its subject cascades onto it without
// writing to the artifact's stream (missingGPSCascadedAway). Re-projecting it
// would resurrect research about nothing, so it counts as removed for a reason
// main's log explains. And PersonMerged re-points a merged person's artifacts
// to the survivor when it is projected, so a row still missing after such a
// merge means its own create projection failed BEFORE the merge; re-projecting
// it from its stream would attach it to the merged-away person. Unless the
// survivor (followed through any later merges) was itself deleted, the resume
// refuses such a landed stream with ErrMergeResumeRepairUnsound before writing
// anything.

// GPS artifact stream types, as the command layer writes them.
var gpsStreamTypes = []string{"EvidenceAnalysis", "EvidenceConflict", "ResearchLog", "ProofSummary"}

// isGPSStream reports whether a stream type is a GPS artifact's (#760).
func isGPSStream(streamType string) bool {
	for _, t := range gpsStreamTypes {
		if strings.EqualFold(streamType, t) {
			return true
		}
	}
	return false
}

// mainGPSState is mainReadModelState for the GPS artifact streams.
func (h *Handler) mainGPSState(ctx context.Context, group streamGroup) (readModelState, error) {
	var (
		version int64
		found   bool
		err     error
	)
	switch {
	case strings.EqualFold(group.streamType, "EvidenceAnalysis"):
		var a *repository.EvidenceAnalysisReadModel
		if a, err = h.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, group.streamID); a != nil {
			found, version = true, a.Version
		}
	case strings.EqualFold(group.streamType, "EvidenceConflict"):
		var c *repository.EvidenceConflictReadModel
		if c, err = h.readStore.GetEvidenceConflict(ctx, domain.MainBranchID, group.streamID); c != nil {
			found, version = true, c.Version
		}
	case strings.EqualFold(group.streamType, "ResearchLog"):
		var l *repository.ResearchLogReadModel
		if l, err = h.readStore.GetResearchLog(ctx, domain.MainBranchID, group.streamID); l != nil {
			found, version = true, l.Version
		}
	default: // isGPSStream admits only the four types
		var p *repository.ProofSummaryReadModel
		if p, err = h.readStore.GetProofSummary(ctx, domain.MainBranchID, group.streamID); p != nil {
			found, version = true, p.Version
		}
	}
	if err != nil {
		return readModelState{}, fmt.Errorf("reading main %s %s: %w", group.streamType, group.streamID, err)
	}
	return readModelState{present: found, version: version}, nil
}

// missingGPSCascadedAway reports which missing GPS artifact rows were removed
// by their subject's delete cascade: the subject the stream's main log last
// set — followed through any person merges main recorded since — ends in a
// delete on main. subjectOf holds the candidates with their subjects
// (missingGPSSubjects), and survivorOf the person merges main recorded since
// the earliest of them (survivorsAfter). A subject may be a family, which no
// merge moves; following a family id through the person merges leaves it
// where it is.
//
// refuse names the candidates already on main by payload id: for those, a
// subject merged into a person main still has is refused with
// ErrMergeResumeRepairUnsound (see the note above). A candidate not in refuse
// is reported as not removed; the resume's own checks decide whether its
// replay is sound (checkGPSSubjectSurvives flags an edit of a missing
// artifact).
//
// The work is set-based: the caller's one paged scan of main for person
// merges (shared with the person and media checks), then one paged scan of
// the final subjects' streams — never a scan per artifact.
func (h *Handler) missingGPSCascadedAway(
	ctx context.Context,
	missing []streamGroup,
	subjectOf map[uuid.UUID]uuid.UUID,
	survivorOf map[uuid.UUID]uuid.UUID,
	refuse map[uuid.UUID]bool,
) (map[uuid.UUID]bool, error) {
	if len(subjectOf) == 0 {
		return nil, nil
	}
	finalOf := make(map[uuid.UUID]uuid.UUID, len(subjectOf))
	var finals []uuid.UUID
	for artifactID, subjectID := range subjectOf {
		final := finalSurvivor(subjectID, survivorOf)
		finalOf[artifactID] = final
		finals = appendUnique(finals, final)
	}
	subjectEvents, err := h.readMainStreams(ctx, finals)
	if err != nil {
		return nil, err
	}

	cascaded := make(map[uuid.UUID]bool)
	for _, group := range missing {
		subjectID, ok := subjectOf[group.streamID]
		if !ok {
			continue
		}
		final := finalOf[group.streamID]
		switch {
		case endsInDelete(subjectEvents[final]):
			cascaded[group.streamID] = true
		case final != subjectID && refuse[group.streamID]:
			return nil, fmt.Errorf(
				"%w: %s %s is on main in the log but missing from main's read model, and its subject person %s "+
					"was merged into person %s since; re-projecting it would attach it to the merged-away person. "+
					"Nothing has been written; rebuild main's read model from the log to repair it",
				ErrMergeResumeRepairUnsound, group.streamType, group.streamID, subjectID, final)
		}
	}
	return cascaded, nil
}

// missingGPSSubjects returns the subject each missing GPS row's main log last
// set, for the rows missingGPSCascadedAway considers — GPS streams whose row
// is missing and whose stream does not itself end in a delete — and the
// position a scan for person merges must start from (-1 when there are none).
func missingGPSSubjects(
	missing []streamGroup,
	states map[uuid.UUID]readModelState,
	mainEvents map[uuid.UUID][]repository.StoredEvent,
) (map[uuid.UUID]uuid.UUID, int64, error) {
	subjectOf := make(map[uuid.UUID]uuid.UUID)
	scanFrom := int64(-1)
	for _, group := range missing {
		events := mainEvents[group.streamID]
		if states[group.streamID].present || len(events) == 0 || !isGPSStream(group.streamType) || endsInDelete(events) {
			continue
		}
		outcome, err := gpsOutcomeOf(streamGroup{streamID: group.streamID, streamType: group.streamType, events: events})
		if err != nil {
			return nil, 0, err
		}
		if !outcome.subjectSet {
			continue
		}
		subjectOf[group.streamID] = outcome.subjectID
		if first := events[0].Position; scanFrom < 0 || first < scanFrom {
			scanFrom = first
		}
	}
	return subjectOf, scanFrom, nil
}

// checkLandedGPSSubjects is the GPS half of validateResumeEvidence's
// landed-stream check: a GPS artifact already on main may not lose its subject
// to this call's own "main" resolution of the stream that creates that
// subject. Decisions recorded earlier were checked when they were made, and a
// subject main itself removed later is main's change (its delete cascaded the
// artifact).
func (h *Handler) checkLandedGPSSubjects(
	ctx context.Context,
	groups []streamGroup,
	byID map[uuid.UUID]streamGroup,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
) error {
	for _, group := range groups {
		if !view.landed[group.streamID] {
			continue
		}
		outcome, err := gpsOutcomeOf(group)
		if err != nil {
			return err
		}
		if !outcome.subjectSet || outcome.deleted || resolutions[outcome.subjectID] != ResolveMain {
			continue
		}
		subjectGroup := byID[outcome.subjectID]
		if !createsMediaOwner(subjectGroup, "person") && !createsMediaOwner(subjectGroup, "family") {
			continue
		}
		exists, err := h.gpsSubjectOnMain(ctx, outcome.subjectID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf(
				"%w: %s %s is already on main and is about subject %s, which main does not have; "+
					"resolving that subject to main would leave the research orphaned — resolve it to branch instead",
				ErrMergeDanglingReference, group.streamType, group.streamID, outcome.subjectID)
		}
	}
	return nil
}
