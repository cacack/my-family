// Package domain provides the core domain types for the genealogy application.
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// SubmitterValidationError represents a validation error for a Submitter.
type SubmitterValidationError struct {
	Field   string
	Message string
}

func (e SubmitterValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Submitter represents a GEDCOM SUBM (Submitter) record.
// Submitters track who created or submitted genealogical data, useful for
// tracking data provenance, contacting original researchers, and crediting contributors.
type Submitter struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`                  // NAME - Submitter's name (required)
	Address    *Address   `json:"address,omitempty"`     // ADDR - Structured address
	Phone      []string   `json:"phone,omitempty"`       // PHON - Multiple phone numbers
	Email      []string   `json:"email,omitempty"`       // EMAIL - Multiple email addresses
	Language   string     `json:"language,omitempty"`    // LANG - Preferred language
	MediaID    *uuid.UUID `json:"media_id,omitempty"`    // OBJE - Link to submitter photo
	GedcomXref string     `json:"gedcom_xref,omitempty"` // GEDCOM cross-reference ID for round-trip
	Version    int64      `json:"version"`
}

// NewSubmitter creates a new Submitter with a generated UUID.
func NewSubmitter(name string) *Submitter {
	return &Submitter{
		ID:      uuid.New(),
		Name:    name,
		Version: 1,
	}
}

// NewSubmitterWithID creates a new Submitter with a specific UUID.
// This is useful for importing from GEDCOM where we need to assign IDs.
func NewSubmitterWithID(id uuid.UUID, name string) *Submitter {
	return &Submitter{
		ID:      id,
		Name:    name,
		Version: 1,
	}
}

// Validate checks if the Submitter is valid.
func (s *Submitter) Validate() error {
	// Name is required
	if s.Name == "" {
		return &SubmitterValidationError{Field: "name", Message: "cannot be empty"}
	}
	if len(s.Name) > 200 {
		return &SubmitterValidationError{Field: "name", Message: "cannot exceed 200 characters"}
	}
	return nil
}

// SetName updates the submitter name.
func (s *Submitter) SetName(name string) {
	s.Name = name
}

// SetAddress sets the submitter's address.
func (s *Submitter) SetAddress(addr *Address) {
	s.Address = addr
}

// AddPhone adds a phone number to the submitter.
func (s *Submitter) AddPhone(phone string) {
	if phone != "" {
		s.Phone = append(s.Phone, phone)
	}
}

// AddEmail adds an email address to the submitter.
func (s *Submitter) AddEmail(email string) {
	if email != "" {
		s.Email = append(s.Email, email)
	}
}

// SetLanguage sets the submitter's preferred language.
func (s *Submitter) SetLanguage(lang string) {
	s.Language = lang
}

// SetMediaID sets the ID of the submitter's photo.
func (s *Submitter) SetMediaID(mediaID *uuid.UUID) {
	s.MediaID = mediaID
}

// SetGedcomXref sets the GEDCOM cross-reference ID for round-trip support.
func (s *Submitter) SetGedcomXref(xref string) {
	s.GedcomXref = xref
}
