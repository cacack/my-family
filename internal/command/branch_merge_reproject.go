package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// readModelState is what main's read model holds for one replayed aggregate.
type readModelState struct {
	present bool
	version int64
}

// reprojectLandedStreams repairs main's read model for the streams a resume
// found already replayed in the log (#685).
//
// ADR-003 projects synchronously after each Append, and the event store and the
// read model share no transaction. So an interrupted merge can leave a stream
// whose events are on main in the LOG but whose projection failed partway:
// main's read model still lacks the entity or shows it as it was. The log is
// the source of truth and the events must not be appended again, so the repair
// is to finish the projection from the log.
//
// Detection is by version. Every projection handler for the events a branch
// may carry (BR-006's branch-aware set: the person, family, association,
// source, citation and note streams) writes the aggregate's read-model version as its LAST step, so the
// read-model version is the number of events on the stream whose projection
// completed. A row behind main's stream version is re-projected from the first
// event past it; re-running an event whose projection stopped midway is safe
// because each of its writes is an upsert or a delete, and the family's child
// count and version are set absolutely (the number of child rows; the event's
// version, never lowered) rather than stepped.
//
// Nothing here appends, so no optimistic check serializes two resumes
// repairing the same stream, or a resume and a mainline write to it. Most
// projections set the row's version outright, so re-running an OLD event after
// a newer one was projected would roll the row back (fields and version). The
// repair therefore never lowers a row: see reprojectStream, which skips every
// event the row has already reached, re-checked immediately before each one,
// and does not report a stream repaired until a final read finds the row level
// with main's log — re-projecting again if a racing write slipped in between
// its check and its save. So a resume that returns success has left each
// stream it repaired level with the log as that final read saw it.
//
// A missing row is re-projected from the start unless the entity is gone for a
// reason the log explains: its stream ends in a delete; (for a person) main
// merged it into another person with PersonMerged, which removes the row
// without writing to the merged person's stream; (for an association) one
// of its persons is gone from main, whose delete cascade removes the
// association's row without writing to its stream either; or (for a citation)
// the source it cites was deleted on main, whose cascade removes the citation
// the same way; or (for media, #759) its owner was deleted on main, whose
// cascade removes the item the same way; or (for a GPS artifact, #760) its
// subject person or family was deleted on main, likewise.
//
// Media streams (#759) follow the same version rule: each media projection
// writes the row, version included, in one save (or deletes it). The repair
// projects onto main only and never copies bytes: see
// branch_merge_resume_media.go, including the owner-merged case, where the
// re-projected row is then re-linked to the merge survivor as PersonMerged
// would have done (relinkMergedMedia).
//
// GPS artifact streams (#760) follow the same version rule: each of their
// projections writes the row, version included, in one save (or deletes it).
// See branch_merge_resume_gps.go for the missing-row cases, including the
// subject-merged case, where the re-projected row is then re-linked to the
// merge survivor as PersonMerged would have done (relinkMergedGPS).
//
// Source, citation and note streams (#758) are covered by the same version
// rule: each of their projections writes the row, version included, in one
// save (or deletes it). The one write outside that rule is the source
// citation count a citation projection steps, which reconcileCitationCounts
// recounts after this repair.
//
// Only already-replayed streams are checked, and each check is one read-model
// lookup; main's events are read, in one set-based paged scan, only for the
// streams found behind.
func (h *Handler) reprojectLandedStreams(ctx context.Context, groups []streamGroup, landed map[uuid.UUID]bool, mainVersions map[uuid.UUID]int64) ([]uuid.UUID, error) {
	behind, states, err := h.streamsBehindOnMain(ctx, groups, landed, mainVersions)
	if err != nil || len(behind) == 0 {
		return nil, err
	}

	streamIDs := make([]uuid.UUID, 0, len(behind))
	for _, group := range behind {
		streamIDs = append(streamIDs, group.streamID)
	}
	mainEvents, err := h.readMainStreams(ctx, streamIDs)
	if err != nil {
		return nil, err
	}
	removedElsewhere, relink, err := h.missingRowsRemovedElsewhere(ctx, behind, states, mainEvents)
	if err != nil {
		return nil, err
	}

	var repaired []uuid.UUID
	for _, group := range behind {
		events := mainEvents[group.streamID]
		if len(events) == 0 {
			return nil, fmt.Errorf("stream %s is on main by payload id but main has no events for it", group.streamID)
		}
		if !states[group.streamID].present {
			gone, err := h.goneForLoggedReason(ctx, group, events, removedElsewhere[group.streamID])
			if err != nil {
				return nil, err
			}
			if gone {
				continue // nothing to repair
			}
		}
		var move *mergeRelink
		if m, ok := relink[group.streamID]; ok {
			move = &m
		}
		if err := h.reprojectStream(ctx, group, events, move); err != nil {
			return nil, err
		}
		repaired = append(repaired, group.streamID)
	}
	return repaired, nil
}

