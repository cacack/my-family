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
	PersonsImported      int
	FamiliesImported     int
	SourcesImported      int
	CitationsImported    int
	MediaImported        int
	RepositoriesImported int
	EventsImported       int
	AttributesImported   int
	Warnings             []string
	Errors               []string

	// Vendor is the detected vendor that created this GEDCOM file (e.g., "ancestry", "familysearch").
	// Empty string if vendor could not be determined.
	Vendor string

	// Mappings from GEDCOM XREFs to internal UUIDs
	PersonXrefToID     map[string]uuid.UUID
	FamilyXrefToID     map[string]uuid.UUID
	SourceXrefToID     map[string]uuid.UUID
	MediaXrefToID      map[string]uuid.UUID
	RepositoryXrefToID map[string]uuid.UUID
}

// PersonData contains parsed person data ready for creation.
type PersonData struct {
	ID            uuid.UUID
	GedcomXref    string
	GivenName     string
	Surname       string
	NamePrefix    string          // Dr., Rev., Sir (NPFX)
	NameSuffix    string          // Jr., III, PhD (NSFX)
	SurnamePrefix string          // von, de, van (SPFX)
	Nickname      string          // Informal name (NICK)
	NameType      domain.NameType // birth, married, aka (TYPE)
	Gender        domain.Gender
	BirthDate     string
	BirthPlace    string
	DeathDate     string
	DeathPlace    string
	Notes         string

	// FamilySearchID is the FamilySearch Family Tree ID from the _FSFTID tag.
	// This is a vendor extension from FamilySearch.org that uniquely identifies
	// an individual in their Family Tree database. Format: alphanumeric like "KWCJ-QN7".
	FamilySearchID string
}

// FamilyData contains parsed family data ready for creation.
type FamilyData struct {
	ID               uuid.UUID
	GedcomXref       string
	Partner1ID       *uuid.UUID
	Partner2ID       *uuid.UUID
	RelationshipType domain.RelationType
	MarriageDate     string
	MarriagePlace    string
	ChildIDs         []uuid.UUID
	ChildRelTypes    []domain.ChildRelationType
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
	FileRef     string // GEDCOM file reference (path or URL)
	Description string
}

// EventData contains parsed life event data ready for creation.
type EventData struct {
	ID          uuid.UUID
	OwnerType   string // "person" or "family"
	OwnerID     uuid.UUID
	FactType    domain.FactType
	Date        string
	Place       string
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

// Importer handles GEDCOM file parsing and conversion to domain events.
type Importer struct{}

// NewImporter creates a new GEDCOM importer.
func NewImporter() *Importer {
	return &Importer{}
}

// Import parses a GEDCOM file and returns structured data for import.
func (imp *Importer) Import(ctx context.Context, reader io.Reader) (*ImportResult, []PersonData, []FamilyData, []SourceData, []CitationData, []RepositoryData, []EventData, []AttributeData, error) {
	result := &ImportResult{
		PersonXrefToID:     make(map[string]uuid.UUID),
		FamilyXrefToID:     make(map[string]uuid.UUID),
		SourceXrefToID:     make(map[string]uuid.UUID),
		MediaXrefToID:      make(map[string]uuid.UUID),
		RepositoryXrefToID: make(map[string]uuid.UUID),
	}

	// Parse GEDCOM using cacack/gedcom-go decoder
	// The decoder handles encoding detection (UTF-8, ANSEL, Windows-1252, ISO-8859-1) automatically
	opts := decoder.DefaultOptions()
	opts.Context = ctx

	doc, err := decoder.DecodeWithOptions(reader, opts)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to parse GEDCOM: %w", err)
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

	result.PersonsImported = len(persons)
	result.FamiliesImported = len(families)
	result.SourcesImported = len(sources)
	result.CitationsImported = len(citations)
	result.RepositoriesImported = len(repositories)
	result.EventsImported = len(events)
	result.AttributesImported = len(attributes)

	return result, persons, families, sources, citations, repositories, events, attributes, nil
}

// parseIndividual converts a GEDCOM individual record to PersonData.
// The doc parameter provides access to other records for PEDI lookup.
func parseIndividual(indi *gedcom.Individual, doc *gedcom.Document, result *ImportResult) PersonData {
	person := PersonData{
		ID:         uuid.New(),
		GedcomXref: indi.XRef,
	}

	// Parse name with all components
	if len(indi.Names) > 0 {
		name := indi.Names[0]
		person.GivenName = strings.TrimSpace(name.Given)
		person.Surname = strings.TrimSpace(name.Surname)
		person.NamePrefix = strings.TrimSpace(name.Prefix)
		person.NameSuffix = strings.TrimSpace(name.Suffix)
		person.SurnamePrefix = strings.TrimSpace(name.SurnamePrefix)
		person.Nickname = strings.TrimSpace(name.Nickname)

		// Map name type
		switch strings.ToLower(name.Type) {
		case "birth":
			person.NameType = domain.NameTypeBirth
		case "married":
			person.NameType = domain.NameTypeMarried
		case "aka":
			person.NameType = domain.NameTypeAKA
		}

		// Given name is required
		if person.GivenName == "" {
			person.GivenName = "Unknown"
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Individual %s: missing given name, using 'Unknown'", indi.XRef))
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
func parseRepository(repo *gedcom.Repository, result *ImportResult) RepositoryData {
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

// extractCitationsFromIndividual extracts all citations from individual events.
func extractCitationsFromIndividual(indi *gedcom.Individual, personID uuid.UUID, result *ImportResult) []CitationData {
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
func extractCitationsFromFamily(fam *gedcom.Family, familyID uuid.UUID, result *ImportResult) []CitationData {
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
		events = append(events, eventData)
	}

	return events
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
		events = append(events, eventData)
	}

	return events
}
