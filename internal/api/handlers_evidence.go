package api

import (
	"context"
	"errors"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// ============================================================================
// Evidence Analysis endpoints
// ============================================================================

// ListEvidenceAnalyses implements StrictServerInterface.
func (ss *StrictServer) ListEvidenceAnalyses(ctx context.Context, request ListEvidenceAnalysesRequestObject) (ListEvidenceAnalysesResponseObject, error) {
	limit := 20
	offset := 0
	sort := "created_at"
	order := "desc"

	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}
	if request.Params.Sort != nil {
		sort = string(*request.Params.Sort)
	}
	if request.Params.Order != nil {
		order = string(*request.Params.Order)
	}

	result, err := ss.server.evidenceService.ListEvidenceAnalyses(ctx, query.ListInput{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sort,
		SortOrder: order,
	})
	if err != nil {
		return nil, err
	}

	analyses := make([]EvidenceAnalysis, len(result.Analyses))
	for i, a := range result.Analyses {
		analyses[i] = convertQueryEvidenceAnalysisToGenerated(a)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return ListEvidenceAnalyses200JSONResponse{
		Analyses: analyses,
		Total:    result.Total,
		Limit:    &limitVal,
		Offset:   &offsetVal,
	}, nil
}

// CreateEvidenceAnalysis implements StrictServerInterface.
func (ss *StrictServer) CreateEvidenceAnalysis(ctx context.Context, request CreateEvidenceAnalysisRequestObject) (CreateEvidenceAnalysisResponseObject, error) {
	input := command.CreateEvidenceAnalysisInput{
		FactType:   request.Body.FactType,
		SubjectID:  request.Body.SubjectId,
		Conclusion: request.Body.Conclusion,
	}
	if request.Body.CitationIds != nil {
		input.CitationIDs = *request.Body.CitationIds
	}
	if request.Body.ResearchStatus != nil {
		input.ResearchStatus = string(*request.Body.ResearchStatus)
	}
	if request.Body.Notes != nil {
		input.Notes = *request.Body.Notes
	}

	result, err := ss.server.commandHandler.CreateEvidenceAnalysis(ctx, input)
	if err != nil {
		if errors.Is(err, command.ErrInvalidInput) {
			return CreateEvidenceAnalysis400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_input",
				Message: err.Error(),
			}}, nil
		}
		return nil, err
	}

	// Fetch the created analysis
	analysis, err := ss.server.evidenceService.GetEvidenceAnalysis(ctx, result.ID)
	if err != nil {
		return nil, err
	}

	resp := convertQueryEvidenceAnalysisToGenerated(*analysis)
	if result.ConflictID != nil {
		resp.ConflictId = result.ConflictID
	}

	return CreateEvidenceAnalysis201JSONResponse(resp), nil
}

// GetEvidenceAnalysis implements StrictServerInterface.
func (ss *StrictServer) GetEvidenceAnalysis(ctx context.Context, request GetEvidenceAnalysisRequestObject) (GetEvidenceAnalysisResponseObject, error) {
	analysis, err := ss.server.evidenceService.GetEvidenceAnalysis(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetEvidenceAnalysis404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Evidence analysis not found",
			}}, nil
		}
		return nil, err
	}

	return GetEvidenceAnalysis200JSONResponse(convertQueryEvidenceAnalysisToGenerated(*analysis)), nil
}

