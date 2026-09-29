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
	PersonID       uuid.UUID      `json:"person_id"`
	GivenName      string         `json:"given_name"`
	Surname        string         `json:"surname"`
	NamePrefix     string         `json:"name_prefix,omitempty"`
	NameSuffix     string         `json:"name_suffix,omitempty"`
	SurnamePrefix  string         `json:"surname_prefix,omitempty"`
	Nickname       string         `json:"nickname,omitempty"`
	NameType       NameType       `json:"name_type,omitempty"`
	Gender         Gender         `json:"gender,omitempty"`
	BirthDate      *GenDate       `json:"birth_date,omitempty"`
	BirthPlace     string         `json:"birth_place,omitempty"`
	DeathDate      *GenDate       `json:"death_date,omitempty"`
	DeathPlace     string         `json:"death_place,omitempty"`
	Notes          string         `json:"notes,omitempty"`
	ResearchStatus ResearchStatus `json:"research_status,omitempty"`
	GedcomXref     string         `json:"gedcom_xref,omitempty"`
}

func (e PersonCreated) EventType() string      { return "PersonCreated" }
func (e PersonCreated) AggregateID() uuid.UUID { return e.PersonID }

// NewPersonCreated creates a PersonCreated event from a Person.
func NewPersonCreated(p *Person) PersonCreated {
	return PersonCreated{
		BaseEvent:      NewBaseEvent(),
		PersonID:       p.ID,
		GivenName:      p.GivenName,
		Surname:        p.Surname,
		NamePrefix:     p.NamePrefix,
		NameSuffix:     p.NameSuffix,
		SurnamePrefix:  p.SurnamePrefix,
		Nickname:       p.Nickname,
		NameType:       p.NameType,
		Gender:         p.Gender,
		BirthDate:      p.BirthDate,
		BirthPlace:     p.BirthPlace,
		DeathDate:      p.DeathDate,
		DeathPlace:     p.DeathPlace,
		Notes:          p.Notes,
		ResearchStatus: p.ResearchStatus,
		GedcomXref:     p.GedcomXref,
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

// EventMetadata contains correlation and causation data for events. It is
// stored in the event envelope (the event store's metadata column), never in
// the payload, so a payload stays byte-identical wherever it is re-appended.
type EventMetadata struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	CausationID   string `json:"causation_id,omitempty"`
	UserID        string `json:"user_id,omitempty"`
	// MergedFromBranch is set on a mainline event a merge replayed from a
	// research branch (#832): which branch, which merge, and why. Absent on
	// every other event, including replays written before #832.
	MergedFromBranch *MergeProvenance `json:"merged_from_branch,omitempty"`
}

// IsZero reports whether the metadata carries nothing worth storing.
func (m EventMetadata) IsZero() bool {
	return m.CorrelationID == "" && m.CausationID == "" && m.UserID == "" && m.MergedFromBranch == nil
}

// MergeProvenance links a replayed mainline event to the merge that wrote it
// (#832). Every field comes from the branch and its BranchMerged claim, so a
// merge and any resume of it stamp every replayed event identically.
type MergeProvenance struct {
	// BranchID and BranchName name the research branch the change came from,
	// as it was called when the merge was claimed.
	BranchID   uuid.UUID `json:"id"`
	BranchName string    `json:"name"`
	// ClaimID is the BranchMerged event's id, and MergedAtPosition the log
	// head it recorded: together they identify the merge.
	ClaimID          uuid.UUID `json:"claim_id"`
	MergedAtPosition int64     `json:"merged_at_position"`
	// MergedAt is when the merge was claimed. The event store records a
	// replayed event at this time, so the mainline's history shows the change
	// when it reached the mainline; the payload keeps the time it was made.
	MergedAt time.Time `json:"merged_at"`
	// Note is the merge note: the "why" of the promotion.
	Note string `json:"note,omitempty"`
}

// StampedEvent carries envelope data for the event store alongside an event:
// the metadata to store with it and the time to record it at. It delegates
// the Event methods to the wrapped event, and the stores unwrap it
// (UnwrapEvent) before encoding, so the stored payload is the inner event's
// alone. Projections take the inner event.
type StampedEvent struct {
	Event      Event
	Metadata   EventMetadata
	RecordedAt time.Time
}

func (s StampedEvent) EventType() string      { return s.Event.EventType() }
func (s StampedEvent) AggregateID() uuid.UUID { return s.Event.AggregateID() }
func (s StampedEvent) OccurredAt() time.Time  { return s.Event.OccurredAt() }

// Stamp wraps event with the metadata the store keeps beside it and the time
// the store records it at. A zero recordedAt records it at its OccurredAt.
func Stamp(event Event, metadata EventMetadata, recordedAt time.Time) Event {
	return StampedEvent{Event: event, Metadata: metadata, RecordedAt: recordedAt}
}

// UnwrapEvent splits an event handed to an event store into the payload to
// encode, the metadata to store (nil when there is none) and the time to
// record it at: the stamp's RecordedAt when set, else the event's OccurredAt.
func UnwrapEvent(event Event) (Event, *EventMetadata, time.Time) {
	stamped, ok := event.(StampedEvent)
	if !ok {
		return event, nil, event.OccurredAt()
	}
	inner, _, at := UnwrapEvent(stamped.Event)
	if !stamped.RecordedAt.IsZero() {
		at = stamped.RecordedAt
	}
	if stamped.Metadata.IsZero() {
		return inner, nil, at
	}
	meta := stamped.Metadata
	return inner, &meta, at
}

// EncodeForStore is UnwrapEvent plus the JSON encoding every store needs: the
// payload, the metadata (nil when there is none) and the record time.
func EncodeForStore(event Event) (data, metadata []byte, recordedAt time.Time, err error) {
	inner, meta, at := UnwrapEvent(event)
	data, err = json.Marshal(inner)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	if meta != nil {
		metadata, err = json.Marshal(meta)
		if err != nil {
			return nil, nil, time.Time{}, err
		}
	}
	return data, metadata, at, nil
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
	CitationID    uuid.UUID         `json:"citation_id"`
	SourceID      uuid.UUID         `json:"source_id"`
	FactType      FactType          `json:"fact_type"`
	FactOwnerID   uuid.UUID         `json:"fact_owner_id"`
	Page          string            `json:"page,omitempty"`
	Volume        string            `json:"volume,omitempty"`
	SourceQuality SourceQuality     `json:"source_quality,omitempty"`
	InformantType InformantType     `json:"informant_type,omitempty"`
	EvidenceType  EvidenceType      `json:"evidence_type,omitempty"`
	QuotedText    string            `json:"quoted_text,omitempty"`
	Analysis      string            `json:"analysis,omitempty"`
	TemplateID    string            `json:"template_id,omitempty"`
	Fields        map[string]string `json:"fields,omitempty"`
	GedcomXref    string            `json:"gedcom_xref,omitempty"`
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
		Fields:        c.Fields,
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
	// GEDCOM 7.0 enhanced fields
	Files        []MediaFile `json:"files,omitempty"`        // Multiple file references
	Format       string      `json:"format,omitempty"`       // Primary format/MIME type
	Translations []string    `json:"translations,omitempty"` // Translated titles
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
		Files:         m.Files,
		Format:        m.Format,
		Translations:  m.Translations,
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
	RepositoryID  uuid.UUID `json:"repository_id"`
	Name          string    `json:"name"`
	StreetAddress string    `json:"street_address,omitempty"`
	City          string    `json:"city,omitempty"`
	State         string    `json:"state,omitempty"`
	PostalCode    string    `json:"postal_code,omitempty"`
	Country       string    `json:"country,omitempty"`
	Phone         string    `json:"phone,omitempty"`
	Email         string    `json:"email,omitempty"`
	Website       string    `json:"website,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	GedcomXref    string    `json:"gedcom_xref,omitempty"`
}

func (e RepositoryCreated) EventType() string      { return "RepositoryCreated" }
func (e RepositoryCreated) AggregateID() uuid.UUID { return e.RepositoryID }

// NewRepositoryCreated creates a RepositoryCreated event from a Repository.
func NewRepositoryCreated(r *Repository) RepositoryCreated {
	return RepositoryCreated{
		BaseEvent:     NewBaseEvent(),
		RepositoryID:  r.ID,
		Name:          r.Name,
		StreetAddress: r.StreetAddress,
		City:          r.City,
		State:         r.State,
		PostalCode:    r.PostalCode,
		Country:       r.Country,
		Phone:         r.Phone,
		Email:         r.Email,
		Website:       r.Website,
		Notes:         r.Notes,
		GedcomXref:    r.GedcomXref,
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
	PersonID    *uuid.UUID `json:"person_id,omitempty"` // nil for family events
	FamilyID    *uuid.UUID `json:"family_id,omitempty"` // nil for person events
	FactType    FactType   `json:"fact_type"`
	Date        *GenDate   `json:"date,omitempty"`
	Place       string     `json:"place,omitempty"`
	Address     *Address   `json:"address,omitempty"` // Structured address (RESI, etc.)
	Description string     `json:"description,omitempty"`
	Cause       string     `json:"cause,omitempty"`      // For death/burial events
	Age         string     `json:"age,omitempty"`        // Age at event
	IsNegated   bool       `json:"is_negated,omitempty"` // Negative assertion (GEDCOM 7.0 NO tag)
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
		Address:     le.Address,
		Description: le.Description,
		Cause:       le.Cause,
		Age:         le.Age,
		IsNegated:   le.IsNegated,
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

// NameAdded event is emitted when a name is added to a person.
type NameAdded struct {
	BaseEvent
	PersonID      uuid.UUID `json:"person_id"`
	NameID        uuid.UUID `json:"name_id"`
	GivenName     string    `json:"given_name"`
	Surname       string    `json:"surname,omitempty"`
	NamePrefix    string    `json:"name_prefix,omitempty"`
	NameSuffix    string    `json:"name_suffix,omitempty"`
	SurnamePrefix string    `json:"surname_prefix,omitempty"`
	Nickname      string    `json:"nickname,omitempty"`
	NameType      NameType  `json:"name_type,omitempty"`
	IsPrimary     bool      `json:"is_primary"`
}

func (e NameAdded) EventType() string      { return "NameAdded" }
func (e NameAdded) AggregateID() uuid.UUID { return e.PersonID }

// NewNameAdded creates a NameAdded event from a PersonName.
func NewNameAdded(pn *PersonName) NameAdded {
	return NameAdded{
		BaseEvent:     NewBaseEvent(),
		PersonID:      pn.PersonID,
		NameID:        pn.ID,
		GivenName:     pn.GivenName,
		Surname:       pn.Surname,
		NamePrefix:    pn.NamePrefix,
		NameSuffix:    pn.NameSuffix,
		SurnamePrefix: pn.SurnamePrefix,
		Nickname:      pn.Nickname,
		NameType:      pn.NameType,
		IsPrimary:     pn.IsPrimary,
	}
}

// NameUpdated event is emitted when a name is modified.
type NameUpdated struct {
	BaseEvent
	PersonID      uuid.UUID `json:"person_id"`
	NameID        uuid.UUID `json:"name_id"`
	GivenName     string    `json:"given_name"`
	Surname       string    `json:"surname,omitempty"`
	NamePrefix    string    `json:"name_prefix,omitempty"`
	NameSuffix    string    `json:"name_suffix,omitempty"`
	SurnamePrefix string    `json:"surname_prefix,omitempty"`
	Nickname      string    `json:"nickname,omitempty"`
	NameType      NameType  `json:"name_type,omitempty"`
	IsPrimary     bool      `json:"is_primary"`
}

func (e NameUpdated) EventType() string      { return "NameUpdated" }
func (e NameUpdated) AggregateID() uuid.UUID { return e.PersonID }

// NewNameUpdated creates a NameUpdated event from a PersonName.
func NewNameUpdated(pn *PersonName) NameUpdated {
	return NameUpdated{
		BaseEvent:     NewBaseEvent(),
		PersonID:      pn.PersonID,
		NameID:        pn.ID,
		GivenName:     pn.GivenName,
		Surname:       pn.Surname,
		NamePrefix:    pn.NamePrefix,
		NameSuffix:    pn.NameSuffix,
		SurnamePrefix: pn.SurnamePrefix,
		Nickname:      pn.Nickname,
		NameType:      pn.NameType,
		IsPrimary:     pn.IsPrimary,
	}
}

// NameRemoved event is emitted when a name is deleted.
type NameRemoved struct {
	BaseEvent
	PersonID uuid.UUID `json:"person_id"`
	NameID   uuid.UUID `json:"name_id"`
}

func (e NameRemoved) EventType() string      { return "NameRemoved" }
func (e NameRemoved) AggregateID() uuid.UUID { return e.PersonID }

// NewNameRemoved creates a NameRemoved event.
func NewNameRemoved(personID, nameID uuid.UUID) NameRemoved {
	return NameRemoved{
		BaseEvent: NewBaseEvent(),
		PersonID:  personID,
		NameID:    nameID,
	}
}

// SnapshotCreated event is emitted when a new snapshot is created (issue #624).
//
// Position is the log head captured BEFORE this event was appended, so a
// snapshot never includes its own creation event in the range it marks. That
// ordering is what makes the event safe: emitting it moves the head, but not
// the position the snapshot points at.
//
// BranchID is the branch whose view the snapshot marks (issue #839). Events
// written before #839 have no branch_id field and decode to uuid.Nil, which is
// the mainline — exactly what every snapshot was before branches could take
// them. Like the branch lifecycle events, the payload carries a plain uuid.UUID;
// wrap it as domain.BranchID(e.BranchID) to use it as a scope.
type SnapshotCreated struct {
	BaseEvent
	SnapshotID  uuid.UUID `json:"snapshot_id"`
	BranchID    uuid.UUID `json:"branch_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Position    int64     `json:"position"`
}

func (e SnapshotCreated) EventType() string      { return "SnapshotCreated" }
func (e SnapshotCreated) AggregateID() uuid.UUID { return e.SnapshotID }

// NewSnapshotCreated creates a SnapshotCreated event from a Snapshot.
func NewSnapshotCreated(s *Snapshot) SnapshotCreated {
	return SnapshotCreated{
		BaseEvent:   NewBaseEvent(),
		SnapshotID:  s.ID,
		BranchID:    s.BranchID.UUID(),
		Name:        s.Name,
		Description: s.Description,
		Position:    s.Position,
	}
}

// SnapshotDeleted event is emitted when a snapshot marker is removed. The events
// the snapshot pointed at are untouched — the log is append-only (ES-002) and a
// snapshot is only a named pointer into it.
//
// BranchID records the branch the deleted snapshot belonged to, for the audit
// trail (issue #839); the projection deletes by SnapshotID alone. Events written
// before #839 decode it as uuid.Nil, the mainline.
type SnapshotDeleted struct {
	BaseEvent
	SnapshotID uuid.UUID `json:"snapshot_id"`
	BranchID   uuid.UUID `json:"branch_id"`
}

func (e SnapshotDeleted) EventType() string      { return "SnapshotDeleted" }
func (e SnapshotDeleted) AggregateID() uuid.UUID { return e.SnapshotID }

// NewSnapshotDeleted creates a SnapshotDeleted event for a snapshot on branchID.
func NewSnapshotDeleted(snapshotID uuid.UUID, branchID BranchID) SnapshotDeleted {
	return SnapshotDeleted{
		BaseEvent:  NewBaseEvent(),
		SnapshotID: snapshotID,
		BranchID:   branchID.UUID(),
	}
}

// BranchCreated event is emitted when a new research branch is established
// off main at BasePosition. Note: the BranchID field here (and on BranchDeleted /
// BranchMerged) is the branch ENTITY's identity (uuid.UUID), NOT the branch-scope
// type domain.BranchID — wrap it as domain.BranchID(e.BranchID) to use it as a
// read/write scope. See the doc comment on domain.BranchID.
type BranchCreated struct {
	BaseEvent
	BranchID     uuid.UUID `json:"branch_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	BasePosition int64     `json:"base_position"`

	// The research record the branch was created with (#835). All optional:
	// an event written before #835 carries none of them, and an empty Outcome
	// is read as open.
	Hypothesis      string          `json:"hypothesis,omitempty"`
	Subjects        []BranchSubject `json:"subjects,omitempty"`
	Outcome         BranchOutcome   `json:"outcome,omitempty"`
	ProofSummaryIDs []uuid.UUID     `json:"proof_summary_ids,omitempty"`
}

// Research returns the research record carried by the event, with an empty
// outcome defaulted to open.
func (e BranchCreated) Research() BranchResearch {
	return BranchResearch{
		Hypothesis:      e.Hypothesis,
		Subjects:        append([]BranchSubject(nil), e.Subjects...),
		Outcome:         e.Outcome.OrDefault(),
		ProofSummaryIDs: append([]uuid.UUID(nil), e.ProofSummaryIDs...),
	}
}

func (e BranchCreated) EventType() string      { return "BranchCreated" }
func (e BranchCreated) AggregateID() uuid.UUID { return e.BranchID }

// NewBranchCreated creates a BranchCreated event from a Branch.
func NewBranchCreated(b *Branch) BranchCreated {
	return BranchCreated{
		BaseEvent:    NewBaseEvent(),
		BranchID:     b.ID,
		Name:         b.Name,
		Description:  b.Description,
		BasePosition: b.BasePosition,

		Hypothesis:      b.Hypothesis,
		Subjects:        append([]BranchSubject(nil), b.Subjects...),
		Outcome:         b.Outcome.OrDefault(),
		ProofSummaryIDs: append([]uuid.UUID(nil), b.ProofSummaryIDs...),
	}
}

// BranchUpdated event is emitted when a branch's description or research
// record (#835) is edited. It carries the full post-edit value of every
// editable field, not only the changed ones, so the projection is a plain
// overwrite and replaying the log in order always reconstructs the registry.
// ChangedFields names what the edit actually changed, for the audit trail.
//
// It is appended to the branch's own stream. It is research metadata, not a
// genealogy change: it is never replayed onto main by a merge and never shown
// as a branch change in a comparison (see researchMetadataEventTypes in
// internal/query/branch_queries.go).
type BranchUpdated struct {
	BaseEvent
	BranchID        uuid.UUID       `json:"branch_id"`
	Description     string          `json:"description"`
	Hypothesis      string          `json:"hypothesis"`
	Subjects        []BranchSubject `json:"subjects"`
	Outcome         BranchOutcome   `json:"outcome"`
	ProofSummaryIDs []uuid.UUID     `json:"proof_summary_ids"`
	ChangedFields   []string        `json:"changed_fields"`
}

func (e BranchUpdated) EventType() string      { return "BranchUpdated" }
func (e BranchUpdated) AggregateID() uuid.UUID { return e.BranchID }

// Research returns the research record carried by the event, with an empty
// outcome defaulted to open.
func (e BranchUpdated) Research() BranchResearch {
	return BranchResearch{
		Hypothesis:      e.Hypothesis,
		Subjects:        append([]BranchSubject(nil), e.Subjects...),
		Outcome:         e.Outcome.OrDefault(),
		ProofSummaryIDs: append([]uuid.UUID(nil), e.ProofSummaryIDs...),
	}
}

// NewBranchUpdated creates a BranchUpdated event from the branch as it stands
// after the edit. The slices are copied and stored as [] rather than null.
func NewBranchUpdated(b *Branch, changedFields []string) BranchUpdated {
	subjects := make([]BranchSubject, len(b.Subjects))
	copy(subjects, b.Subjects)
	proofs := make([]uuid.UUID, len(b.ProofSummaryIDs))
	copy(proofs, b.ProofSummaryIDs)
	changed := make([]string, len(changedFields))
	copy(changed, changedFields)
	return BranchUpdated{
		BaseEvent:       NewBaseEvent(),
		BranchID:        b.ID,
		Description:     b.Description,
		Hypothesis:      b.Hypothesis,
		Subjects:        subjects,
		Outcome:         b.Outcome.OrDefault(),
		ProofSummaryIDs: proofs,
		ChangedFields:   changed,
	}
}

// BranchDeleted event is emitted when a branch is archived/discarded. Despite the
// name, it transitions the branch to BranchStatusArchived (there is no "deleted"
// status) — "delete" is the user action, "archived" the retained terminal state.
// Append-only: it records the deletion as a new event and does not remove
// the branch's prior events from the log (ES-002).
type BranchDeleted struct {
	BaseEvent
	BranchID uuid.UUID `json:"branch_id"`
}

func (e BranchDeleted) EventType() string      { return "BranchDeleted" }
func (e BranchDeleted) AggregateID() uuid.UUID { return e.BranchID }

// NewBranchDeleted creates a BranchDeleted event.
func NewBranchDeleted(branchID uuid.UUID) BranchDeleted {
	return BranchDeleted{
		BaseEvent: NewBaseEvent(),
		BranchID:  branchID,
	}
}

// BranchMerged event is emitted when a branch's changes are promoted to main.
// It is the merge *marker* only — the promoted changes are separate, replayed
// main events (ADR-005 §Merge), so this event is never itself replayed.
//
// MergedAtPosition is the EVENT LOG's head Position at the moment of the merge:
// every main event after it, on the branch's streams, is the replay. Together
// with BasePosition (the log head the branch forked from, read the same way) it
// bounds the promotion in main's log.
//
// "Log head", not "mainline head", deliberately: Position is one global
// sequence shared by every branch (ADR-005 §The model), and GetMaxPosition does
// not filter by branch, so this number counts other branches' events too. That
// is consistent — BasePosition is read from the same unfiltered sequence and
// the tail scans compare against it — but it means the difference between the
// two is not a count of this merge's events, and arithmetic on it will be
// inflated by unrelated branch activity.
//
// Note carries the researcher's rationale for the merge — the "merge commit
// message" that makes the promotion reviewable after the fact (#55).
//
// ReplayStreamVersions is the replay PLAN the claim committed to (#685): one
// entry per stream the merge will replay onto main, mapped to main's version of
// that stream when the conflict verdict was computed. A stream the branch
// touched but that is absent here was resolved to "main" and is deliberately
// not replayed. Recording it on the claim is what makes an interrupted replay
// resumable: the resolutions and the #698 staleness pins survive the request
// that chose them, in the log, rather than only in that request's memory.
//
// The tag has NO omitempty, on purpose. A merge that replays nothing records
// `{}`, which decodes to an empty non-nil map; a claim written before #685
// carries no key at all and decodes to nil. Those are different facts — "the
// plan was: replay nothing" versus "the plan was not recorded" — and resume
// must be able to tell them apart (see command.Handler.ResumeMerge).
type BranchMerged struct {
	BaseEvent
	BranchID             uuid.UUID           `json:"branch_id"`
	BasePosition         int64               `json:"base_position"`
	MergedAtPosition     int64               `json:"merged_at_position"`
	Note                 string              `json:"note,omitempty"`
	ReplayStreamVersions map[uuid.UUID]int64 `json:"replay_stream_versions"`
	// ResolutionRationales records, per stream, why the researcher chose the
	// side they did for a conflict (#828): the evidence weighed, where GPS asks
	// a conflict to be resolved by reasoning. Optional and additive — claims
	// written before it, or with no rationale given, omit it and decode to nil.
	// Which side won is the replay plan: a stream in ReplayStreamVersions took
	// the branch, one absent from it kept main.
	//
	// Since #832 the rationale of a conflict decision or an exclusion is also
	// copied onto its entry in Resolutions or Exclusions; this map stays the
	// complete record (a rationale may explain a non-conflicting "branch"
	// resolution too, which has no entry there).
	ResolutionRationales map[uuid.UUID]string `json:"resolution_rationales,omitempty"`

	// The merge record (#832): what was decided and what was left behind, as
	// the researcher saw it when the merge was claimed. All optional and
	// additive — a claim written before #832 omits them and decodes to nil,
	// which is how a reader tells "not recorded" from "nothing decided"
	// (HasRecord).
	//
	// Resolutions are the conflict decisions, one per conflicting entity, with
	// the conflict as it was reviewed. Exclusions are the entities the branch
	// changed without conflict but that were left out (resolved to main).
	Resolutions []MergeDecision  `json:"resolutions,omitempty"`
	Exclusions  []MergeExclusion `json:"exclusions,omitempty"`
	// ReplayedEventCount is how many branch events the merge set out to
	// replay onto main (a resume can later leave some behind; see
	// BranchMergeResumed). SkippedStreamIDs are the entities resolved to main
	// — conflicts and exclusions alike — whose branch events are not replayed.
	ReplayedEventCount *int        `json:"replayed_event_count,omitempty"`
	SkippedStreamIDs   []uuid.UUID `json:"skipped_stream_ids,omitempty"`

	// PreMergeSnapshotID is the mainline snapshot the merge took just before
	// it claimed the branch (#833): "Before merging <branch>", marking the
	// mainline as it stood before the merge's first replayed event. Optional
	// and additive — a merge that took no snapshot, or a claim written before
	// #833, omits it and decodes to nil.
	PreMergeSnapshotID *uuid.UUID `json:"pre_merge_snapshot_id,omitempty"`
}

// HasRecord reports whether the claim carries a #832 merge record. An older
// claim records only its note and replay plan.
func (e BranchMerged) HasRecord() bool { return e.ReplayedEventCount != nil }

// MergeDecision is one conflict decision on a merge record (#832).
type MergeDecision struct {
	StreamID   uuid.UUID `json:"stream_id"`
	EntityType string    `json:"entity_type,omitempty"`
	EntityName string    `json:"entity_name,omitempty"`
	// Kind is the conflict class (edit_edit, delete_edit, create_create);
	// Fields the contested fields of an edit_edit; DeletedBy the side that
	// deleted, for a delete_edit.
	Kind      string   `json:"kind"`
	Fields    []string `json:"fields,omitempty"`
	DeletedBy string   `json:"deleted_by,omitempty"`
	// Resolution is the side that won: "branch" or "main".
	Resolution string `json:"resolution"`
	Rationale  string `json:"rationale,omitempty"`
}

// MergeExclusion is one entity a merge left behind without a conflict (#832).
type MergeExclusion struct {
	StreamID   uuid.UUID `json:"stream_id"`
	EntityType string    `json:"entity_type,omitempty"`
	EntityName string    `json:"entity_name,omitempty"`
	Rationale  string    `json:"rationale,omitempty"`
}

func (e BranchMerged) EventType() string      { return "BranchMerged" }
func (e BranchMerged) AggregateID() uuid.UUID { return e.BranchID }

// NewBranchMerged creates a BranchMerged event. note may be empty — a merge
// rationale is encouraged but not required. replayStreamVersions is the replay
// plan (see BranchMerged.ReplayStreamVersions); a nil map is stored as `{}`, so
// every claim this constructor builds records its plan, even an empty one. The
// map is copied, so the caller may reuse its own.
func NewBranchMerged(branchID uuid.UUID, basePosition, mergedAtPosition int64, note string, replayStreamVersions map[uuid.UUID]int64) BranchMerged {
	plan := make(map[uuid.UUID]int64, len(replayStreamVersions))
	for streamID, version := range replayStreamVersions {
		plan[streamID] = version
	}
	return BranchMerged{
		BaseEvent:            NewBaseEvent(),
		BranchID:             branchID,
		BasePosition:         basePosition,
		MergedAtPosition:     mergedAtPosition,
		Note:                 note,
		ReplayStreamVersions: plan,
	}
}

// BranchMergeResumed records the decisions a resumed merge made (#685). It is
// appended to the branch's OWN stream, after the BranchMerged claim, by
// command.Handler.ResumeMerge — and only when that resume had to decide
// something the claim could not (a stream main moved on after the claim, or a
// claim that predates #685 and recorded no plan). Like BranchMerged it is a
// marker and is never replayed onto main.
//
// ReplayStreamVersions is the replay plan AS IT NOW STANDS, replacing the
// claim's (and any earlier BranchMergeResumed's) in full, with the same
// meaning as BranchMerged.ReplayStreamVersions: every stream the merge
// replays, mapped to the main version its replay asserts; a stream the branch
// touched but absent here was resolved to "main". Recording the whole plan,
// rather than only the delta, keeps "what is the plan now" a matter of reading
// the latest marker. Resolutions is the audit record of what this resume was
// asked to decide ("branch" or "main" per stream).
//
// Without this record a resume-time "main" decision would exist only as the
// absence of replayed events, so the next resume would find the stream
// unreplayed and stale again and ask for — and accept — a fresh decision,
// letting a second request quietly reverse what the first one reviewed.
type BranchMergeResumed struct {
	BaseEvent
	BranchID             uuid.UUID            `json:"branch_id"`
	MergedAtPosition     int64                `json:"merged_at_position"`
	ReplayStreamVersions map[uuid.UUID]int64  `json:"replay_stream_versions"`
	Resolutions          map[uuid.UUID]string `json:"resolutions"`
	// Rationales is the optional reasoning given for Resolutions (#828), by
	// stream. Additive: omitted when none was given.
	Rationales map[uuid.UUID]string `json:"rationales,omitempty"`
}

func (e BranchMergeResumed) EventType() string      { return "BranchMergeResumed" }
func (e BranchMergeResumed) AggregateID() uuid.UUID { return e.BranchID }

// NewBranchMergeResumed creates a BranchMergeResumed event. Both maps are
// copied, and a nil map is stored as `{}`.
func NewBranchMergeResumed(branchID uuid.UUID, mergedAtPosition int64, replayStreamVersions map[uuid.UUID]int64, resolutions map[uuid.UUID]string) BranchMergeResumed {
	plan := make(map[uuid.UUID]int64, len(replayStreamVersions))
	for streamID, version := range replayStreamVersions {
		plan[streamID] = version
	}
	decided := make(map[uuid.UUID]string, len(resolutions))
	for streamID, side := range resolutions {
		decided[streamID] = side
	}
	return BranchMergeResumed{
		BaseEvent:            NewBaseEvent(),
		BranchID:             branchID,
		MergedAtPosition:     mergedAtPosition,
		ReplayStreamVersions: plan,
		Resolutions:          decided,
	}
}

// PersonMerged event is emitted when two persons are merged into one.
// The survivor person continues to exist with merged data; the merged person is deleted.
type PersonMerged struct {
	BaseEvent
	SurvivorID           uuid.UUID      `json:"survivor_id"`
	MergedID             uuid.UUID      `json:"merged_id"`
	MergedPersonSnapshot map[string]any `json:"merged_person_snapshot"` // Full state for audit/unmerge
	ResolvedFields       map[string]any `json:"resolved_fields"`        // Fields merged into survivor
	AffectedFamilyIDs    []uuid.UUID    `json:"affected_family_ids"`
	AffectedCitationIDs  []uuid.UUID    `json:"affected_citation_ids"`
	TransferredNameIDs   []uuid.UUID    `json:"transferred_name_ids"`
	TransferredEventIDs  []uuid.UUID    `json:"transferred_event_ids"`
	TransferredMediaIDs  []uuid.UUID    `json:"transferred_media_ids"`
}

func (e PersonMerged) EventType() string      { return "PersonMerged" }
func (e PersonMerged) AggregateID() uuid.UUID { return e.SurvivorID }

// NewPersonMerged creates a PersonMerged event.
func NewPersonMerged(
	survivorID, mergedID uuid.UUID,
	mergedSnapshot, resolvedFields map[string]any,
	affectedFamilies, affectedCitations, transferredNames, transferredEvents, transferredMedia []uuid.UUID,
) PersonMerged {
	return PersonMerged{
		BaseEvent:            NewBaseEvent(),
		SurvivorID:           survivorID,
		MergedID:             mergedID,
		MergedPersonSnapshot: mergedSnapshot,
		ResolvedFields:       resolvedFields,
		AffectedFamilyIDs:    affectedFamilies,
		AffectedCitationIDs:  affectedCitations,
		TransferredNameIDs:   transferredNames,
		TransferredEventIDs:  transferredEvents,
		TransferredMediaIDs:  transferredMedia,
	}
}

// NoteCreated event is emitted when a new note is created.
type NoteCreated struct {
	BaseEvent
	NoteID       uuid.UUID         `json:"note_id"`
	Text         string            `json:"text"`
	MIME         string            `json:"mime,omitempty"`         // GEDCOM 7.0 SNOTE media type
	Language     string            `json:"language,omitempty"`     // GEDCOM 7.0 SNOTE BCP 47 language tag
	Translations []NoteTranslation `json:"translations,omitempty"` // GEDCOM 7.0 SNOTE alternate-language renderings
	GedcomXref   string            `json:"gedcom_xref,omitempty"`
}

func (e NoteCreated) EventType() string      { return "NoteCreated" }
func (e NoteCreated) AggregateID() uuid.UUID { return e.NoteID }

// NewNoteCreated creates a NoteCreated event from a Note.
func NewNoteCreated(n *Note) NoteCreated {
	return NoteCreated{
		BaseEvent:    NewBaseEvent(),
		NoteID:       n.ID,
		Text:         n.Text,
		MIME:         n.MIME,
		Language:     n.Language,
		Translations: n.Translations,
		GedcomXref:   n.GedcomXref,
	}
}

// NoteUpdated event is emitted when a note is updated.
type NoteUpdated struct {
	BaseEvent
	NoteID  uuid.UUID      `json:"note_id"`
	Changes map[string]any `json:"changes"`
}

func (e NoteUpdated) EventType() string      { return "NoteUpdated" }
func (e NoteUpdated) AggregateID() uuid.UUID { return e.NoteID }

// NewNoteUpdated creates a NoteUpdated event.
func NewNoteUpdated(noteID uuid.UUID, changes map[string]any) NoteUpdated {
	return NoteUpdated{
		BaseEvent: NewBaseEvent(),
		NoteID:    noteID,
		Changes:   changes,
	}
}

// NoteDeleted event is emitted when a note is deleted.
type NoteDeleted struct {
	BaseEvent
	NoteID uuid.UUID `json:"note_id"`
	Reason string    `json:"reason,omitempty"`
}

func (e NoteDeleted) EventType() string      { return "NoteDeleted" }
func (e NoteDeleted) AggregateID() uuid.UUID { return e.NoteID }

// NewNoteDeleted creates a NoteDeleted event.
func NewNoteDeleted(noteID uuid.UUID, reason string) NoteDeleted {
	return NoteDeleted{
		BaseEvent: NewBaseEvent(),
		NoteID:    noteID,
		Reason:    reason,
	}
}

// SubmitterCreated event is emitted when a new submitter is created.
type SubmitterCreated struct {
	BaseEvent
	SubmitterID uuid.UUID  `json:"submitter_id"`
	Name        string     `json:"name"`
	Address     *Address   `json:"address,omitempty"`
	Phone       []string   `json:"phone,omitempty"`
	Email       []string   `json:"email,omitempty"`
	Language    string     `json:"language,omitempty"`
	MediaID     *uuid.UUID `json:"media_id,omitempty"`
	GedcomXref  string     `json:"gedcom_xref,omitempty"`
}

func (e SubmitterCreated) EventType() string      { return "SubmitterCreated" }
func (e SubmitterCreated) AggregateID() uuid.UUID { return e.SubmitterID }

// NewSubmitterCreated creates a SubmitterCreated event from a Submitter.
func NewSubmitterCreated(s *Submitter) SubmitterCreated {
	return SubmitterCreated{
		BaseEvent:   NewBaseEvent(),
		SubmitterID: s.ID,
		Name:        s.Name,
		Address:     s.Address,
		Phone:       s.Phone,
		Email:       s.Email,
		Language:    s.Language,
		MediaID:     s.MediaID,
		GedcomXref:  s.GedcomXref,
	}
}

// SubmitterUpdated event is emitted when a submitter is updated.
type SubmitterUpdated struct {
	BaseEvent
	SubmitterID uuid.UUID      `json:"submitter_id"`
	Changes     map[string]any `json:"changes"`
}

func (e SubmitterUpdated) EventType() string      { return "SubmitterUpdated" }
func (e SubmitterUpdated) AggregateID() uuid.UUID { return e.SubmitterID }

// NewSubmitterUpdated creates a SubmitterUpdated event.
func NewSubmitterUpdated(submitterID uuid.UUID, changes map[string]any) SubmitterUpdated {
	return SubmitterUpdated{
		BaseEvent:   NewBaseEvent(),
		SubmitterID: submitterID,
		Changes:     changes,
	}
}

// SubmitterDeleted event is emitted when a submitter is deleted.
type SubmitterDeleted struct {
	BaseEvent
	SubmitterID uuid.UUID `json:"submitter_id"`
	Reason      string    `json:"reason,omitempty"`
}

func (e SubmitterDeleted) EventType() string      { return "SubmitterDeleted" }
func (e SubmitterDeleted) AggregateID() uuid.UUID { return e.SubmitterID }

// NewSubmitterDeleted creates a SubmitterDeleted event.
func NewSubmitterDeleted(submitterID uuid.UUID, reason string) SubmitterDeleted {
	return SubmitterDeleted{
		BaseEvent:   NewBaseEvent(),
		SubmitterID: submitterID,
		Reason:      reason,
	}
}

// AssociationCreated event is emitted when a new association is created.
type AssociationCreated struct {
	BaseEvent
	AssociationID uuid.UUID   `json:"association_id"`
	PersonID      uuid.UUID   `json:"person_id"`
	AssociateID   uuid.UUID   `json:"associate_id"`
	Role          string      `json:"role"`
	Phrase        string      `json:"phrase,omitempty"`
	Notes         string      `json:"notes,omitempty"`
	NoteIDs       []uuid.UUID `json:"note_ids,omitempty"`
	GedcomXref    string      `json:"gedcom_xref,omitempty"`
}

func (e AssociationCreated) EventType() string      { return "AssociationCreated" }
func (e AssociationCreated) AggregateID() uuid.UUID { return e.AssociationID }

// NewAssociationCreated creates an AssociationCreated event from an Association.
func NewAssociationCreated(a *Association) AssociationCreated {
	return AssociationCreated{
		BaseEvent:     NewBaseEvent(),
		AssociationID: a.ID,
		PersonID:      a.PersonID,
		AssociateID:   a.AssociateID,
		Role:          a.Role,
		Phrase:        a.Phrase,
		Notes:         a.Notes,
		NoteIDs:       a.NoteIDs,
		GedcomXref:    a.GedcomXref,
	}
}

// AssociationUpdated event is emitted when an association is updated.
type AssociationUpdated struct {
	BaseEvent
	AssociationID uuid.UUID      `json:"association_id"`
	Changes       map[string]any `json:"changes"`
}

func (e AssociationUpdated) EventType() string      { return "AssociationUpdated" }
func (e AssociationUpdated) AggregateID() uuid.UUID { return e.AssociationID }

// NewAssociationUpdated creates an AssociationUpdated event.
func NewAssociationUpdated(associationID uuid.UUID, changes map[string]any) AssociationUpdated {
	return AssociationUpdated{
		BaseEvent:     NewBaseEvent(),
		AssociationID: associationID,
		Changes:       changes,
	}
}

// AssociationDeleted event is emitted when an association is deleted.
type AssociationDeleted struct {
	BaseEvent
	AssociationID uuid.UUID `json:"association_id"`
	Reason        string    `json:"reason,omitempty"`
}

func (e AssociationDeleted) EventType() string      { return "AssociationDeleted" }
func (e AssociationDeleted) AggregateID() uuid.UUID { return e.AssociationID }

// NewAssociationDeleted creates an AssociationDeleted event.
func NewAssociationDeleted(associationID uuid.UUID, reason string) AssociationDeleted {
	return AssociationDeleted{
		BaseEvent:     NewBaseEvent(),
		AssociationID: associationID,
		Reason:        reason,
	}
}

// LDSOrdinanceCreated event is emitted when a new LDS ordinance is created.
type LDSOrdinanceCreated struct {
	BaseEvent
	OrdinanceID uuid.UUID        `json:"ordinance_id"`
	Type        LDSOrdinanceType `json:"type"`
	PersonID    *uuid.UUID       `json:"person_id,omitempty"` // For individual ordinances
	FamilyID    *uuid.UUID       `json:"family_id,omitempty"` // For SLGS (sealing to spouse)
	Date        *GenDate         `json:"date,omitempty"`
	Place       string           `json:"place,omitempty"`
	Temple      string           `json:"temple,omitempty"` // Temple code (TEMP)
	Status      string           `json:"status,omitempty"` // COMPLETED, BIC, etc.
}

func (e LDSOrdinanceCreated) EventType() string      { return "LDSOrdinanceCreated" }
func (e LDSOrdinanceCreated) AggregateID() uuid.UUID { return e.OrdinanceID }

// NewLDSOrdinanceCreated creates a LDSOrdinanceCreated event from an LDSOrdinance.
func NewLDSOrdinanceCreated(o *LDSOrdinance) LDSOrdinanceCreated {
	return LDSOrdinanceCreated{
		BaseEvent:   NewBaseEvent(),
		OrdinanceID: o.ID,
		Type:        o.Type,
		PersonID:    o.PersonID,
		FamilyID:    o.FamilyID,
		Date:        o.Date,
		Place:       o.Place,
		Temple:      o.Temple,
		Status:      o.Status,
	}
}

// LDSOrdinanceUpdated event is emitted when an LDS ordinance is updated.
type LDSOrdinanceUpdated struct {
	BaseEvent
	OrdinanceID uuid.UUID      `json:"ordinance_id"`
	Changes     map[string]any `json:"changes"`
}

func (e LDSOrdinanceUpdated) EventType() string      { return "LDSOrdinanceUpdated" }
func (e LDSOrdinanceUpdated) AggregateID() uuid.UUID { return e.OrdinanceID }

// NewLDSOrdinanceUpdated creates a LDSOrdinanceUpdated event.
func NewLDSOrdinanceUpdated(ordinanceID uuid.UUID, changes map[string]any) LDSOrdinanceUpdated {
	return LDSOrdinanceUpdated{
		BaseEvent:   NewBaseEvent(),
		OrdinanceID: ordinanceID,
		Changes:     changes,
	}
}

// LDSOrdinanceDeleted event is emitted when an LDS ordinance is deleted.
type LDSOrdinanceDeleted struct {
	BaseEvent
	OrdinanceID uuid.UUID `json:"ordinance_id"`
	Reason      string    `json:"reason,omitempty"`
}

func (e LDSOrdinanceDeleted) EventType() string      { return "LDSOrdinanceDeleted" }
func (e LDSOrdinanceDeleted) AggregateID() uuid.UUID { return e.OrdinanceID }

// NewLDSOrdinanceDeleted creates a LDSOrdinanceDeleted event.
func NewLDSOrdinanceDeleted(ordinanceID uuid.UUID, reason string) LDSOrdinanceDeleted {
	return LDSOrdinanceDeleted{
		BaseEvent:   NewBaseEvent(),
		OrdinanceID: ordinanceID,
		Reason:      reason,
	}
}

// EvidenceAnalysisCreated event is emitted when a new evidence analysis is created.
type EvidenceAnalysisCreated struct {
	BaseEvent
	AnalysisID     uuid.UUID      `json:"analysis_id"`
	FactType       FactType       `json:"fact_type"`
	SubjectID      uuid.UUID      `json:"subject_id"`
	CitationIDs    []uuid.UUID    `json:"citation_ids,omitempty"`
	Conclusion     string         `json:"conclusion"`
	ResearchStatus ResearchStatus `json:"research_status,omitempty"`
	Notes          string         `json:"notes,omitempty"`
}

func (e EvidenceAnalysisCreated) EventType() string      { return "EvidenceAnalysisCreated" }
func (e EvidenceAnalysisCreated) AggregateID() uuid.UUID { return e.AnalysisID }

// NewEvidenceAnalysisCreated creates an EvidenceAnalysisCreated event from an EvidenceAnalysis.
func NewEvidenceAnalysisCreated(ea *EvidenceAnalysis) EvidenceAnalysisCreated {
	return EvidenceAnalysisCreated{
		BaseEvent:      NewBaseEvent(),
		AnalysisID:     ea.ID,
		FactType:       ea.FactType,
		SubjectID:      ea.SubjectID,
		CitationIDs:    ea.CitationIDs,
		Conclusion:     ea.Conclusion,
		ResearchStatus: ea.ResearchStatus,
		Notes:          ea.Notes,
	}
}

// EvidenceAnalysisUpdated event is emitted when an evidence analysis is updated.
type EvidenceAnalysisUpdated struct {
	BaseEvent
	AnalysisID uuid.UUID      `json:"analysis_id"`
	Changes    map[string]any `json:"changes"`
}

func (e EvidenceAnalysisUpdated) EventType() string      { return "EvidenceAnalysisUpdated" }
func (e EvidenceAnalysisUpdated) AggregateID() uuid.UUID { return e.AnalysisID }

// NewEvidenceAnalysisUpdated creates an EvidenceAnalysisUpdated event.
func NewEvidenceAnalysisUpdated(analysisID uuid.UUID, changes map[string]any) EvidenceAnalysisUpdated {
	return EvidenceAnalysisUpdated{
		BaseEvent:  NewBaseEvent(),
		AnalysisID: analysisID,
		Changes:    changes,
	}
}

// EvidenceAnalysisDeleted event is emitted when an evidence analysis is deleted.
type EvidenceAnalysisDeleted struct {
	BaseEvent
	AnalysisID uuid.UUID `json:"analysis_id"`
	Reason     string    `json:"reason,omitempty"`
}

func (e EvidenceAnalysisDeleted) EventType() string      { return "EvidenceAnalysisDeleted" }
func (e EvidenceAnalysisDeleted) AggregateID() uuid.UUID { return e.AnalysisID }

// NewEvidenceAnalysisDeleted creates an EvidenceAnalysisDeleted event.
func NewEvidenceAnalysisDeleted(analysisID uuid.UUID, reason string) EvidenceAnalysisDeleted {
	return EvidenceAnalysisDeleted{
		BaseEvent:  NewBaseEvent(),
		AnalysisID: analysisID,
		Reason:     reason,
	}
}

// EvidenceConflictDetected event is emitted when contradictory evidence is detected.
type EvidenceConflictDetected struct {
	BaseEvent
	ConflictID  uuid.UUID      `json:"conflict_id"`
	FactType    FactType       `json:"fact_type"`
	SubjectID   uuid.UUID      `json:"subject_id"`
	AnalysisIDs []uuid.UUID    `json:"analysis_ids"`
	Description string         `json:"description"`
	Status      ConflictStatus `json:"status"`
}

func (e EvidenceConflictDetected) EventType() string      { return "EvidenceConflictDetected" }
func (e EvidenceConflictDetected) AggregateID() uuid.UUID { return e.ConflictID }

// NewEvidenceConflictDetected creates an EvidenceConflictDetected event from an EvidenceConflict.
func NewEvidenceConflictDetected(ec *EvidenceConflict) EvidenceConflictDetected {
	return EvidenceConflictDetected{
		BaseEvent:   NewBaseEvent(),
		ConflictID:  ec.ID,
		FactType:    ec.FactType,
		SubjectID:   ec.SubjectID,
		AnalysisIDs: ec.AnalysisIDs,
		Description: ec.Description,
		Status:      ec.Status,
	}
}

// EvidenceConflictResolved event is emitted when an evidence conflict is resolved.
type EvidenceConflictResolved struct {
	BaseEvent
	ConflictID uuid.UUID      `json:"conflict_id"`
	Resolution string         `json:"resolution"`
	Status     ConflictStatus `json:"status"`
}

func (e EvidenceConflictResolved) EventType() string      { return "EvidenceConflictResolved" }
func (e EvidenceConflictResolved) AggregateID() uuid.UUID { return e.ConflictID }

// NewEvidenceConflictResolved creates an EvidenceConflictResolved event.
func NewEvidenceConflictResolved(conflictID uuid.UUID, resolution string, status ConflictStatus) EvidenceConflictResolved {
	return EvidenceConflictResolved{
		BaseEvent:  NewBaseEvent(),
		ConflictID: conflictID,
		Resolution: resolution,
		Status:     status,
	}
}

// ResearchLogCreated event is emitted when a new research log entry is created.
type ResearchLogCreated struct {
	BaseEvent
	LogID             uuid.UUID       `json:"log_id"`
	SubjectID         uuid.UUID       `json:"subject_id"`
	SubjectType       string          `json:"subject_type"`
	Repository        string          `json:"repository"`
	SearchDescription string          `json:"search_description"`
	Outcome           ResearchOutcome `json:"outcome"`
	Notes             string          `json:"notes,omitempty"`
	SearchDate        time.Time       `json:"search_date"`
}

func (e ResearchLogCreated) EventType() string      { return "ResearchLogCreated" }
func (e ResearchLogCreated) AggregateID() uuid.UUID { return e.LogID }

// NewResearchLogCreated creates a ResearchLogCreated event from a ResearchLog.
func NewResearchLogCreated(rl *ResearchLog) ResearchLogCreated {
	return ResearchLogCreated{
		BaseEvent:         NewBaseEvent(),
		LogID:             rl.ID,
		SubjectID:         rl.SubjectID,
		SubjectType:       rl.SubjectType,
		Repository:        rl.Repository,
		SearchDescription: rl.SearchDescription,
		Outcome:           rl.Outcome,
		Notes:             rl.Notes,
		SearchDate:        rl.SearchDate,
	}
}

// ResearchLogUpdated event is emitted when a research log entry is updated.
type ResearchLogUpdated struct {
	BaseEvent
	LogID   uuid.UUID      `json:"log_id"`
	Changes map[string]any `json:"changes"`
}

func (e ResearchLogUpdated) EventType() string      { return "ResearchLogUpdated" }
func (e ResearchLogUpdated) AggregateID() uuid.UUID { return e.LogID }

// NewResearchLogUpdated creates a ResearchLogUpdated event.
func NewResearchLogUpdated(logID uuid.UUID, changes map[string]any) ResearchLogUpdated {
	return ResearchLogUpdated{
		BaseEvent: NewBaseEvent(),
		LogID:     logID,
		Changes:   changes,
	}
}

// ResearchLogDeleted event is emitted when a research log entry is deleted.
type ResearchLogDeleted struct {
	BaseEvent
	LogID  uuid.UUID `json:"log_id"`
	Reason string    `json:"reason,omitempty"`
}

func (e ResearchLogDeleted) EventType() string      { return "ResearchLogDeleted" }
func (e ResearchLogDeleted) AggregateID() uuid.UUID { return e.LogID }

// NewResearchLogDeleted creates a ResearchLogDeleted event.
func NewResearchLogDeleted(logID uuid.UUID, reason string) ResearchLogDeleted {
	return ResearchLogDeleted{
		BaseEvent: NewBaseEvent(),
		LogID:     logID,
		Reason:    reason,
	}
}

// ProofSummaryCreated event is emitted when a new proof summary is created.
type ProofSummaryCreated struct {
	BaseEvent
	SummaryID      uuid.UUID      `json:"summary_id"`
	FactType       FactType       `json:"fact_type"`
	SubjectID      uuid.UUID      `json:"subject_id"`
	Conclusion     string         `json:"conclusion"`
	Argument       string         `json:"argument"`
	AnalysisIDs    []uuid.UUID    `json:"analysis_ids,omitempty"`
	ResearchStatus ResearchStatus `json:"research_status,omitempty"`
}

func (e ProofSummaryCreated) EventType() string      { return "ProofSummaryCreated" }
func (e ProofSummaryCreated) AggregateID() uuid.UUID { return e.SummaryID }

// NewProofSummaryCreated creates a ProofSummaryCreated event from a ProofSummary.
func NewProofSummaryCreated(ps *ProofSummary) ProofSummaryCreated {
	return ProofSummaryCreated{
		BaseEvent:      NewBaseEvent(),
		SummaryID:      ps.ID,
		FactType:       ps.FactType,
		SubjectID:      ps.SubjectID,
		Conclusion:     ps.Conclusion,
		Argument:       ps.Argument,
		AnalysisIDs:    ps.AnalysisIDs,
		ResearchStatus: ps.ResearchStatus,
	}
}

// ProofSummaryUpdated event is emitted when a proof summary is updated.
type ProofSummaryUpdated struct {
	BaseEvent
	SummaryID uuid.UUID      `json:"summary_id"`
	Changes   map[string]any `json:"changes"`
}

func (e ProofSummaryUpdated) EventType() string      { return "ProofSummaryUpdated" }
func (e ProofSummaryUpdated) AggregateID() uuid.UUID { return e.SummaryID }

// NewProofSummaryUpdated creates a ProofSummaryUpdated event.
func NewProofSummaryUpdated(summaryID uuid.UUID, changes map[string]any) ProofSummaryUpdated {
	return ProofSummaryUpdated{
		BaseEvent: NewBaseEvent(),
		SummaryID: summaryID,
		Changes:   changes,
	}
}

// ProofSummaryDeleted event is emitted when a proof summary is deleted.
type ProofSummaryDeleted struct {
	BaseEvent
	SummaryID uuid.UUID `json:"summary_id"`
	Reason    string    `json:"reason,omitempty"`
}

func (e ProofSummaryDeleted) EventType() string      { return "ProofSummaryDeleted" }
func (e ProofSummaryDeleted) AggregateID() uuid.UUID { return e.SummaryID }

// NewProofSummaryDeleted creates a ProofSummaryDeleted event.
func NewProofSummaryDeleted(summaryID uuid.UUID, reason string) ProofSummaryDeleted {
	return ProofSummaryDeleted{
		BaseEvent: NewBaseEvent(),
		SummaryID: summaryID,
		Reason:    reason,
	}
}
