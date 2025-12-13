// Package domain contains the core domain types for the genealogy application.
package domain

// Gender represents the gender of a person.
type Gender string

const (
	GenderMale    Gender = "male"
	GenderFemale  Gender = "female"
	GenderUnknown Gender = "unknown"
)

// IsValid checks if the gender value is valid.
func (g Gender) IsValid() bool {
	switch g {
	case GenderMale, GenderFemale, GenderUnknown, "":
		return true
	default:
		return false
	}
}

// RelationType represents the type of relationship between partners in a family.
type RelationType string

const (
	RelationMarriage    RelationType = "marriage"
	RelationPartnership RelationType = "partnership"
	RelationUnknown     RelationType = "unknown"
)

// IsValid checks if the relation type value is valid.
func (r RelationType) IsValid() bool {
	switch r {
	case RelationMarriage, RelationPartnership, RelationUnknown, "":
		return true
	default:
		return false
	}
}

// ChildRelationType represents the type of relationship between a child and family.
type ChildRelationType string

const (
	ChildBiological ChildRelationType = "biological"
	ChildAdopted    ChildRelationType = "adopted"
	ChildFoster     ChildRelationType = "foster"
)

// IsValid checks if the child relation type value is valid.
func (c ChildRelationType) IsValid() bool {
	switch c {
	case ChildBiological, ChildAdopted, ChildFoster:
		return true
	default:
		return false
	}
}
