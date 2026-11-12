package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Submitter-related errors.
var (
	ErrSubmitterNotFound = errors.New("submitter not found")
)

// CreateSubmitterInput contains the data for creating a new submitter.
type CreateSubmitterInput struct {
	Name       string
	Address    *domain.Address
	Phone      []string
	Email      []string
	Language   string
	MediaID    *uuid.UUID
	GedcomXref string
}

// CreateSubmitterResult contains the result of creating a submitter.
type CreateSubmitterResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateSubmitter creates a new submitter record.
func (h *Handler) CreateSubmitter(ctx context.Context, input CreateSubmitterInput) (*CreateSubmitterResult, error) {
	// Create submitter entity
	submitter := domain.NewSubmitter(input.Name)

	if input.Address != nil {
		submitter.SetAddress(input.Address)
	}
	for _, phone := range input.Phone {
		submitter.AddPhone(phone)
	}
	for _, email := range input.Email {
		submitter.AddEmail(email)
	}
	if input.Language != "" {
		submitter.SetLanguage(input.Language)
	}
	if input.MediaID != nil {
		submitter.SetMediaID(input.MediaID)
	}
	if input.GedcomXref != "" {
		if err := validateGedcomXref(input.GedcomXref); err != nil {
			return nil, err
		}
		submitter.SetGedcomXref(input.GedcomXref)
	}

	// Validate submitter
	if err := submitter.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewSubmitterCreated(submitter)

	// Execute command (append + project)
	version, err := h.execute(ctx, submitter.ID.String(), "Submitter", []domain.Event{event}, -1)
	if err != nil {
		return nil, fmt.Errorf("executing create submitter command: %w", err)
	}

	return &CreateSubmitterResult{
		ID:      submitter.ID,
		Version: version,
	}, nil
}

// UpdateSubmitterInput contains the data for updating a submitter.
type UpdateSubmitterInput struct {
	ID       uuid.UUID
	Name     *string // required: cannot be cleared
	Address  Patch[domain.Address]
	Phone    Patch[[]string]
	Email    Patch[[]string]
	Language Patch[string]
	MediaID  Patch[uuid.UUID]
	Version  int64 // Required for optimistic locking
}

// UpdateSubmitterResult contains the result of updating a submitter.
type UpdateSubmitterResult struct {
	Version int64
}

// UpdateSubmitter updates an existing submitter record.
func (h *Handler) UpdateSubmitter(ctx context.Context, input UpdateSubmitterInput) (*UpdateSubmitterResult, error) {
	// Get current submitter from read model
	current, err := h.readStore.GetSubmitter(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting submitter: %w", err)
	}
	if current == nil {
		return nil, ErrSubmitterNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. A field equal to its current value is not a change
	// (the edit form sends every field), so only differing fields are recorded.
	changes := make(map[string]any)

	if input.Name != nil && *input.Name != current.Name {
		// Only a changed name is validated: an imported submitter may carry a
		// stored name (e.g. empty) that must not block an unrelated edit.
		changed := domain.Submitter{ID: current.ID, Name: *input.Name}
		if err := changed.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		changes["name"] = *input.Name
	}
	recordAddressChange(changes, input.Address, current.Address)
	recordListChange(changes, "phone", input.Phone, current.Phone)
	recordListChange(changes, "email", input.Email, current.Email)
	if language, ok := input.Language.Change(); ok && language != current.Language {
		changes["language"] = language
	}
	recordIDChange(changes, "media_id", input.MediaID, current.MediaID)

	// No changes?
	if len(changes) == 0 {
		return &UpdateSubmitterResult{Version: current.Version}, nil
	}

	// Create event
	event := domain.NewSubmitterUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Submitter", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, fmt.Errorf("executing update submitter command: %w", err)
	}

	return &UpdateSubmitterResult{Version: version}, nil
}

// DeleteSubmitter deletes a submitter record.
func (h *Handler) DeleteSubmitter(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current submitter from read model
	current, err := h.readStore.GetSubmitter(ctx, id)
	if err != nil {
		return fmt.Errorf("getting submitter: %w", err)
	}
	if current == nil {
		return ErrSubmitterNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Create event
	event := domain.NewSubmitterDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "Submitter", []domain.Event{event}, version)
	if err != nil {
		return fmt.Errorf("executing delete submitter command: %w", err)
	}
	return nil
}