// goneForLoggedReason reports whether a stream's missing main read-model row is
// missing for a reason the log explains (see reprojectLandedStreams), so
// re-projecting it would resurrect something main removed. removedElsewhere
// carries the verdict of missingRowsRemovedElsewhere for the stream: a person
// merged away, or a citation cascaded away with its source.
func (h *Handler) goneForLoggedReason(ctx context.Context, group streamGroup, events []repository.StoredEvent, removedElsewhere bool) (bool, error) {
	if endsInDelete(events) || removedElsewhere {
		return true, nil
	}
	if !isAssociationStream(group.streamType) {
		return false, nil
	}
	for i := range events {
		if events[i].EventType != "AssociationCreated" {
			continue
		}
		personIDs, err := personReferences(events[i])
		if err != nil {
			return false, err
		}
		for _, personID := range personIDs {
			person, err := h.readStore.GetPerson(ctx, domain.MainBranchID, personID)
			if err != nil {
				return false, fmt.Errorf("reading main person %s for association %s: %w", personID, group.streamID, err)
			}
			if person == nil {
				return true, nil // removed by the person's delete cascade
			}
		}
		return false, nil
	}
	return false, nil
}

// reprojectAttempts bounds how often reprojectStream re-runs a stream that a
// racing mainline write keeps pushing ahead. Each attempt only replays events
// the row has not reached, so two are enough unless writes land continuously.
const reprojectAttempts = 3

// errReprojectRaced is returned when a stream's row is still behind main's log
// after every attempt. Nothing was lost — the log holds every event — and a
// further resume repairs it.
var errReprojectRaced = errors.New("main's read model kept moving while the resume repaired it")

// reprojectStream brings one stream's main read-model row level with main's
// log without ever lowering it. Before each event it re-reads the row and
// skips the event if the row already reached that version — a concurrent
// resume or a mainline write projected it, or something newer. After the pass
// it re-reads the row and main's stream version: a racing write that
// committed between a check and the save after it could have been rolled back
// by that save, and shows up here as a row behind the log, so the stream is
// re-read and the pass repeated from the row's version. A row that vanished
// during the pass is not re-created (see the comment at that check).
//
// relink, when set, is the transfer a media or GPS row needs after its
// re-projection (see mergeRelink); it is made after every pass, before the
// row is checked against the log, so a racing write the transfer's save
// rolled back is re-projected by the next pass like any other.
func (h *Handler) reprojectStream(ctx context.Context, group streamGroup, events []repository.StoredEvent, relink *mergeRelink) error {
	for attempt := 0; attempt < reprojectAttempts; attempt++ {
		if attempt > 0 {
			reread, err := h.readMainStreams(ctx, []uuid.UUID{group.streamID})
			if err != nil {
				return err
			}
			events = reread[group.streamID]
		}
		if err := h.reprojectForward(ctx, group, events); err != nil {
			return err
		}
		if relink != nil {
			if err := h.relinkMerged(ctx, group, *relink); err != nil {
				return err
			}
		}

		head, err := h.eventStore.GetStreamVersion(ctx, group.streamID, domain.MainBranchID)
		if err != nil {
			return fmt.Errorf("getting main stream version for %s: %w", group.streamID, err)
		}
		state, err := h.mainReadModelState(ctx, group)
		if err != nil {
			return err
		}
		if state.present && state.version >= head {
			return nil
		}
		if !state.present {
			if len(events) > 0 && events[len(events)-1].Version >= head && endsInDelete(events) {
				return nil // deleted by the last event this pass saw
			}
			// Removed while this pass ran by something that does not write to
			// the stream (a person merge or a delete cascade). Re-projecting
			// from the start would resurrect it; the next resume sees it
			// missing up front and classifies it.
			return fmt.Errorf("%w: stream %s's row was removed from main during the repair; resume again to finish",
				errReprojectRaced, group.streamID)
		}
	}
	return fmt.Errorf("%w: stream %s is still behind main's log after %d attempts; resume again to finish the repair",
		errReprojectRaced, group.streamID, reprojectAttempts)
}