// UpdateEvidenceAnalysis implements StrictServerInterface.
func (ss *StrictServer) UpdateEvidenceAnalysis(ctx context.Context, request UpdateEvidenceAnalysisRequestObject) (UpdateEvidenceAnalysisResponseObject, error) {
	input := command.UpdateEvidenceAnalysisInput{
		ID:      request.Id,
		Version: request.Body.Version,
	}
	if request.Body.FactType != nil {
		input.FactType = request.Body.FactType
	}
	if request.Body.SubjectId != nil {
		sid := *request.Body.SubjectId
		input.SubjectID = &sid
	}
	if request.Body.CitationIds != nil {
		input.CitationIDs = *request.Body.CitationIds
	}
	if request.Body.Conclusion != nil {
		input.Conclusion = request.Body.Conclusion
	}
	if request.Body.ResearchStatus != nil {
		rs := string(*request.Body.ResearchStatus)
		input.ResearchStatus = &rs
	}
	if request.Body.Notes != nil {
		input.Notes = request.Body.Notes
	}

	result, err := ss.server.commandHandler.UpdateEvidenceAnalysis(ctx, input)
	if err != nil {
		if errors.Is(err, command.ErrEvidenceAnalysisNotFound) {
			return UpdateEvidenceAnalysis404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Evidence analysis not found",
			}}, nil
		}
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return UpdateEvidenceAnalysis409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict",
			}}, nil
		}
		if errors.Is(err, command.ErrInvalidInput) {
			return UpdateEvidenceAnalysis400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_input",
				Message: err.Error(),
			}}, nil
		}
		return nil, err
	}

	// Fetch the updated analysis
	analysis, err := ss.server.evidenceService.GetEvidenceAnalysis(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	resp := convertQueryEvidenceAnalysisToGenerated(*analysis)
	if result.ConflictID != nil {
		resp.ConflictId = result.ConflictID
	}

	return UpdateEvidenceAnalysis200JSONResponse(resp), nil
}

// DeleteEvidenceAnalysis implements StrictServerInterface.
func (ss *StrictServer) DeleteEvidenceAnalysis(ctx context.Context, request DeleteEvidenceAnalysisRequestObject) (DeleteEvidenceAnalysisResponseObject, error) {
	version := int64(0)
	if request.Params.Version != nil {
		version = *request.Params.Version
	}

	err := ss.server.commandHandler.DeleteEvidenceAnalysis(ctx, request.Id, version, "")
	if err != nil {
		if errors.Is(err, command.ErrEvidenceAnalysisNotFound) {
			return DeleteEvidenceAnalysis404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Evidence analysis not found",
			}}, nil
		}
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return DeleteEvidenceAnalysis409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict",
			}}, nil
		}
		return nil, err
	}

	return DeleteEvidenceAnalysis204Response{}, nil
}

// GetAnalysesByFact implements StrictServerInterface.
func (ss *StrictServer) GetAnalysesByFact(ctx context.Context, request GetAnalysesByFactRequestObject) (GetAnalysesByFactResponseObject, error) {
	analyses, err := ss.server.evidenceService.GetAnalysesForFact(ctx, request.Params.FactType, request.Params.SubjectId)
	if err != nil {
		return nil, err
	}

	result := make([]EvidenceAnalysis, len(analyses))
	for i, a := range analyses {
		result[i] = convertQueryEvidenceAnalysisToGenerated(a)
	}

	return GetAnalysesByFact200JSONResponse(result), nil
}

// ============================================================================
// Evidence Conflict endpoints
// ============================================================================

// ListEvidenceConflicts implements StrictServerInterface.
func (ss *StrictServer) ListEvidenceConflicts(ctx context.Context, request ListEvidenceConflictsRequestObject) (ListEvidenceConflictsResponseObject, error) {
	limit := 20
	offset := 0

	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	// If status filter is provided, use specialized queries
	if request.Params.Status != nil && string(*request.Params.Status) == "open" {
		conflicts, err := ss.server.evidenceService.ListUnresolvedConflicts(ctx)
		if err != nil {
			return nil, err
		}

		genConflicts := make([]EvidenceConflict, len(conflicts))
		for i, c := range conflicts {
			genConflicts[i] = convertQueryEvidenceConflictToGenerated(c)
		}

		total := len(genConflicts)
		return ListEvidenceConflicts200JSONResponse{
			Conflicts: genConflicts,
			Total:     total,
			Limit:     &limit,
			Offset:    &offset,
		}, nil
	}

	result, err := ss.server.evidenceService.ListEvidenceConflicts(ctx, query.ListInput{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	conflicts := make([]EvidenceConflict, len(result.Conflicts))
	for i, c := range result.Conflicts {
		conflicts[i] = convertQueryEvidenceConflictToGenerated(c)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return ListEvidenceConflicts200JSONResponse{
		Conflicts: conflicts,
		Total:     result.Total,
		Limit:     &limitVal,
		Offset:    &offsetVal,
	}, nil
}

// GetEvidenceConflict implements StrictServerInterface.
func (ss *StrictServer) GetEvidenceConflict(ctx context.Context, request GetEvidenceConflictRequestObject) (GetEvidenceConflictResponseObject, error) {
	conflict, err := ss.server.evidenceService.GetEvidenceConflict(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetEvidenceConflict404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Evidence conflict not found",
			}}, nil
		}
		return nil, err
	}

	return GetEvidenceConflict200JSONResponse(convertQueryEvidenceConflictToGenerated(*conflict)), nil
}

