package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event represents a domain event.
type Event interface {
	EventType() string
	AggregateID() uuid.UUID
	OccurredAt() time.Time
}

// BaseEvent contains common event fields.
type BaseEvent struct {
	ID        uuid.UUID `json:"id"`
	Timestamp time.Time `json:"timestamp"`
}

// OccurredAt returns when the event occurred.
func (e BaseEvent) OccurredAt() time.Time {
	return e.Timestamp
}

// NewBaseEvent creates a new base event with generated ID and current timestamp.
func NewBaseEvent() BaseEvent {
	return BaseEvent{
		ID:        uuid.New(),
		Timestamp: time.Now().UTC(),
	}
}

// PersonCreated event is emitted when a new person is created.
type PersonCreated struct {
	BaseEvent
	PersonID      uuid.UUID `json:"person_id"`
	GivenName     string    `json:"given_name"`
	Surname       string    `json:"surname"`
	NamePrefix    string    `json:"name_prefix,omitempty"`
	NameSuffix    string    `json:"name_suffix,omitempty"`
	SurnamePrefix string    `json:"surname_prefix,omitempty"`
	Nickname      string    `json:"nickname,omitempty"`
	NameType      NameType  `json:"name_type,omitempty"`
	Gender        Gender    `json:"gender,omitempty"`
	BirthDate     *GenDate  `json:"birth_date,omitempty"`
	BirthPlace    string    `json:"birth_place,omitempty"`
	DeathDate     *GenDate  `json:"death_date,omitempty"`
	DeathPlace    string    `json:"death_place,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	GedcomXref    string    `json:"gedcom_xref,omitempty"`
}

func (e PersonCreated) EventType() string      { return "PersonCreated" }
func (e PersonCreated) AggregateID() uuid.UUID { return e.PersonID }

// NewPersonCreated creates a PersonCreated event from a Person.
func NewPersonCreated(p *Person) PersonCreated {
	return PersonCreated{
		BaseEvent:     NewBaseEvent(),
		PersonID:      p.ID,
		GivenName:     p.GivenName,
		Surname:       p.Surname,
		NamePrefix:    p.NamePrefix,
		NameSuffix:    p.NameSuffix,
		SurnamePrefix: p.SurnamePrefix,
		Nickname:      p.Nickname,
		NameType:      p.NameType,
		Gender:        p.Gender,
		BirthDate:     p.BirthDate,
		BirthPlace:    p.BirthPlace,
		DeathDate:     p.DeathDate,
		DeathPlace:    p.DeathPlace,
		Notes:         p.Notes,
		GedcomXref:    p.GedcomXref,
	}
}

// PersonUpdated event is emitted when a person is updated.
type PersonUpdated struct {
	BaseEvent
	PersonID uuid.UUID      `json:"person_id"`
	Changes  map[string]any `json:"changes"`
}

func (e PersonUpdated) EventType() string      { return "PersonUpdated" }
func (e PersonUpdated) AggregateID() uuid.UUID { return e.PersonID }

// NewPersonUpdated creates a PersonUpdated event.
func NewPersonUpdated(personID uuid.UUID, changes map[string]any) PersonUpdated {
	return PersonUpdated{
		BaseEvent: NewBaseEvent(),
		PersonID:  personID,
		Changes:   changes,
	}
}

// PersonDeleted event is emitted when a person is deleted.
type PersonDeleted struct {
	BaseEvent
	PersonID uuid.UUID `json:"person_id"`
	Reason   string    `json:"reason,omitempty"`
}

func (e PersonDeleted) EventType() string      { return "PersonDeleted" }
func (e PersonDeleted) AggregateID() uuid.UUID { return e.PersonID }

// NewPersonDeleted creates a PersonDeleted event.
func NewPersonDeleted(personID uuid.UUID, reason string) PersonDeleted {
	return PersonDeleted{
		BaseEvent: NewBaseEvent(),
		PersonID:  personID,
		Reason:    reason,
	}
}

// FamilyCreated event is emitted when a new family is created.
type FamilyCreated struct {
	BaseEvent
	FamilyID         uuid.UUID    `json:"family_id"`
	Partner1ID       *uuid.UUID   `json:"partner1_id,omitempty"`
	Partner2ID       *uuid.UUID   `json:"partner2_id,omitempty"`
	RelationshipType RelationType `json:"relationship_type,omitempty"`
	MarriageDate     *GenDate     `json:"marriage_date,omitempty"`
	MarriagePlace    string       `json:"marriage_place,omitempty"`
	GedcomXref       string       `json:"gedcom_xref,omitempty"`
}

func (e FamilyCreated) EventType() string      { return "FamilyCreated" }
func (e FamilyCreated) AggregateID() uuid.UUID { return e.FamilyID }

// NewFamilyCreated creates a FamilyCreated event from a Family.
func NewFamilyCreated(f *Family) FamilyCreated {
	return FamilyCreated{
		BaseEvent:        NewBaseEvent(),
		FamilyID:         f.ID,
		Partner1ID:       f.Partner1ID,
		Partner2ID:       f.Partner2ID,
		RelationshipType: f.RelationshipType,
		MarriageDate:     f.MarriageDate,
		MarriagePlace:    f.MarriagePlace,
		GedcomXref:       f.GedcomXref,
	}
}

// FamilyUpdated event is emitted when a family is updated.
type FamilyUpdated struct {
	BaseEvent
	FamilyID uuid.UUID      `json:"family_id"`
	Changes  map[string]any `json:"changes"`
}

func (e FamilyUpdated) EventType() string      { return "FamilyUpdated" }
func (e FamilyUpdated) AggregateID() uuid.UUID { return e.FamilyID }

// NewFamilyUpdated creates a FamilyUpdated event.
func NewFamilyUpdated(familyID uuid.UUID, changes map[string]any) FamilyUpdated {
	return FamilyUpdated{
		BaseEvent: NewBaseEvent(),
		FamilyID:  familyID,
		Changes:   changes,
	}
}

// ChildLinkedToFamily event is emitted when a child is added to a family.
type ChildLinkedToFamily struct {
	BaseEvent
	FamilyID         uuid.UUID         `json:"family_id"`
	PersonID         uuid.UUID         `json:"person_id"`
	RelationshipType ChildRelationType `json:"relationship_type"`
	Sequence         *int              `json:"sequence,omitempty"`
}

func (e ChildLinkedToFamily) EventType() string      { return "ChildLinkedToFamily" }
func (e ChildLinkedToFamily) AggregateID() uuid.UUID { return e.FamilyID }

// NewChildLinkedToFamily creates a ChildLinkedToFamily event.
func NewChildLinkedToFamily(fc *FamilyChild) ChildLinkedToFamily {
	return ChildLinkedToFamily{
		BaseEvent:        NewBaseEvent(),
		FamilyID:         fc.FamilyID,
		PersonID:         fc.PersonID,
		RelationshipType: fc.RelationshipType,
		Sequence:         fc.Sequence,
	}
}

// ChildUnlinkedFromFamily event is emitted when a child is removed from a family.
type ChildUnlinkedFromFamily struct {
	BaseEvent
	FamilyID uuid.UUID `json:"family_id"`
	PersonID uuid.UUID `json:"person_id"`
}

func (e ChildUnlinkedFromFamily) EventType() string      { return "ChildUnlinkedFromFamily" }
func (e ChildUnlinkedFromFamily) AggregateID() uuid.UUID { return e.FamilyID }

// NewChildUnlinkedFromFamily creates a ChildUnlinkedFromFamily event.
func NewChildUnlinkedFromFamily(familyID, personID uuid.UUID) ChildUnlinkedFromFamily {
	return ChildUnlinkedFromFamily{
		BaseEvent: NewBaseEvent(),
		FamilyID:  familyID,
		PersonID:  personID,
	}
}

// FamilyDeleted event is emitted when a family is deleted.
type FamilyDeleted struct {
	BaseEvent
	FamilyID uuid.UUID `json:"family_id"`
	Reason   string    `json:"reason,omitempty"`
}

func (e FamilyDeleted) EventType() string      { return "FamilyDeleted" }
func (e FamilyDeleted) AggregateID() uuid.UUID { return e.FamilyID }

// NewFamilyDeleted creates a FamilyDeleted event.
func NewFamilyDeleted(familyID uuid.UUID, reason string) FamilyDeleted {
	return FamilyDeleted{
		BaseEvent: NewBaseEvent(),
		FamilyID:  familyID,
		Reason:    reason,
	}
}

// GedcomImported event is emitted after a GEDCOM file import.
type GedcomImported struct {
	BaseEvent
	ImportID         uuid.UUID `json:"import_id"`
	Filename         string    `json:"filename"`
	FileSize         int64     `json:"file_size"`
	PersonsImported  int       `json:"persons_imported"`
	FamiliesImported int       `json:"families_imported"`
	Warnings         []string  `json:"warnings,omitempty"`
	Errors           []string  `json:"errors,omitempty"`
}

func (e GedcomImported) EventType() string      { return "GedcomImported" }
func (e GedcomImported) AggregateID() uuid.UUID { return e.ImportID }

// NewGedcomImported creates a GedcomImported event.
func NewGedcomImported(filename string, fileSize int64, persons, families int, warnings, errors []string) GedcomImported {
	return GedcomImported{
		BaseEvent:        NewBaseEvent(),
		ImportID:         uuid.New(),
		Filename:         filename,
		FileSize:         fileSize,
		PersonsImported:  persons,
		FamiliesImported: families,
		Warnings:         warnings,
		Errors:           errors,
	}
}

// EventEnvelope wraps an event for storage with metadata.
type EventEnvelope struct {
	ID        uuid.UUID       `json:"id"`
	StreamID  uuid.UUID       `json:"stream_id"`
	Type      string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Version   int64           `json:"version"`
	Position  int64           `json:"position"`
	Timestamp time.Time       `json:"timestamp"`
}

// EventMetadata contains correlation and causation data for events.
type EventMetadata struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	CausationID   string `json:"causation_id,omitempty"`
	UserID        string `json:"user_id,omitempty"`
}

// SourceCreated event is emitted when a new source is created.
type SourceCreated struct {
	BaseEvent
	SourceID       uuid.UUID  `json:"source_id"`
	SourceType     SourceType `json:"source_type"`
	Title          string     `json:"title"`
	Author         string     `json:"author,omitempty"`
	Publisher      string     `json:"publisher,omitempty"`
	PublishDate    *GenDate   `json:"publish_date,omitempty"`
	URL            string     `json:"url,omitempty"`
	RepositoryID   *uuid.UUID `json:"repository_id,omitempty"`
	RepositoryName string     `json:"repository_name,omitempty"`
	CollectionName string     `json:"collection_name,omitempty"`
	CallNumber     string     `json:"call_number,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	GedcomXref     string     `json:"gedcom_xref,omitempty"`
}

