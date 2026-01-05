package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

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
	case domain.SourceCreated:
		return p.projectSourceCreated(ctx, e, version)
	case domain.SourceUpdated:
		return p.projectSourceUpdated(ctx, e, version)
	case domain.SourceDeleted:
		return p.projectSourceDeleted(ctx, e)
	case domain.CitationCreated:
		return p.projectCitationCreated(ctx, e, version)
	case domain.CitationUpdated:
		return p.projectCitationUpdated(ctx, e, version)
	case domain.CitationDeleted:
		return p.projectCitationDeleted(ctx, e)
	case domain.MediaCreated:
		return p.projectMediaCreated(ctx, e, version)
	case domain.MediaUpdated:
		return p.projectMediaUpdated(ctx, e, version)
	case domain.MediaDeleted:
		return p.projectMediaDeleted(ctx, e)
	case domain.LifeEventCreated:
		return p.projectLifeEventCreated(ctx, e, version)
	case domain.LifeEventDeleted:
		return p.projectLifeEventDeleted(ctx, e)
	case domain.AttributeCreated:
		return p.projectAttributeCreated(ctx, e, version)
	case domain.AttributeDeleted:
		return p.projectAttributeDeleted(ctx, e)
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
		ID:             e.PersonID,
		GivenName:      e.GivenName,
		Surname:        e.Surname,
		FullName:       e.GivenName + " " + e.Surname,
		Gender:         e.Gender,
		BirthDateRaw:   birthDateRaw,
		BirthDateSort:  birthDateSort,
		BirthPlace:     e.BirthPlace,
		DeathDateRaw:   deathDateRaw,
		DeathDateSort:  deathDateSort,
		DeathPlace:     e.DeathPlace,
		Notes:          e.Notes,
		ResearchStatus: e.ResearchStatus,
		Version:        version,
		UpdatedAt:      e.OccurredAt(),
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
		case "research_status":
			if v, ok := value.(string); ok {
				person.ResearchStatus = domain.ParseResearchStatus(v)
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

func (p *Projector) projectSourceCreated(ctx context.Context, e domain.SourceCreated, version int64) error {
	var publishDateSort *time.Time
	var publishDateRaw string

	if e.PublishDate != nil {
		publishDateRaw = e.PublishDate.Raw
		t := e.PublishDate.ToTime()
		if !t.IsZero() {
			publishDateSort = &t
		}
	}

	source := &SourceReadModel{
		ID:              e.SourceID,
		SourceType:      e.SourceType,
		Title:           e.Title,
		Author:          e.Author,
		Publisher:       e.Publisher,
		PublishDateRaw:  publishDateRaw,
		PublishDateSort: publishDateSort,
		URL:             e.URL,
		RepositoryName:  e.RepositoryName,
		CollectionName:  e.CollectionName,
		CallNumber:      e.CallNumber,
		Notes:           e.Notes,
		GedcomXref:      e.GedcomXref,
		CitationCount:   0,
		Version:         version,
		UpdatedAt:       e.OccurredAt(),
	}

	return p.readStore.SaveSource(ctx, source)
}

func (p *Projector) projectSourceUpdated(ctx context.Context, e domain.SourceUpdated, version int64) error {
	source, err := p.readStore.GetSource(ctx, e.SourceID)
	if err != nil {
		return err
	}
	if source == nil {
		return nil // Source doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "source_type":
			if v, ok := value.(string); ok {
				source.SourceType = domain.SourceType(v)
			}
		case "title":
			if v, ok := value.(string); ok {
				source.Title = v
			}
		case "author":
			if v, ok := value.(string); ok {
				source.Author = v
			}
		case "publisher":
			if v, ok := value.(string); ok {
				source.Publisher = v
			}
		case "publish_date":
			if v, ok := value.(string); ok {
				source.PublishDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					source.PublishDateSort = &t
				} else {
					source.PublishDateSort = nil
				}
			}
		case "url":
			if v, ok := value.(string); ok {
				source.URL = v
			}
		case "repository_name":
			if v, ok := value.(string); ok {
				source.RepositoryName = v
			}
		case "collection_name":
			if v, ok := value.(string); ok {
				source.CollectionName = v
			}
		case "call_number":
			if v, ok := value.(string); ok {
				source.CallNumber = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				source.Notes = v
			}
		}
	}

	source.Version = version
	source.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveSource(ctx, source)
}

