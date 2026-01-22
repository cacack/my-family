package query

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// SubmitterService provides query operations for submitters.
type SubmitterService struct {
	readStore repository.ReadModelStore
}

// NewSubmitterService creates a new submitter query service.
func NewSubmitterService(readStore repository.ReadModelStore) *SubmitterService {
	return &SubmitterService{readStore: readStore}
}

// Submitter represents a submitter in query results.
type Submitter struct {
	ID         uuid.UUID       `json:"id"`
	Name       string          `json:"name"`
	Address    *domain.Address `json:"address,omitempty"`
	Phone      []string        `json:"phone,omitempty"`
	Email      []string        `json:"email,omitempty"`
	Language   *string         `json:"language,omitempty"`
	MediaID    *uuid.UUID      `json:"media_id,omitempty"`
	GedcomXref *string         `json:"gedcom_xref,omitempty"`
	Version    int64           `json:"version"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// ListSubmittersInput contains options for listing submitters.
type ListSubmittersInput struct {
	Limit     int
	Offset    int
	Sort      string // name, updated_at
	SortOrder string // asc, desc
}

// SubmitterListResult contains paginated submitter results.
type SubmitterListResult struct {
	Submitters []Submitter `json:"submitters"`
	Total      int         `json:"total"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
}

// ListSubmitters returns a paginated list of submitters.
func (s *SubmitterService) ListSubmitters(ctx context.Context, input ListSubmittersInput) (*SubmitterListResult, error) {
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

	readModels, total, err := s.readStore.ListSubmitters(ctx, opts)
	if err != nil {
		return nil, err
	}

	submitters := make([]Submitter, len(readModels))
	for i, rm := range readModels {
		submitters[i] = convertReadModelToSubmitter(rm)
	}

	return &SubmitterListResult{
		Submitters: submitters,
		Total:      total,
		Limit:      opts.Limit,
		Offset:     opts.Offset,
	}, nil
}

// GetSubmitter returns a submitter by ID.
func (s *SubmitterService) GetSubmitter(ctx context.Context, id uuid.UUID) (*Submitter, error) {
	rm, err := s.readStore.GetSubmitter(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	submitter := convertReadModelToSubmitter(*rm)
	return &submitter, nil
}

// Helper function to convert read model to query result.
func convertReadModelToSubmitter(rm repository.SubmitterReadModel) Submitter {
	sub := Submitter{
		ID:        rm.ID,
		Name:      rm.Name,
		Address:   rm.Address,
		Phone:     rm.Phone,
		Email:     rm.Email,
		MediaID:   rm.MediaID,
		Version:   rm.Version,
		UpdatedAt: rm.UpdatedAt,
	}

	if rm.GedcomXref != "" {
		sub.GedcomXref = &rm.GedcomXref
	}
	if rm.Language != "" {
		sub.Language = &rm.Language
	}

	return sub
}
