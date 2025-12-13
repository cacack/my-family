// Package sqlite provides SQLite implementations of repository interfaces.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// EventStore is a SQLite implementation of repository.EventStore.
type EventStore struct {
	db *sql.DB
	mu sync.Mutex // serialize writes for SQLite
}

// NewEventStore creates a new SQLite event store.
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
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			metadata TEXT
		);

		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			stream_id TEXT NOT NULL,
			stream_type TEXT NOT NULL,
			version INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			data TEXT NOT NULL,
			metadata TEXT,
			timestamp TEXT NOT NULL,
			position INTEGER NOT NULL,
			FOREIGN KEY (stream_id) REFERENCES streams(id),
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
		"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = ?",
		streamID.String(),
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
			"INSERT OR IGNORE INTO streams (id, type) VALUES (?, ?)",
			streamID.String(), streamType,
		)
		if err != nil {
			return fmt.Errorf("create stream: %w", err)
		}
	}

	// Get max position
	var maxPosition int64
	err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), 0) FROM events").Scan(&maxPosition)
	if err != nil {
		return fmt.Errorf("get max position: %w", err)
	}

	// Append events
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (id, stream_id, stream_type, version, event_type, data, timestamp, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		maxPosition++
		currentVersion++

		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}

		_, err = stmt.ExecContext(ctx,
			uuid.New().String(),
			streamID.String(),
			streamType,
			currentVersion,
			event.EventType(),
			string(data),
			event.OccurredAt().Format("2006-01-02T15:04:05.999999999Z07:00"),
			maxPosition,
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
		WHERE stream_id = ?
		ORDER BY version ASC
	`, streamID.String())
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
		WHERE position > ?
		ORDER BY position ASC
		LIMIT ?
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
		"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = ?",
		streamID.String(),
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
			idStr, streamIDStr, streamType, eventType, dataStr, timestampStr string
			version, position                                                 int64
			metadataStr                                                       sql.NullString
		)
		err := rows.Scan(&idStr, &streamIDStr, &streamType, &version, &eventType, &dataStr, &metadataStr, &timestampStr, &position)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		id, _ := uuid.Parse(idStr)
		streamID, _ := uuid.Parse(streamIDStr)

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   streamID,
			StreamType: streamType,
			EventType:  eventType,
			Data:       json.RawMessage(dataStr),
			Version:    version,
			Position:   position,
		}

		if metadataStr.Valid {
			event.Metadata = json.RawMessage(metadataStr.String)
		}

		// Parse timestamp
		ts, err := parseTimestamp(timestampStr)
		if err == nil {
			event.Timestamp = ts
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
