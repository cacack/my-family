// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Snapshot comparison errors.
var (
	// ErrSnapshotBranchMismatch is returned when a comparison names a snapshot
	// marked on a different branch than the one the request is scoped to — or
	// two snapshots marked on different branches. A snapshot marks a position
	// in ONE branch's view of the log (ADR-005, issue #839); the range between
	// positions in two different views is not a history anyone saw, so the
	// comparison is refused rather than answered from either view.
	ErrSnapshotBranchMismatch = errors.New("snapshots on different branches cannot be compared")
)

// SnapshotService provides query operations for research milestone snapshots.
//
// Reads only. Creating and deleting a snapshot are commands on
// command.Handler — they append SnapshotCreated / SnapshotDeleted and let the
// projection write this store (issue #624). A mutating method here would be a
// second path to the registry that bypasses the event log, which is the exact
// gap #624 closed.
//
// Every read is scoped to a branch (issue #839): a snapshot marks
// (branch_id, position), and a branch sees only its own snapshots. Pass
// domain.MainBranchID for the mainline.
type SnapshotService struct {
	snapshotStore  repository.SnapshotStore
	eventStore     repository.EventStore
	historyService *HistoryService
}

// NewSnapshotService creates a new snapshot query service.
func NewSnapshotService(snapshotStore repository.SnapshotStore, eventStore repository.EventStore, historyService *HistoryService) *SnapshotService {
	return &SnapshotService{
		snapshotStore:  snapshotStore,
		eventStore:     eventStore,
		historyService: historyService,
	}
}

// ListSnapshots returns branchID's snapshots ordered by created_at DESC.
func (s *SnapshotService) ListSnapshots(ctx context.Context, branchID domain.BranchID) ([]*domain.Snapshot, error) {
	return s.snapshotStore.List(ctx, branchID)
}

// GetSnapshot retrieves a single snapshot of branchID's by ID. A snapshot marked
// on another branch is not found in this scope.
func (s *SnapshotService) GetSnapshot(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*domain.Snapshot, error) {
	snapshot, err := s.snapshotStore.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if snapshot.BranchID != branchID {
		return nil, repository.ErrSnapshotNotFound
	}
	return snapshot, nil
}

// SnapshotComparisonResult contains the events between two snapshot positions.
type SnapshotComparisonResult struct {
	Snapshot1  *domain.Snapshot `json:"snapshot1"`
	Snapshot2  *domain.Snapshot `json:"snapshot2"`
	Changes    []ChangeEntry    `json:"changes"`
	TotalCount int              `json:"total_count"`
	HasMore    bool             `json:"has_more"`
	OlderFirst bool             `json:"older_first"` // true if snapshot1 is older
}

// SnapshotCurrentComparisonResult contains the events between a snapshot and
// the current head of the log, in the snapshot's branch's view.
type SnapshotCurrentComparisonResult struct {
	Snapshot     *domain.Snapshot `json:"snapshot"`
	HeadPosition int64            `json:"head_position"`
	Changes      []ChangeEntry    `json:"changes"`
	TotalCount   int              `json:"total_count"`
	HasMore      bool             `json:"has_more"`
}

// CompareSnapshots returns the changes between two snapshots of branchID's, in
// branchID's view of the log (see changesBetween). The snapshots are ordered by
// position (older first) to show changes chronologically.
//
// An unknown id is repository.ErrSnapshotNotFound. A snapshot marked on a
// branch other than branchID — or two snapshots on different branches — is
// ErrSnapshotBranchMismatch.
func (s *SnapshotService) CompareSnapshots(ctx context.Context, branchID domain.BranchID, id1, id2 uuid.UUID) (*SnapshotComparisonResult, error) {
	snapshot1, err := s.snapshotStore.Get(ctx, id1)
	if err != nil {
		return nil, fmt.Errorf("get snapshot 1: %w", err)
	}

	snapshot2, err := s.snapshotStore.Get(ctx, id2)
	if err != nil {
		return nil, fmt.Errorf("get snapshot 2: %w", err)
	}

	if snapshot1.BranchID != snapshot2.BranchID || snapshot1.BranchID != branchID {
		return nil, ErrSnapshotBranchMismatch
	}

	// Order by position (older first)
	olderFirst := true
	fromSnapshot := snapshot1
	toSnapshot := snapshot2
	if snapshot1.Position > snapshot2.Position {
		fromSnapshot = snapshot2
		toSnapshot = snapshot1
		olderFirst = false
	}

	changes, hasMore, err := s.changesBetween(ctx, branchID, fromSnapshot.Position, toSnapshot.Position)
	if err != nil {
		return nil, err
	}

	return &SnapshotComparisonResult{
		Snapshot1:  snapshot1,
		Snapshot2:  snapshot2,
		Changes:    changes,
		TotalCount: len(changes),
		HasMore:    hasMore,
		OlderFirst: olderFirst,
	}, nil
}

