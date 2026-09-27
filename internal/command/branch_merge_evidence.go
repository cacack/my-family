package command

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// citationOutcome is what the replay of one stream leaves a citation citing.
type citationOutcome struct {
	// deleted is true when the stream ends with CitationDeleted.
	deleted bool
	// repointed is true when the stream sets the citation's source (a create,
	// or an update carrying source_id); sourceID is then the last one set.
	repointed bool
	sourceID  uuid.UUID
}

// citationOutcomeOf folds a stream's citation events into the source the
// citation ends up citing. Events that are not citation events are ignored.
func citationOutcomeOf(group streamGroup) (citationOutcome, error) {
	var out citationOutcome
	for _, evt := range group.events {
		switch evt.EventType {
		case "CitationCreated":
			var payload struct {
				SourceID uuid.UUID `json:"source_id"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return out, fmt.Errorf("decoding citation create on stream %s: %w", group.streamID, err)
			}
			out = citationOutcome{repointed: true, sourceID: payload.SourceID}
		case "CitationUpdated":
			var payload struct {
				Changes map[string]any `json:"changes"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return out, fmt.Errorf("decoding citation update on stream %s: %w", group.streamID, err)
			}
			raw, ok := payload.Changes["source_id"].(string)
			if !ok {
				continue
			}
			id, err := uuid.Parse(raw)
			if err != nil {
				return out, fmt.Errorf("decoding citation update on stream %s: source_id %q: %w", group.streamID, raw, err)
			}
			out.repointed, out.sourceID = true, id
		case "CitationDeleted":
			out.deleted = true
		}
	}
	return out, nil
}

// orderEvidenceForReplay reorders the replay's stream groups so that citation
// replay never runs against a source in the wrong state (#758).
//
// The replay is one Append per stream, so a citation stream's events all land
// before or all land after a source stream's. The projection resolves a
// citation's source on main at the moment the citation event lands: to bump
// that source's citation_count and denormalize its title. The branch's own
// first-touch order gets both halves of that wrong:
//
//   - A citation re-pointed (or created) at a source the branch created after
//     first touching the citation would replay before that source exists on
//     main — the citation moves but keeps the old title, and the new source's
//     count is never bumped.
//
//   - A source the branch deletes after re-pointing main's citation away from
//     it, but touched before that citation, would replay first — and the
//     store's source→citation cascade would delete main's citation before its
//     re-point lands.
//
// So: every source stream that survives the replay goes first (the citations
// can then find it), every source stream that ends deleted goes last (by then
// every citation has left it, which checkSourceDeleteOrphansNothing and the
// branch's ErrSourceHasCitations guard ensure), and all other streams keep
// their relative first-touch order in between. Sources do not reference any
// other replayed aggregate, so moving them cannot break another ordering.
func orderEvidenceForReplay(groups []streamGroup) []streamGroup {
	ordered := make([]streamGroup, 0, len(groups))
	var middle, last []streamGroup
	for _, group := range groups {
		switch {
		case group.streamType != sourceStreamType:
			middle = append(middle, group)
		case groupDeletesSource(group):
			last = append(last, group)
		default:
			ordered = append(ordered, group)
		}
	}
	ordered = append(ordered, middle...)
	return append(ordered, last...)
}

// sourceStreamType is the stream type the command layer writes sources under.
const sourceStreamType = "Source"

// groupDeletesSource reports whether a stream's replay deletes its source.
func groupDeletesSource(group streamGroup) bool {
	return groupDeletes(group, "SourceDeleted")
}

