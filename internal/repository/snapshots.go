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

// SnapshotStore provides storage for the research milestone snapshot registry.
//
// The registry is event-sourced (issue #624): the projector writes it from
// SnapshotCreated / SnapshotDeleted events, so rebuilding the projection
// reconstructs it. This mirrors BranchStore — snapshots and branch base points
// are the same primitive, a named pointer to a global Position (ADR-005
// §"Interaction with snapshots and rollback").
//
// CAVEAT — rows predating #624. Snapshots created before this store became
// projection-written were inserted directly and have NO event on their stream,
// so a rebuild would not reconstruct them. Nothing replays the log into a
// projector today (no rebuild command exists), so the gap is latent rather than
// live; issue #680 tracks rebuild tooling and must backfill these rows, or
// deliberately drop them, before it ships.
type SnapshotStore interface {
	// Create stores a new snapshot. It has no production caller since the
	// registry became projection-written — Upsert is that path — and is retained
	// for tests that seed a registry row directly, mirroring BranchStore.
	Create(ctx context.Context, snapshot *domain.Snapshot) error

	// Upsert stores a snapshot, inserting it or overwriting an existing row with
	// the same ID. Used by the projection, which may replay events idempotently.
	Upsert(ctx context.Context, snapshot *domain.Snapshot) error

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