func (p *Projector) projectSourceDeleted(ctx context.Context, e domain.SourceDeleted) error {
	return p.readStore.DeleteSource(ctx, e.SourceID)
}

func (p *Projector) projectCitationCreated(ctx context.Context, e domain.CitationCreated, version int64) error {
	// Get source title for denormalization
	var sourceTitle string
	if source, _ := p.readStore.GetSource(ctx, e.SourceID); source != nil {
		sourceTitle = source.Title
	}

	citation := &CitationReadModel{
		ID:            e.CitationID,
		SourceID:      e.SourceID,
		SourceTitle:   sourceTitle,
		FactType:      e.FactType,
		FactOwnerID:   e.FactOwnerID,
		Page:          e.Page,
		Volume:        e.Volume,
		SourceQuality: e.SourceQuality,
		InformantType: e.InformantType,
		EvidenceType:  e.EvidenceType,
		QuotedText:    e.QuotedText,
		Analysis:      e.Analysis,
		TemplateID:    e.TemplateID,
		GedcomXref:    e.GedcomXref,
		Version:       version,
		CreatedAt:     e.OccurredAt(),
	}

	if err := p.readStore.SaveCitation(ctx, citation); err != nil {
		return err
	}

	// Increment citation count on source
	source, err := p.readStore.GetSource(ctx, e.SourceID)
	if err != nil {
		return err
	}
	if source != nil {
		source.CitationCount++
		source.UpdatedAt = e.OccurredAt()
		return p.readStore.SaveSource(ctx, source)
	}

	return nil
}

func (p *Projector) projectCitationUpdated(ctx context.Context, e domain.CitationUpdated, version int64) error {
	citation, err := p.readStore.GetCitation(ctx, e.CitationID)
	if err != nil {
		return err
	}
	if citation == nil {
		return nil // Citation doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "source_id":
			if v, ok := value.(string); ok {
				if newSourceID, err := uuid.Parse(v); err == nil {
					// Update source citation counts
					if citation.SourceID != newSourceID {
						// Decrement old source
						if oldSource, _ := p.readStore.GetSource(ctx, citation.SourceID); oldSource != nil {
							if oldSource.CitationCount > 0 {
								oldSource.CitationCount--
							}
							p.readStore.SaveSource(ctx, oldSource)
						}
						// Increment new source
						if newSource, _ := p.readStore.GetSource(ctx, newSourceID); newSource != nil {
							newSource.CitationCount++
							citation.SourceTitle = newSource.Title
							p.readStore.SaveSource(ctx, newSource)
						}
						citation.SourceID = newSourceID
					}
				}
			}
		case "fact_type":
			if v, ok := value.(string); ok {
				citation.FactType = domain.FactType(v)
			}
		case "fact_owner_id":
			if v, ok := value.(string); ok {
				if id, err := uuid.Parse(v); err == nil {
					citation.FactOwnerID = id
				}
			}
		case "page":
			if v, ok := value.(string); ok {
				citation.Page = v
			}
		case "volume":
			if v, ok := value.(string); ok {
				citation.Volume = v
			}
		case "source_quality":
			if v, ok := value.(string); ok {
				citation.SourceQuality = domain.SourceQuality(v)
			}
		case "informant_type":
			if v, ok := value.(string); ok {
				citation.InformantType = domain.InformantType(v)
			}
		case "evidence_type":
			if v, ok := value.(string); ok {
				citation.EvidenceType = domain.EvidenceType(v)
			}
		case "quoted_text":
			if v, ok := value.(string); ok {
				citation.QuotedText = v
			}
		case "analysis":
			if v, ok := value.(string); ok {
				citation.Analysis = v
			}
		case "template_id":
			if v, ok := value.(string); ok {
				citation.TemplateID = v
			}
		}
	}

	citation.Version = version
	return p.readStore.SaveCitation(ctx, citation)
}

func (p *Projector) projectCitationDeleted(ctx context.Context, e domain.CitationDeleted) error {
	// Get citation first to update source citation count
	citation, err := p.readStore.GetCitation(ctx, e.CitationID)
	if err != nil {
		return err
	}
	if citation != nil {
		// Decrement source citation count
		source, err := p.readStore.GetSource(ctx, citation.SourceID)
		if err != nil {
			return err
		}
		if source != nil {
			if source.CitationCount > 0 {
				source.CitationCount--
			}
			source.UpdatedAt = e.OccurredAt()
			if err := p.readStore.SaveSource(ctx, source); err != nil {
				return err
			}
		}
	}

	return p.readStore.DeleteCitation(ctx, e.CitationID)
}

