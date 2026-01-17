package api

import (
	"context"
	"errors"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ============================================================================
// Snapshot endpoints
// ============================================================================

// ListSnapshots implements StrictServerInterface.
func (ss *StrictServer) ListSnapshots(ctx context.Context, _ ListSnapshotsRequestObject) (ListSnapshotsResponseObject, error) {
	snapshots, err := ss.server.snapshotService.ListSnapshots(ctx)
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

	name := request.Body.Name
	description := ""
	if request.Body.Description != nil {
		description = *request.Body.Description
	}

	snapshot, err := ss.server.snapshotService.CreateSnapshot(ctx, name, description)
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
	snapshot, err := ss.server.snapshotService.GetSnapshot(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrSnapshotNotFound) {
			return GetSnapshot404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Snapshot not found",
			}}, nil
		}
		return nil, err
	}

	return GetSnapshot200JSONResponse(convertDomainSnapshotToGenerated(snapshot)), nil
}

// DeleteSnapshot implements StrictServerInterface.
func (ss *StrictServer) DeleteSnapshot(ctx context.Context, request DeleteSnapshotRequestObject) (DeleteSnapshotResponseObject, error) {
	err := ss.server.snapshotService.DeleteSnapshot(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrSnapshotNotFound) {
			return DeleteSnapshot404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Snapshot not found",
			}}, nil
		}
		return nil, err
	}

	return DeleteSnapshot204Response{}, nil
}

// CompareSnapshots implements StrictServerInterface.
func (ss *StrictServer) CompareSnapshots(ctx context.Context, request CompareSnapshotsRequestObject) (CompareSnapshotsResponseObject, error) {
	result, err := ss.server.snapshotService.CompareSnapshots(ctx, request.Id1, request.Id2)
	if err != nil {
		if errors.Is(err, repository.ErrSnapshotNotFound) {
			return CompareSnapshots404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Snapshot not found",
			}}, nil
		}
		return nil, err
	}

	// Convert changes to generated type
	changes := make([]ChangeEntry, len(result.Changes))
	for i, c := range result.Changes {
		changes[i] = convertQueryChangeEntryToGenerated(c)
	}

	return CompareSnapshots200JSONResponse{
		Snapshot1:  convertDomainSnapshotToGenerated(result.Snapshot1),
		Snapshot2:  convertDomainSnapshotToGenerated(result.Snapshot2),
		Changes:    changes,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
		OlderFirst: result.OlderFirst,
	}, nil
}

// convertDomainSnapshotToGenerated converts a domain.Snapshot to the generated Snapshot type.
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
	return snapshot
}
