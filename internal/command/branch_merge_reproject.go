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
// may carry (BR-006's branch-aware set: the person and family streams) writes
// the aggregate's read-model version as its LAST step, so the read-model
// version is the number of events on the stream whose projection completed. A
// row behind main's stream version is re-projected from the first event past
// it; re-running an event whose projection stopped midway is safe because each
// of its writes is an upsert or a delete, and the family's child count and
// version are set absolutely (the number of child rows; the event's version,
// never lowered) rather than stepped.
//
// That is also what makes the repair safe to run concurrently. Nothing here
// appends, so no optimistic check serializes two resumes repairing the same
// stream, or a resume and a mainline write to it: both may project the same
// events. Every projection they can run is idempotent in that sense, so the
// row converges on the log whichever finishes last.
//
// A missing row is re-projected from the start unless the entity is gone for a
// reason the log explains: its stream ends in a delete, or (for a person) main
// merged it into another person with PersonMerged, which removes the row
// without writing to the merged person's stream.
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
	mainEvents, err := h.readMainStreams(ctx, streamIDs, 0)
	if err != nil {
		return nil, err
	}
	mergedAway, err := h.missingPersonsMergedAway(ctx, behind, states, mainEvents)
	if err != nil {
		return nil, err
	}

	var repaired []uuid.UUID
	for _, group := range behind {
		events := mainEvents[group.streamID]
		if len(events) == 0 {
			return nil, fmt.Errorf("stream %s is on main by payload id but main has no events for it", group.streamID)
		}
		state := states[group.streamID]
		if !state.present && (endsInDelete(events) || mergedAway[group.streamID]) {
			continue // gone for a reason the log records: nothing to repair
		}
		if err := h.reprojectFrom(ctx, group.streamID, events, state.version); err != nil {
			return nil, err
		}
		repaired = append(repaired, group.streamID)
	}
	return repaired, nil
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

// missingPersonsMergedAway reports which missing person rows are missing
// because main merged the person into another. Only persons whose stream does
// not end in a delete are candidates, and the scan for PersonMerged starts at
// the earliest point such a merge could sit: after the candidate's last event.
func (h *Handler) missingPersonsMergedAway(
	ctx context.Context,
	behind []streamGroup,
	states map[uuid.UUID]readModelState,
	mainEvents map[uuid.UUID][]repository.StoredEvent,
) (map[uuid.UUID]bool, error) {
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
	if len(candidates) == 0 {
		return nil, nil
	}
	return h.personsMergedAwayOnMain(ctx, candidates, scanFrom)
}

// reprojectFrom projects a stream's main events past fromVersion onto main's
// read model, in version order.
func (h *Handler) reprojectFrom(ctx context.Context, streamID uuid.UUID, events []repository.StoredEvent, fromVersion int64) error {
	for i := range events {
		if events[i].Version <= fromVersion {
			continue
		}
		decoded, err := events[i].DecodeEvent()
		if err != nil {
			return fmt.Errorf("decoding main %s event on stream %s for re-projection: %w", events[i].EventType, streamID, err)
		}
		if err := h.projector.Project(ctx, decoded, events[i].Version, domain.MainBranchID); err != nil {
			return fmt.Errorf("re-projecting main %s (version %d) on stream %s: %w",
				events[i].EventType, events[i].Version, streamID, err)
		}
	}
	return nil
}

// endsInDelete reports whether a stream's last event deletes its aggregate.
func endsInDelete(events []repository.StoredEvent) bool {
	return len(events) > 0 && strings.HasSuffix(events[len(events)-1].EventType, "Deleted")
}

// mainReadModelState reads main's read-model row for a replayed aggregate. The
// replay set holds only BR-006's branch-aware events, which live on person and
// family streams; any other stream type means that allowlist grew without this
// check, so it is refused rather than reported as in sync.
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
	}
	return readModelState{}, fmt.Errorf("cannot verify main's read model for stream %s of type %q after a merge replay", group.streamID, group.streamType)
}

// isPersonStream reports whether a stream type is a person's. Person streams
// are written as both "Person" (commands) and "person" (GEDCOM import).
func isPersonStream(streamType string) bool {
	return strings.EqualFold(streamType, "person")
}

// readMainStreams reads main's events on a set of streams after a position, in
// one paged set-based scan, grouped by stream in position order.
func (h *Handler) readMainStreams(ctx context.Context, streamIDs []uuid.UUID, fromPosition int64) (map[uuid.UUID][]repository.StoredEvent, error) {
	byStream := make(map[uuid.UUID][]repository.StoredEvent, len(streamIDs))
	from := fromPosition
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

// personsMergedAwayOnMain reports which of the given persons main merged into
// another person (PersonMerged.MergedID) after fromPosition. It pages through
// main's own events from that point.
func (h *Handler) personsMergedAwayOnMain(ctx context.Context, personIDs []uuid.UUID, fromPosition int64) (map[uuid.UUID]bool, error) {
	wanted := make(map[uuid.UUID]bool, len(personIDs))
	for _, id := range personIDs {
		wanted[id] = true
	}
	found := make(map[uuid.UUID]bool)
	from := fromPosition
	for {
		page, err := h.eventStore.ReadBranch(ctx, domain.MainBranchID, from, resumeScanPage)
		if err != nil {
			return nil, fmt.Errorf("reading main events for person merges: %w", err)
		}
		for i := range page {
			if page[i].EventType != "PersonMerged" {
				continue
			}
			var payload struct {
				MergedID uuid.UUID `json:"merged_id"`
			}
			if err := json.Unmarshal(page[i].Data, &payload); err != nil {
				return nil, fmt.Errorf("decoding PersonMerged at position %d: %w", page[i].Position, err)
			}
			if wanted[payload.MergedID] {
				found[payload.MergedID] = true
			}
		}
		if len(page) < resumeScanPage {
			return found, nil
		}
		from = page[len(page)-1].Position
	}
}
