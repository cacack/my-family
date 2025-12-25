// Package repository provides data access interfaces and implementations.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// Common errors for event store operations.
var (
	ErrStreamNotFound      = errors.New("stream not found")
	ErrConcurrencyConflict = errors.New("concurrency conflict: expected version mismatch")
	ErrEventNotFound       = errors.New("event not found")
)

// EventStore provides append-only storage for domain events.
type EventStore interface {
	// Append adds events to a stream with optimistic concurrency control.
	// Returns ErrConcurrencyConflict if expectedVersion doesn't match current version.
	// Use expectedVersion=-1 for new streams.
	Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64) error

	// ReadStream reads all events for a specific aggregate.
	ReadStream(ctx context.Context, streamID uuid.UUID) ([]StoredEvent, error)

	// ReadAll reads all events from a position for projection rebuilds.
	ReadAll(ctx context.Context, fromPosition int64, limit int) ([]StoredEvent, error)

	// GetStreamVersion returns the current version of a stream.
	GetStreamVersion(ctx context.Context, streamID uuid.UUID) (int64, error)

	// ReadByStream returns paginated events for a specific stream (entity).
	// Results are ordered by version ascending.
	// Parameters:
	//   - streamID: The UUID of the stream to query
	//   - limit: Maximum number of events to return
	//   - offset: Number of events to skip (for pagination)
	// Returns a HistoryPage with events, total count, and hasMore flag.
	ReadByStream(ctx context.Context, streamID uuid.UUID, limit, offset int) (*HistoryPage, error)

	// ReadGlobalByTime returns paginated events filtered by time range and optional event types.
	// Results are ordered by timestamp ascending.
	// Parameters:
	//   - fromTime: Start of time range (inclusive)
	//   - toTime: End of time range (inclusive)
	//   - eventTypes: Optional list of event types to filter (nil or empty means all types)
	//   - limit: Maximum number of events to return
	//   - offset: Number of events to skip (for pagination)
	// Returns a HistoryPage with events, total count, and hasMore flag.
	ReadGlobalByTime(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*HistoryPage, error)
}

// StoredEvent represents an event as stored in the event store.
type StoredEvent struct {
	ID         uuid.UUID       `json:"id"`
	StreamID   uuid.UUID       `json:"stream_id"`
	StreamType string          `json:"stream_type"`
	EventType  string          `json:"event_type"`
	Data       json.RawMessage `json:"data"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	Version    int64           `json:"version"`
	Position   int64           `json:"position"`
	Timestamp  time.Time       `json:"timestamp"`
}

// HistoryPage represents a paginated result set of events for history queries.
type HistoryPage struct {
	Events     []StoredEvent `json:"events"`
	TotalCount int           `json:"total_count"`
	HasMore    bool          `json:"has_more"`
}

// DecodeEvent decodes the stored event data into a domain event.
func (e *StoredEvent) DecodeEvent() (domain.Event, error) {
	switch e.EventType {
	case "PersonCreated":
		var event domain.PersonCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "PersonUpdated":
		var event domain.PersonUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "PersonDeleted":
		var event domain.PersonDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "FamilyCreated":
		var event domain.FamilyCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "FamilyUpdated":
		var event domain.FamilyUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ChildLinkedToFamily":
		var event domain.ChildLinkedToFamily
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ChildUnlinkedFromFamily":
		var event domain.ChildUnlinkedFromFamily
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "FamilyDeleted":
		var event domain.FamilyDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "GedcomImported":
		var event domain.GedcomImported
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SourceCreated":
		var event domain.SourceCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SourceUpdated":
		var event domain.SourceUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SourceDeleted":
		var event domain.SourceDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "CitationCreated":
		var event domain.CitationCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "CitationUpdated":
		var event domain.CitationUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "CitationDeleted":
		var event domain.CitationDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	default:
		return nil, errors.New("unknown event type: " + e.EventType)
	}
}

// EncodeEvent creates a StoredEvent from a domain event.
func EncodeEvent(streamID uuid.UUID, streamType string, event domain.Event, version, position int64) (StoredEvent, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return StoredEvent{}, err
	}

	return StoredEvent{
		ID:         uuid.New(),
		StreamID:   streamID,
		StreamType: streamType,
		EventType:  event.EventType(),
		Data:       data,
		Version:    version,
		Position:   position,
		Timestamp:  event.OccurredAt(),
	}, nil
}