// ResolveEvidenceConflict implements StrictServerInterface.
func (ss *StrictServer) ResolveEvidenceConflict(ctx context.Context, request ResolveEvidenceConflictRequestObject) (ResolveEvidenceConflictResponseObject, error) {
	_, err := ss.server.commandHandler.ResolveEvidenceConflict(ctx, request.Id, request.Body.Resolution, request.Body.Version)
	if err != nil {
		if errors.Is(err, command.ErrEvidenceConflictNotFound) {
			return ResolveEvidenceConflict404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Evidence conflict not found",
			}}, nil
		}
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return ResolveEvidenceConflict409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict",
			}}, nil
		}
		if errors.Is(err, command.ErrInvalidInput) {
			return ResolveEvidenceConflict400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_input",
				Message: err.Error(),
			}}, nil
		}
		return nil, err
	}

	// Fetch the resolved conflict
	conflict, err := ss.server.evidenceService.GetEvidenceConflict(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return ResolveEvidenceConflict200JSONResponse(convertQueryEvidenceConflictToGenerated(*conflict)), nil
}

// GetConflictsBySubject implements StrictServerInterface.
func (ss *StrictServer) GetConflictsBySubject(ctx context.Context, request GetConflictsBySubjectRequestObject) (GetConflictsBySubjectResponseObject, error) {
	conflicts, err := ss.server.evidenceService.GetConflictsForSubject(ctx, request.SubjectId)
	if err != nil {
		return nil, err
	}

	result := make([]EvidenceConflict, len(conflicts))
	for i, c := range conflicts {
		result[i] = convertQueryEvidenceConflictToGenerated(c)
	}

	return GetConflictsBySubject200JSONResponse(result), nil
}

// ============================================================================
// Research Log endpoints
// ============================================================================

// ListResearchLogs implements StrictServerInterface.
func (ss *StrictServer) ListResearchLogs(ctx context.Context, request ListResearchLogsRequestObject) (ListResearchLogsResponseObject, error) {
	limit := 20
	offset := 0
	sort := "created_at"
	order := "desc"

	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}
	if request.Params.Sort != nil {
		sort = string(*request.Params.Sort)
	}
	if request.Params.Order != nil {
		order = string(*request.Params.Order)
	}

	result, err := ss.server.evidenceService.ListResearchLogs(ctx, query.ListInput{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sort,
		SortOrder: order,
	})
	if err != nil {
		return nil, err
	}

	logs := make([]ResearchLog, len(result.Logs))
	for i, l := range result.Logs {
		logs[i] = convertQueryResearchLogToGenerated(l)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return ListResearchLogs200JSONResponse{
		Logs:   logs,
		Total:  result.Total,
		Limit:  &limitVal,
		Offset: &offsetVal,
	}, nil
}

// CreateResearchLog implements StrictServerInterface.
func (ss *StrictServer) CreateResearchLog(ctx context.Context, request CreateResearchLogRequestObject) (CreateResearchLogResponseObject, error) {
	input := command.CreateResearchLogInput{
		SubjectID:         request.Body.SubjectId,
		SubjectType:       request.Body.SubjectType,
		Repository:        request.Body.Repository,
		SearchDescription: request.Body.SearchDescription,
		Outcome:           string(request.Body.Outcome),
		SearchDate:        request.Body.SearchDate,
	}
	if request.Body.Notes != nil {
		input.Notes = *request.Body.Notes
	}

	result, err := ss.server.commandHandler.CreateResearchLog(ctx, input)
	if err != nil {
		if errors.Is(err, command.ErrInvalidInput) {
			return CreateResearchLog400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_input",
				Message: err.Error(),
			}}, nil
		}
		return nil, err
	}

	// Fetch the created log
	log, err := ss.server.evidenceService.GetResearchLog(ctx, result.ID)
	if err != nil {
		return nil, err
	}

	return CreateResearchLog201JSONResponse(convertQueryResearchLogToGenerated(*log)), nil
}