// CompareSnapshotToCurrent returns the changes from a snapshot of branchID's to
// the current head of the log — "what changed since Pre-DNA?" without taking a
// second snapshot first — in branchID's view (see changesBetween).
//
// The head is the global log head read now, the same position a snapshot
// taken now would mark. An unknown id is repository.ErrSnapshotNotFound; a
// snapshot on another branch is ErrSnapshotBranchMismatch.
func (s *SnapshotService) CompareSnapshotToCurrent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*SnapshotCurrentComparisonResult, error) {
	snapshot, err := s.snapshotStore.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get snapshot: %w", err)
	}
	if snapshot.BranchID != branchID {
		return nil, ErrSnapshotBranchMismatch
	}

	head, err := s.snapshotStore.GetMaxPosition(ctx)
	if err != nil {
		return nil, fmt.Errorf("get log head: %w", err)
	}

	changes, hasMore, err := s.changesBetween(ctx, branchID, snapshot.Position, head)
	if err != nil {
		return nil, err
	}

	return &SnapshotCurrentComparisonResult{
		Snapshot:     snapshot,
		HeadPosition: head,
		Changes:      changes,
		TotalCount:   len(changes),
		HasMore:      hasMore,
	}, nil
}

// changesBetween returns the changes recorded in branchID's view of the log in
// the position range (from, to], oldest first, and whether the range held more
// events than one comparison reads (maxComparisonEvents per source).
//
// On the mainline the view is the mainline's own events; research branches'
// deltas share the log but are not the mainline's history.
//
// On a branch the view follows the copy-on-write overlay exactly as the
// branch's entity history does (ADR-005, #824 — GetEntityHistoryOn):
//
//   - the branch's own events in the range (Origin "branch"), plus
//   - the mainline events in the range the branch's view inherits (Origin
//     "main"): an event on an entity the branch had not yet written when the
//     event happened. The overlay is live, so until the branch's first write to
//     an entity a branch read shows the mainline's current row; a mainline
//     edit made after that first write is not in the branch's data and is left
//     out (it surfaces in the branch compare and at merge instead).
//
// A branch snapshot is taken after its branch forked, so both ends of the range
// are at or after the fork and nothing before base_position is ever in range.
// Other branches' events never appear.
func (s *SnapshotService) changesBetween(ctx context.Context, branchID domain.BranchID, from, to int64) ([]ChangeEntry, bool, error) {
	if to <= from {
		return []ChangeEntry{}, false, nil
	}

	mainEvents, err := s.eventStore.ReadBranch(ctx, domain.MainBranchID, from, maxComparisonEvents)
	if err != nil {
		return nil, false, fmt.Errorf("read events: %w", err)
	}
	cut := truncationCut(mainEvents, to)

	if branchID.IsMain() {
		inRange := eventsUpTo(mainEvents, cut)
		changes, err := s.historyService.transformStoredEvents(ctx, inRange)
		if err != nil {
			return nil, false, fmt.Errorf("transform events: %w", err)
		}
		return changes, cut < to, nil
	}

	ownEvents, err := s.eventStore.ReadBranch(ctx, branchID, from, maxComparisonEvents)
	if err != nil {
		return nil, false, fmt.Errorf("read branch events: %w", err)
	}
	// The window ends where the first of the two reads ran out, so neither
	// source reports events past a point the other could not see.
	cut = min(cut, truncationCut(ownEvents, to))
	own := eventsUpTo(ownEvents, cut)
	mainInRange := eventsUpTo(mainEvents, cut)

	inherited, err := s.inheritedMainEvents(ctx, branchID, from, own, mainInRange)
	if err != nil {
		return nil, false, err
	}

	merged := make([]repository.StoredEvent, 0, len(own)+len(inherited))
	merged = append(merged, own...)
	merged = append(merged, inherited...)
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Position < merged[j].Position })

	changes, err := s.historyService.transformStoredEventsOn(ctx, branchID, merged)
	if err != nil {
		return nil, false, fmt.Errorf("transform events: %w", err)
	}
	origins := make(map[uuid.UUID]string, len(merged))
	for i := range merged {
		if merged[i].BranchID == branchID {
			origins[merged[i].ID] = ChangeOriginBranch
		} else {
			origins[merged[i].ID] = ChangeOriginMain
		}
	}
	for i := range changes {
		changes[i].Origin = origins[changes[i].ID]
	}
	return changes, cut < to, nil
}

