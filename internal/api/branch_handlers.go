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
		ss.addMergeState(ctx, &items[i], b)
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
		return CreateBranch400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	input := command.CreateBranchInput{Name: request.Body.Name}
	if request.Body.Description != nil {
		input.Description = *request.Body.Description
	}
	if request.Body.Hypothesis != nil {
		input.Research.Hypothesis = *request.Body.Hypothesis
	}
	if request.Body.Subjects != nil {
		input.Research.Subjects = convertGeneratedBranchSubjects(*request.Body.Subjects)
	}
	if request.Body.Outcome != nil {
		// The command reads an empty outcome as "open" (its default for callers
		// that set none); a client that names one must name a valid value.
		outcome := domain.BranchOutcome(*request.Body.Outcome)
		if !outcome.IsValid() {
			return CreateBranch400JSONResponse{
				Code:    "validation_error",
				Message: domain.ErrBranchInvalidOutcome.Error(),
			}, nil
		}
		input.Research.Outcome = outcome
	}
	if request.Body.ProofSummaryIds != nil {
		input.Research.ProofSummaryIDs = append([]uuid.UUID(nil), (*request.Body.ProofSummaryIds)...)
	}

	branch, err := ss.server.commandHandler.CreateBranchWithResearch(ctx, input)
	if err != nil {
		if errors.Is(err, domain.ErrBranchNameRequired) ||
			errors.Is(err, domain.ErrBranchNameTooLong) ||
			errors.Is(err, domain.ErrBranchDescTooLong) ||
			isBranchResearchValidationError(err) {
			return CreateBranch400JSONResponse{
				Code:    "validation_error",
				Message: err.Error(),
			}, nil
		}
		if isBranchReferenceError(err) {
			return CreateBranch400JSONResponse{
				Code:    "invalid_reference",
				Message: err.Error(),
			}, nil
		}
		if errors.Is(err, command.ErrBranchStoreRequired) || errors.Is(err, command.ErrPositionSourceRequired) {
			return CreateBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
		}
		return nil, err
	}

	resp, err := ss.branchWithLinks(ctx, branch)
	if err != nil {
		return nil, err
	}
	return CreateBranch201JSONResponse(resp), nil
}

// UpdateBranch implements StrictServerInterface: a partial edit of a branch's
// description and research record (#835). See the operation description in
// openapi.yaml for which fields each status accepts.
func (ss *StrictServer) UpdateBranch(ctx context.Context, request UpdateBranchRequestObject) (UpdateBranchResponseObject, error) {
	if ss.server.branchStore == nil {
		return UpdateBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}
	if request.Body == nil {
		return UpdateBranch400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	body := request.Body
	input := command.UpdateBranchInput{
		BranchID:    request.Id,
		Description: body.Description,
		Hypothesis:  body.Hypothesis,
	}
	if body.Subjects != nil {
		subjects := convertGeneratedBranchSubjects(*body.Subjects)
		input.Subjects = &subjects
	}
	if body.Outcome != nil {
		outcome := domain.BranchOutcome(*body.Outcome)
		input.Outcome = &outcome
	}
	if body.ProofSummaryIds != nil {
		ids := append([]uuid.UUID{}, (*body.ProofSummaryIds)...)
		input.ProofSummaryIDs = &ids
	}

	branch, err := ss.server.commandHandler.UpdateBranch(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrBranchNotFound):
			return UpdateBranch404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Branch not found",
			}}, nil
		case errors.Is(err, command.ErrBranchNotActive):
			return UpdateBranch409JSONResponse{Code: "branch_not_active", Message: err.Error()}, nil
		case errors.Is(err, command.ErrBranchFieldLocked):
			return UpdateBranch409JSONResponse{Code: "branch_field_locked", Message: err.Error()}, nil
		case errors.Is(err, repository.ErrConcurrencyConflict):
			// A rival write to the branch's stream landed after this edit read
			// the branch; nothing was recorded.
			return UpdateBranch409JSONResponse{Code: "branch_changed", Message: err.Error()}, nil
		case errors.Is(err, command.ErrBranchUpdateEmpty),
			errors.Is(err, domain.ErrBranchDescTooLong),
			isBranchResearchValidationError(err):
			return UpdateBranch400JSONResponse{Code: "validation_error", Message: err.Error()}, nil
		case isBranchReferenceError(err):
			return UpdateBranch400JSONResponse{Code: "invalid_reference", Message: err.Error()}, nil
		case errors.Is(err, command.ErrBranchStoreRequired):
			return UpdateBranch503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
		}
		return nil, err
	}

	resp, err := ss.branchWithLinks(ctx, branch)
	if err != nil {
		return nil, err
	}
	return UpdateBranch200JSONResponse(resp), nil
}

