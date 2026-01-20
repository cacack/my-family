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
		return nil, err
	}

	return &CreateSubmitterResult{
		ID:      submitter.ID,
		Version: version,
	}, nil
}

// UpdateSubmitterInput contains the data for updating a submitter.
type UpdateSubmitterInput struct {
	ID       uuid.UUID
	Name     *string
	Address  *domain.Address
	Phone    []string
	Email    []string
	Language *string
	MediaID  *uuid.UUID
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
		return nil, err
	}
	if current == nil {
		return nil, ErrSubmitterNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	if input.Name != nil && *input.Name != current.Name {
		changes["name"] = *input.Name
	}
	if input.Address != nil {
		changes["address"] = input.Address
	}
	if input.Phone != nil {
		changes["phone"] = input.Phone
	}
	if input.Email != nil {
		changes["email"] = input.Email
	}
	if input.Language != nil && *input.Language != current.Language {
		changes["language"] = *input.Language
	}
	if input.MediaID != nil {
		changes["media_id"] = input.MediaID
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdateSubmitterResult{Version: current.Version}, nil
	}

	// Create event
	event := domain.NewSubmitterUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Submitter", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateSubmitterResult{Version: version}, nil
}

// DeleteSubmitter deletes a submitter record.
func (h *Handler) DeleteSubmitter(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current submitter from read model
	current, err := h.readStore.GetSubmitter(ctx, id)
	if err != nil {
		return err
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
	return err
}
