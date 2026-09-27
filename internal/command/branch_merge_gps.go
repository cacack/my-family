package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// GPS artifact events (#760), by what they do to the artifact. The creates
// (EvidenceConflictDetected is the conflict's create) carry subject_id at the
// top level; the updates carry it in their Changes map when they re-point the
// artifact; EvidenceConflictResolved edits a conflict without a subject.
var (
	gpsCreateEvents = map[string]bool{
		"EvidenceAnalysisCreated":  true,
		"EvidenceConflictDetected": true,
		"ResearchLogCreated":       true,
		"ProofSummaryCreated":      true,
	}
	gpsUpdateEvents = map[string]bool{
		"EvidenceAnalysisUpdated": true,
		"ResearchLogUpdated":      true,
		"ProofSummaryUpdated":     true,
	}
	gpsEditEvents = map[string]bool{
		"EvidenceConflictResolved": true,
	}
	gpsDeleteEvents = map[string]bool{
		"EvidenceAnalysisDeleted": true,
		"ResearchLogDeleted":      true,
		"ProofSummaryDeleted":     true,
	}
)

// gpsOutcome is what the replay of one stream does to a GPS artifact.
type gpsOutcome struct {
	// touched is true when the stream carries any GPS artifact event.
	touched bool
	// created is true when the stream creates the artifact (so main need not
	// have it yet).
	created bool
	// deleted is true when the stream ends by deleting the artifact.
	deleted bool
	// subjectSet is true when the stream sets the artifact's subject (a create,
	// or an update carrying subject_id); subjectID is then the last one set.
	subjectSet bool
	subjectID  uuid.UUID
}

// gpsOutcomeOf folds a stream's GPS artifact events into what its replay leaves
// the artifact as. Events of other entity types are ignored.
func gpsOutcomeOf(group streamGroup) (gpsOutcome, error) {
	var out gpsOutcome
	for _, evt := range group.events {
		switch {
		case gpsCreateEvents[evt.EventType]:
			var payload struct {
				SubjectID uuid.UUID `json:"subject_id"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return out, fmt.Errorf("decoding %s on stream %s: %w", evt.EventType, group.streamID, err)
			}
			out = gpsOutcome{touched: true, created: true, subjectSet: true, subjectID: payload.SubjectID}
		case gpsUpdateEvents[evt.EventType]:
			out.touched = true
			var payload struct {
				Changes map[string]any `json:"changes"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return out, fmt.Errorf("decoding %s on stream %s: %w", evt.EventType, group.streamID, err)
			}
			raw, present := payload.Changes["subject_id"].(string)
			if !present {
				continue
			}
			id, err := uuid.Parse(raw)
			if err != nil {
				return out, fmt.Errorf("decoding %s on stream %s: subject_id %q: %w", evt.EventType, group.streamID, raw, err)
			}
			out.subjectSet, out.subjectID = true, id
		case gpsEditEvents[evt.EventType]:
			out.touched = true
		case gpsDeleteEvents[evt.EventType]:
			out.touched, out.deleted = true, true
		}
	}
	return out, nil
}

// gpsSubjectDeleteEvents are the events whose store cascade removes a GPS
// artifact with that subject (DeletePerson / DeleteFamily, #760).
var gpsSubjectDeleteEvents = []string{"PersonDeleted", "FamilyDeleted"}

// groupDeletesGPSSubject reports whether a stream's replay deletes a person or
// family, cascading the GPS artifacts about it.
func groupDeletesGPSSubject(group streamGroup) bool {
	for _, eventType := range gpsSubjectDeleteEvents {
		if groupDeletes(group, eventType) {
			return true
		}
	}
	return false
}