// isBranchResearchValidationError reports whether err is one of the research
// record's field-validation errors (#835).
func isBranchResearchValidationError(err error) bool {
	for _, target := range []error{
		domain.ErrBranchHypothesisTooLong,
		domain.ErrBranchInvalidOutcome,
		domain.ErrBranchTooManySubjects,
		domain.ErrBranchInvalidSubject,
		domain.ErrBranchDuplicateSubject,
		domain.ErrBranchTooManyProofSummaries,
		domain.ErrBranchInvalidProofSummaryID,
		domain.ErrBranchDuplicateProofSummary,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// isBranchReferenceError reports whether err says a subject or proof summary
// does not exist where the branch can see it.
func isBranchReferenceError(err error) bool {
	return errors.Is(err, command.ErrBranchSubjectNotFound) || errors.Is(err, command.ErrBranchProofSummaryNotFound)
}

// convertGeneratedBranchSubjects converts wire subjects to domain subjects.
// Unknown types pass through and are rejected by domain validation.
func convertGeneratedBranchSubjects(in []BranchSubjectInput) []domain.BranchSubject {
	out := make([]domain.BranchSubject, len(in))
	for i, subject := range in {
		out[i] = domain.BranchSubject{Type: domain.BranchSubjectType(subject.Type), ID: subject.Id}
	}
	return out
}

// branchWithLinks converts a branch and resolves its research record for
// display: subject names and the linked proof summaries (#835), plus a merged
// branch's merge state (#830). Used by the single-branch reads; the list skips
// the research-record resolution.
func (ss *StrictServer) branchWithLinks(ctx context.Context, b *domain.Branch) (Branch, error) {
	out := convertDomainBranchToGenerated(b)
	ss.addMergeState(ctx, &out, b)
	if ss.server.branchService == nil {
		return out, nil
	}
	links, err := ss.server.branchService.ResolveBranchLinks(ctx, b)
	if err != nil {
		return Branch{}, err
	}
	for i := range out.Subjects {
		if name, ok := links.SubjectNames[out.Subjects[i].Id]; ok {
			resolved := name
			out.Subjects[i].Name = &resolved
		}
	}
	refs := make([]BranchProofSummaryRef, len(links.ProofSummaries))
	for i, summary := range links.ProofSummaries {
		refs[i] = BranchProofSummaryRef{
			Id:         summary.ID,
			FactType:   string(summary.FactType),
			Conclusion: summary.Conclusion,
		}
	}
	out.ProofSummaries = &refs
	return out, nil
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

	resp, err := ss.branchWithLinks(ctx, branch)
	if err != nil {
		return nil, err
	}
	return GetBranch200JSONResponse(resp), nil
}

// DeleteBranch implements StrictServerInterface. It closes the branch as
// abandoned with no reason (CloseBranch records why, #836): the branch record
// and its events are retained (ES-002); only the read-model overlay is purged.
// See the operation description in openapi.yaml.
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

	branch, err := ss.branchWithLinks(ctx, result.Branch)
	if err != nil {
		return nil, err
	}

	return CompareBranch200JSONResponse{
		Branch:               branch,
		BasePosition:         result.BasePosition,
		BranchChanges:        convertQueryChangeEntriesToGenerated(result.BranchChanges),
		MainChanges:          convertQueryChangeEntriesToGenerated(result.MainChanges),
		BranchChangeCount:    result.BranchChangeCount,
		MainChangeCount:      result.MainChangeCount,
		HasMore:              result.HasMore,
		OverlappingStreamIds: overlapping,
		Conflicts:            convertQueryMergeConflictsToGenerated(result.Conflicts),
		ReplayedChangeCount:  &result.ReplayedChangeCount,
		MergeRecord:          convertQueryMergeRecordToGenerated(result.MergeRecord),
	}, nil
}

// convertQueryMergeRecordToGenerated converts a merged branch's merge record
// (#832); nil for a branch that was never merged.
func convertQueryMergeRecordToGenerated(record *query.MergeRecord) *MergeRecord {
	if record == nil {
		return nil
	}
	out := &MergeRecord{
		ClaimId:                 record.ClaimID,
		MergedAt:                record.MergedAt,
		MergedAtPosition:        record.MergedAtPosition,
		Recorded:                record.Recorded,
		ReplayedEventCount:      record.ReplayedEventCount,
		ResumeCount:             record.ResumeCount,
		SkippedStreamIds:        append([]openapi_types.UUID{}, record.SkippedStreamIDs...),
		PreMergeSnapshotId:      record.PreMergeSnapshotID,
		ReplayedThroughPosition: record.ReplayedThroughPosition,
		Decisions:               make([]MergeRecordDecision, 0, len(record.Decisions)),
		Exclusions:              make([]MergeRecordExclusion, 0, len(record.Exclusions)),
	}
	if record.Note != "" {
		note := record.Note
		out.Note = &note
	}
	for _, d := range record.Decisions {
		decision := MergeRecordDecision{
			StreamId:   d.StreamID,
			EntityType: d.EntityType,
			EntityName: d.EntityName,
			Resolution: MergeRecordDecisionResolution(d.Resolution),
			DecidedAt:  MergeRecordDecisionDecidedAt(d.DecidedAt),
		}
		if d.Kind != "" {
			kind := MergeRecordDecisionKind(d.Kind)
			decision.Kind = &kind
		}
		if len(d.Fields) > 0 {
			fields := append([]string{}, d.Fields...)
			decision.Fields = &fields
		}
		if d.DeletedBy != "" {
			deletedBy := MergeRecordDecisionDeletedBy(d.DeletedBy)
			decision.DeletedBy = &deletedBy
		}
		if d.Rationale != "" {
			rationale := d.Rationale
			decision.Rationale = &rationale
		}
		out.Decisions = append(out.Decisions, decision)
	}
	for _, e := range record.Exclusions {
		exclusion := MergeRecordExclusion{StreamId: e.StreamID, EntityType: e.EntityType, EntityName: e.EntityName}
		if e.Rationale != "" {
			rationale := e.Rationale
			exclusion.Rationale = &rationale
		}
		out.Exclusions = append(out.Exclusions, exclusion)
	}
	return out
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
		resolutions, rationales, err := convertGeneratedResolutionsToCommand(request.Body.Resolutions)
		if err != nil {
			return MergeBranch400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_resolution",
				Message: err.Error(),
			}}, nil
		}
		input.Resolutions = resolutions
		input.Rationales = rationales
		input.SnapshotBefore = request.Body.SnapshotBefore != nil && *request.Body.SnapshotBefore
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

	response := MergeBranch200JSONResponse{
		Branch:             convertDomainBranchToGenerated(result.Branch),
		MergedAtPosition:   result.MergedAtPosition,
		ReplayedEventCount: result.ReplayedEventCount,
		SkippedStreamIds:   skipped,
	}
	if result.PreMergeSnapshot != nil {
		snapshot := convertDomainSnapshotToGenerated(result.PreMergeSnapshot)
		response.PreMergeSnapshot = &snapshot
	}
	return response, nil
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
		// Every blocker travels with the refusal, named, so the review can
		// show them all and offer each one's fix (#831).
		return MergeBranch409JSONResponse{
			Code:     MergeDanglingReference,
			Message:  err.Error(),
			Blockers: mergeBlockersOf(err),
		}, nil

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

	case errors.Is(err, command.ErrMergeEmpty):
		// A refusal like the others: nothing was written and the branch is
		// still active, so it shares their 409 family.
		return refuse(MergeEmpty)

	case errors.Is(err, domain.ErrBranchMergeNoteTooLong),
		errors.Is(err, domain.ErrResolutionRationaleTooLong):
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

