package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Source represents a genealogical source per GPS standards.
type Source struct {
	ID             uuid.UUID  `json:"id"`
	SourceType     SourceType `json:"source_type"`
	Title          string     `json:"title"`
	Author         string     `json:"author,omitempty"`
	Publisher      string     `json:"publisher,omitempty"`
	PublishDate    *GenDate   `json:"publish_date,omitempty"`
	URL            string     `json:"url,omitempty"`
	RepositoryName string     `json:"repository_name,omitempty"`
	CollectionName string     `json:"collection_name,omitempty"`
	CallNumber     string     `json:"call_number,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	GedcomXref     string     `json:"gedcom_xref,omitempty"` // Original GEDCOM @XREF@ for round-trip
	Version        int64      `json:"version"`               // Optimistic locking version
}

// SourceValidationError represents a validation error for a Source.
type SourceValidationError struct {
	Field   string
	Message string
}

func (e SourceValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewSource creates a new Source with the given required fields.
func NewSource(title string, sourceType SourceType) *Source {
	return &Source{
		ID:         uuid.New(),
		Title:      title,
		SourceType: sourceType,
		Version:    1,
	}
}

// Validate checks if the source has valid data.
func (s *Source) Validate() error {
	var errs []error

	// Required fields
	if s.Title == "" {
		errs = append(errs, SourceValidationError{Field: "title", Message: "cannot be empty"})
	}

	// SourceType validation
	if !s.SourceType.IsValid() {
		errs = append(errs, SourceValidationError{Field: "source_type", Message: fmt.Sprintf("invalid value: %s", s.SourceType)})
	}

	// Date validation
	if s.PublishDate != nil {
		if err := s.PublishDate.Validate(); err != nil {
			errs = append(errs, SourceValidationError{Field: "publish_date", Message: err.Error()})
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Citation represents a citation linking a source to a specific fact.
type Citation struct {
	ID            uuid.UUID     `json:"id"`
	SourceID      uuid.UUID     `json:"source_id"`
	FactType      FactType      `json:"fact_type"`
	FactOwnerID   uuid.UUID     `json:"fact_owner_id"`    // Person or Family ID
	Page          string        `json:"page,omitempty"`   // Page/location within source
	Volume        string        `json:"volume,omitempty"` // Volume/issue/series
	SourceQuality SourceQuality `json:"source_quality,omitempty"`
	InformantType InformantType `json:"informant_type,omitempty"`
	EvidenceType  EvidenceType  `json:"evidence_type,omitempty"`
	QuotedText    string        `json:"quoted_text,omitempty"` // Direct quote from source
	Analysis      string        `json:"analysis,omitempty"`    // Researcher's analysis
	TemplateID    string        `json:"template_id,omitempty"` // For future Evidence Explained templates
	GedcomXref    string        `json:"gedcom_xref,omitempty"` // Original GEDCOM @XREF@ for round-trip
	Version       int64         `json:"version"`               // Optimistic locking version
}

// CitationValidationError represents a validation error for a Citation.
type CitationValidationError struct {
	Field   string
	Message string
}

func (e CitationValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewCitation creates a new Citation with the given required fields.
func NewCitation(sourceID uuid.UUID, factType FactType, factOwnerID uuid.UUID) *Citation {
	return &Citation{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    factType,
		FactOwnerID: factOwnerID,
		Version:     1,
	}
}

// Validate checks if the citation has valid data.
func (c *Citation) Validate() error {
	var errs []error

	// Required fields
	if c.SourceID == uuid.Nil {
		errs = append(errs, CitationValidationError{Field: "source_id", Message: "cannot be empty"})
	}
	if c.FactOwnerID == uuid.Nil {
		errs = append(errs, CitationValidationError{Field: "fact_owner_id", Message: "cannot be empty"})
	}

	// Enum validation
	if !c.FactType.IsValid() {
		errs = append(errs, CitationValidationError{Field: "fact_type", Message: fmt.Sprintf("invalid value: %s", c.FactType)})
	}
	if !c.SourceQuality.IsValid() {
		errs = append(errs, CitationValidationError{Field: "source_quality", Message: fmt.Sprintf("invalid value: %s", c.SourceQuality)})
	}
	if !c.InformantType.IsValid() {
		errs = append(errs, CitationValidationError{Field: "informant_type", Message: fmt.Sprintf("invalid value: %s", c.InformantType)})
	}
	if !c.EvidenceType.IsValid() {
		errs = append(errs, CitationValidationError{Field: "evidence_type", Message: fmt.Sprintf("invalid value: %s", c.EvidenceType)})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
