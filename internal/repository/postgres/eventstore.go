// Package postgres provides PostgreSQL implementations of repository interfaces.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

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
// The events table carries a branch_id envelope column and versions events per
// (stream_id, branch_id) (ADR-005); existing databases are migrated in place —
// an idempotent ADD COLUMN backfills rows to MainBranchID through the column
// default, and the per-stream UNIQUE constraint/index is swapped for the
// composite one. Uniqueness lives in idx_events_stream_branch_version rather
// than a table constraint so the fresh and upgraded paths converge on exactly
// one mechanism.
func (s *EventStore) createTables() error {
	// #nosec G201 -- mainBranch is a constant UUID literal, not user input.
	mainBranch := domain.MainBranchID.String()
	_, err := s.db.Exec(fmt.Sprintf(`
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
			branch_id UUID NOT NULL DEFAULT '%[1]s',
			version BIGINT NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			data JSONB NOT NULL,
			metadata JSONB,
			timestamp TIMESTAMPTZ NOT NULL,
			position BIGSERIAL UNIQUE
		);

		ALTER TABLE events ADD COLUMN IF NOT EXISTS branch_id UUID NOT NULL DEFAULT '%[1]s';

		-- Per-branch versioning (ADR-005): drop the pre-branch per-stream uniqueness.
		-- events_stream_id_version_key is the name Postgres auto-generated for the
		-- table-level UNIQUE(stream_id, version); idx_events_stream_version was its
		-- companion lookup index. Both are superseded by the composite unique index
		-- below. CREATE INDEX IF NOT EXISTS cannot redefine an existing index, hence
		-- the explicit DROP of the old one plus a new name; both are no-ops once done.
		ALTER TABLE events DROP CONSTRAINT IF EXISTS events_stream_id_version_key;
		DROP INDEX IF EXISTS idx_events_stream_version;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_events_stream_branch_version ON events(stream_id, branch_id, version);

		CREATE INDEX IF NOT EXISTS idx_events_position ON events(position);
		CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type, timestamp);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp_position ON events(timestamp, position);
		CREATE INDEX IF NOT EXISTS idx_events_branch ON events(branch_id);
		CREATE INDEX IF NOT EXISTS idx_events_stream_branch ON events(stream_id, branch_id);
	`, mainBranch))
	return err
}

