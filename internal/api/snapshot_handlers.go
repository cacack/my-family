package api

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// ============================================================================
// Snapshot endpoints
//
// Every snapshot operation is branch-scoped (issue #839): a snapshot marks
// (branch_id, position) in ADR-005's model, so the list, get and delete answer
// only for the requested scope's snapshots, creation marks the scope's view,
// and a comparison reads the scope's view of the log. Comparing snapshots from
// different branches is refused with 409 snapshot_branch_mismatch.
// ============================================================================

// errSnapshotBranchMismatch is the body of a refused cross-branch comparison.
var errSnapshotBranchMismatch = Error{
	Code: "snapshot_branch_mismatch",
	Message: "Snapshots can only be compared within the branch they were taken on; " +
		"switch to that branch (or the mainline) to compare them",
}

// snapshotNotFound is the standard 404 body for an unknown (or out-of-scope) snapshot.
var snapshotNotFound = NotFoundJSONResponse{
	Code:    "not_found",
	Message: "Snapshot not found",
}

// ListSnapshots implements StrictServerInterface.
func (ss *StrictServer) ListSnapshots(ctx context.Context, request ListSnapshotsRequestObject) (ListSnapshotsResponseObject, error) {
	branch, err := ss.resolveBranchScope(ctx, request.Params.Branch, branchScopeRead)
	if err != nil {
		return nil, err
	}

	snapshots, err := ss.server.snapshotService.ListSnapshots(ctx, branchScopeID(branch))
	if err != nil {
		return nil, err
	}

	items := make([]Snapshot, len(snapshots))
	for i, s := range snapshots {
		items[i] = convertDomainSnapshotToGenerated(s)
	}

	return ListSnapshots200JSONResponse{
		Items: items,
		Total: len(items),
	}, nil
}

// CreateSnapshot implements StrictServerInterface.
func (ss *StrictServer) CreateSnapshot(ctx context.Context, request CreateSnapshotRequestObject) (CreateSnapshotResponseObject, error) {
	if request.Body == nil {
		return CreateSnapshot400JSONResponse{BadRequestJSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}}, nil
	}

	branch, err := ss.resolveBranchScope(ctx, request.Params.Branch, branchScopeWrite)
	if err != nil {
		return nil, err
	}

	name := request.Body.Name
	description := ""
	if request.Body.Description != nil {
		description = *request.Body.Description
	}

	// Snapshot creation is a command, not a query: it appends SnapshotCreated and
	// the projection writes the registry row (issue #624). On a branch the
	// snapshot marks the branch's view (issue #839).
	snapshot, err := ss.branchWriter(branch).CreateSnapshot(ctx, name, description)
	if err != nil {
		// Check for validation errors
		if errors.Is(err, domain.ErrSnapshotNameRequired) ||
			errors.Is(err, domain.ErrSnapshotNameTooLong) ||
			errors.Is(err, domain.ErrSnapshotDescTooLong) {
			return CreateSnapshot400JSONResponse{BadRequestJSONResponse{
				Code:    "validation_error",
				Message: err.Error(),
			}}, nil
		}
		return nil, err
	}

	return CreateSnapshot201JSONResponse(convertDomainSnapshotToGenerated(snapshot)), nil
}

// GetSnapshot implements StrictServerInterface.
func (ss *StrictServer) GetSnapshot(ctx context.Context, request GetSnapshotRequestObject) (GetSnapshotResponseObject, error) {
	branch, err := ss.resolveBranchScope(ctx, request.Params.Branch, branchScopeRead)
	if err != nil {
		return nil, err
	}

	snapshot, err := ss.server.snapshotService.GetSnapshot(ctx, branchScopeID(branch), request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrSnapshotNotFound) {
			return GetSnapshot404JSONResponse{snapshotNotFound}, nil
		}
		return nil, err
	}

	return GetSnapshot200JSONResponse(convertDomainSnapshotToGenerated(snapshot)), nil
}

