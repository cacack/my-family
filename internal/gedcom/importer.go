// Package gedcom provides GEDCOM file import/export functionality.
package gedcom

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/cacack/gedcom-go/v3/decoder"
	"github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/cacack/gedcom-go/v3/validator"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// noteJoin separates a record's several NOTE values in its single Notes field.
const noteJoin = "\n\n"

// associationNoteJoin separates an association's NOTE values in its single
// Notes field.
const associationNoteJoin = "\n"

// ImportResult contains the results of a GEDCOM import operation.
type ImportResult struct {
	PersonsImported       int
	FamiliesImported      int
	SourcesImported       int
	CitationsImported     int
	MediaImported         int
	RepositoriesImported  int
	EventsImported        int
	AttributesImported    int
	NotesImported         int
	SubmittersImported    int
	AssociationsImported  int
	LDSOrdinancesImported int
	Warnings              []string
	Errors                []string

	// Vendor is the detected vendor that created this GEDCOM file (e.g., "ancestry", "familysearch").
	// Empty string if vendor could not be determined.
	Vendor string

	// SchemaMappings holds GEDCOM 7.0 SCHMA tag→URI definitions from the header,
	// used to interpret vendor extension tags. Empty when the file has no SCHMA.
	SchemaMappings map[string]string

	// Mappings from GEDCOM XREFs to internal UUIDs
	PersonXrefToID     map[string]uuid.UUID
	FamilyXrefToID     map[string]uuid.UUID
	SourceXrefToID     map[string]uuid.UUID
	MediaXrefToID      map[string]uuid.UUID
	RepositoryXrefToID map[string]uuid.UUID
	NoteXrefToID       map[string]uuid.UUID
	SubmitterXrefToID  map[string]uuid.UUID
}

// PersonNameData contains parsed name data for a person.
type PersonNameData struct {
	GivenName     string
	Surname       string
	NamePrefix    string          // Dr., Rev., Sir (NPFX)
	NameSuffix    string          // Jr., III, PhD (NSFX)
	SurnamePrefix string          // von, de, van (SPFX)
	Nickname      string          // Informal name (NICK)
	NameType      domain.NameType // birth, married, aka (TYPE)
	IsPrimary     bool            // First name is primary
}

// PersonData contains parsed person data ready for creation.
type PersonData struct {
	ID             uuid.UUID
	GedcomXref     string
	GivenName      string
	Surname        string
	NamePrefix     string          // Dr., Rev., Sir (NPFX)
	NameSuffix     string          // Jr., III, PhD (NSFX)
	SurnamePrefix  string          // von, de, van (SPFX)
	Nickname       string          // Informal name (NICK)
	NameType       domain.NameType // birth, married, aka (TYPE)
	Gender         domain.Gender
	BirthDate      string
	BirthPlace     string
	BirthPlaceLat  *string // Latitude in GEDCOM format (e.g., "N42.3601")
	BirthPlaceLong *string // Longitude in GEDCOM format (e.g., "W71.0589")
	DeathDate      string
	DeathPlace     string
	DeathPlaceLat  *string // Latitude in GEDCOM format
	DeathPlaceLong *string // Longitude in GEDCOM format
	Notes          string
	BirthNotes     string // NOTEs under BIRT, joined with noteJoin (#1250)
	DeathNotes     string // NOTEs under DEAT, joined with noteJoin (#1250)

	// Names contains all name variants from the GEDCOM file.
	// The first name is also stored in the main GivenName/Surname fields.
	Names []PersonNameData

	// FamilySearchID is the FamilySearch Family Tree ID from the _FSFTID tag.
	// This is a vendor extension from FamilySearch.org that uniquely identifies
	// an individual in their Family Tree database. Format: alphanumeric like "KWCJ-QN7".
	FamilySearchID string

	// ExternalIDs are GEDCOM 7.0 external identifiers (EXID tags) linking this
	// person to records in external systems (FamilySearch, Find a Grave, etc.).
	ExternalIDs []domain.ExternalIdentifier
}

// FamilyData contains parsed family data ready for creation.
type FamilyData struct {
	ID                uuid.UUID
	GedcomXref        string
	Partner1ID        *uuid.UUID
	Partner2ID        *uuid.UUID
	RelationshipType  domain.RelationType
	MarriageDate      string
	MarriagePlace     string
	MarriagePlaceLat  *string // Latitude in GEDCOM format (e.g., "N39.7817")
	MarriagePlaceLong *string // Longitude in GEDCOM format (e.g., "W89.6501")
	MarriageNotes     string  // NOTEs under MARR, joined with noteJoin (#1250)
	ChildIDs          []uuid.UUID
	ChildRelTypes     []domain.ChildRelationType

	// ExternalIDs are GEDCOM 7.0 external identifiers (EXID tags) linking this
	// family to records in external systems.
	ExternalIDs []domain.ExternalIdentifier
}

// SourceData contains parsed source data ready for creation.
type SourceData struct {
	ID             uuid.UUID
	GedcomXref     string
	SourceType     string
	Title          string
	Author         string
	Publisher      string
	PublishDate    string
	RepositoryID   *uuid.UUID // Link to Repository entity
	RepositoryName string     // Fallback for unlinked repositories
	CallNumber     string     // CALN - location within repository
	Notes          string

	// ExternalIDs are GEDCOM 7.0 external identifiers (EXID tags) linking this
	// source to records in external systems.
	ExternalIDs []domain.ExternalIdentifier
}

// RepositoryData contains parsed repository data ready for creation.
type RepositoryData struct {
	ID         uuid.UUID
	GedcomXref string
	Name       string
	Address    string
	City       string
	State      string
	PostalCode string
	Country    string
	Phone      string
	Email      string
	Website    string
	Notes      string

	// ExternalIDs are GEDCOM 7.0 external identifiers (EXID tags) linking this
	// repository to records in external systems.
	ExternalIDs []domain.ExternalIdentifier
}

// AncestryAPIDData represents an Ancestry Permanent Identifier from the _APID tag.
// This is a vendor-specific extension used by Ancestry.com to link GEDCOM data
// to their online databases.
type AncestryAPIDData struct {
	// Database is the Ancestry database ID (e.g., "7602")
	Database string

	// Record is the record ID within the database (e.g., "2771226")
	Record string

	// RawValue is the original unparsed APID value (e.g., "1,7602::2771226")
	RawValue string
}

// CitationData contains parsed citation data ready for creation.
type CitationData struct {
	ID          uuid.UUID
	SourceXref  string
	FactType    string
	FactOwnerID uuid.UUID
	Page        string
	Quality     string
	QuotedText  string
	GedcomXref  string

	// AncestryAPID is the Ancestry Permanent Identifier from the _APID tag.
	// This links the citation to a specific record in an Ancestry database.
	AncestryAPID *AncestryAPIDData

	// ExternalIDs are the citation's vendor identifiers as typed external IDs:
	// the Ancestry APID, if any, typed ancestryAPIDExternalIDType (#1175).
	ExternalIDs []domain.ExternalIdentifier
}

// MediaData contains parsed media data ready for creation.
type MediaData struct {
	ID          uuid.UUID
	GedcomXref  string
	EntityType  string // "person", "family", "source"
	EntityID    uuid.UUID
	Title       string
	MimeType    string
	MediaType   domain.MediaType
	FileRef     string // GEDCOM file reference (path or URL) - backwards compat
	Description string
	// GEDCOM 7.0 enhanced fields
	Files        []domain.MediaFile // Multiple file references (GEDCOM 7.0)
	Format       string             // Primary format/MIME type (FORM)
	Translations []string           // Translated titles (GEDCOM 7.0)
}

// EventData contains parsed life event data ready for creation.
type EventData struct {
	ID          uuid.UUID
	OwnerType   string // "person" or "family"
	OwnerID     uuid.UUID
	FactType    domain.FactType
	Date        string
	Place       string
	PlaceLat    *string // Latitude in GEDCOM format (e.g., "N42.3601")
	PlaceLong   *string // Longitude in GEDCOM format (e.g., "W71.0589")
	Address     *domain.Address
	Description string // The descriptor on a generic EVEN line
	TypeDetail  string // The event's TYPE (#1276)
	Cause       string // For death/burial events
	Age         string // Age at event
	IsNegated   bool   // Negative assertion (GEDCOM 7.0 NO tag)
	Notes       string // The event's NOTEs, joined with noteJoin (#1250)
}

// AttributeData contains parsed attribute data ready for creation.
type AttributeData struct {
	ID       uuid.UUID
	PersonID uuid.UUID
	FactType domain.FactType
	Value    string
	Date     string
	Place    string
}

// NoteData contains parsed note data ready for creation.
// GEDCOM supports two note styles:
// - Inline notes: embedded directly in an entity (stored in entity's Notes field)
// - Shared notes: top-level NOTE records that can be referenced by multiple entities via XRef
//
// GEDCOM 7.0 shared notes (SNOTE records) additionally carry a MIME media type,
// a BCP 47 language tag, and alternate-language translations.
type NoteData struct {
	ID           uuid.UUID
	GedcomXref   string                   // Cross-reference ID (e.g., "@N1@")
	Text         string                   // Full text with embedded newlines
	MIME         string                   // GEDCOM 7.0 SNOTE media type (e.g., "text/html")
	Language     string                   // GEDCOM 7.0 SNOTE BCP 47 language tag (e.g., "en")
	Translations []domain.NoteTranslation // GEDCOM 7.0 SNOTE alternate-language renderings
}

