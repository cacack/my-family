package repository

import (
	"context"
	"time"

	"github.com/cacack/my-family/internal/domain"
)

// Projector handles event-to-read-model projections.
type Projector struct {
	readStore ReadModelStore
}

// NewProjector creates a new projector with the given read model store.
func NewProjector(readStore ReadModelStore) *Projector {
	return &Projector{readStore: readStore}
}

// Apply is a convenience method for applying a single event (version is auto-incremented).
func (p *Projector) Apply(ctx context.Context, event domain.Event) error {
	return p.Project(ctx, event, 1) // Version will be updated properly by the caller
}

// Project applies a domain event to the read model.
func (p *Projector) Project(ctx context.Context, event domain.Event, version int64) error {
	switch e := event.(type) {
	case domain.PersonCreated:
		return p.projectPersonCreated(ctx, e, version)
	case domain.PersonUpdated:
		return p.projectPersonUpdated(ctx, e, version)
	case domain.PersonDeleted:
		return p.projectPersonDeleted(ctx, e)
	case domain.FamilyCreated:
		return p.projectFamilyCreated(ctx, e, version)
	case domain.FamilyUpdated:
		return p.projectFamilyUpdated(ctx, e, version)
	case domain.ChildLinkedToFamily:
		return p.projectChildLinked(ctx, e)
	case domain.ChildUnlinkedFromFamily:
		return p.projectChildUnlinked(ctx, e)
	case domain.FamilyDeleted:
		return p.projectFamilyDeleted(ctx, e)
	default:
		// Unknown event types are ignored (forward compatibility)
		return nil
	}
}

func (p *Projector) projectPersonCreated(ctx context.Context, e domain.PersonCreated, version int64) error {
	var birthDateSort, deathDateSort *time.Time
	var birthDateRaw, deathDateRaw string

	if e.BirthDate != nil {
		birthDateRaw = e.BirthDate.Raw
		t := e.BirthDate.ToTime()
		if !t.IsZero() {
			birthDateSort = &t
		}
	}
	if e.DeathDate != nil {
		deathDateRaw = e.DeathDate.Raw
		t := e.DeathDate.ToTime()
		if !t.IsZero() {
			deathDateSort = &t
		}
	}

	person := &PersonReadModel{
		ID:            e.PersonID,
		GivenName:     e.GivenName,
		Surname:       e.Surname,
		FullName:      e.GivenName + " " + e.Surname,
		Gender:        e.Gender,
		BirthDateRaw:  birthDateRaw,
		BirthDateSort: birthDateSort,
		BirthPlace:    e.BirthPlace,
		DeathDateRaw:  deathDateRaw,
		DeathDateSort: deathDateSort,
		DeathPlace:    e.DeathPlace,
		Notes:         e.Notes,
		Version:       version,
		UpdatedAt:     e.OccurredAt(),
	}

	return p.readStore.SavePerson(ctx, person)
}

func (p *Projector) projectPersonUpdated(ctx context.Context, e domain.PersonUpdated, version int64) error {
	person, err := p.readStore.GetPerson(ctx, e.PersonID)
	if err != nil {
		return err
	}
	if person == nil {
		return nil // Person doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "given_name":
			if v, ok := value.(string); ok {
				person.GivenName = v
				person.FullName = v + " " + person.Surname
			}
		case "surname":
			if v, ok := value.(string); ok {
				person.Surname = v
				person.FullName = person.GivenName + " " + v
			}
		case "gender":
			if v, ok := value.(string); ok {
				person.Gender = domain.Gender(v)
			}
		case "birth_date":
			if v, ok := value.(string); ok {
				person.BirthDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					person.BirthDateSort = &t
				} else {
					person.BirthDateSort = nil
				}
			}
		case "birth_place":
			if v, ok := value.(string); ok {
				person.BirthPlace = v
			}
		case "death_date":
			if v, ok := value.(string); ok {
				person.DeathDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					person.DeathDateSort = &t
				} else {
					person.DeathDateSort = nil
				}
			}
		case "death_place":
			if v, ok := value.(string); ok {
				person.DeathPlace = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				person.Notes = v
			}
		}
	}

	person.Version = version
	person.UpdatedAt = e.OccurredAt()

	return p.readStore.SavePerson(ctx, person)
}

func (p *Projector) projectPersonDeleted(ctx context.Context, e domain.PersonDeleted) error {
	return p.readStore.DeletePerson(ctx, e.PersonID)
}

