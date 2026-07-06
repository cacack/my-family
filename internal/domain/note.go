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

// NoteTranslation is an alternate-language rendering of a shared note (SNOTE).
// It mirrors the GEDCOM 7.0 TRAN substructure, which carries its own MIME media
// type and BCP 47 language tag independent of the primary text.
type NoteTranslation struct {
	Text     string `json:"text"`               // Translated note content
	MIME     string `json:"mime,omitempty"`     // Media type, e.g. "text/plain" or "text/html"
	Language string `json:"language,omitempty"` // BCP 47 language tag, e.g. "es"
}

// Note represents a GEDCOM NOTE record that can be shared across multiple entities.
// GEDCOM supports two note styles:
// - Inline notes: embedded directly in an entity
// - Shared notes: top-level NOTE records referenced by multiple entities via @N1@
//
// GEDCOM 7.0 adds the SNOTE (shared note) record, which enriches a shared note
// with a MIME media type (plain text vs HTML), a BCP 47 language tag, and
// alternate-language translations. A Note carrying any of that metadata is
// treated as a shared note and round-trips as an SNOTE record; see IsShared.
type Note struct {
	ID           uuid.UUID         `json:"id"`
	Text         string            `json:"text"`                   // Full text with embedded newlines
	MIME         string            `json:"mime,omitempty"`         // Media type (SNOTE), e.g. "text/html"
	Language     string            `json:"language,omitempty"`     // BCP 47 language tag (SNOTE), e.g. "en"
	Translations []NoteTranslation `json:"translations,omitempty"` // Alternate-language renderings (SNOTE TRAN)
	GedcomXref   string            `json:"gedcom_xref,omitempty"`  // GEDCOM cross-reference ID (e.g., "@N1@")
	Version      int64             `json:"version"`
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

// SetSharedNoteMetadata sets the GEDCOM 7.0 SNOTE metadata: the MIME media type,
// BCP 47 language tag, and alternate-language translations.
func (n *Note) SetSharedNoteMetadata(mime, language string, translations []NoteTranslation) {
	n.MIME = mime
	n.Language = language
	n.Translations = translations
}

// IsShared reports whether this note carries GEDCOM 7.0 shared-note (SNOTE)
// metadata — a MIME media type, a language tag, or any translations. Such notes
// are exported as SNOTE records; notes without metadata export as plain NOTE
// records for GEDCOM 5.5.1 compatibility.
func (n *Note) IsShared() bool {
	return n.MIME != "" || n.Language != "" || len(n.Translations) > 0
}
