package api

import (
	"context"
	"errors"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// branchNotFound is the 404 body shared by the close-related handlers.
var branchNotFound = NotFoundJSONResponse{Code: "not_found", Message: "Branch not found"}

// CloseBranch implements StrictServerInterface: close an active branch
// without merging, recording the outcome and reason (#836).
func (ss *StrictServer) CloseBranch(ctx context.Context, request CloseBranchRequestObject) (CloseBranchResponseObject, error) {
	if ss.server.branchStore == nil || ss.server.branchService == nil {
		return CloseBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}
	if request.Body == nil {
		return CloseBranch400JSONResponse{Code: "invalid_request", Message: "Request body is required"}, nil
	}
	reason := ""
	if request.Body.Reason != nil {
		reason = *request.Body.Reason
	}

	err := ss.server.commandHandler.CloseBranch(ctx, request.Id, domain.BranchOutcome(request.Body.Outcome), reason)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrBranchNotFound):
			return CloseBranch404JSONResponse{branchNotFound}, nil
		case errors.Is(err, command.ErrBranchNotActive):
			return CloseBranch409JSONResponse{Code: "branch_not_active", Message: err.Error()}, nil
		case errors.Is(err, repository.ErrConcurrencyConflict):
			return CloseBranch409JSONResponse{Code: "branch_changed", Message: err.Error()}, nil
		case errors.Is(err, domain.ErrBranchInvalidCloseOutcome), errors.Is(err, domain.ErrBranchCloseReasonTooLong):
			return CloseBranch400JSONResponse{Code: "validation_error", Message: err.Error()}, nil
		case errors.Is(err, command.ErrBranchStoreRequired):
			return CloseBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
		}
		return nil, err
	}

	branch, err := ss.server.branchService.GetBranch(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	resp, err := ss.branchWithLinks(ctx, branch)
	if err != nil {
		return nil, err
	}
	return CloseBranch200JSONResponse(resp), nil
}

// GetBranchResearch implements StrictServerInterface: the branch's GPS
// artifacts rebuilt from its events (#836).
func (ss *StrictServer) GetBranchResearch(ctx context.Context, request GetBranchResearchRequestObject) (GetBranchResearchResponseObject, error) {
	if ss.server.branchService == nil {
		return GetBranchResearch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}
	archive, err := ss.server.branchService.BranchResearchArchive(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrBranchNotFound) {
			return GetBranchResearch404JSONResponse{branchNotFound}, nil
		}
		return nil, err
	}
	return GetBranchResearch200JSONResponse(convertBranchResearchArchive(archive)), nil
}

// PromoteBranchResearchLogs implements StrictServerInterface: copy a closed
// branch's research logs to the mainline (#836).
func (ss *StrictServer) PromoteBranchResearchLogs(ctx context.Context, request PromoteBranchResearchLogsRequestObject) (PromoteBranchResearchLogsResponseObject, error) {
	if ss.server.branchStore == nil {
		return PromoteBranchResearchLogs503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}
	var ids []uuid.UUID
	if request.Body != nil && request.Body.LogIds != nil {
		// Checked here too, before copying: the server has no request
		// validator, so maxItems in the spec is enforced by hand.
		if len(*request.Body.LogIds) > command.MaxPromoteLogIDs {
			return PromoteBranchResearchLogs400JSONResponse{Code: "validation_error", Message: command.ErrTooManyPromoteLogIDs.Error()}, nil
		}
		ids = append(ids, (*request.Body.LogIds)...)
	}

	result, err := ss.server.commandHandler.PromoteBranchResearchLogs(ctx, request.Id, ids)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrBranchNotFound):
			return PromoteBranchResearchLogs404JSONResponse{branchNotFound}, nil
		case errors.Is(err, command.ErrBranchNotClosed):
			return PromoteBranchResearchLogs409JSONResponse{Code: "branch_not_closed", Message: err.Error()}, nil
		case errors.Is(err, command.ErrBranchStoreRequired):
			return PromoteBranchResearchLogs503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
		}
		return nil, err
	}

	resp := PromoteResearchLogsResult{
		Promoted:  append([]openapi_types.UUID{}, result.Promoted...),
		Skipped:   make([]PromoteSkippedLog, len(result.Skipped)),
		Truncated: result.Truncated,
	}
	for i, skipped := range result.Skipped {
		resp.Skipped[i] = PromoteSkippedLog{Id: skipped.ID, Reason: PromoteSkippedLogReason(skipped.Reason)}
	}
	return PromoteBranchResearchLogs200JSONResponse(resp), nil
}

// convertBranchResearchArchive converts the query archive to the wire shape,
// with every array present ([] when empty).
func convertBranchResearchArchive(archive *query.BranchResearchArchive) BranchResearchArchive {
	out := BranchResearchArchive{
		BranchId:         archive.Branch.ID,
		ResearchLogs:     make([]ArchivedResearchLog, len(archive.ResearchLogs)),
		EvidenceAnalyses: make([]ArchivedEvidenceAnalysis, len(archive.EvidenceAnalyses)),
		ProofSummaries:   make([]ArchivedProofSummary, len(archive.ProofSummaries)),
		DeletedCount:     archive.DeletedCount,
		Truncated:        archive.Truncated,
	}
	for i, entry := range archive.ResearchLogs {
		out.ResearchLogs[i] = ArchivedResearchLog{
			Log:             convertQueryResearchLogToGenerated(entry.ResearchLogEntry),
			SubjectName:     optionalName(entry.SubjectName),
			CreatedOnBranch: entry.CreatedOnBranch,
		}
	}
	for i, entry := range archive.EvidenceAnalyses {
		out.EvidenceAnalyses[i] = ArchivedEvidenceAnalysis{
			Analysis:        convertQueryEvidenceAnalysisToGenerated(entry.EvidenceAnalysis),
			SubjectName:     optionalName(entry.SubjectName),
			CreatedOnBranch: entry.CreatedOnBranch,
		}
	}
	for i, entry := range archive.ProofSummaries {
		out.ProofSummaries[i] = ArchivedProofSummary{
			Summary:         convertQueryProofSummaryToGenerated(entry.ProofSummaryResult),
			SubjectName:     optionalName(entry.SubjectName),
			CreatedOnBranch: entry.CreatedOnBranch,
		}
	}
	return out
}

// optionalName returns nil for an empty name so it is omitted on the wire.
func optionalName(name string) *string {
	if name == "" {
		return nil
	}
	return &name
}