// SubmitterData contains parsed submitter data ready for creation.
// GEDCOM SUBM records track who created or submitted genealogical data.
type SubmitterData struct {
	ID         uuid.UUID
	GedcomXref string          // Cross-reference ID (e.g., "@U1@")
	Name       string          // Submitter's name (NAME)
	Address    *domain.Address // Structured address (ADDR)
	Phone      []string        // Phone numbers (PHON)
	Email      []string        // Email addresses (EMAIL)
	Language   string          // Preferred language (LANG) - only first is stored
}

// AssociationData contains parsed association data ready for creation.
// GEDCOM ASSO records link individuals with specific roles like godparents, witnesses, etc.
type AssociationData struct {
	ID            uuid.UUID
	PersonID      uuid.UUID // The person who has the association (from INDI record)
	AssociateXref string    // The associated person's XREF (resolved after person pass)
	AssociateID   uuid.UUID // Resolved UUID of the associate
	Role          string    // Role: godparent, witness, or custom
	Phrase        string    // GEDCOM 7.0 PHRASE - human-readable description
	Notes         string    // Inline note text
	NoteXrefs     []string  // Note XREFs (resolved after note pass)
}

// LDSOrdinanceData contains parsed LDS temple ordinance data ready for creation.
// GEDCOM was originally developed by the LDS Church and includes tags for temple ordinances.
type LDSOrdinanceData struct {
	ID       uuid.UUID
	Type     domain.LDSOrdinanceType // BAPL, CONL, ENDL, SLGC, SLGS
	PersonID *uuid.UUID              // For individual ordinances (BAPL, CONL, ENDL, SLGC)
	FamilyID *uuid.UUID              // For spouse sealing (SLGS)
	Date     string                  // Ordinance date
	Place    string                  // Ordinance place (optional)
	Temple   string                  // Temple code (TEMP)
	Status   string                  // Status: COMPLETED, BIC, CHILD, EXCLUDED, etc.
}

// ImportProgressCallback reports parsing progress during a GEDCOM import.
// bytesRead is the cumulative number of bytes read from the input so far.
// totalBytes is the expected total size in bytes, or -1 when the size is unknown.
type ImportProgressCallback func(bytesRead, totalBytes int64)

// ImportOptions configures an import operation.
type ImportOptions struct {
	// TotalSize is the expected total size of the input in bytes. When greater
	// than zero it allows OnProgress to report a meaningful percentage. Leave
	// as zero when the size is unknown.
	TotalSize int64

	// OnProgress, when non-nil, is invoked periodically during parsing with the
	// cumulative bytes read and the total size (or -1 when unknown). The callback
	// must be cheap and non-blocking; parsing happens on the calling goroutine.
	// When nil there is zero progress-reporting overhead.
	OnProgress ImportProgressCallback

	// ExistingXrefs lists the stored xrefs of the records already in the tree
	// that the exporter keeps (sources, repositories, notes, submitters). A
	// NOTE record whose xref is among them is given a fresh xref, and this
	// file's NOTE pointers follow it, so a second file's @N1@ stays apart
	// from the tree's (#1228).
	ExistingXrefs []string
}

// Importer handles GEDCOM file parsing and conversion to domain events.
type Importer struct{}

// NewImporter creates a new GEDCOM importer.
func NewImporter() *Importer {
	return &Importer{}
}

// Import parses a GEDCOM file and returns structured data for import.
func (imp *Importer) Import(ctx context.Context, reader io.Reader) (*ImportResult, []PersonData, []FamilyData, []SourceData, []CitationData, []RepositoryData, []EventData, []AttributeData, []NoteData, []SubmitterData, []AssociationData, []LDSOrdinanceData, []MediaData, error) {
	return imp.ImportWithOptions(ctx, reader, ImportOptions{})
}

// ImportWithOptions parses a GEDCOM file with the given options and returns
// structured data for import. It is identical to Import but allows callers to
// receive progress callbacks during decoding (see ImportOptions).
func (imp *Importer) ImportWithOptions(ctx context.Context, reader io.Reader, importOpts ImportOptions) (*ImportResult, []PersonData, []FamilyData, []SourceData, []CitationData, []RepositoryData, []EventData, []AttributeData, []NoteData, []SubmitterData, []AssociationData, []LDSOrdinanceData, []MediaData, error) {
	result := &ImportResult{
		PersonXrefToID:     make(map[string]uuid.UUID),
		FamilyXrefToID:     make(map[string]uuid.UUID),
		SourceXrefToID:     make(map[string]uuid.UUID),
		MediaXrefToID:      make(map[string]uuid.UUID),
		RepositoryXrefToID: make(map[string]uuid.UUID),
		NoteXrefToID:       make(map[string]uuid.UUID),
		SubmitterXrefToID:  make(map[string]uuid.UUID),
	}

	// Parse GEDCOM using cacack/gedcom-go decoder with lenient mode
	// The decoder handles encoding detection (UTF-8, ANSEL, Windows-1252, ISO-8859-1) automatically
	// Lenient mode collects parse errors as diagnostics instead of failing
	opts := decoder.DefaultOptions()
	opts.Context = ctx

	// Wire progress reporting through to the decoder when requested. The decoder
	// reports -1 as the total when TotalSize is unknown (zero), matching our
	// ProgressCallback contract, so we forward callbacks unchanged.
	if importOpts.OnProgress != nil {
		opts.TotalSize = importOpts.TotalSize
		opts.OnProgress = decoder.ProgressCallback(importOpts.OnProgress)
	}

	decodeResult, err := decoder.DecodeWithDiagnostics(reader, opts)
	if err != nil {
		// If we got no usable results, fail immediately
		if decodeResult == nil || decodeResult.Document == nil || len(decodeResult.Document.Records) == 0 {
			return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to parse GEDCOM: %w", err)
		}
		// Partial parse succeeded; add the fatal error as an import error
		result.Errors = append(result.Errors, fmt.Sprintf("GEDCOM parse error: %v", err))
	}

	doc := decodeResult.Document

	// Map decoder diagnostics to import warnings/errors
	for _, d := range decodeResult.Diagnostics {
		switch d.Severity {
		case decoder.SeverityError:
			result.Errors = append(result.Errors, d.String())
		default:
			result.Warnings = append(result.Warnings, d.String())
		}
	}

	// Run gedcom-go validation to catch structural issues
	v := validator.New()
	validationErrors := v.Validate(doc)
	for _, verr := range validationErrors {
		// Add validation errors as warnings (don't fail import)
		result.Warnings = append(result.Warnings, verr.Error())
	}

	// Extract vendor information from the document
	result.Vendor = string(doc.Vendor)

	// Capture GEDCOM 7.0 schema definitions (SCHMA) so vendor extension tags can
	// be interpreted by consumers of the import result.
	if doc.Schema != nil && len(doc.Schema.TagMappings) > 0 {
		result.SchemaMappings = make(map[string]string, len(doc.Schema.TagMappings))
		for tag, uri := range doc.Schema.TagMappings {
			result.SchemaMappings[tag] = uri
		}
	}

	// First pass: create repository mappings (before sources)
	var repositories []RepositoryData
	for _, repo := range doc.Repositories() {
		repository := parseRepository(repo, result)
		repositories = append(repositories, repository)
		result.RepositoryXrefToID[repo.XRef] = repository.ID
	}

	// Second pass: create source mappings (links to repositories)
	var sources []SourceData
	for _, src := range doc.Sources() {
		source := parseSource(src, result)
		sources = append(sources, source)
		result.SourceXrefToID[src.XRef] = source.ID
	}

	// Third pass: create person mappings
	var persons []PersonData
	var citations []CitationData
	var events []EventData
	var attributes []AttributeData
	for _, indi := range doc.Individuals() {
		person := parseIndividual(indi, doc, result)
		persons = append(persons, person)
		result.PersonXrefToID[indi.XRef] = person.ID

		// Extract citations from person events
		personCitations := extractCitationsFromIndividual(indi, person.ID, result)
		citations = append(citations, personCitations...)

		// Extract life events from individual
		personEvents := extractEventsFromIndividual(indi, person.ID)
		events = append(events, personEvents...)

		// Extract attributes from individual
		personAttributes := extractAttributesFromIndividual(indi, person.ID)
		attributes = append(attributes, personAttributes...)
	}

	// Fourth pass: create family mappings and resolve person references
	var families []FamilyData
	for _, fam := range doc.Families() {
		family := parseFamily(fam, doc, result)
		families = append(families, family)
		result.FamilyXrefToID[fam.XRef] = family.ID

		// Extract citations from family events
		familyCitations := extractCitationsFromFamily(fam, family.ID, result)
		citations = append(citations, familyCitations...)

		// Extract life events from family
		familyEvents := extractEventsFromFamily(fam, family.ID)
		events = append(events, familyEvents...)
	}

	// Fifth pass: parse shared (top-level) NOTE records
	// These are NOTE records that can be referenced by multiple entities via XRef
	var notes []NoteData
	for _, note := range doc.Notes() {
		noteData := parseNote(note)
		notes = append(notes, noteData)
		result.NoteXrefToID[note.XRef] = noteData.ID
	}

	// Also parse GEDCOM 7.0 shared note (SNOTE) records. Unlike plain NOTE
	// records, these carry a MIME media type, language tag, and translations.
	// SNOTE and NOTE are distinct record types, so there is no overlap.
	for _, snote := range doc.SharedNotes() {
		noteData := parseSharedNote(snote)
		notes = append(notes, noteData)
		result.NoteXrefToID[snote.XRef] = noteData.ID
	}

	// Sixth pass: parse SUBM (submitter) records
	// These track who created or submitted the genealogical data
	var submitters []SubmitterData
	for _, subm := range doc.Submitters() {
		submitterData := parseSubmitter(subm)
		submitters = append(submitters, submitterData)
		result.SubmitterXrefToID[subm.XRef] = submitterData.ID
	}

	// Seventh pass: parse ASSO (association) records from individuals
	// These are extracted after person XREFs are mapped so we can resolve associate references
	var associations []AssociationData
	for _, indi := range doc.Individuals() {
		personID := result.PersonXrefToID[indi.XRef]
		personAssocs := extractAssociationsFromIndividual(indi, personID, result)
		associations = append(associations, personAssocs...)
	}

	warnDroppedNestedNotes(doc, result)

	renameCollidingNotes(doc, importOpts.ExistingXrefs, notes, persons, families, events, sources, repositories, associations, result)

	// Eighth pass: parse LDS ordinance records from individuals and families
	// GEDCOM was originally developed by the LDS Church and includes temple ordinance tags
	var ldsOrdinances []LDSOrdinanceData
	for _, indi := range doc.Individuals() {
		personID := result.PersonXrefToID[indi.XRef]
		personOrdinances := extractLDSOrdinancesFromIndividual(indi, personID)
		ldsOrdinances = append(ldsOrdinances, personOrdinances...)
	}
	for _, fam := range doc.Families() {
		familyID := result.FamilyXrefToID[fam.XRef]
		familyOrdinances := extractLDSOrdinancesFromFamily(fam, familyID)
		ldsOrdinances = append(ldsOrdinances, familyOrdinances...)
	}

	// Ninth pass: parse OBJE (media object) records
	// These contain full media metadata including multiple files and translations
	var mediaObjects []MediaData
	for _, media := range doc.MediaObjects() {
		mediaData := parseMediaObject(media, result)
		mediaObjects = append(mediaObjects, mediaData)
		result.MediaXrefToID[media.XRef] = mediaData.ID
	}

	result.PersonsImported = len(persons)
	result.FamiliesImported = len(families)
	result.SourcesImported = len(sources)
	result.CitationsImported = len(citations)
	result.RepositoriesImported = len(repositories)
	result.EventsImported = len(events)
	result.AttributesImported = len(attributes)
	result.NotesImported = len(notes)
	result.SubmittersImported = len(submitters)
	result.AssociationsImported = len(associations)
	result.LDSOrdinancesImported = len(ldsOrdinances)
	result.MediaImported = len(mediaObjects)

	return result, persons, families, sources, citations, repositories, events, attributes, notes, submitters, associations, ldsOrdinances, mediaObjects, nil
}

