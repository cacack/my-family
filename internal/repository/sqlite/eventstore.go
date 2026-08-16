// Package sqlite provides SQLite implementations of repository interfaces.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ErrReadModelEventsTable is returned by NewEventStore when the database's `events`
// table is the read model's pre-#733 life-fact table (it carries owner_type) rather
// than the event log. Nothing has been mutated when it is returned: the event store
// must not rename or migrate a table it does not own.
//
// The remedy is to construct the read model store first. Since #733 that construction
// either completes the events -> life_events rename or fails loudly (see
// ErrConflictingEventsTables and ReadModelStore.renameLegacyEventsTable), so once
// NewReadModelStore has returned nil the name is free and reopening the event store
// succeeds. The advice below is therefore sound, not a hope.
var ErrReadModelEventsTable = errors.New(
	`the "events" table in this database belongs to the read model, not the event log (pre-#733 schema): ` +
		`open the read model store first so it renames its table to "life_events", then reopen the event store`)

// EventStore is a SQLite implementation of repository.EventStore.
type EventStore struct {
	db *sql.DB
	mu sync.Mutex // serialize writes for SQLite
}

// NewEventStore creates a new SQLite event store.
//
// It can fail at construction. On a pre-#733 database — one whose `events` table is
// still the read model's life-fact table — it returns ErrReadModelEventsTable without
// touching the schema. Construct the read model store FIRST on such a database
// (NewReadModelStore renames its table to `life_events`), then call this. On a fresh
// or already-migrated database the construction order does not matter.
func NewEventStore(db *sql.DB) (*EventStore, error) {
	store := &EventStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return store, nil
}

// createTables creates the event store schema if it doesn't exist.
// The events table carries a branch_id envelope column and versions events per
// (stream_id, branch_id) (ADR-005). Fresh databases get both from the CREATE
// below; existing databases are upgraded first — migrateBranchID adds the
// column, migrateCompositeVersionKey rebuilds the table for the composite
// UNIQUE. Both run BEFORE the schema batch because that batch indexes branch_id,
// which an un-migrated table doesn't have yet — and both run AFTER the #733
// ownership guard, which refuses a database whose `events` table is the read
// model's so neither migration can mutate it.
func (s *EventStore) createTables() error {
	mainBranch := domain.MainBranchID.String()

	// #733 guard: refuse a database whose `events` table is the read model's, BEFORE
	// either migration touches it. migrateBranchID would happily ALTER a branch_id
	// column onto the read model's table, and migrateCompositeVersionKey would read
	// its DDL, conclude the event log is legacy, and abort mid-rebuild with the
	// opaque "copy events: no such column: stream_id". This is NOT dead code just
	// because both stores now use distinct table names — pre-#733 databases in the
	// wild still carry the old name, and the read model only renames itself when it
	// is opened. Do not remove it.
	ownedByReadModel, err := eventsTableBelongsToReadModel(s.db)
	if err != nil {
		return err
	}
	if ownedByReadModel {
		return ErrReadModelEventsTable
	}

	s.migrateBranchID(mainBranch)
	if err := s.migrateCompositeVersionKey(mainBranch); err != nil {
		return err
	}

	// #nosec G201 -- mainBranch is a constant UUID literal, not user input.
	_, err = s.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS streams (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			metadata TEXT
		);

		%[1]s;

		%[2]s
	`, eventsTableDDL("IF NOT EXISTS events", mainBranch), eventsIndexDDL))
	return err
}

// eventsTableDDL returns the CREATE TABLE statement for the events table.
// target is the name clause — "IF NOT EXISTS events" for the fresh create,
// "events_new" for the legacy rebuild — so both paths share one definition and
// cannot drift.
func eventsTableDDL(target, mainBranch string) string {
	// #nosec G201 -- target is a package-internal literal and mainBranch is a
	// constant UUID literal; neither is user input.
	return fmt.Sprintf(`
		CREATE TABLE %[1]s (
			id TEXT PRIMARY KEY,
			stream_id TEXT NOT NULL,
			stream_type TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '%[2]s',
			version INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			data TEXT NOT NULL,
			metadata TEXT,
			timestamp TEXT NOT NULL,
			position INTEGER NOT NULL,
			FOREIGN KEY (stream_id) REFERENCES streams(id),
			UNIQUE(stream_id, branch_id, version)
		)`, target, mainBranch)
}

// eventsIndexDDL is the full index set for the events table. The rebuild
// recreates it verbatim because DROP TABLE takes the original indexes with it.
const eventsIndexDDL = `
	CREATE INDEX IF NOT EXISTS idx_events_stream_branch_version ON events(stream_id, branch_id, version);
	CREATE INDEX IF NOT EXISTS idx_events_position ON events(position);
	CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type, timestamp);
	CREATE INDEX IF NOT EXISTS idx_events_timestamp_position ON events(timestamp, position);
	CREATE INDEX IF NOT EXISTS idx_events_branch ON events(branch_id);
	CREATE INDEX IF NOT EXISTS idx_events_stream_branch ON events(stream_id, branch_id);