func (e SourceCreated) EventType() string      { return "SourceCreated" }
func (e SourceCreated) AggregateID() uuid.UUID { return e.SourceID }

// NewSourceCreated creates a SourceCreated event from a Source.
func NewSourceCreated(s *Source) SourceCreated {
	return SourceCreated{
		BaseEvent:      NewBaseEvent(),
		SourceID:       s.ID,
		SourceType:     s.SourceType,
		Title:          s.Title,
		Author:         s.Author,
		Publisher:      s.Publisher,
		PublishDate:    s.PublishDate,
		URL:            s.URL,
		RepositoryID:   s.RepositoryID,
		RepositoryName: s.RepositoryName,
		CollectionName: s.CollectionName,
		CallNumber:     s.CallNumber,
		Notes:          s.Notes,
		GedcomXref:     s.GedcomXref,
	}
}

// SourceUpdated event is emitted when a source is updated.
type SourceUpdated struct {
	BaseEvent
	SourceID uuid.UUID      `json:"source_id"`
	Changes  map[string]any `json:"changes"`
}

func (e SourceUpdated) EventType() string      { return "SourceUpdated" }
func (e SourceUpdated) AggregateID() uuid.UUID { return e.SourceID }

// NewSourceUpdated creates a SourceUpdated event.
func NewSourceUpdated(sourceID uuid.UUID, changes map[string]any) SourceUpdated {
	return SourceUpdated{
		BaseEvent: NewBaseEvent(),
		SourceID:  sourceID,
		Changes:   changes,
	}
}