func (p *Projector) projectMediaCreated(ctx context.Context, e domain.MediaCreated, version int64) error {
	media := &MediaReadModel{
		ID:            e.MediaID,
		EntityType:    e.EntityType,
		EntityID:      e.EntityID,
		Title:         e.Title,
		Description:   e.Description,
		MimeType:      e.MimeType,
		MediaType:     e.MediaType,
		Filename:      e.Filename,
		FileSize:      e.FileSize,
		FileData:      e.FileData,
		ThumbnailData: e.ThumbnailData,
		GedcomXref:    e.GedcomXref,
		Version:       version,
		CreatedAt:     e.OccurredAt(),
		UpdatedAt:     e.OccurredAt(),
	}

	return p.readStore.SaveMedia(ctx, media)
}

func (p *Projector) projectMediaUpdated(ctx context.Context, e domain.MediaUpdated, version int64) error {
	media, err := p.readStore.GetMediaWithData(ctx, e.MediaID)
	if err != nil {
		return err
	}
	if media == nil {
		return nil // Media doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "title":
			if v, ok := value.(string); ok {
				media.Title = v
			}
		case "description":
			if v, ok := value.(string); ok {
				media.Description = v
			}
		case "media_type":
			if v, ok := value.(string); ok {
				media.MediaType = domain.MediaType(v)
			}
		case "crop_left":
			if v, ok := value.(int); ok {
				media.CropLeft = &v
			}
		case "crop_top":
			if v, ok := value.(int); ok {
				media.CropTop = &v
			}
		case "crop_width":
			if v, ok := value.(int); ok {
				media.CropWidth = &v
			}
		case "crop_height":
			if v, ok := value.(int); ok {
				media.CropHeight = &v
			}
		}
	}

	media.Version = version
	media.UpdatedAt = time.Now()

	return p.readStore.SaveMedia(ctx, media)
}

func (p *Projector) projectMediaDeleted(ctx context.Context, e domain.MediaDeleted) error {
	return p.readStore.DeleteMedia(ctx, e.MediaID)
}

func (p *Projector) projectLifeEventCreated(ctx context.Context, e domain.LifeEventCreated, version int64) error {
	var dateSort *time.Time
	var dateRaw string

	if e.Date != nil {
		dateRaw = e.Date.Raw
		t := e.Date.ToTime()
		if !t.IsZero() {
			dateSort = &t
		}
	}

	// Derive owner type and ID from PersonID/FamilyID
	var ownerType string
	var ownerID uuid.UUID
	if e.PersonID != nil {
		ownerType = "person"
		ownerID = *e.PersonID
	} else if e.FamilyID != nil {
		ownerType = "family"
		ownerID = *e.FamilyID
	}

	event := &EventReadModel{
		ID:          e.EventID,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		FactType:    e.FactType,
		DateRaw:     dateRaw,
		DateSort:    dateSort,
		Place:       e.Place,
		Description: e.Description,
		Cause:       e.Cause,
		Age:         e.Age,
		Version:     version,
		CreatedAt:   e.OccurredAt(),
	}

	return p.readStore.SaveEvent(ctx, event)
}

func (p *Projector) projectLifeEventDeleted(ctx context.Context, e domain.LifeEventDeleted) error {
	return p.readStore.DeleteEvent(ctx, e.EventID)
}

func (p *Projector) projectAttributeCreated(ctx context.Context, e domain.AttributeCreated, version int64) error {
	var dateSort *time.Time
	var dateRaw string

	if e.Date != nil {
		dateRaw = e.Date.Raw
		t := e.Date.ToTime()
		if !t.IsZero() {
			dateSort = &t
		}
	}

	attribute := &AttributeReadModel{
		ID:        e.AttributeID,
		PersonID:  e.PersonID,
		FactType:  e.FactType,
		Value:     e.Value,
		DateRaw:   dateRaw,
		DateSort:  dateSort,
		Place:     e.Place,
		Version:   version,
		CreatedAt: e.OccurredAt(),
	}

	return p.readStore.SaveAttribute(ctx, attribute)
}

func (p *Projector) projectAttributeDeleted(ctx context.Context, e domain.AttributeDeleted) error {
	return p.readStore.DeleteAttribute(ctx, e.AttributeID)
}
