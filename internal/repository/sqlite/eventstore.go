// Package sqlite provides SQLite implementations of repository interfaces.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

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

// NewEventStore creates a new SQLite event store, first bringing the store
// schema up to date (ADR-011): the event log's tables are migrated, not created
// here.
//
// It can fail at construction. On a pre-#733 database — one whose `events` table is
// still the read model's life-fact table — it returns ErrReadModelEventsTable without
// touching the schema. Construct the read model store FIRST on such a database
// (NewReadModelStore renames its table to `life_events`), then call this. On a fresh
// or already-migrated database the construction order does not matter.
func NewEventStore(db *sql.DB) (*EventStore, error) {
	if err := migrateStoreSchema(db); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return &EventStore{db: db}, nil
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

	// A branch's first write to an existing aggregate continues the version line
	// the branch's read shows rather than restarting at 1: main's CURRENT version,
	// or the branch's cross-stream shadow row version when the caller reports one
	// (repository.BranchSeedVersion; ADR-005, #844).
	if currentVersion == 0 && !scope.BranchID.IsMain() {
		err = tx.QueryRowContext(ctx,
			"SELECT COALESCE(MAX(version), 0) FROM events WHERE stream_id = ? AND branch_id = ?",
			streamID.String(), domain.MainBranchID.String(),
		).Scan(&currentVersion)
		if err != nil {
			return fmt.Errorf("seed branch version: %w", err)
		}
		currentVersion, err = repository.BranchSeedVersion(currentVersion, scope)
		if err != nil {
			return err
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
		INSERT INTO events (id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		maxPosition++
		currentVersion++

		// The payload is the event's own; a stamp's metadata and record time
		// (a merge's provenance, #832) go in their own columns.
		data, metadata, recordedAt, err := domain.EncodeForStore(event)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		var metadataArg any
		if metadata != nil {
			metadataArg = string(metadata)
		}

		_, err = stmt.ExecContext(ctx,
			uuid.New().String(),
			streamID.String(),
			streamType,
			scope.BranchID.String(),
			currentVersion,
			event.EventType(),
			string(data),
			metadataArg,
			recordedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
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

// countMainDriftSQL counts, per requested (branch, base position), main's
// events after the base and the subset on streams the branch wrote. The scopes
// and the excluded event types each arrive as one JSON bind parameter, so the
// statement text is constant and the whole set is answered at once, with no
// bind-variable ceiling to chunk around. Each count is a scalar subquery over a
// LIMITed derived table, so the cap bounds the scan per branch.
const countMainDriftSQL = `
	WITH scopes AS (
		SELECT json_extract(value, '$.b') AS branch_id, MIN(json_extract(value, '$.p')) AS base_position
		FROM json_each(?1)
		GROUP BY json_extract(value, '$.b')
	),
	excluded AS (
		SELECT value AS event_type FROM json_each(?2)
	)
	SELECT s.branch_id,
		(SELECT COUNT(*) FROM (
			SELECT 1 FROM events m
			WHERE m.branch_id = ?3 AND m.position > s.base_position
				AND m.event_type NOT IN (SELECT event_type FROM excluded)
			LIMIT ?4
		)) AS main_changes,
		(SELECT COUNT(*) FROM (
			SELECT 1 FROM events m
			WHERE m.branch_id = ?3 AND m.position > s.base_position
				AND m.event_type NOT IN (SELECT event_type FROM excluded)
				AND m.stream_id IN (
					SELECT b.stream_id FROM events b
					WHERE b.branch_id = s.branch_id
						AND b.event_type NOT IN (SELECT event_type FROM excluded)
				)
			LIMIT ?4
		)) AS main_changes_on_branch_streams
	FROM scopes s
`

// driftScopeJSON is one scope as countMainDriftSQL reads it from json_each.
type driftScopeJSON struct {
	BranchID     string `json:"b"`
	BasePosition int64  `json:"p"`
}

// CountMainDrift reports how far main has moved under each branch, in one
// statement for the whole set. See repository.EventStore.CountMainDrift.
func (s *EventStore) CountMainDrift(ctx context.Context, scopes []repository.DriftScope, excludeEventTypes []string, limit int) (map[domain.BranchID]repository.DriftCount, error) {
	result := make(map[domain.BranchID]repository.DriftCount, len(scopes))
	if len(scopes) == 0 || limit <= 0 {
		return result, nil
	}

	encodedScopes := make([]driftScopeJSON, len(scopes))
	for i, scope := range scopes {
		encodedScopes[i] = driftScopeJSON{BranchID: scope.BranchID.String(), BasePosition: scope.BasePosition}
	}
	scopesJSON, err := json.Marshal(encodedScopes)
	if err != nil {
		return nil, fmt.Errorf("encode drift scopes: %w", err)
	}
	excluded := excludeEventTypes
	if excluded == nil {
		excluded = []string{}
	}
	excludedJSON, err := json.Marshal(excluded)
	if err != nil {
		return nil, fmt.Errorf("encode excluded event types: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, countMainDriftSQL,
		string(scopesJSON), string(excludedJSON), domain.MainBranchID.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("count main drift: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			rawBranchID string
			count       repository.DriftCount
		)
		if err := rows.Scan(&rawBranchID, &count.MainChanges, &count.MainChangesOnBranchStreams); err != nil {
			return nil, fmt.Errorf("scan main drift: %w", err)
		}
		branchID, err := uuid.Parse(rawBranchID)
		if err != nil {
			return nil, fmt.Errorf("parse drift branch id: %w", err)
		}
		result[domain.BranchID(branchID)] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate main drift: %w", err)
	}
	return result, nil
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

// Fixed SQL fragments for ReadGlobalHistory. The type lists expand to one ?
// per element; every value is a bind parameter.
//
// Timestamps are compared and ordered through julianday(), not as text: the
// stored RFC 3339 form drops trailing fractional zeros, so "…:01Z" sorts after
// "…:01.5Z" as a string although it is earlier. The expression index
// idx_events_branch_julian_position matches these expressions exactly, so a
// page walks the index in order — no full scan and sort — and the count is
// answered from the index alone.
const (
	globalHistoryFromTime = "julianday(timestamp) >= julianday(?)"
	globalHistoryToTime   = "julianday(timestamp) <= julianday(?)"
	globalHistoryInclude  = "event_type IN (%s)"
	globalHistoryExclude  = "event_type NOT IN (%s)"
	globalHistoryBranch   = "branch_id = ?"
	// globalHistoryBranchOr keeps the branch's events and, from any branch,
	// the types in the list (GlobalHistoryQuery.AnyBranchEventTypes). The
	// unary + keeps the planner from OR-ing two index searches and sorting
	// the union: it walks idx_events_julian_position_branch in order instead,
	// testing both columns from the index.
	globalHistoryBranchOr = "(+branch_id = ? OR +event_type IN (%s))"
	globalHistoryCount    = "SELECT COUNT(*) FROM events"
	globalHistorySelect   = "SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position FROM events"
	globalHistoryOrder    = " ORDER BY julianday(timestamp) ASC, position ASC LIMIT ? OFFSET ?"
)

// globalHistorySQL builds ReadGlobalHistory's count and page queries from the
// constants above, with the filter's bind arguments (the page query takes
// limit and offset after them).
func globalHistorySQL(q repository.GlobalHistoryQuery) (countQuery, pageQuery string, args []any) {
	var where []string
	inList := func(fragment string, values []string) {
		placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(values)), ", ")
		where = append(where, fmt.Sprintf(fragment, placeholders))
		for _, v := range values {
			args = append(args, v)
		}
	}
	if !q.FromTime.IsZero() {
		where = append(where, globalHistoryFromTime)
		args = append(args, formatTimestamp(q.FromTime))
	}
	if !q.ToTime.IsZero() {
		where = append(where, globalHistoryToTime)
		args = append(args, formatTimestamp(q.ToTime))
	}
	if len(q.IncludeEventTypes) > 0 {
		inList(globalHistoryInclude, q.IncludeEventTypes)
	}
	if len(q.ExcludeEventTypes) > 0 {
		inList(globalHistoryExclude, q.ExcludeEventTypes)
	}
	switch {
	case q.BranchID != nil && len(q.AnyBranchEventTypes) > 0:
		args = append(args, q.BranchID.String())
		inList(globalHistoryBranchOr, q.AnyBranchEventTypes)
	case q.BranchID != nil:
		where = append(where, globalHistoryBranch)
		args = append(args, q.BranchID.String())
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}
	return globalHistoryCount + whereClause, globalHistorySelect + whereClause + globalHistoryOrder, args
}

// ReadGlobalHistory returns one page of the global history with every filter
// applied before pagination (see repository.GlobalHistoryQuery). The total is
// counted by its own query rather than a window function, so a page past the
// end still reports the real total instead of zero.
func (s *EventStore) ReadGlobalHistory(ctx context.Context, q repository.GlobalHistoryQuery) (*repository.HistoryPage, error) {
	countQuery, pageQuery, args := globalHistorySQL(q)

	var total int
	// #nosec G202 -- countQuery is joined from the package constants above, carrying only ? placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count global history: %w", err)
	}

	limit := max(q.Limit, 0)
	offset := max(q.Offset, 0)
	pageArgs := append(append([]any{}, args...), limit, offset)
	// #nosec G202 -- pageQuery is joined from the package constants above; limit and offset are bind parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, pageQuery, pageArgs...)
	if err != nil {
		return nil, fmt.Errorf("query global history: %w", err)
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []repository.StoredEvent{}
	}
	return &repository.HistoryPage{
		Events:     events,
		TotalCount: total,
		HasMore:    offset+len(events) < total,
	}, nil
}

// Close closes the database connection.
func (s *EventStore) Close() error {
	return s.db.Close()
}
