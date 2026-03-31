package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Common evidence command errors.
var (
	ErrEvidenceAnalysisNotFound = errors.New("evidence analysis not found")
	ErrEvidenceConflictNotFound = errors.New("evidence conflict not found")
	ErrResearchLogNotFound      = errors.New("research log not found")
	ErrProofSummaryNotFound     = errors.New("proof summary not found")
)

// --- EvidenceAnalysis CRUD ---

// CreateEvidenceAnalysisInput contains the data for creating a new evidence analysis.
type CreateEvidenceAnalysisInput struct {
	FactType       string
	SubjectID      uuid.UUID
	CitationIDs    []uuid.UUID
	Conclusion     string
	ResearchStatus string
	Notes          string
}

// CreateEvidenceAnalysisResult contains the result of creating an evidence analysis.
type CreateEvidenceAnalysisResult struct {
	ID         uuid.UUID
	Version    int64
	ConflictID *uuid.UUID // Non-nil if a conflict was auto-detected
}

// CreateEvidenceAnalysis creates a new evidence analysis record.
func (h *Handler) CreateEvidenceAnalysis(ctx context.Context, input CreateEvidenceAnalysisInput) (*CreateEvidenceAnalysisResult, error) {
	// Create domain entity
	analysis := domain.NewEvidenceAnalysis(
		domain.FactType(input.FactType),
		input.SubjectID,
		input.Conclusion,
	)

	if len(input.CitationIDs) > 0 {
		analysis.CitationIDs = input.CitationIDs
	}
	if input.ResearchStatus != "" {
		analysis.ResearchStatus = domain.ResearchStatus(input.ResearchStatus)
	}
	if input.Notes != "" {
		analysis.Notes = input.Notes
	}

	// Validate
	if err := analysis.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewEvidenceAnalysisCreated(analysis)

	// Execute command (append + project)
	version, err := h.execute(ctx, analysis.ID.String(), "EvidenceAnalysis", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	result := &CreateEvidenceAnalysisResult{
		ID:      analysis.ID,
		Version: version,
	}

	// Auto-detect conflicts: query existing analyses for same FactType + SubjectID
	conflictID, err := h.detectEvidenceConflicts(ctx, analysis.ID, analysis.FactType, analysis.SubjectID, analysis.Conclusion)
	if err != nil {
		// Non-critical: conflict detection failure should not fail the create
		_ = err
	} else if conflictID != nil {
		result.ConflictID = conflictID
	}

	return result, nil
}

// UpdateEvidenceAnalysisInput contains the data for updating an evidence analysis.
type UpdateEvidenceAnalysisInput struct {
	ID             uuid.UUID
	FactType       *string
	SubjectID      *uuid.UUID
	CitationIDs    []uuid.UUID // nil means no change
	Conclusion     *string
	ResearchStatus *string
	Notes          *string
	Version        int64 // Required for optimistic locking
}

// UpdateEvidenceAnalysisResult contains the result of updating an evidence analysis.
type UpdateEvidenceAnalysisResult struct {
	Version    int64
	ConflictID *uuid.UUID // Non-nil if a conflict was auto-detected
}

// UpdateEvidenceAnalysis updates an existing evidence analysis record.
func (h *Handler) UpdateEvidenceAnalysis(ctx context.Context, input UpdateEvidenceAnalysisInput) (*UpdateEvidenceAnalysisResult, error) {
	// Get current from read model
	current, err := h.readStore.GetEvidenceAnalysis(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrEvidenceAnalysisNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	// Build a test entity for validation
	testAnalysis := &domain.EvidenceAnalysis{
		ID:             current.ID,
		FactType:       current.FactType,
		SubjectID:      current.SubjectID,
		Conclusion:     current.Conclusion,
		ResearchStatus: current.ResearchStatus,
		Notes:          current.Notes,
	}

	// Parse existing citation IDs
	if current.CitationIDsJSON != "" {
		var ids []uuid.UUID
		if err := json.Unmarshal([]byte(current.CitationIDsJSON), &ids); err == nil {
			testAnalysis.CitationIDs = ids
		}
	}

	if input.FactType != nil {
		testAnalysis.FactType = domain.FactType(*input.FactType)
		changes["fact_type"] = *input.FactType
	}
	if input.SubjectID != nil {
		testAnalysis.SubjectID = *input.SubjectID
		changes["subject_id"] = input.SubjectID.String()
	}
	if input.CitationIDs != nil {
		testAnalysis.CitationIDs = input.CitationIDs
		changes["citation_ids"] = input.CitationIDs
	}
	if input.Conclusion != nil {
		testAnalysis.Conclusion = *input.Conclusion
		changes["conclusion"] = *input.Conclusion
	}
	if input.ResearchStatus != nil {
		testAnalysis.ResearchStatus = domain.ResearchStatus(*input.ResearchStatus)
		changes["research_status"] = *input.ResearchStatus
	}
	if input.Notes != nil {
		testAnalysis.Notes = *input.Notes
		changes["notes"] = *input.Notes
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdateEvidenceAnalysisResult{Version: current.Version}, nil
	}

	// Validate updated entity
	if err := testAnalysis.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewEvidenceAnalysisUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "EvidenceAnalysis", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	result := &UpdateEvidenceAnalysisResult{Version: version}

	// Auto-detect conflicts after update
	conflictID, err := h.detectEvidenceConflicts(ctx, testAnalysis.ID, testAnalysis.FactType, testAnalysis.SubjectID, testAnalysis.Conclusion)
	if err != nil {
		_ = err
	} else if conflictID != nil {
		result.ConflictID = conflictID
	}

	return result, nil
}

// DeleteEvidenceAnalysis deletes an evidence analysis record.
func (h *Handler) DeleteEvidenceAnalysis(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	current, err := h.readStore.GetEvidenceAnalysis(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrEvidenceAnalysisNotFound
	}
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	event := domain.NewEvidenceAnalysisDeleted(id, reason)
	_, err = h.execute(ctx, id.String(), "EvidenceAnalysis", []domain.Event{event}, version)
	return err
}

// --- EvidenceConflict management ---

// ResolveEvidenceConflictResult contains the result of resolving an evidence conflict.
type ResolveEvidenceConflictResult struct {
	Version int64
}

// ResolveEvidenceConflict resolves an evidence conflict.
func (h *Handler) ResolveEvidenceConflict(ctx context.Context, id uuid.UUID, resolution string, version int64) (*ResolveEvidenceConflictResult, error) {
	current, err := h.readStore.GetEvidenceConflict(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrEvidenceConflictNotFound
	}
	if current.Version != version {
		return nil, repository.ErrConcurrencyConflict
	}

	if resolution == "" {
		return nil, fmt.Errorf("%w: resolution is required", ErrInvalidInput)
	}

	event := domain.NewEvidenceConflictResolved(id, resolution, domain.ConflictStatusResolved)

	newVersion, err := h.execute(ctx, id.String(), "EvidenceConflict", []domain.Event{event}, version)
	if err != nil {
		return nil, err
	}

	return &ResolveEvidenceConflictResult{Version: newVersion}, nil
}

// --- ResearchLog CRUD ---

// CreateResearchLogInput contains the data for creating a new research log entry.
type CreateResearchLogInput struct {
	SubjectID         uuid.UUID
	SubjectType       string
	Repository        string
	SearchDescription string
	Outcome           string
	Notes             string
	SearchDate        time.Time
}

// CreateResearchLogResult contains the result of creating a research log entry.
type CreateResearchLogResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateResearchLog creates a new research log entry.
func (h *Handler) CreateResearchLog(ctx context.Context, input CreateResearchLogInput) (*CreateResearchLogResult, error) {
	log := domain.NewResearchLog(
		input.SubjectID,
		input.SubjectType,
		input.Repository,
		input.SearchDescription,
		domain.ResearchOutcome(input.Outcome),
		input.SearchDate,
	)

	if input.Notes != "" {
		log.Notes = input.Notes
	}

	if err := log.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	event := domain.NewResearchLogCreated(log)
	version, err := h.execute(ctx, log.ID.String(), "ResearchLog", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &CreateResearchLogResult{ID: log.ID, Version: version}, nil
}

// UpdateResearchLogInput contains the data for updating a research log entry.
type UpdateResearchLogInput struct {
	ID                uuid.UUID
	SubjectID         *uuid.UUID
	SubjectType       *string
	Repository        *string
	SearchDescription *string
	Outcome           *string
	Notes             *string
	SearchDate        *time.Time
	Version           int64
}

// UpdateResearchLogResult contains the result of updating a research log entry.
type UpdateResearchLogResult struct {
	Version int64
}

// UpdateResearchLog updates an existing research log entry.
func (h *Handler) UpdateResearchLog(ctx context.Context, input UpdateResearchLogInput) (*UpdateResearchLogResult, error) {
	current, err := h.readStore.GetResearchLog(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrResearchLogNotFound
	}
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	changes := make(map[string]any)

	testLog := &domain.ResearchLog{
		ID:                current.ID,
		SubjectID:         current.SubjectID,
		SubjectType:       current.SubjectType,
		Repository:        current.Repository,
		SearchDescription: current.SearchDescription,
		Outcome:           current.Outcome,
		Notes:             current.Notes,
		SearchDate:        current.SearchDate,
	}

	if input.SubjectID != nil {
		testLog.SubjectID = *input.SubjectID
		changes["subject_id"] = input.SubjectID.String()
	}
	if input.SubjectType != nil {
		testLog.SubjectType = *input.SubjectType
		changes["subject_type"] = *input.SubjectType
	}
	if input.Repository != nil {
		testLog.Repository = *input.Repository
		changes["repository"] = *input.Repository
	}
	if input.SearchDescription != nil {
		testLog.SearchDescription = *input.SearchDescription
		changes["search_description"] = *input.SearchDescription
	}
	if input.Outcome != nil {
		testLog.Outcome = domain.ResearchOutcome(*input.Outcome)
		changes["outcome"] = *input.Outcome
	}
	if input.Notes != nil {
		testLog.Notes = *input.Notes
		changes["notes"] = *input.Notes
	}
	if input.SearchDate != nil {
		testLog.SearchDate = *input.SearchDate
		changes["search_date"] = *input.SearchDate
	}

	if len(changes) == 0 {
		return &UpdateResearchLogResult{Version: current.Version}, nil
	}

	if err := testLog.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	event := domain.NewResearchLogUpdated(input.ID, changes)
	version, err := h.execute(ctx, input.ID.String(), "ResearchLog", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateResearchLogResult{Version: version}, nil
}

// DeleteResearchLog deletes a research log entry.
func (h *Handler) DeleteResearchLog(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	current, err := h.readStore.GetResearchLog(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrResearchLogNotFound
	}
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	event := domain.NewResearchLogDeleted(id, reason)
	_, err = h.execute(ctx, id.String(), "ResearchLog", []domain.Event{event}, version)
	return err
}

// --- ProofSummary CRUD ---

// CreateProofSummaryInput contains the data for creating a new proof summary.
type CreateProofSummaryInput struct {
	FactType       string
	SubjectID      uuid.UUID
	Conclusion     string
	Argument       string
	AnalysisIDs    []uuid.UUID
	ResearchStatus string
}

// CreateProofSummaryResult contains the result of creating a proof summary.
type CreateProofSummaryResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateProofSummary creates a new proof summary.
func (h *Handler) CreateProofSummary(ctx context.Context, input CreateProofSummaryInput) (*CreateProofSummaryResult, error) {
	summary := domain.NewProofSummary(
		domain.FactType(input.FactType),
		input.SubjectID,
		input.Conclusion,
		input.Argument,
	)

	if len(input.AnalysisIDs) > 0 {
		summary.AnalysisIDs = input.AnalysisIDs
	}
	if input.ResearchStatus != "" {
		summary.ResearchStatus = domain.ResearchStatus(input.ResearchStatus)
	}

	if err := summary.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	event := domain.NewProofSummaryCreated(summary)
	version, err := h.execute(ctx, summary.ID.String(), "ProofSummary", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &CreateProofSummaryResult{ID: summary.ID, Version: version}, nil
}

// UpdateProofSummaryInput contains the data for updating a proof summary.
type UpdateProofSummaryInput struct {
	ID             uuid.UUID
	FactType       *string
	SubjectID      *uuid.UUID
	Conclusion     *string
	Argument       *string
	AnalysisIDs    []uuid.UUID // nil means no change
	ResearchStatus *string
	Version        int64
}

// UpdateProofSummaryResult contains the result of updating a proof summary.
type UpdateProofSummaryResult struct {
	Version int64
}

// UpdateProofSummary updates an existing proof summary.
func (h *Handler) UpdateProofSummary(ctx context.Context, input UpdateProofSummaryInput) (*UpdateProofSummaryResult, error) {
	current, err := h.readStore.GetProofSummary(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrProofSummaryNotFound
	}
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	changes := make(map[string]any)

	testSummary := &domain.ProofSummary{
		ID:             current.ID,
		FactType:       current.FactType,
		SubjectID:      current.SubjectID,
		Conclusion:     current.Conclusion,
		Argument:       current.Argument,
		ResearchStatus: current.ResearchStatus,
	}

	// Parse existing analysis IDs
	if current.AnalysisIDsJSON != "" {
		var ids []uuid.UUID
		if err := json.Unmarshal([]byte(current.AnalysisIDsJSON), &ids); err == nil {
			testSummary.AnalysisIDs = ids
		}
	}

	if input.FactType != nil {
		testSummary.FactType = domain.FactType(*input.FactType)
		changes["fact_type"] = *input.FactType
	}
	if input.SubjectID != nil {
		testSummary.SubjectID = *input.SubjectID
		changes["subject_id"] = input.SubjectID.String()
	}
	if input.Conclusion != nil {
		testSummary.Conclusion = *input.Conclusion
		changes["conclusion"] = *input.Conclusion
	}
	if input.Argument != nil {
		testSummary.Argument = *input.Argument
		changes["argument"] = *input.Argument
	}
	if input.AnalysisIDs != nil {
		testSummary.AnalysisIDs = input.AnalysisIDs
		changes["analysis_ids"] = input.AnalysisIDs
	}
	if input.ResearchStatus != nil {
		testSummary.ResearchStatus = domain.ResearchStatus(*input.ResearchStatus)
		changes["research_status"] = *input.ResearchStatus
	}

	if len(changes) == 0 {
		return &UpdateProofSummaryResult{Version: current.Version}, nil
	}

	if err := testSummary.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	event := domain.NewProofSummaryUpdated(input.ID, changes)
	version, err := h.execute(ctx, input.ID.String(), "ProofSummary", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateProofSummaryResult{Version: version}, nil
}

// DeleteProofSummary deletes a proof summary.
func (h *Handler) DeleteProofSummary(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	current, err := h.readStore.GetProofSummary(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrProofSummaryNotFound
	}
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	event := domain.NewProofSummaryDeleted(id, reason)
	_, err = h.execute(ctx, id.String(), "ProofSummary", []domain.Event{event}, version)
	return err
}

// --- Conflict auto-detection helper ---

// detectEvidenceConflicts checks for conflicting conclusions among analyses
// sharing the same FactType and SubjectID. Returns the conflict ID if one was created.
func (h *Handler) detectEvidenceConflicts(ctx context.Context, analysisID uuid.UUID, factType domain.FactType, subjectID uuid.UUID, conclusion string) (*uuid.UUID, error) {
	// Get all analyses to find ones with same factType+subjectID
	allAnalyses, err := repository.ListAll(ctx, 100, h.readStore.ListEvidenceAnalyses)
	if err != nil {
		return nil, err
	}

	var conflicting []uuid.UUID
	for _, a := range allAnalyses {
		if a.FactType == factType && a.SubjectID == subjectID && a.ID != analysisID {
			if a.Conclusion != conclusion {
				conflicting = append(conflicting, a.ID)
			}
		}
	}

	if len(conflicting) == 0 {
		return nil, nil
	}

	// Create a conflict between this analysis and the disagreeing ones
	allIDs := append([]uuid.UUID{analysisID}, conflicting...)
	conflict := domain.NewEvidenceConflict(
		factType,
		subjectID,
		allIDs,
		fmt.Sprintf("Conflicting conclusions for %s on subject %s", factType, subjectID),
	)

	event := domain.NewEvidenceConflictDetected(conflict)
	_, err = h.execute(ctx, conflict.ID.String(), "EvidenceConflict", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &conflict.ID, nil
}