// PrecheckBranchMerge implements StrictServerInterface. It reports the merge
// blockers the proposed resolutions would be refused with, writing nothing
// (#831); see command.Handler.PrecheckMerge.
func (ss *StrictServer) PrecheckBranchMerge(ctx context.Context, request PrecheckBranchMergeRequestObject) (PrecheckBranchMergeResponseObject, error) {
	if ss.server.branchStore == nil {
		return PrecheckBranchMerge503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}
	input := command.PrecheckMergeInput{BranchID: request.Id}
	if request.Body != nil {
		resolutions, _, err := convertGeneratedResolutionsToCommand(request.Body.Resolutions)
		if err != nil {
			return PrecheckBranchMerge400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_resolution",
				Message: err.Error(),
			}}, nil
		}
		input.Resolutions = resolutions
	}

	blockers, err := ss.server.commandHandler.PrecheckMerge(ctx, input)
	if err != nil {
		return precheckBranchMergeErrorResponse(err)
	}
	return PrecheckBranchMerge200JSONResponse{Blockers: convertMergeBlockersToGenerated(blockers)}, nil
}

// precheckBranchMergeErrorResponse maps the refusals a precheck shares with
// the merge onto its responses. Unrecognized errors go to customErrorHandler
// as a 500.
func precheckBranchMergeErrorResponse(err error) (PrecheckBranchMergeResponseObject, error) {
	refuse := func(code BranchMergeConflictErrorCode) (PrecheckBranchMergeResponseObject, error) {
		return PrecheckBranchMerge409JSONResponse{Code: code, Message: err.Error()}, nil
	}
	switch {
	case errors.Is(err, command.ErrBranchNotActive):
		return refuse(BranchNotActive)
	case errors.Is(err, command.ErrBranchTooLargeToMerge):
		return refuse(BranchTooLarge)
	case errors.Is(err, command.ErrMainTooFarAheadToMerge):
		return refuse(MainTooFarAhead)
	case errors.Is(err, command.ErrMergeEmpty):
		return refuse(MergeEmpty)
	case errors.Is(err, command.ErrUnknownResolution),
		errors.Is(err, command.ErrUnsupportedResolution):
		return PrecheckBranchMerge400JSONResponse{BadRequestJSONResponse{
			Code:    "invalid_resolution",
			Message: err.Error(),
		}}, nil
	case errors.Is(err, repository.ErrBranchNotFound):
		return PrecheckBranchMerge404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Branch not found",
		}}, nil
	case errors.Is(err, command.ErrBranchStoreRequired):
		return PrecheckBranchMerge503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}
	return nil, err
}

