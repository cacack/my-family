package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Person represents an individual in the family tree.
type Person struct {
	ID         uuid.UUID `json:"id"`
	GivenName  string    `json:"given_name"`
	Surname    string    `json:"surname"`
	Gender     Gender    `json:"gender,omitempty"`
	BirthDate  *GenDate  `json:"birth_date,omitempty"`
	BirthPlace string    `json:"birth_place,omitempty"`
	DeathDate  *GenDate  `json:"death_date,omitempty"`
	DeathPlace string    `json:"death_place,omitempty"`
	Notes      string    `json:"notes,omitempty"`
	GedcomXref string    `json:"gedcom_xref,omitempty"` // Original GEDCOM @XREF@ for round-trip
	Version    int64     `json:"version"`               // Optimistic locking version
}

// PersonValidationError represents a validation error for a Person.
type PersonValidationError struct {
	Field   string
	Message string
}

func (e PersonValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewPerson creates a new Person with the given required fields.
func NewPerson(givenName, surname string) *Person {
	return &Person{
		ID:        uuid.New(),
		GivenName: givenName,
		Surname:   surname,
		Version:   1,
	}
}

// Validate checks if the person has valid data.
func (p *Person) Validate() error {
	var errs []error

	// Required fields
	if p.GivenName == "" {
		errs = append(errs, PersonValidationError{Field: "given_name", Message: "cannot be empty"})
	}
	if len(p.GivenName) > 100 {
		errs = append(errs, PersonValidationError{Field: "given_name", Message: "cannot exceed 100 characters"})
	}

	if p.Surname == "" {
		errs = append(errs, PersonValidationError{Field: "surname", Message: "cannot be empty"})
	}
	if len(p.Surname) > 100 {
		errs = append(errs, PersonValidationError{Field: "surname", Message: "cannot exceed 100 characters"})
	}

	// Gender validation
	if !p.Gender.IsValid() {
		errs = append(errs, PersonValidationError{Field: "gender", Message: fmt.Sprintf("invalid value: %s", p.Gender)})
	}

	// Date validation
	if p.BirthDate != nil {
		if err := p.BirthDate.Validate(); err != nil {
			errs = append(errs, PersonValidationError{Field: "birth_date", Message: err.Error()})
		}
	}
	if p.DeathDate != nil {
		if err := p.DeathDate.Validate(); err != nil {
			errs = append(errs, PersonValidationError{Field: "death_date", Message: err.Error()})
		}
	}

	// Death date must be after or equal to birth date
	if p.BirthDate != nil && p.DeathDate != nil && !p.BirthDate.IsEmpty() && !p.DeathDate.IsEmpty() {
		if p.DeathDate.Before(*p.BirthDate) {
			errs = append(errs, PersonValidationError{Field: "death_date", Message: "cannot be before birth_date"})
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// FullName returns the full name of the person.
func (p *Person) FullName() string {
	return p.GivenName + " " + p.Surname
}

// SetBirthDate sets the birth date from a string.
func (p *Person) SetBirthDate(dateStr string) {
	if dateStr == "" {
		p.BirthDate = nil
		return
	}
	gd := ParseGenDate(dateStr)
	p.BirthDate = &gd
}

// SetDeathDate sets the death date from a string.
func (p *Person) SetDeathDate(dateStr string) {
	if dateStr == "" {
		p.DeathDate = nil
		return
	}
	gd := ParseGenDate(dateStr)
	p.DeathDate = &gd
}
