// Package sqlite provides SQLite implementations of repository interfaces.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

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
	defer func() { _ = tx.Rollback() }()

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
			version, position                                                int64
			metadataStr                                                      sql.NullString
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

// ReadByStream returns paginated events for a specific stream (entity).
// Results are ordered by version ascending.
func (s *EventStore) ReadByStream(ctx context.Context, streamID uuid.UUID, limit, offset int) (*repository.HistoryPage, error) {
	// Query with window function for total count
	query := `
		SELECT
			id, stream_id, stream_type, version, event_type, data, metadata, timestamp, position,
			COUNT(*) OVER() as total_count
		FROM events
		WHERE stream_id = ?
		ORDER BY version ASC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, query, streamID.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query events by stream: %w", err)
	}
	defer rows.Close()

	var events []repository.StoredEvent
	var totalCount int

	for rows.Next() {
		var (
			idStr, streamIDStr, streamType, eventType, dataStr, timestampStr string
			version, position                                                int64
			metadataStr                                                      sql.NullString
		)
		err := rows.Scan(&idStr, &streamIDStr, &streamType, &version, &eventType, &dataStr, &metadataStr, &timestampStr, &position, &totalCount)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		id, _ := uuid.Parse(idStr)
		sid, _ := uuid.Parse(streamIDStr)

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   sid,
			StreamType: streamType,
			EventType:  eventType,
			Data:       []byte(dataStr),
			Version:    version,
			Position:   position,
		}

		if metadataStr.Valid {
			event.Metadata = []byte(metadataStr.String)
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

	// Return empty page if no results
	if len(events) == 0 {
		return &repository.HistoryPage{
			Events:     []repository.StoredEvent{},
			TotalCount: 0,
			HasMore:    false,
		}, nil
	}

	hasMore := offset+len(events) < totalCount

	return &repository.HistoryPage{
		Events:     events,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// ReadGlobalByTime returns paginated events filtered by time range and optional event types.
// Results are ordered by timestamp ascending.
func (s *EventStore) ReadGlobalByTime(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error) {
	// Build WHERE clause dynamically
	var whereClauses []string
	var args []any

	// Handle time boundaries
	if !fromTime.IsZero() {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, formatTimestamp(fromTime))
	}

	if !toTime.IsZero() {
		whereClauses = append(whereClauses, "timestamp <= ?")
		args = append(args, formatTimestamp(toTime))
	}

	// Handle event type filter
	if len(eventTypes) > 0 {
		placeholders := ""
		for i, et := range eventTypes {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
			args = append(args, et)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("event_type IN (%s)", placeholders))
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + whereClauses[0]
		for i := 1; i < len(whereClauses); i++ {
			whereClause += " AND " + whereClauses[i]
		}
	}

	// Add limit and offset to args
	args = append(args, limit, offset)

	// Query with window function for total count
	// #nosec G201 -- whereClause contains only hardcoded SQL fragments; user values are parameterized in args
	query := fmt.Sprintf(`
		SELECT
			id, stream_id, stream_type, version, event_type, data, metadata, timestamp, position,
			COUNT(*) OVER() as total_count
		FROM events
		%s
		ORDER BY timestamp ASC
		LIMIT ? OFFSET ?
	`, whereClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events by time: %w", err)
	}
	defer rows.Close()

	var events []repository.StoredEvent
	var totalCount int

	for rows.Next() {
		var (
			idStr, streamIDStr, streamType, eventType, dataStr, timestampStr string
			version, position                                                int64
			metadataStr                                                      sql.NullString
		)
		err := rows.Scan(&idStr, &streamIDStr, &streamType, &version, &eventType, &dataStr, &metadataStr, &timestampStr, &position, &totalCount)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		id, _ := uuid.Parse(idStr)
		sid, _ := uuid.Parse(streamIDStr)

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   sid,
			StreamType: streamType,
			EventType:  eventType,
			Data:       []byte(dataStr),
			Version:    version,
			Position:   position,
		}

		if metadataStr.Valid {
			event.Metadata = []byte(metadataStr.String)
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

	// Return empty page if no results
	if len(events) == 0 {
		return &repository.HistoryPage{
			Events:     []repository.StoredEvent{},
			TotalCount: 0,
			HasMore:    false,
		}, nil
	}

	hasMore := offset+len(events) < totalCount

	return &repository.HistoryPage{
		Events:     events,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// Close closes the database connection.
func (s *EventStore) Close() error {
	return s.db.Close()
}