// GetResearchLog implements StrictServerInterface.
func (ss *StrictServer) GetResearchLog(ctx context.Context, request GetResearchLogRequestObject) (GetResearchLogResponseObject, error) {
	log, err := ss.server.evidenceService.GetResearchLog(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetResearchLog404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Research log not found",
			}}, nil
		}
		return nil, err
	}

	return GetResearchLog200JSONResponse(convertQueryResearchLogToGenerated(*log)), nil
}

// UpdateResearchLog implements StrictServerInterface.
func (ss *StrictServer) UpdateResearchLog(ctx context.Context, request UpdateResearchLogRequestObject) (UpdateResearchLogResponseObject, error) {
	input := command.UpdateResearchLogInput{
		ID:      request.Id,
		Version: request.Body.Version,
	}
	if request.Body.SubjectId != nil {
		sid := *request.Body.SubjectId
		input.SubjectID = &sid
	}
	if request.Body.SubjectType != nil {
		input.SubjectType = request.Body.SubjectType
	}
	if request.Body.Repository != nil {
		input.Repository = request.Body.Repository
	}
	if request.Body.SearchDescription != nil {
		input.SearchDescription = request.Body.SearchDescription
	}
	if request.Body.Outcome != nil {
		o := string(*request.Body.Outcome)
		input.Outcome = &o
	}
	if request.Body.Notes != nil {
		input.Notes = request.Body.Notes
	}
	if request.Body.SearchDate != nil {
		input.SearchDate = request.Body.SearchDate
	}

	_, err := ss.server.commandHandler.UpdateResearchLog(ctx, input)
	if err != nil {
		if errors.Is(err, command.ErrResearchLogNotFound) {
			return UpdateResearchLog404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Research log not found",
			}}, nil
		}
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return UpdateResearchLog409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict",
			}}, nil
		}
		if errors.Is(err, command.ErrInvalidInput) {
			return UpdateResearchLog400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_input",
				Message: err.Error(),
			}}, nil
		}
		return nil, err
	}

	// Fetch the updated log
	log, err := ss.server.evidenceService.GetResearchLog(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return UpdateResearchLog200JSONResponse(convertQueryResearchLogToGenerated(*log)), nil
}

// DeleteResearchLog implements StrictServerInterface.
func (ss *StrictServer) DeleteResearchLog(ctx context.Context, request DeleteResearchLogRequestObject) (DeleteResearchLogResponseObject, error) {
	version := int64(0)
	if request.Params.Version != nil {
		version = *request.Params.Version
	}

	err := ss.server.commandHandler.DeleteResearchLog(ctx, request.Id, version, "")
	if err != nil {
		if errors.Is(err, command.ErrResearchLogNotFound) {
			return DeleteResearchLog404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Research log not found",
			}}, nil
		}
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return DeleteResearchLog409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict",
			}}, nil
		}
		return nil, err
	}

	return DeleteResearchLog204Response{}, nil
}

// GetResearchLogsBySubject implements StrictServerInterface.
func (ss *StrictServer) GetResearchLogsBySubject(ctx context.Context, request GetResearchLogsBySubjectRequestObject) (GetResearchLogsBySubjectResponseObject, error) {
	logs, err := ss.server.evidenceService.GetResearchLogsForSubject(ctx, request.SubjectId)
	if err != nil {
		return nil, err
	}

	result := make([]ResearchLog, len(logs))
	for i, l := range logs {
		result[i] = convertQueryResearchLogToGenerated(l)
	}

	return GetResearchLogsBySubject200JSONResponse(result), nil
}

// ============================================================================
// Proof Summary endpoints
// ============================================================================

