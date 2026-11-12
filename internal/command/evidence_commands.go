package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	FactID         *uuid.UUID // the analyzed life event or attribute, if any (#1304)
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
	analysis.FactID = input.FactID

	// Validate
	if err := analysis.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if analysis.FactID != nil {
		if err := h.checkFactInstance(ctx, *analysis.FactID, analysis.FactType, analysis.SubjectID); err != nil {
			return nil, err
		}
	}

	// Create event
	event := domain.NewEvidenceAnalysisCreated(analysis)

	// Execute command (append + project)
	version, err := h.execute(ctx, analysis.ID.String(), "EvidenceAnalysis", []domain.Event{event}, -1)
	if err != nil {
		return nil, fmt.Errorf("executing create evidence analysis command: %w", err)
	}

	result := &CreateEvidenceAnalysisResult{
		ID:      analysis.ID,
		Version: version,
	}

	// Auto-detect conflicts: query existing analyses for same FactType + SubjectID.
	// Non-critical: the analysis is already committed, so a detection failure is
	// logged rather than failing the create.
	conflictID, err := h.detectEvidenceConflicts(ctx, analysis)
	if err != nil {
		slog.Warn("evidence conflict detection failed after creating an analysis",
			"analysis_id", analysis.ID, "branch_id", h.branchID, "error", err)
	} else if conflictID != nil {
		result.ConflictID = conflictID
	}

	return result, nil
}