// parseIndividual converts a GEDCOM individual record to PersonData.
// The doc parameter provides access to other records for PEDI lookup.
// TODO: doc parameter reserved for future cross-record lookups
func parseIndividual(indi *gedcom.Individual, _ *gedcom.Document, result *ImportResult) PersonData {
	person := PersonData{
		ID:         uuid.New(),
		GedcomXref: indi.XRef,
	}

	// Parse ALL names with all components
	if len(indi.Names) > 0 {
		for i, name := range indi.Names {
			nameData := PersonNameData{
				GivenName:     strings.TrimSpace(name.Given),
				Surname:       strings.TrimSpace(name.Surname),
				NamePrefix:    strings.TrimSpace(name.Prefix),
				NameSuffix:    strings.TrimSpace(name.Suffix),
				SurnamePrefix: strings.TrimSpace(name.SurnamePrefix),
				Nickname:      strings.TrimSpace(name.Nickname),
				NameType:      mapNameType(name.Type),
				IsPrimary:     i == 0, // First name is primary
			}

			// Given name is required - use "Unknown" if missing
			if nameData.GivenName == "" {
				nameData.GivenName = "Unknown"
				if i == 0 {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("Individual %s: missing given name, using 'Unknown'", indi.XRef))
				}
			}

			person.Names = append(person.Names, nameData)

			// First name populates the main Person fields for display
			if i == 0 {
				person.GivenName = nameData.GivenName
				person.Surname = nameData.Surname
				person.NamePrefix = nameData.NamePrefix
				person.NameSuffix = nameData.NameSuffix
				person.SurnamePrefix = nameData.SurnamePrefix
				person.Nickname = nameData.Nickname
				person.NameType = nameData.NameType
			}
		}
		// Surname can be empty (historical records, royalty, single-name individuals)
	} else {
		person.GivenName = "Unknown"
		// Leave surname empty - no name record at all
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Individual %s: no name record, using 'Unknown'", indi.XRef))
	}

	// Parse sex
	switch strings.ToUpper(indi.Sex) {
	case "M":
		person.Gender = domain.GenderMale
	case "F":
		person.Gender = domain.GenderFemale
	default:
		person.Gender = domain.GenderUnknown
	}

	// Parse events for birth and death with date validation
	// Negated events (NO BIRT / NO DEAT) are handled as LifeEvents, not on Person fields
	for _, event := range indi.Events {
		if event.IsNegative {
			continue
		}
		switch event.Type {
		case gedcom.EventBirth:
			person.BirthDate = event.Date
			person.BirthPlace = event.PlaceName()
			person.BirthNotes = eventNotes(event)
			// Extract coordinates from PlaceDetail if available
			if event.PlaceDetail != nil && event.PlaceDetail.Coordinates != nil {
				if event.PlaceDetail.Coordinates.Latitude != "" {
					lat := event.PlaceDetail.Coordinates.Latitude
					person.BirthPlaceLat = &lat
				}
				if event.PlaceDetail.Coordinates.Longitude != "" {
					long := event.PlaceDetail.Coordinates.Longitude
					person.BirthPlaceLong = &long
				}
			}
			// Validate the date if parsed
			if event.ParsedDate != nil {
				if err := event.ParsedDate.Validate(); err != nil {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("Individual %s: invalid birth date '%s': %v", indi.XRef, event.Date, err))
				}
			}
		case gedcom.EventDeath:
			person.DeathDate = event.Date
			person.DeathPlace = event.PlaceName()
			person.DeathNotes = eventNotes(event)
			// Extract coordinates from PlaceDetail if available
			if event.PlaceDetail != nil && event.PlaceDetail.Coordinates != nil {
				if event.PlaceDetail.Coordinates.Latitude != "" {
					lat := event.PlaceDetail.Coordinates.Latitude
					person.DeathPlaceLat = &lat
				}
				if event.PlaceDetail.Coordinates.Longitude != "" {
					long := event.PlaceDetail.Coordinates.Longitude
					person.DeathPlaceLong = &long
				}
			}
			// Validate the date if parsed
			if event.ParsedDate != nil {
				if err := event.ParsedDate.Validate(); err != nil {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("Individual %s: invalid death date '%s': %v", indi.XRef, event.Date, err))
				}
			}
		}
	}

	person.Notes = recordNotes(indi.Tags)

	// Extract FamilySearch Family Tree ID (vendor extension)
	person.FamilySearchID = indi.FamilySearchID

	// Extract GEDCOM 7.0 external identifiers (EXID). The FamilySearch ID is
	// kept as one too, since the read model stores external IDs but has no
	// vendor-tag field: export writes it as a FamilySearch EXID, which a
	// 5.5.1 export maps back to _FSFTID (#1175).
	person.ExternalIDs = withFamilySearchID(toDomainExternalIDs(indi.ExternalIDs), indi.FamilySearchID)

	return person
}

// familySearchExternalIDType is the EXID TYPE for a FamilySearch Family Tree
// ID. The gedcom-go converter maps an EXID of this type to _FSFTID on a
// 5.5.x export.
const familySearchExternalIDType = "https://www.familysearch.org/ark"

// ancestryAPIDExternalIDType is the external-ID type under which a citation's
// Ancestry _APID is kept (#1175). GEDCOM defines no URI for it and 7.0 has no
// EXID on a citation, so it never reaches a file as a TYPE: export writes the
// value back as _APID.
const ancestryAPIDExternalIDType = "https://www.ancestry.com/apid"