// ListProofSummaries implements StrictServerInterface.
func (ss *StrictServer) ListProofSummaries(ctx context.Context, request ListProofSummariesRequestObject) (ListProofSummariesResponseObject, error) {
	limit := 20
	offset := 0
	sort := "created_at"
	order := "desc"

	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}
	if request.Params.Sort != nil {
		sort = string(*request.Params.Sort)
	}
	if request.Params.Order != nil {
		order = string(*request.Params.Order)
	}

	result, err := ss.server.evidenceService.ListProofSummaries(ctx, query.ListInput{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sort,
		SortOrder: order,
	})
	if err != nil {
		return nil, err
	}

	summaries := make([]ProofSummary, len(result.Summaries))
	for i, s := range result.Summaries {
		summaries[i] = convertQueryProofSummaryToGenerated(s)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return ListProofSummaries200JSONResponse{
		Summaries: summaries,
		Total:     result.Total,
		Limit:     &limitVal,
		Offset:    &offsetVal,
	}, nil
}

// CreateProofSummary implements StrictServerInterface.
func (ss *StrictServer) CreateProofSummary(ctx context.Context, request CreateProofSummaryRequestObject) (CreateProofSummaryResponseObject, error) {
	input := command.CreateProofSummaryInput{
		FactType:   request.Body.FactType,
		SubjectID:  request.Body.SubjectId,
		Conclusion: request.Body.Conclusion,
		Argument:   request.Body.Argument,
	}
	if request.Body.AnalysisIds != nil {
		input.AnalysisIDs = *request.Body.AnalysisIds
	}
	if request.Body.ResearchStatus != nil {
		input.ResearchStatus = string(*request.Body.ResearchStatus)
	}

	result, err := ss.server.commandHandler.CreateProofSummary(ctx, input)
	if err != nil {
		if errors.Is(err, command.ErrInvalidInput) {
			return CreateProofSummary400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_input",
				Message: err.Error(),
			}}, nil
		}
		return nil, err
	}

	// Fetch the created summary
	summary, err := ss.server.evidenceService.GetProofSummary(ctx, result.ID)
	if err != nil {
		return nil, err
	}

	return CreateProofSummary201JSONResponse(convertQueryProofSummaryToGenerated(*summary)), nil
}

// GetProofSummary implements StrictServerInterface.
func (ss *StrictServer) GetProofSummary(ctx context.Context, request GetProofSummaryRequestObject) (GetProofSummaryResponseObject, error) {
	summary, err := ss.server.evidenceService.GetProofSummary(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetProofSummary404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Proof summary not found",
			}}, nil
		}
		return nil, err
	}

	return GetProofSummary200JSONResponse(convertQueryProofSummaryToGenerated(*summary)), nil
}

// UpdateProofSummary implements StrictServerInterface.
func (ss *StrictServer) UpdateProofSummary(ctx context.Context, request UpdateProofSummaryRequestObject) (UpdateProofSummaryResponseObject, error) {
	input := command.UpdateProofSummaryInput{
		ID:      request.Id,
		Version: request.Body.Version,
	}
	if request.Body.FactType != nil {
		input.FactType = request.Body.FactType
	}
	if request.Body.SubjectId != nil {
		sid := *request.Body.SubjectId
		input.SubjectID = &sid
	}
	if request.Body.Conclusion != nil {
		input.Conclusion = request.Body.Conclusion
	}
	if request.Body.Argument != nil {
		input.Argument = request.Body.Argument
	}
	if request.Body.AnalysisIds != nil {
		input.AnalysisIDs = *request.Body.AnalysisIds
	}
	if request.Body.ResearchStatus != nil {
		rs := string(*request.Body.ResearchStatus)
		input.ResearchStatus = &rs
	}

	_, err := ss.server.commandHandler.UpdateProofSummary(ctx, input)
	if err != nil {
		if errors.Is(err, command.ErrProofSummaryNotFound) {
			return UpdateProofSummary404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Proof summary not found",
			}}, nil
		}
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return UpdateProofSummary409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict",
			}}, nil
		}
		if errors.Is(err, command.ErrInvalidInput) {
			return UpdateProofSummary400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_input",
				Message: err.Error(),
			}}, nil
		}
		return nil, err
	}

	// Fetch the updated summary
	summary, err := ss.server.evidenceService.GetProofSummary(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return UpdateProofSummary200JSONResponse(convertQueryProofSummaryToGenerated(*summary)), nil
}

