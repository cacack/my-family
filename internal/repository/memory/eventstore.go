// Package memory provides in-memory implementations of repository interfaces for testing.
package memory

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// EventStore is an in-memory implementation of repository.EventStore for testing.
type EventStore struct {
	mu       sync.RWMutex
	events   []repository.StoredEvent
	streams  map[uuid.UUID][]repository.StoredEvent
	position int64
}

// NewEventStore creates a new in-memory event store.
func NewEventStore() *EventStore {
	return &EventStore{
		streams: make(map[uuid.UUID][]repository.StoredEvent),
	}
}

// Append adds events to a stream with optimistic concurrency control.
func (s *EventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream := s.streams[streamID]
	currentVersion := int64(len(stream))

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
			EventType:  event.EventType(),
			Data:       data,
			Version:    currentVersion,
			Position:   s.position,
			Timestamp:  event.OccurredAt(),
		}

		s.events = append(s.events, stored)
		s.streams[streamID] = append(s.streams[streamID], stored)
	}

	return nil
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

// GetStreamVersion returns the current version of a stream.
func (s *EventStore) GetStreamVersion(ctx context.Context, streamID uuid.UUID) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, exists := s.streams[streamID]
	if !exists {
		return 0, nil
	}
	return int64(len(stream)), nil
}

// Reset clears all data (useful for tests).
func (s *EventStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = nil
	s.streams = make(map[uuid.UUID][]repository.StoredEvent)
	s.position = 0
}

// EventCount returns the total number of events stored.
func (s *EventStore) EventCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}
