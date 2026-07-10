package query

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// RepositoryService provides query operations for repositories.
type RepositoryService struct {
	readStore repository.ReadModelStore
}

// NewRepositoryService creates a new repository query service.
func NewRepositoryService(readStore repository.ReadModelStore) *RepositoryService {
	return &RepositoryService{readStore: readStore}
}

// Repository represents a repository in query results.
type Repository struct {
	ID         uuid.UUID       `json:"id"`
	Name       string          `json:"name"`
	Address    *domain.Address `json:"address,omitempty"`
	Notes      *string         `json:"notes,omitempty"`
	GedcomXref *string         `json:"gedcom_xref,omitempty"`
	Version    int64           `json:"version"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// RepositoryDetail includes GEDCOM 7.0 external identifiers.
type RepositoryDetail struct {
	Repository
	ExternalIDs []domain.ExternalIdentifier `json:"external_ids,omitempty"`
}

// ListRepositoriesInput contains options for listing repositories.
type ListRepositoriesInput struct {
	Limit     int
	Offset    int
	Sort      string // name, updated_at
	SortOrder string // asc, desc
}

// RepositoryListResult contains paginated repository results.
type RepositoryListResult struct {
	Repositories []Repository `json:"repositories"`
	Total        int          `json:"total"`
	Limit        int          `json:"limit"`
	Offset       int          `json:"offset"`
}

// ListRepositories returns a paginated list of repositories.
func (s *RepositoryService) ListRepositories(ctx context.Context, input ListRepositoriesInput) (*RepositoryListResult, error) {
	opts := repository.ListOptions{
		Limit:  input.Limit,
		Offset: input.Offset,
		Sort:   input.Sort,
		Order:  input.SortOrder,
	}

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if opts.Order == "" {
		opts.Order = "desc"
	}

	readModels, total, err := s.readStore.ListRepositories(ctx, opts)
	if err != nil {
		return nil, err
	}

	repositories := make([]Repository, len(readModels))
	for i, rm := range readModels {
		repositories[i] = convertReadModelToRepository(rm)
	}

	return &RepositoryListResult{
		Repositories: repositories,
		Total:        total,
		Limit:        opts.Limit,
		Offset:       opts.Offset,
	}, nil
}

// GetRepository returns a repository by ID with its external identifiers.
func (s *RepositoryService) GetRepository(ctx context.Context, id uuid.UUID) (*RepositoryDetail, error) {
	rm, err := s.readStore.GetRepository(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	detail := &RepositoryDetail{
		Repository: convertReadModelToRepository(*rm),
	}

	// Get GEDCOM 7.0 external identifiers (EXID) so the UI can render
	// "View on <system>" links.
	externalIDs, err := s.readStore.GetRepositoryExternalIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, ext := range externalIDs {
		detail.ExternalIDs = append(detail.ExternalIDs, domain.ExternalIdentifier{
			Value: ext.Value,
			Type:  ext.Type,
		})
	}

	return detail, nil
}

// Helper function to convert read model to query result.
func convertReadModelToRepository(rm repository.RepositoryReadModel) Repository {
	repo := Repository{
		ID:        rm.ID,
		Name:      rm.Name,
		Address:   rm.Address,
		Version:   rm.Version,
		UpdatedAt: rm.UpdatedAt,
	}

	if rm.Notes != "" {
		repo.Notes = &rm.Notes
	}
	if rm.GedcomXref != "" {
		repo.GedcomXref = &rm.GedcomXref
	}

	return repo
}
