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
	ID               uuid.UUID       `json:"id"`
	Partner1ID       *uuid.UUID      `json:"partner1_id,omitempty"`
	Partner1Name     *string         `json:"partner1_name,omitempty"`
	Partner2ID       *uuid.UUID      `json:"partner2_id,omitempty"`
	Partner2Name     *string         `json:"partner2_name,omitempty"`
	RelationshipType *string         `json:"relationship_type,omitempty"`
	MarriageDate     *domain.GenDate `json:"marriage_date,omitempty"`
	MarriagePlace    *string         `json:"marriage_place,omitempty"`
	ChildCount       int             `json:"child_count"`
	Version          int64           `json:"version"`
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

// GroupSheet types for family group sheet view

// GroupSheetEvent represents an event with date, place, and citations.
type GroupSheetEvent struct {
	Date       string               `json:"date,omitempty"`
	Place      string               `json:"place,omitempty"`
	IsNegated  bool                 `json:"is_negated,omitempty"`
	Citations  []GroupSheetCitation `json:"citations,omitempty"`
}

// GroupSheetCitation represents a citation reference for the group sheet.
type GroupSheetCitation struct {
	ID          uuid.UUID `json:"id"`
	SourceID    uuid.UUID `json:"source_id"`
	SourceTitle string    `json:"source_title"`
	Page        string    `json:"page,omitempty"`
	Detail      string    `json:"detail,omitempty"`
}

// GroupSheetPerson represents a person in the group sheet with events.
type GroupSheetPerson struct {
	ID         uuid.UUID        `json:"id"`
	GivenName  string           `json:"given_name"`
	Surname    string           `json:"surname"`
	Gender     string           `json:"gender,omitempty"`
	Birth      *GroupSheetEvent `json:"birth,omitempty"`
	Death      *GroupSheetEvent `json:"death,omitempty"`
	FatherName string           `json:"father_name,omitempty"`
	FatherID   *uuid.UUID       `json:"father_id,omitempty"`
	MotherName string           `json:"mother_name,omitempty"`
	MotherID   *uuid.UUID       `json:"mother_id,omitempty"`
}

// GroupSheetChild represents a child entry in the group sheet.
type GroupSheetChild struct {
	ID               uuid.UUID        `json:"id"`
	GivenName        string           `json:"given_name"`
	Surname          string           `json:"surname"`
	Gender           string           `json:"gender,omitempty"`
	RelationshipType string           `json:"relationship_type,omitempty"`
	Sequence         *int             `json:"sequence,omitempty"`
	Birth            *GroupSheetEvent `json:"birth,omitempty"`
	Death            *GroupSheetEvent `json:"death,omitempty"`
	SpouseName       string           `json:"spouse_name,omitempty"`
	SpouseID         *uuid.UUID       `json:"spouse_id,omitempty"`
}

// GroupSheet represents a traditional family group sheet.
type GroupSheet struct {
	ID       uuid.UUID         `json:"id"`
	Husband  *GroupSheetPerson `json:"husband,omitempty"`
	Wife     *GroupSheetPerson `json:"wife,omitempty"`
	Marriage *GroupSheetEvent  `json:"marriage,omitempty"`
	Children []GroupSheetChild `json:"children,omitempty"`
}

