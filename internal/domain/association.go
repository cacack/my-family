// Package domain provides the core domain types for the genealogy application.
package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Standard association role constants.
// These map to GEDCOM RELA values: GODP (godparent), WITN (witness).
const (
	RoleGodparent = "godparent"
	RoleWitness   = "witness"
)

// AssociationValidationError represents a validation error for an Association.
type AssociationValidationError struct {
	Field   string
	Message string
}

func (e AssociationValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Association represents a GEDCOM ASSO (association) record.
// GEDCOM supports associations to link individuals with specific roles like
// godparents, witnesses, business partners, mentors, etc.
// These capture non-family relationships that are important for genealogical research.
//
// Structure is DIRECTED: PersonID has association with AssociateID.
// This matches the GEDCOM structure where ASSO is a subordinate tag under INDI.
type Association struct {
	ID          uuid.UUID   `json:"id"`
	PersonID    uuid.UUID   `json:"person_id"`             // The individual (INDI containing ASSO)
	AssociateID uuid.UUID   `json:"associate_id"`          // The associated person (@I2@)
	Role        string      `json:"role"`                  // godparent, witness, or custom
	Phrase      string      `json:"phrase,omitempty"`      // GEDCOM 7.0 human-readable description (PHRASE)
	Notes       string      `json:"notes,omitempty"`       // Inline note text
	NoteIDs     []uuid.UUID `json:"note_ids,omitempty"`    // Linked Note entities
	GedcomXref  string      `json:"gedcom_xref,omitempty"` // Original GEDCOM XREF for round-trip
	Version     int64       `json:"version"`               // Optimistic locking version
}

// NewAssociation creates a new Association with the given required fields.
func NewAssociation(personID, associateID uuid.UUID, role string) *Association {
	return &Association{
		ID:          uuid.New(),
		PersonID:    personID,
		AssociateID: associateID,
		Role:        role,
		Version:     1,
	}
}

// NewAssociationWithID creates a new Association with a specific UUID.
// This is useful for importing from GEDCOM where we need to assign IDs.
func NewAssociationWithID(id, personID, associateID uuid.UUID, role string) *Association {
	return &Association{
		ID:          id,
		PersonID:    personID,
		AssociateID: associateID,
		Role:        role,
		Version:     1,
	}
}

// Validate checks if the Association has valid data.
func (a *Association) Validate() error {
	var errs []error

	if a.PersonID == uuid.Nil {
		errs = append(errs, AssociationValidationError{Field: "person_id", Message: "cannot be empty"})
	}
	if a.AssociateID == uuid.Nil {
		errs = append(errs, AssociationValidationError{Field: "associate_id", Message: "cannot be empty"})
	}
	if a.Role == "" {
		errs = append(errs, AssociationValidationError{Field: "role", Message: "cannot be empty"})
	}
	if a.PersonID == a.AssociateID && a.PersonID != uuid.Nil {
		errs = append(errs, AssociationValidationError{Field: "associate_id", Message: "cannot associate with self"})
	}
	if len(a.Role) > 100 {
		errs = append(errs, AssociationValidationError{Field: "role", Message: "cannot exceed 100 characters"})
	}
	if len(a.Phrase) > 500 {
		errs = append(errs, AssociationValidationError{Field: "phrase", Message: "cannot exceed 500 characters"})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// SetPhrase sets the GEDCOM 7.0 human-readable description.
func (a *Association) SetPhrase(phrase string) {
	a.Phrase = phrase
}

// SetNotes sets inline note text.
func (a *Association) SetNotes(notes string) {
	a.Notes = notes
}

// AddNoteID adds a linked Note entity ID.
func (a *Association) AddNoteID(noteID uuid.UUID) {
	if noteID != uuid.Nil {
		a.NoteIDs = append(a.NoteIDs, noteID)
	}
}

// SetGedcomXref sets the GEDCOM cross-reference ID for round-trip support.
func (a *Association) SetGedcomXref(xref string) {
	a.GedcomXref = xref
}
