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
	PersonsImported  int
	FamiliesImported int
	Warnings         []string
	Errors           []string

	// Mappings from GEDCOM XREFs to internal UUIDs
	PersonXrefToID map[string]uuid.UUID
	FamilyXrefToID map[string]uuid.UUID
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

// Importer handles GEDCOM file parsing and conversion to domain events.
type Importer struct{}

// NewImporter creates a new GEDCOM importer.
func NewImporter() *Importer {
	return &Importer{}
}

// Import parses a GEDCOM file and returns structured data for import.
func (imp *Importer) Import(ctx context.Context, reader io.Reader) (*ImportResult, []PersonData, []FamilyData, error) {
	result := &ImportResult{
		PersonXrefToID: make(map[string]uuid.UUID),
		FamilyXrefToID: make(map[string]uuid.UUID),
	}

	// Parse GEDCOM using cacack/gedcom-go decoder
	// The decoder handles encoding detection (UTF-8, ANSEL, Windows-1252, ISO-8859-1) automatically
	opts := decoder.DefaultOptions()
	opts.Context = ctx

	doc, err := decoder.DecodeWithOptions(reader, opts)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse GEDCOM: %w", err)
	}

	// First pass: create person mappings
	var persons []PersonData
	for _, indi := range doc.Individuals() {
		person := parseIndividual(indi, result)
		persons = append(persons, person)
		result.PersonXrefToID[indi.XRef] = person.ID
	}

	// Second pass: create family mappings and resolve person references
	var families []FamilyData
	for _, fam := range doc.Families() {
		family := parseFamily(fam, result)
		families = append(families, family)
		result.FamilyXrefToID[fam.XRef] = family.ID
	}

	result.PersonsImported = len(persons)
	result.FamiliesImported = len(families)

	return result, persons, families, nil
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

		// Handle empty names
		if person.GivenName == "" {
			person.GivenName = "Unknown"
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Individual %s: missing given name, using 'Unknown'", indi.XRef))
		}
		if person.Surname == "" {
			person.Surname = "Unknown"
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Individual %s: missing surname, using 'Unknown'", indi.XRef))
		}
	} else {
		person.GivenName = "Unknown"
		person.Surname = "Unknown"
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Individual %s: no name record, using 'Unknown Unknown'", indi.XRef))
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

// ValidateImportData checks for issues that would prevent import.
func ValidateImportData(persons []PersonData, families []FamilyData) error {
	if len(persons) == 0 && len(families) == 0 {
		return errors.New("GEDCOM file contains no individuals or families")
	}
	return nil
}
