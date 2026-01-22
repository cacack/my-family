package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Association-related errors.
var (
	ErrAssociationNotFound = errors.New("association not found")
)

// CreateAssociationInput contains the data for creating a new association.
type CreateAssociationInput struct {
	PersonID    uuid.UUID
	AssociateID uuid.UUID
	Role        string
	Phrase      string
	Notes       string
	NoteIDs     []uuid.UUID
	GedcomXref  string
}

// CreateAssociationResult contains the result of creating an association.
type CreateAssociationResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateAssociation creates a new association record.
func (h *Handler) CreateAssociation(ctx context.Context, input CreateAssociationInput) (*CreateAssociationResult, error) {
	// Create association entity
	association := domain.NewAssociation(input.PersonID, input.AssociateID, input.Role)

	if input.Phrase != "" {
		association.SetPhrase(input.Phrase)
	}
	if input.Notes != "" {
		association.SetNotes(input.Notes)
	}
	for _, noteID := range input.NoteIDs {
		association.AddNoteID(noteID)
	}
	if input.GedcomXref != "" {
		association.SetGedcomXref(input.GedcomXref)
	}

	// Validate association
	if err := association.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Verify that PersonID and AssociateID exist
	person, err := h.readStore.GetPerson(ctx, input.PersonID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify person: %w", err)
	}
	if person == nil {
		return nil, fmt.Errorf("%w: person %s not found", ErrInvalidInput, input.PersonID)
	}

	associate, err := h.readStore.GetPerson(ctx, input.AssociateID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify associate: %w", err)
	}
	if associate == nil {
		return nil, fmt.Errorf("%w: associate %s not found", ErrInvalidInput, input.AssociateID)
	}

	// Create event
	event := domain.NewAssociationCreated(association)

	// Execute command (append + project)
	version, err := h.execute(ctx, association.ID.String(), "Association", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &CreateAssociationResult{
		ID:      association.ID,
		Version: version,
	}, nil
}

// UpdateAssociationInput contains the data for updating an association.
type UpdateAssociationInput struct {
	ID      uuid.UUID
	Role    *string
	Phrase  *string
	Notes   *string
	NoteIDs *[]uuid.UUID
	Version int64 // Required for optimistic locking
}

// UpdateAssociationResult contains the result of updating an association.
type UpdateAssociationResult struct {
	Version int64
}

// UpdateAssociation updates an existing association record.
func (h *Handler) UpdateAssociation(ctx context.Context, input UpdateAssociationInput) (*UpdateAssociationResult, error) {
	// Get current association from read model
	current, err := h.readStore.GetAssociation(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrAssociationNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	if input.Role != nil && *input.Role != current.Role {
		if *input.Role == "" {
			return nil, fmt.Errorf("%w: role cannot be empty", ErrInvalidInput)
		}
		changes["role"] = *input.Role
	}

	if input.Phrase != nil && *input.Phrase != current.Phrase {
		changes["phrase"] = *input.Phrase
	}

	if input.Notes != nil && *input.Notes != current.Notes {
		changes["notes"] = *input.Notes
	}

	if input.NoteIDs != nil {
		// Convert to string slice for comparison
		currentNoteIDs := make([]string, len(current.NoteIDs))
		for i, id := range current.NoteIDs {
			currentNoteIDs[i] = id.String()
		}
		newNoteIDs := make([]string, len(*input.NoteIDs))
		for i, id := range *input.NoteIDs {
			newNoteIDs[i] = id.String()
		}
		if !equalStringSlices(currentNoteIDs, newNoteIDs) {
			changes["note_ids"] = *input.NoteIDs
		}
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdateAssociationResult{Version: current.Version}, nil
	}

	// Create event
	event := domain.NewAssociationUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Association", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateAssociationResult{Version: version}, nil
}

// DeleteAssociation deletes an association record.
func (h *Handler) DeleteAssociation(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current association from read model
	current, err := h.readStore.GetAssociation(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrAssociationNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Create event
	event := domain.NewAssociationDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "Association", []domain.Event{event}, version)
	return err
}

// equalStringSlices compares two string slices for equality.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