func (p *Projector) projectFamilyCreated(ctx context.Context, e domain.FamilyCreated, version int64) error {
	var marriageDateSort *time.Time
	var marriageDateRaw string

	if e.MarriageDate != nil {
		marriageDateRaw = e.MarriageDate.Raw
		t := e.MarriageDate.ToTime()
		if !t.IsZero() {
			marriageDateSort = &t
		}
	}

	// Get partner names if available
	var partner1Name, partner2Name string
	if e.Partner1ID != nil {
		if p1, _ := p.readStore.GetPerson(ctx, *e.Partner1ID); p1 != nil {
			partner1Name = p1.FullName
		}
	}
	if e.Partner2ID != nil {
		if p2, _ := p.readStore.GetPerson(ctx, *e.Partner2ID); p2 != nil {
			partner2Name = p2.FullName
		}
	}

	family := &FamilyReadModel{
		ID:               e.FamilyID,
		Partner1ID:       e.Partner1ID,
		Partner1Name:     partner1Name,
		Partner2ID:       e.Partner2ID,
		Partner2Name:     partner2Name,
		RelationshipType: e.RelationshipType,
		MarriageDateRaw:  marriageDateRaw,
		MarriageDateSort: marriageDateSort,
		MarriagePlace:    e.MarriagePlace,
		ChildCount:       0,
		Version:          version,
		UpdatedAt:        e.OccurredAt(),
	}

	return p.readStore.SaveFamily(ctx, family)
}

func (p *Projector) projectFamilyUpdated(ctx context.Context, e domain.FamilyUpdated, version int64) error {
	family, err := p.readStore.GetFamily(ctx, e.FamilyID)
	if err != nil {
		return err
	}
	if family == nil {
		return nil // Family doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "partner1_id":
			// Handle partner ID changes - complex, may need to fetch new name
		case "partner2_id":
			// Handle partner ID changes
		case "relationship_type":
			if v, ok := value.(string); ok {
				family.RelationshipType = domain.RelationType(v)
			}
		case "marriage_date":
			if v, ok := value.(string); ok {
				family.MarriageDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					family.MarriageDateSort = &t
				} else {
					family.MarriageDateSort = nil
				}
			}
		case "marriage_place":
			if v, ok := value.(string); ok {
				family.MarriagePlace = v
			}
		}
	}

	family.Version = version
	family.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveFamily(ctx, family)
}

func (p *Projector) projectChildLinked(ctx context.Context, e domain.ChildLinkedToFamily) error {
	// Get child name
	var childName string
	if child, _ := p.readStore.GetPerson(ctx, e.PersonID); child != nil {
		childName = child.FullName
	}

	fc := &FamilyChildReadModel{
		FamilyID:         e.FamilyID,
		PersonID:         e.PersonID,
		PersonName:       childName,
		RelationshipType: e.RelationshipType,
		Sequence:         e.Sequence,
	}

	if err := p.readStore.SaveFamilyChild(ctx, fc); err != nil {
		return err
	}

	// Update pedigree edge for child
	family, err := p.readStore.GetFamily(ctx, e.FamilyID)
	if err != nil {
		return err
	}
	if family != nil {
		edge := &PedigreeEdge{
			PersonID: e.PersonID,
		}
		if family.Partner1ID != nil {
			// Determine father/mother based on gender (simplified)
			p1, _ := p.readStore.GetPerson(ctx, *family.Partner1ID)
			if p1 != nil {
				if p1.Gender == domain.GenderMale {
					edge.FatherID = family.Partner1ID
					edge.FatherName = p1.FullName
				} else {
					edge.MotherID = family.Partner1ID
					edge.MotherName = p1.FullName
				}
			}
		}
		if family.Partner2ID != nil {
			p2, _ := p.readStore.GetPerson(ctx, *family.Partner2ID)
			if p2 != nil {
				if p2.Gender == domain.GenderMale {
					edge.FatherID = family.Partner2ID
					edge.FatherName = p2.FullName
				} else {
					edge.MotherID = family.Partner2ID
					edge.MotherName = p2.FullName
				}
			}
		}
		if err := p.readStore.SavePedigreeEdge(ctx, edge); err != nil {
			return err
		}
	}

	// Increment family child count and version
	if family != nil {
		family.ChildCount++
		family.Version++
		family.UpdatedAt = e.OccurredAt()
		return p.readStore.SaveFamily(ctx, family)
	}

	return nil
}

func (p *Projector) projectChildUnlinked(ctx context.Context, e domain.ChildUnlinkedFromFamily) error {
	if err := p.readStore.DeleteFamilyChild(ctx, e.FamilyID, e.PersonID); err != nil {
		return err
	}

	// Remove pedigree edge
	if err := p.readStore.DeletePedigreeEdge(ctx, e.PersonID); err != nil {
		return err
	}

	// Decrement family child count and increment version
	family, err := p.readStore.GetFamily(ctx, e.FamilyID)
	if err != nil {
		return err
	}
	if family != nil {
		if family.ChildCount > 0 {
			family.ChildCount--
		}
		family.Version++
		family.UpdatedAt = e.OccurredAt()
		return p.readStore.SaveFamily(ctx, family)
	}

	return nil
}

func (p *Projector) projectFamilyDeleted(ctx context.Context, e domain.FamilyDeleted) error {
	// Delete all children first
	children, err := p.readStore.GetFamilyChildren(ctx, e.FamilyID)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := p.readStore.DeleteFamilyChild(ctx, e.FamilyID, child.PersonID); err != nil {
			return err
		}
		if err := p.readStore.DeletePedigreeEdge(ctx, child.PersonID); err != nil {
			return err
		}
	}

	return p.readStore.DeleteFamily(ctx, e.FamilyID)
}