// setAncestryAPID records an Ancestry APID (vendor extension) on citation,
// both as AncestryAPID and as an external ID, which is what the read model
// stores. A nil apid leaves citation unchanged.
func setAncestryAPID(citation *CitationData, apid *gedcom.AncestryAPID) {
	if apid == nil {
		return
	}
	citation.AncestryAPID = &AncestryAPIDData{
		Database: apid.Database,
		Record:   apid.Record,
		RawValue: apid.Raw,
	}
	citation.ExternalIDs = []domain.ExternalIdentifier{{Value: apid.Raw, Type: ancestryAPIDExternalIDType}}
}

// withFamilySearchID appends the FamilySearch ID fsID to ids as a FamilySearch
// external identifier, unless it is empty or ids already holds it.
func withFamilySearchID(ids []domain.ExternalIdentifier, fsID string) []domain.ExternalIdentifier {
	if fsID == "" {
		return ids
	}
	for _, id := range ids {
		if id.Value == fsID && isFamilySearchExternalIDType(id.Type) {
			return ids
		}
	}
	return append(ids, domain.ExternalIdentifier{Value: fsID, Type: familySearchExternalIDType})
}

// toDomainExternalIDs converts gedcom-go EXID records to domain external
// identifiers, skipping any with an empty value.
func toDomainExternalIDs(exids []*gedcom.ExternalID) []domain.ExternalIdentifier {
	if len(exids) == 0 {
		return nil
	}
	result := make([]domain.ExternalIdentifier, 0, len(exids))
	for _, exid := range exids {
		if exid == nil || exid.Value == "" {
			continue
		}
		result = append(result, domain.ExternalIdentifier{
			Value: exid.Value,
			Type:  exid.Type,
		})
	}
	return result
}

// parseFamily converts a GEDCOM family record to FamilyData.
// The doc parameter provides access to individual records for PEDI lookup.
func parseFamily(fam *gedcom.Family, doc *gedcom.Document, result *ImportResult) FamilyData {
	family := FamilyData{
		ID:               uuid.New(),
		GedcomXref:       fam.XRef,
		RelationshipType: domain.RelationUnknown,
	}

	// Link husband/wife (partner1/partner2) using gedcom-go's nil-safe
	// relationship traversal, which resolves the XRef against the document
	// and returns nil for missing/broken references.
	family.Partner1ID = resolvePartner(fam.Husband, fam.HusbandIndividual(doc), fam.XRef, "husband", result)
	family.Partner2ID = resolvePartner(fam.Wife, fam.WifeIndividual(doc), fam.XRef, "wife", result)

	// Parse events for marriage with date validation
	for _, event := range fam.Events {
		switch event.Type {
		case gedcom.EventMarriage:
			// Negated marriage (NO MARR) is handled as a LifeEvent, not on Family fields
			if event.IsNegative {
				continue
			}
			family.RelationshipType = domain.RelationMarriage
			family.MarriageDate = event.Date
			family.MarriagePlace = event.PlaceName()
			family.MarriageNotes = eventNotes(event)
			// Extract coordinates from PlaceDetail if available
			if event.PlaceDetail != nil && event.PlaceDetail.Coordinates != nil {
				if event.PlaceDetail.Coordinates.Latitude != "" {
					lat := event.PlaceDetail.Coordinates.Latitude
					family.MarriagePlaceLat = &lat
				}
				if event.PlaceDetail.Coordinates.Longitude != "" {
					long := event.PlaceDetail.Coordinates.Longitude
					family.MarriagePlaceLong = &long
				}
			}
			// Validate the date if parsed
			if event.ParsedDate != nil {
				if err := event.ParsedDate.Validate(); err != nil {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("Family %s: invalid marriage date '%s': %v", fam.XRef, event.Date, err))
				}
			}
		case gedcom.EventDivorce:
			// Divorce event - we note it but keep as marriage type
		}
	}

	// Link children using gedcom-go's nil-safe relationship traversal.
	// ChildrenIndividuals resolves each child XRef against the document and
	// silently drops broken references; we detect and warn about those below.
	resolvedChildren := fam.ChildrenIndividuals(doc)
	for _, child := range resolvedChildren {
		id := result.PersonXrefToID[child.XRef]
		family.ChildIDs = append(family.ChildIDs, id)

		// Look up the child's PEDI for this family
		family.ChildRelTypes = append(family.ChildRelTypes, childPedigreeType(child, fam.XRef))
	}
	// Warn about child references that could not be resolved.
	if len(resolvedChildren) < len(fam.Children) {
		resolved := make(map[string]bool, len(resolvedChildren))
		for _, child := range resolvedChildren {
			resolved[child.XRef] = true
		}
		for _, childXRef := range fam.Children {
			if !resolved[childXRef] {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Family %s: child %s not found", fam.XRef, childXRef))
			}
		}
	}

	// Extract GEDCOM 7.0 external identifiers (EXID)
	family.ExternalIDs = toDomainExternalIDs(fam.ExternalIDs)

	return family
}

// resolvePartner maps a resolved partner individual to its internal UUID.
// xref is the raw GEDCOM reference (empty when no partner is declared) and
// indi is the result of gedcom-go's nil-safe traversal for that partner.
// A declared-but-unresolvable reference produces a warning and a nil result.
func resolvePartner(xref string, indi *gedcom.Individual, familyXRef, role string, result *ImportResult) *uuid.UUID {
	if xref == "" {
		return nil
	}
	if indi == nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Family %s: %s %s not found", familyXRef, role, xref))
		return nil
	}
	id := result.PersonXrefToID[indi.XRef]
	return &id
}

// childPedigreeType returns the pedigree linkage type for a child in a family
// based on the GEDCOM PEDI tag on the child's FAMC link.
func childPedigreeType(indi *gedcom.Individual, familyXRef string) domain.ChildRelationType {
	// Find the FAMC link for this family and check its Pedigree
	for _, famLink := range indi.ChildInFamilies {
		if famLink.FamilyXRef == familyXRef {
			switch strings.ToLower(famLink.Pedigree) {
			case "adopted", "adop":
				return domain.ChildAdopted
			case "foster":
				return domain.ChildFoster
			case "birth", "":
				return domain.ChildBiological
			case "sealing":
				// LDS sealing - treat as biological for now
				return domain.ChildBiological
			default:
				// Unknown pedigree type, default to biological
				return domain.ChildBiological
			}
		}
	}

	return domain.ChildBiological // Default if no FAMC link found
}

// renameCollidingNotes gives each NOTE record whose xref an existing record of
// the tree already holds a fresh @N<n>@ that neither the tree nor this file
// uses, and rewrites this file's NOTE pointers (kept as raw xrefs in Notes) to
// match. Without it, a second file's @N1@ is stored beside the tree's and an
// export cannot tell which one a pointer meant (#1228).
func renameCollidingNotes(doc *gedcom.Document, existing []string, notes []NoteData, persons []PersonData, families []FamilyData, events []EventData, sources []SourceData, repositories []RepositoryData, associations []AssociationData, result *ImportResult) {
	inTree := make(map[string]bool, len(existing))
	for _, xref := range existing {
		if xref != "" {
			inTree[xref] = true
		}
	}
	used := make(map[string]bool, len(inTree)+len(doc.Records))
	for xref := range inTree {
		used[xref] = true
	}
	for _, rec := range doc.Records {
		used[rec.XRef] = true
	}

	renamed := map[string]string{}
	n := 0
	for i := range notes {
		old := notes[i].GedcomXref
		if !inTree[old] {
			continue
		}
		xref := ""
		for xref == "" || used[xref] {
			n++
			xref = fmt.Sprintf("@N%d@", n)
		}
		used[xref] = true
		notes[i].GedcomXref = xref
		// Keep NoteXrefToID keyed by the pointers as rewritten below.
		delete(result.NoteXrefToID, old)
		result.NoteXrefToID[xref] = notes[i].ID
		// A file defining one xref twice is already ambiguous; its pointers
		// follow the first record.
		if _, seen := renamed[old]; !seen {
			renamed[old] = xref
		}
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Note %s renamed to %s: the tree already has a record with xref %s", old, xref, old))
	}
	if len(renamed) == 0 {
		return
	}

	rename := func(v string) (string, bool) {
		xref, ok := renamed[v]
		return xref, ok
	}
	for i := range persons {
		persons[i].Notes = renameXrefs(persons[i].Notes, noteJoin, rename)
		persons[i].BirthNotes = renameXrefs(persons[i].BirthNotes, noteJoin, rename)
		persons[i].DeathNotes = renameXrefs(persons[i].DeathNotes, noteJoin, rename)
	}
	for i := range families {
		families[i].MarriageNotes = renameXrefs(families[i].MarriageNotes, noteJoin, rename)
	}
	for i := range events {
		events[i].Notes = renameXrefs(events[i].Notes, noteJoin, rename)
	}
	for i := range sources {
		sources[i].Notes = renameXrefs(sources[i].Notes, noteJoin, rename)
	}
	for i := range repositories {
		repositories[i].Notes = renameXrefs(repositories[i].Notes, noteJoin, rename)
	}
	for i := range associations {
		associations[i].Notes = renameXrefs(associations[i].Notes, associationNoteJoin, rename)
	}
}

