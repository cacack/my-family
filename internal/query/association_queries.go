package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// AssociationService provides query operations for associations.
type AssociationService struct {
	readStore repository.ReadModelStore
}

// NewAssociationService creates a new AssociationService.
func NewAssociationService(readStore repository.ReadModelStore) *AssociationService {
	return &AssociationService{readStore: readStore}
}

// GetAssociation retrieves an association by ID within the branch overlay
// (ADR-005); the zero branchID (MainBranchID) reads the mainline.
func (s *AssociationService) GetAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AssociationReadModel, error) {
	return s.readStore.GetAssociation(ctx, branchID, id)
}

// ListAssociations retrieves a paginated list of all associations visible on
// opts.BranchID.
func (s *AssociationService) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	return s.readStore.ListAssociations(ctx, opts)
}

// ListAssociationsForPerson retrieves all associations involving a given person.
// This includes associations where the person is either the PersonID or AssociateID.
// branchID scopes the lookup to a branch's overlay (ADR-005).
func (s *AssociationService) ListAssociationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.AssociationReadModel, error) {
	return s.readStore.ListAssociationsForPerson(ctx, branchID, personID)
}
