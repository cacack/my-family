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
	PersonID   uuid.UUID `json:"person_id"`
	GivenName  string    `json:"given_name"`
	Surname    string    `json:"surname"`
	Gender     Gender    `json:"gender,omitempty"`
	BirthDate  *GenDate  `json:"birth_date,omitempty"`
	BirthPlace string    `json:"birth_place,omitempty"`
	DeathDate  *GenDate  `json:"death_date,omitempty"`
	DeathPlace string    `json:"death_place,omitempty"`
	Notes      string    `json:"notes,omitempty"`
	GedcomXref string    `json:"gedcom_xref,omitempty"`
}

func (e PersonCreated) EventType() string      { return "PersonCreated" }
func (e PersonCreated) AggregateID() uuid.UUID { return e.PersonID }

// NewPersonCreated creates a PersonCreated event from a Person.
func NewPersonCreated(p *Person) PersonCreated {
	return PersonCreated{
		BaseEvent:  NewBaseEvent(),
		PersonID:   p.ID,
		GivenName:  p.GivenName,
		Surname:    p.Surname,
		Gender:     p.Gender,
		BirthDate:  p.BirthDate,
		BirthPlace: p.BirthPlace,
		DeathDate:  p.DeathDate,
		DeathPlace: p.DeathPlace,
		Notes:      p.Notes,
		GedcomXref: p.GedcomXref,
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