// recordNotes joins a record's NOTE values with noteJoin, in file order. It
// reads the raw tags rather than gedcom-go's InlineNotes/NoteXRefs so pointers
// and text keep their relative order. A raw tag value excludes its CONT/CONC
// subordinates, so inline text has them folded back in (CONT as a newline,
// CONC concatenated); a pointer is kept verbatim. Only the record's own
// level-1 NOTE lines count: a NOTE nested under an ASSO, event, NAME or other
// substructure belongs to that substructure (#1246).
func recordNotes(tags []*gedcom.Tag) string {
	var notes []string
	for i, tag := range tags {
		if tag.Level != 1 || tag.Tag != "NOTE" {
			continue
		}
		if value := noteText(tags, i); value != "" {
			notes = append(notes, value)
		}
	}
	return strings.Join(notes, noteJoin)
}

// noteText returns the value of the NOTE line tags[idx]: a pointer verbatim,
// inline text with its CONT/CONC lines folded in.
func noteText(tags []*gedcom.Tag, idx int) string {
	if value := tags[idx].Value; gedcom.IsPointerXRef(strings.TrimSpace(value)) {
		return value
	}
	return foldContinuations(tags, idx)
}

// foldContinuations returns tags[idx].Value with its direct CONT/CONC
// subordinates folded in: CONT starts a new line, CONC concatenates.
func foldContinuations(tags []*gedcom.Tag, idx int) string {
	base := tags[idx].Level
	var b strings.Builder
	b.WriteString(tags[idx].Value)
	for _, sub := range tags[idx+1:] {
		if sub.Level <= base {
			break
		}
		if sub.Level != base+1 {
			continue
		}
		switch sub.Tag {
		case "CONT":
			b.WriteString("\n")
			b.WriteString(sub.Value)
		case "CONC":
			b.WriteString(sub.Value)
		}
	}
	return b.String()
}

// recordLabels names a record type in an import warning.
var recordLabels = map[gedcom.RecordType]string{
	gedcom.RecordTypeIndividual: "Individual",
	gedcom.RecordTypeFamily:     "Family",
	gedcom.RecordTypeSource:     "Source",
	gedcom.RecordTypeRepository: "Repository",
	gedcom.RecordTypeMedia:      "Media",
	gedcom.RecordTypeSubmitter:  "Submitter",
}

// recordKeepsNotes lists the record types whose own level-1 NOTE lines the
// import keeps (recordNotes). A FAM, OBJE or SUBM record has no notes field.
var recordKeepsNotes = map[gedcom.RecordType]bool{
	gedcom.RecordTypeIndividual: true,
	gedcom.RecordTypeSource:     true,
	gedcom.RecordTypeRepository: true,
}

// droppedNoteMaxLen caps how much of a dropped note a warning quotes.
const droppedNoteMaxLen = 60

// droppedNotesPerPath caps the dropped notes warned about one by one under
// the same record type and path (e.g. INDI.BIRT.SOUR); the rest are counted
// in one summary warning, so a large file does not flood the result.
const droppedNotesPerPath = 5

// warnDroppedNestedNotes adds a warning for each NOTE or SNOTE line the
// import does not keep, naming the record, the substructure and the note, so
// the loss is visible (#1250). Kept are the level-1 NOTE lines of the record
// types in recordKeepsNotes, and the NOTEs directly under an individual's ASSO
// and events and a family's imported events. Of several BIRT, DEAT or MARR
// blocks only the last is kept, as for their date and place.
func warnDroppedNestedNotes(doc *gedcom.Document, result *ImportResult) {
	counts := map[string]int{}
	var paths []string // keys of counts, in first-seen order
	for _, rec := range doc.Records {
		label := recordLabels[rec.Type]
		if label == "" {
			label = string(rec.Type)
		}
		for _, d := range droppedNotes(rec.Tags, recordKeepsNotes[rec.Type], keptEventNotes(rec)) {
			key := strings.Join(append([]string{string(rec.Type)}, d.path...), ".")
			if counts[key] == 0 {
				paths = append(paths, key)
			}
			counts[key]++
			if counts[key] > droppedNotesPerPath {
				continue
			}
			where := ""
			if len(d.path) > 0 {
				where = " under " + strings.Join(d.path, ".")
			}
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s %s: %s%s not imported: %q", label, rec.XRef, d.tag, where, d.text))
		}
	}
	for _, key := range paths {
		if n := counts[key] - droppedNotesPerPath; n > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%d more notes under %s not imported", n, key))
		}
	}
}

// keptEventNotes reports, for a record, whether the NOTEs directly under a
// level-1 block are kept; last is true for the last block of its kind.
func keptEventNotes(rec *gedcom.Record) func(block string, last bool) bool {
	if indi, ok := rec.GetIndividual(); ok {
		return func(block string, last bool) bool {
			switch block {
			case "BIRT", "DEAT":
				return last // parseIndividual keeps the last one's notes
			case "ASSO":
				return true
			}
			eventType := gedcom.EventType(strings.TrimPrefix(block, "NO "))
			return slices.ContainsFunc(indi.Events, func(e *gedcom.Event) bool {
				return e != nil && e.Type == eventType
			})
		}
	}
	if rec.IsFamily() {
		return func(block string, last bool) bool {
			if block == "MARR" {
				return last // parseFamily keeps the last one's notes
			}
			_, ok := familyEventFactType(gedcom.EventType(strings.TrimPrefix(block, "NO ")))
			return ok
		}
	}
	return func(string, bool) bool { return false }
}

// droppedNote is a NOTE or SNOTE line the import does not keep: its tag, the
// path of the substructure it sits under (empty for the record's own), and
// its text or pointer, cut to droppedNoteMaxLen.
type droppedNote struct {
	tag  string
	path []string
	text string
}

// droppedNotes returns the non-empty NOTE and SNOTE lines in a record's tags
// that the import does not keep: a level-1 line unless keepsOwn and it is a
// NOTE, a level-2 line unless kept reports true for its level-1 block, and any
// deeper one. A level-1 block is named by its tag, a 7.0 negative assertion
// as "NO <event>".
func droppedNotes(tags []*gedcom.Tag, keepsOwn bool, kept func(block string, last bool) bool) []droppedNote {
	blockName := func(tag *gedcom.Tag) string {
		if tag.Tag == "NO" {
			return "NO " + strings.TrimSpace(tag.Value)
		}
		return tag.Tag
	}
	lastBlock := map[string]int{}
	for i, tag := range tags {
		if tag.Level == 1 {
			lastBlock[blockName(tag)] = i
		}
	}

	var dropped []droppedNote
	var path []string // path[l-1] names the open level-l substructure
	block := -1       // index of the open level-1 block
	for i, tag := range tags {
		if tag.Level < 1 {
			path = path[:0]
			continue
		}
		path = path[:min(len(path), tag.Level-1)]
		if tag.Tag == "NOTE" || tag.Tag == "SNOTE" {
			var keep bool
			switch {
			case tag.Level == 1:
				keep = keepsOwn && tag.Tag == "NOTE"
			case tag.Level == 2 && len(path) == 1:
				keep = kept(path[0], lastBlock[path[0]] == block)
			}
			if text := noteText(tags, i); !keep && text != "" {
				if r := []rune(text); len(r) > droppedNoteMaxLen {
					text = string(r[:droppedNoteMaxLen]) + "..."
				}
				dropped = append(dropped, droppedNote{tag: tag.Tag, path: slices.Clone(path), text: text})
			}
		}
		name := tag.Tag
		if tag.Level == 1 {
			name, block = blockName(tag), i
		}
		path = append(path, name)
	}
	return dropped
}

// eventNotes joins an event's NOTE values with noteJoin: its NOTE pointers,
// kept verbatim, then its inline text. gedcom-go keeps the two in separate
// lists, so their relative order in the file is not known; the encoder writes
// pointers first, so an export restores that order (#1250).
func eventNotes(event *gedcom.Event) string {
	return joinNotes(append(slices.Clone(event.NoteXRefs), event.InlineNotes...)...)
}

// joinNotes joins the non-empty notes with noteJoin.
func joinNotes(notes ...string) string {
	kept := make([]string, 0, len(notes))
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, noteJoin)
}

// parseSource converts a GEDCOM source record to SourceData.
func parseSource(src *gedcom.Source, result *ImportResult) SourceData {
	source := SourceData{
		ID:         uuid.New(),
		GedcomXref: src.XRef,
		Title:      src.Title,
		Author:     src.Author,
		Publisher:  src.Publication,
	}

	// Link to repository if available. The domain models one repository per
	// source, so take the first REPO link in document order (v2's scalar
	// held the last); it carries the inline NAME and CALN subordinates.
	if len(src.RepositoryLinks) > 0 {
		link := src.RepositoryLinks[0]
		switch {
		case link.XRef != "":
			if repoID, ok := result.RepositoryXrefToID[link.XRef]; ok {
				source.RepositoryID = &repoID
			} else {
				// Repository not found, store the XRef as fallback name
				source.RepositoryName = link.XRef
			}
		case link.Inline != nil && link.Inline.Name != "":
			// Inline SOUR.REPO with a NAME subordinate (name-only source).
			source.RepositoryName = link.Inline.Name
		}
		// The domain models a single call number; GEDCOM allows several.
		if len(link.CallNumbers) > 0 {
			source.CallNumber = link.CallNumbers[0].Value
		}
	}

	source.Notes = recordNotes(src.Tags)

	// Default source type to "other" if not specified
	source.SourceType = string(domain.SourceOther)

	// Extract GEDCOM 7.0 external identifiers (EXID)
	source.ExternalIDs = toDomainExternalIDs(src.ExternalIDs)

	return source
}

