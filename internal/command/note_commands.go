package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Note-related errors.
var (
	ErrNoteNotFound = errors.New("note not found")
)

// CreateNoteInput contains the data for creating a new note.
type CreateNoteInput struct {
	Text       string
	GedcomXref string
}

// CreateNoteResult contains the result of creating a note.
type CreateNoteResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateNote creates a new note record.
func (h *Handler) CreateNote(ctx context.Context, input CreateNoteInput) (*CreateNoteResult, error) {
	// Create note entity
	note := domain.NewNote(input.Text)

	if input.GedcomXref != "" {
		note.SetGedcomXref(input.GedcomXref)
	}

	// Validate note
	if err := note.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewNoteCreated(note)

	// Execute command (append + project)
	version, err := h.execute(ctx, note.ID.String(), "Note", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &CreateNoteResult{
		ID:      note.ID,
		Version: version,
	}, nil
}

// UpdateNoteInput contains the data for updating a note.
type UpdateNoteInput struct {
	ID      uuid.UUID
	Text    *string
	Version int64 // Required for optimistic locking
}

// UpdateNoteResult contains the result of updating a note.
type UpdateNoteResult struct {
	Version int64
}

// UpdateNote updates an existing note record.
func (h *Handler) UpdateNote(ctx context.Context, input UpdateNoteInput) (*UpdateNoteResult, error) {
	// Get current note from read model
	current, err := h.readStore.GetNote(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrNoteNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	if input.Text != nil && *input.Text != current.Text {
		changes["text"] = *input.Text
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdateNoteResult{Version: current.Version}, nil
	}

	// Create event
	event := domain.NewNoteUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Note", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateNoteResult{Version: version}, nil
}

// DeleteNote deletes a note record.
func (h *Handler) DeleteNote(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current note from read model
	current, err := h.readStore.GetNote(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrNoteNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Create event
	event := domain.NewNoteDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "Note", []domain.Event{event}, version)
	return err
}
