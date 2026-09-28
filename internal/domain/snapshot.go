package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Snapshot validation errors.
var (
	ErrSnapshotNameRequired = errors.New("snapshot name is required")
	ErrSnapshotNameTooLong  = errors.New("snapshot name must be 100 characters or less")
	ErrSnapshotDescTooLong  = errors.New("snapshot description must be 500 characters or less")
)

// Snapshot represents a named point in the event store,
// allowing users to mark research milestones like "Pre-DNA results"
// or "After courthouse trip".
//
// A snapshot marks (BranchID, Position) (ADR-005, issue #839): Position is a
// global log position, and BranchID says whose view of the log it marks. A
// mainline snapshot carries MainBranchID. BranchID is excluded from the JSON
// form because domain.BranchID has no text marshalling of its own; API
// responses carry it explicitly.
type Snapshot struct {
	ID          uuid.UUID `json:"id"`
	BranchID    BranchID  `json:"-"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Position    int64     `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
}

// NewSnapshot creates a new mainline Snapshot with validation.
func NewSnapshot(name, description string, position int64) (*Snapshot, error) {
	return NewSnapshotOn(MainBranchID, name, description, position)
}

// NewSnapshotOn creates a new Snapshot marking position in branchID's view of
// the log, with validation.
func NewSnapshotOn(branchID BranchID, name, description string, position int64) (*Snapshot, error) {
	s := &Snapshot{
		ID:          uuid.New(),
		BranchID:    branchID,
		Name:        name,
		Description: description,
		Position:    position,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}

	return s, nil
}

// Validate checks that the snapshot has valid field values.
func (s *Snapshot) Validate() error {
	if s.Name == "" {
		return ErrSnapshotNameRequired
	}
	if len(s.Name) > 100 {
		return ErrSnapshotNameTooLong
	}
	if len(s.Description) > 500 {
		return ErrSnapshotDescTooLong
	}
	return nil
}
