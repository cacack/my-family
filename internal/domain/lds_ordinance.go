// Package domain provides the core domain types for the genealogy application.
package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// LDSOrdinanceType represents the type of LDS temple ordinance.
// GEDCOM was originally developed by the LDS Church and includes tags for temple ordinances.
type LDSOrdinanceType string

// LDS ordinance type constants mapping to GEDCOM tags.
const (
	LDSBaptism       LDSOrdinanceType = "BAPL" // Baptism (LDS) - Individual
	LDSConfirmation  LDSOrdinanceType = "CONL" // Confirmation (LDS) - Individual
	LDSEndowment     LDSOrdinanceType = "ENDL" // Endowment - Individual
	LDSSealingChild  LDSOrdinanceType = "SLGC" // Sealing to Parents - Individual
	LDSSealingSpouse LDSOrdinanceType = "SLGS" // Sealing to Spouse - Family
)

// IsValid returns true if the ordinance type is recognized.
func (t LDSOrdinanceType) IsValid() bool {
	switch t {
	case LDSBaptism, LDSConfirmation, LDSEndowment, LDSSealingChild, LDSSealingSpouse:
		return true
	}
	return false
}

// IsIndividual returns true if the ordinance is an individual ordinance (not SLGS).
func (t LDSOrdinanceType) IsIndividual() bool {
	return t != LDSSealingSpouse
}

// Label returns a human-readable label for the ordinance type.
func (t LDSOrdinanceType) Label() string {
	switch t {
	case LDSBaptism:
		return "Baptism (LDS)"
	case LDSConfirmation:
		return "Confirmation (LDS)"
	case LDSEndowment:
		return "Endowment"
	case LDSSealingChild:
		return "Sealing to Parents"
	case LDSSealingSpouse:
		return "Sealing to Spouse"
	default:
		return string(t)
	}
}

// LDSOrdinanceValidationError represents a validation error for an LDS Ordinance.
type LDSOrdinanceValidationError struct {
	Field   string
	Message string
}

func (e LDSOrdinanceValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// LDSOrdinance represents an LDS temple ordinance record.
// These are important for users with LDS heritage or data from FamilySearch.
//
// Individual ordinances (BAPL, CONL, ENDL, SLGC) are linked to a Person via PersonID.
// Spouse sealing (SLGS) is linked to a Family via FamilyID.
type LDSOrdinance struct {
	ID       uuid.UUID        `json:"id"`
	Type     LDSOrdinanceType `json:"type"`
	PersonID *uuid.UUID       `json:"person_id,omitempty"` // For individual ordinances
	FamilyID *uuid.UUID       `json:"family_id,omitempty"` // For SLGS (sealing to spouse)
	Date     *GenDate         `json:"date,omitempty"`      // When performed
	Place    string           `json:"place,omitempty"`     // Location (optional)
	Temple   string           `json:"temple,omitempty"`    // Temple code (TEMP)
	Status   string           `json:"status,omitempty"`    // COMPLETED, BIC, CHILD, EXCLUDED, etc.
	Version  int64            `json:"version"`             // Optimistic locking version
}

// NewLDSOrdinance creates a new LDS Ordinance with the given type.
func NewLDSOrdinance(ordinanceType LDSOrdinanceType) *LDSOrdinance {
	return &LDSOrdinance{
		ID:      uuid.New(),
		Type:    ordinanceType,
		Version: 1,
	}
}

// NewLDSOrdinanceWithID creates a new LDS Ordinance with a specific UUID.
// This is useful for importing from GEDCOM where we need to assign IDs.
func NewLDSOrdinanceWithID(id uuid.UUID, ordinanceType LDSOrdinanceType) *LDSOrdinance {
	return &LDSOrdinance{
		ID:      id,
		Type:    ordinanceType,
		Version: 1,
	}
}

// Validate checks if the LDS Ordinance has valid data.
func (o *LDSOrdinance) Validate() error {
	var errs []error

	// Type must be valid
	if !o.Type.IsValid() {
		errs = append(errs, LDSOrdinanceValidationError{Field: "type", Message: "invalid ordinance type"})
	}

	// Individual ordinances need PersonID, SLGS needs FamilyID
	if o.Type.IsValid() {
		if o.Type.IsIndividual() && o.PersonID == nil {
			errs = append(errs, LDSOrdinanceValidationError{Field: "person_id", Message: "required for individual ordinances"})
		}
		if o.Type == LDSSealingSpouse && o.FamilyID == nil {
			errs = append(errs, LDSOrdinanceValidationError{Field: "family_id", Message: "required for spouse sealing"})
		}
	}

	// Individual ordinances should not have FamilyID
	if o.Type.IsIndividual() && o.FamilyID != nil {
		errs = append(errs, LDSOrdinanceValidationError{Field: "family_id", Message: "should not be set for individual ordinances"})
	}

	// Spouse sealing should not have PersonID
	if o.Type == LDSSealingSpouse && o.PersonID != nil {
		errs = append(errs, LDSOrdinanceValidationError{Field: "person_id", Message: "should not be set for spouse sealing"})
	}

	// Date validation
	if o.Date != nil {
		if err := o.Date.Validate(); err != nil {
			errs = append(errs, LDSOrdinanceValidationError{Field: "date", Message: err.Error()})
		}
	}

	// Temple code validation (optional but should be reasonable length)
	if len(o.Temple) > 10 {
		errs = append(errs, LDSOrdinanceValidationError{Field: "temple", Message: "temple code cannot exceed 10 characters"})
	}

	// Status validation (optional but should be reasonable length)
	if len(o.Status) > 50 {
		errs = append(errs, LDSOrdinanceValidationError{Field: "status", Message: "status cannot exceed 50 characters"})
	}

	// Place validation
	if len(o.Place) > 255 {
		errs = append(errs, LDSOrdinanceValidationError{Field: "place", Message: "place cannot exceed 255 characters"})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// SetDate sets the ordinance date from a string.
func (o *LDSOrdinance) SetDate(dateStr string) {
	if dateStr == "" {
		o.Date = nil
		return
	}
	gd := ParseGenDate(dateStr)
	o.Date = &gd
}

// SetPersonID sets the person ID for individual ordinances.
func (o *LDSOrdinance) SetPersonID(personID uuid.UUID) {
	o.PersonID = &personID
}

// SetFamilyID sets the family ID for spouse sealing.
func (o *LDSOrdinance) SetFamilyID(familyID uuid.UUID) {
	o.FamilyID = &familyID
}

// SetTemple sets the temple code.
func (o *LDSOrdinance) SetTemple(temple string) {
	o.Temple = temple
}

// SetStatus sets the ordinance status.
func (o *LDSOrdinance) SetStatus(status string) {
	o.Status = status
}

// SetPlace sets the ordinance place.
func (o *LDSOrdinance) SetPlace(place string) {
	o.Place = place
}
