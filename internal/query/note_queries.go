package query

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// NoteService provides query operations for notes.
type NoteService struct {
	readStore repository.ReadModelStore
}

// NewNoteService creates a new note query service.
func NewNoteService(readStore repository.ReadModelStore) *NoteService {
	return &NoteService{readStore: readStore}
}

// Note represents a note in query results.
type Note struct {
	ID         uuid.UUID `json:"id"`
	Text       string    `json:"text"`
	GedcomXref *string   `json:"gedcom_xref,omitempty"`
	Version    int64     `json:"version"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListNotesInput contains options for listing notes.
type ListNotesInput struct {
	Limit     int
	Offset    int
	SortOrder string // asc, desc (sorted by updated_at)
}

// NoteListResult contains paginated note results.
type NoteListResult struct {
	Notes  []Note `json:"notes"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// ListNotes returns a paginated list of notes.
func (s *NoteService) ListNotes(ctx context.Context, input ListNotesInput) (*NoteListResult, error) {
	opts := repository.ListOptions{
		Limit:  input.Limit,
		Offset: input.Offset,
		Order:  input.SortOrder,
	}

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if opts.Order == "" {
		opts.Order = "desc"
	}

	readModels, total, err := s.readStore.ListNotes(ctx, opts)
	if err != nil {
		return nil, err
	}

	notes := make([]Note, len(readModels))
	for i, rm := range readModels {
		notes[i] = convertReadModelToNote(rm)
	}

	return &NoteListResult{
		Notes:  notes,
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}

// GetNote returns a note by ID.
func (s *NoteService) GetNote(ctx context.Context, id uuid.UUID) (*Note, error) {
	rm, err := s.readStore.GetNote(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	note := convertReadModelToNote(*rm)
	return &note, nil
}

// Helper function to convert read model to query result.
func convertReadModelToNote(rm repository.NoteReadModel) Note {
	n := Note{
		ID:        rm.ID,
		Text:      rm.Text,
		Version:   rm.Version,
		UpdatedAt: rm.UpdatedAt,
	}

	if rm.GedcomXref != "" {
		n.GedcomXref = &rm.GedcomXref
	}

	return n
}
