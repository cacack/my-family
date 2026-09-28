package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Compile-time assertion that SnapshotStore satisfies the interface.
var _ repository.SnapshotStore = (*SnapshotStore)(nil)

// SnapshotStore is a SQLite implementation of repository.SnapshotStore.
type SnapshotStore struct {
	db *sql.DB
}

// NewSnapshotStore creates a new SQLite snapshot store.
func NewSnapshotStore(db *sql.DB) (*SnapshotStore, error) {
	store := &SnapshotStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return store, nil
}

// createTables creates the snapshots table if it doesn't exist, and migrates a
// table that predates branch-scoped snapshots (issue #839): the branch_id
// column is added with the mainline id as its default, so every existing row
// becomes a mainline snapshot — which is what it always was.
func (s *SnapshotStore) createTables() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS snapshots (
			id TEXT PRIMARY KEY,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			name TEXT NOT NULL,
			description TEXT,
			position INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_snapshots_created_at ON snapshots(created_at DESC);
	`); err != nil {
		return err
	}
	if err := s.migrateBranchID(); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_snapshots_branch_created_at ON snapshots(branch_id, created_at DESC)`)
	return err
}

// migrateBranchID adds the branch_id column to a snapshots table created before
// #839. SQLite has no ADD COLUMN IF NOT EXISTS, so the column's presence is
// checked first rather than swallowing a duplicate-column error.
func (s *SnapshotStore) migrateBranchID() error {
	var present int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('snapshots') WHERE name = 'branch_id'`,
	).Scan(&present); err != nil {
		return fmt.Errorf("inspect snapshots table: %w", err)
	}
	if present > 0 {
		return nil
	}
	if _, err := s.db.Exec(
		`ALTER TABLE snapshots ADD COLUMN branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'`,
	); err != nil {
		return fmt.Errorf("add snapshots.branch_id: %w", err)
	}
	return nil
}

// Create stores a new snapshot.
func (s *SnapshotStore) Create(ctx context.Context, snapshot *domain.Snapshot) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO snapshots (id, branch_id, name, description, position, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		snapshot.ID.String(),
		snapshot.BranchID.String(),
		snapshot.Name,
		nullableString(snapshot.Description),
		snapshot.Position,
		formatTimestamp(snapshot.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	return nil
}

// Upsert stores a snapshot, inserting it or overwriting an existing row with the
// same ID. The projection uses this so replaying SnapshotCreated is idempotent.
func (s *SnapshotStore) Upsert(ctx context.Context, snapshot *domain.Snapshot) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO snapshots (id, branch_id, name, description, position, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			branch_id = excluded.branch_id,
			name = excluded.name,
			description = excluded.description,
			position = excluded.position,
			created_at = excluded.created_at
	`,
		snapshot.ID.String(),
		snapshot.BranchID.String(),
		snapshot.Name,
		nullableString(snapshot.Description),
		snapshot.Position,
		formatTimestamp(snapshot.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	return nil
}

// scanSnapshot reads one snapshot row selected as
// (id, branch_id, name, description, position, created_at).
func scanSnapshot(row rowScanner) (*domain.Snapshot, error) {
	var (
		idStr, branchStr, name, createdAtStr string
		description                          sql.NullString
		position                             int64
	)
	if err := row.Scan(&idStr, &branchStr, &name, &description, &position, &createdAtStr); err != nil {
		return nil, err
	}

	parsedID, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("parse snapshot id: %w", err)
	}
	branchID, err := uuid.Parse(branchStr)
	if err != nil {
		return nil, fmt.Errorf("parse snapshot branch id: %w", err)
	}

	createdAt, err := parseTimestamp(createdAtStr)
	if err != nil {
		createdAt = time.Now().UTC()
	}

	snapshot := &domain.Snapshot{
		ID:        parsedID,
		BranchID:  domain.BranchID(branchID),
		Name:      name,
		Position:  position,
		CreatedAt: createdAt,
	}
	if description.Valid {
		snapshot.Description = description.String
	}
	return snapshot, nil
}

// Get retrieves a snapshot by ID, whichever branch it belongs to.
func (s *SnapshotStore) Get(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error) {
	snapshot, err := scanSnapshot(s.db.QueryRowContext(ctx,
		`SELECT id, branch_id, name, description, position, created_at FROM snapshots WHERE id = ?`,
		id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query snapshot: %w", err)
	}
	return snapshot, nil
}

// List retrieves the snapshots marked on branchID, ordered by created_at DESC.
func (s *SnapshotStore) List(ctx context.Context, branchID domain.BranchID) ([]*domain.Snapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, branch_id, name, description, position, created_at
		 FROM snapshots WHERE branch_id = ? ORDER BY created_at DESC`,
		branchID.String())
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	snapshots := []*domain.Snapshot{}
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshots: %w", err)
	}
	return snapshots, nil
}

// Delete removes a snapshot by ID.
func (s *SnapshotStore) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM snapshots WHERE id = ?
	`, id.String())
	if err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrSnapshotNotFound
	}

	return nil
}

// GetMaxPosition returns the current maximum position from the event store.
func (s *SnapshotStore) GetMaxPosition(ctx context.Context) (int64, error) {
	var maxPosition int64
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), 0) FROM events").Scan(&maxPosition)
	if err != nil {
		return 0, fmt.Errorf("get max position: %w", err)
	}
	return maxPosition, nil
}
