package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Snapshot lifecycle command errors.
var (
	// ErrSnapshotStoreRequired is returned when a snapshot command runs on a
	// handler built without a snapshot registry store. The event would append but
	// the registry row would never appear, so fail loudly instead.
	ErrSnapshotStoreRequired = errors.New("snapshot registry store is not configured")

	// ErrSnapshotNotBranchScoped is returned when a snapshot command runs on a
	// branch-scoped handler. ADR-005 §"Interaction with snapshots and rollback"
	// defines a branch snapshot as a pointer to (branch_id, position), but the
	// registry has no branch_id column yet, so a branch-scoped snapshot would be
	// indistinguishable from a mainline one. Refuse rather than record it wrong.
	ErrSnapshotNotBranchScoped = errors.New("snapshots are not supported on a branch-scoped handler")
)

// snapshotStreamType is the event-store stream type for snapshot lifecycle
// events. A snapshot's own id is its stream id.
const snapshotStreamType = "snapshot"

// CreateSnapshot marks the event log's current head with a named snapshot
// (issue #624). The registry row is written by the projection of the
// SnapshotCreated event, never by a direct SnapshotStore call, so rebuilding the
// projection reconstructs the registry.
//
// The head is read BEFORE the event is appended, so the snapshot points at the
// log as it stood without its own creation event. That resolves the
// chicken-and-egg the issue raised: emitting the event moves the head, but not
// the position the snapshot marks.
func (h *Handler) CreateSnapshot(ctx context.Context, name, description string) (*domain.Snapshot, error) {
	if h.snapshots == nil {
		return nil, ErrSnapshotStoreRequired
	}
	if !h.branchID.IsMain() {
		return nil, ErrSnapshotNotBranchScoped
	}

	position, err := h.snapshots.GetMaxPosition(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting max event position: %w", err)
	}

	snapshot, err := domain.NewSnapshot(name, description, position)
	if err != nil {
		return nil, err
	}

	event := domain.NewSnapshotCreated(snapshot)

	// The snapshot's own stream, expectedVersion -1: this is its first event.
	if err := h.eventStore.Append(ctx, snapshot.ID, snapshotStreamType, []domain.Event{event}, -1, repository.MainScope); err != nil {
		return nil, fmt.Errorf("appending snapshot created event: %w", err)
	}

	if err := h.projector.Project(ctx, event, 1, domain.MainBranchID); err != nil {
		return nil, fmt.Errorf("projecting snapshot created event: %w", err)
	}

	// Return the projected record, not the locally built one: the projection
	// derives CreatedAt from the event's OccurredAt, and a caller comparing the
	// create response against a later GET must see the same timestamp.
	stored, err := h.snapshots.Get(ctx, snapshot.ID)
	if err != nil {
		return nil, fmt.Errorf("reading back created snapshot: %w", err)
	}
	return stored, nil
}

// DeleteSnapshot removes a snapshot marker. The events it pointed at are
// untouched — the log is append-only (ES-002) and a snapshot is only a named
// pointer into it. The registry row is dropped by the projection, not here.
func (h *Handler) DeleteSnapshot(ctx context.Context, snapshotID uuid.UUID) error {
	if h.snapshots == nil {
		return ErrSnapshotStoreRequired
	}
	if !h.branchID.IsMain() {
		return ErrSnapshotNotBranchScoped
	}

	// Existence check first, so deleting an unknown snapshot 404s instead of
	// appending a tombstone event for a snapshot that never existed.
	if _, err := h.snapshots.Get(ctx, snapshotID); err != nil {
		return err // includes repository.ErrSnapshotNotFound
	}

	// One read of the stream answers both questions the append needs: is there
	// already a tombstone, and at what version does this stream stand.
	stored, err := h.eventStore.ReadStream(ctx, snapshotID)
	if err != nil {
		return fmt.Errorf("reading snapshot stream: %w", err)
	}
	tombstone, currentVersion := scanSnapshotStream(stored)

	// A tombstone already on the log with the registry row still present is the
	// torn state a delete leaves when its append succeeded but its projection did
	// not — and what a rival concurrent delete leaves behind. Re-project that
	// tombstone rather than appending a second one: same outcome, and ES-002 keeps
	// the log free of a duplicate that says nothing new.
	if tombstone != nil {
		decoded, err := tombstone.DecodeEvent()
		if err != nil {
			return fmt.Errorf("decoding the existing snapshot tombstone: %w", err)
		}
		if err := h.projector.Project(ctx, decoded, tombstone.Version, domain.MainBranchID); err != nil {
			return fmt.Errorf("projecting the existing snapshot tombstone: %w", err)
		}
		return nil
	}

	// The snapshot's stream already holds SnapshotCreated, so append at its
	// current version. A registry row with no events reports version 0; that
	// stream does not exist yet, so the append must claim "new stream" with -1.
	// Unlike the branch equivalent, this is not merely defensive: every snapshot
	// created before #624 was written straight to the registry and has no event,
	// so this is the ordinary path for them.
	expectedVersion := currentVersion
	if currentVersion == 0 {
		expectedVersion = -1
	}

	event := domain.NewSnapshotDeleted(snapshotID)
	if err := h.eventStore.Append(ctx, snapshotID, snapshotStreamType, []domain.Event{event}, expectedVersion, repository.MainScope); err != nil {
		return fmt.Errorf("appending snapshot deleted event: %w", err)
	}

	if err := h.projector.Project(ctx, event, currentVersion+1, domain.MainBranchID); err != nil {
		return fmt.Errorf("projecting snapshot deleted event: %w", err)
	}

	return nil
}

// scanSnapshotStream reports the snapshot's tombstone, if the stream carries one,
// and the stream's current version. Only mainline events count: snapshots cannot
// be created on a branch, so a branch-tagged event on a snapshot stream is not
// something this command should reason about.
func scanSnapshotStream(stored []repository.StoredEvent) (tombstone *repository.StoredEvent, currentVersion int64) {
	for i := range stored {
		if stored[i].BranchID != domain.MainBranchID {
			continue
		}
		if stored[i].Version > currentVersion {
			currentVersion = stored[i].Version
		}
		if stored[i].EventType == "SnapshotDeleted" && tombstone == nil {
			tombstone = &stored[i]
		}
	}
	return tombstone, currentVersion
}
