// Package domain provides the core domain types for the genealogy application.
package domain

import (
	"errors"
	"fmt"
	"slices"

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

// AllLDSOrdinanceTypes returns every valid LDSOrdinanceType value.
func AllLDSOrdinanceTypes() []LDSOrdinanceType {
	return []LDSOrdinanceType{LDSBaptism, LDSConfirmation, LDSEndowment, LDSSealingChild, LDSSealingSpouse}
}

// IsValid reports whether the value is one of AllLDSOrdinanceTypes.
func (t LDSOrdinanceType) IsValid() bool {
	return slices.Contains(AllLDSOrdinanceTypes(), t)
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

// LDSOrdinanceStatus is the status of an LDS ordinance: the GEDCOM 7 STAT
// enumeration (g7:enumset-ord-STAT).
type LDSOrdinanceStatus string

// LDS ordinance status constants.
const (
	LDSStatusBIC       LDSOrdinanceStatus = "BIC"       // Born in the covenant
	LDSStatusCanceled  LDSOrdinanceStatus = "CANCELED"  // Sealing canceled (divorce)
	LDSStatusChild     LDSOrdinanceStatus = "CHILD"     // Died before age 8
	LDSStatusCompleted LDSOrdinanceStatus = "COMPLETED" // Completed, date unknown
	LDSStatusExcluded  LDSOrdinanceStatus = "EXCLUDED"  // Patron excluded from the ordinance
	LDSStatusDNS       LDSOrdinanceStatus = "DNS"       // Do not seal
	LDSStatusDNSCan    LDSOrdinanceStatus = "DNS_CAN"   // Do not seal, previous sealing canceled
	LDSStatusInfant    LDSOrdinanceStatus = "INFANT"    // Died before age 1
	LDSStatusPre1970   LDSOrdinanceStatus = "PRE_1970"  // Completed before 1970
	LDSStatusStillborn LDSOrdinanceStatus = "STILLBORN" // Born dead, no ordinance needed
	LDSStatusSubmitted LDSOrdinanceStatus = "SUBMITTED" // Submitted, not yet approved
	LDSStatusUncleared LDSOrdinanceStatus = "UNCLEARED" // Data insufficient for clearance
)

// AllLDSOrdinanceStatuses returns every valid LDSOrdinanceStatus value.
func AllLDSOrdinanceStatuses() []LDSOrdinanceStatus {
	return []LDSOrdinanceStatus{
		LDSStatusBIC, LDSStatusCanceled, LDSStatusChild, LDSStatusCompleted,
		LDSStatusExcluded, LDSStatusDNS, LDSStatusDNSCan, LDSStatusInfant,
		LDSStatusPre1970, LDSStatusStillborn, LDSStatusSubmitted, LDSStatusUncleared,
	}
}

// IsValid reports whether the value is empty (unset) or one of
// AllLDSOrdinanceStatuses.
func (s LDSOrdinanceStatus) IsValid() bool {
	return s == "" || slices.Contains(AllLDSOrdinanceStatuses(), s)
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

	if !LDSOrdinanceStatus(o.Status).IsValid() {
		errs = append(errs, LDSOrdinanceValidationError{Field: "status", Message: fmt.Sprintf("invalid value: %s", o.Status)})
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
