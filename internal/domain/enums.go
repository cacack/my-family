// Package domain contains the core domain types for the genealogy application.
package domain

import "slices"

// Gender represents the gender of a person.
type Gender string

const (
	GenderMale    Gender = "male"
	GenderFemale  Gender = "female"
	GenderUnknown Gender = "unknown"
)

// AllGenders returns every valid Gender value.
func AllGenders() []Gender {
	return []Gender{GenderMale, GenderFemale, GenderUnknown}
}

// IsValid reports whether the value is empty (unset) or one of AllGenders.
func (g Gender) IsValid() bool {
	return g == "" || slices.Contains(AllGenders(), g)
}

// RelationType represents the type of relationship between partners in a family.
type RelationType string

const (
	RelationMarriage    RelationType = "marriage"
	RelationPartnership RelationType = "partnership"
	RelationUnknown     RelationType = "unknown"
)

// AllRelationTypes returns every valid RelationType value.
func AllRelationTypes() []RelationType {
	return []RelationType{RelationMarriage, RelationPartnership, RelationUnknown}
}

// IsValid reports whether the value is empty (unset) or one of AllRelationTypes.
func (r RelationType) IsValid() bool {
	return r == "" || slices.Contains(AllRelationTypes(), r)
}

// ChildRelationType represents the type of relationship between a child and family.
type ChildRelationType string

const (
	ChildBiological ChildRelationType = "biological"
	ChildAdopted    ChildRelationType = "adopted"
	ChildFoster     ChildRelationType = "foster"
)

// AllChildRelationTypes returns every valid ChildRelationType value.
func AllChildRelationTypes() []ChildRelationType {
	return []ChildRelationType{ChildBiological, ChildAdopted, ChildFoster}
}

