package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// BranchID is the branch SCOPE attached to reads, writes, and stored events
// (ADR-005). It is a distinct type from the branch ENTITY's identity
// (Branch.ID, a uuid.UUID) so that a scope can never be silently transposed
// with an entity id in a call — the compiler rejects the mismatch. Convert to
// the underlying uuid.UUID with UUID() at DB boundaries; wrap a branch entity
// id into a scope with BranchID(branch.ID).
type BranchID uuid.UUID

// UUID returns the underlying uuid.UUID for DB binds and comparisons.
func (b BranchID) UUID() uuid.UUID { return uuid.UUID(b) }

// String returns the canonical string form of the branch scope.
func (b BranchID) String() string { return uuid.UUID(b).String() }

// IsMain reports whether this scope is the reserved mainline.
func (b BranchID) IsMain() bool { return b == MainBranchID }

// MainBranchID is the reserved branch scope for the mainline of research.
// It is fixed as the zero UUID: a zero value means "main". Downstream code
// cites this constant rather than re-deciding the reserved value (ADR-005).
// uuid values can't be const, so this is a var — treat it as immutable.
var MainBranchID = BranchID(uuid.Nil)

// Branch validation errors.
var (
	ErrBranchNameRequired  = errors.New("branch name is required")
	ErrBranchNameTooLong   = errors.New("branch name must be 100 characters or less")
	ErrBranchDescTooLong   = errors.New("branch description must be 500 characters or less")
	ErrBranchInvalidStatus = errors.New("branch status is invalid")
)

// BranchStatus represents the lifecycle state of a branch.
type BranchStatus string

const (
	BranchStatusActive BranchStatus = "active"
	BranchStatusMerged BranchStatus = "merged"
	// BranchStatusArchived is the terminal state a branch enters on delete/discard.
	// Note the deliberate vocabulary split: the lifecycle *event* is named
	// BranchDeleted (a "delete branch" action) but the resulting *status* is
	// "archived" — the branch record and its history are retained (append-only,
	// ES-002), only its overlay rows are purged. A UI "Delete branch" maps here.
	BranchStatusArchived BranchStatus = "archived"
)

// IsValid checks if the branch status value is valid.
func (s BranchStatus) IsValid() bool {
	switch s {
	case BranchStatusActive, BranchStatusMerged, BranchStatusArchived:
		return true
	default:
		return false
	}
}

// Branch is a lightweight record marking an isolated line of research off a
// point on main. BasePosition is a main global Position — the same
// base-pointer concept as a Snapshot (ADR-005 §The model).
//
// Legal status transitions: active→merged (on a successful merge) and
// active→archived (on discard/delete). merged and archived are terminal —
// a branch in either state accepts no further writes.
type Branch struct {
	ID           uuid.UUID    `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	BasePosition int64        `json:"base_position"`
	Status       BranchStatus `json:"status"`
	CreatedAt    time.Time    `json:"created_at"`
}

// NewBranch creates a new active Branch with validation.
func NewBranch(name, description string, basePosition int64) (*Branch, error) {
	b := &Branch{
		ID:           uuid.New(),
		Name:         name,
		Description:  description,
		BasePosition: basePosition,
		Status:       BranchStatusActive,
		CreatedAt:    time.Now().UTC(),
	}

	if err := b.Validate(); err != nil {
		return nil, err
	}

	return b, nil
}

// Validate checks that the branch has valid field values.
func (b *Branch) Validate() error {
	if b.Name == "" {
		return ErrBranchNameRequired
	}
	if len(b.Name) > 100 {
		return ErrBranchNameTooLong
	}
	if len(b.Description) > 500 {
		return ErrBranchDescTooLong
	}
	if !b.Status.IsValid() {
		return ErrBranchInvalidStatus
	}
	return nil
}