// SourceDeleted event is emitted when a source is deleted.
type SourceDeleted struct {
	BaseEvent
	SourceID uuid.UUID `json:"source_id"`
	Reason   string    `json:"reason,omitempty"`
}

func (e SourceDeleted) EventType() string      { return "SourceDeleted" }
func (e SourceDeleted) AggregateID() uuid.UUID { return e.SourceID }

// NewSourceDeleted creates a SourceDeleted event.
func NewSourceDeleted(sourceID uuid.UUID, reason string) SourceDeleted {
	return SourceDeleted{
		BaseEvent: NewBaseEvent(),
		SourceID:  sourceID,
		Reason:    reason,
	}
}

// CitationCreated event is emitted when a new citation is created.
type CitationCreated struct {
	BaseEvent
	CitationID    uuid.UUID     `json:"citation_id"`
	SourceID      uuid.UUID     `json:"source_id"`
	FactType      FactType      `json:"fact_type"`
	FactOwnerID   uuid.UUID     `json:"fact_owner_id"`
	Page          string        `json:"page,omitempty"`
	Volume        string        `json:"volume,omitempty"`
	SourceQuality SourceQuality `json:"source_quality,omitempty"`
	InformantType InformantType `json:"informant_type,omitempty"`
	EvidenceType  EvidenceType  `json:"evidence_type,omitempty"`
	QuotedText    string        `json:"quoted_text,omitempty"`
	Analysis      string        `json:"analysis,omitempty"`
	TemplateID    string        `json:"template_id,omitempty"`
	GedcomXref    string        `json:"gedcom_xref,omitempty"`
}

