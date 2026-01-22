package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// LifeEvent represents an occurrence in a person's or family's history.
// Examples include baptism, burial, census, emigration, etc.
type LifeEvent struct {
	ID             uuid.UUID      `json:"id"`
	PersonID       *uuid.UUID     `json:"person_id,omitempty"` // nil for family events
	FamilyID       *uuid.UUID     `json:"family_id,omitempty"` // nil for person events
	FactType       FactType       `json:"fact_type"`
	Date           *GenDate       `json:"date,omitempty"`            // when it occurred
	Place          string         `json:"place,omitempty"`           // where it occurred
	Address        *Address       `json:"address,omitempty"`         // structured address (RESI, etc.)
	Description    string         `json:"description,omitempty"`     // additional details
	Cause          string         `json:"cause,omitempty"`           // cause of event (e.g., death cause)
	Age            string         `json:"age,omitempty"`             // age at time of event
	ResearchStatus ResearchStatus `json:"research_status,omitempty"` // Confidence level (GPS-compliant)
	GedcomXref     string         `json:"gedcom_xref,omitempty"`     // for round-trip preservation
	Version        int64          `json:"version"`                   // optimistic locking
}

// LifeEventValidationError represents a validation error for a LifeEvent.
type LifeEventValidationError struct {
	Field   string
	Message string
}

func (e LifeEventValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewLifeEvent creates a new LifeEvent for a person.
func NewLifeEvent(personID uuid.UUID, factType FactType) *LifeEvent {
	return &LifeEvent{
		ID:       uuid.New(),
		PersonID: &personID,
		FactType: factType,
		Version:  1,
	}
}

// NewFamilyLifeEvent creates a new LifeEvent for a family.
func NewFamilyLifeEvent(familyID uuid.UUID, factType FactType) *LifeEvent {
	return &LifeEvent{
		ID:       uuid.New(),
		FamilyID: &familyID,
		FactType: factType,
		Version:  1,
	}
}

// Validate checks if the life event has valid data.
func (e *LifeEvent) Validate() error {
	var errs []error

	// Exactly one of PersonID or FamilyID must be set
	if e.PersonID == nil && e.FamilyID == nil {
		errs = append(errs, LifeEventValidationError{
			Field:   "owner",
			Message: "either person_id or family_id must be set",
		})
	}
	if e.PersonID != nil && e.FamilyID != nil {
		errs = append(errs, LifeEventValidationError{
			Field:   "owner",
			Message: "cannot set both person_id and family_id",
		})
	}

	// FactType is required and must be valid
	if e.FactType == "" {
		errs = append(errs, LifeEventValidationError{
			Field:   "fact_type",
			Message: "cannot be empty",
		})
	} else if !e.FactType.IsValid() {
		errs = append(errs, LifeEventValidationError{
			Field:   "fact_type",
			Message: fmt.Sprintf("invalid value: %s", e.FactType),
		})
	}

	// Validate date if present
	if e.Date != nil {
		if err := e.Date.Validate(); err != nil {
			errs = append(errs, LifeEventValidationError{
				Field:   "date",
				Message: err.Error(),
			})
		}
	}

	// Validate place length
	if len(e.Place) > 500 {
		errs = append(errs, LifeEventValidationError{
			Field:   "place",
			Message: "cannot exceed 500 characters",
		})
	}

	// Validate description length
	if len(e.Description) > 2000 {
		errs = append(errs, LifeEventValidationError{
			Field:   "description",
			Message: "cannot exceed 2000 characters",
		})
	}

	// Validate research status
	if !e.ResearchStatus.IsValid() {
		errs = append(errs, LifeEventValidationError{
			Field:   "research_status",
			Message: fmt.Sprintf("invalid value: %s", e.ResearchStatus),
		})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// IsPersonEvent returns true if this event belongs to a person.
func (e *LifeEvent) IsPersonEvent() bool {
	return e.PersonID != nil
}

// IsFamilyEvent returns true if this event belongs to a family.
func (e *LifeEvent) IsFamilyEvent() bool {
	return e.FamilyID != nil
}

// OwnerID returns the ID of the entity this event belongs to.
func (e *LifeEvent) OwnerID() uuid.UUID {
	if e.PersonID != nil {
		return *e.PersonID
	}
	if e.FamilyID != nil {
		return *e.FamilyID
	}
	return uuid.Nil
}

// SetDate sets the date from a string.
func (e *LifeEvent) SetDate(dateStr string) {
	if dateStr == "" {
		e.Date = nil
		return
	}
	gd := ParseGenDate(dateStr)
	e.Date = &gd
}

// Attribute represents a biographical attribute of a person.
// Examples include occupation, residence, education, religion, title.
type Attribute struct {
	ID         uuid.UUID `json:"id"`
	PersonID   uuid.UUID `json:"person_id"` // required, attributes are person-only
	FactType   FactType  `json:"fact_type"`
	Value      string    `json:"value"`                 // the attribute value (e.g., "Blacksmith")
	Date       *GenDate  `json:"date,omitempty"`        // period of applicability
	Place      string    `json:"place,omitempty"`       // where applicable
	GedcomXref string    `json:"gedcom_xref,omitempty"` // for round-trip preservation
	Version    int64     `json:"version"`               // optimistic locking
}

// AttributeValidationError represents a validation error for an Attribute.
type AttributeValidationError struct {
	Field   string
	Message string
}

func (e AttributeValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewAttribute creates a new Attribute for a person.
func NewAttribute(personID uuid.UUID, factType FactType, value string) *Attribute {
	return &Attribute{
		ID:       uuid.New(),
		PersonID: personID,
		FactType: factType,
		Value:    value,
		Version:  1,
	}
}

// Validate checks if the attribute has valid data.
func (a *Attribute) Validate() error {
	var errs []error

	// PersonID is required
	if a.PersonID == uuid.Nil {
		errs = append(errs, AttributeValidationError{
			Field:   "person_id",
			Message: "cannot be empty",
		})
	}

	// FactType is required and must be valid
	if a.FactType == "" {
		errs = append(errs, AttributeValidationError{
			Field:   "fact_type",
			Message: "cannot be empty",
		})
	} else if !a.FactType.IsValid() {
		errs = append(errs, AttributeValidationError{
			Field:   "fact_type",
			Message: fmt.Sprintf("invalid value: %s", a.FactType),
		})
	}

	// Value is required
	if a.Value == "" {
		errs = append(errs, AttributeValidationError{
			Field:   "value",
			Message: "cannot be empty",
		})
	}

	// Validate value length
	if len(a.Value) > 500 {
		errs = append(errs, AttributeValidationError{
			Field:   "value",
			Message: "cannot exceed 500 characters",
		})
	}

	// Validate date if present
	if a.Date != nil {
		if err := a.Date.Validate(); err != nil {
			errs = append(errs, AttributeValidationError{
				Field:   "date",
				Message: err.Error(),
			})
		}
	}

	// Validate place length
	if len(a.Place) > 500 {
		errs = append(errs, AttributeValidationError{
			Field:   "place",
			Message: "cannot exceed 500 characters",
		})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// SetDate sets the date from a string.
func (a *Attribute) SetDate(dateStr string) {
	if dateStr == "" {
		a.Date = nil
		return
	}
	gd := ParseGenDate(dateStr)
	a.Date = &gd
}