`

// migrateBranchID adds the branch_id column to a pre-existing events table.
// SQLite lacks ADD COLUMN IF NOT EXISTS, so the duplicate-column error on
// already-migrated databases is intentionally swallowed.
func (s *EventStore) migrateBranchID(mainBranch string) {
	// #nosec G201 -- mainBranch is a constant UUID literal, not user input.
	_, _ = s.db.Exec(fmt.Sprintf(
		"ALTER TABLE events ADD COLUMN branch_id TEXT NOT NULL DEFAULT '%s'", mainBranch))
}

// migrateCompositeVersionKey upgrades a legacy events table whose UNIQUE
// constraint is (stream_id, version) to (stream_id, branch_id, version), so
// optimistic versioning is per-branch (ADR-005). SQLite cannot alter a table
// constraint, so this runs the standard 12-step rebuild. Fresh databases (no
// events table) and already-composite ones are left untouched. This is a
// one-time in-place migration of the source of truth: it verifies the row count
// and preserves every position and id, and any failure rolls back and surfaces
// as an error from NewEventStore rather than leaving a half-migrated database.
func (s *EventStore) migrateCompositeVersionKey(mainBranch string) error {
	legacy, err := s.hasLegacyVersionKey()
	if err != nil {
		return err
	}
	if !legacy {
		return nil
	}

	slog.Info("rebuilding sqlite events table for per-branch versioning (ADR-005): UNIQUE(stream_id, version) -> UNIQUE(stream_id, branch_id, version)")

	// PRAGMA foreign_keys is a no-op inside a transaction, so it is toggled on the
	// connection first. OpenDB caps the pool at one connection, so this reaches the
	// same connection the rebuild transaction runs on.
	if _, err := s.db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys for events rebuild: %w", err)
	}
	defer func() { _, _ = s.db.Exec("PRAGMA foreign_keys = ON") }()

	if err := s.rebuildEventsTable(mainBranch); err != nil {
		return fmt.Errorf("rebuild events table for per-branch versioning: %w", err)
	}
	return nil
}

// hasLegacyVersionKey reports whether the stored events DDL predates the
// composite UNIQUE(stream_id, branch_id, version) constraint. sqlite_master
// holds the CREATE statement verbatim, so the match is made against a
// whitespace- and case-normalized copy. A missing events table (fresh database)
// is not legacy.
func (s *EventStore) hasLegacyVersionKey() (bool, error) {
	var ddl string
	err := s.db.QueryRow(
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'events'").Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect events schema: %w", err)
	}

	normalized := strings.ToLower(strings.Join(strings.Fields(ddl), ""))
	return !strings.Contains(normalized, "unique(stream_id,branch_id,version)"), nil
}

// rebuildEventsTable performs the copy/drop/rename rebuild in one transaction.
func (s *EventStore) rebuildEventsTable(mainBranch string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var before int64
	if err := tx.QueryRow("SELECT COUNT(*) FROM events").Scan(&before); err != nil {
		return fmt.Errorf("count events: %w", err)
	}

	if _, err := tx.Exec(eventsTableDDL("events_new", mainBranch)); err != nil {
		return fmt.Errorf("create events_new: %w", err)
	}

	// Explicit column lists: every position and id value is carried over as-is.
	if _, err := tx.Exec(`
		INSERT INTO events_new (id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position)
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position FROM events
	`); err != nil {
		return fmt.Errorf("copy events: %w", err)
	}

	var after int64
	if err := tx.QueryRow("SELECT COUNT(*) FROM events_new").Scan(&after); err != nil {
		return fmt.Errorf("count copied events: %w", err)
	}
	if after != before {
		return fmt.Errorf("copied %d of %d events", after, before)
	}

	if _, err := tx.Exec("DROP TABLE events"); err != nil {
		return fmt.Errorf("drop legacy events: %w", err)
	}
	if _, err := tx.Exec("ALTER TABLE events_new RENAME TO events"); err != nil {
		return fmt.Errorf("rename events_new: %w", err)
	}
	if _, err := tx.Exec(eventsIndexDDL); err != nil {
		return fmt.Errorf("recreate indexes: %w", err)
	}

	return tx.Commit()
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
		"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = ? AND branch_id = ?",
		streamID.String(), scope.BranchID.String(),
	).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	// A branch's first write to an existing aggregate continues main's version line
	// as of the branch's base position rather than restarting at 1 (ADR-005).
	if currentVersion == 0 && !scope.BranchID.IsMain() {
		err = tx.QueryRowContext(ctx,
			"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = ? AND branch_id = ? AND position <= ?",
			streamID.String(), domain.MainBranchID.String(), scope.BasePosition,
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
	// and fail the foreign key. INSERT OR IGNORE keeps this idempotent.
	if currentVersion == 0 {
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
		INSERT INTO events (id, stream_id, stream_type, branch_id, version, event_type, data, timestamp, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			scope.BranchID.String(),
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
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position
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
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position
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

// ReadBranch reads a single branch's own events from a position.
func (s *EventStore) ReadBranch(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position
		FROM events
		WHERE branch_id = ? AND position > ?
		ORDER BY position ASC
		LIMIT ?
	`, branchID.String(), fromPosition, limit)
	if err != nil {
		return nil, fmt.Errorf("query branch events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// sqliteMaxVariables is the conservative floor for SQLITE_MAX_VARIABLE_NUMBER.
// Modern SQLite defaults to 32766, but builds compiled against the historical
// 999 are still in the wild, so set-based queries chunk to stay under it.
const sqliteMaxVariables = 999

// ReadStreamsForBranch reads one branch's events for a set of streams.
//
// SQLite has no array binding, so the stream ids go into a generated IN clause
// of "?" placeholders — the ids themselves are always bound, never interpolated.
// A branch may touch more streams than SQLite will bind at once, so the ids are
// chunked; ORDER BY and LIMIT are pushed into every chunk query so the database
// enforces the cap, and the merged result is re-ordered and re-capped. Taking
// the oldest `limit` rows per chunk is sufficient: an event in the global oldest
// `limit` is also in its own chunk's oldest `limit`.
func (s *EventStore) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	if len(streamIDs) == 0 || limit <= 0 {
		return nil, nil
	}

	// branch_id, position and limit take three of the bind slots.
	chunkSize := sqliteMaxVariables - 3

	var events []repository.StoredEvent
	for start := 0; start < len(streamIDs); start += chunkSize {
		end := min(start+chunkSize, len(streamIDs))

		chunk, err := s.readStreamsChunk(ctx, streamIDs[start:end], branchID, fromPosition, limit)
		if err != nil {
			return nil, err
		}
		events = append(events, chunk...)
	}

	// One chunk is already ordered and capped by SQL; several are not.
	if len(streamIDs) > chunkSize {
		sort.Slice(events, func(i, j int) bool { return events[i].Position < events[j].Position })
		if len(events) > limit {
			events = events[:limit]
		}
	}

	return events, nil
}

// readStreamsChunk runs the set query for one bind-safe slice of stream ids.
func (s *EventStore) readStreamsChunk(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	args := make([]any, 0, len(streamIDs)+3)
	for _, id := range streamIDs {
		args = append(args, id.String())
	}
	args = append(args, branchID.String(), fromPosition, limit)

	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(streamIDs)), ", ")

	// #nosec G201 -- placeholders is a generated run of "?" bind markers; every
	// stream id is passed as a parameter in args, never interpolated.
	query := fmt.Sprintf(`
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position
		FROM events
		WHERE stream_id IN (%s) AND branch_id = ? AND position > ?
		ORDER BY position ASC
		LIMIT ?
	`, placeholders)

	rows, err := s.db.QueryContext(ctx, query, args...)
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
		"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = ? AND branch_id = ?",
		streamID.String(), branchID.String(),
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
			idStr, streamIDStr, streamType, branchIDStr, eventType, dataStr, timestampStr string
			version, position                                                             int64
			metadataStr                                                                   sql.NullString
		)
		err := rows.Scan(&idStr, &streamIDStr, &streamType, &branchIDStr, &version, &eventType, &dataStr, &metadataStr, &timestampStr, &position)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		id, _ := uuid.Parse(idStr)
		streamID, _ := uuid.Parse(streamIDStr)
		branchID, _ := uuid.Parse(branchIDStr)

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   streamID,
			StreamType: streamType,
			BranchID:   domain.BranchID(branchID),
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