// inheritedMainEvents filters mainline events in the range (from, cut] down to
// those branchID's view inherits: events on a stream the branch had not written
// at that position. own is the branch's events in the same range; the branch's
// writes at or before `from` are found with one set-based read over the streams
// the mainline touched (paged, never one query per stream).
func (s *SnapshotService) inheritedMainEvents(ctx context.Context, branchID domain.BranchID, from int64, own, mainInRange []repository.StoredEvent) ([]repository.StoredEvent, error) {
	if len(mainInRange) == 0 {
		return nil, nil
	}

	streamSet := make(map[uuid.UUID]struct{}, len(mainInRange))
	streamIDs := make([]uuid.UUID, 0, len(mainInRange))
	for i := range mainInRange {
		id := mainInRange[i].StreamID
		if _, seen := streamSet[id]; !seen {
			streamSet[id] = struct{}{}
			streamIDs = append(streamIDs, id)
		}
	}

	// firstWrite[stream] = the position of the branch's first write to it that
	// matters here: any write at or before `from` counts as "already written".
	firstWrite := make(map[uuid.UUID]int64, len(streamIDs))
	var cursor int64
	for {
		page, err := s.eventStore.ReadStreamsForBranch(ctx, streamIDs, branchID, cursor, maxComparisonEvents)
		if err != nil {
			return nil, fmt.Errorf("read branch writes before the range: %w", err)
		}
		done := len(page) < maxComparisonEvents
		for i := range page {
			if page[i].Position > from {
				done = true
				break
			}
			if _, ok := firstWrite[page[i].StreamID]; !ok {
				firstWrite[page[i].StreamID] = page[i].Position
			}
		}
		if done {
			break
		}
		cursor = page[len(page)-1].Position
	}
	for i := range own {
		if _, ok := firstWrite[own[i].StreamID]; !ok {
			firstWrite[own[i].StreamID] = own[i].Position
		}
	}

	inherited := make([]repository.StoredEvent, 0, len(mainInRange))
	for i := range mainInRange {
		first, wrote := firstWrite[mainInRange[i].StreamID]
		if !wrote || mainInRange[i].Position < first {
			inherited = append(inherited, mainInRange[i])
		}
	}
	return inherited, nil
}

// truncationCut returns the last position a capped read can vouch for within a
// range ending at `to`: `to` itself when the read reached the end of the range
// (or ran out of events), otherwise the position of the last event it returned.
func truncationCut(events []repository.StoredEvent, to int64) int64 {
	if len(events) < maxComparisonEvents {
		return to
	}
	last := events[len(events)-1].Position
	if last >= to {
		return to
	}
	return last
}

// eventsUpTo returns the prefix of position-ordered events at or before limit.
func eventsUpTo(events []repository.StoredEvent, limit int64) []repository.StoredEvent {
	n := sort.Search(len(events), func(i int) bool { return events[i].Position > limit })
	return events[:n]
}
