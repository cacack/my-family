package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EvidenceAnalysis aggregates citations for a fact with a researcher's conclusion.
type EvidenceAnalysis struct {
	ID             uuid.UUID      `json:"id"`
	FactType       FactType       `json:"fact_type"`
	SubjectID      uuid.UUID      `json:"subject_id"`
	CitationIDs    []uuid.UUID    `json:"citation_ids,omitempty"`
	Conclusion     string         `json:"conclusion"`
	ResearchStatus ResearchStatus `json:"research_status,omitempty"`
	Notes          string         `json:"notes,omitempty"`
	Version        int64          `json:"version"`
}

// EvidenceAnalysisValidationError represents a validation error for an EvidenceAnalysis.
type EvidenceAnalysisValidationError struct {
	Field   string
	Message string
}

func (e EvidenceAnalysisValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewEvidenceAnalysis creates a new EvidenceAnalysis with the given required fields.
func NewEvidenceAnalysis(factType FactType, subjectID uuid.UUID, conclusion string) *EvidenceAnalysis {
	return &EvidenceAnalysis{
		ID:         uuid.New(),
		FactType:   factType,
		SubjectID:  subjectID,
		Conclusion: conclusion,
		Version:    1,
	}
}

// Validate checks if the evidence analysis has valid data.
func (ea *EvidenceAnalysis) Validate() error {
	var errs []error

	if ea.SubjectID == uuid.Nil {
		errs = append(errs, EvidenceAnalysisValidationError{Field: "subject_id", Message: "cannot be empty"})
	}
	if ea.Conclusion == "" {
		errs = append(errs, EvidenceAnalysisValidationError{Field: "conclusion", Message: "cannot be empty"})
	}
	if !ea.FactType.IsValid() {
		errs = append(errs, EvidenceAnalysisValidationError{Field: "fact_type", Message: fmt.Sprintf("invalid value: %s", ea.FactType)})
	}
	if !ea.ResearchStatus.IsValid() {
		errs = append(errs, EvidenceAnalysisValidationError{Field: "research_status", Message: fmt.Sprintf("invalid value: %s", ea.ResearchStatus)})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// EvidenceConflict tracks contradictory evidence between analyses.
type EvidenceConflict struct {
	ID          uuid.UUID      `json:"id"`
	FactType    FactType       `json:"fact_type"`
	SubjectID   uuid.UUID      `json:"subject_id"`
	AnalysisIDs []uuid.UUID    `json:"analysis_ids"`
	Description string         `json:"description"`
	Resolution  string         `json:"resolution,omitempty"`
	Status      ConflictStatus `json:"status"`
	Version     int64          `json:"version"`
}

// EvidenceConflictValidationError represents a validation error for an EvidenceConflict.
type EvidenceConflictValidationError struct {
	Field   string
	Message string
}

func (e EvidenceConflictValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewEvidenceConflict creates a new EvidenceConflict with the given required fields.
func NewEvidenceConflict(factType FactType, subjectID uuid.UUID, analysisIDs []uuid.UUID, description string) *EvidenceConflict {
	return &EvidenceConflict{
		ID:          uuid.New(),
		FactType:    factType,
		SubjectID:   subjectID,
		AnalysisIDs: analysisIDs,
		Description: description,
		Status:      ConflictStatusOpen,
		Version:     1,
	}
}

// Validate checks if the evidence conflict has valid data.
func (ec *EvidenceConflict) Validate() error {
	var errs []error

	if ec.SubjectID == uuid.Nil {
		errs = append(errs, EvidenceConflictValidationError{Field: "subject_id", Message: "cannot be empty"})
	}
	if ec.Description == "" {
		errs = append(errs, EvidenceConflictValidationError{Field: "description", Message: "cannot be empty"})
	}
	if len(ec.AnalysisIDs) < 2 {
		errs = append(errs, EvidenceConflictValidationError{Field: "analysis_ids", Message: "must have at least 2 conflicting analyses"})
	}
	if !ec.FactType.IsValid() {
		errs = append(errs, EvidenceConflictValidationError{Field: "fact_type", Message: fmt.Sprintf("invalid value: %s", ec.FactType)})
	}
	if !ec.Status.IsValid() {
		errs = append(errs, EvidenceConflictValidationError{Field: "status", Message: fmt.Sprintf("invalid value: %s", ec.Status)})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ResearchLog documents research activity including negative results.
type ResearchLog struct {
	ID                uuid.UUID       `json:"id"`
	SubjectID         uuid.UUID       `json:"subject_id"`
	SubjectType       string          `json:"subject_type"` // "person" or "family"
	Repository        string          `json:"repository"`
	SearchDescription string          `json:"search_description"`
	Outcome           ResearchOutcome `json:"outcome"`
	Notes             string          `json:"notes,omitempty"`
	SearchDate        time.Time       `json:"search_date"`
	Version           int64           `json:"version"`
}

// ResearchLogValidationError represents a validation error for a ResearchLog.
type ResearchLogValidationError struct {
	Field   string
	Message string
}

func (e ResearchLogValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewResearchLog creates a new ResearchLog with the given required fields.
func NewResearchLog(subjectID uuid.UUID, subjectType, repository, searchDescription string, outcome ResearchOutcome, searchDate time.Time) *ResearchLog {
	return &ResearchLog{
		ID:                uuid.New(),
		SubjectID:         subjectID,
		SubjectType:       subjectType,
		Repository:        repository,
		SearchDescription: searchDescription,
		Outcome:           outcome,
		SearchDate:        searchDate,
		Version:           1,
	}
}

// Validate checks if the research log has valid data.
func (rl *ResearchLog) Validate() error {
	var errs []error

	if rl.SubjectID == uuid.Nil {
		errs = append(errs, ResearchLogValidationError{Field: "subject_id", Message: "cannot be empty"})
	}
	if rl.SubjectType != "person" && rl.SubjectType != "family" {
		errs = append(errs, ResearchLogValidationError{Field: "subject_type", Message: "must be 'person' or 'family'"})
	}
	if rl.Repository == "" {
		errs = append(errs, ResearchLogValidationError{Field: "repository", Message: "cannot be empty"})
	}
	if rl.SearchDescription == "" {
		errs = append(errs, ResearchLogValidationError{Field: "search_description", Message: "cannot be empty"})
	}
	if !rl.Outcome.IsValid() {
		errs = append(errs, ResearchLogValidationError{Field: "outcome", Message: fmt.Sprintf("invalid value: %s", rl.Outcome)})
	}
	if rl.SearchDate.IsZero() {
		errs = append(errs, ResearchLogValidationError{Field: "search_date", Message: "cannot be empty"})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ProofSummary is a written proof argument for non-obvious conclusions.
type ProofSummary struct {
	ID             uuid.UUID      `json:"id"`
	FactType       FactType       `json:"fact_type"`
	SubjectID      uuid.UUID      `json:"subject_id"`
	Conclusion     string         `json:"conclusion"`
	Argument       string         `json:"argument"`
	AnalysisIDs    []uuid.UUID    `json:"analysis_ids,omitempty"`
	ResearchStatus ResearchStatus `json:"research_status,omitempty"`
	Version        int64          `json:"version"`
}

// ProofSummaryValidationError represents a validation error for a ProofSummary.
type ProofSummaryValidationError struct {
	Field   string
	Message string
}

func (e ProofSummaryValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewProofSummary creates a new ProofSummary with the given required fields.
func NewProofSummary(factType FactType, subjectID uuid.UUID, conclusion, argument string) *ProofSummary {
	return &ProofSummary{
		ID:         uuid.New(),
		FactType:   factType,
		SubjectID:  subjectID,
		Conclusion: conclusion,
		Argument:   argument,
		Version:    1,
	}
}

// Validate checks if the proof summary has valid data.
func (ps *ProofSummary) Validate() error {
	var errs []error

	if ps.SubjectID == uuid.Nil {
		errs = append(errs, ProofSummaryValidationError{Field: "subject_id", Message: "cannot be empty"})
	}
	if ps.Conclusion == "" {
		errs = append(errs, ProofSummaryValidationError{Field: "conclusion", Message: "cannot be empty"})
	}
	if ps.Argument == "" {
		errs = append(errs, ProofSummaryValidationError{Field: "argument", Message: "cannot be empty"})
	}
	if !ps.FactType.IsValid() {
		errs = append(errs, ProofSummaryValidationError{Field: "fact_type", Message: fmt.Sprintf("invalid value: %s", ps.FactType)})
	}
	if !ps.ResearchStatus.IsValid() {
		errs = append(errs, ProofSummaryValidationError{Field: "research_status", Message: fmt.Sprintf("invalid value: %s", ps.ResearchStatus)})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
