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
	Name       *string
	Address    *domain.Address
	Notes      *string
	GedcomXref *string
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
	// handler exactly: name, address (*domain.Address), notes, gedcom_xref.
	changes := make(map[string]any)

	if input.Name != nil && *input.Name != current.Name {
		changes["name"] = *input.Name
	}
	if input.Address != nil {
		changes["address"] = input.Address
	}
	if input.Notes != nil && *input.Notes != current.Notes {
		changes["notes"] = *input.Notes
	}
	if input.GedcomXref != nil && *input.GedcomXref != current.GedcomXref {
		changes["gedcom_xref"] = *input.GedcomXref
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