// IsValid reports whether the value is one of AllChildRelationTypes.
func (c ChildRelationType) IsValid() bool {
	return slices.Contains(AllChildRelationTypes(), c)
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

// AllSourceTypes returns every valid SourceType value.
func AllSourceTypes() []SourceType {
	return []SourceType{
		SourceBook, SourceArchive, SourceWebpage, SourceCensus, SourceVitalRecord,
		SourceChurch, SourceNewspaper, SourcePhotograph, SourceInterview,
		SourceCorrespond, SourceOther,
	}
}

// IsValid reports whether the value is empty (unset) or one of AllSourceTypes.
func (s SourceType) IsValid() bool {
	return s == "" || slices.Contains(AllSourceTypes(), s)
}

// SourceQuality represents the quality of a source per GPS standards.
type SourceQuality string

const (
	SourceOriginal   SourceQuality = "original"   // Original source (best quality)
	SourceDerivative SourceQuality = "derivative" // Derived from original
	SourceAuthored   SourceQuality = "authored"   // Authored/compiled work
)

// AllSourceQualities returns every valid SourceQuality value.
func AllSourceQualities() []SourceQuality {
	return []SourceQuality{SourceOriginal, SourceDerivative, SourceAuthored}
}

// IsValid reports whether the value is empty (unset) or one of AllSourceQualities.
func (s SourceQuality) IsValid() bool {
	return s == "" || slices.Contains(AllSourceQualities(), s)
}

// InformantType represents the type of informant per GPS standards.
type InformantType string

const (
	InformantPrimary       InformantType = "primary"       // Witnessed the event
	InformantSecondary     InformantType = "secondary"     // Heard from others
	InformantIndeterminate InformantType = "indeterminate" // Cannot be determined
)

// AllInformantTypes returns every valid InformantType value.
func AllInformantTypes() []InformantType {
	return []InformantType{InformantPrimary, InformantSecondary, InformantIndeterminate}
}

// IsValid reports whether the value is empty (unset) or one of AllInformantTypes.
func (i InformantType) IsValid() bool {
	return i == "" || slices.Contains(AllInformantTypes(), i)
}

// EvidenceType represents the type of evidence per GPS standards.
type EvidenceType string

const (
	EvidenceDirect   EvidenceType = "direct"   // Directly states the fact
	EvidenceIndirect EvidenceType = "indirect" // Implies the fact
	EvidenceNegative EvidenceType = "negative" // Absence of evidence
)

// AllEvidenceTypes returns every valid EvidenceType value.
func AllEvidenceTypes() []EvidenceType {
	return []EvidenceType{EvidenceDirect, EvidenceIndirect, EvidenceNegative}
}

// IsValid reports whether the value is empty (unset) or one of AllEvidenceTypes.
func (e EvidenceType) IsValid() bool {
	return e == "" || slices.Contains(AllEvidenceTypes(), e)
}

// MediaType represents the type of media file.
type MediaType string

const (
	MediaPhoto       MediaType = "photo"
	MediaDocument    MediaType = "document"
	MediaAudio       MediaType = "audio"
	MediaVideo       MediaType = "video"
	MediaCertificate MediaType = "certificate"
)

// AllMediaTypes returns every valid MediaType value.
func AllMediaTypes() []MediaType {
	return []MediaType{MediaPhoto, MediaDocument, MediaAudio, MediaVideo, MediaCertificate}
}

// IsValid reports whether the value is empty (unset) or one of AllMediaTypes.
func (m MediaType) IsValid() bool {
	return m == "" || slices.Contains(AllMediaTypes(), m)
}

// NameType represents the type of name for a person.
type NameType string

const (
	NameTypeBirth        NameType = "birth"        // Name at birth
	NameTypeMarried      NameType = "married"      // Married name
	NameTypeAKA          NameType = "aka"          // Also known as
	NameTypeImmigrant    NameType = "immigrant"    // Name after immigration (anglicized, etc.)
	NameTypeReligious    NameType = "religious"    // Religious name (confirmation, ordination)
	NameTypeProfessional NameType = "professional" // Professional/stage name
)

// AllNameTypes returns every valid NameType value.
func AllNameTypes() []NameType {
	return []NameType{
		NameTypeBirth, NameTypeMarried, NameTypeAKA, NameTypeImmigrant, NameTypeReligious,
		NameTypeProfessional,
	}
}

// IsValid reports whether the value is empty (unset) or one of AllNameTypes.
func (n NameType) IsValid() bool {
	return n == "" || slices.Contains(AllNameTypes(), n)
}

// ResearchStatus represents the confidence level of genealogical data per GPS standards.
type ResearchStatus string

const (
	ResearchStatusCertain  ResearchStatus = "certain"  // Confirmed with strong evidence
	ResearchStatusProbable ResearchStatus = "probable" // Likely correct, good supporting evidence
	ResearchStatusPossible ResearchStatus = "possible" // Speculative, limited evidence
	ResearchStatusUnknown  ResearchStatus = "unknown"  // Not yet assessed (default)
)

// String returns the string representation of the research status.
func (r ResearchStatus) String() string {
	return string(r)
}

// AllResearchStatuses returns every valid ResearchStatus value.
func AllResearchStatuses() []ResearchStatus {
	return []ResearchStatus{ResearchStatusCertain, ResearchStatusProbable, ResearchStatusPossible, ResearchStatusUnknown}
}

// IsValid reports whether the value is empty (unset) or one of AllResearchStatuses.
func (r ResearchStatus) IsValid() bool {
	return r == "" || slices.Contains(AllResearchStatuses(), r)
}

// ParseResearchStatus parses a string into a ResearchStatus value.
// Returns ResearchStatusUnknown if the string is empty or invalid.
func ParseResearchStatus(s string) ResearchStatus {
	switch s {
	case "certain":
		return ResearchStatusCertain
	case "probable":
		return ResearchStatusProbable
	case "possible":
		return ResearchStatusPossible
	default:
		return ResearchStatusUnknown
	}
}

// ConflictStatus represents the status of an evidence conflict.
type ConflictStatus string

const (
	ConflictStatusOpen     ConflictStatus = "open"
	ConflictStatusResolved ConflictStatus = "resolved"
	ConflictStatusAccepted ConflictStatus = "accepted"
)

// IsValid reports whether the value is one of AllConflictStatuses.
func (c ConflictStatus) IsValid() bool {
	return slices.Contains(AllConflictStatuses(), c)
}

// AllConflictStatuses returns all valid ConflictStatus values.
func AllConflictStatuses() []ConflictStatus {
	return []ConflictStatus{ConflictStatusOpen, ConflictStatusResolved, ConflictStatusAccepted}
}

// ResearchOutcome represents the outcome of a research log entry.
type ResearchOutcome string

const (
	ResearchOutcomeFound        ResearchOutcome = "found"
	ResearchOutcomeNotFound     ResearchOutcome = "not_found"
	ResearchOutcomeInconclusive ResearchOutcome = "inconclusive"
)

// IsValid reports whether the value is one of AllResearchOutcomes.
func (r ResearchOutcome) IsValid() bool {
	return slices.Contains(AllResearchOutcomes(), r)
}

// AllResearchOutcomes returns all valid ResearchOutcome values.
func AllResearchOutcomes() []ResearchOutcome {
	return []ResearchOutcome{ResearchOutcomeFound, ResearchOutcomeNotFound, ResearchOutcomeInconclusive}
}

// FactType represents the type of fact that a citation can attach to.
type FactType string

const (
	// Core person facts
	FactPersonBirth  FactType = "person_birth"
	FactPersonDeath  FactType = "person_death"
	FactPersonName   FactType = "person_name"
	FactPersonGender FactType = "person_gender"

	// Individual life events (GEDCOM tags in comments)
	FactPersonBurial         FactType = "person_burial"         // BURI
	FactPersonCremation      FactType = "person_cremation"      // CREM
	FactPersonBaptism        FactType = "person_baptism"        // BAPM
	FactPersonChristening    FactType = "person_christening"    // CHR
	FactPersonEmigration     FactType = "person_emigration"     // EMIG
	FactPersonImmigration    FactType = "person_immigration"    // IMMI
	FactPersonNaturalization FactType = "person_naturalization" // NATU
	FactPersonCensus         FactType = "person_census"         // CENS
	FactPersonGenericEvent   FactType = "person_generic_event"  // EVEN

	// Individual attributes
	FactPersonOccupation FactType = "person_occupation" // OCCU
	FactPersonResidence  FactType = "person_residence"  // RESI
	FactPersonEducation  FactType = "person_education"  // EDUC
	FactPersonReligion   FactType = "person_religion"   // RELI
	FactPersonTitle      FactType = "person_title"      // TITL

	// Core family facts
	FactFamilyMarriage FactType = "family_marriage"
	FactFamilyDivorce  FactType = "family_divorce"

	// Family events (GEDCOM tags in comments)
	FactFamilyMarriageBann       FactType = "family_marriage_bann"       // MARB
	FactFamilyMarriageContract   FactType = "family_marriage_contract"   // MARC
	FactFamilyMarriageLicense    FactType = "family_marriage_license"    // MARL
	FactFamilyMarriageSettlement FactType = "family_marriage_settlement" // MARS
	FactFamilyAnnulment          FactType = "family_annulment"           // ANUL
	FactFamilyEngagement         FactType = "family_engagement"          // ENGA
)

// AllFactTypes returns every valid FactType value.
func AllFactTypes() []FactType {
	return []FactType{
		// Core person facts
		FactPersonBirth, FactPersonDeath, FactPersonName, FactPersonGender,
		// Individual life events
		FactPersonBurial, FactPersonCremation, FactPersonBaptism, FactPersonChristening,
		FactPersonEmigration, FactPersonImmigration, FactPersonNaturalization,
		FactPersonCensus, FactPersonGenericEvent,
		// Individual attributes
		FactPersonOccupation, FactPersonResidence, FactPersonEducation,
		FactPersonReligion, FactPersonTitle,
		// Core family facts
		FactFamilyMarriage, FactFamilyDivorce,
		// Family events
		FactFamilyMarriageBann, FactFamilyMarriageContract, FactFamilyMarriageLicense,
		FactFamilyMarriageSettlement, FactFamilyAnnulment, FactFamilyEngagement,
	}
}

// IsValid reports whether the value is empty (unset) or one of AllFactTypes.
func (f FactType) IsValid() bool {
	return f == "" || slices.Contains(AllFactTypes(), f)
}