// UpdateEvidenceAnalysisInput contains the data for updating an evidence analysis.
type UpdateEvidenceAnalysisInput struct {
	ID             uuid.UUID
	FactType       *string          // required: cannot be cleared
	SubjectID      *uuid.UUID       // required: cannot be cleared
	FactID         Patch[uuid.UUID] // omitted means no change, unless FactType or SubjectID changes (#1304)
	CitationIDs    Patch[[]uuid.UUID]
	Conclusion     *string // required: cannot be cleared
	ResearchStatus Patch[string]
	Notes          Patch[string]
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
	current, err := h.readStore.GetEvidenceAnalysis(ctx, h.branchID, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting evidence analysis: %w", err)
	}
	if current == nil {
		return nil, ErrEvidenceAnalysisNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. A field equal to its current value is not a change
	// (the edit form sends every field), so only differing fields are recorded.
	changes := make(map[string]any)

	// Build a test entity for validation
	testAnalysis := &domain.EvidenceAnalysis{
		ID:             current.ID,
		FactType:       current.FactType,
		SubjectID:      current.SubjectID,
		FactID:         current.FactID,
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

	recordChange(changes, "fact_type", SetPtr(input.FactType), &testAnalysis.FactType)
	if input.SubjectID != nil && *input.SubjectID != current.SubjectID {
		testAnalysis.SubjectID = *input.SubjectID
		changes["subject_id"] = input.SubjectID.String()
	}
	recordListChange(changes, "citation_ids", input.CitationIDs, testAnalysis.CitationIDs)
	if _, ok := changes["citation_ids"]; ok {
		testAnalysis.CitationIDs, _ = input.CitationIDs.Change()
	}
	recordChange(changes, "conclusion", SetPtr(input.Conclusion), &testAnalysis.Conclusion)
	recordChange(changes, "research_status", input.ResearchStatus, &testAnalysis.ResearchStatus)
	recordChange(changes, "notes", input.Notes, &testAnalysis.Notes)
	testAnalysis.FactID = changeFactID(changes, current.FactID, input.FactID)

	// No changes?
	if len(changes) == 0 {
		return &UpdateEvidenceAnalysisResult{Version: current.Version}, nil
	}

	// Validate updated entity
	if err := testAnalysis.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if factLinkChanged(changes, testAnalysis.FactID) {
		if err := h.checkFactInstance(ctx, *testAnalysis.FactID, testAnalysis.FactType, testAnalysis.SubjectID); err != nil {
			return nil, err
		}
	}

	// Create event
	event := domain.NewEvidenceAnalysisUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "EvidenceAnalysis", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, fmt.Errorf("executing update evidence analysis command: %w", err)
	}

	result := &UpdateEvidenceAnalysisResult{Version: version}

	// Auto-detect conflicts after update (non-critical, as on create).
	conflictID, err := h.detectEvidenceConflicts(ctx, testAnalysis)
	if err != nil {
		slog.Warn("evidence conflict detection failed after updating an analysis",
			"analysis_id", testAnalysis.ID, "branch_id", h.branchID, "error", err)
	} else if conflictID != nil {
		result.ConflictID = conflictID
	}

	return result, nil
}

// DeleteEvidenceAnalysis deletes an evidence analysis record.
func (h *Handler) DeleteEvidenceAnalysis(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	current, err := h.readStore.GetEvidenceAnalysis(ctx, h.branchID, id)
	if err != nil {
		return fmt.Errorf("getting evidence analysis: %w", err)
	}
	if current == nil {
		return ErrEvidenceAnalysisNotFound
	}
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	event := domain.NewEvidenceAnalysisDeleted(id, reason)
	_, err = h.execute(ctx, id.String(), "EvidenceAnalysis", []domain.Event{event}, version)
	if err != nil {
		return fmt.Errorf("executing delete evidence analysis command: %w", err)
	}
	return nil
}

// --- EvidenceConflict management ---

// ResolveEvidenceConflictResult contains the result of resolving an evidence conflict.
type ResolveEvidenceConflictResult struct {
	Version int64
}

// ResolveEvidenceConflict resolves an evidence conflict.
func (h *Handler) ResolveEvidenceConflict(ctx context.Context, id uuid.UUID, resolution string, version int64) (*ResolveEvidenceConflictResult, error) {
	current, err := h.readStore.GetEvidenceConflict(ctx, h.branchID, id)
	if err != nil {
		return nil, fmt.Errorf("getting evidence conflict: %w", err)
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
		return nil, fmt.Errorf("executing resolve evidence conflict command: %w", err)
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
		return nil, fmt.Errorf("executing create research log command: %w", err)
	}

	return &CreateResearchLogResult{ID: log.ID, Version: version}, nil
}

// UpdateResearchLogInput contains the data for updating a research log entry.
type UpdateResearchLogInput struct {
	ID                uuid.UUID
	SubjectID         *uuid.UUID // required: cannot be cleared
	SubjectType       *string    // required: cannot be cleared
	Repository        *string    // required: cannot be cleared
	SearchDescription *string    // required: cannot be cleared
	Outcome           *string    // required: cannot be cleared
	Notes             Patch[string]
	SearchDate        *time.Time // required: cannot be cleared
	Version           int64
}

// UpdateResearchLogResult contains the result of updating a research log entry.
type UpdateResearchLogResult struct {
	Version int64
}

// UpdateResearchLog updates an existing research log entry.
func (h *Handler) UpdateResearchLog(ctx context.Context, input UpdateResearchLogInput) (*UpdateResearchLogResult, error) {
	current, err := h.readStore.GetResearchLog(ctx, h.branchID, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting research log: %w", err)
	}
	if current == nil {
		return nil, ErrResearchLogNotFound
	}
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. A field equal to its current value is not a change
	// (the edit form sends every field), so only differing fields are recorded.
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

	if input.SubjectID != nil && *input.SubjectID != current.SubjectID {
		testLog.SubjectID = *input.SubjectID
		changes["subject_id"] = input.SubjectID.String()
	}
	recordChange(changes, "subject_type", SetPtr(input.SubjectType), &testLog.SubjectType)
	recordChange(changes, "repository", SetPtr(input.Repository), &testLog.Repository)
	recordChange(changes, "search_description", SetPtr(input.SearchDescription), &testLog.SearchDescription)
	recordChange(changes, "outcome", SetPtr(input.Outcome), &testLog.Outcome)
	recordChange(changes, "notes", input.Notes, &testLog.Notes)
	if input.SearchDate != nil && !input.SearchDate.Equal(current.SearchDate) {
		testLog.SearchDate = *input.SearchDate
		// Stored as the RFC 3339 string the projection parses: a time.Time value
		// would only survive a JSON round trip, so the synchronous projection of
		// the live event would silently drop the change.
		changes["search_date"] = input.SearchDate.Format(time.RFC3339Nano)
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
		return nil, fmt.Errorf("executing update research log command: %w", err)
	}

	return &UpdateResearchLogResult{Version: version}, nil
}

// DeleteResearchLog deletes a research log entry.
func (h *Handler) DeleteResearchLog(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	current, err := h.readStore.GetResearchLog(ctx, h.branchID, id)
	if err != nil {
		return fmt.Errorf("getting research log: %w", err)
	}
	if current == nil {
		return ErrResearchLogNotFound
	}
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	event := domain.NewResearchLogDeleted(id, reason)
	_, err = h.execute(ctx, id.String(), "ResearchLog", []domain.Event{event}, version)
	if err != nil {
		return fmt.Errorf("executing delete research log command: %w", err)
	}
	return nil
}

// --- ProofSummary CRUD ---

// CreateProofSummaryInput contains the data for creating a new proof summary.
type CreateProofSummaryInput struct {
	FactType       string
	SubjectID      uuid.UUID
	FactID         *uuid.UUID // the proved life event or attribute, if any (#1304)
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
	summary.FactID = input.FactID

	if err := summary.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if summary.FactID != nil {
		if err := h.checkFactInstance(ctx, *summary.FactID, summary.FactType, summary.SubjectID); err != nil {
			return nil, err
		}
	}

	event := domain.NewProofSummaryCreated(summary)
	version, err := h.execute(ctx, summary.ID.String(), "ProofSummary", []domain.Event{event}, -1)
	if err != nil {
		return nil, fmt.Errorf("executing create proof summary command: %w", err)
	}

	return &CreateProofSummaryResult{ID: summary.ID, Version: version}, nil
}

// UpdateProofSummaryInput contains the data for updating a proof summary.
type UpdateProofSummaryInput struct {
	ID             uuid.UUID
	FactType       *string          // required: cannot be cleared
	SubjectID      *uuid.UUID       // required: cannot be cleared
	FactID         Patch[uuid.UUID] // omitted means no change, unless FactType or SubjectID changes (#1304)
	Conclusion     *string          // required: cannot be cleared
	Argument       *string          // required: cannot be cleared
	AnalysisIDs    Patch[[]uuid.UUID]
	ResearchStatus Patch[string]
	Version        int64
}

// UpdateProofSummaryResult contains the result of updating a proof summary.
type UpdateProofSummaryResult struct {
	Version int64
}

// UpdateProofSummary updates an existing proof summary.
func (h *Handler) UpdateProofSummary(ctx context.Context, input UpdateProofSummaryInput) (*UpdateProofSummaryResult, error) {
	current, err := h.readStore.GetProofSummary(ctx, h.branchID, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting proof summary: %w", err)
	}
	if current == nil {
		return nil, ErrProofSummaryNotFound
	}
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. A field equal to its current value is not a change
	// (the edit form sends every field), so only differing fields are recorded.
	changes := make(map[string]any)

	testSummary := &domain.ProofSummary{
		ID:             current.ID,
		FactType:       current.FactType,
		SubjectID:      current.SubjectID,
		FactID:         current.FactID,
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

	recordChange(changes, "fact_type", SetPtr(input.FactType), &testSummary.FactType)
	if input.SubjectID != nil && *input.SubjectID != current.SubjectID {
		testSummary.SubjectID = *input.SubjectID
		changes["subject_id"] = input.SubjectID.String()
	}
	recordChange(changes, "conclusion", SetPtr(input.Conclusion), &testSummary.Conclusion)
	recordChange(changes, "argument", SetPtr(input.Argument), &testSummary.Argument)
	recordListChange(changes, "analysis_ids", input.AnalysisIDs, testSummary.AnalysisIDs)
	if _, ok := changes["analysis_ids"]; ok {
		testSummary.AnalysisIDs, _ = input.AnalysisIDs.Change()
	}
	recordChange(changes, "research_status", input.ResearchStatus, &testSummary.ResearchStatus)
	testSummary.FactID = changeFactID(changes, current.FactID, input.FactID)

	if len(changes) == 0 {
		return &UpdateProofSummaryResult{Version: current.Version}, nil
	}

	if err := testSummary.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if factLinkChanged(changes, testSummary.FactID) {
		if err := h.checkFactInstance(ctx, *testSummary.FactID, testSummary.FactType, testSummary.SubjectID); err != nil {
			return nil, err
		}
	}

	event := domain.NewProofSummaryUpdated(input.ID, changes)
	version, err := h.execute(ctx, input.ID.String(), "ProofSummary", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, fmt.Errorf("executing update proof summary command: %w", err)
	}

	return &UpdateProofSummaryResult{Version: version}, nil
}

// DeleteProofSummary deletes a proof summary.
func (h *Handler) DeleteProofSummary(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	current, err := h.readStore.GetProofSummary(ctx, h.branchID, id)
	if err != nil {
		return fmt.Errorf("getting proof summary: %w", err)
	}
	if current == nil {
		return ErrProofSummaryNotFound
	}
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	event := domain.NewProofSummaryDeleted(id, reason)
	_, err = h.execute(ctx, id.String(), "ProofSummary", []domain.Event{event}, version)
	if err != nil {
		return fmt.Errorf("executing delete proof summary command: %w", err)
	}
	return nil
}

// --- Conflict auto-detection helper ---

// detectEvidenceConflicts checks for conflicting conclusions among analyses
// sharing the analysis's FactType and SubjectID and overlapping its fact
// instance (#1304): an analysis of one census is not compared with one of
// another census, but is with one of the fact as a whole. Returns the conflict
// ID if one was created.
//
// It reads and writes on the handler's branch (#760): on a branch it compares the
// analyses the branch sees, and a conflict it records lives on that branch until
// merged. This is the genealogical evidence conflict (two analyses disagree), not
// an ADR-005 merge conflict.
func (h *Handler) detectEvidenceConflicts(ctx context.Context, analysis *domain.EvidenceAnalysis) (*uuid.UUID, error) {
	analysisID, factType, subjectID := analysis.ID, analysis.FactType, analysis.SubjectID

	// Get analyses for the same fact type and subject
	analyses, err := h.readStore.GetAnalysesForFact(ctx, h.branchID, factType, subjectID)
	if err != nil {
		return nil, fmt.Errorf("getting analyses for fact: %w", err)
	}

	var conflicting []uuid.UUID
	for _, a := range analyses {
		if a.ID != analysisID && a.Conclusion != analysis.Conclusion && domain.FactInstancesOverlap(a.FactID, analysis.FactID) {
			conflicting = append(conflicting, a.ID)
		}
	}

	if len(conflicting) == 0 {
		return nil, nil
	}

	// Check for existing open conflict to avoid duplicates
	existingConflicts, err := h.readStore.GetConflictsForSubject(ctx, h.branchID, subjectID)
	if err != nil {
		return nil, fmt.Errorf("getting conflicts for subject: %w", err)
	}
	for _, c := range existingConflicts {
		if c.FactType == factType && sameFactInstance(c.FactID, analysis.FactID) && c.Status == domain.ConflictStatusOpen {
			return &c.ID, nil
		}
	}

	// Create a conflict between this analysis and the disagreeing ones
	allIDs := append([]uuid.UUID{analysisID}, conflicting...)
	conflict := domain.NewEvidenceConflict(
		factType,
		subjectID,
		allIDs,
		fmt.Sprintf("Conflicting conclusions for %s on subject %s", factType, subjectID),
	)
	conflict.FactID = analysis.FactID

	event := domain.NewEvidenceConflictDetected(conflict)
	_, err = h.execute(ctx, conflict.ID.String(), "EvidenceConflict", []domain.Event{event}, -1)
	if err != nil {
		return nil, fmt.Errorf("executing create evidence conflict command: %w", err)
	}

	return &conflict.ID, nil
}

// --- Fact instance helpers (#1304) ---

// changeFactID records in changes the fact_id an update of an evidence
// analysis or proof summary leaves it with, and returns that fact_id. A
// requested fact_id that differs from current is recorded as its string; an
// artifact moved to another fact type or subject without one no longer names
// its old life event or attribute, so its link is recorded as removed (nil),
// as is a link the update clears. Call it after the fact_type and subject_id
// changes are recorded.
func changeFactID(changes map[string]any, current *uuid.UUID, input Patch[uuid.UUID]) *uuid.UUID {
	if input.Cleared() {
		if current != nil {
			changes["fact_id"] = nil
		}
		return nil
	}
	if id, ok := input.Value(); ok {
		if !sameFactInstance(current, &id) {
			changes["fact_id"] = id.String()
		}
		return &id
	}
	_, typeChanged := changes["fact_type"]
	_, subjectChanged := changes["subject_id"]
	if current != nil && (typeChanged || subjectChanged) {
		changes["fact_id"] = nil
		return nil
	}
	return current
}

// factLinkChanged reports whether an update leaves an artifact linked to
// factID with a new fact_id, fact_type or subject_id, so the link must be
// checked again (checkFactInstance).
func factLinkChanged(changes map[string]any, factID *uuid.UUID) bool {
	if factID == nil {
		return false
	}
	for _, key := range []string{"fact_id", "fact_type", "subject_id"} {
		if _, changed := changes[key]; changed {
			return true
		}
	}
	return false
}

// sameFactInstance reports whether a and b name the same fact instance, or
// are both about the fact as a whole.
func sameFactInstance(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