// ReadByStream returns paginated events for a specific stream (entity) on one branch.
// Results are ordered by version ascending.
func (s *EventStore) ReadByStream(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID, limit, offset int) (*repository.HistoryPage, error) {
	// Query with window function for total count. The branch predicate sits inside
	// the same statement as COUNT(*) OVER() so the total counts only this branch's
	// events (ADR-005).
	query := `
		SELECT
			id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position,
			COUNT(*) OVER() as total_count
		FROM events
		WHERE stream_id = ? AND branch_id = ?
		ORDER BY version ASC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, query, streamID.String(), branchID.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query events by stream: %w", err)
	}
	defer rows.Close()

	var events []repository.StoredEvent
	var totalCount int

	for rows.Next() {
		var (
			idStr, streamIDStr, streamType, branchIDStr, eventType, dataStr, timestampStr string
			version, position                                                             int64
			metadataStr                                                                   sql.NullString
		)
		err := rows.Scan(&idStr, &streamIDStr, &streamType, &branchIDStr, &version, &eventType, &dataStr, &metadataStr, &timestampStr, &position, &totalCount)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		id, _ := uuid.Parse(idStr)
		sid, _ := uuid.Parse(streamIDStr)
		branchID, _ := uuid.Parse(branchIDStr)

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   sid,
			StreamType: streamType,
			BranchID:   domain.BranchID(branchID),
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
			id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position,
			COUNT(*) OVER() as total_count
		FROM events
		%s
		ORDER BY timestamp ASC, position ASC
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
			idStr, streamIDStr, streamType, branchIDStr, eventType, dataStr, timestampStr string
			version, position                                                             int64
			metadataStr                                                                   sql.NullString
		)
		err := rows.Scan(&idStr, &streamIDStr, &streamType, &branchIDStr, &version, &eventType, &dataStr, &metadataStr, &timestampStr, &position, &totalCount)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		id, _ := uuid.Parse(idStr)
		sid, _ := uuid.Parse(streamIDStr)
		branchID, _ := uuid.Parse(branchIDStr)

		event := repository.StoredEvent{
			ID:         id,
			StreamID:   sid,
			StreamType: streamType,
			BranchID:   domain.BranchID(branchID),
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
