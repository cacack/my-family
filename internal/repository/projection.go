package repository

import (
	"context"
	"fmt"
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
	case domain.NameAdded:
		return p.projectNameAdded(ctx, e, version)
	case domain.NameUpdated:
		return p.projectNameUpdated(ctx, e, version)
	case domain.NameRemoved:
		return p.projectNameRemoved(ctx, e, version)
	case domain.PersonMerged:
		return p.projectPersonMerged(ctx, e, version)
	case domain.NoteCreated:
		return p.projectNoteCreated(ctx, e, version)
	case domain.NoteUpdated:
		return p.projectNoteUpdated(ctx, e, version)
	case domain.NoteDeleted:
		return p.projectNoteDeleted(ctx, e)
	case domain.SubmitterCreated:
		return p.projectSubmitterCreated(ctx, e, version)
	case domain.SubmitterUpdated:
		return p.projectSubmitterUpdated(ctx, e, version)
	case domain.SubmitterDeleted:
		return p.projectSubmitterDeleted(ctx, e)
	case domain.AssociationCreated:
		return p.projectAssociationCreated(ctx, e, version)
	case domain.AssociationUpdated:
		return p.projectAssociationUpdated(ctx, e, version)
	case domain.AssociationDeleted:
		return p.projectAssociationDeleted(ctx, e)
	case domain.LDSOrdinanceCreated:
		return p.projectLDSOrdinanceCreated(ctx, e, version)
	case domain.LDSOrdinanceUpdated:
		return p.projectLDSOrdinanceUpdated(ctx, e, version)
	case domain.LDSOrdinanceDeleted:
		return p.projectLDSOrdinanceDeleted(ctx, e)
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
							_ = p.readStore.SaveSource(ctx, oldSource)
						}
						// Increment new source
						if newSource, _ := p.readStore.GetSource(ctx, newSourceID); newSource != nil {
							newSource.CitationCount++
							citation.SourceTitle = newSource.Title
							_ = p.readStore.SaveSource(ctx, newSource)
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
		// GEDCOM 7.0 enhanced fields
		Files:        e.Files,
		Format:       e.Format,
		Translations: e.Translations,
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
		case "files":
			if v, ok := value.([]domain.MediaFile); ok {
				media.Files = v
			}
		case "format":
			if v, ok := value.(string); ok {
				media.Format = v
			}
		case "translations":
			if v, ok := value.([]string); ok {
				media.Translations = v
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
		Address:     e.Address,
		Description: e.Description,
		Cause:       e.Cause,
		Age:         e.Age,
		IsNegated:   e.IsNegated,
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

func (p *Projector) projectNameAdded(ctx context.Context, e domain.NameAdded, version int64) error {
	// Build full name from components
	fullName := buildFullName(e.GivenName, e.Surname, e.NamePrefix, e.NameSuffix, e.SurnamePrefix)

	name := &PersonNameReadModel{
		ID:            e.NameID,
		PersonID:      e.PersonID,
		GivenName:     e.GivenName,
		Surname:       e.Surname,
		FullName:      fullName,
		NamePrefix:    e.NamePrefix,
		NameSuffix:    e.NameSuffix,
		SurnamePrefix: e.SurnamePrefix,
		Nickname:      e.Nickname,
		NameType:      e.NameType,
		IsPrimary:     e.IsPrimary,
		UpdatedAt:     e.OccurredAt(),
	}

	if err := p.readStore.SavePersonName(ctx, name); err != nil {
		return err
	}

	// Update person version to stay in sync with event stream
	return p.updatePersonVersion(ctx, e.PersonID, version)
}

func (p *Projector) projectNameUpdated(ctx context.Context, e domain.NameUpdated, version int64) error {
	// Build full name from components
	fullName := buildFullName(e.GivenName, e.Surname, e.NamePrefix, e.NameSuffix, e.SurnamePrefix)

	name := &PersonNameReadModel{
		ID:            e.NameID,
		PersonID:      e.PersonID,
		GivenName:     e.GivenName,
		Surname:       e.Surname,
		FullName:      fullName,
		NamePrefix:    e.NamePrefix,
		NameSuffix:    e.NameSuffix,
		SurnamePrefix: e.SurnamePrefix,
		Nickname:      e.Nickname,
		NameType:      e.NameType,
		IsPrimary:     e.IsPrimary,
		UpdatedAt:     e.OccurredAt(),
	}

	if err := p.readStore.SavePersonName(ctx, name); err != nil {
		return err
	}

	// Update person version to stay in sync with event stream
	return p.updatePersonVersion(ctx, e.PersonID, version)
}

func (p *Projector) projectNameRemoved(ctx context.Context, e domain.NameRemoved, version int64) error {
	if err := p.readStore.DeletePersonName(ctx, e.NameID); err != nil {
		return err
	}

	// Update person version to stay in sync with event stream
	return p.updatePersonVersion(ctx, e.PersonID, version)
}

// updatePersonVersion updates a person's version in the read model.
func (p *Projector) updatePersonVersion(ctx context.Context, personID uuid.UUID, version int64) error {
	person, err := p.readStore.GetPerson(ctx, personID)
	if err != nil {
		return fmt.Errorf("get person for version update: %w", err)
	}
	if person == nil {
		// Person may not exist yet if events are out of order
		return nil
	}
	person.Version = version
	return p.readStore.SavePerson(ctx, person)
}

// buildFullName constructs a full name from its components.
func buildFullName(givenName, surname, namePrefix, nameSuffix, surnamePrefix string) string {
	var parts []string

	if namePrefix != "" {
		parts = append(parts, namePrefix)
	}
	if givenName != "" {
		parts = append(parts, givenName)
	}
	if surnamePrefix != "" {
		parts = append(parts, surnamePrefix)
	}
	if surname != "" {
		parts = append(parts, surname)
	}
	if nameSuffix != "" {
		parts = append(parts, nameSuffix)
	}

	if len(parts) == 0 {
		return ""
	}

	// Join with spaces
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += " " + parts[i]
	}
	return result
}

// projectPersonMerged handles the PersonMerged event by updating the survivor,
// transferring relationships/data, and deleting the merged person.
func (p *Projector) projectPersonMerged(ctx context.Context, e domain.PersonMerged, version int64) error {
	// 1. Update survivor person with resolved fields
	survivor, err := p.readStore.GetPerson(ctx, e.SurvivorID)
	if err != nil {
		return err
	}
	if survivor == nil {
		return nil // Survivor doesn't exist, skip
	}

	// Apply resolved fields to survivor (same logic as PersonUpdated)
	for key, value := range e.ResolvedFields {
		switch key {
		case "given_name":
			if v, ok := value.(string); ok {
				survivor.GivenName = v
				survivor.FullName = v + " " + survivor.Surname
			}
		case "surname":
			if v, ok := value.(string); ok {
				survivor.Surname = v
				survivor.FullName = survivor.GivenName + " " + v
			}
		case "gender":
			if v, ok := value.(string); ok {
				survivor.Gender = domain.Gender(v)
			}
		case "birth_date":
			if v, ok := value.(string); ok {
				survivor.BirthDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					survivor.BirthDateSort = &t
				} else {
					survivor.BirthDateSort = nil
				}
			}
		case "birth_place":
			if v, ok := value.(string); ok {
				survivor.BirthPlace = v
			}
		case "death_date":
			if v, ok := value.(string); ok {
				survivor.DeathDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					survivor.DeathDateSort = &t
				} else {
					survivor.DeathDateSort = nil
				}
			}
		case "death_place":
			if v, ok := value.(string); ok {
				survivor.DeathPlace = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				survivor.Notes = v
			}
		case "research_status":
			if v, ok := value.(string); ok {
				survivor.ResearchStatus = domain.ParseResearchStatus(v)
			}
		}
	}

	survivor.Version = version
	survivor.UpdatedAt = e.OccurredAt()

	if err := p.readStore.SavePerson(ctx, survivor); err != nil {
		return err
	}

	// 2. Update families where merged person is a partner
	families, err := p.readStore.GetFamiliesForPerson(ctx, e.MergedID)
	if err != nil {
		return err
	}
	for _, family := range families {
		if family.Partner1ID != nil && *family.Partner1ID == e.MergedID {
			family.Partner1ID = &e.SurvivorID
			family.Partner1Name = survivor.FullName
		}
		if family.Partner2ID != nil && *family.Partner2ID == e.MergedID {
			family.Partner2ID = &e.SurvivorID
			family.Partner2Name = survivor.FullName
		}
		family.UpdatedAt = e.OccurredAt()
		if err := p.readStore.SaveFamily(ctx, &family); err != nil {
			return err
		}
	}

	// 3. Update pedigree edges where merged person is a parent
	// (We need to find all children who have the merged person as father/mother)
	// This requires iterating through all pedigree edges - a bit expensive but necessary
	// For now, handle the merged person's own pedigree edge (if they are a child somewhere)
	mergedEdge, err := p.readStore.GetPedigreeEdge(ctx, e.MergedID)
	if err != nil {
		return err
	}
	if mergedEdge != nil {
		// Transfer the child-family relationship to survivor
		// First, get the family where merged person is a child
		mergedChildFamily, err := p.readStore.GetChildFamily(ctx, e.MergedID)
		if err != nil {
			return err
		}
		if mergedChildFamily != nil {
			// Delete old family-child record
			if err := p.readStore.DeleteFamilyChild(ctx, mergedChildFamily.ID, e.MergedID); err != nil {
				return err
			}
			// Delete old pedigree edge
			if err := p.readStore.DeletePedigreeEdge(ctx, e.MergedID); err != nil {
				return err
			}

			// Check if survivor already has a child-family relationship
			survivorChildFamily, err := p.readStore.GetChildFamily(ctx, e.SurvivorID)
			if err != nil {
				return err
			}
			if survivorChildFamily == nil {
				// Survivor doesn't have a child-family, transfer the relationship
				fc := &FamilyChildReadModel{
					FamilyID:         mergedChildFamily.ID,
					PersonID:         e.SurvivorID,
					PersonName:       survivor.FullName,
					RelationshipType: domain.ChildBiological, // Default, could be improved
				}
				if err := p.readStore.SaveFamilyChild(ctx, fc); err != nil {
					return err
				}

				// Create pedigree edge for survivor
				edge := &PedigreeEdge{
					PersonID:   e.SurvivorID,
					FatherID:   mergedEdge.FatherID,
					MotherID:   mergedEdge.MotherID,
					FatherName: mergedEdge.FatherName,
					MotherName: mergedEdge.MotherName,
				}
				if err := p.readStore.SavePedigreeEdge(ctx, edge); err != nil {
					return err
				}

				// Update family child count
				mergedChildFamily.ChildCount-- // We removed merged
				mergedChildFamily.ChildCount++ // We added survivor (net 0 change if same family)
				mergedChildFamily.UpdatedAt = e.OccurredAt()
				if err := p.readStore.SaveFamily(ctx, mergedChildFamily); err != nil {
					return err
				}
			}
			// If survivor already has a child-family, we don't transfer (plan says block this case)
		}
	}

	// 4. Reassign citations from merged person to survivor
	citations, err := p.readStore.GetCitationsForPerson(ctx, e.MergedID)
	if err != nil {
		return err
	}
	for _, citation := range citations {
		citation.FactOwnerID = e.SurvivorID
		if err := p.readStore.SaveCitation(ctx, &citation); err != nil {
			return err
		}
	}

	// 5. Transfer PersonName records from merged to survivor
	names, err := p.readStore.GetPersonNames(ctx, e.MergedID)
	if err != nil {
		return err
	}
	for _, name := range names {
		// Change the person ID to survivor and mark as non-primary
		name.PersonID = e.SurvivorID
		name.IsPrimary = false // Transferred names become alternate names
		name.UpdatedAt = e.OccurredAt()
		if err := p.readStore.SavePersonName(ctx, &name); err != nil {
			return err
		}
	}

	// 6. Transfer life events from merged person to survivor
	events, err := p.readStore.ListEventsForPerson(ctx, e.MergedID)
	if err != nil {
		return err
	}
	for _, event := range events {
		event.OwnerID = e.SurvivorID
		if err := p.readStore.SaveEvent(ctx, &event); err != nil {
			return err
		}
	}

	// 7. Transfer media from merged person to survivor
	mediaList, _, err := p.readStore.ListMediaForEntity(ctx, "person", e.MergedID, ListOptions{Limit: 10000})
	if err != nil {
		return err
	}
	for _, media := range mediaList {
		media.EntityID = e.SurvivorID
		media.UpdatedAt = e.OccurredAt()
		if err := p.readStore.SaveMedia(ctx, &media); err != nil {
			return err
		}
	}

	// 8. Transfer attributes from merged person to survivor
	attributes, err := p.readStore.ListAttributesForPerson(ctx, e.MergedID)
	if err != nil {
		return err
	}
	for _, attr := range attributes {
		attr.PersonID = e.SurvivorID
		if err := p.readStore.SaveAttribute(ctx, &attr); err != nil {
			return err
		}
	}

	// 9. Delete merged person from read model
	return p.readStore.DeletePerson(ctx, e.MergedID)
}

