package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// FamilyService provides query operations for families.
type FamilyService struct {
	readStore repository.ReadModelStore
}

// NewFamilyService creates a new family query service.
func NewFamilyService(readStore repository.ReadModelStore) *FamilyService {
	return &FamilyService{readStore: readStore}
}

// Family represents a family in query results.
type Family struct {
	ID               uuid.UUID          `json:"id"`
	Partner1ID       *uuid.UUID         `json:"partner1_id,omitempty"`
	Partner1Name     *string            `json:"partner1_name,omitempty"`
	Partner2ID       *uuid.UUID         `json:"partner2_id,omitempty"`
	Partner2Name     *string            `json:"partner2_name,omitempty"`
	RelationshipType *string            `json:"relationship_type,omitempty"`
	MarriageDate     *domain.GenDate    `json:"marriage_date,omitempty"`
	MarriagePlace    *string            `json:"marriage_place,omitempty"`
	ChildCount       int                `json:"child_count"`
	Version          int64              `json:"version"`
}

// FamilyDetail includes children information.
type FamilyDetail struct {
	Family
	Children []FamilyChildInfo `json:"children,omitempty"`
}

// FamilyChildInfo represents a child in a family.
type FamilyChildInfo struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	RelationshipType string    `json:"relationship_type"`
}

// FamilyListResult contains paginated family results.
type FamilyListResult struct {
	Items  []Family `json:"items"`
	Total  int      `json:"total"`
	Limit  int      `json:"limit"`
	Offset int      `json:"offset"`
}

// ListFamiliesInput contains options for listing families.
type ListFamiliesInput struct {
	Limit  int
	Offset int
}

// ListFamilies returns a paginated list of families.
func (s *FamilyService) ListFamilies(ctx context.Context, input ListFamiliesInput) (*FamilyListResult, error) {
	opts := repository.ListOptions{
		Limit:  input.Limit,
		Offset: input.Offset,
	}

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}

	readModels, total, err := s.readStore.ListFamilies(ctx, opts)
	if err != nil {
		return nil, err
	}

	families := make([]Family, len(readModels))
	for i, rm := range readModels {
		families[i] = convertReadModelToFamily(rm)
	}

	return &FamilyListResult{
		Items:  families,
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}

// GetFamily returns a family by ID with children.
func (s *FamilyService) GetFamily(ctx context.Context, id uuid.UUID) (*FamilyDetail, error) {
	rm, err := s.readStore.GetFamily(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	family := convertReadModelToFamily(*rm)
	detail := &FamilyDetail{
		Family: family,
	}

	// Get children
	children, err := s.readStore.GetFamilyChildren(ctx, id)
	if err != nil {
		return nil, err
	}

	for _, c := range children {
		detail.Children = append(detail.Children, FamilyChildInfo{
			ID:               c.PersonID,
			Name:             c.PersonName,
			RelationshipType: string(c.RelationshipType),
		})
	}

	return detail, nil
}

// GetFamiliesForPerson returns all families where a person is a partner.
func (s *FamilyService) GetFamiliesForPerson(ctx context.Context, personID uuid.UUID) ([]Family, error) {
	readModels, err := s.readStore.GetFamiliesForPerson(ctx, personID)
	if err != nil {
		return nil, err
	}

	families := make([]Family, len(readModels))
	for i, rm := range readModels {
		families[i] = convertReadModelToFamily(rm)
	}

	return families, nil
}

// Helper function to convert read model to query result.
func convertReadModelToFamily(rm repository.FamilyReadModel) Family {
	f := Family{
		ID:         rm.ID,
		Partner1ID: rm.Partner1ID,
		Partner2ID: rm.Partner2ID,
		ChildCount: rm.ChildCount,
		Version:    rm.Version,
	}

	if rm.Partner1Name != "" {
		f.Partner1Name = &rm.Partner1Name
	}
	if rm.Partner2Name != "" {
		f.Partner2Name = &rm.Partner2Name
	}
	if rm.RelationshipType != "" {
		rt := string(rm.RelationshipType)
		f.RelationshipType = &rt
	}
	if rm.MarriageDateRaw != "" {
		gd := domain.ParseGenDate(rm.MarriageDateRaw)
		f.MarriageDate = &gd
	}
	if rm.MarriagePlace != "" {
		f.MarriagePlace = &rm.MarriagePlace
	}

	return f
}
