package command

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
)

// ImportGedcomInput contains the data for importing a GEDCOM file.
type ImportGedcomInput struct {
	Filename string
	FileSize int64
	Reader   io.Reader
}

// ImportGedcomResult contains the result of a GEDCOM import.
type ImportGedcomResult struct {
	ImportID         uuid.UUID
	PersonsImported  int
	FamiliesImported int
	Warnings         []string
	Errors           []string
}

// ImportGedcom imports persons and families from a GEDCOM file.
func (h *Handler) ImportGedcom(ctx context.Context, input ImportGedcomInput) (*ImportGedcomResult, error) {
	importer := gedcom.NewImporter()

	// Parse the GEDCOM file
	importResult, persons, families, err := importer.Import(ctx, input.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GEDCOM file: %w", err)
	}

	// Validate the import data
	if err := gedcom.ValidateImportData(persons, families); err != nil {
		return nil, fmt.Errorf("invalid GEDCOM data: %w", err)
	}

	result := &ImportGedcomResult{
		ImportID: uuid.New(),
		Warnings: importResult.Warnings,
		Errors:   importResult.Errors,
	}

	// Import persons
	for _, p := range persons {
		err := h.importPerson(ctx, p)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("Failed to import person %s (%s %s): %v", p.GedcomXref, p.GivenName, p.Surname, err))
			continue
		}
		result.PersonsImported++
	}

	// Import families (after persons so we can link them)
	for _, f := range families {
		err := h.importFamily(ctx, f)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("Failed to import family %s: %v", f.GedcomXref, err))
			continue
		}
		result.FamiliesImported++

		// Link children to family
		for i, childID := range f.ChildIDs {
			relType := domain.ChildBiological
			if i < len(f.ChildRelTypes) {
				relType = f.ChildRelTypes[i]
			}
			err := h.linkChildToFamily(ctx, f.ID, childID, relType)
			if err != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Failed to link child to family %s: %v", f.GedcomXref, err))
			}
		}
	}

	// Record the import event
	importEvent := domain.NewGedcomImported(
		input.Filename,
		input.FileSize,
		result.PersonsImported,
		result.FamiliesImported,
		result.Warnings,
		result.Errors,
	)

	// Store import event (using a special "import" stream)
	_ = h.eventStore.Append(ctx, importEvent.ImportID, "import", []domain.Event{importEvent}, -1)

	return result, nil
}

// importPerson creates a person from GEDCOM data.
func (h *Handler) importPerson(ctx context.Context, p gedcom.PersonData) error {
	// Create person entity
	person := &domain.Person{
		ID:         p.ID,
		GivenName:  p.GivenName,
		Surname:    p.Surname,
		Gender:     p.Gender,
		BirthPlace: p.BirthPlace,
		DeathPlace: p.DeathPlace,
		Notes:      p.Notes,
		GedcomXref: p.GedcomXref,
		Version:    1,
	}

	if p.BirthDate != "" {
		bd := domain.ParseGenDate(p.BirthDate)
		person.BirthDate = &bd
	}
	if p.DeathDate != "" {
		dd := domain.ParseGenDate(p.DeathDate)
		person.DeathDate = &dd
	}

	// Create event
	event := domain.NewPersonCreated(person)

	// Append to event store
	err := h.eventStore.Append(ctx, person.ID, "person", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}

// importFamily creates a family from GEDCOM data.
func (h *Handler) importFamily(ctx context.Context, f gedcom.FamilyData) error {
	// Create family entity
	family := &domain.Family{
		ID:               f.ID,
		Partner1ID:       f.Partner1ID,
		Partner2ID:       f.Partner2ID,
		RelationshipType: f.RelationshipType,
		GedcomXref:       f.GedcomXref,
		Version:          1,
	}

	if f.MarriageDate != "" {
		md := domain.ParseGenDate(f.MarriageDate)
		family.MarriageDate = &md
	}
	family.MarriagePlace = f.MarriagePlace

	// Validate - allow families without partners (will be linked later or single-parent)
	if family.Partner1ID == nil && family.Partner2ID == nil {
		// Skip families with no partners
		return nil
	}

	// Create event
	event := domain.NewFamilyCreated(family)

	// Append to event store
	err := h.eventStore.Append(ctx, family.ID, "family", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}

// linkChildToFamily links a child to a family.
func (h *Handler) linkChildToFamily(ctx context.Context, familyID, childID uuid.UUID, relType domain.ChildRelationType) error {
	// Check if child is already linked
	existingFamily, err := h.readStore.GetChildFamily(ctx, childID)
	if err != nil {
		return err
	}
	if existingFamily != nil {
		// Child already in a family, skip
		return nil
	}

	// Create family child
	fc := domain.NewFamilyChild(familyID, childID, relType)
	event := domain.NewChildLinkedToFamily(fc)

	// Get current family version
	family, err := h.readStore.GetFamily(ctx, familyID)
	if err != nil {
		return err
	}
	if family == nil {
		return nil // Family doesn't exist, skip
	}

	// Append to event store
	err = h.eventStore.Append(ctx, familyID, "family", []domain.Event{event}, family.Version)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Apply(ctx, event)
}