func (e CitationCreated) EventType() string      { return "CitationCreated" }
func (e CitationCreated) AggregateID() uuid.UUID { return e.CitationID }

// NewCitationCreated creates a CitationCreated event from a Citation.
func NewCitationCreated(c *Citation) CitationCreated {
	return CitationCreated{
		BaseEvent:     NewBaseEvent(),
		CitationID:    c.ID,
		SourceID:      c.SourceID,
		FactType:      c.FactType,
		FactOwnerID:   c.FactOwnerID,
		Page:          c.Page,
		Volume:        c.Volume,
		SourceQuality: c.SourceQuality,
		InformantType: c.InformantType,
		EvidenceType:  c.EvidenceType,
		QuotedText:    c.QuotedText,
		Analysis:      c.Analysis,
		TemplateID:    c.TemplateID,
		GedcomXref:    c.GedcomXref,
	}
}

// CitationUpdated event is emitted when a citation is updated.
type CitationUpdated struct {
	BaseEvent
	CitationID uuid.UUID      `json:"citation_id"`
	Changes    map[string]any `json:"changes"`
}

func (e CitationUpdated) EventType() string      { return "CitationUpdated" }
func (e CitationUpdated) AggregateID() uuid.UUID { return e.CitationID }

// NewCitationUpdated creates a CitationUpdated event.
func NewCitationUpdated(citationID uuid.UUID, changes map[string]any) CitationUpdated {
	return CitationUpdated{
		BaseEvent:  NewBaseEvent(),
		CitationID: citationID,
		Changes:    changes,
	}
}

// CitationDeleted event is emitted when a citation is deleted.
type CitationDeleted struct {
	BaseEvent
	CitationID uuid.UUID `json:"citation_id"`
	Reason     string    `json:"reason,omitempty"`
}

