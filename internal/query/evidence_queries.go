package query

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// EvidenceQueryService provides query operations for evidence analysis entities.
type EvidenceQueryService struct {
	readStore repository.ReadModelStore
}

// NewEvidenceQueryService creates a new evidence query service.
func NewEvidenceQueryService(readStore repository.ReadModelStore) *EvidenceQueryService {
	return &EvidenceQueryService{readStore: readStore}
}

// --- Query result types ---

// EvidenceAnalysis represents an evidence analysis in query results.
type EvidenceAnalysis struct {
	ID             uuid.UUID   `json:"id"`
	FactType       string      `json:"fact_type"`
	SubjectID      uuid.UUID   `json:"subject_id"`
	CitationIDs    []uuid.UUID `json:"citation_ids,omitempty"`
	Conclusion     string      `json:"conclusion"`
	ResearchStatus *string     `json:"research_status,omitempty"`
	Notes          *string     `json:"notes,omitempty"`
	Version        int64       `json:"version"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// EvidenceConflict represents an evidence conflict in query results.
type EvidenceConflict struct {
	ID          uuid.UUID   `json:"id"`
	FactType    string      `json:"fact_type"`
	SubjectID   uuid.UUID   `json:"subject_id"`
	AnalysisIDs []uuid.UUID `json:"analysis_ids"`
	Description string      `json:"description"`
	Resolution  *string     `json:"resolution,omitempty"`
	Status      string      `json:"status"`
	Version     int64       `json:"version"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// ResearchLogEntry represents a research log entry in query results.
type ResearchLogEntry struct {
	ID                uuid.UUID `json:"id"`
	SubjectID         uuid.UUID `json:"subject_id"`
	SubjectType       string    `json:"subject_type"`
	Repository        string    `json:"repository"`
	SearchDescription string    `json:"search_description"`
	Outcome           string    `json:"outcome"`
	Notes             *string   `json:"notes,omitempty"`
	SearchDate        time.Time `json:"search_date"`
	Version           int64     `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ProofSummaryResult represents a proof summary in query results.
type ProofSummaryResult struct {
	ID             uuid.UUID   `json:"id"`
	FactType       string      `json:"fact_type"`
	SubjectID      uuid.UUID   `json:"subject_id"`
	Conclusion     string      `json:"conclusion"`
	Argument       string      `json:"argument"`
	AnalysisIDs    []uuid.UUID `json:"analysis_ids,omitempty"`
	ResearchStatus *string     `json:"research_status,omitempty"`
	Version        int64       `json:"version"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// --- List input/result types ---

// EvidenceAnalysisListResult contains paginated evidence analysis results.
type EvidenceAnalysisListResult struct {
	Analyses []EvidenceAnalysis `json:"analyses"`
	Total    int                `json:"total"`
	Limit    int                `json:"limit"`
	Offset   int                `json:"offset"`
}

// EvidenceConflictListResult contains paginated evidence conflict results.
type EvidenceConflictListResult struct {
	Conflicts []EvidenceConflict `json:"conflicts"`
	Total     int                `json:"total"`
	Limit     int                `json:"limit"`
	Offset    int                `json:"offset"`
}

// ResearchLogListResult contains paginated research log results.
type ResearchLogListResult struct {
	Logs   []ResearchLogEntry `json:"logs"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// ProofSummaryListResult contains paginated proof summary results.
type ProofSummaryListResult struct {
	Summaries []ProofSummaryResult `json:"summaries"`
	Total     int                  `json:"total"`
	Limit     int                  `json:"limit"`
	Offset    int                  `json:"offset"`
}

// ListInput contains options for listing evidence entities.
type ListInput struct {
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string
}

// --- EvidenceAnalysis queries ---

// GetEvidenceAnalysis returns an evidence analysis by ID.
func (s *EvidenceQueryService) GetEvidenceAnalysis(ctx context.Context, id uuid.UUID) (*EvidenceAnalysis, error) {
	rm, err := s.readStore.GetEvidenceAnalysis(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	result := convertReadModelToEvidenceAnalysis(*rm)
	return &result, nil
}

// ListEvidenceAnalyses returns a paginated list of evidence analyses.
func (s *EvidenceQueryService) ListEvidenceAnalyses(ctx context.Context, input ListInput) (*EvidenceAnalysisListResult, error) {
	opts := normalizeListOptions(input)

	readModels, total, err := s.readStore.ListEvidenceAnalyses(ctx, opts)
	if err != nil {
		return nil, err
	}

	analyses := make([]EvidenceAnalysis, len(readModels))
	for i, rm := range readModels {
		analyses[i] = convertReadModelToEvidenceAnalysis(rm)
	}

	return &EvidenceAnalysisListResult{
		Analyses: analyses,
		Total:    total,
		Limit:    opts.Limit,
		Offset:   opts.Offset,
	}, nil
}

// GetAnalysesForFact returns all evidence analyses for a given fact type and subject.
func (s *EvidenceQueryService) GetAnalysesForFact(ctx context.Context, factType string, subjectID uuid.UUID) ([]EvidenceAnalysis, error) {
	allAnalyses, err := repository.ListAll(ctx, 100, s.readStore.ListEvidenceAnalyses)
	if err != nil {
		return nil, err
	}

	ft := domain.FactType(factType)
	var results []EvidenceAnalysis
	for _, rm := range allAnalyses {
		if rm.FactType == ft && rm.SubjectID == subjectID {
			results = append(results, convertReadModelToEvidenceAnalysis(rm))
		}
	}

	return results, nil
}

// --- EvidenceConflict queries ---

// GetEvidenceConflict returns an evidence conflict by ID.
func (s *EvidenceQueryService) GetEvidenceConflict(ctx context.Context, id uuid.UUID) (*EvidenceConflict, error) {
	rm, err := s.readStore.GetEvidenceConflict(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	result := convertReadModelToEvidenceConflict(*rm)
	return &result, nil
}

// ListEvidenceConflicts returns a paginated list of evidence conflicts.
func (s *EvidenceQueryService) ListEvidenceConflicts(ctx context.Context, input ListInput) (*EvidenceConflictListResult, error) {
	opts := normalizeListOptions(input)

	readModels, total, err := s.readStore.ListEvidenceConflicts(ctx, opts)
	if err != nil {
		return nil, err
	}

	conflicts := make([]EvidenceConflict, len(readModels))
	for i, rm := range readModels {
		conflicts[i] = convertReadModelToEvidenceConflict(rm)
	}

	return &EvidenceConflictListResult{
		Conflicts: conflicts,
		Total:     total,
		Limit:     opts.Limit,
		Offset:    opts.Offset,
	}, nil
}

// GetConflictsForSubject returns all evidence conflicts for a given subject.
func (s *EvidenceQueryService) GetConflictsForSubject(ctx context.Context, subjectID uuid.UUID) ([]EvidenceConflict, error) {
	allConflicts, err := repository.ListAll(ctx, 100, s.readStore.ListEvidenceConflicts)
	if err != nil {
		return nil, err
	}

	var results []EvidenceConflict
	for _, rm := range allConflicts {
		if rm.SubjectID == subjectID {
			results = append(results, convertReadModelToEvidenceConflict(rm))
		}
	}

	return results, nil
}

// ListUnresolvedConflicts returns all unresolved evidence conflicts.
func (s *EvidenceQueryService) ListUnresolvedConflicts(ctx context.Context) ([]EvidenceConflict, error) {
	allConflicts, err := repository.ListAll(ctx, 100, s.readStore.ListEvidenceConflicts)
	if err != nil {
		return nil, err
	}

	var results []EvidenceConflict
	for _, rm := range allConflicts {
		if rm.Status == domain.ConflictStatusOpen {
			results = append(results, convertReadModelToEvidenceConflict(rm))
		}
	}

	return results, nil
}

// --- ResearchLog queries ---

// GetResearchLog returns a research log entry by ID.
func (s *EvidenceQueryService) GetResearchLog(ctx context.Context, id uuid.UUID) (*ResearchLogEntry, error) {
	rm, err := s.readStore.GetResearchLog(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	result := convertReadModelToResearchLog(*rm)
	return &result, nil
}

// ListResearchLogs returns a paginated list of research log entries.
func (s *EvidenceQueryService) ListResearchLogs(ctx context.Context, input ListInput) (*ResearchLogListResult, error) {
	opts := normalizeListOptions(input)

	readModels, total, err := s.readStore.ListResearchLogs(ctx, opts)
	if err != nil {
		return nil, err
	}

	logs := make([]ResearchLogEntry, len(readModels))
	for i, rm := range readModels {
		logs[i] = convertReadModelToResearchLog(rm)
	}

	return &ResearchLogListResult{
		Logs:   logs,
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}

// GetResearchLogsForSubject returns all research log entries for a given subject.
func (s *EvidenceQueryService) GetResearchLogsForSubject(ctx context.Context, subjectID uuid.UUID) ([]ResearchLogEntry, error) {
	allLogs, err := repository.ListAll(ctx, 100, s.readStore.ListResearchLogs)
	if err != nil {
		return nil, err
	}

	var results []ResearchLogEntry
	for _, rm := range allLogs {
		if rm.SubjectID == subjectID {
			results = append(results, convertReadModelToResearchLog(rm))
		}
	}

	return results, nil
}

// --- ProofSummary queries ---

// GetProofSummary returns a proof summary by ID.
func (s *EvidenceQueryService) GetProofSummary(ctx context.Context, id uuid.UUID) (*ProofSummaryResult, error) {
	rm, err := s.readStore.GetProofSummary(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	result := convertReadModelToProofSummary(*rm)
	return &result, nil
}

// ListProofSummaries returns a paginated list of proof summaries.
func (s *EvidenceQueryService) ListProofSummaries(ctx context.Context, input ListInput) (*ProofSummaryListResult, error) {
	opts := normalizeListOptions(input)

	readModels, total, err := s.readStore.ListProofSummaries(ctx, opts)
	if err != nil {
		return nil, err
	}

	summaries := make([]ProofSummaryResult, len(readModels))
	for i, rm := range readModels {
		summaries[i] = convertReadModelToProofSummary(rm)
	}

	return &ProofSummaryListResult{
		Summaries: summaries,
		Total:     total,
		Limit:     opts.Limit,
		Offset:    opts.Offset,
	}, nil
}

// GetProofSummaryForFact returns the proof summary for a specific fact type and subject.
func (s *EvidenceQueryService) GetProofSummaryForFact(ctx context.Context, factType string, subjectID uuid.UUID) (*ProofSummaryResult, error) {
	allSummaries, err := repository.ListAll(ctx, 100, s.readStore.ListProofSummaries)
	if err != nil {
		return nil, err
	}

	ft := domain.FactType(factType)
	for _, rm := range allSummaries {
		if rm.FactType == ft && rm.SubjectID == subjectID {
			result := convertReadModelToProofSummary(rm)
			return &result, nil
		}
	}

	return nil, ErrNotFound
}

// --- Helper functions ---

func normalizeListOptions(input ListInput) repository.ListOptions {
	opts := repository.ListOptions{
		Limit:  input.Limit,
		Offset: input.Offset,
		Sort:   input.SortBy,
		Order:  input.SortOrder,
	}

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if opts.Sort == "" {
		opts.Sort = "created_at"
	}
	if opts.Order == "" {
		opts.Order = "desc"
	}

	return opts
}

func convertReadModelToEvidenceAnalysis(rm repository.EvidenceAnalysisReadModel) EvidenceAnalysis {
	ea := EvidenceAnalysis{
		ID:         rm.ID,
		FactType:   string(rm.FactType),
		SubjectID:  rm.SubjectID,
		Conclusion: rm.Conclusion,
		Version:    rm.Version,
		CreatedAt:  rm.CreatedAt,
		UpdatedAt:  rm.UpdatedAt,
	}

	if rm.ResearchStatus != "" {
		rs := string(rm.ResearchStatus)
		ea.ResearchStatus = &rs
	}
	if rm.Notes != "" {
		ea.Notes = &rm.Notes
	}
	if rm.CitationIDsJSON != "" {
		var ids []uuid.UUID
		if err := json.Unmarshal([]byte(rm.CitationIDsJSON), &ids); err == nil {
			ea.CitationIDs = ids
		}
	}

	return ea
}

func convertReadModelToEvidenceConflict(rm repository.EvidenceConflictReadModel) EvidenceConflict {
	ec := EvidenceConflict{
		ID:          rm.ID,
		FactType:    string(rm.FactType),
		SubjectID:   rm.SubjectID,
		Description: rm.Description,
		Status:      string(rm.Status),
		Version:     rm.Version,
		CreatedAt:   rm.CreatedAt,
		UpdatedAt:   rm.UpdatedAt,
	}

	if rm.Resolution != "" {
		ec.Resolution = &rm.Resolution
	}
	if rm.AnalysisIDsJSON != "" {
		var ids []uuid.UUID
		if err := json.Unmarshal([]byte(rm.AnalysisIDsJSON), &ids); err == nil {
			ec.AnalysisIDs = ids
		}
	}

	return ec
}

func convertReadModelToResearchLog(rm repository.ResearchLogReadModel) ResearchLogEntry {
	rl := ResearchLogEntry{
		ID:                rm.ID,
		SubjectID:         rm.SubjectID,
		SubjectType:       rm.SubjectType,
		Repository:        rm.Repository,
		SearchDescription: rm.SearchDescription,
		Outcome:           string(rm.Outcome),
		SearchDate:        rm.SearchDate,
		Version:           rm.Version,
		CreatedAt:         rm.CreatedAt,
		UpdatedAt:         rm.UpdatedAt,
	}

	if rm.Notes != "" {
		rl.Notes = &rm.Notes
	}

	return rl
}

func convertReadModelToProofSummary(rm repository.ProofSummaryReadModel) ProofSummaryResult {
	ps := ProofSummaryResult{
		ID:         rm.ID,
		FactType:   string(rm.FactType),
		SubjectID:  rm.SubjectID,
		Conclusion: rm.Conclusion,
		Argument:   rm.Argument,
		Version:    rm.Version,
		CreatedAt:  rm.CreatedAt,
		UpdatedAt:  rm.UpdatedAt,
	}

	if rm.ResearchStatus != "" {
		rs := string(rm.ResearchStatus)
		ps.ResearchStatus = &rs
	}
	if rm.AnalysisIDsJSON != "" {
		var ids []uuid.UUID
		if err := json.Unmarshal([]byte(rm.AnalysisIDsJSON), &ids); err == nil {
			ps.AnalysisIDs = ids
		}
	}

	return ps
}
