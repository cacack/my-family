// Package memory provides in-memory implementations of repository interfaces for testing.
package memory

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// streamBranch keys the version counter by the (stream, branch) pair: versioning
// is per-branch so a branch write never contends with main (ADR-005).
type streamBranch struct {
	streamID uuid.UUID
	branchID domain.BranchID
}

// EventStore is an in-memory implementation of repository.EventStore for testing.
type EventStore struct {
	mu       sync.RWMutex
	events   []repository.StoredEvent
	streams  map[uuid.UUID][]repository.StoredEvent
	versions map[streamBranch]int64
	position int64
}

// NewEventStore creates a new in-memory event store.
func NewEventStore() *EventStore {
	return &EventStore{
		streams:  make(map[uuid.UUID][]repository.StoredEvent),
		versions: make(map[streamBranch]int64),
	}
}

// Append adds events to a stream with optimistic concurrency control.
func (s *EventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := streamBranch{streamID: streamID, branchID: scope.BranchID}
	currentVersion := s.versions[key]

	// A branch's first write to an existing aggregate continues main's version
	// line as of the branch's base position rather than restarting at 1.
	if currentVersion == 0 && !scope.BranchID.IsMain() {
		currentVersion = s.seedVersion(streamID, scope.BasePosition)
	}

	// Check optimistic concurrency
	if expectedVersion >= 0 && currentVersion != expectedVersion {
		return repository.ErrConcurrencyConflict
	}

	// Append events
	for _, event := range events {
		s.position++
		currentVersion++

		data, err := json.Marshal(event)
		if err != nil {
			return err
		}

		stored := repository.StoredEvent{
			ID:         uuid.New(),
			StreamID:   streamID,
			StreamType: streamType,
			BranchID:   scope.BranchID,
			EventType:  event.EventType(),
			Data:       data,
			Version:    currentVersion,
			Position:   s.position,
			Timestamp:  event.OccurredAt(),
		}

		s.events = append(s.events, stored)
		s.streams[streamID] = append(s.streams[streamID], stored)
	}

	// Only record a version once the branch actually holds events for the stream —
	// an empty append must not persist a seeded version the SQL backends (which
	// derive it from MAX(version)) would report as 0.
	if len(events) > 0 {
		s.versions[key] = currentVersion
	}

	return nil
}

// seedVersion returns the aggregate's main version as of basePosition — the
// version a branch's first write to that aggregate continues from. Callers hold
// the lock.
func (s *EventStore) seedVersion(streamID uuid.UUID, basePosition int64) int64 {
	var seed int64
	for _, event := range s.streams[streamID] {
		if event.BranchID.IsMain() && event.Position <= basePosition && event.Version > seed {
			seed = event.Version
		}
	}
	return seed
}

// ReadStream reads all events for a specific aggregate.
func (s *EventStore) ReadStream(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, exists := s.streams[streamID]
	if !exists {
		return nil, nil // Return empty slice, not error, for non-existent streams
	}

	// Return a copy to prevent mutation
	result := make([]repository.StoredEvent, len(stream))
	copy(result, stream)
	return result, nil
}

// ReadAll reads all events from a position for projection rebuilds.
func (s *EventStore) ReadAll(ctx context.Context, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []repository.StoredEvent
	for _, event := range s.events {
		if event.Position > fromPosition {
			result = append(result, event)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

// ReadBranch reads a single branch's own events from a position.
func (s *EventStore) ReadBranch(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []repository.StoredEvent
	for _, event := range s.events {
		if event.BranchID != branchID || event.Position <= fromPosition {
			continue
		}
		result = append(result, event)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

// ReadStreamsForBranch reads one branch's events for a set of streams.
func (s *EventStore) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	if len(streamIDs) == 0 || limit <= 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	wanted := make(map[uuid.UUID]bool, len(streamIDs))
	for _, id := range streamIDs {
		wanted[id] = true
	}

	// s.events is append-ordered, which is position order, so the scan yields the
	// oldest matching events first and the cap can stop it early — the same
	// "ordered then limited by the store" contract the SQL backends get from
	// ORDER BY ... LIMIT.
	var result []repository.StoredEvent
	for _, event := range s.events {
		if event.BranchID != branchID || event.Position <= fromPosition || !wanted[event.StreamID] {
			continue
		}
		result = append(result, event)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

// GetStreamVersion returns the current version of a stream on a branch.
func (s *EventStore) GetStreamVersion(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.versions[streamBranch{streamID: streamID, branchID: branchID}], nil
}

// Reset clears all data (useful for tests).
func (s *EventStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = nil
	s.streams = make(map[uuid.UUID][]repository.StoredEvent)
	s.versions = make(map[streamBranch]int64)
	s.position = 0
}

// EventCount returns the total number of events stored.
func (s *EventStore) EventCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// ReadByStream returns paginated events for a specific stream (entity) on one branch.
func (s *EventStore) ReadByStream(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID, limit, offset int) (*repository.HistoryPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter to the branch BEFORE paginating: TotalCount and HasMore describe the
	// branch's view of the stream, not the whole shared log (ADR-005).
	var stream []repository.StoredEvent
	for _, event := range s.streams[streamID] {
		if event.BranchID == branchID {
			stream = append(stream, event)
		}
	}

	if len(stream) == 0 {
		return &repository.HistoryPage{
			Events:     []repository.StoredEvent{},
			TotalCount: 0,
			HasMore:    false,
		}, nil
	}

	totalCount := len(stream)

	// Apply offset and limit
	start := offset
	if start > totalCount {
		start = totalCount
	}
	end := start + limit
	if end > totalCount {
		end = totalCount
	}

	// Copy the slice to prevent mutation
	result := make([]repository.StoredEvent, end-start)
	copy(result, stream[start:end])

	return &repository.HistoryPage{
		Events:     result,
		TotalCount: totalCount,
		HasMore:    end < totalCount,
	}, nil
}

// ReadGlobalByTime returns paginated events filtered by time range and optional event types.
func (s *EventStore) ReadGlobalByTime(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build a set of event types for fast lookup
	typeFilter := make(map[string]bool)
	for _, t := range eventTypes {
		typeFilter[t] = true
	}
	filterByType := len(eventTypes) > 0

	// Filter events by time and optionally by type
	var filtered []repository.StoredEvent
	for _, event := range s.events {
		if event.Timestamp.Before(fromTime) || event.Timestamp.After(toTime) {
			continue
		}
		if filterByType && !typeFilter[event.EventType] {
			continue
		}
		filtered = append(filtered, event)
	}

	totalCount := len(filtered)

	// Apply offset and limit
	start := offset
	if start > totalCount {
		start = totalCount
	}
	end := start + limit
	if end > totalCount {
		end = totalCount
	}

	result := filtered[start:end]
	if result == nil {
		result = []repository.StoredEvent{}
	}

	return &repository.HistoryPage{
		Events:     result,
		TotalCount: totalCount,
		HasMore:    end < totalCount,
	}, nil
}
