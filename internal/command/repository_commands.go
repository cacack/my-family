package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Repository-related errors.
var (
	ErrRepositoryNotFound = errors.New("repository not found")
)

// CreateRepositoryInput contains the data for creating a new repository.
type CreateRepositoryInput struct {
	Name       string
	Address    *domain.Address
	Notes      string
	GedcomXref string
}

// CreateRepositoryResult contains the result of creating a repository.
type CreateRepositoryResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateRepository creates a new repository record.
func (h *Handler) CreateRepository(ctx context.Context, input CreateRepositoryInput) (*CreateRepositoryResult, error) {
	// Create repository entity
	repo := domain.NewRepository(input.Name)

	if input.Address != nil {
		repo.SetAddress(input.Address)
	}
	if input.Notes != "" {
		repo.Notes = input.Notes
	}
	if input.GedcomXref != "" {
		if err := validateGedcomXref(input.GedcomXref); err != nil {
			return nil, err
		}
		repo.GedcomXref = input.GedcomXref
	}

	// Validate repository
	if err := repo.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewRepositoryCreated(repo)

	// Execute command (append + project)
	version, err := h.execute(ctx, repo.ID.String(), "Repository", []domain.Event{event}, -1)
	if err != nil {
		return nil, fmt.Errorf("executing create repository command: %w", err)
	}

	return &CreateRepositoryResult{
		ID:      repo.ID,
		Version: version,
	}, nil
}

// UpdateRepositoryInput contains the data for updating a repository.
type UpdateRepositoryInput struct {
	ID         uuid.UUID
	Name       *string // required: cannot be cleared
	Address    Patch[domain.Address]
	Notes      Patch[string]
	GedcomXref Patch[string]
	Version    int64 // Required for optimistic locking
}

// UpdateRepositoryResult contains the result of updating a repository.
type UpdateRepositoryResult struct {
	Version int64
}

// UpdateRepository updates an existing repository record.
func (h *Handler) UpdateRepository(ctx context.Context, input UpdateRepositoryInput) (*UpdateRepositoryResult, error) {
	// Get current repository from read model
	current, err := h.readStore.GetRepository(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting repository: %w", err)
	}
	if current == nil {
		return nil, ErrRepositoryNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. Keys must match the projection's RepositoryUpdated
	// handler exactly: name, address (*domain.Address, nil when cleared),
	// notes, gedcom_xref. A field equal to its current value is not a change,
	// so only differing fields are recorded.
	changes := make(map[string]any)

	if input.Name != nil && *input.Name != current.Name {
		// Only a changed name is validated: an imported repository may carry a
		// stored name (e.g. empty) that must not block an unrelated edit.
		changed := domain.Repository{ID: current.ID, Name: *input.Name}
		if err := changed.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		changes["name"] = *input.Name
	}
	recordAddressChange(changes, input.Address, current.Address)
	if notes, ok := input.Notes.Change(); ok && notes != current.Notes {
		changes["notes"] = notes
	}
	if xref, ok := input.GedcomXref.Change(); ok && xref != current.GedcomXref {
		if err := validateGedcomXref(xref); err != nil {
			return nil, err
		}
		changes["gedcom_xref"] = xref
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdateRepositoryResult{Version: current.Version}, nil
	}

	// Create event
	event := domain.NewRepositoryUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Repository", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, fmt.Errorf("executing update repository command: %w", err)
	}

	return &UpdateRepositoryResult{Version: version}, nil
}

// recordAddressChange records under "address" a changed address, or nil when
// the update clears an address that is set. A missing address compares as an
// empty one, so a form that sends a blank address for it records no change.
func recordAddressChange(changes map[string]any, input Patch[domain.Address], current *domain.Address) {
	if input.Cleared() {
		if current != nil {
			changes["address"] = nil
		}
		return
	}
	addr, ok := input.Value()
	if !ok {
		return
	}
	var currentAddress domain.Address
	if current != nil {
		currentAddress = *current
	}
	if addr != currentAddress {
		changes["address"] = &addr
	}
}

// DeleteRepository deletes a repository record.
func (h *Handler) DeleteRepository(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current repository from read model
	current, err := h.readStore.GetRepository(ctx, id)
	if err != nil {
		return fmt.Errorf("getting repository: %w", err)
	}
	if current == nil {
		return ErrRepositoryNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Create event
	event := domain.NewRepositoryDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "Repository", []domain.Event{event}, version)
	if err != nil {
		return fmt.Errorf("executing delete repository command: %w", err)
	}
	return nil
}