// parseRepository converts a GEDCOM repository record to RepositoryData.
// TODO: result parameter reserved for future error/warning tracking
func parseRepository(repo *gedcom.Repository, _ *ImportResult) RepositoryData {
	repository := RepositoryData{
		ID:         uuid.New(),
		GedcomXref: repo.XRef,
		Name:       repo.Name,
	}

	// Extract address components if available
	if repo.Address != nil {
		addr := repo.Address
		// Combine address lines
		addrParts := []string{}
		if addr.Line1 != "" {
			addrParts = append(addrParts, addr.Line1)
		}
		if addr.Line2 != "" {
			addrParts = append(addrParts, addr.Line2)
		}
		if addr.Line3 != "" {
			addrParts = append(addrParts, addr.Line3)
		}
		repository.Address = strings.Join(addrParts, ", ")
		repository.City = addr.City
		repository.State = addr.State
		repository.PostalCode = addr.PostalCode
		repository.Country = addr.Country
	}

	// Contact details are siblings of ADDR on the repository (each may repeat;
	// the domain keeps the first).
	if len(repo.Phone) > 0 {
		repository.Phone = repo.Phone[0]
	}
	if len(repo.Email) > 0 {
		repository.Email = repo.Email[0]
	}
	if len(repo.Website) > 0 {
		repository.Website = repo.Website[0]
	}

	repository.Notes = recordNotes(repo.Tags)

	// Extract GEDCOM 7.0 external identifiers (EXID)
	repository.ExternalIDs = toDomainExternalIDs(repo.ExternalIDs)

	return repository
}

// parseNote converts a GEDCOM note record to NoteData.
// gedcom-go folds CONT/CONC lines into Note.Text, the complete text.
func parseNote(note *gedcom.Note) NoteData {
	return NoteData{
		ID:         uuid.New(),
		GedcomXref: note.XRef,
		Text:       note.Text,
	}
}

// parseSharedNote converts a GEDCOM 7.0 shared note (SNOTE) record to NoteData,
// preserving its MIME media type, language tag, and alternate-language
// translations. Multi-line primary text round-trips (gedcom-go >= v2.2.2 folds
// CONT/CONC into SharedNote.Text), but multi-line *translation* text is
// currently truncated to its first line: gedcom-go does not yet fold CONT/CONC
// under a TRAN sub-value (upstream cacack/gedcom-go#339).
func parseSharedNote(snote *gedcom.SharedNote) NoteData {
	data := NoteData{
		ID:         uuid.New(),
		GedcomXref: snote.XRef,
		Text:       snote.Text,
		MIME:       snote.MIME,
		Language:   snote.Language,
	}
	for _, tran := range snote.Translations {
		if tran == nil {
			continue
		}
		data.Translations = append(data.Translations, domain.NoteTranslation{
			Text:     tran.Value,
			MIME:     tran.MIME,
			Language: tran.Language,
		})
	}
	return data
}

// parseSubmitter converts a GEDCOM submitter record to SubmitterData.
func parseSubmitter(subm *gedcom.Submitter) SubmitterData {
	submitter := SubmitterData{
		ID:         uuid.New(),
		GedcomXref: subm.XRef,
		Name:       subm.Name,
		Phone:      subm.Phone,
		Email:      subm.Email,
	}

	// Convert address if present
	if subm.Address != nil {
		submitter.Address = convertGedcomAddress(subm.Address)
	}

	// GEDCOM allows multiple languages, we store only the first
	if len(subm.Language) > 0 {
		submitter.Language = subm.Language[0]
	}

	return submitter
}

// extractCitationsFromIndividual extracts all citations from individual events.
// TODO: result parameter reserved for future error/warning tracking
func extractCitationsFromIndividual(indi *gedcom.Individual, personID uuid.UUID, _ *ImportResult) []CitationData {
	var citations []CitationData

	for _, event := range indi.Events {
		// A GEDCOM 7.0 "NO OCCU" decodes as an event of type OCCU and is
		// imported as a negated generic event, so its citations follow that
		// event's fact type.
		factType := individualEventFactType(event.Type)

		for _, srcCit := range event.SourceCitations {
			if srcCit.SourceXRef == "" {
				continue
			}

			citation := CitationData{
				ID:          uuid.New(),
				SourceXref:  srcCit.SourceXRef,
				FactType:    string(factType),
				FactOwnerID: personID,
				Page:        srcCit.Page,
				Quality:     mapGedcomQualityToGPS(srcCit.Quality),
			}

			// Extract quoted text if available
			citation.QuotedText = citationQuotedText(srcCit.Data)

			setAncestryAPID(&citation, srcCit.AncestryAPID)

			citations = append(citations, citation)
		}
	}

	return citations
}

// extractCitationsFromFamily extracts all citations from family events.
// TODO: result parameter reserved for future error/warning tracking
func extractCitationsFromFamily(fam *gedcom.Family, familyID uuid.UUID, _ *ImportResult) []CitationData {
	var citations []CitationData

	for _, event := range fam.Events {
		factType, ok := familyEventFactType(event.Type)
		if !ok {
			continue // Skip events we don't track citations for
		}

		for _, srcCit := range event.SourceCitations {
			if srcCit.SourceXRef == "" {
				continue
			}

			citation := CitationData{
				ID:          uuid.New(),
				SourceXref:  srcCit.SourceXRef,
				FactType:    string(factType),
				FactOwnerID: familyID,
				Page:        srcCit.Page,
				Quality:     mapGedcomQualityToGPS(srcCit.Quality),
			}

			// Extract quoted text if available
			citation.QuotedText = citationQuotedText(srcCit.Data)

			setAncestryAPID(&citation, srcCit.AncestryAPID)

			citations = append(citations, citation)
		}
	}

	return citations
}

// citationQuotedText returns a citation's DATA.TEXT passages joined with "\n",
// in document order, or "" when there are none. The domain keeps one quoted
// text; GEDCOM allows TEXT to repeat.
func citationQuotedText(data *gedcom.SourceCitationData) string {
	if data == nil {
		return ""
	}
	var parts []string
	for _, t := range data.Text {
		if t != nil && t.Value != "" {
			parts = append(parts, t.Value)
		}
	}
	return strings.Join(parts, "\n")
}

// mapGedcomQualityToGPS maps GEDCOM QUAY values (0-3) to GPS quality terms.
// QUAY 0 = Unreliable -> "negative"
// QUAY 1 = Questionable -> "indirect"
// QUAY 2 = Secondary evidence -> "secondary"
// QUAY 3 = Direct evidence -> "direct"
// No QUAY (nil) -> "" (no rating recorded)
func mapGedcomQualityToGPS(quay *int) string {
	if quay == nil {
		return ""
	}
	switch *quay {
	case 0:
		return "negative"
	case 1:
		return "indirect"
	case 2:
		return "secondary"
	case 3:
		return "direct"
	default:
		return ""
	}
}

// ValidateImportData checks for issues that would prevent import.
func ValidateImportData(persons []PersonData, families []FamilyData) error {
	if len(persons) == 0 && len(families) == 0 {
		return errors.New("GEDCOM file contains no individuals or families")
	}
	return nil
}

// individualEventFactTypes maps each individual event tag gedcom-go decodes to
// its fact type. A tag not listed (EVEN, or a negated attribute such as
// "NO OCCU") is a generic event.
var individualEventFactTypes = map[gedcom.EventType]domain.FactType{
	gedcom.EventBirth:            domain.FactPersonBirth,
	gedcom.EventDeath:            domain.FactPersonDeath,
	gedcom.EventBurial:           domain.FactPersonBurial,
	gedcom.EventCremation:        domain.FactPersonCremation,
	gedcom.EventBaptism:          domain.FactPersonBaptism,
	gedcom.EventChristening:      domain.FactPersonChristening,
	gedcom.EventEmigration:       domain.FactPersonEmigration,
	gedcom.EventImmigration:      domain.FactPersonImmigration,
	gedcom.EventNaturalization:   domain.FactPersonNaturalization,
	gedcom.EventCensus:           domain.FactPersonCensus,
	gedcom.EventResidence:        domain.FactPersonResidence,
	gedcom.EventAdoption:         domain.FactPersonAdoption,
	gedcom.EventBarMitzvah:       domain.FactPersonBarMitzvah,
	gedcom.EventBasMitzvah:       domain.FactPersonBasMitzvah,
	gedcom.EventBlessing:         domain.FactPersonBlessing,
	gedcom.EventAdultChristening: domain.FactPersonAdultChristening,
	gedcom.EventConfirmation:     domain.FactPersonConfirmation,
	gedcom.EventFirstCommunion:   domain.FactPersonFirstCommunion,
	gedcom.EventGraduation:       domain.FactPersonGraduation,
	gedcom.EventRetirement:       domain.FactPersonRetirement,
	gedcom.EventOrdination:       domain.FactPersonOrdination,
	gedcom.EventProbate:          domain.FactPersonProbate,
	gedcom.EventWill:             domain.FactPersonWill,
}

