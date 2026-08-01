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

// Compile-time assertion that BranchStore satisfies the interface.
var _ repository.BranchStore = (*BranchStore)(nil)

// BranchStore is a PostgreSQL implementation of repository.BranchStore.
type BranchStore struct {
	db *sql.DB
}

// NewBranchStore creates a new PostgreSQL branch store.
func NewBranchStore(db *sql.DB) (*BranchStore, error) {
	store := &BranchStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return store, nil
}

// createTables creates the branches table if it doesn't exist and brings an
// existing one up to the current shape.
func (s *BranchStore) createTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS branches (
			id UUID PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500),
			base_position BIGINT NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			merged_at TIMESTAMPTZ,
			merge_note TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_branches_created_at ON branches(created_at DESC);
	`)
	if err != nil {
		return err
	}
	return s.migrateMergeColumns()
}

// migrateMergeColumns adds the merge-record columns (issue #55) to a branches
// table created before they existed, so a pre-#55 database gains them on open.
// ADD COLUMN IF NOT EXISTS makes this idempotent across repeated opens.
func (s *BranchStore) migrateMergeColumns() error {
	_, err := s.db.Exec(`
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS merged_at TIMESTAMPTZ;
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS merge_note TEXT;
	`)
	return err
}

// Create stores a new branch.
func (s *BranchStore) Create(ctx context.Context, branch *domain.Branch) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO branches (id, name, description, base_position, status, created_at, merged_at, merge_note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		branch.ID,
		branch.Name,
		nullableString(branch.Description),
		branch.BasePosition,
		string(branch.Status),
		branch.CreatedAt,
		nullableTime(branch.MergedAt),
		nullableString(branch.MergeNote),
	)
	if err != nil {
		return fmt.Errorf("insert branch: %w", err)
	}
	return nil
}

// Upsert stores a branch, inserting or updating on ID conflict.
func (s *BranchStore) Upsert(ctx context.Context, branch *domain.Branch) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO branches (id, name, description, base_position, status, created_at, merged_at, merge_note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			base_position = EXCLUDED.base_position,
			status = EXCLUDED.status,
			created_at = EXCLUDED.created_at,
			merged_at = EXCLUDED.merged_at,
			merge_note = EXCLUDED.merge_note
	`,
		branch.ID,
		branch.Name,
		nullableString(branch.Description),
		branch.BasePosition,
		string(branch.Status),
		branch.CreatedAt,
		nullableTime(branch.MergedAt),
		nullableString(branch.MergeNote),
	)
	if err != nil {
		return fmt.Errorf("upsert branch: %w", err)
	}
	return nil
}

// Get retrieves a branch by ID.
func (s *BranchStore) Get(ctx context.Context, id uuid.UUID) (*domain.Branch, error) {
	var (
		branchID     uuid.UUID
		name         string
		description  sql.NullString
		basePosition int64
		status       string
		createdAt    time.Time
		mergedAt     sql.NullTime
		mergeNote    sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, base_position, status, created_at, merged_at, merge_note
		FROM branches
		WHERE id = $1
	`, id).Scan(&branchID, &name, &description, &basePosition, &status, &createdAt,
		&mergedAt, &mergeNote)

	if err == sql.ErrNoRows {
		return nil, repository.ErrBranchNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query branch: %w", err)
	}

	branch := &domain.Branch{
		ID:           branchID,
		Name:         name,
		BasePosition: basePosition,
		Status:       domain.BranchStatus(status),
		CreatedAt:    createdAt,
		MergeNote:    mergeNote.String,
	}

	if mergedAt.Valid {
		branch.MergedAt = &mergedAt.Time
	}
	if description.Valid {
		branch.Description = description.String
	}

	return branch, nil
}

// List retrieves all branches ordered by created_at DESC.
func (s *BranchStore) List(ctx context.Context) ([]*domain.Branch, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, base_position, status, created_at, merged_at, merge_note
		FROM branches
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query branches: %w", err)
	}
	defer rows.Close()

	var branches []*domain.Branch
	for rows.Next() {
		var (
			id           uuid.UUID
			name         string
			description  sql.NullString
			basePosition int64
			status       string
			createdAt    time.Time
			mergedAt     sql.NullTime
			mergeNote    sql.NullString
		)

		if err := rows.Scan(&id, &name, &description, &basePosition, &status, &createdAt,
			&mergedAt, &mergeNote); err != nil {
			return nil, fmt.Errorf("scan branch: %w", err)
		}

		branch := &domain.Branch{
			ID:           id,
			Name:         name,
			BasePosition: basePosition,
			Status:       domain.BranchStatus(status),
			CreatedAt:    createdAt,
			MergeNote:    mergeNote.String,
		}

		if mergedAt.Valid {
			branch.MergedAt = &mergedAt.Time
		}
		if description.Valid {
			branch.Description = description.String
		}

		branches = append(branches, branch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branches: %w", err)
	}

	// Return empty slice instead of nil
	if branches == nil {
		branches = []*domain.Branch{}
	}

	return branches, nil
}

// Delete removes a branch by ID.
func (s *BranchStore) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM branches WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete branch: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrBranchNotFound
	}

	return nil
}

// UpdateStatus changes a branch's status.
func (s *BranchStore) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.BranchStatus) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE branches SET status = $1 WHERE id = $2
	`, string(status), id)
	if err != nil {
		return fmt.Errorf("update branch status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrBranchNotFound
	}

	return nil
}

// MarkMerged records the merge: status, timestamp and note in one statement.
func (s *BranchStore) MarkMerged(ctx context.Context, id uuid.UUID, mergedAt time.Time, note string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE branches SET status = $1, merged_at = $2, merge_note = $3 WHERE id = $4
	`, string(domain.BranchStatusMerged), mergedAt, nullableString(note), id)
	if err != nil {
		return fmt.Errorf("mark branch merged: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrBranchNotFound
	}

	return nil
}
