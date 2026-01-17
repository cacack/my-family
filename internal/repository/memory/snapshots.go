package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// SnapshotStore is an in-memory implementation of repository.SnapshotStore for testing.
type SnapshotStore struct {
	mu         sync.RWMutex
	snapshots  map[uuid.UUID]*domain.Snapshot
	eventStore *EventStore // Reference to event store for getting max position
}

// NewSnapshotStore creates a new in-memory snapshot store.
func NewSnapshotStore(eventStore *EventStore) *SnapshotStore {
	return &SnapshotStore{
		snapshots:  make(map[uuid.UUID]*domain.Snapshot),
		eventStore: eventStore,
	}
}

// Create stores a new snapshot.
func (s *SnapshotStore) Create(_ context.Context, snapshot *domain.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy to prevent external mutation
	copied := *snapshot
	s.snapshots[snapshot.ID] = &copied
	return nil
}

// Get retrieves a snapshot by ID.
func (s *SnapshotStore) Get(_ context.Context, id uuid.UUID) (*domain.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot, exists := s.snapshots[id]
	if !exists {
		return nil, repository.ErrSnapshotNotFound
	}

	// Return a copy to prevent mutation
	copied := *snapshot
	return &copied, nil
}

// List retrieves all snapshots ordered by created_at DESC.
func (s *SnapshotStore) List(_ context.Context) ([]*domain.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*domain.Snapshot, 0, len(s.snapshots))
	for _, snapshot := range s.snapshots {
		// Make a copy
		copied := *snapshot
		result = append(result, &copied)
	}

	// Sort by created_at DESC
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// Delete removes a snapshot by ID.
func (s *SnapshotStore) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.snapshots[id]; !exists {
		return repository.ErrSnapshotNotFound
	}

	delete(s.snapshots, id)
	return nil
}

// GetMaxPosition returns the current maximum position from the event store.
func (s *SnapshotStore) GetMaxPosition(_ context.Context) (int64, error) {
	// Access the event store's position directly
	// The event store tracks the max position internally
	s.eventStore.mu.RLock()
	defer s.eventStore.mu.RUnlock()
	return s.eventStore.position, nil
}

// Reset clears all data (useful for tests).
func (s *SnapshotStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots = make(map[uuid.UUID]*domain.Snapshot)
}
