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
	ImportID              uuid.UUID
	PersonsImported       int
	FamiliesImported      int
	SourcesImported       int
	CitationsImported     int
	RepositoriesImported  int
	EventsImported        int
	AttributesImported    int
	NotesImported         int
	SubmittersImported    int
	AssociationsImported  int
	LDSOrdinancesImported int
	Warnings              []string
	Errors                []string
}

// ImportGedcom imports persons and families from a GEDCOM file.
func (h *Handler) ImportGedcom(ctx context.Context, input ImportGedcomInput) (*ImportGedcomResult, error) {
	importer := gedcom.NewImporter()

	// Parse the GEDCOM file
	importResult, persons, families, sources, citations, repositories, events, attributes, notes, submitters, associations, ldsOrdinances, _, err := importer.Import(ctx, input.Reader)
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

	// Import repositories first (before sources that reference them)
	for _, r := range repositories {
		err := h.importRepository(ctx, r)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("Failed to import repository %s (%s): %v", r.GedcomXref, r.Name, err))
			continue
		}
		result.RepositoriesImported++
	}

	// Import sources (after repositories so we can link them)
	for _, s := range sources {
		err := h.importSource(ctx, s)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("Failed to import source %s (%s): %v", s.GedcomXref, s.Title, err))
			continue
		}
		result.SourcesImported++
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

	// Import citations (after persons, families, and sources exist)
	// Build source lookup map from XRef to ID
	sourceXrefToID := make(map[string]uuid.UUID)
	for _, s := range sources {
		sourceXrefToID[s.GedcomXref] = s.ID
	}

	for _, c := range citations {
		// Resolve source XRef to ID
		sourceID, ok := sourceXrefToID[c.SourceXref]
		if !ok {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Citation references unknown source %s", c.SourceXref))
			continue
		}

		err := h.importCitation(ctx, c, sourceID)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to import citation: %v", err))
			continue
		}
		result.CitationsImported++
	}

	// Import events (after persons and families exist)
	for _, e := range events {
		err := h.importEvent(ctx, e)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to import event (%s): %v", e.FactType, err))
			continue
		}
		result.EventsImported++
	}

	// Import attributes (after persons exist)
	for _, a := range attributes {
		err := h.importAttribute(ctx, a)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to import attribute (%s): %v", a.FactType, err))
			continue
		}
		result.AttributesImported++
	}

	// Import shared notes
	for _, n := range notes {
		err := h.importNote(ctx, n)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to import note (%s): %v", n.GedcomXref, err))
			continue
		}
		result.NotesImported++
	}

	// Import submitters
	for _, s := range submitters {
		err := h.importSubmitter(ctx, s)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to import submitter (%s): %v", s.GedcomXref, err))
			continue
		}
		result.SubmittersImported++
	}

	// Import associations (after persons exist, since they reference PersonID and AssociateID)
	for _, a := range associations {
		err := h.importAssociation(ctx, a)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to import association (%s -> %s, %s): %v",
					a.PersonID.String(), a.AssociateID.String(), a.Role, err))
			continue
		}
		result.AssociationsImported++
	}

	// Import LDS ordinances (after persons and families exist)
	for _, o := range ldsOrdinances {
		err := h.importLDSOrdinance(ctx, o)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to import LDS ordinance (%s): %v", o.Type, err))
			continue
		}
		result.LDSOrdinancesImported++
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
	// Create person entity with all name components
	person := &domain.Person{
		ID:            p.ID,
		GivenName:     p.GivenName,
		Surname:       p.Surname,
		NamePrefix:    p.NamePrefix,
		NameSuffix:    p.NameSuffix,
		SurnamePrefix: p.SurnamePrefix,
		Nickname:      p.Nickname,
		NameType:      p.NameType,
		Gender:        p.Gender,
		BirthPlace:    p.BirthPlace,
		DeathPlace:    p.DeathPlace,
		Notes:         p.Notes,
		GedcomXref:    p.GedcomXref,
		Version:       1,
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
	if err := h.projector.Project(ctx, event, 1); err != nil {
		return err
	}

	// Update read model with GEDCOM coordinates (not in event schema)
	if p.BirthPlaceLat != nil || p.BirthPlaceLong != nil || p.DeathPlaceLat != nil || p.DeathPlaceLong != nil {
		readModel, err := h.readStore.GetPerson(ctx, person.ID)
		if err != nil {
			return fmt.Errorf("failed to get person read model for coordinate update: %w", err)
		}
		if readModel != nil {
			readModel.BirthPlaceLat = p.BirthPlaceLat
			readModel.BirthPlaceLong = p.BirthPlaceLong
			readModel.DeathPlaceLat = p.DeathPlaceLat
			readModel.DeathPlaceLong = p.DeathPlaceLong
			if err := h.readStore.SavePerson(ctx, readModel); err != nil {
				return fmt.Errorf("failed to save person coordinates: %w", err)
			}
		}
	}

	// Emit NameAdded events for all names from GEDCOM
	// Track version to keep read model in sync with event stream
	currentVersion := int64(1) // PersonCreated was version 1
	for _, nameData := range p.Names {
		personName := domain.NewPersonName(person.ID, nameData.GivenName, nameData.Surname)
		personName.NamePrefix = nameData.NamePrefix
		personName.NameSuffix = nameData.NameSuffix
		personName.SurnamePrefix = nameData.SurnamePrefix
		personName.Nickname = nameData.Nickname
		personName.NameType = nameData.NameType
		personName.IsPrimary = nameData.IsPrimary

		nameEvent := domain.NewNameAdded(personName)

		// Append name event to person stream with version tracking
		err := h.eventStore.Append(ctx, person.ID, "person", []domain.Event{nameEvent}, currentVersion)
		if err != nil {
			return err
		}
		currentVersion++

		// Project to read model with correct version
		if err := h.projector.Project(ctx, nameEvent, currentVersion); err != nil {
			return err
		}
	}

	return nil
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

// importSource creates a source from GEDCOM data.
func (h *Handler) importSource(ctx context.Context, s gedcom.SourceData) error {
	// Parse source type - default to "other" if not specified or invalid
	sourceType := domain.SourceType(s.SourceType)
	if !sourceType.IsValid() {
		sourceType = domain.SourceOther
	}

	// Create source entity
	source := &domain.Source{
		ID:             s.ID,
		SourceType:     sourceType,
		Title:          s.Title,
		Author:         s.Author,
		Publisher:      s.Publisher,
		RepositoryID:   s.RepositoryID,
		RepositoryName: s.RepositoryName,
		CallNumber:     s.CallNumber,
		Notes:          s.Notes,
		GedcomXref:     s.GedcomXref,
		Version:        1,
	}

	// Parse publish date if provided
	if s.PublishDate != "" {
		pd := domain.ParseGenDate(s.PublishDate)
		source.PublishDate = &pd
	}

	// Create event
	event := domain.NewSourceCreated(source)

	// Append to event store
	err := h.eventStore.Append(ctx, source.ID, "source", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}

// importRepository creates a repository from GEDCOM data.
func (h *Handler) importRepository(ctx context.Context, r gedcom.RepositoryData) error {
	// Create repository entity
	repo := &domain.Repository{
		ID:            r.ID,
		Name:          r.Name,
		StreetAddress: r.Address,
		City:          r.City,
		State:         r.State,
		PostalCode:    r.PostalCode,
		Country:       r.Country,
		Phone:         r.Phone,
		Email:         r.Email,
		Website:       r.Website,
		Notes:         r.Notes,
		GedcomXref:    r.GedcomXref,
		Version:       1,
	}

	// Create event
	event := domain.NewRepositoryCreated(repo)

	// Append to event store
	err := h.eventStore.Append(ctx, repo.ID, "repository", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}

// importCitation creates a citation from GEDCOM data.
func (h *Handler) importCitation(ctx context.Context, c gedcom.CitationData, sourceID uuid.UUID) error {
	// Parse fact type
	factType := domain.FactType(c.FactType)
	if !factType.IsValid() {
		return fmt.Errorf("invalid fact type: %s", c.FactType)
	}

	// Map quality string to GPS terms
	// The quality string from GEDCOM importer is already in GPS format:
	// "direct", "indirect", "secondary", "negative"
	var evidenceType domain.EvidenceType
	var informantType domain.InformantType
	switch c.Quality {
	case "direct":
		evidenceType = domain.EvidenceDirect
		informantType = domain.InformantPrimary
	case "secondary":
		informantType = domain.InformantSecondary
	case "indirect":
		evidenceType = domain.EvidenceIndirect
	case "negative":
		evidenceType = domain.EvidenceNegative
	}

	// Create citation entity
	citation := &domain.Citation{
		ID:            c.ID,
		SourceID:      sourceID,
		FactType:      factType,
		FactOwnerID:   c.FactOwnerID,
		Page:          c.Page,
		InformantType: informantType,
		EvidenceType:  evidenceType,
		QuotedText:    c.QuotedText,
		GedcomXref:    c.GedcomXref,
		Version:       1,
	}

	// Create event
	event := domain.NewCitationCreated(citation)

	// Append to event store
	err := h.eventStore.Append(ctx, citation.ID, "citation", []domain.Event{event}, -1)
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

// importEvent creates a life event from GEDCOM data.
func (h *Handler) importEvent(ctx context.Context, e gedcom.EventData) error {
	// Create domain model
	var lifeEvent *domain.LifeEvent
	if e.OwnerType == "person" {
		lifeEvent = domain.NewLifeEvent(e.OwnerID, e.FactType)
	} else {
		lifeEvent = domain.NewFamilyLifeEvent(e.OwnerID, e.FactType)
	}

	// Override ID to preserve GEDCOM-assigned ID
	lifeEvent.ID = e.ID
	lifeEvent.Place = e.Place
	lifeEvent.Address = e.Address
	lifeEvent.Description = e.Description
	lifeEvent.Cause = e.Cause
	lifeEvent.Age = e.Age
	lifeEvent.IsNegated = e.IsNegated

	// Set date if provided
	if e.Date != "" {
		lifeEvent.SetDate(e.Date)
	}

	// Create domain event from model
	event := domain.NewLifeEventCreatedFromModel(lifeEvent)

	// Append to event store using owner's stream
	err := h.eventStore.Append(ctx, e.ID, "event", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	if err := h.projector.Project(ctx, event, 1); err != nil {
		return err
	}

	// Update read model with GEDCOM coordinates (not in event schema)
	if e.PlaceLat != nil || e.PlaceLong != nil {
		readModel, err := h.readStore.GetEvent(ctx, e.ID)
		if err != nil {
			return fmt.Errorf("failed to get event read model for coordinate update: %w", err)
		}
		if readModel != nil {
			readModel.PlaceLat = e.PlaceLat
			readModel.PlaceLong = e.PlaceLong
			if err := h.readStore.SaveEvent(ctx, readModel); err != nil {
				return fmt.Errorf("failed to save event coordinates: %w", err)
			}
		}
	}

	return nil
}

// importAttribute creates a person attribute from GEDCOM data.
func (h *Handler) importAttribute(ctx context.Context, a gedcom.AttributeData) error {
	// Create domain model
	attr := domain.NewAttribute(a.PersonID, a.FactType, a.Value)

	// Override ID to preserve GEDCOM-assigned ID
	attr.ID = a.ID
	attr.Place = a.Place

	// Set date if provided
	if a.Date != "" {
		attr.SetDate(a.Date)
	}

	// Create domain event from model
	event := domain.NewAttributeCreatedFromModel(attr)

	// Append to event store
	err := h.eventStore.Append(ctx, a.ID, "attribute", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}

// importNote creates a shared note from GEDCOM data.
func (h *Handler) importNote(ctx context.Context, n gedcom.NoteData) error {
	// Create note entity
	note := domain.NewNoteWithID(n.ID, n.Text)
	note.SetGedcomXref(n.GedcomXref)

	// Create event
	event := domain.NewNoteCreated(note)

	// Append to event store
	err := h.eventStore.Append(ctx, note.ID, "note", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}

// importSubmitter creates a submitter from GEDCOM data.
func (h *Handler) importSubmitter(ctx context.Context, s gedcom.SubmitterData) error {
	// Create submitter entity
	submitter := domain.NewSubmitterWithID(s.ID, s.Name)
	submitter.SetGedcomXref(s.GedcomXref)

	if s.Address != nil {
		submitter.SetAddress(s.Address)
	}
	for _, phone := range s.Phone {
		submitter.AddPhone(phone)
	}
	for _, email := range s.Email {
		submitter.AddEmail(email)
	}
	if s.Language != "" {
		submitter.SetLanguage(s.Language)
	}

	// Create event
	event := domain.NewSubmitterCreated(submitter)

	// Append to event store
	err := h.eventStore.Append(ctx, submitter.ID, "submitter", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}

// importAssociation creates an association from GEDCOM data.
func (h *Handler) importAssociation(ctx context.Context, a gedcom.AssociationData) error {
	// Create association entity
	association := domain.NewAssociationWithID(a.ID, a.PersonID, a.AssociateID, a.Role)

	if a.Phrase != "" {
		association.SetPhrase(a.Phrase)
	}
	if a.Notes != "" {
		association.SetNotes(a.Notes)
	}

	// Create event
	event := domain.NewAssociationCreated(association)

	// Append to event store
	err := h.eventStore.Append(ctx, association.ID, "association", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}

// importLDSOrdinance creates an LDS ordinance from GEDCOM data.
func (h *Handler) importLDSOrdinance(ctx context.Context, o gedcom.LDSOrdinanceData) error {
	// Create ordinance entity
	ordinance := domain.NewLDSOrdinanceWithID(o.ID, o.Type)

	if o.PersonID != nil {
		ordinance.SetPersonID(*o.PersonID)
	}
	if o.FamilyID != nil {
		ordinance.SetFamilyID(*o.FamilyID)
	}
	if o.Date != "" {
		ordinance.SetDate(o.Date)
	}
	if o.Place != "" {
		ordinance.SetPlace(o.Place)
	}
	if o.Temple != "" {
		ordinance.SetTemple(o.Temple)
	}
	if o.Status != "" {
		ordinance.SetStatus(o.Status)
	}

	// Create event
	event := domain.NewLDSOrdinanceCreated(ordinance)

	// Append to event store
	err := h.eventStore.Append(ctx, ordinance.ID, "LDSOrdinance", []domain.Event{event}, -1)
	if err != nil {
		return err
	}

	// Project to read model
	return h.projector.Project(ctx, event, 1)
}
