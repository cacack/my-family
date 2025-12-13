package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Family represents a family unit linking partners and their children.
type Family struct {
	ID               uuid.UUID    `json:"id"`
	Partner1ID       *uuid.UUID   `json:"partner1_id,omitempty"`
	Partner2ID       *uuid.UUID   `json:"partner2_id,omitempty"`
	RelationshipType RelationType `json:"relationship_type,omitempty"`
	MarriageDate     *GenDate     `json:"marriage_date,omitempty"`
	MarriagePlace    string       `json:"marriage_place,omitempty"`
	GedcomXref       string       `json:"gedcom_xref,omitempty"` // Original GEDCOM @XREF@ for round-trip
	Version          int64        `json:"version"`               // Optimistic locking version
}

// FamilyChild represents the junction entity linking children to families.
type FamilyChild struct {
	FamilyID         uuid.UUID         `json:"family_id"`
	PersonID         uuid.UUID         `json:"person_id"`
	RelationshipType ChildRelationType `json:"relationship_type"`
	Sequence         *int              `json:"sequence,omitempty"` // Birth order (optional)
}

// FamilyValidationError represents a validation error for a Family.
type FamilyValidationError struct {
	Field   string
	Message string
}

func (e FamilyValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewFamily creates a new Family with generated ID.
func NewFamily() *Family {
	return &Family{
		ID:      uuid.New(),
		Version: 1,
	}
}

// NewFamilyWithPartners creates a new Family with the specified partners.
func NewFamilyWithPartners(partner1, partner2 *uuid.UUID) *Family {
	return &Family{
		ID:         uuid.New(),
		Partner1ID: partner1,
		Partner2ID: partner2,
		Version:    1,
	}
}

// Validate checks if the family has valid data.
func (f *Family) Validate() error {
	var errs []error

	// At least one partner must be set
	if f.Partner1ID == nil && f.Partner2ID == nil {
		errs = append(errs, FamilyValidationError{Field: "partners", Message: "at least one partner must be set"})
	}

	// Partners must be different if both set
	if f.Partner1ID != nil && f.Partner2ID != nil && *f.Partner1ID == *f.Partner2ID {
		errs = append(errs, FamilyValidationError{Field: "partner2_id", Message: "cannot be the same as partner1_id"})
	}

	// Relationship type validation
	if !f.RelationshipType.IsValid() {
		errs = append(errs, FamilyValidationError{Field: "relationship_type", Message: fmt.Sprintf("invalid value: %s", f.RelationshipType)})
	}

	// Marriage date validation
	if f.MarriageDate != nil {
		if err := f.MarriageDate.Validate(); err != nil {
			errs = append(errs, FamilyValidationError{Field: "marriage_date", Message: err.Error()})
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// HasPartner checks if the given person ID is a partner in this family.
func (f *Family) HasPartner(personID uuid.UUID) bool {
	return (f.Partner1ID != nil && *f.Partner1ID == personID) ||
		(f.Partner2ID != nil && *f.Partner2ID == personID)
}

// SetMarriageDate sets the marriage date from a string.
func (f *Family) SetMarriageDate(dateStr string) {
	if dateStr == "" {
		f.MarriageDate = nil
		return
	}
	gd := ParseGenDate(dateStr)
	f.MarriageDate = &gd
}

// NewFamilyChild creates a new FamilyChild junction entity.
func NewFamilyChild(familyID, personID uuid.UUID, relType ChildRelationType) *FamilyChild {
	if relType == "" {
		relType = ChildBiological
	}
	return &FamilyChild{
		FamilyID:         familyID,
		PersonID:         personID,
		RelationshipType: relType,
	}
}

// Validate checks if the family child relationship is valid.
func (fc *FamilyChild) Validate() error {
	var errs []error

	if fc.FamilyID == uuid.Nil {
		errs = append(errs, errors.New("family_id cannot be empty"))
	}
	if fc.PersonID == uuid.Nil {
		errs = append(errs, errors.New("person_id cannot be empty"))
	}
	if !fc.RelationshipType.IsValid() {
		errs = append(errs, fmt.Errorf("invalid relationship_type: %s", fc.RelationshipType))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ValidateChildNotPartner checks that a child is not also a partner in the family.
func ValidateChildNotPartner(family *Family, childID uuid.UUID) error {
	if family.HasPartner(childID) {
		return FamilyValidationError{
			Field:   "person_id",
			Message: "child cannot be the same as a partner in the family",
		}
	}
	return nil
}