// reprojectForward projects a stream's main events onto main's read model in
// version order, skipping each event the row has already reached. The row is
// re-read before every event, not once, so an event projected concurrently —
// or a newer one — is never re-applied over it.
func (h *Handler) reprojectForward(ctx context.Context, group streamGroup, events []repository.StoredEvent) error {
	for i := range events {
		state, err := h.mainReadModelState(ctx, group)
		if err != nil {
			return err
		}
		if state.present && state.version >= events[i].Version {
			continue
		}
		decoded, err := events[i].DecodeEvent()
		if err != nil {
			return fmt.Errorf("decoding main %s event on stream %s for re-projection: %w", events[i].EventType, group.streamID, err)
		}
		if err := h.projector.Project(ctx, decoded, events[i].Version, domain.MainBranchID); err != nil {
			return fmt.Errorf("re-projecting main %s (version %d) on stream %s: %w",
				events[i].EventType, events[i].Version, group.streamID, err)
		}
	}
	return nil
}

// streamsBehindOnMain returns the already-replayed streams whose main read
// model is missing or behind main's stream version, with what the read model
// holds for each.
func (h *Handler) streamsBehindOnMain(ctx context.Context, groups []streamGroup, landed map[uuid.UUID]bool, mainVersions map[uuid.UUID]int64) ([]streamGroup, map[uuid.UUID]readModelState, error) {
	var behind []streamGroup
	states := make(map[uuid.UUID]readModelState)
	for _, group := range groups {
		if !landed[group.streamID] {
			continue
		}
		state, err := h.mainReadModelState(ctx, group)
		if err != nil {
			return nil, nil, err
		}
		if state.present && state.version >= mainVersions[group.streamID] {
			continue
		}
		states[group.streamID] = state
		behind = append(behind, group)
	}
	return behind, states, nil
}

// missingRowsRemovedElsewhere reports which missing rows were removed by a
// write to ANOTHER stream that main's log records: a person merged into
// another (PersonMerged), a citation whose source was deleted (the
// source→citation cascade), a media item whose owner was deleted (the
// owner→media cascade, #759), or a GPS artifact whose subject was deleted (the
// subject→GPS cascade, #760). Each kind is detected with one set-based scan
// across all the missing streams, never a scan per stream.
//
// It also returns, for each missing media row whose person owner — and each
// missing GPS row whose subject person — main merged into a person it still
// has, the transfer its re-projection must be followed by (see
// missingMediaCascadedAway and missingGPSCascadedAway).
func (h *Handler) missingRowsRemovedElsewhere(
	ctx context.Context,
	missing []streamGroup,
	states map[uuid.UUID]readModelState,
	mainEvents map[uuid.UUID][]repository.StoredEvent,
) (map[uuid.UUID]bool, map[uuid.UUID]mergeRelink, error) {
	personCandidates, personsFrom := missingPersonCandidates(missing, states, mainEvents)
	ownerOf, ownersFrom, err := missingMediaOwners(missing, states, mainEvents)
	if err != nil {
		return nil, nil, err
	}
	subjectOf, subjectsFrom, err := missingGPSSubjects(missing, states, mainEvents)
	if err != nil {
		return nil, nil, err
	}
	// One scan of main for person merges serves all three consumers, starting
	// at the earliest of their start points; each filters it to its own.
	var merges []personMerge
	if scanFrom := earliestScan(earliestScan(personsFrom, ownersFrom), subjectsFrom); scanFrom >= 0 {
		merges, err = h.personMergesOnMain(ctx, scanFrom)
		if err != nil {
			return nil, nil, err
		}
	}
	mergedAway := mergedAwayAfter(personCandidates, personsFrom, merges)
	cascaded, err := h.missingCitationsCascadedAway(ctx, missing, states, mainEvents)
	if err != nil {
		return nil, nil, err
	}
	mediaCascaded, relink, err := h.missingMediaCascadedAway(ctx, missing, ownerOf, survivorsAfter(merges, ownersFrom), ownersFrom)
	if err != nil {
		return nil, nil, err
	}
	gpsCascaded, gpsRelink, err := h.missingGPSCascadedAway(ctx, missing, subjectOf, survivorsAfter(merges, subjectsFrom), subjectsFrom)
	if err != nil {
		return nil, nil, err
	}
	for id, move := range gpsRelink {
		if relink == nil {
			relink = make(map[uuid.UUID]mergeRelink, len(gpsRelink))
		}
		relink[id] = move
	}
	removed := make(map[uuid.UUID]bool, len(mergedAway)+len(cascaded)+len(mediaCascaded)+len(gpsCascaded))
	for _, set := range []map[uuid.UUID]bool{mergedAway, cascaded, mediaCascaded, gpsCascaded} {
		for id := range set {
			removed[id] = true
		}
	}
	return removed, relink, nil
}

