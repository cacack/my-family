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
	// GEDCOM 7.0 enhanced fields
	Files        []domain.MediaFile `json:"files,omitempty"`        // Multiple file references (GEDCOM 7.0)
	Format       string             `json:"format,omitempty"`       // Primary format/MIME type (FORM)
	Translations []string           `json:"translations,omitempty"` // Translated titles (GEDCOM 7.0)
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
	Address        *domain.Address       `json:"address,omitempty"` // Structured address (RESI, etc.)
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

// NoteReadModel represents a shared GEDCOM note in the read model.
// GEDCOM supports two note styles:
// - Inline notes: embedded directly in an entity
// - Shared notes: top-level NOTE records that can be referenced by multiple entities via @N1@
type NoteReadModel struct {
	ID         uuid.UUID `json:"id"`
	Text       string    `json:"text"`                  // Full text with embedded newlines
	GedcomXref string    `json:"gedcom_xref,omitempty"` // GEDCOM cross-reference ID (e.g., "@N1@")
	Version    int64     `json:"version"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SubmitterReadModel represents a GEDCOM SUBM (Submitter) record in the read model.
// Submitters track who created or submitted genealogical data.
type SubmitterReadModel struct {
	ID         uuid.UUID       `json:"id"`
	Name       string          `json:"name"`                  // NAME - Submitter's name
	Address    *domain.Address `json:"address,omitempty"`     // ADDR - Structured address
	Phone      []string        `json:"phone,omitempty"`       // PHON - Multiple phone numbers
	Email      []string        `json:"email,omitempty"`       // EMAIL - Multiple email addresses
	Language   string          `json:"language,omitempty"`    // LANG - Preferred language
	MediaID    *uuid.UUID      `json:"media_id,omitempty"`    // OBJE - Link to submitter photo
	GedcomXref string          `json:"gedcom_xref,omitempty"` // GEDCOM cross-reference ID for round-trip
	Version    int64           `json:"version"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// AssociationReadModel represents a GEDCOM ASSO (Association) record in the read model.
// Associations capture non-family relationships like godparents, witnesses, mentors, etc.
type AssociationReadModel struct {
	ID            uuid.UUID   `json:"id"`
	PersonID      uuid.UUID   `json:"person_id"`             // The individual (INDI containing ASSO)
	PersonName    string      `json:"person_name"`           // Denormalized for display
	AssociateID   uuid.UUID   `json:"associate_id"`          // The associated person
	AssociateName string      `json:"associate_name"`        // Denormalized for display
	Role          string      `json:"role"`                  // godparent, witness, or custom
	Phrase        string      `json:"phrase,omitempty"`      // GEDCOM 7.0 human-readable description
	Notes         string      `json:"notes,omitempty"`       // Inline note text
	NoteIDs       []uuid.UUID `json:"note_ids,omitempty"`    // Linked Note entities
	GedcomXref    string      `json:"gedcom_xref,omitempty"` // GEDCOM cross-reference ID for round-trip
	Version       int64       `json:"version"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// LDSOrdinanceReadModel represents an LDS temple ordinance in the read model.
// GEDCOM was originally developed by the LDS Church and includes tags for temple ordinances
// (BAPL, CONL, ENDL, SLGC, SLGS). These are important for users with LDS heritage or data from FamilySearch.
type LDSOrdinanceReadModel struct {
	ID         uuid.UUID               `json:"id"`
	Type       domain.LDSOrdinanceType `json:"type"`                  // BAPL, CONL, ENDL, SLGC, SLGS
	TypeLabel  string                  `json:"type_label"`            // Human-readable: "Baptism (LDS)"
	PersonID   *uuid.UUID              `json:"person_id,omitempty"`   // For individual ordinances
	PersonName string                  `json:"person_name,omitempty"` // Denormalized for display
	FamilyID   *uuid.UUID              `json:"family_id,omitempty"`   // For SLGS (sealing to spouse)
	DateRaw    string                  `json:"date_raw,omitempty"`    // Raw date string
	DateSort   *time.Time              `json:"date_sort,omitempty"`   // Parsed date for sorting
	Place      string                  `json:"place,omitempty"`       // Location
	Temple     string                  `json:"temple,omitempty"`      // Temple code (TEMP)
	Status     string                  `json:"status,omitempty"`      // COMPLETED, BIC, CHILD, EXCLUDED, etc.
	Version    int64                   `json:"version"`
	UpdatedAt  time.Time               `json:"updated_at"`
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
	ListCitations(ctx context.Context, opts ListOptions) ([]CitationReadModel, int, error)
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
	ListEvents(ctx context.Context, opts ListOptions) ([]EventReadModel, int, error)
	ListEventsForPerson(ctx context.Context, personID uuid.UUID) ([]EventReadModel, error)
	ListEventsForFamily(ctx context.Context, familyID uuid.UUID) ([]EventReadModel, error)
	SaveEvent(ctx context.Context, event *EventReadModel) error
	DeleteEvent(ctx context.Context, id uuid.UUID) error

	// Attribute operations
	GetAttribute(ctx context.Context, id uuid.UUID) (*AttributeReadModel, error)
	ListAttributes(ctx context.Context, opts ListOptions) ([]AttributeReadModel, int, error)
	ListAttributesForPerson(ctx context.Context, personID uuid.UUID) ([]AttributeReadModel, error)
	SaveAttribute(ctx context.Context, attribute *AttributeReadModel) error
	DeleteAttribute(ctx context.Context, id uuid.UUID) error

	// Note operations
	GetNote(ctx context.Context, id uuid.UUID) (*NoteReadModel, error)
	ListNotes(ctx context.Context, opts ListOptions) ([]NoteReadModel, int, error)
	SaveNote(ctx context.Context, note *NoteReadModel) error
	DeleteNote(ctx context.Context, id uuid.UUID) error

	// Submitter operations
	GetSubmitter(ctx context.Context, id uuid.UUID) (*SubmitterReadModel, error)
	ListSubmitters(ctx context.Context, opts ListOptions) ([]SubmitterReadModel, int, error)
	SaveSubmitter(ctx context.Context, submitter *SubmitterReadModel) error
	DeleteSubmitter(ctx context.Context, id uuid.UUID) error

	// Association operations
	GetAssociation(ctx context.Context, id uuid.UUID) (*AssociationReadModel, error)
	ListAssociations(ctx context.Context, opts ListOptions) ([]AssociationReadModel, int, error)
	ListAssociationsForPerson(ctx context.Context, personID uuid.UUID) ([]AssociationReadModel, error)
	SaveAssociation(ctx context.Context, association *AssociationReadModel) error
	DeleteAssociation(ctx context.Context, id uuid.UUID) error

	// LDS Ordinance operations
	GetLDSOrdinance(ctx context.Context, id uuid.UUID) (*LDSOrdinanceReadModel, error)
	ListLDSOrdinances(ctx context.Context, opts ListOptions) ([]LDSOrdinanceReadModel, int, error)
	ListLDSOrdinancesForPerson(ctx context.Context, personID uuid.UUID) ([]LDSOrdinanceReadModel, error)
	ListLDSOrdinancesForFamily(ctx context.Context, familyID uuid.UUID) ([]LDSOrdinanceReadModel, error)
	SaveLDSOrdinance(ctx context.Context, ordinance *LDSOrdinanceReadModel) error
	DeleteLDSOrdinance(ctx context.Context, id uuid.UUID) error

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


// ListAll fetches all records using pagination to avoid truncation from hard-coded limits.
func ListAll[T any](ctx context.Context, pageSize int, listFn func(ctx context.Context, opts ListOptions) ([]T, int, error)) ([]T, error) {
	var all []T
	for offset := 0; ; offset += pageSize {
		page, total, err := listFn(ctx, ListOptions{Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		if offset == 0 && total > 0 {
			all = make([]T, 0, total)
		}
		all = append(all, page...)
		if len(all) >= total || len(page) < pageSize {
			break
		}
	}
	return all, nil
}