func (e CitationDeleted) EventType() string      { return "CitationDeleted" }
func (e CitationDeleted) AggregateID() uuid.UUID { return e.CitationID }

// NewCitationDeleted creates a CitationDeleted event.
func NewCitationDeleted(citationID uuid.UUID, reason string) CitationDeleted {
	return CitationDeleted{
		BaseEvent:  NewBaseEvent(),
		CitationID: citationID,
		Reason:     reason,
	}
}

// MediaCreated event is emitted when a new media is created.
type MediaCreated struct {
	BaseEvent
	MediaID       uuid.UUID `json:"media_id"`
	EntityType    string    `json:"entity_type"`
	EntityID      uuid.UUID `json:"entity_id"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	MimeType      string    `json:"mime_type"`
	MediaType     MediaType `json:"media_type"`
	Filename      string    `json:"filename"`
	FileSize      int64     `json:"file_size"`
	FileData      []byte    `json:"file_data"`
	ThumbnailData []byte    `json:"thumbnail_data,omitempty"`
	GedcomXref    string    `json:"gedcom_xref,omitempty"`
}

func (e MediaCreated) EventType() string      { return "MediaCreated" }
func (e MediaCreated) AggregateID() uuid.UUID { return e.MediaID }

// NewMediaCreated creates a MediaCreated event from a Media.
func NewMediaCreated(m *Media) MediaCreated {
	return MediaCreated{
		BaseEvent:     NewBaseEvent(),
		MediaID:       m.ID,
		EntityType:    m.EntityType,
		EntityID:      m.EntityID,
		Title:         m.Title,
		Description:   m.Description,
		MimeType:      m.MimeType,
		MediaType:     m.MediaType,
		Filename:      m.Filename,
		FileSize:      m.FileSize,
		FileData:      m.FileData,
		ThumbnailData: m.ThumbnailData,
		GedcomXref:    m.GedcomXref,
	}
}

// MediaUpdated event is emitted when a media is updated.
type MediaUpdated struct {
	BaseEvent
	MediaID uuid.UUID      `json:"media_id"`
	Changes map[string]any `json:"changes"`
}

func (e MediaUpdated) EventType() string      { return "MediaUpdated" }
func (e MediaUpdated) AggregateID() uuid.UUID { return e.MediaID }

// NewMediaUpdated creates a MediaUpdated event.
func NewMediaUpdated(mediaID uuid.UUID, changes map[string]any) MediaUpdated {
	return MediaUpdated{
		BaseEvent: NewBaseEvent(),
		MediaID:   mediaID,
		Changes:   changes,
	}
}

// MediaDeleted event is emitted when a media is deleted.
type MediaDeleted struct {
	BaseEvent
	MediaID uuid.UUID `json:"media_id"`
	Reason  string    `json:"reason,omitempty"`
}

func (e MediaDeleted) EventType() string      { return "MediaDeleted" }
func (e MediaDeleted) AggregateID() uuid.UUID { return e.MediaID }

// NewMediaDeleted creates a MediaDeleted event.
func NewMediaDeleted(mediaID uuid.UUID, reason string) MediaDeleted {
	return MediaDeleted{
		BaseEvent: NewBaseEvent(),
		MediaID:   mediaID,
		Reason:    reason,
	}
}

// RepositoryCreated event is emitted when a new repository is created.
type RepositoryCreated struct {
	BaseEvent
	RepositoryID uuid.UUID `json:"repository_id"`
	Name         string    `json:"name"`
	Address      string    `json:"address,omitempty"`
	City         string    `json:"city,omitempty"`
	State        string    `json:"state,omitempty"`
	PostalCode   string    `json:"postal_code,omitempty"`
	Country      string    `json:"country,omitempty"`
	Phone        string    `json:"phone,omitempty"`
	Email        string    `json:"email,omitempty"`
	Website      string    `json:"website,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	GedcomXref   string    `json:"gedcom_xref,omitempty"`
}