// validateNoDanglingEvidence is the source/citation half of
// validateNoDanglingReferences (#758). A citation lives on its own stream and
// names a source on another, so per-aggregate resolutions and per-aggregate
// conflict detection both miss two shapes:
//
//   - A replayed citation that ends up citing a source main will not have —
//     deleted on main after the fork, or excluded by a "main" resolution. The
//     projection saves such a citation anyway, with a blank source title, so
//     main would gain an orphaned citation.
//
//   - A replayed SourceDeleted while main still has citations of that source
//     that the replay does not delete or re-point — typically one main added
//     after the fork, which the branch never saw when its own delete guard
//     (ErrSourceHasCitations) ran. The store's source→citation cascade would
//     delete those citations on main with no CitationDeleted event and no
//     conflict shown.
//
//   - A replayed media upload whose owner (person, family or source) will not
//     exist on main when the upload lands (#759) — deleted on main after the
//     fork, excluded by a "main" resolution, or deleted by the branch itself in
//     a stream that replays first. The projection saves the media row without
//     checking its owner, so main would gain an orphaned media item.
//
//   - A replayed GPS artifact (evidence analysis, evidence conflict, research
//     log or proof summary; #760) whose final subject — the person or family it
//     is about — will not exist on main when it lands, by the same rule as the
//     media owner. The projection saves the artifact without checking its
//     subject, so main would gain research about nothing.
//
//   - A replayed edit of a GPS artifact main no longer has. Main's
//     DeletePerson/DeleteFamily cascade removes a subject's artifacts with no
//     event on their streams, so per-stream conflict detection sees nothing,
//     and the replayed update lands on a missing row as a silent no-op: the
//     branch's research would be dropped with no conflict shown.
//
//   - A replayed PersonDeleted/FamilyDeleted while main has GPS artifacts about
//     that subject the branch never saw — added or changed on main after the
//     fork — and the replay does not itself delete or re-point first. The
//     store's cascade would delete them from main with no event and no
//     conflict shown (the GPS counterpart of checkSourceDeleteOrphansNothing).
//
// All are refused before the claim, like the dangling child link.
func (h *Handler) validateNoDanglingEvidence(ctx context.Context, plan *query.MergePlan, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution) error {
	replayed := make(map[uuid.UUID]streamGroup, len(groups))
	order := make(map[uuid.UUID]int, len(groups))
	for i, group := range groups {
		if resolutions[group.streamID] != ResolveMain {
			replayed[group.streamID] = group
			order[group.streamID] = i
		}
	}

	// A stream with a merge conflict is the conflict machinery's to report: a
	// main-side delete of a GPS artifact is an edit-vs-delete conflict whose
	// only honourable resolution skips the branch's stream, so it must not be
	// pre-empted here by a dangling-reference refusal.
	conflicted := make(map[uuid.UUID]bool, len(plan.Conflicts))
	for _, conflict := range plan.Conflicts {
		conflicted[conflict.StreamID] = true
	}

	for _, group := range groups {
		if _, ok := replayed[group.streamID]; !ok {
			continue
		}
		if err := h.checkCitationSourceSurvives(ctx, group, replayed); err != nil {
			return err
		}
		if err := h.checkSourceDeleteOrphansNothing(ctx, group, replayed); err != nil {
			return err
		}
		if err := h.checkMediaOwnerSurvives(ctx, group, replayed, order); err != nil {
			return err
		}
		if err := h.checkGPSSubjectSurvives(ctx, group, replayed, order, conflicted[group.streamID]); err != nil {
			return err
		}
		if err := h.checkSubjectDeleteOrphansNoGPS(ctx, plan.Branch.BasePosition, group, replayed, order); err != nil {
			return err
		}
	}
	return nil
}

// mediaOwnerDeleteEvents maps a media owner's entity type to the event that
// deletes that owner (and, through the store's cascade, its media).
var mediaOwnerDeleteEvents = map[string]string{
	"person": "PersonDeleted",
	"family": "FamilyDeleted",
	"source": "SourceDeleted",
}

