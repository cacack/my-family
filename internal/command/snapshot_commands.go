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
)

// snapshotStreamType is the event-store stream type for snapshot lifecycle
// events. A snapshot's own id is its stream id.
const snapshotStreamType = "snapshot"

// CreateSnapshot marks the event log's current head with a named snapshot
// (issue #624) on the handler's branch: a snapshot is the pair
// (branch_id, position) of ADR-005, so one taken on a branch marks the log head
// in that branch's view, and one on the mainline marks it in the mainline's
// (issue #839). The registry row is written by the projection of the
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

	position, err := h.snapshots.GetMaxPosition(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting max event position: %w", err)
	}

	snapshot, err := domain.NewSnapshotOn(h.branchID, name, description, position)
	if err != nil {
		return nil, err
	}

	event := domain.NewSnapshotCreated(snapshot)

	// The snapshot's own stream, expectedVersion -1: this is its first event.
	//
	// The branch travels in the event payload, and the envelope stays on the
	// mainline even for a branch snapshot. The registry is not an overlay
	// entity — a snapshot id is unique and has no shadow row — and a
	// branch-tagged envelope would put a lifecycle marker into the branch's own
	// event set, which merge replay, conflict detection and branch compare read.
	// This mirrors the branch lifecycle events, which are mainline-enveloped
	// records naming a branch in their payload.
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
//
// Only a snapshot on the handler's branch can be deleted: one marked on another
// branch (or on the mainline, from a branch) is not found in this scope, the
// same answer the scoped list and get give (issue #839).
func (h *Handler) DeleteSnapshot(ctx context.Context, snapshotID uuid.UUID) error {
	if h.snapshots == nil {
		return ErrSnapshotStoreRequired
	}

	// Existence check first, so deleting an unknown snapshot 404s instead of
	// appending a tombstone event for a snapshot that never existed.
	existing, err := h.snapshots.Get(ctx, snapshotID)
	if err != nil {
		return err // includes repository.ErrSnapshotNotFound
	}
	if existing.BranchID != h.branchID {
		return repository.ErrSnapshotNotFound
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
	// not — and what a rival delete that finished before this read leaves behind.
	// Re-project that tombstone rather than appending a second one: same outcome,
	// and ES-002 keeps the log free of a duplicate that says nothing new.
	if tombstone != nil {
		return h.reprojectSnapshotTombstone(ctx, tombstone)
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

	event := domain.NewSnapshotDeleted(snapshotID, existing.BranchID)
	if err := h.eventStore.Append(ctx, snapshotID, snapshotStreamType, []domain.Event{event}, expectedVersion, repository.MainScope); err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			// A rival delete read the same version and appended first. Its
			// tombstone is the delete this caller asked for, so converge on it
			// instead of reporting a failure for a snapshot that is now gone.
			return h.convergeOnRivalSnapshotDelete(ctx, snapshotID, err)
		}
		return fmt.Errorf("appending snapshot deleted event: %w", err)
	}

	if err := h.projector.Project(ctx, event, currentVersion+1, domain.MainBranchID); err != nil {
		return fmt.Errorf("projecting snapshot deleted event: %w", err)
	}

	return nil
}

// convergeOnRivalSnapshotDelete handles a concurrency conflict on the tombstone
// append. It re-reads the stream: when a tombstone is now present, a rival delete
// won the race, and re-projecting that tombstone makes this call succeed exactly
// as if it had arrived second. Any other conflict (the stream moved for a reason
// other than a delete) is returned unchanged.
func (h *Handler) convergeOnRivalSnapshotDelete(ctx context.Context, snapshotID uuid.UUID, appendErr error) error {
	stored, err := h.eventStore.ReadStream(ctx, snapshotID)
	if err != nil {
		return fmt.Errorf("re-reading snapshot stream after a concurrent append: %w", err)
	}
	tombstone, _ := scanSnapshotStream(stored)
	if tombstone == nil {
		return fmt.Errorf("appending snapshot deleted event: %w", appendErr)
	}
	return h.reprojectSnapshotTombstone(ctx, tombstone)
}

// reprojectSnapshotTombstone projects a SnapshotDeleted event already on the
// log. Projecting a tombstone for a registry row that is already gone is a
// no-op, so this is safe whether or not the rival's projection has run yet.
func (h *Handler) reprojectSnapshotTombstone(ctx context.Context, tombstone *repository.StoredEvent) error {
	decoded, err := tombstone.DecodeEvent()
	if err != nil {
		return fmt.Errorf("decoding the existing snapshot tombstone: %w", err)
	}
	if err := h.projector.Project(ctx, decoded, tombstone.Version, domain.MainBranchID); err != nil {
		return fmt.Errorf("projecting the existing snapshot tombstone: %w", err)
	}
	return nil
}

// scanSnapshotStream reports the snapshot's tombstone, if the stream carries one,
// and the stream's current version. Only mainline-enveloped events count: every
// snapshot lifecycle event is appended on the mainline envelope, a branch
// snapshot's included (its branch is in the payload — see CreateSnapshot), so a
// branch-tagged event on a snapshot stream is not something this command
// should reason about.
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
