// Package domain provides the core domain types for the genealogy application.
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// NoteValidationError represents a validation error for a Note.
type NoteValidationError struct {
	Field   string
	Message string
}

func (e NoteValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Note represents a GEDCOM NOTE record that can be shared across multiple entities.
// GEDCOM supports two note styles:
// - Inline notes: embedded directly in an entity
// - Shared notes: top-level NOTE records referenced by multiple entities via @N1@
type Note struct {
	ID         uuid.UUID `json:"id"`
	Text       string    `json:"text"`                  // Full text with embedded newlines
	GedcomXref string    `json:"gedcom_xref,omitempty"` // GEDCOM cross-reference ID (e.g., "@N1@")
	Version    int64     `json:"version"`
}

// NewNote creates a new Note with a generated UUID.
func NewNote(text string) *Note {
	return &Note{
		ID:      uuid.New(),
		Text:    text,
		Version: 1,
	}
}

// NewNoteWithID creates a new Note with a specific UUID.
// This is useful for importing from GEDCOM where we need to assign IDs.
func NewNoteWithID(id uuid.UUID, text string) *Note {
	return &Note{
		ID:      id,
		Text:    text,
		Version: 1,
	}
}

// Validate checks if the Note is valid.
// Notes can have empty text (representing a placeholder or pending note).
func (n *Note) Validate() error {
	// Text can be empty but the Note must have a valid ID
	if n.ID == uuid.Nil {
		return &NoteValidationError{Field: "id", Message: "id is required"}
	}
	return nil
}

// SetText updates the note text.
func (n *Note) SetText(text string) {
	n.Text = text
}

// SetGedcomXref sets the GEDCOM cross-reference ID for round-trip support.
func (n *Note) SetGedcomXref(xref string) {
	n.GedcomXref = xref
}
