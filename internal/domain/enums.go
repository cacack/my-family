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

// SourceType represents the type of source material.
type SourceType string

const (
	SourceBook        SourceType = "book"
	SourceArchive     SourceType = "archive"
	SourceWebpage     SourceType = "webpage"
	SourceCensus      SourceType = "census"
	SourceVitalRecord SourceType = "vital_record"
	SourceChurch      SourceType = "church_record"
	SourceNewspaper   SourceType = "newspaper"
	SourcePhotograph  SourceType = "photograph"
	SourceInterview   SourceType = "interview"
	SourceCorrespond  SourceType = "correspondence"
	SourceOther       SourceType = "other"
)

// IsValid checks if the source type value is valid.
func (s SourceType) IsValid() bool {
	switch s {
	case SourceBook, SourceArchive, SourceWebpage, SourceCensus, SourceVitalRecord,
		SourceChurch, SourceNewspaper, SourcePhotograph, SourceInterview,
		SourceCorrespond, SourceOther, "":
		return true
	default:
		return false
	}
}

// SourceQuality represents the quality of a source per GPS standards.
type SourceQuality string

const (
	SourceOriginal   SourceQuality = "original"   // Original source (best quality)
	SourceDerivative SourceQuality = "derivative" // Derived from original
	SourceAuthored   SourceQuality = "authored"   // Authored/compiled work
)

// IsValid checks if the source quality value is valid.
func (s SourceQuality) IsValid() bool {
	switch s {
	case SourceOriginal, SourceDerivative, SourceAuthored, "":
		return true
	default:
		return false
	}
}

// InformantType represents the type of informant per GPS standards.
type InformantType string

const (
	InformantPrimary       InformantType = "primary"       // Witnessed the event
	InformantSecondary     InformantType = "secondary"     // Heard from others
	InformantIndeterminate InformantType = "indeterminate" // Cannot be determined
)

// IsValid checks if the informant type value is valid.
func (i InformantType) IsValid() bool {
	switch i {
	case InformantPrimary, InformantSecondary, InformantIndeterminate, "":
		return true
	default:
		return false
	}
}

// EvidenceType represents the type of evidence per GPS standards.
type EvidenceType string

const (
	EvidenceDirect   EvidenceType = "direct"   // Directly states the fact
	EvidenceIndirect EvidenceType = "indirect" // Implies the fact
	EvidenceNegative EvidenceType = "negative" // Absence of evidence
)

// IsValid checks if the evidence type value is valid.
func (e EvidenceType) IsValid() bool {
	switch e {
	case EvidenceDirect, EvidenceIndirect, EvidenceNegative, "":
		return true
	default:
		return false
	}
}

// FactType represents the type of fact that a citation can attach to.
type FactType string

const (
	FactPersonBirth    FactType = "person_birth"
	FactPersonDeath    FactType = "person_death"
	FactPersonName     FactType = "person_name"
	FactPersonGender   FactType = "person_gender"
	FactFamilyMarriage FactType = "family_marriage"
	FactFamilyDivorce  FactType = "family_divorce"
)

// IsValid checks if the fact type value is valid.
func (f FactType) IsValid() bool {
	switch f {
	case FactPersonBirth, FactPersonDeath, FactPersonName, FactPersonGender,
		FactFamilyMarriage, FactFamilyDivorce, "":
		return true
	default:
		return false
	}
}