// individualEventFactType returns the fact type of an individual event tag,
// or FactPersonGenericEvent for a tag with none of its own.
func individualEventFactType(t gedcom.EventType) domain.FactType {
	if factType, ok := individualEventFactTypes[t]; ok {
		return factType
	}
	return domain.FactPersonGenericEvent
}

// extractEventsFromIndividual extracts all life events from an individual.
// Birth and Death are excluded as they are stored on Person directly,
// unless they are negated (NO BIRT / NO DEAT), which represent negative assertions
// and must be stored as LifeEvents.
func extractEventsFromIndividual(indi *gedcom.Individual, personID uuid.UUID) []EventData {
	var events []EventData

	for _, event := range indi.Events {
		var factType domain.FactType

		// Negated birth/death events (NO BIRT / NO DEAT) are stored as LifeEvents,
		// not on Person fields, because they represent "event did NOT occur".
		if event.IsNegative && (event.Type == gedcom.EventBirth || event.Type == gedcom.EventDeath) {
			switch event.Type {
			case gedcom.EventBirth:
				factType = domain.FactPersonBirth
			case gedcom.EventDeath:
				factType = domain.FactPersonDeath
			}
			eventData := EventData{
				ID:          uuid.New(),
				OwnerType:   "person",
				OwnerID:     personID,
				FactType:    factType,
				Date:        event.Date,
				Place:       event.PlaceName(),
				Description: event.Description,
				TypeDetail:  event.EventTypeDetail,
				Cause:       event.Cause,
				Age:         event.Age,
				IsNegated:   true,
				Notes:       eventNotes(event),
			}
			if event.PlaceDetail != nil && event.PlaceDetail.Coordinates != nil {
				if event.PlaceDetail.Coordinates.Latitude != "" {
					lat := event.PlaceDetail.Coordinates.Latitude
					eventData.PlaceLat = &lat
				}
				if event.PlaceDetail.Coordinates.Longitude != "" {
					long := event.PlaceDetail.Coordinates.Longitude
					eventData.PlaceLong = &long
				}
			}
			if event.Address != nil {
				eventData.Address = convertGedcomAddress(event.Address)
			}
			events = append(events, eventData)
			continue
		}

		switch event.Type {
		case gedcom.EventBirth, gedcom.EventDeath:
			// Non-negated birth and death are stored on Person entity, skip
			continue
		default:
			// RESI is imported once, as a life event: unlike an attribute, it
			// keeps the residence's coordinates, address, notes and citations.
			factType = individualEventFactType(event.Type)
		}

		eventData := EventData{
			ID:          uuid.New(),
			OwnerType:   "person",
			OwnerID:     personID,
			FactType:    factType,
			Date:        event.Date,
			Place:       event.PlaceName(),
			Description: event.Description,
			TypeDetail:  event.EventTypeDetail,
			Cause:       event.Cause,
			Age:         event.Age,
			IsNegated:   event.IsNegative,
			Notes:       eventNotes(event),
		}
		// Extract coordinates from PlaceDetail if available
		if event.PlaceDetail != nil && event.PlaceDetail.Coordinates != nil {
			if event.PlaceDetail.Coordinates.Latitude != "" {
				lat := event.PlaceDetail.Coordinates.Latitude
				eventData.PlaceLat = &lat
			}
			if event.PlaceDetail.Coordinates.Longitude != "" {
				long := event.PlaceDetail.Coordinates.Longitude
				eventData.PlaceLong = &long
			}
		}
		// Extract structured address if available
		if event.Address != nil {
			eventData.Address = convertGedcomAddress(event.Address)
		}
		events = append(events, eventData)
	}

	return events
}

// convertGedcomAddress converts a gedcom.Address to a domain.Address.
func convertGedcomAddress(addr *gedcom.Address) *domain.Address {
	if addr == nil {
		return nil
	}
	domainAddr := &domain.Address{
		Line1:      addr.Line1,
		Line2:      addr.Line2,
		Line3:      addr.Line3,
		City:       addr.City,
		State:      addr.State,
		PostalCode: addr.PostalCode,
		Country:    addr.Country,
	}
	if domainAddr.IsEmpty() {
		return nil
	}
	return domainAddr
}

// extractAttributesFromIndividual extracts all attributes from an individual.
func extractAttributesFromIndividual(indi *gedcom.Individual, personID uuid.UUID) []AttributeData {
	var attributes []AttributeData

	// RESI decodes into indi.Events and is imported as a life event
	// (extractEventsFromIndividual), not here, so it is stored once. The
	// attributes are read from indi.Attributes, which gedcom-go fills from the
	// level-1 attribute lines only, with their DATE and PLAC (#1275). The raw
	// indi.Tags would also yield nested lines such as OBJE.TITL or BAPM.RELI.
	// Attribute tags with no person fact type (DSCR, NATI, CAST, PROP, SSN,
	// IDNO, NCHI, NMR, FACT) are not imported.
	for _, ga := range indi.Attributes {
		var factType domain.FactType

		switch ga.Type {
		case gedcom.AttributeOccupation:
			factType = domain.FactPersonOccupation
		case gedcom.AttributeEducation:
			factType = domain.FactPersonEducation
		case gedcom.AttributeReligion:
			factType = domain.FactPersonReligion
		case gedcom.AttributeTitle:
			factType = domain.FactPersonTitle
		default:
			continue
		}

		if ga.Value == "" {
			continue
		}

		attr := AttributeData{
			ID:       uuid.New(),
			PersonID: personID,
			FactType: factType,
			Value:    ga.Value,
			Date:     ga.Date,
			Place:    ga.PlaceName(),
		}

		attributes = append(attributes, attr)
	}

	return attributes
}

// extractAssociationsFromIndividual extracts all ASSO records from an individual.
// GEDCOM associations link individuals with specific roles like godparents, witnesses, etc.
func extractAssociationsFromIndividual(indi *gedcom.Individual, personID uuid.UUID, result *ImportResult) []AssociationData {
	var associations []AssociationData

	for _, assoc := range indi.Associations {
		// Skip if no associate reference
		if assoc.IndividualXRef == "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Individual %s: association without IndividualXRef, skipping", indi.XRef))
			continue
		}

		// Look up the associate's UUID
		associateID, found := result.PersonXrefToID[assoc.IndividualXRef]
		if !found {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Individual %s: association references unknown individual %s, skipping", indi.XRef, assoc.IndividualXRef))
			continue
		}

		// Map GEDCOM role to lowercase
		role := mapAssociationRole(assoc.Role)
		if role == "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Individual %s: association without role, using 'unknown'", indi.XRef))
			role = "unknown"
		}

		associationData := AssociationData{
			ID:            uuid.New(),
			PersonID:      personID,
			AssociateXref: assoc.IndividualXRef,
			AssociateID:   associateID,
			Role:          role,
			Phrase:        assoc.Phrase,
		}

		// Combine notes into a single string. Pointers are kept verbatim, as
		// v2's interleaved Notes slice did; v3 groups them ahead of the text.
		if notes := append(slices.Clone(assoc.NoteXRefs), assoc.InlineNotes...); len(notes) > 0 {
			associationData.Notes = strings.Join(notes, associationNoteJoin)
		}

		associations = append(associations, associationData)
	}

	return associations
}

// mapAssociationRole converts GEDCOM RELA values to lowercase role names.
// GODP -> godparent, WITN -> witness, custom values -> lowercase.
func mapAssociationRole(gedcomRole string) string {
	switch strings.ToUpper(gedcomRole) {
	case "GODP":
		return domain.RoleGodparent
	case "WITN":
		return domain.RoleWitness
	default:
		// Custom role: lowercase and trim
		return strings.ToLower(strings.TrimSpace(gedcomRole))
	}
}