func (e RepositoryCreated) EventType() string      { return "RepositoryCreated" }
func (e RepositoryCreated) AggregateID() uuid.UUID { return e.RepositoryID }

// NewRepositoryCreated creates a RepositoryCreated event from a Repository.
func NewRepositoryCreated(r *Repository) RepositoryCreated {
	return RepositoryCreated{
		BaseEvent:    NewBaseEvent(),
		RepositoryID: r.ID,
		Name:         r.Name,
		Address:      r.Address,
		City:         r.City,
		State:        r.State,
		PostalCode:   r.PostalCode,
		Country:      r.Country,
		Phone:        r.Phone,
		Email:        r.Email,
		Website:      r.Website,
		Notes:        r.Notes,
		GedcomXref:   r.GedcomXref,
	}
}

// RepositoryUpdated event is emitted when a repository is updated.
type RepositoryUpdated struct {
	BaseEvent
	RepositoryID uuid.UUID      `json:"repository_id"`
	Changes      map[string]any `json:"changes"`
}

func (e RepositoryUpdated) EventType() string      { return "RepositoryUpdated" }
func (e RepositoryUpdated) AggregateID() uuid.UUID { return e.RepositoryID }

// NewRepositoryUpdated creates a RepositoryUpdated event.
func NewRepositoryUpdated(repositoryID uuid.UUID, changes map[string]any) RepositoryUpdated {
	return RepositoryUpdated{
		BaseEvent:    NewBaseEvent(),
		RepositoryID: repositoryID,
		Changes:      changes,
	}
}

// RepositoryDeleted event is emitted when a repository is deleted.
type RepositoryDeleted struct {
	BaseEvent
	RepositoryID uuid.UUID `json:"repository_id"`
	Reason       string    `json:"reason,omitempty"`
}

func (e RepositoryDeleted) EventType() string      { return "RepositoryDeleted" }
func (e RepositoryDeleted) AggregateID() uuid.UUID { return e.RepositoryID }

// NewRepositoryDeleted creates a RepositoryDeleted event.
func NewRepositoryDeleted(repositoryID uuid.UUID, reason string) RepositoryDeleted {
	return RepositoryDeleted{
		BaseEvent:    NewBaseEvent(),
		RepositoryID: repositoryID,
		Reason:       reason,
	}
}

// LifeEventCreated event is emitted when a new life event is created.
type LifeEventCreated struct {
	BaseEvent
	EventID     uuid.UUID  `json:"event_id"`
	PersonID    *uuid.UUID `json:"person_id,omitempty"`  // nil for family events
	FamilyID    *uuid.UUID `json:"family_id,omitempty"`  // nil for person events
	FactType    FactType   `json:"fact_type"`
	Date        *GenDate   `json:"date,omitempty"`
	Place       string     `json:"place,omitempty"`
	Description string     `json:"description,omitempty"`
	Cause       string     `json:"cause,omitempty"` // For death/burial events
	Age         string     `json:"age,omitempty"`   // Age at event
	GedcomXref  string     `json:"gedcom_xref,omitempty"`
}

func (e LifeEventCreated) EventType() string      { return "LifeEventCreated" }
func (e LifeEventCreated) AggregateID() uuid.UUID { return e.EventID }

// NewLifeEventCreatedFromModel creates a LifeEventCreated event from a LifeEvent model.
func NewLifeEventCreatedFromModel(le *LifeEvent) LifeEventCreated {
	return LifeEventCreated{
		BaseEvent:   NewBaseEvent(),
		EventID:     le.ID,
		PersonID:    le.PersonID,
		FamilyID:    le.FamilyID,
		FactType:    le.FactType,
		Date:        le.Date,
		Place:       le.Place,
		Description: le.Description,
		Cause:       le.Cause,
		Age:         le.Age,
		GedcomXref:  le.GedcomXref,
	}
}