// Note projections

func (p *Projector) projectNoteCreated(ctx context.Context, e domain.NoteCreated, version int64) error {
	note := &NoteReadModel{
		ID:         e.NoteID,
		Text:       e.Text,
		GedcomXref: e.GedcomXref,
		Version:    version,
		UpdatedAt:  e.OccurredAt(),
	}

	return p.readStore.SaveNote(ctx, note)
}

func (p *Projector) projectNoteUpdated(ctx context.Context, e domain.NoteUpdated, version int64) error {
	note, err := p.readStore.GetNote(ctx, e.NoteID)
	if err != nil {
		return err
	}
	if note == nil {
		return nil // Note doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		if key == "text" {
			if v, ok := value.(string); ok {
				note.Text = v
			}
		}
	}

	note.Version = version
	note.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveNote(ctx, note)
}

func (p *Projector) projectNoteDeleted(ctx context.Context, e domain.NoteDeleted) error {
	return p.readStore.DeleteNote(ctx, e.NoteID)
}

// Submitter projections

func (p *Projector) projectSubmitterCreated(ctx context.Context, e domain.SubmitterCreated, version int64) error {
	submitter := &SubmitterReadModel{
		ID:         e.SubmitterID,
		Name:       e.Name,
		Address:    e.Address,
		Phone:      e.Phone,
		Email:      e.Email,
		Language:   e.Language,
		MediaID:    e.MediaID,
		GedcomXref: e.GedcomXref,
		Version:    version,
		UpdatedAt:  e.OccurredAt(),
	}

	return p.readStore.SaveSubmitter(ctx, submitter)
}