// extractEventsFromFamily extracts all life events from a family.
// Marriage is excluded as it is stored on Family directly,
// unless it is negated (NO MARR), which represents a negative assertion
// and must be stored as a LifeEvent.
func extractEventsFromFamily(fam *gedcom.Family, familyID uuid.UUID) []EventData {
	var events []EventData

	for _, event := range fam.Events {
		// Negated marriage events (NO MARR) are stored as LifeEvents,
		// not on Family fields, because they represent "event did NOT occur".
		if event.IsNegative && event.Type == gedcom.EventMarriage {
			eventData := EventData{
				ID:          uuid.New(),
				OwnerType:   "family",
				OwnerID:     familyID,
				FactType:    domain.FactFamilyMarriage,
				Date:        event.Date,
				Place:       event.PlaceName(),
				Description: event.Description,
				TypeDetail:  event.EventTypeDetail,
				IsNegated:   true,
				Notes:       eventNotes(event),
			}
			if event.PlaceDetail != nil && event.PlaceDetail.Coordinates != nil {
				if event.PlaceDetail.Coordinates.Latitude != "" {
					lat := event.PlaceDetail.Coordinates.Latitude
					eventData.PlaceLat = &lat
				}
				if event.PlaceDetail.Coordinates.Longitude != "" {
					long := event.PlaceDetail.Coordinates.Longitude
					eventData.PlaceLong = &long
				}
			}
			if event.Address != nil {
				eventData.Address = convertGedcomAddress(event.Address)
			}
			events = append(events, eventData)
			continue
		}

		if event.Type == gedcom.EventMarriage {
			// Non-negated marriage is stored on Family entity, skip
			continue
		}
		factType, ok := familyEventFactType(event.Type)
		if !ok {
			// Skip unrecognized family event types
			continue
		}

		eventData := EventData{
			ID:          uuid.New(),
			OwnerType:   "family",
			OwnerID:     familyID,
			FactType:    factType,
			Date:        event.Date,
			Place:       event.PlaceName(),
			Description: event.Description,
			TypeDetail:  event.EventTypeDetail,
			IsNegated:   event.IsNegative,
			Notes:       eventNotes(event),
		}
		// Extract coordinates from PlaceDetail if available
		if event.PlaceDetail != nil && event.PlaceDetail.Coordinates != nil {
			if event.PlaceDetail.Coordinates.Latitude != "" {
				lat := event.PlaceDetail.Coordinates.Latitude
				eventData.PlaceLat = &lat
			}
			if event.PlaceDetail.Coordinates.Longitude != "" {
				long := event.PlaceDetail.Coordinates.Longitude
				eventData.PlaceLong = &long
			}
		}
		// Extract structured address if available
		if event.Address != nil {
			eventData.Address = convertGedcomAddress(event.Address)
		}
		events = append(events, eventData)
	}

	return events
}

// familyEventFactTypes maps each family event tag gedcom-go decodes to its
// fact type. gedcom-go has no constant for a generic EVEN, so it is spelled
// out (#1285, cacack/gedcom-go#775).
var familyEventFactTypes = map[gedcom.EventType]domain.FactType{
	gedcom.EventMarriage:           domain.FactFamilyMarriage,
	gedcom.EventDivorce:            domain.FactFamilyDivorce,
	gedcom.EventMarriageBann:       domain.FactFamilyMarriageBann,
	gedcom.EventMarriageContract:   domain.FactFamilyMarriageContract,
	gedcom.EventMarriageLicense:    domain.FactFamilyMarriageLicense,
	gedcom.EventMarriageSettlement: domain.FactFamilyMarriageSettlement,
	gedcom.EventAnnulment:          domain.FactFamilyAnnulment,
	gedcom.EventEngagement:         domain.FactFamilyEngagement,
	gedcom.EventDivorceFiling:      domain.FactFamilyDivorceFiling,
	gedcom.EventCensus:             domain.FactFamilyCensus,
	gedcom.EventResidence:          domain.FactFamilyResidence,
	gedcom.EventType("EVEN"):       domain.FactFamilyGenericEvent,
}

// familyEventFactType maps a family event type to its fact type; ok is false
// for a type the importer does not keep.
func familyEventFactType(eventType gedcom.EventType) (factType domain.FactType, ok bool) {
	factType, ok = familyEventFactTypes[eventType]
	return factType, ok
}

// mapNameType converts a GEDCOM name TYPE value to a domain NameType.
func mapNameType(gedcomType string) domain.NameType {
	switch strings.ToLower(gedcomType) {
	case "birth":
		return domain.NameTypeBirth
	case "married", "marriage":
		return domain.NameTypeMarried
	case "aka", "alias":
		return domain.NameTypeAKA
	case "immigrant", "immigration":
		return domain.NameTypeImmigrant
	case "religious":
		return domain.NameTypeReligious
	case "professional", "stage":
		return domain.NameTypeProfessional
	default:
		// Return empty for unknown types (valid per NameType.IsValid)
		return ""
	}
}

// extractLDSOrdinancesFromIndividual extracts LDS ordinance records from an individual.
// Individual ordinances: BAPL (Baptism), CONL (Confirmation), ENDL (Endowment), SLGC (Sealing to Parents)
func extractLDSOrdinancesFromIndividual(indi *gedcom.Individual, personID uuid.UUID) []LDSOrdinanceData {
	var ordinances []LDSOrdinanceData

	for _, ord := range indi.LDSOrdinances {
		// Map gedcom ordinance type to domain type
		var ordType domain.LDSOrdinanceType
		switch ord.Type {
		case gedcom.LDSBaptism:
			ordType = domain.LDSBaptism
		case gedcom.LDSConfirmation:
			ordType = domain.LDSConfirmation
		case gedcom.LDSEndowment:
			ordType = domain.LDSEndowment
		case gedcom.LDSSealingChild:
			ordType = domain.LDSSealingChild
		default:
			// Skip unknown ordinance types
			continue
		}

		ordinances = append(ordinances, LDSOrdinanceData{
			ID:       uuid.New(),
			Type:     ordType,
			PersonID: &personID,
			Date:     ord.Date,
			Place:    ord.Place,
			Temple:   ord.Temple,
			Status:   ord.Status,
		})
	}

	return ordinances
}

// extractLDSOrdinancesFromFamily extracts LDS ordinance records from a family.
// Family ordinance: SLGS (Sealing to Spouse)
func extractLDSOrdinancesFromFamily(fam *gedcom.Family, familyID uuid.UUID) []LDSOrdinanceData {
	var ordinances []LDSOrdinanceData

	// Extract SLGS (Sealing to Spouse) from family's LDS ordinances
	for _, ord := range fam.LDSOrdinances {
		if ord.Type == gedcom.LDSSealingSpouse {
			ordinances = append(ordinances, LDSOrdinanceData{
				ID:       uuid.New(),
				Type:     domain.LDSSealingSpouse,
				FamilyID: &familyID,
				Date:     ord.Date,
				Place:    ord.Place,
				Temple:   ord.Temple,
				Status:   ord.Status,
			})
		}
	}

	return ordinances
}

// parseMediaObject converts a GEDCOM media object (OBJE) record to MediaData.
// Note: This parses top-level OBJE records. Entity-specific media links are resolved
// during entity import by referencing the MediaXrefToID mapping.
// TODO: result parameter reserved for future error/warning tracking
func parseMediaObject(media *gedcom.MediaObject, _ *ImportResult) MediaData {
	mediaData := MediaData{
		ID:         uuid.New(),
		GedcomXref: media.XRef,
	}

	// Parse all files from the OBJE record
	for i, file := range media.Files {
		domainFile := domain.MediaFile{
			Path:      file.FileRef,
			Format:    file.Form,
			MediaType: file.MediaType,
			Title:     file.Title,
		}

		// Parse translations for this file
		for _, trans := range file.Translations {
			domainFile.Translations = append(domainFile.Translations, domain.MediaTranslation{
				Path:   trans.FileRef,
				Format: trans.Form,
			})
		}

		mediaData.Files = append(mediaData.Files, domainFile)

		// First file's data is used for legacy single-file fields
		if i == 0 {
			mediaData.FileRef = file.FileRef
			mediaData.MimeType = file.Form
			mediaData.Format = file.Form
			if file.Title != "" {
				mediaData.Title = file.Title
			}
			// Map GEDCOM media type (MEDI) to domain MediaType
			mediaData.MediaType = mapGedcomMediaType(file.MediaType)
		}
	}

	// Fallback title from the first file or XRef
	if mediaData.Title == "" {
		if len(media.Files) > 0 && media.Files[0].Title != "" {
			mediaData.Title = media.Files[0].Title
		} else {
			mediaData.Title = media.XRef // Use XRef as fallback
		}
	}

	return mediaData
}

// mapGedcomMediaType converts GEDCOM MEDI values to domain MediaType.
// GEDCOM 7.0 MEDI types: AUDIO, BOOK, CARD, ELECTRONIC, FICHE, FILM, MAGAZINE,
// MANUSCRIPT, MAP, NEWSPAPER, PHOTO, TOMBSTONE, VIDEO
func mapGedcomMediaType(medi string) domain.MediaType {
	switch strings.ToUpper(medi) {
	case "PHOTO", "PHOTOGRAPH":
		return domain.MediaPhoto
	case "AUDIO":
		return domain.MediaAudio
	case "VIDEO":
		return domain.MediaVideo
	case "BOOK", "MAGAZINE", "NEWSPAPER", "MANUSCRIPT":
		return domain.MediaDocument
	case "ELECTRONIC", "FICHE", "FILM":
		return domain.MediaDocument
	case "CARD", "MAP", "TOMBSTONE":
		return domain.MediaPhoto // Treat as photos/images
	default:
		return domain.MediaDocument // Default to document
	}
}
