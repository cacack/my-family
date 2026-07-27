package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// ============================================================================
// Branch scope resolution (?branch=)
// ============================================================================

// branchScopeMode says whether a request wants to read a branch's view or write
// to it. The two differ only for terminal branches — see resolveBranchScope.
type branchScopeMode int

const (
	branchScopeRead branchScopeMode = iota
	branchScopeWrite
)

// resolveBranchScope turns the optional `?branch=` query parameter into the
// branch a request is scoped to. A nil parameter means the mainline and returns
// (nil, nil) — callers pass that straight to Handler.WithBranch and
// branchScopeID, both of which treat nil as main, so omitting the parameter is
// behaviorally identical to the pre-branch API.
//
// Errors are *echo.HTTPError so handlers can return them unchanged and let
// customErrorHandler render the standard {code, message} body:
//
//   - unknown branch id (or no branch registry configured) -> 404
//   - write to a merged/archived branch -> 409; terminal branches are read-only
//     per ADR-005
//   - read of a merged/archived branch -> 404, not 409: archiving purges the
//     branch's read-model overlay rows, so a terminal branch has no view left to
//     return. There is nothing to read, hence "not found" rather than "refused".
func (ss *StrictServer) resolveBranchScope(ctx context.Context, param *BranchScope, mode branchScopeMode) (*domain.Branch, error) {
	if param == nil {
		return nil, nil
	}

	if ss.server.branchStore == nil {
		// No registry means no branch can exist on this server.
		return nil, echo.NewHTTPError(http.StatusNotFound, "Branch not found")
	}

	branch, err := ss.server.branchStore.Get(ctx, *param)
	if err != nil {
		if errors.Is(err, repository.ErrBranchNotFound) {
			return nil, echo.NewHTTPError(http.StatusNotFound, "Branch not found")
		}
		return nil, err
	}

	if branch.Status != domain.BranchStatusActive {
		if mode == branchScopeWrite {
			return nil, echo.NewHTTPError(http.StatusConflict,
				"Branch is "+string(branch.Status)+" and accepts no further writes")
		}
		return nil, echo.NewHTTPError(http.StatusNotFound,
			"Branch is "+string(branch.Status)+" and its isolated view no longer exists")
	}

	return branch, nil
}

// branchScopeID is the read-side counterpart of Handler.WithBranch: a nil branch
// is the mainline.
func branchScopeID(branch *domain.Branch) domain.BranchID {
	if branch == nil {
		return domain.MainBranchID
	}
	return domain.BranchID(branch.ID)
}

// branchWriter returns the command handler scoped to branch (the receiver
// itself when branch is nil, i.e. the mainline).
func (ss *StrictServer) branchWriter(branch *domain.Branch) *command.Handler {
	return ss.server.commandHandler.WithBranch(branch)
}

// ============================================================================
// Branch endpoints
// ============================================================================

// errBranchesUnavailable is the body returned when the server was built without
// a branch registry (api.WithBranchStore was not supplied).
var errBranchesUnavailable = Error{
	Code:    "branches_unavailable",
	Message: "Branch registry is not configured on this server",
}

// ListBranches implements StrictServerInterface.
func (ss *StrictServer) ListBranches(ctx context.Context, _ ListBranchesRequestObject) (ListBranchesResponseObject, error) {
	if ss.server.branchService == nil {
		return ListBranches503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	branches, err := ss.server.branchService.ListBranches(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]Branch, len(branches))
	for i, b := range branches {
		items[i] = convertDomainBranchToGenerated(b)
	}

	return ListBranches200JSONResponse{
		Items: items,
		Total: len(items),
	}, nil
}