// mergeBlockersOf returns the blockers a merge or resume refusal carries, or
// nil when it carries none.
func mergeBlockersOf(err error) *[]MergeBlocker {
	var blocked *command.MergeBlockedError
	if !errors.As(err, &blocked) {
		return nil
	}
	converted := convertMergeBlockersToGenerated(blocked.Blockers)
	return &converted
}

// convertMergeBlockersToGenerated converts command blockers, always returning
// a non-nil slice so the JSON payload carries [] rather than null.
func convertMergeBlockersToGenerated(blockers []command.MergeBlocker) []MergeBlocker {
	out := make([]MergeBlocker, len(blockers))
	for i, b := range blockers {
		out[i] = MergeBlocker{
			StreamId:            b.StreamID,
			EntityType:          b.EntityType,
			EntityName:          b.EntityName,
			ReferencedId:        b.ReferencedID,
			ReferencedType:      b.ReferencedType,
			ReferencedName:      b.ReferencedName,
			Kind:                MergeBlockerKind(b.Kind),
			SuggestedResolution: MergeBlockerSuggestedResolution(b.SuggestedResolution),
			Message:             b.Message,
		}
	}
	return out
}

// ResumeBranchMerge implements StrictServerInterface. It finishes a merge whose
// replay onto main was interrupted (#685); see the operation description in
// openapi.yaml and command.Handler.ResumeMerge.
func (ss *StrictServer) ResumeBranchMerge(ctx context.Context, request ResumeBranchMergeRequestObject) (ResumeBranchMergeResponseObject, error) {
	if ss.server.branchStore == nil {
		return ResumeBranchMerge503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	input := command.ResumeMergeInput{BranchID: request.Id}
	if request.Body != nil {
		resolutions, rationales, err := convertGeneratedResolutionsToCommand(request.Body.Resolutions)
		if err != nil {
			return ResumeBranchMerge400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_resolution",
				Message: err.Error(),
			}}, nil
		}
		input.Resolutions = resolutions
		input.Rationales = rationales
	}

	// result is non-nil alongside ErrMergeResumeNeedsResolution and carries the
	// pending streams; every other error path returns a nil result.
	result, err := ss.server.commandHandler.ResumeMerge(ctx, input)
	if err != nil {
		return resumeBranchMergeErrorResponse(result, err)
	}

	// The finished merge reads as complete (#830), so a client can adopt the
	// branch straight from the result.
	branch := convertDomainBranchToGenerated(result.Branch)
	ss.addMergeState(ctx, &branch, result.Branch)

	return ResumeBranchMerge200JSONResponse{
		Branch:                   branch,
		MergedAtPosition:         result.MergedAtPosition,
		ReplayedEventCount:       result.ReplayedEventCount,
		AlreadyReplayedStreamIds: nonNilUUIDs(result.AlreadyReplayedStreamIDs),
		SkippedStreamIds:         nonNilUUIDs(result.SkippedStreamIDs),
		ReprojectedStreamIds:     nonNilUUIDs(result.ReprojectedStreamIDs),
	}, nil
}

