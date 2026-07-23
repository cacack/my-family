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

// createTables creates the branches table if it doesn't exist.
func (s *BranchStore) createTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS branches (
			id UUID PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500),
			base_position BIGINT NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_branches_created_at ON branches(created_at DESC);
	`)
	return err
}

// Create stores a new branch.
func (s *BranchStore) Create(ctx context.Context, branch *domain.Branch) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO branches (id, name, description, base_position, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`,
		branch.ID,
		branch.Name,
		nullableString(branch.Description),
		branch.BasePosition,
		string(branch.Status),
		branch.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert branch: %w", err)
	}
	return nil
}

// Upsert stores a branch, inserting or updating on ID conflict.
func (s *BranchStore) Upsert(ctx context.Context, branch *domain.Branch) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO branches (id, name, description, base_position, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			base_position = EXCLUDED.base_position,
			status = EXCLUDED.status,
			created_at = EXCLUDED.created_at
	`,
		branch.ID,
		branch.Name,
		nullableString(branch.Description),
		branch.BasePosition,
		string(branch.Status),
		branch.CreatedAt,
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
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, base_position, status, created_at
		FROM branches
		WHERE id = $1
	`, id).Scan(&branchID, &name, &description, &basePosition, &status, &createdAt)

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
	}

	if description.Valid {
		branch.Description = description.String
	}

	return branch, nil
}

// List retrieves all branches ordered by created_at DESC.
func (s *BranchStore) List(ctx context.Context) ([]*domain.Branch, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, base_position, status, created_at
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
		)

		if err := rows.Scan(&id, &name, &description, &basePosition, &status, &createdAt); err != nil {
			return nil, fmt.Errorf("scan branch: %w", err)
		}

		branch := &domain.Branch{
			ID:           id,
			Name:         name,
			BasePosition: basePosition,
			Status:       domain.BranchStatus(status),
			CreatedAt:    createdAt,
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