// CreateBranch implements StrictServerInterface.
func (ss *StrictServer) CreateBranch(ctx context.Context, request CreateBranchRequestObject) (CreateBranchResponseObject, error) {
	if ss.server.branchStore == nil {
		return CreateBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}
	if request.Body == nil {
		return CreateBranch400JSONResponse{BadRequestJSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}}, nil
	}

	description := ""
	if request.Body.Description != nil {
		description = *request.Body.Description
	}

	branch, err := ss.server.commandHandler.CreateBranch(ctx, request.Body.Name, description)
	if err != nil {
		if errors.Is(err, domain.ErrBranchNameRequired) ||
			errors.Is(err, domain.ErrBranchNameTooLong) ||
			errors.Is(err, domain.ErrBranchDescTooLong) {
			return CreateBranch400JSONResponse{BadRequestJSONResponse{
				Code:    "validation_error",
				Message: err.Error(),
			}}, nil
		}
		if errors.Is(err, command.ErrBranchStoreRequired) || errors.Is(err, command.ErrPositionSourceRequired) {
			return CreateBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
		}
		return nil, err
	}

	return CreateBranch201JSONResponse(convertDomainBranchToGenerated(branch)), nil
}

// GetBranch implements StrictServerInterface.
func (ss *StrictServer) GetBranch(ctx context.Context, request GetBranchRequestObject) (GetBranchResponseObject, error) {
	if ss.server.branchService == nil {
		return GetBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	branch, err := ss.server.branchService.GetBranch(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrBranchNotFound) {
			return GetBranch404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Branch not found",
			}}, nil
		}
		return nil, err
	}

	return GetBranch200JSONResponse(convertDomainBranchToGenerated(branch)), nil
}

// DeleteBranch implements StrictServerInterface. Deleting archives: the branch
// record and its events are retained (ES-002); only the read-model overlay is
// purged. See the operation description in openapi.yaml.
func (ss *StrictServer) DeleteBranch(ctx context.Context, request DeleteBranchRequestObject) (DeleteBranchResponseObject, error) {
	if ss.server.branchStore == nil {
		return DeleteBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	err := ss.server.commandHandler.DeleteBranch(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrBranchNotFound) {
			return DeleteBranch404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Branch not found",
			}}, nil
		}
		if errors.Is(err, command.ErrBranchNotActive) {
			return DeleteBranch409JSONResponse{
				Code:    "conflict",
				Message: err.Error(),
			}, nil
		}
		if errors.Is(err, command.ErrBranchStoreRequired) {
			return DeleteBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
		}
		return nil, err
	}

	return DeleteBranch204Response{}, nil
}

// CompareBranch implements StrictServerInterface.
func (ss *StrictServer) CompareBranch(ctx context.Context, request CompareBranchRequestObject) (CompareBranchResponseObject, error) {
	if ss.server.branchService == nil {
		return CompareBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	result, err := ss.server.branchService.CompareBranch(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrBranchNotFound) {
			return CompareBranch404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Branch not found",
			}}, nil
		}
		return nil, err
	}

	// The schema requires the hint array, so serialize [] rather than null.
	overlapping := result.OverlappingStreamIDs
	if overlapping == nil {
		overlapping = []openapi_types.UUID{}
	}

	return CompareBranch200JSONResponse{
		Branch:               convertDomainBranchToGenerated(result.Branch),
		BasePosition:         result.BasePosition,
		BranchChanges:        convertQueryChangeEntriesToGenerated(result.BranchChanges),
		MainChanges:          convertQueryChangeEntriesToGenerated(result.MainChanges),
		BranchChangeCount:    result.BranchChangeCount,
		MainChangeCount:      result.MainChangeCount,
		HasMore:              result.HasMore,
		OverlappingStreamIds: overlapping,
	}, nil
}

// convertDomainBranchToGenerated converts a domain.Branch to the generated Branch type.
func convertDomainBranchToGenerated(b *domain.Branch) Branch {
	branch := Branch{
		Id:           b.ID,
		Name:         b.Name,
		BasePosition: b.BasePosition,
		Status:       BranchStatus(b.Status),
		CreatedAt:    b.CreatedAt,
	}
	if b.Description != "" {
		branch.Description = &b.Description
	}
	return branch
}

// convertQueryChangeEntriesToGenerated converts a slice of query change entries,
// always returning a non-nil slice so the JSON payload carries [] rather than null.
func convertQueryChangeEntriesToGenerated(entries []query.ChangeEntry) []ChangeEntry {
	out := make([]ChangeEntry, len(entries))
	for i, e := range entries {
		out[i] = convertQueryChangeEntryToGenerated(e)
	}
	return out
}