// missingPersonCandidates returns the missing person rows that may be
// missing because main merged the person into another, and the position a
// scan for PersonMerged must start from (-1 when there are none). Only
// persons whose stream does not end in a delete are candidates, and the scan
// starts at the earliest point such a merge could sit: after the candidate's
// last event.
func missingPersonCandidates(
	behind []streamGroup,
	states map[uuid.UUID]readModelState,
	mainEvents map[uuid.UUID][]repository.StoredEvent,
) ([]uuid.UUID, int64) {
	var candidates []uuid.UUID
	scanFrom := int64(-1)
	for _, group := range behind {
		events := mainEvents[group.streamID]
		if states[group.streamID].present || len(events) == 0 || !isPersonStream(group.streamType) || endsInDelete(events) {
			continue
		}
		candidates = append(candidates, group.streamID)
		if last := events[len(events)-1].Position; scanFrom < 0 || last < scanFrom {
			scanFrom = last
		}
	}
	return candidates, scanFrom
}

// mergedAwayAfter reports which of the candidate persons main merged into
// another person (PersonMerged.MergedID) after fromPosition.
func mergedAwayAfter(candidates []uuid.UUID, fromPosition int64, merges []personMerge) map[uuid.UUID]bool {
	if len(candidates) == 0 {
		return nil
	}
	survivorOf := survivorsAfter(merges, fromPosition)
	found := make(map[uuid.UUID]bool)
	for _, id := range candidates {
		if _, merged := survivorOf[id]; merged {
			found[id] = true
		}
	}
	return found
}

// earliestScan returns the earlier of two scan start positions, where -1
// means "no scan needed".
func earliestScan(a, b int64) int64 {
	switch {
	case a < 0:
		return b
	case b < 0 || a < b:
		return a
	}
	return b
}

// endsInDelete reports whether a stream's last event deletes its aggregate.
func endsInDelete(events []repository.StoredEvent) bool {
	return len(events) > 0 && strings.HasSuffix(events[len(events)-1].EventType, "Deleted")
}

