// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ErrNotFound is returned when a requested entity doesn't exist.
var ErrNotFound = errors.New("not found")

// PersonService provides query operations for persons.
type PersonService struct {
	readStore repository.ReadModelStore
}

// NewPersonService creates a new person query service.
func NewPersonService(readStore repository.ReadModelStore) *PersonService {
	return &PersonService{readStore: readStore}
}

// Person represents a person in query results.
type Person struct {
	ID             uuid.UUID       `json:"id"`
	GivenName      string          `json:"given_name"`
	Surname        string          `json:"surname"`
	Gender         *string         `json:"gender,omitempty"`
	BirthDate      *domain.GenDate `json:"birth_date,omitempty"`
	BirthPlace     *string         `json:"birth_place,omitempty"`
	DeathDate      *domain.GenDate `json:"death_date,omitempty"`
	DeathPlace     *string         `json:"death_place,omitempty"`
	Notes          *string         `json:"notes,omitempty"`
	ResearchStatus *string         `json:"research_status,omitempty"`
	Version        int64           `json:"version"`
}

// PersonDetail includes family relationships.
type PersonDetail struct {
	Person
	FamiliesAsPartner []FamilySummary `json:"families_as_partner,omitempty"`
	FamilyAsChild     *FamilySummary  `json:"family_as_child,omitempty"`
}

// FamilySummary is a brief family representation.
type FamilySummary struct {
	ID               uuid.UUID `json:"id"`
	Partner1Name     *string   `json:"partner1_name,omitempty"`
	Partner2Name     *string   `json:"partner2_name,omitempty"`
	RelationshipType *string   `json:"relationship_type,omitempty"`
}

// PersonListResult contains paginated person results.
type PersonListResult struct {
	Items  []Person `json:"items"`
	Total  int      `json:"total"`
	Limit  int      `json:"limit"`
	Offset int      `json:"offset"`
}

// ListPersonsInput contains options for listing persons.
type ListPersonsInput struct {
	Limit  int
	Offset int
	Sort   string // surname, given_name, birth_date, updated_at
	Order  string // asc, desc
}

// ListPersons returns a paginated list of persons.
func (s *PersonService) ListPersons(ctx context.Context, input ListPersonsInput) (*PersonListResult, error) {
	opts := repository.ListOptions{
		Limit:  input.Limit,
		Offset: input.Offset,
		Sort:   input.Sort,
		Order:  input.Order,
	}

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if opts.Sort == "" {
		opts.Sort = "surname"
	}
	if opts.Order == "" {
		opts.Order = "asc"
	}

	readModels, total, err := s.readStore.ListPersons(ctx, opts)
	if err != nil {
		return nil, err
	}

	persons := make([]Person, len(readModels))
	for i, rm := range readModels {
		persons[i] = convertReadModelToPerson(rm)
	}

	return &PersonListResult{
		Items:  persons,
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}

// GetPerson returns a person by ID with family relationships.
func (s *PersonService) GetPerson(ctx context.Context, id uuid.UUID) (*PersonDetail, error) {
	rm, err := s.readStore.GetPerson(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	person := convertReadModelToPerson(*rm)
	detail := &PersonDetail{
		Person: person,
	}

	// Get families where person is a partner
	families, err := s.readStore.GetFamiliesForPerson(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, f := range families {
		detail.FamiliesAsPartner = append(detail.FamiliesAsPartner, convertToFamilySummary(f))
	}

	// Get family where person is a child
	childFamily, err := s.readStore.GetChildFamily(ctx, id)
	if err != nil {
		return nil, err
	}
	if childFamily != nil {
		summary := convertToFamilySummary(*childFamily)
		detail.FamilyAsChild = &summary
	}

	return detail, nil
}

// SearchPersonsInput contains options for searching persons.
type SearchPersonsInput struct {
	Query string
	Fuzzy bool
	Limit int
}

// SearchResult represents a search result with relevance score.
type SearchResult struct {
	Person
	Score float64 `json:"score"`
}

// SearchPersonsResult contains search results.
type SearchPersonsResult struct {
	Items []SearchResult `json:"items"`
	Total int            `json:"total"`
	Query string         `json:"query"`
}

// SearchPersons searches for persons by name.
func (s *PersonService) SearchPersons(ctx context.Context, input SearchPersonsInput) (*SearchPersonsResult, error) {
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}

	readModels, err := s.readStore.SearchPersons(ctx, input.Query, input.Fuzzy, input.Limit)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, len(readModels))
	for i, rm := range readModels {
		results[i] = SearchResult{
			Person: convertReadModelToPerson(rm),
			Score:  1.0, // In-memory search doesn't have scoring; SQLite/PostgreSQL would provide this
		}
	}

	return &SearchPersonsResult{
		Items: results,
		Total: len(results),
		Query: input.Query,
	}, nil
}

// Helper function to convert read model to query result.
func convertReadModelToPerson(rm repository.PersonReadModel) Person {
	p := Person{
		ID:        rm.ID,
		GivenName: rm.GivenName,
		Surname:   rm.Surname,
		Version:   rm.Version,
	}

	if rm.Gender != "" {
		g := string(rm.Gender)
		p.Gender = &g
	}
	if rm.BirthDateRaw != "" {
		gd := domain.ParseGenDate(rm.BirthDateRaw)
		p.BirthDate = &gd
	}
	if rm.BirthPlace != "" {
		p.BirthPlace = &rm.BirthPlace
	}
	if rm.DeathDateRaw != "" {
		gd := domain.ParseGenDate(rm.DeathDateRaw)
		p.DeathDate = &gd
	}
	if rm.DeathPlace != "" {
		p.DeathPlace = &rm.DeathPlace
	}
	if rm.Notes != "" {
		p.Notes = &rm.Notes
	}
	if rm.ResearchStatus != "" {
		rs := string(rm.ResearchStatus)
		p.ResearchStatus = &rs
	}

	return p
}

func convertToFamilySummary(rm repository.FamilyReadModel) FamilySummary {
	s := FamilySummary{
		ID: rm.ID,
	}
	if rm.Partner1Name != "" {
		s.Partner1Name = &rm.Partner1Name
	}
	if rm.Partner2Name != "" {
		s.Partner2Name = &rm.Partner2Name
	}
	if rm.RelationshipType != "" {
		rt := string(rm.RelationshipType)
		s.RelationshipType = &rt
	}
	return s
}

// GenDateToSortTime converts a GenDate to a sortable time for queries.
func GenDateToSortTime(gd *domain.GenDate) *time.Time {
	if gd == nil || gd.IsEmpty() {
		return nil
	}
	t := gd.ToTime()
	if t.IsZero() {
		return nil
	}
	return &t
}
