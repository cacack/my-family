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
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// ImportResult contains the results of a GEDCOM import operation.
type ImportResult struct {
	PersonsImported   int
	FamiliesImported  int
	SourcesImported   int
	CitationsImported int
	MediaImported     int
	Warnings          []string
	Errors            []string

	// Mappings from GEDCOM XREFs to internal UUIDs
	PersonXrefToID map[string]uuid.UUID
	FamilyXrefToID map[string]uuid.UUID
	SourceXrefToID map[string]uuid.UUID
	MediaXrefToID  map[string]uuid.UUID
}

// PersonData contains parsed person data ready for creation.
type PersonData struct {
	ID         uuid.UUID
	GedcomXref string
	GivenName  string
	Surname    string
	Gender     domain.Gender
	BirthDate  string
	BirthPlace string
	DeathDate  string
	DeathPlace string
	Notes      string
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
	RepositoryName string
	Notes          string
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

// Importer handles GEDCOM file parsing and conversion to domain events.
type Importer struct{}

// NewImporter creates a new GEDCOM importer.
func NewImporter() *Importer {
	return &Importer{}
}

// Import parses a GEDCOM file and returns structured data for import.
func (imp *Importer) Import(ctx context.Context, reader io.Reader) (*ImportResult, []PersonData, []FamilyData, []SourceData, []CitationData, error) {
	result := &ImportResult{
		PersonXrefToID: make(map[string]uuid.UUID),
		FamilyXrefToID: make(map[string]uuid.UUID),
		SourceXrefToID: make(map[string]uuid.UUID),
		MediaXrefToID:  make(map[string]uuid.UUID),
	}

	// Parse GEDCOM using cacack/gedcom-go decoder
	// The decoder handles encoding detection (UTF-8, ANSEL, Windows-1252, ISO-8859-1) automatically
	opts := decoder.DefaultOptions()
	opts.Context = ctx

	doc, err := decoder.DecodeWithOptions(reader, opts)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to parse GEDCOM: %w", err)
	}

	// First pass: create source mappings
	var sources []SourceData
	for _, src := range doc.Sources() {
		source := parseSource(src, result)
		sources = append(sources, source)
		result.SourceXrefToID[src.XRef] = source.ID
	}

	// Second pass: create person mappings
	var persons []PersonData
	var citations []CitationData
	for _, indi := range doc.Individuals() {
		person := parseIndividual(indi, result)
		persons = append(persons, person)
		result.PersonXrefToID[indi.XRef] = person.ID

		// Extract citations from person events
		personCitations := extractCitationsFromIndividual(indi, person.ID, result)
		citations = append(citations, personCitations...)
	}

	// Third pass: create family mappings and resolve person references
	var families []FamilyData
	for _, fam := range doc.Families() {
		family := parseFamily(fam, result)
		families = append(families, family)
		result.FamilyXrefToID[fam.XRef] = family.ID

		// Extract citations from family events
		familyCitations := extractCitationsFromFamily(fam, family.ID, result)
		citations = append(citations, familyCitations...)
	}

	result.PersonsImported = len(persons)
	result.FamiliesImported = len(families)
	result.SourcesImported = len(sources)
	result.CitationsImported = len(citations)

	return result, persons, families, sources, citations, nil
}

// parseIndividual converts a GEDCOM individual record to PersonData.
func parseIndividual(indi *gedcom.Individual, result *ImportResult) PersonData {
	person := PersonData{
		ID:         uuid.New(),
		GedcomXref: indi.XRef,
	}

	// Parse name
	if len(indi.Names) > 0 {
		name := indi.Names[0]
		person.GivenName = strings.TrimSpace(name.Given)
		person.Surname = strings.TrimSpace(name.Surname)

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

	// Parse events for birth and death
	for _, event := range indi.Events {
		switch event.Type {
		case gedcom.EventBirth:
			person.BirthDate = event.Date
			person.BirthPlace = event.Place
		case gedcom.EventDeath:
			person.DeathDate = event.Date
			person.DeathPlace = event.Place
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

	return person
}

// parseFamily converts a GEDCOM family record to FamilyData.
func parseFamily(fam *gedcom.Family, result *ImportResult) FamilyData {
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

	// Parse events for marriage
	for _, event := range fam.Events {
		switch event.Type {
		case gedcom.EventMarriage:
			family.RelationshipType = domain.RelationMarriage
			family.MarriageDate = event.Date
			family.MarriagePlace = event.Place
		case gedcom.EventDivorce:
			// Divorce event - we note it but keep as marriage type
		}
	}

	// Link children - in cacack/gedcom-go, Children is []string of XRefs
	for _, childXRef := range fam.Children {
		if id, ok := result.PersonXrefToID[childXRef]; ok {
			family.ChildIDs = append(family.ChildIDs, id)
			// Default to biological, could be refined by checking ADOP events
			family.ChildRelTypes = append(family.ChildRelTypes, domain.ChildBiological)
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Family %s: child %s not found", fam.XRef, childXRef))
		}
	}

	return family
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

	// Extract repository name if available
	if src.RepositoryRef != "" {
		// This is just a reference - we could look it up in the document if needed
		// For now, store the XRef as repository name
		source.RepositoryName = src.RepositoryRef
	}

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
				FactOwnerID: personID,
				Page:        srcCit.Page,
				Quality:     mapGedcomQualityToGPS(srcCit.Quality),
			}

			// Extract quoted text if available
			if srcCit.Data != nil && srcCit.Data.Text != "" {
				citation.QuotedText = srcCit.Data.Text
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