// checkGPSSubjectSurvives refuses a replayed GPS artifact that will not land on
// main as the branch left it (#760):
//
//   - An edit (no create, no delete) of an artifact main no longer has. Main's
//     subject-delete cascade removes artifacts with no event on their stream,
//     so no merge conflict flags it, and the replayed update would be a silent
//     no-op. On a merge, a stream whose merge conflict is still undecided is
//     skipped, so the conflict is reported first; once decided "branch" (an
//     edit_edit conflict does not show main's later subject delete) the rule
//     applies. A resume has no undecided conflicts, so it applies the rule to
//     every stream it would replay; one that fails it is made pending.
//
//   - An artifact whose final subject will not exist on main when it lands,
//     though it was a person or family (a subject id no person or family ever
//     had is accepted, as the write path accepts it).
//     The rule is checkMediaOwnerSurvives': a subject the replay itself deletes
//     is fine only when its stream replays AFTER the artifact's (and, on a
//     resume, has not already landed), so the subject's delete cascades the
//     artifact on main exactly as it did on the branch. On a resume a subject
//     main removed since the claim is gone whatever its replayed stream holds.
func (h *Handler) checkGPSSubjectSurvives(ctx context.Context, group streamGroup, plan evidencePlan) error {
	outcome, err := gpsOutcomeOf(group)
	if err != nil || !outcome.touched || outcome.deleted {
		return err
	}
	if !outcome.created && !plan.undecided[group.streamID] {
		exists, err := h.gpsArtifactOnMain(ctx, group.streamType, group.streamID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf(
				"%w: the branch edits %s %s, but main no longer has it (removed with its subject after the fork); "+
					"merging would drop the branch's edit with no record",
				ErrMergeDanglingReference, group.streamType, group.streamID)
		}
	}
	if !outcome.subjectSet {
		return nil
	}
	subjectID := outcome.subjectID
	if subjectGroup, replaysSubject := plan.replayed[subjectID]; replaysSubject {
		deletes := groupDeletesGPSSubject(subjectGroup)
		deletesLater := deletes && !plan.landed[subjectID] && plan.order[subjectID] > plan.order[group.streamID]
		if !plan.removed[subjectID] && (!deletes || deletesLater) {
			return nil
		}
	} else {
		exists, err := h.gpsSubjectOnMain(ctx, subjectID)
		if err != nil || exists {
			return err
		}
		// The write path does not require the subject to exist (only a
		// non-nil id), so a subject no person or family ever had is accepted
		// on main as on the branch; refusing it here would make the branch
		// hold research its own merge can never take. Only a subject that
		// was a person or family, and is gone, is dangling.
		known, err := h.gpsSubjectEverExisted(ctx, subjectID)
		if err != nil || !known {
			return err
		}
	}
	return fmt.Errorf(
		"%w: the branch's %s %s is about subject %s, but no person or family with that id will exist on main "+
			"when it lands (deleted there, excluded by a \"main\" resolution, or deleted earlier in the replay)",
		ErrMergeDanglingReference, group.streamType, group.streamID, subjectID)
}

// gpsArtifactOnMain reports whether main currently has the GPS artifact a
// stream of the given type writes.
func (h *Handler) gpsArtifactOnMain(ctx context.Context, streamType string, id uuid.UUID) (bool, error) {
	var found bool
	var err error
	switch streamType {
	case "EvidenceAnalysis":
		var a *repository.EvidenceAnalysisReadModel
		a, err = h.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, id)
		found = a != nil
	case "EvidenceConflict":
		var c *repository.EvidenceConflictReadModel
		c, err = h.readStore.GetEvidenceConflict(ctx, domain.MainBranchID, id)
		found = c != nil
	case "ResearchLog":
		var l *repository.ResearchLogReadModel
		l, err = h.readStore.GetResearchLog(ctx, domain.MainBranchID, id)
		found = l != nil
	case "ProofSummary":
		var p *repository.ProofSummaryReadModel
		p, err = h.readStore.GetProofSummary(ctx, domain.MainBranchID, id)
		found = p != nil
	default:
		return false, fmt.Errorf("%w: stream %s carries GPS artifact events under unknown stream type %q",
			ErrMergeDanglingReference, id, streamType)
	}
	if err != nil {
		return false, fmt.Errorf("checking %s %s on main: %w", streamType, id, err)
	}
	return found, nil
}

// gpsSubjectOnMain reports whether main has a person or a family with the id. A
// GPS artifact's subject is one or the other (a research log also records which,
// the other artifacts leave it to the fact type), and the ids never collide.
func (h *Handler) gpsSubjectOnMain(ctx context.Context, subjectID uuid.UUID) (bool, error) {
	for _, entityType := range []string{"person", "family"} {
		exists, err := h.mediaOwnerOnMain(ctx, entityType, subjectID)
		if err != nil || exists {
			return exists, err
		}
	}
	return false, nil
}

