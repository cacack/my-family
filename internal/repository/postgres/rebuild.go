package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// mainOnlyTables are the read-model tables without a branch_id column, plus the
// projection-failure markers (#845). repository_external_ids precedes
// repositories, which it references.
var mainOnlyTables = []string{
	"submitters", "repository_external_ids", "repositories", "lds_ordinances",
	"projection_failures",
}

// readModelTables is every table ResetReadModel drops, in drop order.
func readModelTables() []string {
	return append(append([]string{}, branchScopedTables...), mainOnlyTables...)
}

// ResetReadModel drops every read-model table in one transaction, then
// re-creates them at this binary's schema (#680), so a rebuild also applies
// schema changes that ALTER TABLE ADD COLUMN cannot, such as a new primary key.
// The event log, branch registry and snapshot registry live in other tables and
// are untouched, and so are rebuild_state, which must outlive the reset, and
// the schema version, which the rebuild stamps afterwards.
//
// The tables are re-created after the drop commits. If that fails, the next
// start re-creates them, and the saved rebuild state makes `serve` warn until
// a rebuild finishes.
func (s *ReadModelStore) ResetReadModel(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range readModelTables() {
		// #nosec G202 -- table is from the fixed readModelTables list, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			return fmt.Errorf("drop %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit drop: %w", err)
	}
	if err := s.createTables(); err != nil {
		return fmt.Errorf("re-create read-model tables: %w", err)
	}
	return nil
}

// GetSchemaVersion returns the stamped read-model schema version, or 0 (#680).
func (s *ReadModelStore) GetSchemaVersion(ctx context.Context) (int, error) {
	var version int
	err := s.db.QueryRowContext(ctx, `SELECT version FROM read_model_schema WHERE id = 1`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get read model schema version: %w", err)
	}
	return version, nil
}

// SetSchemaVersion stamps the read-model schema version (#680).
func (s *ReadModelStore) SetSchemaVersion(ctx context.Context, version int) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO read_model_schema (id, version) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET version = excluded.version`, version); err != nil {
		return fmt.Errorf("set read model schema version: %w", err)
	}
	return nil
}

// RecordProjectionFailure stores a projection-failure marker (#845).
func (s *ReadModelStore) RecordProjectionFailure(ctx context.Context, f repository.ProjectionFailure) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projection_failures (stream_id, branch_id, version, event_type, error, failed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, f.StreamID, f.BranchID.String(), f.Version, f.EventType, f.Error, f.FailedAt)
	if err != nil {
		return fmt.Errorf("record projection failure: %w", err)
	}
	return nil
}

// ListProjectionFailures returns up to limit markers, oldest first (#845).
func (s *ReadModelStore) ListProjectionFailures(ctx context.Context, limit int) ([]repository.ProjectionFailure, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT stream_id, branch_id, version, event_type, error, failed_at
		FROM projection_failures ORDER BY seq LIMIT $1
	`, max(limit, 0))
	if err != nil {
		return nil, fmt.Errorf("list projection failures: %w", err)
	}
	defer rows.Close()
	var out []repository.ProjectionFailure
	for rows.Next() {
		var (
			f      repository.ProjectionFailure
			branch uuid.UUID
		)
		if err := rows.Scan(&f.StreamID, &branch, &f.Version, &f.EventType, &f.Error, &f.FailedAt); err != nil {
			return nil, fmt.Errorf("scan projection failure: %w", err)
		}
		f.BranchID = domain.BranchID(branch)
		out = append(out, f)
	}
	return out, rows.Err()
}

// SaveRebuildState stores the state of a rebuild in progress (#845).
func (s *ReadModelStore) SaveRebuildState(ctx context.Context, state []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rebuild_state (id, state) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET state = excluded.state
	`, string(state))
	if err != nil {
		return fmt.Errorf("save rebuild state: %w", err)
	}
	return nil
}

// GetRebuildState returns the saved rebuild state, or nil (#845).
func (s *ReadModelStore) GetRebuildState(ctx context.Context) ([]byte, error) {
	var state string
	err := s.db.QueryRowContext(ctx, `SELECT state FROM rebuild_state WHERE id = 1`).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rebuild state: %w", err)
	}
	return []byte(state), nil
}

// ClearRebuildState deletes the saved rebuild state (#845).
func (s *ReadModelStore) ClearRebuildState(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM rebuild_state`); err != nil {
		return fmt.Errorf("clear rebuild state: %w", err)
	}
	return nil
}
