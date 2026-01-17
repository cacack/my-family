package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

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

// createTables creates the snapshots table if it doesn't exist.
func (s *SnapshotStore) createTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS snapshots (
			id UUID PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500),
			position BIGINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_snapshots_created_at ON snapshots(created_at DESC);
	`)
	return err
}

// Create stores a new snapshot.
func (s *SnapshotStore) Create(ctx context.Context, snapshot *domain.Snapshot) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO snapshots (id, name, description, position, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`,
		snapshot.ID,
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

// Get retrieves a snapshot by ID.
func (s *SnapshotStore) Get(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error) {
	var (
		snapshotID  uuid.UUID
		name        string
		description sql.NullString
		position    int64
		createdAt   time.Time
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, position, created_at
		FROM snapshots
		WHERE id = $1
	`, id).Scan(&snapshotID, &name, &description, &position, &createdAt)

	if err == sql.ErrNoRows {
		return nil, repository.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query snapshot: %w", err)
	}

	snapshot := &domain.Snapshot{
		ID:        snapshotID,
		Name:      name,
		Position:  position,
		CreatedAt: createdAt,
	}

	if description.Valid {
		snapshot.Description = description.String
	}

	return snapshot, nil
}

// List retrieves all snapshots ordered by created_at DESC.
func (s *SnapshotStore) List(ctx context.Context) ([]*domain.Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, position, created_at
		FROM snapshots
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*domain.Snapshot
	for rows.Next() {
		var (
			id          uuid.UUID
			name        string
			description sql.NullString
			position    int64
			createdAt   time.Time
		)

		if err := rows.Scan(&id, &name, &description, &position, &createdAt); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}

		snapshot := &domain.Snapshot{
			ID:        id,
			Name:      name,
			Position:  position,
			CreatedAt: createdAt,
		}

		if description.Valid {
			snapshot.Description = description.String
		}

		snapshots = append(snapshots, snapshot)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshots: %w", err)
	}

	// Return empty slice instead of nil
	if snapshots == nil {
		snapshots = []*domain.Snapshot{}
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