// gpsSubjectEverExisted reports whether the log has ever held a person or
// family with the id, on main or on any branch. Stream types are compared
// case-insensitively: GEDCOM import writes "person", the commands "Person".
func (h *Handler) gpsSubjectEverExisted(ctx context.Context, subjectID uuid.UUID) (bool, error) {
	events, err := h.eventStore.ReadStream(ctx, subjectID)
	if err != nil {
		return false, fmt.Errorf("checking subject %s in the event log: %w", subjectID, err)
	}
	for i := range events {
		if strings.EqualFold(events[i].StreamType, "Person") || strings.EqualFold(events[i].StreamType, "Family") {
			return true, nil
		}
	}
	return false, nil
}

// gpsArtifactIDsOnMain lists the ids of every GPS artifact main has about a
// subject.
func (h *Handler) gpsArtifactIDsOnMain(ctx context.Context, subjectID uuid.UUID) ([]uuid.UUID, error) {
	main := domain.MainBranchID
	var ids []uuid.UUID
	analyses, err := h.readStore.GetAnalysesBySubject(ctx, main, subjectID)
	if err != nil {
		return nil, fmt.Errorf("checking evidence analyses of %s on main: %w", subjectID, err)
	}
	for i := range analyses {
		ids = append(ids, analyses[i].ID)
	}
	conflicts, err := h.readStore.GetConflictsForSubject(ctx, main, subjectID)
	if err != nil {
		return nil, fmt.Errorf("checking evidence conflicts of %s on main: %w", subjectID, err)
	}
	for i := range conflicts {
		ids = append(ids, conflicts[i].ID)
	}
	logs, err := h.readStore.GetResearchLogsForSubject(ctx, main, subjectID)
	if err != nil {
		return nil, fmt.Errorf("checking research logs of %s on main: %w", subjectID, err)
	}
	for i := range logs {
		ids = append(ids, logs[i].ID)
	}
	proofs, err := h.readStore.GetProofSummariesBySubject(ctx, main, subjectID)
	if err != nil {
		return nil, fmt.Errorf("checking proof summaries of %s on main: %w", subjectID, err)
	}
	for i := range proofs {
		ids = append(ids, proofs[i].ID)
	}
	return ids, nil
}

