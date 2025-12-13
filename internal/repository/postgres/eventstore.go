// Package postgres provides PostgreSQL implementations of repository interfaces.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// EventStore is a PostgreSQL implementation of repository.EventStore.
type EventStore struct {
	db *sql.DB
	mu sync.Mutex // serialize writes for consistency
}

// NewEventStore creates a new PostgreSQL event store.
func NewEventStore(db *sql.DB) (*EventStore, error) {
	store := &EventStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return store, nil
}

// createTables creates the event store schema if it doesn't exist.
func (s *EventStore) createTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS streams (
			id UUID PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			metadata JSONB
		);

		CREATE TABLE IF NOT EXISTS events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			stream_id UUID NOT NULL REFERENCES streams(id),
			stream_type VARCHAR(50) NOT NULL,
			version BIGINT NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			data JSONB NOT NULL,
			metadata JSONB,
			timestamp TIMESTAMPTZ NOT NULL,
			position BIGSERIAL UNIQUE,
			UNIQUE(stream_id, version)
		);

		CREATE INDEX IF NOT EXISTS idx_events_stream_version ON events(stream_id, version);
		CREATE INDEX IF NOT EXISTS idx_events_position ON events(position);
		CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type, timestamp);
	`)
	return err
}

// Append adds events to a stream with optimistic concurrency control.
func (s *EventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current version
	var currentVersion int64
	err = tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = $1",
		streamID,
	).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	// Check optimistic concurrency
	if expectedVersion >= 0 && currentVersion != expectedVersion {
		return repository.ErrConcurrencyConflict
	}

	// Ensure stream exists
	if currentVersion == 0 && expectedVersion == -1 {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO streams (id, type) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING",
			streamID, streamType,
		)
		if err != nil {
			return fmt.Errorf("create stream: %w", err)
		}
	}

	// Append events
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (stream_id, stream_type, version, event_type, data, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6)
	`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		currentVersion++

		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}

		_, err = stmt.ExecContext(ctx,
			streamID,
			streamType,
			currentVersion,
			event.EventType(),
			data,
			event.OccurredAt(),
		)
		if err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}

	return tx.Commit()
}

// ReadStream reads all events for a specific aggregate.
func (s *EventStore) ReadStream(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stream_id, stream_type, version, event_type, data, metadata, timestamp, position
		FROM events
		WHERE stream_id = $1
		ORDER BY version ASC
	`, streamID)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ReadAll reads all events from a position for projection rebuilds.
func (s *EventStore) ReadAll(ctx context.Context, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stream_id, stream_type, version, event_type, data, metadata, timestamp, position
		FROM events
		WHERE position > $1
		ORDER BY position ASC
		LIMIT $2
	`, fromPosition, limit)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// GetStreamVersion returns the current version of a stream.
func (s *EventStore) GetStreamVersion(ctx context.Context, streamID uuid.UUID) (int64, error) {
	var version int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = $1",
		streamID,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get stream version: %w", err)
	}
	return version, nil
}

// scanEvents scans rows into StoredEvent slice.
func scanEvents(rows *sql.Rows) ([]repository.StoredEvent, error) {
	var events []repository.StoredEvent
	for rows.Next() {
		var (
			id, streamID          uuid.UUID
			streamType, eventType string
			version, position     int64
			data                  []byte
			metadata              sql.NullString
			timestamp             time.Time
		)
		err := rows.Scan(&id, &streamID, &streamType, &version, &eventType, &data, &metadata, &timestamp, &position)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   streamID,
			StreamType: streamType,
			EventType:  eventType,
			Data:       json.RawMessage(data),
			Version:    version,
			Position:   position,
			Timestamp:  timestamp,
		}

		if metadata.Valid {
			event.Metadata = json.RawMessage(metadata.String)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	return events, nil
}

// Close closes the database connection.
func (s *EventStore) Close() error {
	return s.db.Close()
}