// LifeEventUpdated event is emitted when a life event is updated.
type LifeEventUpdated struct {
	BaseEvent
	EventID uuid.UUID      `json:"event_id"`
	Changes map[string]any `json:"changes"`
}

func (e LifeEventUpdated) EventType() string      { return "LifeEventUpdated" }
func (e LifeEventUpdated) AggregateID() uuid.UUID { return e.EventID }

// NewLifeEventUpdated creates a LifeEventUpdated event.
func NewLifeEventUpdated(eventID uuid.UUID, changes map[string]any) LifeEventUpdated {
	return LifeEventUpdated{
		BaseEvent: NewBaseEvent(),
		EventID:   eventID,
		Changes:   changes,
	}
}

// LifeEventDeleted event is emitted when a life event is deleted.
type LifeEventDeleted struct {
	BaseEvent
	EventID uuid.UUID `json:"event_id"`
	Reason  string    `json:"reason,omitempty"`
}

func (e LifeEventDeleted) EventType() string      { return "LifeEventDeleted" }
func (e LifeEventDeleted) AggregateID() uuid.UUID { return e.EventID }

// NewLifeEventDeleted creates a LifeEventDeleted event.
func NewLifeEventDeleted(eventID uuid.UUID, reason string) LifeEventDeleted {
	return LifeEventDeleted{
		BaseEvent: NewBaseEvent(),
		EventID:   eventID,
		Reason:    reason,
	}
}

// AttributeCreated event is emitted when a new person attribute is created.
type AttributeCreated struct {
	BaseEvent
	AttributeID uuid.UUID `json:"attribute_id"`
	PersonID    uuid.UUID `json:"person_id"`
	FactType    FactType  `json:"fact_type"`
	Value       string    `json:"value"`
	Date        *GenDate  `json:"date,omitempty"`
	Place       string    `json:"place,omitempty"`
	GedcomXref  string    `json:"gedcom_xref,omitempty"`
}

func (e AttributeCreated) EventType() string      { return "AttributeCreated" }
func (e AttributeCreated) AggregateID() uuid.UUID { return e.AttributeID }

// NewAttributeCreatedFromModel creates an AttributeCreated event from an Attribute model.
func NewAttributeCreatedFromModel(a *Attribute) AttributeCreated {
	return AttributeCreated{
		BaseEvent:   NewBaseEvent(),
		AttributeID: a.ID,
		PersonID:    a.PersonID,
		FactType:    a.FactType,
		Value:       a.Value,
		Date:        a.Date,
		Place:       a.Place,
		GedcomXref:  a.GedcomXref,
	}
}

// AttributeUpdated event is emitted when an attribute is updated.
type AttributeUpdated struct {
	BaseEvent
	AttributeID uuid.UUID      `json:"attribute_id"`
	Changes     map[string]any `json:"changes"`
}

func (e AttributeUpdated) EventType() string      { return "AttributeUpdated" }
func (e AttributeUpdated) AggregateID() uuid.UUID { return e.AttributeID }

// NewAttributeUpdated creates an AttributeUpdated event.
func NewAttributeUpdated(attributeID uuid.UUID, changes map[string]any) AttributeUpdated {
	return AttributeUpdated{
		BaseEvent:   NewBaseEvent(),
		AttributeID: attributeID,
		Changes:     changes,
	}
}

// AttributeDeleted event is emitted when a person attribute is deleted.
type AttributeDeleted struct {
	BaseEvent
	AttributeID uuid.UUID `json:"attribute_id"`
	Reason      string    `json:"reason,omitempty"`
}

func (e AttributeDeleted) EventType() string      { return "AttributeDeleted" }
func (e AttributeDeleted) AggregateID() uuid.UUID { return e.AttributeID }

// NewAttributeDeleted creates an AttributeDeleted event.
func NewAttributeDeleted(attributeID uuid.UUID, reason string) AttributeDeleted {
	return AttributeDeleted{
		BaseEvent:   NewBaseEvent(),
		AttributeID: attributeID,
		Reason:      reason,
	}
}