// DeleteSnapshot implements StrictServerInterface.
func (ss *StrictServer) DeleteSnapshot(ctx context.Context, request DeleteSnapshotRequestObject) (DeleteSnapshotResponseObject, error) {
	branch, err := ss.resolveBranchScope(ctx, request.Params.Branch, branchScopeWrite)
	if err != nil {
		return nil, err
	}

	// Deletion is a command too: it appends SnapshotDeleted and the projection
	// drops the registry row. The events the snapshot marked are untouched. A
	// snapshot on another branch is not found in this scope.
	if err := ss.branchWriter(branch).DeleteSnapshot(ctx, request.Id); err != nil {
		if errors.Is(err, repository.ErrSnapshotNotFound) {
			return DeleteSnapshot404JSONResponse{snapshotNotFound}, nil
		}
		return nil, err
	}

	return DeleteSnapshot204Response{}, nil
}

// CompareSnapshots implements StrictServerInterface.
func (ss *StrictServer) CompareSnapshots(ctx context.Context, request CompareSnapshotsRequestObject) (CompareSnapshotsResponseObject, error) {
	branch, err := ss.resolveBranchScope(ctx, request.Params.Branch, branchScopeRead)
	if err != nil {
		return nil, err
	}

	result, err := ss.server.snapshotService.CompareSnapshots(ctx, branchScopeID(branch), request.Id1, request.Id2)
	if err != nil {
		if errors.Is(err, repository.ErrSnapshotNotFound) {
			return CompareSnapshots404JSONResponse{snapshotNotFound}, nil
		}
		if errors.Is(err, query.ErrSnapshotBranchMismatch) {
			return CompareSnapshots409JSONResponse(errSnapshotBranchMismatch), nil
		}
		return nil, err
	}

	return CompareSnapshots200JSONResponse{
		Snapshot1:  convertDomainSnapshotToGenerated(result.Snapshot1),
		Snapshot2:  convertDomainSnapshotToGenerated(result.Snapshot2),
		Changes:    convertQueryChangeEntries(result.Changes),
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
		OlderFirst: result.OlderFirst,
	}, nil
}

// CompareSnapshotToCurrent implements StrictServerInterface.
func (ss *StrictServer) CompareSnapshotToCurrent(ctx context.Context, request CompareSnapshotToCurrentRequestObject) (CompareSnapshotToCurrentResponseObject, error) {
	branch, err := ss.resolveBranchScope(ctx, request.Params.Branch, branchScopeRead)
	if err != nil {
		return nil, err
	}

	result, err := ss.server.snapshotService.CompareSnapshotToCurrent(ctx, branchScopeID(branch), request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrSnapshotNotFound) {
			return CompareSnapshotToCurrent404JSONResponse{snapshotNotFound}, nil
		}
		if errors.Is(err, query.ErrSnapshotBranchMismatch) {
			return CompareSnapshotToCurrent409JSONResponse(errSnapshotBranchMismatch), nil
		}
		return nil, err
	}

	return CompareSnapshotToCurrent200JSONResponse{
		Snapshot:     convertDomainSnapshotToGenerated(result.Snapshot),
		HeadPosition: result.HeadPosition,
		Changes:      convertQueryChangeEntries(result.Changes),
		TotalCount:   result.TotalCount,
		HasMore:      result.HasMore,
	}, nil
}

// convertQueryChangeEntries converts comparison change entries to the generated type.
func convertQueryChangeEntries(entries []query.ChangeEntry) []ChangeEntry {
	changes := make([]ChangeEntry, len(entries))
	for i, c := range entries {
		changes[i] = convertQueryChangeEntryToGenerated(c)
	}
	return changes
}

// convertDomainSnapshotToGenerated converts a domain.Snapshot to the generated
// Snapshot type. branch_id is set only for a branch snapshot.
func convertDomainSnapshotToGenerated(s *domain.Snapshot) Snapshot {
	snapshot := Snapshot{
		Id:        s.ID,
		Name:      s.Name,
		Position:  s.Position,
		CreatedAt: s.CreatedAt,
	}
	if s.Description != "" {
		snapshot.Description = &s.Description
	}
	if !s.BranchID.IsMain() {
		branchID := uuid.UUID(s.BranchID)
		snapshot.BranchId = &branchID
	}
	return snapshot
}