// DeleteProofSummary implements StrictServerInterface.
func (ss *StrictServer) DeleteProofSummary(ctx context.Context, request DeleteProofSummaryRequestObject) (DeleteProofSummaryResponseObject, error) {
	version := int64(0)
	if request.Params.Version != nil {
		version = *request.Params.Version
	}

	err := ss.server.commandHandler.DeleteProofSummary(ctx, request.Id, version, "")
	if err != nil {
		if errors.Is(err, command.ErrProofSummaryNotFound) {
			return DeleteProofSummary404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Proof summary not found",
			}}, nil
		}
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return DeleteProofSummary409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict",
			}}, nil
		}
		return nil, err
	}

	return DeleteProofSummary204Response{}, nil
}

// GetProofSummaryByFact implements StrictServerInterface.
func (ss *StrictServer) GetProofSummaryByFact(ctx context.Context, request GetProofSummaryByFactRequestObject) (GetProofSummaryByFactResponseObject, error) {
	summary, err := ss.server.evidenceService.GetProofSummaryForFact(ctx, request.Params.FactType, request.Params.SubjectId)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			// Return empty array instead of 404 for by-fact queries
			return GetProofSummaryByFact200JSONResponse([]ProofSummary{}), nil
		}
		return nil, err
	}

	return GetProofSummaryByFact200JSONResponse([]ProofSummary{
		convertQueryProofSummaryToGenerated(*summary),
	}), nil
}

// ============================================================================
// Conversion helpers
// ============================================================================

func convertQueryEvidenceAnalysisToGenerated(a query.EvidenceAnalysis) EvidenceAnalysis {
	resp := EvidenceAnalysis{
		Id:         a.ID,
		FactType:   a.FactType,
		SubjectId:  a.SubjectID,
		Conclusion: a.Conclusion,
		Version:    a.Version,
		CreatedAt:  &a.CreatedAt,
		UpdatedAt:  &a.UpdatedAt,
	}

	if len(a.CitationIDs) > 0 {
		resp.CitationIds = &a.CitationIDs
	}
	if a.ResearchStatus != nil {
		rs := ResearchStatus(*a.ResearchStatus)
		resp.ResearchStatus = &rs
	}
	if a.Notes != nil {
		resp.Notes = a.Notes
	}

	return resp
}

func convertQueryEvidenceConflictToGenerated(c query.EvidenceConflict) EvidenceConflict {
	resp := EvidenceConflict{
		Id:          c.ID,
		FactType:    c.FactType,
		SubjectId:   c.SubjectID,
		AnalysisIds: c.AnalysisIDs,
		Description: c.Description,
		Status:      EvidenceConflictStatus(c.Status),
		Version:     c.Version,
		CreatedAt:   &c.CreatedAt,
		UpdatedAt:   &c.UpdatedAt,
	}

	if c.Resolution != nil {
		resp.Resolution = c.Resolution
	}

	return resp
}

func convertQueryResearchLogToGenerated(l query.ResearchLogEntry) ResearchLog {
	resp := ResearchLog{
		Id:                l.ID,
		SubjectId:         l.SubjectID,
		SubjectType:       l.SubjectType,
		Repository:        l.Repository,
		SearchDescription: l.SearchDescription,
		Outcome:           ResearchLogOutcome(l.Outcome),
		SearchDate:        l.SearchDate,
		Version:           l.Version,
		CreatedAt:         &l.CreatedAt,
		UpdatedAt:         &l.UpdatedAt,
	}

	if l.Notes != nil {
		resp.Notes = l.Notes
	}

	return resp
}

func convertQueryProofSummaryToGenerated(s query.ProofSummaryResult) ProofSummary {
	resp := ProofSummary{
		Id:         s.ID,
		FactType:   s.FactType,
		SubjectId:  s.SubjectID,
		Conclusion: s.Conclusion,
		Argument:   s.Argument,
		Version:    s.Version,
		CreatedAt:  &s.CreatedAt,
		UpdatedAt:  &s.UpdatedAt,
	}

	if len(s.AnalysisIDs) > 0 {
		resp.AnalysisIds = &s.AnalysisIDs
	}
	if s.ResearchStatus != nil {
		rs := ProofSummaryResearchStatus(*s.ResearchStatus)
		resp.ResearchStatus = &rs
	}

	return resp
}
