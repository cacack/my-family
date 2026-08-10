package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
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
		Conflicts:            convertQueryMergeConflictsToGenerated(result.Conflicts),
	}, nil
}

// MergeBranch implements StrictServerInterface. The merge is all-or-nothing and
// every detected conflict must carry a resolution; see the operation
// description in openapi.yaml and ADR-005 §Merge.
func (ss *StrictServer) MergeBranch(ctx context.Context, request MergeBranchRequestObject) (MergeBranchResponseObject, error) {
	if ss.server.branchStore == nil {
		return MergeBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	// The body is optional: merging a clean branch with no note needs nothing.
	input := command.MergeBranchInput{BranchID: request.Id}
	if request.Body != nil {
		if request.Body.Note != nil {
			input.Note = *request.Body.Note
		}
		resolutions, err := convertGeneratedResolutionsToCommand(request.Body.Resolutions)
		if err != nil {
			return MergeBranch400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_resolution",
				Message: err.Error(),
			}}, nil
		}
		input.Resolutions = resolutions
	}

	// result is non-nil alongside ErrMergeConflicts and carries the conflicts;
	// every other error path returns a nil result.
	result, err := ss.server.commandHandler.MergeBranch(ctx, input)
	if err != nil {
		return mergeBranchErrorResponse(result, err)
	}

	// The schema requires the skipped array, so serialize [] rather than null.
	skipped := result.SkippedStreamIDs
	if skipped == nil {
		skipped = []openapi_types.UUID{}
	}

	return MergeBranch200JSONResponse{
		Branch:             convertDomainBranchToGenerated(result.Branch),
		MergedAtPosition:   result.MergedAtPosition,
		ReplayedEventCount: result.ReplayedEventCount,
		SkippedStreamIds:   skipped,
	}, nil
}

// mergeBranchErrorResponse maps the merge command's sentinel errors onto the
// operation's responses. The four refusals share the 409 family and are told
// apart by `code`, so a client can render an actionable message for each.
// Unrecognized errors are returned to customErrorHandler as a 500.
func mergeBranchErrorResponse(result *command.MergeBranchResult, err error) (MergeBranchResponseObject, error) {
	refuse := func(code BranchMergeConflictErrorCode) (MergeBranchResponseObject, error) {
		return MergeBranch409JSONResponse{Code: code, Message: err.Error()}, nil
	}

	switch {
	case errors.Is(err, command.ErrMergeConflicts):
		// The whole conflict list travels with the refusal so the review UI can
		// render everything at once, not just the undecided entries.
		var conflicts []query.MergeConflict
		if result != nil {
			conflicts = result.Conflicts
		}
		converted := convertQueryMergeConflictsToGenerated(conflicts)
		return MergeBranch409JSONResponse{
			Code:      MergeConflicts,
			Message:   err.Error(),
			Conflicts: &converted,
		}, nil

	case errors.Is(err, command.ErrBranchNotActive):
		return refuse(BranchNotActive)

	case errors.Is(err, command.ErrMergeAlreadyClaimed):
		return refuse(MergeAlreadyClaimed)

	case errors.Is(err, command.ErrBranchTooLargeToMerge):
		// A 409 rather than a 422: it is a refusal to act on this branch's
		// current state, and keeping the refusals in one status family keeps
		// the client contract simple.
		return refuse(BranchTooLarge)

	case errors.Is(err, command.ErrMainTooFarAheadToMerge):
		// Distinct from branch_too_large on purpose — same status, different
		// cause and different remedy. Telling someone their three-event branch
		// is "too large" points them at the wrong fix.
		return refuse(MainTooFarAhead)

	case errors.Is(err, command.ErrMergeDanglingReference):
		return refuse(MergeDanglingReference)

	case errors.Is(err, command.ErrUnknownResolution),
		errors.Is(err, command.ErrUnsupportedResolution):
		// Both are "the caller asked for something this merge cannot do", and
		// both are refused before anything is written. The message names the
		// resolutions the conflict does accept.
		return MergeBranch400JSONResponse{BadRequestJSONResponse{
			Code:    "invalid_resolution",
			Message: err.Error(),
		}}, nil

	case errors.Is(err, command.ErrMergePartiallyApplied):
		// Deliberately NOT a 409: the other refusals mean nothing was written,
		// and this one means the opposite — the branch is merged and main is
		// half-updated. Folding it into the 409 family would tell a client it
		// is safe to retry when it is not. See the operation's 500 description.
		return MergeBranch500JSONResponse{
			Code:    "merge_partially_applied",
			Message: err.Error(),
		}, nil

	// MUST stay below the partially-applied case. A stale plan caught during
	// the replay is wrapped in ErrMergePartiallyApplied, so errors.Is matches
	// BOTH sentinels there; ordered the other way, a half-applied merge would
	// come back as a 409 telling the client nothing was written and a retry is
	// safe — the single most dangerous thing this endpoint could say. Reaching
	// here means the pre-claim check refused, which genuinely wrote nothing.
	case errors.Is(err, command.ErrMergePlanStale):
		return refuse(MergePlanStale)

	case errors.Is(err, domain.ErrBranchMergeNoteTooLong):
		return MergeBranch400JSONResponse{BadRequestJSONResponse{
			Code:    "validation_error",
			Message: err.Error(),
		}}, nil

	case errors.Is(err, repository.ErrBranchNotFound):
		return MergeBranch404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Branch not found",
		}}, nil

	case errors.Is(err, command.ErrBranchStoreRequired), errors.Is(err, command.ErrPositionSourceRequired):
		return MergeBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	return nil, err
}

