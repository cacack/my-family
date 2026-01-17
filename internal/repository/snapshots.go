// Package repository provides data access interfaces and implementations.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// Common errors for snapshot store operations.
var (
	ErrSnapshotNotFound = errors.New("snapshot not found")
)

// SnapshotStore provides storage for research milestone snapshots.
type SnapshotStore interface {
	// Create stores a new snapshot.
	Create(ctx context.Context, snapshot *domain.Snapshot) error

	// Get retrieves a snapshot by ID.
	Get(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error)

	// List retrieves all snapshots ordered by created_at DESC.
	List(ctx context.Context) ([]*domain.Snapshot, error)

	// Delete removes a snapshot by ID.
	Delete(ctx context.Context, id uuid.UUID) error

	// GetMaxPosition returns the current maximum position from the event store.
	// This is used when creating a snapshot to capture the current point in time.
	GetMaxPosition(ctx context.Context) (int64, error)
}
