package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Branch lifecycle command errors.
var (
	// ErrBranchStoreRequired is returned when a branch command runs on a handler
	// built without a branch registry store (NewHandler / NewHandlerWithBranchStore
	// with a nil store). The event would append but the registry row would never
	// appear, so fail loudly instead.
	ErrBranchStoreRequired = errors.New("branch registry store is not configured")

	// ErrPositionSourceRequired is returned when CreateBranch runs on a handler
	// built without a MaxPositionReader, so a base position cannot be determined.
	ErrPositionSourceRequired = errors.New("event position source is not configured")

	// ErrBranchNotActive is returned when a branch command targets a branch that
	// is merged or archived. Both are terminal states (ADR-005).
	ErrBranchNotActive = errors.New("branch is not active")
)

// branchStreamType is the event-store stream type for branch lifecycle events.
// A branch's own id is its stream id.
const branchStreamType = "branch"

// MaxPositionReader reports the event log's current head position. It is the
// only thing CreateBranch needs from the snapshot store, kept narrow so the
// command package does not depend on the whole repository.SnapshotStore surface.
// repository.SnapshotStore satisfies it.
type MaxPositionReader interface {
	GetMaxPosition(ctx context.Context) (int64, error)
}

// CreateBranch establishes a new research branch off main at the event log's
// current head (ADR-005). The branch registry row is written by the projection
// of the BranchCreated event, never by a direct BranchStore call, so rebuilding
// the projection reconstructs the registry.
func (h *Handler) CreateBranch(ctx context.Context, name, description string) (*domain.Branch, error) {
	if h.branchStore == nil {
		return nil, ErrBranchStoreRequired
	}
	if h.positions == nil {
		return nil, ErrPositionSourceRequired
	}

	basePosition, err := h.positions.GetMaxPosition(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting max event position: %w", err)
	}

	branch, err := domain.NewBranch(name, description, basePosition)
	if err != nil {
		return nil, err
	}

	event := domain.NewBranchCreated(branch)
	scope := branchScope(branch)

	// The branch's own scope, expectedVersion -1: this is the first event on the
	// branch's stream.
	if err := h.eventStore.Append(ctx, branch.ID, branchStreamType, []domain.Event{event}, -1, scope); err != nil {
		return nil, fmt.Errorf("appending branch created event: %w", err)
	}

	if err := h.projector.Project(ctx, event, 1, scope.BranchID); err != nil {
		return nil, fmt.Errorf("projecting branch created event: %w", err)
	}

	return branch, nil
}

// DeleteBranch archives a branch and drops its overlay rows. Despite the name
// the branch record is retained in the terminal "archived" status — the event
// log is append-only (ES-002). The status flip and the purge are the
// projection's work; this command only emits the event.
func (h *Handler) DeleteBranch(ctx context.Context, branchID uuid.UUID) error {
	if h.branchStore == nil {
		return ErrBranchStoreRequired
	}

	branch, err := h.branchStore.Get(ctx, branchID)
	if err != nil {
		return err // includes repository.ErrBranchNotFound
	}
	if branch.Status != domain.BranchStatusActive {
		return fmt.Errorf("%w: %s", ErrBranchNotActive, branch.Status)
	}

	scope := branchScope(branch)

	// The branch's stream already holds BranchCreated, so append at its current
	// version. GetStreamVersion reports 0 — not an error — when the branch has no
	// events for the stream, so a registry row with no events (only reachable by
	// writing the store directly) still deletes cleanly.
	currentVersion, err := h.eventStore.GetStreamVersion(ctx, branch.ID, scope.BranchID)
	if err != nil {
		return fmt.Errorf("getting branch stream version: %w", err)
	}
	// A registry row with no events reports version 0; that stream does not exist
	// yet, so the append must claim "new stream" with -1 rather than 0.
	expectedVersion := currentVersion
	if currentVersion == 0 {
		expectedVersion = -1
	}

	event := domain.NewBranchDeleted(branch.ID)
	if err := h.eventStore.Append(ctx, branch.ID, branchStreamType, []domain.Event{event}, expectedVersion, scope); err != nil {
		return fmt.Errorf("appending branch deleted event: %w", err)
	}

	if err := h.projector.Project(ctx, event, currentVersion+1, scope.BranchID); err != nil {
		return fmt.Errorf("projecting branch deleted event: %w", err)
	}

	return nil
}

// branchScope is the append scope of a branch's own lifecycle events: the
// branch's identity as a scope, anchored at the branch's base position.
func branchScope(b *domain.Branch) repository.AppendScope {
	return repository.AppendScope{
		BranchID:     domain.BranchID(b.ID),
		BasePosition: b.BasePosition,
	}
}
