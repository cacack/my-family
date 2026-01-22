// Package gedcom provides GEDCOM file import/export functionality.
package gedcom

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cacack/gedcom-go/decoder"
	"github.com/cacack/gedcom-go/gedcom"
	"github.com/cacack/gedcom-go/validator"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

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

	// Names contains all name variants from the GEDCOM file.
	// The first name is also stored in the main GivenName/Surname fields.
	Names []PersonNameData

	// FamilySearchID is the FamilySearch Family Tree ID from the _FSFTID tag.
	// This is a vendor extension from FamilySearch.org that uniquely identifies
	// an individual in their Family Tree database. Format: alphanumeric like "KWCJ-QN7".
	FamilySearchID string
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
	ChildIDs          []uuid.UUID
	ChildRelTypes     []domain.ChildRelationType
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
	Description string
	Cause       string // For death/burial events
	Age         string // Age at event
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
type NoteData struct {
	ID         uuid.UUID
	GedcomXref string // Cross-reference ID (e.g., "@N1@")
	Text       string // Full text with embedded newlines
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

// Importer handles GEDCOM file parsing and conversion to domain events.
type Importer struct{}

// NewImporter creates a new GEDCOM importer.
func NewImporter() *Importer {
	return &Importer{}
}

// Import parses a GEDCOM file and returns structured data for import.
func (imp *Importer) Import(ctx context.Context, reader io.Reader) (*ImportResult, []PersonData, []FamilyData, []SourceData, []CitationData, []RepositoryData, []EventData, []AttributeData, []NoteData, []SubmitterData, []AssociationData, []LDSOrdinanceData, []MediaData, error) {
	result := &ImportResult{
		PersonXrefToID:     make(map[string]uuid.UUID),
		FamilyXrefToID:     make(map[string]uuid.UUID),
		SourceXrefToID:     make(map[string]uuid.UUID),
		MediaXrefToID:      make(map[string]uuid.UUID),
		RepositoryXrefToID: make(map[string]uuid.UUID),
		NoteXrefToID:       make(map[string]uuid.UUID),
		SubmitterXrefToID:  make(map[string]uuid.UUID),
	}

	// Parse GEDCOM using cacack/gedcom-go decoder
	// The decoder handles encoding detection (UTF-8, ANSEL, Windows-1252, ISO-8859-1) automatically
	opts := decoder.DefaultOptions()
	opts.Context = ctx

	doc, err := decoder.DecodeWithOptions(reader, opts)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to parse GEDCOM: %w", err)
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
	for _, event := range indi.Events {
		switch event.Type {
		case gedcom.EventBirth:
			person.BirthDate = event.Date
			person.BirthPlace = event.Place
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
			person.DeathPlace = event.Place
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

	// Collect notes - in cacack/gedcom-go, Notes contains XRefs to note records
	// For inline notes, we need to check for notes stored differently
	// The library stores inline notes as part of the individual's Tags
	var notes []string
	for _, tag := range indi.Tags {
		if tag.Tag == "NOTE" && tag.Value != "" {
			notes = append(notes, tag.Value)
		}
	}
	if len(notes) > 0 {
		person.Notes = strings.Join(notes, "\n\n")
	}

	// Extract FamilySearch Family Tree ID (vendor extension)
	person.FamilySearchID = indi.FamilySearchID

	return person
}

// parseFamily converts a GEDCOM family record to FamilyData.
// The doc parameter provides access to individual records for PEDI lookup.
func parseFamily(fam *gedcom.Family, doc *gedcom.Document, result *ImportResult) FamilyData {
	family := FamilyData{
		ID:               uuid.New(),
		GedcomXref:       fam.XRef,
		RelationshipType: domain.RelationUnknown,
	}

	// Link husband/wife (partner1/partner2)
	// In cacack/gedcom-go, Husband and Wife are XRef strings
	if fam.Husband != "" {
		if id, ok := result.PersonXrefToID[fam.Husband]; ok {
			family.Partner1ID = &id
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Family %s: husband %s not found", fam.XRef, fam.Husband))
		}
	}
	if fam.Wife != "" {
		if id, ok := result.PersonXrefToID[fam.Wife]; ok {
			family.Partner2ID = &id
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Family %s: wife %s not found", fam.XRef, fam.Wife))
		}
	}

	// Parse events for marriage with date validation
	for _, event := range fam.Events {
		switch event.Type {
		case gedcom.EventMarriage:
			family.RelationshipType = domain.RelationMarriage
			family.MarriageDate = event.Date
			family.MarriagePlace = event.Place
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

	// Link children - look up PEDI from individual's FAMC links
	for _, childXRef := range fam.Children {
		if id, ok := result.PersonXrefToID[childXRef]; ok {
			family.ChildIDs = append(family.ChildIDs, id)

			// Look up the child's PEDI for this family
			relType := getPedigreeType(childXRef, fam.XRef, doc)
			family.ChildRelTypes = append(family.ChildRelTypes, relType)
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Family %s: child %s not found", fam.XRef, childXRef))
		}
	}

	return family
}

// getPedigreeType looks up the pedigree linkage type for a child in a family.
// Returns the appropriate ChildRelationType based on GEDCOM PEDI tag.
func getPedigreeType(childXRef, familyXRef string, doc *gedcom.Document) domain.ChildRelationType {
	// Look up the individual
	indi := doc.GetIndividual(childXRef)
	if indi == nil {
		return domain.ChildBiological // Default if not found
	}

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

// parseSource converts a GEDCOM source record to SourceData.
func parseSource(src *gedcom.Source, result *ImportResult) SourceData {
	source := SourceData{
		ID:         uuid.New(),
		GedcomXref: src.XRef,
		Title:      src.Title,
		Author:     src.Author,
		Publisher:  src.Publication,
	}

	// Link to repository if available
	if src.RepositoryRef != "" {
		if repoID, ok := result.RepositoryXrefToID[src.RepositoryRef]; ok {
			source.RepositoryID = &repoID
		} else {
			// Repository not found, store the XRef as fallback name
			source.RepositoryName = src.RepositoryRef
		}
	}

	// Note: Call number (CALN) extraction requires parsing nested REPO tags
	// which is not currently supported in the flat tag structure.
	// This can be added when gedcom-go provides structured REPO references.

	// Collect notes
	var notes []string
	for _, tag := range src.Tags {
		if tag.Tag == "NOTE" && tag.Value != "" {
			notes = append(notes, tag.Value)
		}
	}
	if len(notes) > 0 {
		source.Notes = strings.Join(notes, "\n\n")
	}

	// Default source type to "other" if not specified
	source.SourceType = string(domain.SourceOther)

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
		repository.Phone = addr.Phone
		repository.Email = addr.Email
		repository.Website = addr.Website
	}

	// Collect notes
	var notes []string
	for _, tag := range repo.Tags {
		if tag.Tag == "NOTE" && tag.Value != "" {
			notes = append(notes, tag.Value)
		}
	}
	if len(notes) > 0 {
		repository.Notes = strings.Join(notes, "\n\n")
	}

	return repository
}

// parseNote converts a GEDCOM note record to NoteData.
// gedcom-go handles CONT/CONC lines automatically - FullText() returns the complete text.
func parseNote(note *gedcom.Note) NoteData {
	return NoteData{
		ID:         uuid.New(),
		GedcomXref: note.XRef,
		Text:       note.FullText(),
	}
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
		var factType domain.FactType
		switch event.Type {
		case gedcom.EventBirth:
			factType = domain.FactPersonBirth
		case gedcom.EventDeath:
			factType = domain.FactPersonDeath
		case gedcom.EventBurial:
			factType = domain.FactPersonBurial
		case gedcom.EventCremation:
			factType = domain.FactPersonCremation
		case gedcom.EventBaptism:
			factType = domain.FactPersonBaptism
		case gedcom.EventChristening:
			factType = domain.FactPersonChristening
		case gedcom.EventEmigration:
			factType = domain.FactPersonEmigration
		case gedcom.EventImmigration:
			factType = domain.FactPersonImmigration
		case gedcom.EventNaturalization:
			factType = domain.FactPersonNaturalization
		case gedcom.EventCensus:
			factType = domain.FactPersonCensus
		case gedcom.EventOccupation:
			factType = domain.FactPersonOccupation
		case gedcom.EventResidence:
			factType = domain.FactPersonResidence
		default:
			factType = domain.FactPersonGenericEvent
		}

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
			if srcCit.Data != nil && srcCit.Data.Text != "" {
				citation.QuotedText = srcCit.Data.Text
			}

			// Extract Ancestry APID if available (vendor extension)
			if srcCit.AncestryAPID != nil {
				citation.AncestryAPID = &AncestryAPIDData{
					Database: srcCit.AncestryAPID.Database,
					Record:   srcCit.AncestryAPID.Record,
					RawValue: srcCit.AncestryAPID.Raw,
				}
			}

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
		var factType domain.FactType
		switch event.Type {
		case gedcom.EventMarriage:
			factType = domain.FactFamilyMarriage
		case gedcom.EventDivorce:
			factType = domain.FactFamilyDivorce
		case gedcom.EventMarriageBann:
			factType = domain.FactFamilyMarriageBann
		case gedcom.EventMarriageContract:
			factType = domain.FactFamilyMarriageContract
		case gedcom.EventMarriageLicense:
			factType = domain.FactFamilyMarriageLicense
		case gedcom.EventMarriageSettlement:
			factType = domain.FactFamilyMarriageSettlement
		case gedcom.EventAnnulment:
			factType = domain.FactFamilyAnnulment
		case gedcom.EventEngagement:
			factType = domain.FactFamilyEngagement
		default:
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
			if srcCit.Data != nil && srcCit.Data.Text != "" {
				citation.QuotedText = srcCit.Data.Text
			}

			// Extract Ancestry APID if available (vendor extension)
			if srcCit.AncestryAPID != nil {
				citation.AncestryAPID = &AncestryAPIDData{
					Database: srcCit.AncestryAPID.Database,
					Record:   srcCit.AncestryAPID.Record,
					RawValue: srcCit.AncestryAPID.Raw,
				}
			}

			citations = append(citations, citation)
		}
	}

	return citations
}

// mapGedcomQualityToGPS maps GEDCOM QUAY values (0-3) to GPS quality terms.
// QUAY 0 = Unreliable -> "negative"
// QUAY 1 = Questionable -> "indirect"
// QUAY 2 = Secondary evidence -> "secondary"
// QUAY 3 = Direct evidence -> "direct"
func mapGedcomQualityToGPS(quay int) string {
	switch quay {
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

// extractEventsFromIndividual extracts all life events from an individual.
// Birth and Death are excluded as they are stored on Person directly.
func extractEventsFromIndividual(indi *gedcom.Individual, personID uuid.UUID) []EventData {
	var events []EventData

	for _, event := range indi.Events {
		var factType domain.FactType
		switch event.Type {
		case gedcom.EventBirth, gedcom.EventDeath:
			// Birth and death are stored on Person entity, skip
			continue
		case gedcom.EventBurial:
			factType = domain.FactPersonBurial
		case gedcom.EventCremation:
			factType = domain.FactPersonCremation
		case gedcom.EventBaptism:
			factType = domain.FactPersonBaptism
		case gedcom.EventChristening:
			factType = domain.FactPersonChristening
		case gedcom.EventEmigration:
			factType = domain.FactPersonEmigration
		case gedcom.EventImmigration:
			factType = domain.FactPersonImmigration
		case gedcom.EventNaturalization:
			factType = domain.FactPersonNaturalization
		case gedcom.EventCensus:
			factType = domain.FactPersonCensus
		default:
			// Generic event for unrecognized types
			factType = domain.FactPersonGenericEvent
		}

		eventData := EventData{
			ID:          uuid.New(),
			OwnerType:   "person",
			OwnerID:     personID,
			FactType:    factType,
			Date:        event.Date,
			Place:       event.Place,
			Description: event.Description,
			Cause:       event.Cause,
			Age:         event.Age,
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
		Phone:      addr.Phone,
		Email:      addr.Email,
		Website:    addr.Website,
	}
	if domainAddr.IsEmpty() {
		return nil
	}
	return domainAddr
}

// extractAttributesFromIndividual extracts all attributes from an individual.
func extractAttributesFromIndividual(indi *gedcom.Individual, personID uuid.UUID) []AttributeData {
	var attributes []AttributeData

	for _, event := range indi.Events {
		var factType domain.FactType
		var value string

		switch event.Type {
		case gedcom.EventOccupation:
			factType = domain.FactPersonOccupation
			value = event.Description
		case gedcom.EventResidence:
			factType = domain.FactPersonResidence
			value = event.Place // For residence, place is the value
		default:
			// Not an attribute type, skip
			continue
		}

		attr := AttributeData{
			ID:       uuid.New(),
			PersonID: personID,
			FactType: factType,
			Value:    value,
			Date:     event.Date,
			Place:    event.Place,
		}
		attributes = append(attributes, attr)
	}

	// Also check for attribute-specific tags that might be on individual directly
	// Note: These are top-level tags that may not have DATE/PLAC subordinates accessible
	for _, tag := range indi.Tags {
		var factType domain.FactType
		var value string

		switch tag.Tag {
		case "OCCU":
			factType = domain.FactPersonOccupation
			value = tag.Value
		case "EDUC":
			factType = domain.FactPersonEducation
			value = tag.Value
		case "RELI":
			factType = domain.FactPersonReligion
			value = tag.Value
		case "TITL":
			factType = domain.FactPersonTitle
			value = tag.Value
		default:
			continue
		}

		if value == "" {
			continue
		}

		attr := AttributeData{
			ID:       uuid.New(),
			PersonID: personID,
			FactType: factType,
			Value:    value,
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

		// Combine notes into a single string
		if len(assoc.Notes) > 0 {
			associationData.Notes = strings.Join(assoc.Notes, "\n")
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
// Marriage is excluded as it is stored on Family directly.
func extractEventsFromFamily(fam *gedcom.Family, familyID uuid.UUID) []EventData {
	var events []EventData

	for _, event := range fam.Events {
		var factType domain.FactType
		switch event.Type {
		case gedcom.EventMarriage:
			// Marriage is stored on Family entity, skip
			continue
		case gedcom.EventDivorce:
			factType = domain.FactFamilyDivorce
		case gedcom.EventMarriageBann:
			factType = domain.FactFamilyMarriageBann
		case gedcom.EventMarriageContract:
			factType = domain.FactFamilyMarriageContract
		case gedcom.EventMarriageLicense:
			factType = domain.FactFamilyMarriageLicense
		case gedcom.EventMarriageSettlement:
			factType = domain.FactFamilyMarriageSettlement
		case gedcom.EventAnnulment:
			factType = domain.FactFamilyAnnulment
		case gedcom.EventEngagement:
			factType = domain.FactFamilyEngagement
		default:
			// Skip unrecognized family event types
			continue
		}

		eventData := EventData{
			ID:          uuid.New(),
			OwnerType:   "family",
			OwnerID:     familyID,
			FactType:    factType,
			Date:        event.Date,
			Place:       event.Place,
			Description: event.Description,
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