// resumeBranchMergeErrorResponse maps the resume command's sentinel errors
// onto the operation's responses. Unrecognized errors are returned to
// customErrorHandler as a 500.
func resumeBranchMergeErrorResponse(result *command.ResumeMergeResult, err error) (ResumeBranchMergeResponseObject, error) {
	refuse := func(code BranchMergeResumeErrorCode) (ResumeBranchMergeResponseObject, error) {
		return ResumeBranchMerge409JSONResponse{Code: code, Message: err.Error()}, nil
	}

	switch {
	case errors.Is(err, command.ErrMergeResumeNeedsResolution):
		var pending []openapi_types.UUID
		if result != nil {
			pending = result.PendingStreamIDs
		}
		pending = nonNilUUIDs(pending)
		var described []MergePendingEntity
		if result != nil {
			described = convertPendingMergeEntities(result.Pending)
		} else {
			described = []MergePendingEntity{}
		}
		return ResumeBranchMerge409JSONResponse{
			Code:             ResumeNeedsResolution,
			Message:          err.Error(),
			PendingStreamIds: &pending,
			Pending:          &described,
		}, nil

	case errors.Is(err, command.ErrMergeNotClaimed):
		return refuse(ResumeMergeNotClaimed)

	case errors.Is(err, command.ErrMergeDanglingReference):
		return ResumeBranchMerge409JSONResponse{
			Code:     ResumeDanglingReference,
			Message:  err.Error(),
			Blockers: mergeBlockersOf(err),
		}, nil

	case errors.Is(err, command.ErrBranchTooLargeToMerge):
		return refuse(ResumeBranchTooLarge)

	case errors.Is(err, command.ErrMergeResumeConcurrent),
		errors.Is(err, command.ErrMergeResumeBranchChanged):
		return refuse(ResumeConcurrent)

	case errors.Is(err, command.ErrUnknownResolution):
		return ResumeBranchMerge400JSONResponse{BadRequestJSONResponse{
			Code:    "invalid_resolution",
			Message: err.Error(),
		}}, nil

	case errors.Is(err, domain.ErrResolutionRationaleTooLong):
		return ResumeBranchMerge400JSONResponse{BadRequestJSONResponse{
			Code:    "validation_error",
			Message: err.Error(),
		}}, nil

	// Checked before any stale-plan sentinel it may wrap, for the same reason
	// as in mergeBranchErrorResponse: the resume wrote something and must say
	// so. Retrying the resume IS the remedy here.
	case errors.Is(err, command.ErrMergePartiallyApplied):
		return ResumeBranchMerge500JSONResponse{
			Code:    "merge_partially_applied",
			Message: err.Error(),
		}, nil

	case errors.Is(err, repository.ErrBranchNotFound):
		return ResumeBranchMerge404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Branch not found",
		}}, nil

	case errors.Is(err, command.ErrBranchStoreRequired):
		return ResumeBranchMerge503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	return nil, err
}

// nonNilUUIDs returns ids, or an empty slice for nil, so a required array
// serializes as [] rather than null.
func nonNilUUIDs(ids []uuid.UUID) []openapi_types.UUID {
	if ids == nil {
		return []openapi_types.UUID{}
	}
	return ids
}

