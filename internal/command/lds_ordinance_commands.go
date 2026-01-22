package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// LDS Ordinance-related errors.
var (
	ErrLDSOrdinanceNotFound = errors.New("LDS ordinance not found")
)

// CreateLDSOrdinanceInput contains the data for creating a new LDS ordinance.
type CreateLDSOrdinanceInput struct {
	Type     domain.LDSOrdinanceType
	PersonID *uuid.UUID // For individual ordinances
	FamilyID *uuid.UUID // For SLGS (sealing to spouse)
	Date     string     // Date string to be parsed
	Place    string
	Temple   string
	Status   string
}

// CreateLDSOrdinanceResult contains the result of creating an LDS ordinance.
type CreateLDSOrdinanceResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateLDSOrdinance creates a new LDS ordinance record.
func (h *Handler) CreateLDSOrdinance(ctx context.Context, input CreateLDSOrdinanceInput) (*CreateLDSOrdinanceResult, error) {
	// Create ordinance entity
	ordinance := domain.NewLDSOrdinance(input.Type)

	if input.PersonID != nil {
		ordinance.SetPersonID(*input.PersonID)
	}
	if input.FamilyID != nil {
		ordinance.SetFamilyID(*input.FamilyID)
	}
	if input.Date != "" {
		ordinance.SetDate(input.Date)
	}
	if input.Place != "" {
		ordinance.SetPlace(input.Place)
	}
	if input.Temple != "" {
		ordinance.SetTemple(input.Temple)
	}
	if input.Status != "" {
		ordinance.SetStatus(input.Status)
	}

	// Validate ordinance
	if err := ordinance.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewLDSOrdinanceCreated(ordinance)

	// Execute command (append + project)
	version, err := h.execute(ctx, ordinance.ID.String(), "LDSOrdinance", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &CreateLDSOrdinanceResult{
		ID:      ordinance.ID,
		Version: version,
	}, nil
}

// UpdateLDSOrdinanceInput contains the data for updating an LDS ordinance.
type UpdateLDSOrdinanceInput struct {
	ID      uuid.UUID
	Date    *string // Pointer for optional update
	Place   *string
	Temple  *string
	Status  *string
	Version int64 // Required for optimistic locking
}

// UpdateLDSOrdinanceResult contains the result of updating an LDS ordinance.
type UpdateLDSOrdinanceResult struct {
	Version int64
}

// UpdateLDSOrdinance updates an existing LDS ordinance record.
func (h *Handler) UpdateLDSOrdinance(ctx context.Context, input UpdateLDSOrdinanceInput) (*UpdateLDSOrdinanceResult, error) {
	// Get current ordinance from read model
	current, err := h.readStore.GetLDSOrdinance(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrLDSOrdinanceNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	if input.Date != nil && *input.Date != current.DateRaw {
		changes["date"] = *input.Date
	}
	if input.Place != nil && *input.Place != current.Place {
		changes["place"] = *input.Place
	}
	if input.Temple != nil && *input.Temple != current.Temple {
		changes["temple"] = *input.Temple
	}
	if input.Status != nil && *input.Status != current.Status {
		changes["status"] = *input.Status
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdateLDSOrdinanceResult{Version: current.Version}, nil
	}

	// Create event
	event := domain.NewLDSOrdinanceUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "LDSOrdinance", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateLDSOrdinanceResult{Version: version}, nil
}

// DeleteLDSOrdinance deletes an LDS ordinance record.
func (h *Handler) DeleteLDSOrdinance(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current ordinance from read model
	current, err := h.readStore.GetLDSOrdinance(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrLDSOrdinanceNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Create event
	event := domain.NewLDSOrdinanceDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "LDSOrdinance", []domain.Event{event}, version)
	return err
}
