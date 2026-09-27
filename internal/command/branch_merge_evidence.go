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
	for i := range group.events {
		evt := group.events[i]
		if evt.EventType == "CitationDeleted" {
			out.deleted = true
			continue
		}
		sourceID, sets, err := citedSource(evt)
		if err != nil {
			return out, err
		}
		if !sets {
			continue
		}
		if evt.EventType == "CitationCreated" {
			out = citationOutcome{}
		}
		out.repointed, out.sourceID = true, sourceID
	}
	return out, nil
}

// citedSource returns the source one citation event makes the citation cite:
// a CitationCreated's source, or the source_id a CitationUpdated sets. sets is
// false for any other event, and for an update that leaves the source alone.
func citedSource(evt repository.StoredEvent) (sourceID uuid.UUID, sets bool, err error) {
	switch evt.EventType {
	case "CitationCreated":
		var payload struct {
			SourceID uuid.UUID `json:"source_id"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return uuid.Nil, false, fmt.Errorf("decoding citation create on stream %s: %w", evt.StreamID, err)
		}
		return payload.SourceID, true, nil
	case "CitationUpdated":
		var payload struct {
			Changes map[string]any `json:"changes"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return uuid.Nil, false, fmt.Errorf("decoding citation update on stream %s: %w", evt.StreamID, err)
		}
		raw, ok := payload.Changes["source_id"].(string)
		if !ok {
			return uuid.Nil, false, nil
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, false, fmt.Errorf("decoding citation update on stream %s: source_id %q: %w", evt.StreamID, raw, err)
		}
		return id, true, nil
	}
	return uuid.Nil, false, nil
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
//
// Media gets the same treatment for its person and family owners (#759): a
// media upload whose owner's stream deletes that owner is moved to just
// before the owner's stream (see moveMediaBeforeOwnerDelete), so the upload
// lands before the delete that cascades it away — as it did on the branch.
func orderEvidenceForReplay(groups []streamGroup) ([]streamGroup, error) {
	ordered := make([]streamGroup, 0, len(groups))
	var middle, last []streamGroup
	for _, group := range groups {
		switch {
		case !isSourceStream(group.streamType):
			middle = append(middle, group)
		case groupDeletesSource(group):
			last = append(last, group)
		default:
			ordered = append(ordered, group)
		}
	}
	middle, err := moveMediaBeforeOwnerDelete(middle)
	if err != nil {
		return nil, err
	}
	ordered = append(ordered, middle...)
	return append(ordered, last...), nil
}

// moveMediaBeforeOwnerDelete moves each media stream that uploads an item to
// a person or family whose own stream (also in groups) deletes it to just
// before that owner stream, keeping every other stream's relative order.
//
// First-touch order can put the owner first: a branch that creates (or edits)
// a person, uploads a photo of them, then deletes them touches the person
// before the photo, so the person's delete would replay before the upload and
// the upload would land on a person main no longer has — the media-owner rule
// refuses that. On the branch the delete came after the upload and cascaded
// it, so replaying the upload first reproduces the branch's result. A media
// stream references nothing but its owner, so moving it earlier cannot break
// another ordering; and an owner that is merged away rather than deleted
// (PersonMerged lands on the survivor's stream) is not moved around.
func moveMediaBeforeOwnerDelete(groups []streamGroup) ([]streamGroup, error) {
	pos := make(map[uuid.UUID]int, len(groups))
	for i, group := range groups {
		pos[group.streamID] = i
	}
	// before[i] lists the media streams to emit just ahead of groups[i].
	before := map[int][]int{}
	moved := map[int]bool{}
	for i, group := range groups {
		if !isMediaStream(group.streamType) {
			continue
		}
		entityType, ownerID, ok, err := mediaUploadOf(group)
		if err != nil {
			return nil, err
		}
		if !ok || (entityType != "person" && entityType != "family") {
			continue
		}
		ownerPos, replayed := pos[ownerID]
		if !replayed || ownerPos > i || !groupDeletes(groups[ownerPos], mediaOwnerDeleteEvents[entityType]) {
			continue
		}
		before[ownerPos] = append(before[ownerPos], i)
		moved[i] = true
	}
	if len(moved) == 0 {
		return groups, nil
	}
	ordered := make([]streamGroup, 0, len(groups))
	for i, group := range groups {
		for _, m := range before[i] {
			ordered = append(ordered, groups[m])
		}
		if !moved[i] {
			ordered = append(ordered, group)
		}
	}
	return ordered, nil
}

// isSourceStream reports whether a stream type is a source's. Source streams
// are written as both "Source" (commands) and "source" (GEDCOM import).
func isSourceStream(streamType string) bool {
	return strings.EqualFold(streamType, "source")
}

// isCitationStream reports whether a stream type is a citation's ("Citation"
// from commands, "citation" from GEDCOM import).
func isCitationStream(streamType string) bool {
	return strings.EqualFold(streamType, "citation")
}

// isNoteStream reports whether a stream type is a note's ("Note" from
// commands, "note" from GEDCOM import).
func isNoteStream(streamType string) bool {
	return strings.EqualFold(streamType, "note")
}

// groupDeletesSource reports whether a stream's replay deletes its source.
func groupDeletesSource(group streamGroup) bool {
	return groupDeletes(group, "SourceDeleted")
}

// createsSource reports whether a replay group leaves its source in existence
// on main by itself: a source stream that creates the source and does not end
// by deleting it.
func createsSource(group streamGroup) bool {
	if !isSourceStream(group.streamType) || groupDeletesSource(group) {
		return false
	}
	for i := range group.events {
		if group.events[i].EventType == "SourceCreated" {
			return true
		}
	}
	return false
}

// evidencePlan is what the evidence checks need to know about a replay.
type evidencePlan struct {
	// replayed holds every stream whose branch events are, or will be, on
	// main once the replay is done — for a merge, every stream not resolved
	// to main; for a resume, also the streams already on main.
	replayed map[uuid.UUID]streamGroup

	// removed names the streams whose entity main has removed since the
	// merge was claimed (resume only; see streamsRemovedOnMain). Replaying
	// such a source or media-owner stream restores nothing, so it does not
	// count as one main will have.
	removed map[uuid.UUID]bool

	// order is each stream's position in the replay order
	// (orderEvidenceForReplay), which the media-owner rule needs: an owner the
	// replay deletes must delete it AFTER the upload lands.
	order map[uuid.UUID]int

	// landed names the streams already on main (resume only). A landed
	// stream that deletes a media owner has already deleted it, whatever its
	// place in the order.
	landed map[uuid.UUID]bool
}

// validateNoDanglingEvidence is the evidence and media half of
// validateNoDanglingReferences (#758, #759). A citation lives on its own
// stream and names a source on another, and a media item names its owner on
// another, so per-aggregate resolutions and per-aggregate conflict detection
// all miss four shapes:
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
//     a stream that still replays first (a person or family owner's delete
//     does not: moveMediaBeforeOwnerDelete puts the upload ahead of it). The
//     projection saves the media row without
//     checking its owner, so main would gain an orphaned media item.
//
//   - A replayed PersonDeleted, FamilyDeleted or SourceDeleted while main has a
//     media item of that owner it wrote to after the branch's delete (#759) —
//     typically an upload the branch never saw. The store's owner→media
//     cascade would delete it from main with no MediaDeleted event and no
//     conflict shown (checkOwnerDeleteOrphansNoMedia).
//
// All are refused before the claim, like the dangling child link. ResumeMerge
// applies the same rules through checkEvidence (see
// danglingAutoPlannedStreams and validateResumeReferences).
func (h *Handler) validateNoDanglingEvidence(ctx context.Context, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution) error {
	plan := evidencePlan{replayed: make(map[uuid.UUID]streamGroup, len(groups)), order: replayOrder(groups)}
	for _, group := range groups {
		if resolutions[group.streamID] != ResolveMain {
			plan.replayed[group.streamID] = group
		}
	}

	for _, group := range groups {
		if _, ok := plan.replayed[group.streamID]; !ok {
			continue
		}
		if err := h.checkEvidence(ctx, group, plan); err != nil {
			return err
		}
	}
	return nil
}

// replayOrder maps each stream to its position in the replay order.
func replayOrder(groups []streamGroup) map[uuid.UUID]int {
	order := make(map[uuid.UUID]int, len(groups))
	for i, group := range groups {
		order[group.streamID] = i
	}
	return order
}

// checkEvidence applies the evidence rules — the two citation/source rules
// (#758) and the two media-owner rules (#759) — to one stream the replay will
// append. A refusal wraps ErrMergeDanglingReference; any other error is a
// failure to check.
func (h *Handler) checkEvidence(ctx context.Context, group streamGroup, plan evidencePlan) error {
	if err := h.checkCitationSourceSurvives(ctx, group, plan); err != nil {
		return err
	}
	if err := h.checkSourceDeleteOrphansNothing(ctx, group, plan.replayed); err != nil {
		return err
	}
	if err := h.checkOwnerDeleteOrphansNoMedia(ctx, group, plan.replayed); err != nil {
		return err
	}
	return h.checkMediaOwnerSurvives(ctx, group, plan)
}

// mediaOwnerDeleteEvents maps a media owner's entity type to the event that
// deletes that owner (and, through the store's cascade, its media).
var mediaOwnerDeleteEvents = map[string]string{
	"person": "PersonDeleted",
	"family": "FamilyDeleted",
	"source": "SourceDeleted",
}

// isMediaStream reports whether a stream type is a media item's ("Media" from
// commands, "media" from GEDCOM import).
func isMediaStream(streamType string) bool {
	return strings.EqualFold(streamType, "media")
}

// mediaUploadOf reports the owner a media stream's replay uploads the item to.
// ok is false when the stream creates nothing (a metadata edit of an existing
// item, whose owner is main's business) or ends by deleting the item.
func mediaUploadOf(group streamGroup) (entityType string, entityID uuid.UUID, ok bool, err error) {
	for _, evt := range group.events {
		switch evt.EventType {
		case "MediaCreated":
			entityType, entityID, err = mediaOwnerOf(evt)
			if err != nil {
				return "", uuid.Nil, false, err
			}
			ok = true
		case "MediaDeleted":
			ok = false
		}
	}
	return entityType, entityID, ok, nil
}

// mediaOwnerOf decodes the owner a MediaCreated attaches its item to.
func mediaOwnerOf(evt repository.StoredEvent) (entityType string, entityID uuid.UUID, err error) {
	var payload struct {
		EntityType string    `json:"entity_type"`
		EntityID   uuid.UUID `json:"entity_id"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return "", uuid.Nil, fmt.Errorf("decoding media create on stream %s: %w", evt.StreamID, err)
	}
	return payload.EntityType, payload.EntityID, nil
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
// other way round — or, on a resume, already landed on main — the upload would
// land on an owner that is already gone. On a resume an owner main removed
// since the claim is gone whatever its replayed stream holds.
func (h *Handler) checkMediaOwnerSurvives(ctx context.Context, group streamGroup, plan evidencePlan) error {
	entityType, entityID, ok, err := mediaUploadOf(group)
	if err != nil || !ok {
		return err
	}
	deleteEvent, known := mediaOwnerDeleteEvents[entityType]
	if !known {
		return fmt.Errorf("%w: the branch's media %s is attached to an unknown entity type %q",
			ErrMergeDanglingReference, group.streamID, entityType)
	}
	if ownerGroup, replaysOwner := plan.replayed[entityID]; replaysOwner {
		deletesLater := groupDeletes(ownerGroup, deleteEvent) &&
			!plan.landed[entityID] && plan.order[entityID] > plan.order[group.streamID]
		if !plan.removed[entityID] && (!groupDeletes(ownerGroup, deleteEvent) || deletesLater) {
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
func (h *Handler) checkCitationSourceSurvives(ctx context.Context, group streamGroup, plan evidencePlan) error {
	outcome, err := citationOutcomeOf(group)
	if err != nil {
		return err
	}
	if outcome.deleted || !outcome.repointed {
		return nil
	}
	survives, err := h.sourceSurvivesReplay(ctx, outcome.sourceID, plan)
	if err != nil || survives {
		return err
	}
	return fmt.Errorf(
		"%w: the branch's citation %s cites source %s, but that source will not exist on main "+
			"(deleted there, or excluded by a \"main\" resolution)",
		ErrMergeDanglingReference, group.streamID, outcome.sourceID)
}

// sourceSurvivesReplay reports whether main will have a source once the
// replay is done. A replayed source stream decides it — unless it deletes the
// source, or main has removed the source since the claim; any other source is
// as main's read model has it.
func (h *Handler) sourceSurvivesReplay(ctx context.Context, sourceID uuid.UUID, plan evidencePlan) (bool, error) {
	if sourceGroup, ok := plan.replayed[sourceID]; ok {
		return !groupDeletesSource(sourceGroup) && !plan.removed[sourceID], nil
	}
	source, err := h.readStore.GetSource(ctx, domain.MainBranchID, sourceID)
	if err != nil {
		return false, fmt.Errorf("checking source %s on main: %w", sourceID, err)
	}
	return source != nil, nil
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

// mediaPageSize is the page size checkOwnerDeleteOrphansNoMedia lists an
// owner's main media with.
const mediaPageSize = 500

// ownerDeleteOf reports the media-owner entity type a stream's replay deletes
// and the log position at which the branch deleted it. ok is false for a
// stream that deletes no media owner.
func ownerDeleteOf(group streamGroup) (entityType string, deletedAt int64, ok bool) {
	for entity, deleteEvent := range mediaOwnerDeleteEvents {
		if !strings.EqualFold(group.streamType, entity) {
			continue
		}
		for _, evt := range group.events {
			if evt.EventType == deleteEvent {
				return entity, evt.Position, true
			}
		}
	}
	return "", 0, false
}

// checkOwnerDeleteOrphansNoMedia refuses a replayed PersonDeleted,
// FamilyDeleted or SourceDeleted while main has a media item of that owner
// that the branch never saw when it deleted the owner (#759). The store's
// owner→media cascade deletes the owner's media without writing to the media
// streams, so replaying the delete would remove such an item from main with
// no MediaDeleted event and no conflict shown — the media counterpart of
// checkSourceDeleteOrphansNothing.
//
// Unlike a source's citations, an owner's media has no delete guard: the
// branch's own delete cascades every item the branch sees, and the branch
// sees main's items through the overlay, including ones main added after the
// fork. So what the branch accounted for is decided by the log: an item main
// wrote to (uploaded, edited) only BEFORE the branch's delete event was in
// the branch's view and was cascaded there too, and replaying the delete
// reproduces that. An item main wrote to after it — typically an upload the
// branch never saw — is refused. Items whose media stream the replay itself
// carries are the branch's own business and skipped; a person main merged
// into the owner after the branch's delete wrote PersonMerged to the owner's
// stream, which the merge's conflict detection (or, on resume, the plan's
// staleness pin) already puts in front of the caller.
//
// The work is one media listing per owner-deleting stream and one set-based
// query for the first main event on the listed items' streams after the
// branch's delete — never a read per item, and never the items' histories
// (whose MediaCreated events carry the file bytes).
func (h *Handler) checkOwnerDeleteOrphansNoMedia(ctx context.Context, group streamGroup, replayed map[uuid.UUID]streamGroup) error {
	entityType, deletedAt, ok := ownerDeleteOf(group)
	if !ok {
		return nil
	}
	var candidates []uuid.UUID
	for offset := 0; ; offset += mediaPageSize {
		page, total, err := h.readStore.ListMediaForEntity(ctx, entityType, group.streamID,
			repository.ListOptions{Limit: mediaPageSize, Offset: offset, BranchID: domain.MainBranchID})
		if err != nil {
			return fmt.Errorf("checking media of %s %s on main: %w", entityType, group.streamID, err)
		}
		for i := range page {
			if _, ours := replayed[page[i].ID]; !ours {
				candidates = append(candidates, page[i].ID)
			}
		}
		if len(page) == 0 || offset+len(page) >= total {
			break
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	// Any main event on a candidate's stream after the branch's delete refuses
	// the merge, so one query for the first such event across all candidates
	// answers the question. The limit of one keeps it from materializing the
	// items' histories: a MediaCreated carries the file and thumbnail bytes,
	// and only an offending event's position and type are needed.
	later, err := h.eventStore.ReadStreamsForBranch(ctx, candidates, domain.MainBranchID, deletedAt, 1)
	if err != nil {
		return fmt.Errorf("checking main changes to media of %s %s: %w", entityType, group.streamID, err)
	}
	if len(later) == 0 {
		return nil
	}
	evt := later[0]
	return fmt.Errorf(
		"%w: the branch deletes %s %s, but main's media %s attached to it changed after that delete "+
			"(%s at position %d), so the branch never saw it; merging would delete it from main with no record",
		ErrMergeDanglingReference, entityType, group.streamID, evt.StreamID, evt.EventType, evt.Position)
}
