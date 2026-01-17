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

	// Read events between the two positions
	// We read from fromSnapshot.Position (exclusive) to toSnapshot.Position (inclusive)
	const maxEvents = 1000
	events, err := s.eventStore.ReadAll(ctx, fromSnapshot.Position, maxEvents)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}

	// Filter to only events up to toSnapshot.Position
	var filteredEvents []repository.StoredEvent
	for _, evt := range events {
		if evt.Position <= toSnapshot.Position {
			filteredEvents = append(filteredEvents, evt)
		}
	}

	// Transform to ChangeEntry format using the HistoryService
	changes, err := s.historyService.transformStoredEvents(ctx, filteredEvents)
	if err != nil {
		return nil, fmt.Errorf("transform events: %w", err)
	}

	hasMore := len(events) >= maxEvents

	return &SnapshotComparisonResult{
		Snapshot1:  snapshot1,
		Snapshot2:  snapshot2,
		Changes:    changes,
		TotalCount: len(changes),
		HasMore:    hasMore,
		OlderFirst: olderFirst,
	}, nil
}