// mediaUploadOf reports the owner a media stream's replay uploads the item to.
// ok is false when the stream creates nothing (a metadata edit of an existing
// item, whose owner is main's business) or ends by deleting the item.
func mediaUploadOf(group streamGroup) (entityType string, entityID uuid.UUID, ok bool, err error) {
	for _, evt := range group.events {
		switch evt.EventType {
		case "MediaCreated":
			var payload struct {
				EntityType string    `json:"entity_type"`
				EntityID   uuid.UUID `json:"entity_id"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return "", uuid.Nil, false, fmt.Errorf("decoding media create on stream %s: %w", group.streamID, err)
			}
			entityType, entityID, ok = payload.EntityType, payload.EntityID, true
		case "MediaDeleted":
			ok = false
		}
	}
	return entityType, entityID, ok, nil
}

// groupDeletes reports whether a stream's replay contains the given event type.
func groupDeletes(group streamGroup, eventType string) bool {
	for _, evt := range group.events {
		if evt.EventType == eventType {
			return true
		}
	}
	return false
}

// checkMediaOwnerSurvives refuses a replayed media upload whose owner will not
// exist on main when the upload lands. An owner the replay itself deletes is
// fine only when its stream replays AFTER the media stream: the owner's delete
// then cascades the item on main exactly as it did on the branch. Replayed the
// other way round, the upload would land on an owner that is already gone.
func (h *Handler) checkMediaOwnerSurvives(ctx context.Context, group streamGroup, replayed map[uuid.UUID]streamGroup, order map[uuid.UUID]int) error {
	entityType, entityID, ok, err := mediaUploadOf(group)
	if err != nil || !ok {
		return err
	}
	deleteEvent, known := mediaOwnerDeleteEvents[entityType]
	if !known {
		return fmt.Errorf("%w: the branch's media %s is attached to an unknown entity type %q",
			ErrMergeDanglingReference, group.streamID, entityType)
	}
	if ownerGroup, replaysOwner := replayed[entityID]; replaysOwner {
		if !groupDeletes(ownerGroup, deleteEvent) || order[entityID] > order[group.streamID] {
			return nil
		}
	} else {
		exists, err := h.mediaOwnerOnMain(ctx, entityType, entityID)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: the branch's media %s is attached to %s %s, but that %s will not exist on main when the media lands "+
			"(deleted there, excluded by a \"main\" resolution, or deleted earlier in the replay)",
		ErrMergeDanglingReference, group.streamID, entityType, entityID, entityType)
}

// mediaOwnerOnMain reports whether main currently has the given media owner.
func (h *Handler) mediaOwnerOnMain(ctx context.Context, entityType string, entityID uuid.UUID) (bool, error) {
	var found bool
	var err error
	switch entityType {
	case "person":
		var p *repository.PersonReadModel
		p, err = h.readStore.GetPerson(ctx, domain.MainBranchID, entityID)
		found = p != nil
	case "family":
		var f *repository.FamilyReadModel
		f, err = h.readStore.GetFamily(ctx, domain.MainBranchID, entityID)
		found = f != nil
	case "source":
		var s *repository.SourceReadModel
		s, err = h.readStore.GetSource(ctx, domain.MainBranchID, entityID)
		found = s != nil
	}
	if err != nil {
		return false, fmt.Errorf("checking %s %s on main: %w", entityType, entityID, err)
	}
	return found, nil
}

// checkCitationSourceSurvives refuses a replayed citation stream whose final
// source will not exist on main once the replay is done. Only the FINAL source
// matters: a citation created on a source and later re-pointed lands on the
// second one, and one the branch deleted cites nothing.
func (h *Handler) checkCitationSourceSurvives(ctx context.Context, group streamGroup, replayed map[uuid.UUID]streamGroup) error {
	outcome, err := citationOutcomeOf(group)
	if err != nil {
		return err
	}
	if outcome.deleted || !outcome.repointed {
		return nil
	}
	if sourceGroup, ok := replayed[outcome.sourceID]; ok {
		if !groupDeletesSource(sourceGroup) {
			return nil
		}
	} else {
		source, err := h.readStore.GetSource(ctx, domain.MainBranchID, outcome.sourceID)
		if err != nil {
			return fmt.Errorf("checking source %s on main: %w", outcome.sourceID, err)
		}
		if source != nil {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: the branch's citation %s cites source %s, but that source will not exist on main "+
			"(deleted there, or excluded by a \"main\" resolution)",
		ErrMergeDanglingReference, group.streamID, outcome.sourceID)
}

// checkSourceDeleteOrphansNothing refuses a replayed SourceDeleted while main
// has a citation of that source the replay does not itself delete or re-point
// elsewhere.
func (h *Handler) checkSourceDeleteOrphansNothing(ctx context.Context, group streamGroup, replayed map[uuid.UUID]streamGroup) error {
	if !groupDeletesSource(group) {
		return nil
	}
	citations, err := h.readStore.GetCitationsForSource(ctx, domain.MainBranchID, group.streamID)
	if err != nil {
		return fmt.Errorf("checking citations of source %s on main: %w", group.streamID, err)
	}
	for _, citation := range citations {
		if citationGroup, ok := replayed[citation.ID]; ok {
			outcome, err := citationOutcomeOf(citationGroup)
			if err != nil {
				return err
			}
			if outcome.deleted || (outcome.repointed && outcome.sourceID != group.streamID) {
				continue
			}
		}
		return fmt.Errorf(
			"%w: the branch deletes source %s, but main's citation %s still cites it; "+
				"merging would delete that citation from main with no record",
			ErrMergeDanglingReference, group.streamID, citation.ID)
	}
	return nil
}

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
//     no-op. A stream with a merge conflict is skipped: the conflict is
//     reported (and resolved) on its own terms.
//
//   - An artifact whose final subject will not exist on main when it lands.
//     The rule is checkMediaOwnerSurvives': a subject the replay itself deletes
//     is fine only when its stream replays AFTER the artifact's, so the
//     subject's delete cascades the artifact on main exactly as it did on the
//     branch.
func (h *Handler) checkGPSSubjectSurvives(ctx context.Context, group streamGroup, replayed map[uuid.UUID]streamGroup, order map[uuid.UUID]int, conflicted bool) error {
	outcome, err := gpsOutcomeOf(group)
	if err != nil || !outcome.touched || outcome.deleted {
		return err
	}
	if !outcome.created && !conflicted {
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
	if subjectGroup, replaysSubject := replayed[subjectID]; replaysSubject {
		if !groupDeletesGPSSubject(subjectGroup) || order[subjectID] > order[group.streamID] {
			return nil
		}
	} else {
		exists, err := h.gpsSubjectOnMain(ctx, subjectID)
		if err != nil {
			return err
		}
		if exists {
			return nil
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
func (h *Handler) checkSubjectDeleteOrphansNoGPS(ctx context.Context, basePosition int64, group streamGroup, replayed map[uuid.UUID]streamGroup, order map[uuid.UUID]int) error {
	if !groupDeletesGPSSubject(group) {
		return nil
	}
	subjectID := group.streamID
	ids, err := h.gpsArtifactIDsOnMain(ctx, subjectID)
	if err != nil {
		return err
	}
	var untouched []uuid.UUID
	for _, id := range ids {
		artifactGroup, ok := replayed[id]
		if !ok {
			untouched = append(untouched, id)
			continue
		}
		outcome, err := gpsOutcomeOf(artifactGroup)
		if err != nil {
			return err
		}
		if outcome.subjectSet && !outcome.deleted && outcome.subjectID != subjectID && order[id] > order[subjectID] {
			return fmt.Errorf(
				"%w: the branch re-points main's %s %s away from %s and deletes %s, but the delete replays first "+
					"and would remove the artifact from main before the re-point lands",
				ErrMergeDanglingReference, artifactGroup.streamType, id, subjectID, subjectID)
		}
	}
	if len(untouched) == 0 {
		return nil
	}
	// One set-based read: any main event on these streams after the fork.
	changed, err := h.eventStore.ReadStreamsForBranch(ctx, untouched, domain.MainBranchID, basePosition, 1)
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
