package postgres

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

// SnapshotStore is a PostgreSQL implementation of repository.SnapshotStore.
type SnapshotStore struct {
	db *sql.DB
}

// NewSnapshotStore creates a new PostgreSQL snapshot store.
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
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS snapshots (
			id UUID PRIMARY KEY,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500),
			position BIGINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

		CREATE INDEX IF NOT EXISTS idx_snapshots_created_at ON snapshots(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_snapshots_branch_created_at ON snapshots(branch_id, created_at DESC);
	`)
	return err
}

// Create stores a new snapshot.
func (s *SnapshotStore) Create(ctx context.Context, snapshot *domain.Snapshot) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO snapshots (id, branch_id, name, description, position, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`,
		snapshot.ID,
		snapshot.BranchID.UUID(),
		snapshot.Name,
		nullableString(snapshot.Description),
		snapshot.Position,
		snapshot.CreatedAt,
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
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			branch_id = EXCLUDED.branch_id,
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			position = EXCLUDED.position,
			created_at = EXCLUDED.created_at
	`,
		snapshot.ID,
		snapshot.BranchID.UUID(),
		snapshot.Name,
		nullableString(snapshot.Description),
		snapshot.Position,
		snapshot.CreatedAt,
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
		id          uuid.UUID
		branchID    uuid.UUID
		name        string
		description sql.NullString
		position    int64
		createdAt   time.Time
	)
	if err := row.Scan(&id, &branchID, &name, &description, &position, &createdAt); err != nil {
		return nil, err
	}
	snapshot := &domain.Snapshot{
		ID:        id,
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
		`SELECT id, branch_id, name, description, position, created_at FROM snapshots WHERE id = $1`, id))
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
		 FROM snapshots WHERE branch_id = $1 ORDER BY created_at DESC`,
		branchID.UUID())
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
		DELETE FROM snapshots WHERE id = $1
	`, id)
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
