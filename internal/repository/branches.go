package repository

import (
	"context"
	"errors"
	"time"

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
	// missing. Used by the deleted projection's archive transition.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.BranchStatus) error

	// MarkMerged records the merge: it sets the status to merged and writes the
	// merge timestamp and note in one atomic write, returning ErrBranchNotFound
	// when missing.
	//
	// This is a distinct method rather than a wider UpdateStatus because the two
	// terminal transitions differ in kind: archiving carries no metadata, while a
	// merged branch must never be recorded without its timestamp (issue #55,
	// "merge history preserved"). Folding the metadata into UpdateStatus would
	// make it optional at every call site and let a merge land with a nil
	// MergedAt; a separate method makes the record impossible to omit.
	MarkMerged(ctx context.Context, id uuid.UUID, mergedAt time.Time, note string) error
}