// mainReadModelState reads main's read-model row for a replayed aggregate. The
// replay set holds only BR-006's branch-aware events. Those a branch can
// actually carry live on person, family, association, source, citation, note,
// media and GPS artifact (evidence analysis, evidence conflict, research log,
// proof summary; #760) streams (#757 made life events and attributes branch-aware too, but
// nothing writes one on a branch yet); any other stream type means a branch
// write path grew without this check, so it is refused rather than reported
// as in sync.
func (h *Handler) mainReadModelState(ctx context.Context, group streamGroup) (readModelState, error) {
	switch {
	case isPersonStream(group.streamType):
		person, err := h.readStore.GetPerson(ctx, domain.MainBranchID, group.streamID)
		if err != nil {
			return readModelState{}, fmt.Errorf("reading main person %s: %w", group.streamID, err)
		}
		if person == nil {
			return readModelState{}, nil
		}
		return readModelState{present: true, version: person.Version}, nil
	case strings.EqualFold(group.streamType, familyStreamType):
		family, err := h.readStore.GetFamily(ctx, domain.MainBranchID, group.streamID)
		if err != nil {
			return readModelState{}, fmt.Errorf("reading main family %s: %w", group.streamID, err)
		}
		if family == nil {
			return readModelState{}, nil
		}
		return readModelState{present: true, version: family.Version}, nil
	case isAssociationStream(group.streamType):
		association, err := h.readStore.GetAssociation(ctx, domain.MainBranchID, group.streamID)
		if err != nil {
			return readModelState{}, fmt.Errorf("reading main association %s: %w", group.streamID, err)
		}
		if association == nil {
			return readModelState{}, nil
		}
		return readModelState{present: true, version: association.Version}, nil
	}
	return h.mainEvidenceState(ctx, group)
}

// mainEvidenceState is mainReadModelState for the evidence streams (#758),
// media (#759) and the GPS artifacts (#760).
func (h *Handler) mainEvidenceState(ctx context.Context, group streamGroup) (readModelState, error) {
	switch {
	case isMediaStream(group.streamType):
		return h.mainMediaState(ctx, group)
	case isGPSStream(group.streamType):
		return h.mainGPSState(ctx, group)
	case isSourceStream(group.streamType):
		source, err := h.readStore.GetSource(ctx, domain.MainBranchID, group.streamID)
		if err != nil {
			return readModelState{}, fmt.Errorf("reading main source %s: %w", group.streamID, err)
		}
		if source == nil {
			return readModelState{}, nil
		}
		return readModelState{present: true, version: source.Version}, nil
	case isCitationStream(group.streamType):
		citation, err := h.readStore.GetCitation(ctx, domain.MainBranchID, group.streamID)
		if err != nil {
			return readModelState{}, fmt.Errorf("reading main citation %s: %w", group.streamID, err)
		}
		if citation == nil {
			return readModelState{}, nil
		}
		return readModelState{present: true, version: citation.Version}, nil
	case isNoteStream(group.streamType):
		note, err := h.readStore.GetNote(ctx, domain.MainBranchID, group.streamID)
		if err != nil {
			return readModelState{}, fmt.Errorf("reading main note %s: %w", group.streamID, err)
		}
		if note == nil {
			return readModelState{}, nil
		}
		return readModelState{present: true, version: note.Version}, nil
	}
	return readModelState{}, fmt.Errorf("cannot verify main's read model for stream %s of type %q after a merge replay", group.streamID, group.streamType)
}

// isPersonStream reports whether a stream type is a person's. Person streams
// are written as both "Person" (commands) and "person" (GEDCOM import).
func isPersonStream(streamType string) bool {
	return strings.EqualFold(streamType, "person")
}

// isAssociationStream reports whether a stream type is an association's.
// Association streams are written as both "Association" (commands) and
// "association" (GEDCOM import).
func isAssociationStream(streamType string) bool {
	return strings.EqualFold(streamType, "association")
}

// readMainStreams reads main's whole history of a set of streams, in one paged
// set-based scan, grouped by stream in position order.
func (h *Handler) readMainStreams(ctx context.Context, streamIDs []uuid.UUID) (map[uuid.UUID][]repository.StoredEvent, error) {
	byStream := make(map[uuid.UUID][]repository.StoredEvent, len(streamIDs))
	var from int64
	for {
		page, err := h.eventStore.ReadStreamsForBranch(ctx, streamIDs, domain.MainBranchID, from, resumeScanPage)
		if err != nil {
			return nil, fmt.Errorf("reading main events for re-projection: %w", err)
		}
		for i := range page {
			byStream[page[i].StreamID] = append(byStream[page[i].StreamID], page[i])
		}
		if len(page) < resumeScanPage {
			return byStream, nil
		}
		from = page[len(page)-1].Position
	}
}
