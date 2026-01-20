package query

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// LDSOrdinanceService provides query operations for LDS ordinances.
type LDSOrdinanceService struct {
	readStore repository.ReadModelStore
}

// NewLDSOrdinanceService creates a new LDS ordinance query service.
func NewLDSOrdinanceService(readStore repository.ReadModelStore) *LDSOrdinanceService {
	return &LDSOrdinanceService{readStore: readStore}
}

// LDSOrdinance represents an LDS ordinance in query results.
type LDSOrdinance struct {
	ID         uuid.UUID               `json:"id"`
	Type       domain.LDSOrdinanceType `json:"type"`
	TypeLabel  string                  `json:"type_label"`
	PersonID   *uuid.UUID              `json:"person_id,omitempty"`
	PersonName string                  `json:"person_name,omitempty"`
	FamilyID   *uuid.UUID              `json:"family_id,omitempty"`
	Date       *domain.GenDate         `json:"date,omitempty"`
	Place      string                  `json:"place,omitempty"`
	Temple     string                  `json:"temple,omitempty"`
	Status     string                  `json:"status,omitempty"`
	Version    int64                   `json:"version"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

// ListLDSOrdinancesInput contains options for listing LDS ordinances.
type ListLDSOrdinancesInput struct {
	Limit     int
	Offset    int
	Sort      string // type, date, updated_at
	SortOrder string // asc, desc
}

// LDSOrdinanceListResult contains paginated LDS ordinance results.
type LDSOrdinanceListResult struct {
	LDSOrdinances []LDSOrdinance `json:"lds_ordinances"`
	Total         int            `json:"total"`
	Limit         int            `json:"limit"`
	Offset        int            `json:"offset"`
}

// ListLDSOrdinances returns a paginated list of LDS ordinances.
func (s *LDSOrdinanceService) ListLDSOrdinances(ctx context.Context, input ListLDSOrdinancesInput) (*LDSOrdinanceListResult, error) {
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

	readModels, total, err := s.readStore.ListLDSOrdinances(ctx, opts)
	if err != nil {
		return nil, err
	}

	ordinances := make([]LDSOrdinance, len(readModels))
	for i, rm := range readModels {
		ordinances[i] = convertReadModelToLDSOrdinance(rm)
	}

	return &LDSOrdinanceListResult{
		LDSOrdinances: ordinances,
		Total:         total,
		Limit:         opts.Limit,
		Offset:        opts.Offset,
	}, nil
}

// GetLDSOrdinance returns an LDS ordinance by ID.
func (s *LDSOrdinanceService) GetLDSOrdinance(ctx context.Context, id uuid.UUID) (*LDSOrdinance, error) {
	rm, err := s.readStore.GetLDSOrdinance(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	ordinance := convertReadModelToLDSOrdinance(*rm)
	return &ordinance, nil
}

// ListLDSOrdinancesForPerson returns all LDS ordinances for a given person.
func (s *LDSOrdinanceService) ListLDSOrdinancesForPerson(ctx context.Context, personID uuid.UUID) ([]LDSOrdinance, error) {
	// Verify person exists
	person, err := s.readStore.GetPerson(ctx, personID)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return nil, ErrNotFound
	}

	readModels, err := s.readStore.ListLDSOrdinancesForPerson(ctx, personID)
	if err != nil {
		return nil, err
	}

	ordinances := make([]LDSOrdinance, len(readModels))
	for i, rm := range readModels {
		ordinances[i] = convertReadModelToLDSOrdinance(rm)
	}

	return ordinances, nil
}

// ListLDSOrdinancesForFamily returns all LDS ordinances for a given family.
func (s *LDSOrdinanceService) ListLDSOrdinancesForFamily(ctx context.Context, familyID uuid.UUID) ([]LDSOrdinance, error) {
	// Verify family exists
	family, err := s.readStore.GetFamily(ctx, familyID)
	if err != nil {
		return nil, err
	}
	if family == nil {
		return nil, ErrNotFound
	}

	readModels, err := s.readStore.ListLDSOrdinancesForFamily(ctx, familyID)
	if err != nil {
		return nil, err
	}

	ordinances := make([]LDSOrdinance, len(readModels))
	for i, rm := range readModels {
		ordinances[i] = convertReadModelToLDSOrdinance(rm)
	}

	return ordinances, nil
}

// Helper function to convert read model to query result.
func convertReadModelToLDSOrdinance(rm repository.LDSOrdinanceReadModel) LDSOrdinance {
	o := LDSOrdinance{
		ID:         rm.ID,
		Type:       rm.Type,
		TypeLabel:  rm.TypeLabel,
		PersonID:   rm.PersonID,
		PersonName: rm.PersonName,
		FamilyID:   rm.FamilyID,
		Place:      rm.Place,
		Temple:     rm.Temple,
		Status:     rm.Status,
		Version:    rm.Version,
		UpdatedAt:  rm.UpdatedAt,
	}

	// Convert date
	if rm.DateRaw != "" {
		gd := domain.ParseGenDate(rm.DateRaw)
		o.Date = &gd
	}

	return o
}