// checkSubjectDeleteOrphansNoGPS refuses a replayed PersonDeleted/FamilyDeleted
// that would cascade GPS research off main the branch never agreed to lose
// (#760). For each artifact main has about the subject:
//
//   - The replay deletes it: nothing is lost.
//   - The replay re-points it elsewhere: fine when its stream replays BEFORE
//     the subject's delete; replayed after, the cascade has already removed it
//     and the re-point would land on nothing.
//   - The replay otherwise touches it: the branch saw it and let its own delete
//     cascade it; any divergence from main is a merge conflict on its stream.
//   - The replay does not touch it: fine only when main has not written its
//     stream since the fork, so it is the very artifact the branch's own delete
//     cascaded. One main added or changed after the fork was never seen by the
//     branch, and the cascade would drop it from main with no record.
//
// On a resume, an artifact stream already on main (landed) is judged from
// main's LOG, not the row, and the "touched" reasoning above no longer holds
// for it: nothing conflict-checks main's writes to a landed stream after it
// landed, since a landed stream has no pin and is not replayed again.
//
//   - Landed, re-pointed away, yet main's read model still has it about the
//     subject: the row may be stale (the landed append's projection failed,
//     and reprojectLandedStreams repairs it only after these checks). If
//     main's log leaves the artifact about another subject, or deleted, the
//     cascade will not reach it once the row is repaired. If the log has it
//     about this subject again, main re-pointed it back after the branch's
//     re-point landed — main's own change the branch never saw — and the
//     delete is refused.
//   - Landed otherwise: fine only when main's log has no event on its stream
//     after the landed replay's own events. One main wrote after the landing
//     was never seen by the branch, and the cascade would drop that change
//     from main with no record, so the delete is refused.
func (h *Handler) checkSubjectDeleteOrphansNoGPS(ctx context.Context, group streamGroup, plan evidencePlan) error {
	if !groupDeletesGPSSubject(group) {
		return nil
	}
	subjectID := group.streamID
	ids, err := h.gpsArtifactIDsOnMain(ctx, subjectID)
	if err != nil {
		return err
	}
	var untouched, landedIDs []uuid.UUID
	landedRepoints := make(map[uuid.UUID]bool)
	for _, id := range ids {
		artifactGroup, ok := plan.replayed[id]
		if !ok {
			untouched = append(untouched, id)
			continue
		}
		outcome, err := gpsOutcomeOf(artifactGroup)
		if err != nil {
			return err
		}
		repointsAway := outcome.subjectSet && !outcome.deleted && outcome.subjectID != subjectID
		switch {
		case plan.landed[id]:
			landedIDs = append(landedIDs, id)
			landedRepoints[id] = repointsAway
		case repointsAway && plan.order[id] > plan.order[subjectID]:
			return fmt.Errorf(
				"%w: the branch re-points main's %s %s away from %s and deletes %s, but the delete replays first "+
					"and would remove the artifact from main before the re-point lands",
				ErrMergeDanglingReference, artifactGroup.streamType, id, subjectID, subjectID)
		}
	}
	if err := h.checkLandedGPSArtifacts(ctx, subjectID, landedIDs, landedRepoints, plan); err != nil {
		return err
	}
	if len(untouched) == 0 {
		return nil
	}
	// One set-based read: any main event on these streams after the fork.
	changed, err := h.eventStore.ReadStreamsForBranch(ctx, untouched, domain.MainBranchID, plan.basePosition, 1)
	if err != nil {
		return fmt.Errorf("checking main's GPS research on %s since the fork: %w", subjectID, err)
	}
	if len(changed) > 0 {
		return fmt.Errorf(
			"%w: the branch deletes %s, but main's %s %s about it was added or changed after the fork; "+
				"merging would delete that research from main with no record",
			ErrMergeDanglingReference, subjectID, changed[0].StreamType, changed[0].StreamID)
	}
	return nil
}

// checkLandedGPSArtifacts judges the GPS artifacts main's read model lists
// under a subject a resume is about to delete, whose streams the interrupted
// merge has already landed on main (see checkSubjectDeleteOrphansNoGPS). Main's
// log decides, from one set-based read of the artifacts' streams:
//
//   - one the landed replay re-pointed away (repointsAway) is fine while main's
//     log leaves it about another subject, or deleted;
//   - any other is fine only while main has not written its stream since the
//     landed replay's own events.
func (h *Handler) checkLandedGPSArtifacts(
	ctx context.Context, subjectID uuid.UUID, ids []uuid.UUID, repointsAway map[uuid.UUID]bool, plan evidencePlan,
) error {
	if len(ids) == 0 {
		return nil
	}
	byStream, err := h.readMainStreams(ctx, ids)
	if err != nil {
		return fmt.Errorf("checking main's GPS research on %s: %w", subjectID, err)
	}
	for _, id := range ids {
		events := byStream[id]
		if len(events) == 0 {
			return fmt.Errorf("checking main's GPS research on %s: main's log has no events for landed stream %s", subjectID, id)
		}
		if !repointsAway[id] {
			later, err := mainWriteAfterLanding(plan.replayed[id], events)
			if err != nil {
				return err
			}
			if later == nil {
				continue
			}
			return fmt.Errorf(
				"%w: the branch deletes %s, but main changed %s %s about it (%s at position %d) after the branch's "+
					"own changes to it landed; merging would delete that research from main with no record",
				ErrMergeDanglingReference, subjectID, events[0].StreamType, id, later.EventType, later.Position)
		}
		onMain, err := gpsOutcomeOf(streamGroup{streamID: id, streamType: events[0].StreamType, events: events})
		if err != nil {
			return err
		}
		if onMain.deleted || (onMain.subjectSet && onMain.subjectID != subjectID) {
			continue
		}
		return fmt.Errorf(
			"%w: the branch re-pointed %s %s away from %s and deletes %s, but main re-pointed it back after the "+
				"re-point landed; merging would delete that research from main with no record",
			ErrMergeDanglingReference, events[0].StreamType, id, subjectID, subjectID)
	}
	return nil
}
