package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// PersonReadModel represents a person in the read model.
type PersonReadModel struct {
	ID             uuid.UUID             `json:"id"`
	GivenName      string                `json:"given_name"`
	Surname        string                `json:"surname"`
	FullName       string                `json:"full_name"`
	Gender         domain.Gender         `json:"gender,omitempty"`
	BirthDateRaw   string                `json:"birth_date_raw,omitempty"`
	BirthDateSort  *time.Time            `json:"birth_date_sort,omitempty"`
	BirthPlace     string                `json:"birth_place,omitempty"`
	BirthPlaceLat  *string               `json:"birth_place_lat,omitempty"`
	BirthPlaceLong *string               `json:"birth_place_long,omitempty"`
	DeathDateRaw   string                `json:"death_date_raw,omitempty"`
	DeathDateSort  *time.Time            `json:"death_date_sort,omitempty"`
	DeathPlace     string                `json:"death_place,omitempty"`
	DeathPlaceLat  *string               `json:"death_place_lat,omitempty"`
	DeathPlaceLong *string               `json:"death_place_long,omitempty"`
	Notes          string                `json:"notes,omitempty"`
	ResearchStatus domain.ResearchStatus `json:"research_status,omitempty"`
	Version        int64                 `json:"version"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

// FamilyReadModel represents a family in the read model.
type FamilyReadModel struct {
	ID                uuid.UUID           `json:"id"`
	Partner1ID        *uuid.UUID          `json:"partner1_id,omitempty"`
	Partner1Name      string              `json:"partner1_name,omitempty"`
	Partner2ID        *uuid.UUID          `json:"partner2_id,omitempty"`
	Partner2Name      string              `json:"partner2_name,omitempty"`
	RelationshipType  domain.RelationType `json:"relationship_type,omitempty"`
	MarriageDateRaw   string              `json:"marriage_date_raw,omitempty"`
	MarriageDateSort  *time.Time          `json:"marriage_date_sort,omitempty"`
	MarriagePlace     string              `json:"marriage_place,omitempty"`
	MarriagePlaceLat  *string             `json:"marriage_place_lat,omitempty"`
	MarriagePlaceLong *string             `json:"marriage_place_long,omitempty"`
	ChildCount        int                 `json:"child_count"`
	Version           int64               `json:"version"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

// FamilyChildReadModel represents a child in a family.
type FamilyChildReadModel struct {
	FamilyID         uuid.UUID                `json:"family_id"`
	PersonID         uuid.UUID                `json:"person_id"`
	PersonName       string                   `json:"person_name"`
	RelationshipType domain.ChildRelationType `json:"relationship_type"`
	Sequence         *int                     `json:"sequence,omitempty"`
}

// PedigreeEdge represents a parent-child relationship for pedigree traversal.
type PedigreeEdge struct {
	PersonID   uuid.UUID  `json:"person_id"`
	FatherID   *uuid.UUID `json:"father_id,omitempty"`
	MotherID   *uuid.UUID `json:"mother_id,omitempty"`
	FatherName string     `json:"father_name,omitempty"`
	MotherName string     `json:"mother_name,omitempty"`
}

// SourceReadModel represents a source in the read model.
type SourceReadModel struct {
	ID              uuid.UUID         `json:"id"`
	SourceType      domain.SourceType `json:"source_type"`
	Title           string            `json:"title"`
	Author          string            `json:"author,omitempty"`
	Publisher       string            `json:"publisher,omitempty"`
	PublishDateRaw  string            `json:"publish_date_raw,omitempty"`
	PublishDateSort *time.Time        `json:"publish_date_sort,omitempty"`
	URL             string            `json:"url,omitempty"`
	RepositoryName  string            `json:"repository_name,omitempty"`
	CollectionName  string            `json:"collection_name,omitempty"`
	CallNumber      string            `json:"call_number,omitempty"`
	Notes           string            `json:"notes,omitempty"`
	GedcomXref      string            `json:"gedcom_xref,omitempty"`
	CitationCount   int               `json:"citation_count"`
	Version         int64             `json:"version"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// CitationReadModel represents a citation in the read model.
type CitationReadModel struct {
	ID            uuid.UUID            `json:"id"`
	SourceID      uuid.UUID            `json:"source_id"`
	SourceTitle   string               `json:"source_title"`
	FactType      domain.FactType      `json:"fact_type"`
	FactOwnerID   uuid.UUID            `json:"fact_owner_id"`
	Page          string               `json:"page,omitempty"`
	Volume        string               `json:"volume,omitempty"`
	SourceQuality domain.SourceQuality `json:"source_quality,omitempty"`
	InformantType domain.InformantType `json:"informant_type,omitempty"`
	EvidenceType  domain.EvidenceType  `json:"evidence_type,omitempty"`
	QuotedText    string               `json:"quoted_text,omitempty"`
	Analysis      string               `json:"analysis,omitempty"`
	TemplateID    string               `json:"template_id,omitempty"`
	GedcomXref    string               `json:"gedcom_xref,omitempty"`
	Version       int64                `json:"version"`
	CreatedAt     time.Time            `json:"created_at"`
}

// MediaReadModel represents a media file in the read model.
type MediaReadModel struct {
	ID            uuid.UUID        `json:"id"`
	EntityType    string           `json:"entity_type"`
	EntityID      uuid.UUID        `json:"entity_id"`
	Title         string           `json:"title"`
	Description   string           `json:"description,omitempty"`
	MimeType      string           `json:"mime_type"`
	MediaType     domain.MediaType `json:"media_type"`
	Filename      string           `json:"filename"`
	FileSize      int64            `json:"file_size"`
	FileData      []byte           `json:"-"` // Excluded from JSON by default
	ThumbnailData []byte           `json:"-"` // Excluded from JSON by default
	CropLeft      *int             `json:"crop_left,omitempty"`
	CropTop       *int             `json:"crop_top,omitempty"`
	CropWidth     *int             `json:"crop_width,omitempty"`
	CropHeight    *int             `json:"crop_height,omitempty"`
	GedcomXref    string           `json:"gedcom_xref,omitempty"`
	Version       int64            `json:"version"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// EventReadModel represents a life event in the read model.
type EventReadModel struct {
	ID             uuid.UUID             `json:"id"`
	OwnerType      string                `json:"owner_type"` // "person" or "family"
	OwnerID        uuid.UUID             `json:"owner_id"`
	FactType       domain.FactType       `json:"fact_type"`
	DateRaw        string                `json:"date_raw,omitempty"`
	DateSort       *time.Time            `json:"date_sort,omitempty"`
	Place          string                `json:"place,omitempty"`
	PlaceLat       *string               `json:"place_lat,omitempty"`
	PlaceLong      *string               `json:"place_long,omitempty"`
	Description    string                `json:"description,omitempty"`
	Cause          string                `json:"cause,omitempty"`           // For death/burial events
	Age            string                `json:"age,omitempty"`             // Age at event
	ResearchStatus domain.ResearchStatus `json:"research_status,omitempty"` // Confidence level
	Version        int64                 `json:"version"`
	CreatedAt      time.Time             `json:"created_at"`
}

// AttributeReadModel represents a person attribute in the read model.
type AttributeReadModel struct {
	ID        uuid.UUID       `json:"id"`
	PersonID  uuid.UUID       `json:"person_id"`
	FactType  domain.FactType `json:"fact_type"`
	Value     string          `json:"value"`
	DateRaw   string          `json:"date_raw,omitempty"`
	DateSort  *time.Time      `json:"date_sort,omitempty"`
	Place     string          `json:"place,omitempty"`
	Version   int64           `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
}

// ReadModelStore provides access to denormalized read models.
type ReadModelStore interface {
	// Person operations
	GetPerson(ctx context.Context, id uuid.UUID) (*PersonReadModel, error)
	ListPersons(ctx context.Context, opts ListOptions) ([]PersonReadModel, int, error)
	SearchPersons(ctx context.Context, query string, fuzzy bool, limit int) ([]PersonReadModel, error)
	SavePerson(ctx context.Context, person *PersonReadModel) error
	DeletePerson(ctx context.Context, id uuid.UUID) error

	// Person name operations
	SavePersonName(ctx context.Context, name *PersonNameReadModel) error
	GetPersonName(ctx context.Context, nameID uuid.UUID) (*PersonNameReadModel, error)
	GetPersonNames(ctx context.Context, personID uuid.UUID) ([]PersonNameReadModel, error)
	DeletePersonName(ctx context.Context, nameID uuid.UUID) error

	// Family operations
	GetFamily(ctx context.Context, id uuid.UUID) (*FamilyReadModel, error)
	ListFamilies(ctx context.Context, opts ListOptions) ([]FamilyReadModel, int, error)
	GetFamiliesForPerson(ctx context.Context, personID uuid.UUID) ([]FamilyReadModel, error)
	SaveFamily(ctx context.Context, family *FamilyReadModel) error
	DeleteFamily(ctx context.Context, id uuid.UUID) error

	// Family children operations
	GetFamilyChildren(ctx context.Context, familyID uuid.UUID) ([]FamilyChildReadModel, error)
	GetChildrenOfFamily(ctx context.Context, familyID uuid.UUID) ([]PersonReadModel, error)
	GetChildFamily(ctx context.Context, personID uuid.UUID) (*FamilyReadModel, error)
	SaveFamilyChild(ctx context.Context, child *FamilyChildReadModel) error
	DeleteFamilyChild(ctx context.Context, familyID, personID uuid.UUID) error

	// Pedigree operations
	GetPedigreeEdge(ctx context.Context, personID uuid.UUID) (*PedigreeEdge, error)
	SavePedigreeEdge(ctx context.Context, edge *PedigreeEdge) error
	DeletePedigreeEdge(ctx context.Context, personID uuid.UUID) error

	// Source operations
	GetSource(ctx context.Context, id uuid.UUID) (*SourceReadModel, error)
	ListSources(ctx context.Context, opts ListOptions) ([]SourceReadModel, int, error)
	SearchSources(ctx context.Context, query string, limit int) ([]SourceReadModel, error)
	SaveSource(ctx context.Context, source *SourceReadModel) error
	DeleteSource(ctx context.Context, id uuid.UUID) error

	// Citation operations
	GetCitation(ctx context.Context, id uuid.UUID) (*CitationReadModel, error)
	GetCitationsForSource(ctx context.Context, sourceID uuid.UUID) ([]CitationReadModel, error)
	GetCitationsForPerson(ctx context.Context, personID uuid.UUID) ([]CitationReadModel, error)
	GetCitationsForFact(ctx context.Context, factType domain.FactType, factOwnerID uuid.UUID) ([]CitationReadModel, error)
	SaveCitation(ctx context.Context, citation *CitationReadModel) error
	DeleteCitation(ctx context.Context, id uuid.UUID) error

	// Media operations
	GetMedia(ctx context.Context, id uuid.UUID) (*MediaReadModel, error)
	GetMediaWithData(ctx context.Context, id uuid.UUID) (*MediaReadModel, error) // Includes FileData
	GetMediaThumbnail(ctx context.Context, id uuid.UUID) ([]byte, error)
	ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts ListOptions) ([]MediaReadModel, int, error)
	SaveMedia(ctx context.Context, media *MediaReadModel) error
	DeleteMedia(ctx context.Context, id uuid.UUID) error

	// Event operations
	GetEvent(ctx context.Context, id uuid.UUID) (*EventReadModel, error)
	ListEventsForPerson(ctx context.Context, personID uuid.UUID) ([]EventReadModel, error)
	ListEventsForFamily(ctx context.Context, familyID uuid.UUID) ([]EventReadModel, error)
	SaveEvent(ctx context.Context, event *EventReadModel) error
	DeleteEvent(ctx context.Context, id uuid.UUID) error

	// Attribute operations
	GetAttribute(ctx context.Context, id uuid.UUID) (*AttributeReadModel, error)
	ListAttributesForPerson(ctx context.Context, personID uuid.UUID) ([]AttributeReadModel, error)
	SaveAttribute(ctx context.Context, attribute *AttributeReadModel) error
	DeleteAttribute(ctx context.Context, id uuid.UUID) error

	// Browse operations
	GetSurnameIndex(ctx context.Context) ([]SurnameEntry, []LetterCount, error)
	GetSurnamesByLetter(ctx context.Context, letter string) ([]SurnameEntry, error)
	GetPersonsBySurname(ctx context.Context, surname string, opts ListOptions) ([]PersonReadModel, int, error)
	GetPlaceHierarchy(ctx context.Context, parent string) ([]PlaceEntry, error)
	GetPersonsByPlace(ctx context.Context, place string, opts ListOptions) ([]PersonReadModel, int, error)
}

// SurnameEntry represents a surname with count.
type SurnameEntry struct {
	Surname string `json:"surname"`
	Count   int    `json:"count"`
}

// LetterCount represents count of surnames by starting letter.
type LetterCount struct {
	Letter string `json:"letter"`
	Count  int    `json:"count"`
}

// PlaceEntry represents a place with count and hierarchy info.
type PlaceEntry struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Count       int    `json:"count"`
	HasChildren bool   `json:"has_children"`
}

// PersonNameReadModel represents a name variant for a person in the read model.
type PersonNameReadModel struct {
	ID            uuid.UUID       `json:"id"`
	PersonID      uuid.UUID       `json:"person_id"`
	GivenName     string          `json:"given_name"`
	Surname       string          `json:"surname"`
	FullName      string          `json:"full_name"`
	NamePrefix    string          `json:"name_prefix,omitempty"`
	NameSuffix    string          `json:"name_suffix,omitempty"`
	SurnamePrefix string          `json:"surname_prefix,omitempty"`
	Nickname      string          `json:"nickname,omitempty"`
	NameType      domain.NameType `json:"name_type"`
	IsPrimary     bool            `json:"is_primary"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// ListOptions contains options for list queries.
type ListOptions struct {
	Limit          int
	Offset         int
	Sort           string
	Order          string  // "asc" or "desc"
	ResearchStatus *string // Filter by research_status: certain, probable, possible, unknown, or "unset" for NULL
}

// DefaultListOptions returns sensible defaults for list queries.
func DefaultListOptions() ListOptions {
	return ListOptions{
		Limit:  20,
		Offset: 0,
		Sort:   "surname",
		Order:  "asc",
	}
}