// GetGroupSheet returns a family group sheet with full details.
func (s *FamilyService) GetGroupSheet(ctx context.Context, familyID uuid.UUID) (*GroupSheet, error) {
	// Get family details
	family, err := s.readStore.GetFamily(ctx, familyID)
	if err != nil {
		return nil, err
	}
	if family == nil {
		return nil, ErrNotFound
	}

	gs := &GroupSheet{
		ID: family.ID,
	}

	// Get marriage event
	if family.MarriageDateRaw != "" || family.MarriagePlace != "" {
		gs.Marriage = &GroupSheetEvent{
			Date:  family.MarriageDateRaw,
			Place: family.MarriagePlace,
		}
		// Get marriage citations
		citations, err := s.readStore.GetCitationsForFact(ctx, domain.FactFamilyMarriage, familyID)
		if err == nil && len(citations) > 0 {
			gs.Marriage.Citations = convertCitationsToGroupSheet(citations)
		}
	}

	// Check for negated marriage events
	s.applyNegatedFamilyEvents(ctx, familyID, &gs.Marriage)

	// Get husband/partner1 details
	if family.Partner1ID != nil {
		husband, err := s.getGroupSheetPerson(ctx, *family.Partner1ID)
		if err == nil {
			gs.Husband = husband
		}
	}

	// Get wife/partner2 details
	if family.Partner2ID != nil {
		wife, err := s.getGroupSheetPerson(ctx, *family.Partner2ID)
		if err == nil {
			gs.Wife = wife
		}
	}

	// Get children
	children, err := s.readStore.GetFamilyChildren(ctx, familyID)
	if err == nil {
		for _, child := range children {
			gsChild, err := s.getGroupSheetChild(ctx, child)
			if err == nil {
				gs.Children = append(gs.Children, *gsChild)
			}
		}
	}

	return gs, nil
}

// getGroupSheetPerson builds a GroupSheetPerson from a person ID.
func (s *FamilyService) getGroupSheetPerson(ctx context.Context, personID uuid.UUID) (*GroupSheetPerson, error) {
	person, err := s.readStore.GetPerson(ctx, personID)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return nil, ErrNotFound
	}

	gsp := &GroupSheetPerson{
		ID:        person.ID,
		GivenName: person.GivenName,
		Surname:   person.Surname,
		Gender:    string(person.Gender),
	}

	// Birth event
	if person.BirthDateRaw != "" || person.BirthPlace != "" {
		gsp.Birth = &GroupSheetEvent{
			Date:  person.BirthDateRaw,
			Place: person.BirthPlace,
		}
		// Get birth citations
		citations, err := s.readStore.GetCitationsForFact(ctx, domain.FactPersonBirth, personID)
		if err == nil && len(citations) > 0 {
			gsp.Birth.Citations = convertCitationsToGroupSheet(citations)
		}
	}

	// Death event
	if person.DeathDateRaw != "" || person.DeathPlace != "" {
		gsp.Death = &GroupSheetEvent{
			Date:  person.DeathDateRaw,
			Place: person.DeathPlace,
		}
		// Get death citations
		citations, err := s.readStore.GetCitationsForFact(ctx, domain.FactPersonDeath, personID)
		if err == nil && len(citations) > 0 {
			gsp.Death.Citations = convertCitationsToGroupSheet(citations)
		}
	}

	// Check for negated birth/death events
	s.applyNegatedPersonEvents(ctx, personID, &gsp.Birth, &gsp.Death)

	// Get parents
	edge, err := s.readStore.GetPedigreeEdge(ctx, personID)
	if err == nil && edge != nil {
		if edge.FatherID != nil {
			gsp.FatherID = edge.FatherID
			gsp.FatherName = edge.FatherName
		}
		if edge.MotherID != nil {
			gsp.MotherID = edge.MotherID
			gsp.MotherName = edge.MotherName
		}
	}

	return gsp, nil
}