// convertGeneratedResolutionsToCommand turns the wire array into the command's
// streamID→side map. The array carries no uniqueness guarantee, so a repeated
// stream_id is rejected rather than silently resolved last-one-wins — two
// entries for one entity mean the caller is unsure which side should win.
//
// Unrecognized resolution values are NOT rejected here: the command owns that
// check (ErrUnknownResolution), which also catches a stream the branch never
// touched, so both misuses report through one path.
func convertGeneratedResolutionsToCommand(entries *[]MergeResolutionEntry) (map[uuid.UUID]command.MergeResolution, error) {
	if entries == nil || len(*entries) == 0 {
		return nil, nil
	}

	out := make(map[uuid.UUID]command.MergeResolution, len(*entries))
	for _, entry := range *entries {
		if _, duplicate := out[entry.StreamId]; duplicate {
			return nil, fmt.Errorf("resolutions contains stream %s more than once", entry.StreamId)
		}
		out[entry.StreamId] = command.MergeResolution(entry.Resolution)
	}
	return out, nil
}

// convertQueryMergeConflictsToGenerated converts the query layer's conflicts,
// always returning a non-nil slice so the JSON payload carries [] rather than
// null. Fields stays absent for the kinds that have none (edit_edit is the only
// kind that names fields).
func convertQueryMergeConflictsToGenerated(conflicts []query.MergeConflict) []MergeConflict {
	out := make([]MergeConflict, len(conflicts))
	for i, c := range conflicts {
		// The schema requires supported_resolutions, so build a non-nil slice
		// even in the impossible case of a conflict that accepts neither side —
		// the wire contract says array, never null.
		supported := make([]MergeConflictSupportedResolutions, 0, len(c.SupportedResolutions))
		for _, r := range c.SupportedResolutions {
			supported = append(supported, MergeConflictSupportedResolutions(r))
		}

		out[i] = MergeConflict{
			StreamId:             c.StreamID,
			EntityType:           c.EntityType,
			EntityName:           c.EntityName,
			Kind:                 MergeConflictKind(c.Kind),
			Detail:               c.Detail,
			SupportedResolutions: supported,
		}
		if len(c.Fields) > 0 {
			fields := c.Fields
			out[i].Fields = &fields
		}
	}
	return out
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
	// Merge metadata exists only after the active→merged transition. Copied
	// rather than aliased so the response cannot mutate the registry's branch.
	if b.MergedAt != nil {
		mergedAt := *b.MergedAt
		branch.MergedAt = &mergedAt
	}
	if b.MergeNote != "" {
		mergeNote := b.MergeNote
		branch.MergeNote = &mergeNote
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
