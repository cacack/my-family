// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// SnapshotService provides query operations for research milestone snapshots.
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

// CreateSnapshot creates a new snapshot capturing the current max position from the event store.
func (s *SnapshotService) CreateSnapshot(ctx context.Context, name, description string) (*domain.Snapshot, error) {
	// Get the current max position from the event store
	position, err := s.snapshotStore.GetMaxPosition(ctx)
	if err != nil {
		return nil, fmt.Errorf("get max position: %w", err)
	}

	// Create the snapshot
	snapshot, err := domain.NewSnapshot(name, description, position)
	if err != nil {
		return nil, fmt.Errorf("create snapshot: %w", err)
	}

	// Store the snapshot
	if err := s.snapshotStore.Create(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("store snapshot: %w", err)
	}

	return snapshot, nil
}

// ListSnapshots returns all snapshots ordered by created_at DESC.
func (s *SnapshotService) ListSnapshots(ctx context.Context) ([]*domain.Snapshot, error) {
	return s.snapshotStore.List(ctx)
}

// GetSnapshot retrieves a single snapshot by ID.
func (s *SnapshotService) GetSnapshot(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error) {
	return s.snapshotStore.Get(ctx, id)
}

// DeleteSnapshot removes a snapshot (events remain untouched).
func (s *SnapshotService) DeleteSnapshot(ctx context.Context, id uuid.UUID) error {
	return s.snapshotStore.Delete(ctx, id)
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

// CompareSnapshots returns the events between two snapshot positions.
// The snapshots are ordered by position (older first) to show changes chronologically.
func (s *SnapshotService) CompareSnapshots(ctx context.Context, id1, id2 uuid.UUID) (*SnapshotComparisonResult, error) {
	// Get both snapshots
	snapshot1, err := s.snapshotStore.Get(ctx, id1)
	if err != nil {
		return nil, fmt.Errorf("get snapshot 1: %w", err)
	}

	snapshot2, err := s.snapshotStore.Get(ctx, id2)
	if err != nil {
		return nil, fmt.Errorf("get snapshot 2: %w", err)
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

	// Read the MAINLINE's events after the older position. Snapshots mark
	// positions in the shared log (ADR-005), and that log also carries every
	// research branch's deltas; a branch's unmerged edits are not part of the
	// mainline's history between two milestones, so reading ReadAll here would
	// report them as if they had happened on main.
	events, err := s.eventStore.ReadBranch(ctx, domain.MainBranchID, fromSnapshot.Position, maxComparisonEvents)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}

	// Keep only events up to the newer position (inclusive). The read above is
	// bounded by count, not by position, so it can run past toSnapshot.
	filteredEvents := make([]repository.StoredEvent, 0, len(events))
	reachedEnd := false
	for _, evt := range events {
		if evt.Position > toSnapshot.Position {
			reachedEnd = true
			break
		}
		filteredEvents = append(filteredEvents, evt)
	}

	// Transform to ChangeEntry format using the HistoryService
	changes, err := s.historyService.transformStoredEvents(ctx, filteredEvents)
	if err != nil {
		return nil, fmt.Errorf("transform events: %w", err)
	}

	// The window was truncated only if the read hit its cap before it reached
	// the newer snapshot. Once an event at or past that position was seen,
	// every event in range is already in hand, however full the read was.
	hasMore := !reachedEnd && len(events) >= maxComparisonEvents &&
		events[len(events)-1].Position < toSnapshot.Position

	return &SnapshotComparisonResult{
		Snapshot1:  snapshot1,
		Snapshot2:  snapshot2,
		Changes:    changes,
		TotalCount: len(changes),
		HasMore:    hasMore,
		OlderFirst: olderFirst,
	}, nil
}