func (p *Projector) projectSubmitterUpdated(ctx context.Context, e domain.SubmitterUpdated, version int64) error {
	submitter, err := p.readStore.GetSubmitter(ctx, e.SubmitterID)
	if err != nil {
		return err
	}
	if submitter == nil {
		return nil // Submitter doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "name":
			if v, ok := value.(string); ok {
				submitter.Name = v
			}
		case "address":
			if v, ok := value.(*domain.Address); ok {
				submitter.Address = v
			}
		case "phone":
			if v, ok := value.([]string); ok {
				submitter.Phone = v
			}
		case "email":
			if v, ok := value.([]string); ok {
				submitter.Email = v
			}
		case "language":
			if v, ok := value.(string); ok {
				submitter.Language = v
			}
		case "media_id":
			if v, ok := value.(*uuid.UUID); ok {
				submitter.MediaID = v
			}
		}
	}

	submitter.Version = version
	submitter.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveSubmitter(ctx, submitter)
}

func (p *Projector) projectSubmitterDeleted(ctx context.Context, e domain.SubmitterDeleted) error {
	return p.readStore.DeleteSubmitter(ctx, e.SubmitterID)
}

// Association projections

func (p *Projector) projectAssociationCreated(ctx context.Context, e domain.AssociationCreated, version int64) error {
	// Look up person names for denormalization
	personName := ""
	associateName := ""

	if person, err := p.readStore.GetPerson(ctx, e.PersonID); err == nil && person != nil {
		personName = person.FullName
	}
	if associate, err := p.readStore.GetPerson(ctx, e.AssociateID); err == nil && associate != nil {
		associateName = associate.FullName
	}

	association := &AssociationReadModel{
		ID:            e.AssociationID,
		PersonID:      e.PersonID,
		PersonName:    personName,
		AssociateID:   e.AssociateID,
		AssociateName: associateName,
		Role:          e.Role,
		Phrase:        e.Phrase,
		Notes:         e.Notes,
		NoteIDs:       e.NoteIDs,
		GedcomXref:    e.GedcomXref,
		Version:       version,
		UpdatedAt:     e.OccurredAt(),
	}

	return p.readStore.SaveAssociation(ctx, association)
}