// convertGeneratedResolutionsToCommand turns the wire array into the command's
// streamID→side map. The array carries no uniqueness guarantee, so a repeated
// stream_id is rejected rather than silently resolved last-one-wins — two
// entries for one entity mean the caller is unsure which side should win.
//
// Unrecognized resolution values are NOT rejected here: the command owns that
// check (ErrUnknownResolution), which also catches a stream the branch never
// touched, so both misuses report through one path.
//
// The optional per-entry rationales (#828) come back as their own map, keyed
// the same way; the command validates and records them.
func convertGeneratedResolutionsToCommand(entries *[]MergeResolutionEntry) (map[uuid.UUID]command.MergeResolution, map[uuid.UUID]string, error) {
	if entries == nil || len(*entries) == 0 {
		return nil, nil, nil
	}

	out := make(map[uuid.UUID]command.MergeResolution, len(*entries))
	var rationales map[uuid.UUID]string
	for _, entry := range *entries {
		if _, duplicate := out[entry.StreamId]; duplicate {
			return nil, nil, fmt.Errorf("resolutions contains stream %s more than once", entry.StreamId)
		}
		out[entry.StreamId] = command.MergeResolution(entry.Resolution)
		if entry.Rationale != nil {
			if rationales == nil {
				rationales = make(map[uuid.UUID]string)
			}
			rationales[entry.StreamId] = *entry.Rationale
		}
	}
	return out, rationales, nil
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
		if c.DeletedBy != "" {
			deletedBy := MergeConflictDeletedBy(c.DeletedBy)
			out[i].DeletedBy = &deletedBy
		}
		if len(c.FieldValues) > 0 {
			values := make([]MergeConflictField, len(c.FieldValues))
			for j, v := range c.FieldValues {
				values[j] = MergeConflictField{
					Field:       v.Field,
					Label:       v.Label,
					BaseValue:   v.BaseValue,
					BranchValue: v.BranchValue,
					MainValue:   v.MainValue,
				}
				if v.BaseUnknown {
					unknown := true
					values[j].BaseUnknown = &unknown
				}
			}
			out[i].FieldValues = &values
		}
	}
	return out
}

// addMergeState fills a merged branch's merge_state and, when the merge did
// not finish, merge_pending (#830). The command computes it without writing;
// any other branch is left without either field.
//
// A merge state that cannot be read never fails the request: the branch is
// reported with merge_state "unknown" and the error is logged. One merged
// branch in a shape the resume refuses must not take down the branch list
// (and with it the branch switcher) or that branch's own page, from which the
// user can still run the resume that names the problem.
func (ss *StrictServer) addMergeState(ctx context.Context, out *Branch, b *domain.Branch) {
	if b == nil || b.Status != domain.BranchStatusMerged {
		return
	}
	completeness, err := ss.server.commandHandler.MergeCompleteness(ctx, b)
	if err != nil {
		ss.server.echo.Logger.Errorf("reading the merge state of branch %s: %v", b.ID, err)
		unknown := MergeStateUnknown
		out.MergeState = &unknown
		return
	}
	if completeness == nil {
		return
	}
	state := BranchMergeState(completeness.State)
	out.MergeState = &state
	if completeness.State == command.MergeStateIncomplete {
		pending := convertPendingMergeEntities(completeness.Pending)
		out.MergePending = &pending
	}
}

// convertPendingMergeEntities converts an incomplete merge's pending entities,
// always returning a non-nil slice so the payload carries [] rather than null.
func convertPendingMergeEntities(entities []command.PendingMergeEntity) []MergePendingEntity {
	out := make([]MergePendingEntity, len(entities))
	for i, e := range entities {
		supported := e.Reason.SupportedResolutions()
		resolutions := make([]MergePendingEntitySupportedResolutions, len(supported))
		for j, r := range supported {
			resolutions[j] = MergePendingEntitySupportedResolutions(r)
		}
		out[i] = MergePendingEntity{
			StreamId:             e.StreamID,
			EntityType:           e.EntityType,
			EntityName:           e.EntityName,
			Reason:               MergePendingEntityReason(e.Reason),
			NeedsResolution:      e.Reason.NeedsResolution(),
			SupportedResolutions: resolutions,
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
	// The close record (#836), present once the branch is closed.
	if b.ClosedAt != nil {
		closedAt := *b.ClosedAt
		branch.ClosedAt = &closedAt
	}
	if b.CloseReason != "" {
		closeReason := b.CloseReason
		branch.CloseReason = &closeReason
	}
	// The research record (#835). The arrays are always present, [] when
	// empty, so a client never has to tell null from none.
	if b.Hypothesis != "" {
		hypothesis := b.Hypothesis
		branch.Hypothesis = &hypothesis
	}
	branch.Outcome = BranchOutcome(b.Outcome.OrDefault())
	branch.Subjects = make([]BranchSubject, len(b.Subjects))
	for i, subject := range b.Subjects {
		branch.Subjects[i] = BranchSubject{Type: BranchSubjectType(subject.Type), Id: subject.ID}
	}
	branch.ProofSummaryIds = append([]openapi_types.UUID{}, b.ProofSummaryIDs...)
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