// Append adds events to a stream with optimistic concurrency control.
func (s *EventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get current version for this stream ON THIS BRANCH
	var currentVersion int64
	err = tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = $1 AND branch_id = $2",
		streamID, scope.BranchID.UUID(),
	).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	// A branch's first write to an existing aggregate continues main's version line
	// as of the branch's base position rather than restarting at 1 (ADR-005).
	if currentVersion == 0 && !scope.BranchID.IsMain() {
		err = tx.QueryRowContext(ctx,
			"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = $1 AND branch_id = $2 AND position <= $3",
			streamID, domain.MainBranchID.UUID(), scope.BasePosition,
		).Scan(&currentVersion)
		if err != nil {
			return fmt.Errorf("seed branch version: %w", err)
		}
	}

	// Check optimistic concurrency
	if expectedVersion >= 0 && currentVersion != expectedVersion {
		return repository.ErrConcurrencyConflict
	}

	// Ensure stream exists. Keyed on currentVersion alone, NOT on
	// expectedVersion == -1: a caller that passes 0 for a first append is making
	// an equivalent claim ("no events yet"), and gating the parent-row insert on
	// the sentinel let such a caller write an event referencing a missing stream
	// and fail the foreign key. ON CONFLICT DO NOTHING keeps this idempotent.
	if currentVersion == 0 {
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
		INSERT INTO events (stream_id, stream_type, branch_id, version, event_type, data, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
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
			scope.BranchID.UUID(),
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
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position
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
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position
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

// ReadBranch reads a single branch's own events from a position.
func (s *EventStore) ReadBranch(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position
		FROM events
		WHERE branch_id = $1 AND position > $2
		ORDER BY position ASC
		LIMIT $3
	`, branchID.UUID(), fromPosition, limit)
	if err != nil {
		return nil, fmt.Errorf("query branch events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ReadStreamsForBranch reads one branch's events for a set of streams.
//
// The whole set goes in one statement via = ANY(...), and ORDER BY / LIMIT are
// pushed into SQL so the cap bounds the database's work, not just the returned
// slice. Served by idx_events_stream_branch.
func (s *EventStore) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	if len(streamIDs) == 0 || limit <= 0 {
		return nil, nil
	}

	ids := make([]string, len(streamIDs))
	for i, id := range streamIDs {
		ids[i] = id.String()
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position
		FROM events
		WHERE stream_id = ANY($1::uuid[]) AND branch_id = $2 AND position > $3
		ORDER BY position ASC
		LIMIT $4
	`, pq.Array(ids), branchID.UUID(), fromPosition, limit)
	if err != nil {
		return nil, fmt.Errorf("query streams for branch: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// GetStreamVersion returns the current version of a stream on a branch.
func (s *EventStore) GetStreamVersion(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID) (int64, error) {
	var version int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = $1 AND branch_id = $2",
		streamID, branchID.UUID(),
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
			id, streamID, branchID uuid.UUID
			streamType, eventType  string
			version, position      int64
			data                   []byte
			metadata               sql.NullString
			timestamp              time.Time
		)
		err := rows.Scan(&id, &streamID, &streamType, &branchID, &version, &eventType, &data, &metadata, &timestamp, &position)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   streamID,
			StreamType: streamType,
			BranchID:   domain.BranchID(branchID),
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

// ReadByStream returns paginated events for a specific stream (entity) on one branch.
func (s *EventStore) ReadByStream(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID, limit, offset int) (*repository.HistoryPage, error) {
	// The branch predicate sits inside the same statement as COUNT(*) OVER() so
	// the total counts only this branch's events (ADR-005).
	query := `
		SELECT
			id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position,
			COUNT(*) OVER() as total_count
		FROM events
		WHERE stream_id = $1 AND branch_id = $2
		ORDER BY version ASC
		LIMIT $3 OFFSET $4
	`

	rows, err := s.db.QueryContext(ctx, query, streamID, branchID.UUID(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query events by stream: %w", err)
	}
	defer rows.Close()

	return scanHistoryPage(rows, limit, offset)
}

// ReadGlobalByTime returns paginated events filtered by time range and optional event types.
func (s *EventStore) ReadGlobalByTime(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error) {
	var whereClauses []string
	var args []interface{}
	paramN := 1

	if !fromTime.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("timestamp >= $%d", paramN))
		args = append(args, fromTime)
		paramN++
	}
	if !toTime.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("timestamp <= $%d", paramN))
		args = append(args, toTime)
		paramN++
	}
	if len(eventTypes) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("event_type = ANY($%d)", paramN))
		args = append(args, pq.Array(eventTypes))
		paramN++
	}

	query := `
		SELECT
			id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position,
			COUNT(*) OVER() as total_count
		FROM events`
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	query += fmt.Sprintf(` ORDER BY timestamp ASC, position ASC LIMIT $%d OFFSET $%d`, paramN, paramN+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events by time: %w", err)
	}
	defer rows.Close()

	return scanHistoryPage(rows, limit, offset)
}

// scanHistoryPage scans rows into a HistoryPage with pagination info.
func scanHistoryPage(rows *sql.Rows, limit, offset int) (*repository.HistoryPage, error) {
	var events []repository.StoredEvent
	var totalCount int

	for rows.Next() {
		var (
			id, streamID, branchID uuid.UUID
			streamType, eventType  string
			version, position      int64
			data                   []byte
			metadata               sql.NullString
			timestamp              time.Time
		)
		err := rows.Scan(&id, &streamID, &streamType, &branchID, &version, &eventType, &data, &metadata, &timestamp, &position, &totalCount)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   streamID,
			StreamType: streamType,
			BranchID:   domain.BranchID(branchID),
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

	// Return empty page if no results
	if events == nil {
		events = []repository.StoredEvent{}
		totalCount = 0
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