func (p *Projector) projectAssociationUpdated(ctx context.Context, e domain.AssociationUpdated, version int64) error {
	association, err := p.readStore.GetAssociation(ctx, e.AssociationID)
	if err != nil {
		return err
	}
	if association == nil {
		return nil // Association doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "role":
			if v, ok := value.(string); ok {
				association.Role = v
			}
		case "phrase":
			if v, ok := value.(string); ok {
				association.Phrase = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				association.Notes = v
			}
		case "note_ids":
			if v, ok := value.([]uuid.UUID); ok {
				association.NoteIDs = v
			}
		}
	}

	association.Version = version
	association.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveAssociation(ctx, association)
}

func (p *Projector) projectAssociationDeleted(ctx context.Context, e domain.AssociationDeleted) error {
	return p.readStore.DeleteAssociation(ctx, e.AssociationID)
}

// LDS Ordinance projections

func (p *Projector) projectLDSOrdinanceCreated(ctx context.Context, e domain.LDSOrdinanceCreated, version int64) error {
	var dateSort *time.Time
	var dateRaw string

	if e.Date != nil {
		dateRaw = e.Date.Raw
		t := e.Date.ToTime()
		if !t.IsZero() {
			dateSort = &t
		}
	}

	// Look up person name for denormalization
	personName := ""
	if e.PersonID != nil {
		if person, err := p.readStore.GetPerson(ctx, *e.PersonID); err == nil && person != nil {
			personName = person.FullName
		}
	}

	ordinance := &LDSOrdinanceReadModel{
		ID:         e.OrdinanceID,
		Type:       e.Type,
		TypeLabel:  e.Type.Label(),
		PersonID:   e.PersonID,
		PersonName: personName,
		FamilyID:   e.FamilyID,
		DateRaw:    dateRaw,
		DateSort:   dateSort,
		Place:      e.Place,
		Temple:     e.Temple,
		Status:     e.Status,
		Version:    version,
		UpdatedAt:  e.OccurredAt(),
	}

	return p.readStore.SaveLDSOrdinance(ctx, ordinance)
}

func (p *Projector) projectLDSOrdinanceUpdated(ctx context.Context, e domain.LDSOrdinanceUpdated, version int64) error {
	ordinance, err := p.readStore.GetLDSOrdinance(ctx, e.OrdinanceID)
	if err != nil {
		return err
	}
	if ordinance == nil {
		return nil // Ordinance doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "date":
			if v, ok := value.(string); ok {
				ordinance.DateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					ordinance.DateSort = &t
				} else {
					ordinance.DateSort = nil
				}
			}
		case "place":
			if v, ok := value.(string); ok {
				ordinance.Place = v
			}
		case "temple":
			if v, ok := value.(string); ok {
				ordinance.Temple = v
			}
		case "status":
			if v, ok := value.(string); ok {
				ordinance.Status = v
			}
		}
	}

	ordinance.Version = version
	ordinance.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveLDSOrdinance(ctx, ordinance)
}

func (p *Projector) projectLDSOrdinanceDeleted(ctx context.Context, e domain.LDSOrdinanceDeleted) error {
	return p.readStore.DeleteLDSOrdinance(ctx, e.OrdinanceID)
}