// getGroupSheetChild builds a GroupSheetChild from a FamilyChildReadModel.
func (s *FamilyService) getGroupSheetChild(ctx context.Context, child repository.FamilyChildReadModel) (*GroupSheetChild, error) {
	person, err := s.readStore.GetPerson(ctx, child.PersonID)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return nil, ErrNotFound
	}

	gsc := &GroupSheetChild{
		ID:               person.ID,
		GivenName:        person.GivenName,
		Surname:          person.Surname,
		Gender:           string(person.Gender),
		RelationshipType: string(child.RelationshipType),
		Sequence:         child.Sequence,
	}

	// Birth event
	if person.BirthDateRaw != "" || person.BirthPlace != "" {
		gsc.Birth = &GroupSheetEvent{
			Date:  person.BirthDateRaw,
			Place: person.BirthPlace,
		}
		// Get birth citations
		citations, err := s.readStore.GetCitationsForFact(ctx, domain.FactPersonBirth, person.ID)
		if err == nil && len(citations) > 0 {
			gsc.Birth.Citations = convertCitationsToGroupSheet(citations)
		}
	}

	// Death event
	if person.DeathDateRaw != "" || person.DeathPlace != "" {
		gsc.Death = &GroupSheetEvent{
			Date:  person.DeathDateRaw,
			Place: person.DeathPlace,
		}
		// Get death citations
		citations, err := s.readStore.GetCitationsForFact(ctx, domain.FactPersonDeath, person.ID)
		if err == nil && len(citations) > 0 {
			gsc.Death.Citations = convertCitationsToGroupSheet(citations)
		}
	}

	// Check for negated birth/death events
	s.applyNegatedPersonEvents(ctx, person.ID, &gsc.Birth, &gsc.Death)

	// Get spouse (first partner family where this person is a partner)
	families, err := s.readStore.GetFamiliesForPerson(ctx, person.ID)
	if err == nil && len(families) > 0 {
		for _, fam := range families {
			// Find the other partner
			if fam.Partner1ID != nil && *fam.Partner1ID != person.ID {
				gsc.SpouseID = fam.Partner1ID
				gsc.SpouseName = fam.Partner1Name
				break
			}
			if fam.Partner2ID != nil && *fam.Partner2ID != person.ID {
				gsc.SpouseID = fam.Partner2ID
				gsc.SpouseName = fam.Partner2Name
				break
			}
		}
	}

	return gsc, nil
}

// applyNegatedPersonEvents checks the events table for negated birth/death events
// and applies them to the group sheet person's events. This handles the case where
// a negative assertion exists (e.g., "no birth recorded") but the person read model
// has no birth date/place.
func (s *FamilyService) applyNegatedPersonEvents(ctx context.Context, personID uuid.UUID, birth **GroupSheetEvent, death **GroupSheetEvent) {
	events, err := s.readStore.ListEventsForPerson(ctx, personID)
	if err != nil {
		return
	}
	for _, evt := range events {
		if !evt.IsNegated {
			continue
		}
		switch evt.FactType {
		case domain.FactPersonBirth:
			if *birth == nil {
				*birth = &GroupSheetEvent{IsNegated: true}
			} else {
				(*birth).IsNegated = true
			}
		case domain.FactPersonDeath:
			if *death == nil {
				*death = &GroupSheetEvent{IsNegated: true}
			} else {
				(*death).IsNegated = true
			}
		}
	}
}

// applyNegatedFamilyEvents checks the events table for negated marriage events
// and applies them to the group sheet marriage event.
func (s *FamilyService) applyNegatedFamilyEvents(ctx context.Context, familyID uuid.UUID, marriage **GroupSheetEvent) {
	events, err := s.readStore.ListEventsForFamily(ctx, familyID)
	if err != nil {
		return
	}
	for _, evt := range events {
		if !evt.IsNegated {
			continue
		}
		if evt.FactType == domain.FactFamilyMarriage {
			if *marriage == nil {
				*marriage = &GroupSheetEvent{IsNegated: true}
			} else {
				(*marriage).IsNegated = true
			}
		}
	}
}

// convertCitationsToGroupSheet converts citation read models to group sheet citations.
func convertCitationsToGroupSheet(citations []repository.CitationReadModel) []GroupSheetCitation {
	result := make([]GroupSheetCitation, len(citations))
	for i, c := range citations {
		result[i] = GroupSheetCitation{
			ID:          c.ID,
			SourceID:    c.SourceID,
			SourceTitle: c.SourceTitle,
			Page:        c.Page,
		}
		// Build detail string
		var detail string
		if c.Page != "" {
			detail = "p. " + c.Page
		}
		if c.Volume != "" {
			if detail != "" {
				detail += ", "
			}
			detail += "vol. " + c.Volume
		}
		result[i].Detail = detail
	}
	return result
}
