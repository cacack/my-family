package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// Common errors for branch store operations.
var (
	ErrBranchNotFound = errors.New("branch not found")
)

// BranchStore provides storage for the branch registry read model. It is the
// branch analog of SnapshotStore: the projection writes it from BranchCreated
// events and queries read it.
type BranchStore interface {
	// Create stores a new branch.
	Create(ctx context.Context, branch *domain.Branch) error

	// Upsert stores a branch, inserting it or updating an existing row with the
	// same ID. Used by the projection, which may replay events idempotently.
	Upsert(ctx context.Context, branch *domain.Branch) error

	// Get retrieves a branch by ID, returning ErrBranchNotFound when missing.
	Get(ctx context.Context, id uuid.UUID) (*domain.Branch, error)

	// List retrieves all branches ordered by created_at DESC.
	List(ctx context.Context) ([]*domain.Branch, error)

	// Delete removes a branch by ID, returning ErrBranchNotFound when missing.
	Delete(ctx context.Context, id uuid.UUID) error

	// UpdateStatus changes a branch's status, returning ErrBranchNotFound when
	// missing. Used by the merged/deleted projections.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.BranchStatus) error
}
