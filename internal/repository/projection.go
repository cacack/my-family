package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// Projector handles event-to-read-model projections.
type Projector struct {
	readStore   ReadModelStore
	branchStore BranchStore
}

// NewProjector creates a new projector with the given read model store and
// branch registry store. branchStore may be nil: in that case the three
// branch-lifecycle handlers no-op (slice routing never needs branchStore). The
// production construction sites (api/server.go, cmd/myfamily/main.go) supply a
// real BranchStore; test/command callers that never emit branch events pass nil.
func NewProjector(readStore ReadModelStore, branchStore BranchStore) *Projector {
	return &Projector{readStore: readStore, branchStore: branchStore}
}

// Apply is a convenience method for applying a single event (version is auto-incremented).
func (p *Projector) Apply(ctx context.Context, event domain.Event) error {
	return p.Project(ctx, event, 1, domain.MainBranchID) // Version will be updated properly by the caller
}

// Project applies a domain event to the read model on the given branch. A zero
// branchID (domain.MainBranchID) reproduces pre-branch, main-only behavior.
// Only the slice entities (Person, PersonName, Person EXID, Family, Family EXID,
// FamilyChild, PedigreeEdge) are branch-scoped; all other handlers ignore
// branchID and write main-only.
func (p *Projector) Project(ctx context.Context, event domain.Event, version int64, branchID domain.BranchID) error {
	switch e := event.(type) {
	case domain.PersonCreated:
		return p.projectPersonCreated(ctx, e, version, branchID)
	case domain.PersonUpdated:
		return p.projectPersonUpdated(ctx, e, version, branchID)
	case domain.PersonDeleted:
		return p.projectPersonDeleted(ctx, e, branchID)
	case domain.FamilyCreated:
		return p.projectFamilyCreated(ctx, e, version, branchID)
	case domain.FamilyUpdated:
		return p.projectFamilyUpdated(ctx, e, version, branchID)
	case domain.ChildLinkedToFamily:
		return p.projectChildLinked(ctx, e, branchID)
	case domain.ChildUnlinkedFromFamily:
		return p.projectChildUnlinked(ctx, e, branchID)
	case domain.FamilyDeleted:
		return p.projectFamilyDeleted(ctx, e, branchID)
	case domain.SourceCreated:
		return p.projectSourceCreated(ctx, e, version)
	case domain.SourceUpdated:
		return p.projectSourceUpdated(ctx, e, version)
	case domain.SourceDeleted:
		return p.projectSourceDeleted(ctx, e)
	case domain.CitationCreated:
		return p.projectCitationCreated(ctx, e, version)
	case domain.CitationUpdated:
		return p.projectCitationUpdated(ctx, e, version)
	case domain.CitationDeleted:
		return p.projectCitationDeleted(ctx, e)
	case domain.MediaCreated:
		return p.projectMediaCreated(ctx, e, version)
	case domain.MediaUpdated:
		return p.projectMediaUpdated(ctx, e, version)
	case domain.MediaDeleted:
		return p.projectMediaDeleted(ctx, e)
	case domain.LifeEventCreated:
		return p.projectLifeEventCreated(ctx, e, version)
	case domain.LifeEventUpdated:
		return p.projectLifeEventUpdated(ctx, e, version)
	case domain.LifeEventDeleted:
		return p.projectLifeEventDeleted(ctx, e)
	case domain.AttributeCreated:
		return p.projectAttributeCreated(ctx, e, version)
	case domain.AttributeUpdated:
		return p.projectAttributeUpdated(ctx, e, version)
	case domain.AttributeDeleted:
		return p.projectAttributeDeleted(ctx, e)
	case domain.RepositoryCreated:
		return p.projectRepositoryCreated(ctx, e, version)
	case domain.RepositoryUpdated:
		return p.projectRepositoryUpdated(ctx, e, version)
	case domain.RepositoryDeleted:
		return p.projectRepositoryDeleted(ctx, e)
	case domain.NameAdded:
		return p.projectNameAdded(ctx, e, version, branchID)
	case domain.NameUpdated:
		return p.projectNameUpdated(ctx, e, version, branchID)
	case domain.NameRemoved:
		return p.projectNameRemoved(ctx, e, version, branchID)
	case domain.PersonMerged:
		return p.projectPersonMerged(ctx, e, version, branchID)
	case domain.NoteCreated:
		return p.projectNoteCreated(ctx, e, version)
	case domain.NoteUpdated:
		return p.projectNoteUpdated(ctx, e, version)
	case domain.NoteDeleted:
		return p.projectNoteDeleted(ctx, e)
	case domain.SubmitterCreated:
		return p.projectSubmitterCreated(ctx, e, version)
	case domain.SubmitterUpdated:
		return p.projectSubmitterUpdated(ctx, e, version)
	case domain.SubmitterDeleted:
		return p.projectSubmitterDeleted(ctx, e)
	case domain.AssociationCreated:
		return p.projectAssociationCreated(ctx, e, version, branchID)
	case domain.AssociationUpdated:
		return p.projectAssociationUpdated(ctx, e, version)
	case domain.AssociationDeleted:
		return p.projectAssociationDeleted(ctx, e)
	case domain.LDSOrdinanceCreated:
		return p.projectLDSOrdinanceCreated(ctx, e, version, branchID)
	case domain.LDSOrdinanceUpdated:
		return p.projectLDSOrdinanceUpdated(ctx, e, version)
	case domain.LDSOrdinanceDeleted:
		return p.projectLDSOrdinanceDeleted(ctx, e)
	case domain.EvidenceAnalysisCreated:
		return p.projectEvidenceAnalysisCreated(ctx, e, version)
	case domain.EvidenceAnalysisUpdated:
		return p.projectEvidenceAnalysisUpdated(ctx, e, version)
	case domain.EvidenceAnalysisDeleted:
		return p.projectEvidenceAnalysisDeleted(ctx, e)
	case domain.EvidenceConflictDetected:
		return p.projectEvidenceConflictDetected(ctx, e, version)
	case domain.EvidenceConflictResolved:
		return p.projectEvidenceConflictResolved(ctx, e, version)
	case domain.ResearchLogCreated:
		return p.projectResearchLogCreated(ctx, e, version)
	case domain.ResearchLogUpdated:
		return p.projectResearchLogUpdated(ctx, e, version)
	case domain.ResearchLogDeleted:
		return p.projectResearchLogDeleted(ctx, e)
	case domain.ProofSummaryCreated:
		return p.projectProofSummaryCreated(ctx, e, version)
	case domain.ProofSummaryUpdated:
		return p.projectProofSummaryUpdated(ctx, e, version)
	case domain.ProofSummaryDeleted:
		return p.projectProofSummaryDeleted(ctx, e)
	case domain.BranchCreated:
		return p.projectBranchCreated(ctx, e)
	case domain.BranchDeleted:
		return p.projectBranchDeleted(ctx, e)
	case domain.BranchMerged:
		return p.projectBranchMerged(ctx, e)
	default:
		// Unknown event types are ignored (forward compatibility)
		return nil
	}
}

// projectBranchCreated upserts the branch registry row from the event. The
// registry is event-sourced (ADR-005): the projector derives the Branch from the
// BranchCreated event and writes it via BranchStore.Upsert rather than mirroring
// a direct store write, so a projection rebuild reconstructs the registry.
func (p *Projector) projectBranchCreated(ctx context.Context, e domain.BranchCreated) error {
	if p.branchStore == nil {
		slog.Warn("projection: dropping branch lifecycle event, no BranchStore wired",
			"event", "BranchCreated", "branch_id", e.BranchID)
		return nil
	}
	branch := &domain.Branch{
		ID:           e.BranchID,
		Name:         e.Name,
		Description:  e.Description,
		BasePosition: e.BasePosition,
		Status:       domain.BranchStatusActive,
		CreatedAt:    e.OccurredAt(),
	}
	return p.branchStore.Upsert(ctx, branch)
}

// projectBranchDeleted archives the branch in the registry and drops the
// branch's copy-on-write overlay rows from the read model (ADR-005): first the
// replay-safe registry status change, then PurgeBranch to hard-delete the
// branch's rows across the seven slice tables. PurgeBranch is a no-op for the
// mainline, so an archived main (which cannot occur) would never be purged.
func (p *Projector) projectBranchDeleted(ctx context.Context, e domain.BranchDeleted) error {
	if p.branchStore == nil {
		slog.Warn("projection: dropping branch lifecycle event, no BranchStore wired",
			"event", "BranchDeleted", "branch_id", e.BranchID)
		return nil
	}
	if err := p.branchStore.UpdateStatus(ctx, e.BranchID, domain.BranchStatusArchived); err != nil {
		return err
	}
	return p.readStore.PurgeBranch(ctx, domain.BranchID(e.BranchID))
}

// projectBranchMerged marks the branch merged in the registry (terminal state).
func (p *Projector) projectBranchMerged(ctx context.Context, e domain.BranchMerged) error {
	if p.branchStore == nil {
		slog.Warn("projection: dropping branch lifecycle event, no BranchStore wired",
			"event", "BranchMerged", "branch_id", e.BranchID)
		return nil
	}
	return p.branchStore.UpdateStatus(ctx, e.BranchID, domain.BranchStatusMerged)
}

func (p *Projector) projectPersonCreated(ctx context.Context, e domain.PersonCreated, version int64, branchID domain.BranchID) error {
	var birthDateSort, deathDateSort *time.Time
	var birthDateRaw, deathDateRaw string

	if e.BirthDate != nil {
		birthDateRaw = e.BirthDate.Raw
		t := e.BirthDate.ToTime()
		if !t.IsZero() {
			birthDateSort = &t
		}
	}
	if e.DeathDate != nil {
		deathDateRaw = e.DeathDate.Raw
		t := e.DeathDate.ToTime()
		if !t.IsZero() {
			deathDateSort = &t
		}
	}

	person := &PersonReadModel{
		ID:             e.PersonID,
		GivenName:      e.GivenName,
		Surname:        e.Surname,
		FullName:       e.GivenName + " " + e.Surname,
		Gender:         e.Gender,
		BirthDateRaw:   birthDateRaw,
		BirthDateSort:  birthDateSort,
		BirthPlace:     e.BirthPlace,
		DeathDateRaw:   deathDateRaw,
		DeathDateSort:  deathDateSort,
		DeathPlace:     e.DeathPlace,
		Notes:          e.Notes,
		ResearchStatus: e.ResearchStatus,
		Version:        version,
		UpdatedAt:      e.OccurredAt(),
	}

	return p.readStore.SavePerson(ctx, branchID, person)
}

func (p *Projector) projectPersonUpdated(ctx context.Context, e domain.PersonUpdated, version int64, branchID domain.BranchID) error {
	person, err := p.readStore.GetPerson(ctx, branchID, e.PersonID)
	if err != nil {
		return err
	}
	if person == nil {
		return nil // Person doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "given_name":
			if v, ok := value.(string); ok {
				person.GivenName = v
				person.FullName = v + " " + person.Surname
			}
		case "surname":
			if v, ok := value.(string); ok {
				person.Surname = v
				person.FullName = person.GivenName + " " + v
			}
		case "gender":
			if v, ok := value.(string); ok {
				person.Gender = domain.Gender(v)
			}
		case "birth_date":
			if v, ok := value.(string); ok {
				person.BirthDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					person.BirthDateSort = &t
				} else {
					person.BirthDateSort = nil
				}
			}
		case "birth_place":
			if v, ok := value.(string); ok {
				person.BirthPlace = v
			}
		case "death_date":
			if v, ok := value.(string); ok {
				person.DeathDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					person.DeathDateSort = &t
				} else {
					person.DeathDateSort = nil
				}
			}
		case "death_place":
			if v, ok := value.(string); ok {
				person.DeathPlace = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				person.Notes = v
			}
		case "research_status":
			if v, ok := value.(string); ok {
				person.ResearchStatus = domain.ParseResearchStatus(v)
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "PersonUpdated", "key", key)
		}
	}

	person.Version = version
	person.UpdatedAt = e.OccurredAt()

	return p.readStore.SavePerson(ctx, branchID, person)
}

func (p *Projector) projectPersonDeleted(ctx context.Context, e domain.PersonDeleted, branchID domain.BranchID) error {
	return p.readStore.DeletePerson(ctx, branchID, e.PersonID)
}

func (p *Projector) projectFamilyCreated(ctx context.Context, e domain.FamilyCreated, version int64, branchID domain.BranchID) error {
	var marriageDateSort *time.Time
	var marriageDateRaw string

	if e.MarriageDate != nil {
		marriageDateRaw = e.MarriageDate.Raw
		t := e.MarriageDate.ToTime()
		if !t.IsZero() {
			marriageDateSort = &t
		}
	}

	// Get partner names if available (split into given/surname for API contract)
	var partner1GivenName, partner1Surname, partner2GivenName, partner2Surname string
	if e.Partner1ID != nil {
		if p1, _ := p.readStore.GetPerson(ctx, branchID, *e.Partner1ID); p1 != nil {
			partner1GivenName = p1.GivenName
			partner1Surname = p1.Surname
		}
	}
	if e.Partner2ID != nil {
		if p2, _ := p.readStore.GetPerson(ctx, branchID, *e.Partner2ID); p2 != nil {
			partner2GivenName = p2.GivenName
			partner2Surname = p2.Surname
		}
	}

	family := &FamilyReadModel{
		ID:                e.FamilyID,
		Partner1ID:        e.Partner1ID,
		Partner1GivenName: partner1GivenName,
		Partner1Surname:   partner1Surname,
		Partner2ID:        e.Partner2ID,
		Partner2GivenName: partner2GivenName,
		Partner2Surname:   partner2Surname,
		RelationshipType:  e.RelationshipType,
		MarriageDateRaw:   marriageDateRaw,
		MarriageDateSort:  marriageDateSort,
		MarriagePlace:     e.MarriagePlace,
		ChildCount:        0,
		Version:           version,
		UpdatedAt:         e.OccurredAt(),
	}

	return p.readStore.SaveFamily(ctx, branchID, family)
}

func (p *Projector) projectFamilyUpdated(ctx context.Context, e domain.FamilyUpdated, version int64, branchID domain.BranchID) error {
	family, err := p.readStore.GetFamily(ctx, branchID, e.FamilyID)
	if err != nil {
		return err
	}
	if family == nil {
		return nil // Family doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "partner1_id":
			newID, given, surname := p.resolvePartnerChange(ctx, branchID, value)
			family.Partner1ID = newID
			family.Partner1GivenName = given
			family.Partner1Surname = surname
		case "partner2_id":
			newID, given, surname := p.resolvePartnerChange(ctx, branchID, value)
			family.Partner2ID = newID
			family.Partner2GivenName = given
			family.Partner2Surname = surname
		case "relationship_type":
			if v, ok := value.(string); ok {
				family.RelationshipType = domain.RelationType(v)
			}
		case "marriage_date":
			if v, ok := value.(string); ok {
				family.MarriageDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					family.MarriageDateSort = &t
				} else {
					family.MarriageDateSort = nil
				}
			}
		case "marriage_place":
			if v, ok := value.(string); ok {
				family.MarriagePlace = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "FamilyUpdated", "key", key)
		}
	}

	family.Version = version
	family.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveFamily(ctx, branchID, family)
}

// resolvePartnerChange resolves a partner_id value from a FamilyUpdated.Changes
// map into the new partner ID and split name fields. Values arrive as untyped
// JSON: a string UUID for a swap, nil/empty to clear. Unknown person IDs leave
// the names empty rather than failing — the name will be backfilled the next
// time the projection sees the person.
// parseOptionalUUID coerces a change-map value into an optional UUID. Values may
// arrive as a string (the JSON-decoded form after event replay), a uuid.UUID, or a
// *uuid.UUID (freshly built in-memory). A nil, empty, or unparseable value yields
// nil, which clears the link.
func parseOptionalUUID(value any) *uuid.UUID {
	switch v := value.(type) {
	case nil:
		return nil
	case *uuid.UUID:
		return v
	case uuid.UUID:
		id := v
		return &id
	case string:
		if v == "" {
			return nil
		}
		id, err := uuid.Parse(v)
		if err != nil {
			return nil
		}
		return &id
	default:
		return nil
	}
}

func (p *Projector) resolvePartnerChange(ctx context.Context, branchID domain.BranchID, value any) (*uuid.UUID, string, string) {
	s, ok := value.(string)
	if !ok || s == "" {
		return nil, "", ""
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return nil, "", ""
	}
	person, _ := p.readStore.GetPerson(ctx, branchID, parsed)
	if person == nil {
		return &parsed, "", ""
	}
	return &parsed, person.GivenName, person.Surname
}

func (p *Projector) projectChildLinked(ctx context.Context, e domain.ChildLinkedToFamily, branchID domain.BranchID) error {
	// Get child name (split into given/surname)
	var childGivenName, childSurname string
	if child, _ := p.readStore.GetPerson(ctx, branchID, e.PersonID); child != nil {
		childGivenName = child.GivenName
		childSurname = child.Surname
	}

	fc := &FamilyChildReadModel{
		FamilyID:         e.FamilyID,
		PersonID:         e.PersonID,
		PersonGivenName:  childGivenName,
		PersonSurname:    childSurname,
		RelationshipType: e.RelationshipType,
		Sequence:         e.Sequence,
	}

	if err := p.readStore.SaveFamilyChild(ctx, branchID, fc); err != nil {
		return err
	}

	// Update pedigree edge for child
	family, err := p.readStore.GetFamily(ctx, branchID, e.FamilyID)
	if err != nil {
		return err
	}
	if family != nil {
		edge := &PedigreeEdge{
			PersonID: e.PersonID,
		}
		if family.Partner1ID != nil {
			// Determine father/mother based on gender (simplified)
			p1, _ := p.readStore.GetPerson(ctx, branchID, *family.Partner1ID)
			if p1 != nil {
				if p1.Gender == domain.GenderMale {
					edge.FatherID = family.Partner1ID
					edge.FatherName = p1.FullName
				} else {
					edge.MotherID = family.Partner1ID
					edge.MotherName = p1.FullName
				}
			}
		}
		if family.Partner2ID != nil {
			p2, _ := p.readStore.GetPerson(ctx, branchID, *family.Partner2ID)
			if p2 != nil {
				if p2.Gender == domain.GenderMale {
					edge.FatherID = family.Partner2ID
					edge.FatherName = p2.FullName
				} else {
					edge.MotherID = family.Partner2ID
					edge.MotherName = p2.FullName
				}
			}
		}
		if err := p.readStore.SavePedigreeEdge(ctx, branchID, edge); err != nil {
			return err
		}
	}

	// Increment family child count and version
	if family != nil {
		family.ChildCount++
		family.Version++
		family.UpdatedAt = e.OccurredAt()
		return p.readStore.SaveFamily(ctx, branchID, family)
	}

	return nil
}

func (p *Projector) projectChildUnlinked(ctx context.Context, e domain.ChildUnlinkedFromFamily, branchID domain.BranchID) error {
	if err := p.readStore.DeleteFamilyChild(ctx, branchID, e.FamilyID, e.PersonID); err != nil {
		return err
	}

	// Remove pedigree edge
	if err := p.readStore.DeletePedigreeEdge(ctx, branchID, e.PersonID); err != nil {
		return err
	}

	// Decrement family child count and increment version
	family, err := p.readStore.GetFamily(ctx, branchID, e.FamilyID)
	if err != nil {
		return err
	}
	if family != nil {
		if family.ChildCount > 0 {
			family.ChildCount--
		}
		family.Version++
		family.UpdatedAt = e.OccurredAt()
		return p.readStore.SaveFamily(ctx, branchID, family)
	}

	return nil
}

func (p *Projector) projectFamilyDeleted(ctx context.Context, e domain.FamilyDeleted, branchID domain.BranchID) error {
	// Delete all children first
	children, err := p.readStore.GetFamilyChildren(ctx, branchID, e.FamilyID)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := p.readStore.DeleteFamilyChild(ctx, branchID, e.FamilyID, child.PersonID); err != nil {
			return err
		}
		if err := p.readStore.DeletePedigreeEdge(ctx, branchID, child.PersonID); err != nil {
			return err
		}
	}

	return p.readStore.DeleteFamily(ctx, branchID, e.FamilyID)
}

func (p *Projector) projectSourceCreated(ctx context.Context, e domain.SourceCreated, version int64) error {
	var publishDateSort *time.Time
	var publishDateRaw string

	if e.PublishDate != nil {
		publishDateRaw = e.PublishDate.Raw
		t := e.PublishDate.ToTime()
		if !t.IsZero() {
			publishDateSort = &t
		}
	}

	source := &SourceReadModel{
		ID:              e.SourceID,
		SourceType:      e.SourceType,
		Title:           e.Title,
		Author:          e.Author,
		Publisher:       e.Publisher,
		PublishDateRaw:  publishDateRaw,
		PublishDateSort: publishDateSort,
		URL:             e.URL,
		RepositoryID:    e.RepositoryID,
		RepositoryName:  e.RepositoryName,
		CollectionName:  e.CollectionName,
		CallNumber:      e.CallNumber,
		Notes:           e.Notes,
		GedcomXref:      e.GedcomXref,
		CitationCount:   0,
		Version:         version,
		UpdatedAt:       e.OccurredAt(),
	}

	return p.readStore.SaveSource(ctx, source)
}

func (p *Projector) projectSourceUpdated(ctx context.Context, e domain.SourceUpdated, version int64) error {
	source, err := p.readStore.GetSource(ctx, e.SourceID)
	if err != nil {
		return err
	}
	if source == nil {
		return nil // Source doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "source_type":
			if v, ok := value.(string); ok {
				source.SourceType = domain.SourceType(v)
			}
		case "title":
			if v, ok := value.(string); ok {
				source.Title = v
			}
		case "author":
			if v, ok := value.(string); ok {
				source.Author = v
			}
		case "publisher":
			if v, ok := value.(string); ok {
				source.Publisher = v
			}
		case "publish_date":
			if v, ok := value.(string); ok {
				source.PublishDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					source.PublishDateSort = &t
				} else {
					source.PublishDateSort = nil
				}
			}
		case "url":
			if v, ok := value.(string); ok {
				source.URL = v
			}
		case "repository_id":
			source.RepositoryID = parseOptionalUUID(value)
		case "repository_name":
			if v, ok := value.(string); ok {
				source.RepositoryName = v
			}
		case "collection_name":
			if v, ok := value.(string); ok {
				source.CollectionName = v
			}
		case "call_number":
			if v, ok := value.(string); ok {
				source.CallNumber = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				source.Notes = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "SourceUpdated", "key", key)
		}
	}

	source.Version = version
	source.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveSource(ctx, source)
}

func (p *Projector) projectSourceDeleted(ctx context.Context, e domain.SourceDeleted) error {
	return p.readStore.DeleteSource(ctx, e.SourceID)
}

func (p *Projector) projectCitationCreated(ctx context.Context, e domain.CitationCreated, version int64) error {
	// Get source title for denormalization
	var sourceTitle string
	if source, _ := p.readStore.GetSource(ctx, e.SourceID); source != nil {
		sourceTitle = source.Title
	}

	var fieldsJSON string
	if len(e.Fields) > 0 {
		if b, err := json.Marshal(e.Fields); err == nil {
			fieldsJSON = string(b)
		}
	}

	citation := &CitationReadModel{
		ID:            e.CitationID,
		SourceID:      e.SourceID,
		SourceTitle:   sourceTitle,
		FactType:      e.FactType,
		FactOwnerID:   e.FactOwnerID,
		Page:          e.Page,
		Volume:        e.Volume,
		SourceQuality: e.SourceQuality,
		InformantType: e.InformantType,
		EvidenceType:  e.EvidenceType,
		QuotedText:    e.QuotedText,
		Analysis:      e.Analysis,
		TemplateID:    e.TemplateID,
		FieldsJSON:    fieldsJSON,
		GedcomXref:    e.GedcomXref,
		Version:       version,
		CreatedAt:     e.OccurredAt(),
	}

	if err := p.readStore.SaveCitation(ctx, citation); err != nil {
		return err
	}

	// Increment citation count on source
	source, err := p.readStore.GetSource(ctx, e.SourceID)
	if err != nil {
		return err
	}
	if source != nil {
		source.CitationCount++
		source.UpdatedAt = e.OccurredAt()
		return p.readStore.SaveSource(ctx, source)
	}

	return nil
}

func (p *Projector) projectCitationUpdated(ctx context.Context, e domain.CitationUpdated, version int64) error {
	citation, err := p.readStore.GetCitation(ctx, e.CitationID)
	if err != nil {
		return err
	}
	if citation == nil {
		return nil // Citation doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "source_id":
			if v, ok := value.(string); ok {
				if newSourceID, err := uuid.Parse(v); err == nil {
					// Update source citation counts
					if citation.SourceID != newSourceID {
						// Decrement old source
						if oldSource, _ := p.readStore.GetSource(ctx, citation.SourceID); oldSource != nil {
							if oldSource.CitationCount > 0 {
								oldSource.CitationCount--
							}
							_ = p.readStore.SaveSource(ctx, oldSource)
						}
						// Increment new source
						if newSource, _ := p.readStore.GetSource(ctx, newSourceID); newSource != nil {
							newSource.CitationCount++
							citation.SourceTitle = newSource.Title
							_ = p.readStore.SaveSource(ctx, newSource)
						}
						citation.SourceID = newSourceID
					}
				}
			}
		case "fact_type":
			if v, ok := value.(string); ok {
				citation.FactType = domain.FactType(v)
			}
		case "fact_owner_id":
			if v, ok := value.(string); ok {
				if id, err := uuid.Parse(v); err == nil {
					citation.FactOwnerID = id
				}
			}
		case "page":
			if v, ok := value.(string); ok {
				citation.Page = v
			}
		case "volume":
			if v, ok := value.(string); ok {
				citation.Volume = v
			}
		case "source_quality":
			if v, ok := value.(string); ok {
				citation.SourceQuality = domain.SourceQuality(v)
			}
		case "informant_type":
			if v, ok := value.(string); ok {
				citation.InformantType = domain.InformantType(v)
			}
		case "evidence_type":
			if v, ok := value.(string); ok {
				citation.EvidenceType = domain.EvidenceType(v)
			}
		case "quoted_text":
			if v, ok := value.(string); ok {
				citation.QuotedText = v
			}
		case "analysis":
			if v, ok := value.(string); ok {
				citation.Analysis = v
			}
		case "template_id":
			if v, ok := value.(string); ok {
				citation.TemplateID = v
			}
		case "fields":
			if b, err := json.Marshal(value); err == nil {
				citation.FieldsJSON = string(b)
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "CitationUpdated", "key", key)
		}
	}

	citation.Version = version
	return p.readStore.SaveCitation(ctx, citation)
}

func (p *Projector) projectCitationDeleted(ctx context.Context, e domain.CitationDeleted) error {
	// Get citation first to update source citation count
	citation, err := p.readStore.GetCitation(ctx, e.CitationID)
	if err != nil {
		return err
	}
	if citation != nil {
		// Decrement source citation count
		source, err := p.readStore.GetSource(ctx, citation.SourceID)
		if err != nil {
			return err
		}
		if source != nil {
			if source.CitationCount > 0 {
				source.CitationCount--
			}
			source.UpdatedAt = e.OccurredAt()
			if err := p.readStore.SaveSource(ctx, source); err != nil {
				return err
			}
		}
	}

	return p.readStore.DeleteCitation(ctx, e.CitationID)
}

func (p *Projector) projectMediaCreated(ctx context.Context, e domain.MediaCreated, version int64) error {
	media := &MediaReadModel{
		ID:            e.MediaID,
		EntityType:    e.EntityType,
		EntityID:      e.EntityID,
		Title:         e.Title,
		Description:   e.Description,
		MimeType:      e.MimeType,
		MediaType:     e.MediaType,
		Filename:      e.Filename,
		FileSize:      e.FileSize,
		FileData:      e.FileData,
		ThumbnailData: e.ThumbnailData,
		GedcomXref:    e.GedcomXref,
		Version:       version,
		CreatedAt:     e.OccurredAt(),
		UpdatedAt:     e.OccurredAt(),
		// GEDCOM 7.0 enhanced fields
		Files:        e.Files,
		Format:       e.Format,
		Translations: e.Translations,
	}

	return p.readStore.SaveMedia(ctx, media)
}

func (p *Projector) projectMediaUpdated(ctx context.Context, e domain.MediaUpdated, version int64) error {
	media, err := p.readStore.GetMediaWithData(ctx, e.MediaID)
	if err != nil {
		return err
	}
	if media == nil {
		return nil // Media doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "title":
			if v, ok := value.(string); ok {
				media.Title = v
			}
		case "description":
			if v, ok := value.(string); ok {
				media.Description = v
			}
		case "media_type":
			if v, ok := value.(string); ok {
				media.MediaType = domain.MediaType(v)
			}
		case "crop_left":
			if v, ok := value.(int); ok {
				media.CropLeft = &v
			}
		case "crop_top":
			if v, ok := value.(int); ok {
				media.CropTop = &v
			}
		case "crop_width":
			if v, ok := value.(int); ok {
				media.CropWidth = &v
			}
		case "crop_height":
			if v, ok := value.(int); ok {
				media.CropHeight = &v
			}
		case "files":
			if v, ok := value.([]domain.MediaFile); ok {
				media.Files = v
			}
		case "format":
			if v, ok := value.(string); ok {
				media.Format = v
			}
		case "translations":
			if v, ok := value.([]string); ok {
				media.Translations = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "MediaUpdated", "key", key)
		}
	}

	media.Version = version
	media.UpdatedAt = time.Now()

	return p.readStore.SaveMedia(ctx, media)
}

func (p *Projector) projectMediaDeleted(ctx context.Context, e domain.MediaDeleted) error {
	return p.readStore.DeleteMedia(ctx, e.MediaID)
}

func (p *Projector) projectLifeEventCreated(ctx context.Context, e domain.LifeEventCreated, version int64) error {
	var dateSort *time.Time
	var dateRaw string

	if e.Date != nil {
		dateRaw = e.Date.Raw
		t := e.Date.ToTime()
		if !t.IsZero() {
			dateSort = &t
		}
	}

	// Derive owner type and ID from PersonID/FamilyID
	var ownerType string
	var ownerID uuid.UUID
	if e.PersonID != nil {
		ownerType = "person"
		ownerID = *e.PersonID
	} else if e.FamilyID != nil {
		ownerType = "family"
		ownerID = *e.FamilyID
	}

	event := &EventReadModel{
		ID:          e.EventID,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		FactType:    e.FactType,
		DateRaw:     dateRaw,
		DateSort:    dateSort,
		Place:       e.Place,
		Address:     e.Address,
		Description: e.Description,
		Cause:       e.Cause,
		Age:         e.Age,
		IsNegated:   e.IsNegated,
		Version:     version,
		CreatedAt:   e.OccurredAt(),
	}

	return p.readStore.SaveEvent(ctx, event)
}

func (p *Projector) projectLifeEventDeleted(ctx context.Context, e domain.LifeEventDeleted) error {
	return p.readStore.DeleteEvent(ctx, e.EventID)
}

func (p *Projector) projectAttributeCreated(ctx context.Context, e domain.AttributeCreated, version int64) error {
	var dateSort *time.Time
	var dateRaw string

	if e.Date != nil {
		dateRaw = e.Date.Raw
		t := e.Date.ToTime()
		if !t.IsZero() {
			dateSort = &t
		}
	}

	attribute := &AttributeReadModel{
		ID:        e.AttributeID,
		PersonID:  e.PersonID,
		FactType:  e.FactType,
		Value:     e.Value,
		DateRaw:   dateRaw,
		DateSort:  dateSort,
		Place:     e.Place,
		Version:   version,
		CreatedAt: e.OccurredAt(),
	}

	return p.readStore.SaveAttribute(ctx, attribute)
}

func (p *Projector) projectAttributeDeleted(ctx context.Context, e domain.AttributeDeleted) error {
	return p.readStore.DeleteAttribute(ctx, e.AttributeID)
}

func (p *Projector) projectLifeEventUpdated(ctx context.Context, e domain.LifeEventUpdated, version int64) error {
	event, err := p.readStore.GetEvent(ctx, e.EventID)
	if err != nil {
		return err
	}
	if event == nil {
		return nil // Event doesn't exist in read model, skip
	}

	for key, value := range e.Changes {
		switch key {
		case "fact_type":
			if v, ok := value.(string); ok {
				event.FactType = domain.FactType(v)
			}
		case "date":
			if v, ok := value.(string); ok {
				event.DateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					event.DateSort = &t
				} else {
					event.DateSort = nil
				}
			}
		case "place":
			if v, ok := value.(string); ok {
				event.Place = v
			}
		case "address":
			if value == nil {
				event.Address = nil
			} else {
				switch v := value.(type) {
				case *domain.Address:
					event.Address = v
				case map[string]any:
					b, _ := json.Marshal(v)
					var addr domain.Address
					if json.Unmarshal(b, &addr) == nil {
						event.Address = &addr
					}
				}
			}
		case "description":
			if v, ok := value.(string); ok {
				event.Description = v
			}
		case "cause":
			if v, ok := value.(string); ok {
				event.Cause = v
			}
		case "age":
			if v, ok := value.(string); ok {
				event.Age = v
			}
		case "is_negated":
			if v, ok := value.(bool); ok {
				event.IsNegated = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "LifeEventUpdated", "key", key)
		}
	}

	event.Version = version

	return p.readStore.SaveEvent(ctx, event)
}

func (p *Projector) projectAttributeUpdated(ctx context.Context, e domain.AttributeUpdated, version int64) error {
	attribute, err := p.readStore.GetAttribute(ctx, e.AttributeID)
	if err != nil {
		return err
	}
	if attribute == nil {
		return nil // Attribute doesn't exist in read model, skip
	}

	for key, value := range e.Changes {
		switch key {
		case "fact_type":
			if v, ok := value.(string); ok {
				attribute.FactType = domain.FactType(v)
			}
		case "value":
			if v, ok := value.(string); ok {
				attribute.Value = v
			}
		case "date":
			if v, ok := value.(string); ok {
				attribute.DateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					attribute.DateSort = &t
				} else {
					attribute.DateSort = nil
				}
			}
		case "place":
			if v, ok := value.(string); ok {
				attribute.Place = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "AttributeUpdated", "key", key)
		}
	}

	attribute.Version = version

	return p.readStore.SaveAttribute(ctx, attribute)
}

func (p *Projector) projectRepositoryCreated(ctx context.Context, e domain.RepositoryCreated, version int64) error {
	// Build a domain.Repository to reuse GetAddress() for mapping the flat
	// address fields into a structured *domain.Address.
	r := &domain.Repository{
		ID:            e.RepositoryID,
		Name:          e.Name,
		StreetAddress: e.StreetAddress,
		City:          e.City,
		State:         e.State,
		PostalCode:    e.PostalCode,
		Country:       e.Country,
		Phone:         e.Phone,
		Email:         e.Email,
		Website:       e.Website,
		Notes:         e.Notes,
		GedcomXref:    e.GedcomXref,
	}

	repo := &RepositoryReadModel{
		ID:         e.RepositoryID,
		Name:       e.Name,
		Address:    r.GetAddress(),
		Notes:      e.Notes,
		GedcomXref: e.GedcomXref,
		Version:    version,
		UpdatedAt:  e.OccurredAt(),
	}

	return p.readStore.SaveRepository(ctx, repo)
}

func (p *Projector) projectRepositoryUpdated(ctx context.Context, e domain.RepositoryUpdated, version int64) error {
	repo, err := p.readStore.GetRepository(ctx, e.RepositoryID)
	if err != nil {
		return err
	}
	if repo == nil {
		return nil // Repository doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "name":
			if v, ok := value.(string); ok {
				repo.Name = v
			}
		case "address":
			// On replay the event is decoded from JSON, so the address arrives
			// as map[string]any rather than *domain.Address. Handle both, plus
			// nil to clear. Mirrors projectLifeEventUpdated.
			if value == nil {
				repo.Address = nil
			} else {
				switch v := value.(type) {
				case *domain.Address:
					repo.Address = v
				case map[string]any:
					b, _ := json.Marshal(v)
					var addr domain.Address
					if json.Unmarshal(b, &addr) == nil {
						repo.Address = &addr
					}
				}
			}
		case "notes":
			if v, ok := value.(string); ok {
				repo.Notes = v
			}
		case "gedcom_xref":
			if v, ok := value.(string); ok {
				repo.GedcomXref = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "RepositoryUpdated", "key", key)
		}
	}

	repo.Version = version
	repo.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveRepository(ctx, repo)
}

func (p *Projector) projectRepositoryDeleted(ctx context.Context, e domain.RepositoryDeleted) error {
	return p.readStore.DeleteRepository(ctx, e.RepositoryID)
}

func (p *Projector) projectNameAdded(ctx context.Context, e domain.NameAdded, version int64, branchID domain.BranchID) error {
	// Build full name from components
	fullName := buildFullName(e.GivenName, e.Surname, e.NamePrefix, e.NameSuffix, e.SurnamePrefix)

	name := &PersonNameReadModel{
		ID:            e.NameID,
		PersonID:      e.PersonID,
		GivenName:     e.GivenName,
		Surname:       e.Surname,
		FullName:      fullName,
		NamePrefix:    e.NamePrefix,
		NameSuffix:    e.NameSuffix,
		SurnamePrefix: e.SurnamePrefix,
		Nickname:      e.Nickname,
		NameType:      e.NameType,
		IsPrimary:     e.IsPrimary,
		UpdatedAt:     e.OccurredAt(),
	}

	if err := p.readStore.SavePersonName(ctx, branchID, name); err != nil {
		return err
	}

	// Update person version to stay in sync with event stream
	return p.updatePersonVersion(ctx, branchID, e.PersonID, version)
}

func (p *Projector) projectNameUpdated(ctx context.Context, e domain.NameUpdated, version int64, branchID domain.BranchID) error {
	// Build full name from components
	fullName := buildFullName(e.GivenName, e.Surname, e.NamePrefix, e.NameSuffix, e.SurnamePrefix)

	name := &PersonNameReadModel{
		ID:            e.NameID,
		PersonID:      e.PersonID,
		GivenName:     e.GivenName,
		Surname:       e.Surname,
		FullName:      fullName,
		NamePrefix:    e.NamePrefix,
		NameSuffix:    e.NameSuffix,
		SurnamePrefix: e.SurnamePrefix,
		Nickname:      e.Nickname,
		NameType:      e.NameType,
		IsPrimary:     e.IsPrimary,
		UpdatedAt:     e.OccurredAt(),
	}

	if err := p.readStore.SavePersonName(ctx, branchID, name); err != nil {
		return err
	}

	// Update person version to stay in sync with event stream
	return p.updatePersonVersion(ctx, branchID, e.PersonID, version)
}

func (p *Projector) projectNameRemoved(ctx context.Context, e domain.NameRemoved, version int64, branchID domain.BranchID) error {
	if err := p.readStore.DeletePersonName(ctx, branchID, e.NameID); err != nil {
		return err
	}

	// Update person version to stay in sync with event stream
	return p.updatePersonVersion(ctx, branchID, e.PersonID, version)
}

// updatePersonVersion updates a person's version in the read model.
func (p *Projector) updatePersonVersion(ctx context.Context, branchID domain.BranchID, personID uuid.UUID, version int64) error {
	person, err := p.readStore.GetPerson(ctx, branchID, personID)
	if err != nil {
		return fmt.Errorf("get person for version update: %w", err)
	}
	if person == nil {
		// Person may not exist yet if events are out of order
		return nil
	}
	person.Version = version
	return p.readStore.SavePerson(ctx, branchID, person)
}

// buildFullName constructs a full name from its components.
func buildFullName(givenName, surname, namePrefix, nameSuffix, surnamePrefix string) string {
	var parts []string

	if namePrefix != "" {
		parts = append(parts, namePrefix)
	}
	if givenName != "" {
		parts = append(parts, givenName)
	}
	if surnamePrefix != "" {
		parts = append(parts, surnamePrefix)
	}
	if surname != "" {
		parts = append(parts, surname)
	}
	if nameSuffix != "" {
		parts = append(parts, nameSuffix)
	}

	if len(parts) == 0 {
		return ""
	}

	// Join with spaces
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += " " + parts[i]
	}
	return result
}

// projectPersonMerged handles the PersonMerged event by updating the survivor,
// transferring relationships/data, and deleting the merged person.
func (p *Projector) projectPersonMerged(ctx context.Context, e domain.PersonMerged, version int64, branchID domain.BranchID) error {
	// 1. Update survivor person with resolved fields
	survivor, err := p.readStore.GetPerson(ctx, branchID, e.SurvivorID)
	if err != nil {
		return err
	}
	if survivor == nil {
		return nil // Survivor doesn't exist, skip
	}

	// Apply resolved fields to survivor (same logic as PersonUpdated)
	for key, value := range e.ResolvedFields {
		switch key {
		case "given_name":
			if v, ok := value.(string); ok {
				survivor.GivenName = v
				survivor.FullName = v + " " + survivor.Surname
			}
		case "surname":
			if v, ok := value.(string); ok {
				survivor.Surname = v
				survivor.FullName = survivor.GivenName + " " + v
			}
		case "gender":
			if v, ok := value.(string); ok {
				survivor.Gender = domain.Gender(v)
			}
		case "birth_date":
			if v, ok := value.(string); ok {
				survivor.BirthDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					survivor.BirthDateSort = &t
				} else {
					survivor.BirthDateSort = nil
				}
			}
		case "birth_place":
			if v, ok := value.(string); ok {
				survivor.BirthPlace = v
			}
		case "death_date":
			if v, ok := value.(string); ok {
				survivor.DeathDateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					survivor.DeathDateSort = &t
				} else {
					survivor.DeathDateSort = nil
				}
			}
		case "death_place":
			if v, ok := value.(string); ok {
				survivor.DeathPlace = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				survivor.Notes = v
			}
		case "research_status":
			if v, ok := value.(string); ok {
				survivor.ResearchStatus = domain.ParseResearchStatus(v)
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "PersonMerged", "key", key)
		}
	}

	survivor.Version = version
	survivor.UpdatedAt = e.OccurredAt()

	if err := p.readStore.SavePerson(ctx, branchID, survivor); err != nil {
		return err
	}

	// 2. Update families where merged person is a partner
	families, err := p.readStore.GetFamiliesForPerson(ctx, branchID, e.MergedID)
	if err != nil {
		return err
	}
	for _, family := range families {
		if family.Partner1ID != nil && *family.Partner1ID == e.MergedID {
			family.Partner1ID = &e.SurvivorID
			family.Partner1GivenName = survivor.GivenName
			family.Partner1Surname = survivor.Surname
		}
		if family.Partner2ID != nil && *family.Partner2ID == e.MergedID {
			family.Partner2ID = &e.SurvivorID
			family.Partner2GivenName = survivor.GivenName
			family.Partner2Surname = survivor.Surname
		}
		family.UpdatedAt = e.OccurredAt()
		if err := p.readStore.SaveFamily(ctx, branchID, &family); err != nil {
			return err
		}
	}

	// 3. Update pedigree edges where merged person is a parent
	// (We need to find all children who have the merged person as father/mother)
	// This requires iterating through all pedigree edges - a bit expensive but necessary
	// For now, handle the merged person's own pedigree edge (if they are a child somewhere)
	mergedEdge, err := p.readStore.GetPedigreeEdge(ctx, branchID, e.MergedID)
	if err != nil {
		return err
	}
	if mergedEdge != nil {
		// Transfer the child-family relationship to survivor
		// First, get the family where merged person is a child
		mergedChildFamily, err := p.readStore.GetChildFamily(ctx, branchID, e.MergedID)
		if err != nil {
			return err
		}
		if mergedChildFamily != nil {
			// Delete old family-child record
			if err := p.readStore.DeleteFamilyChild(ctx, branchID, mergedChildFamily.ID, e.MergedID); err != nil {
				return err
			}
			// Delete old pedigree edge
			if err := p.readStore.DeletePedigreeEdge(ctx, branchID, e.MergedID); err != nil {
				return err
			}

			// Check if survivor already has a child-family relationship
			survivorChildFamily, err := p.readStore.GetChildFamily(ctx, branchID, e.SurvivorID)
			if err != nil {
				return err
			}
			if survivorChildFamily == nil {
				// Survivor doesn't have a child-family, transfer the relationship
				fc := &FamilyChildReadModel{
					FamilyID:         mergedChildFamily.ID,
					PersonID:         e.SurvivorID,
					PersonGivenName:  survivor.GivenName,
					PersonSurname:    survivor.Surname,
					RelationshipType: domain.ChildBiological, // Default, could be improved
				}
				if err := p.readStore.SaveFamilyChild(ctx, branchID, fc); err != nil {
					return err
				}

				// Create pedigree edge for survivor
				edge := &PedigreeEdge{
					PersonID:   e.SurvivorID,
					FatherID:   mergedEdge.FatherID,
					MotherID:   mergedEdge.MotherID,
					FatherName: mergedEdge.FatherName,
					MotherName: mergedEdge.MotherName,
				}
				if err := p.readStore.SavePedigreeEdge(ctx, branchID, edge); err != nil {
					return err
				}

				// Update family child count
				mergedChildFamily.ChildCount-- // We removed merged
				mergedChildFamily.ChildCount++ // We added survivor (net 0 change if same family)
				mergedChildFamily.UpdatedAt = e.OccurredAt()
				if err := p.readStore.SaveFamily(ctx, branchID, mergedChildFamily); err != nil {
					return err
				}
			}
			// If survivor already has a child-family, we don't transfer (plan says block this case)
		}
	}

	// 4. Reassign citations from merged person to survivor
	citations, err := p.readStore.GetCitationsForPerson(ctx, e.MergedID)
	if err != nil {
		return fmt.Errorf("fetch citations for merged person %s: %w", e.MergedID, err)
	}
	for _, citation := range citations {
		citation.FactOwnerID = e.SurvivorID
		if err := p.readStore.SaveCitation(ctx, &citation); err != nil {
			return fmt.Errorf("migrate citation %s for merged person %s: %w", citation.ID, e.MergedID, err)
		}
	}

	// 5. Transfer PersonName records from merged to survivor
	names, err := p.readStore.GetPersonNames(ctx, branchID, e.MergedID)
	if err != nil {
		return fmt.Errorf("fetch person names for merged person %s: %w", e.MergedID, err)
	}
	for _, name := range names {
		// Change the person ID to survivor and mark as non-primary
		name.PersonID = e.SurvivorID
		name.IsPrimary = false // Transferred names become alternate names
		name.UpdatedAt = e.OccurredAt()
		if err := p.readStore.SavePersonName(ctx, branchID, &name); err != nil {
			return fmt.Errorf("migrate person name %s for merged person %s: %w", name.ID, e.MergedID, err)
		}
	}

	// 6. Transfer life events from merged person to survivor
	events, err := p.readStore.ListEventsForPerson(ctx, e.MergedID)
	if err != nil {
		return fmt.Errorf("fetch events for merged person %s: %w", e.MergedID, err)
	}
	for _, event := range events {
		event.OwnerID = e.SurvivorID
		if err := p.readStore.SaveEvent(ctx, &event); err != nil {
			return fmt.Errorf("migrate event %s for merged person %s: %w", event.ID, e.MergedID, err)
		}
	}

	// 7. Transfer media from merged person to survivor
	mediaList, _, err := p.readStore.ListMediaForEntity(ctx, "person", e.MergedID, ListOptions{Limit: 10000})
	if err != nil {
		return fmt.Errorf("fetch media for merged person %s: %w", e.MergedID, err)
	}
	for _, media := range mediaList {
		media.EntityID = e.SurvivorID
		media.UpdatedAt = e.OccurredAt()
		if err := p.readStore.SaveMedia(ctx, &media); err != nil {
			return fmt.Errorf("migrate media %s for merged person %s: %w", media.ID, e.MergedID, err)
		}
	}

	// 8. Transfer attributes from merged person to survivor
	attributes, err := p.readStore.ListAttributesForPerson(ctx, e.MergedID)
	if err != nil {
		return fmt.Errorf("fetch attributes for merged person %s: %w", e.MergedID, err)
	}
	for _, attr := range attributes {
		attr.PersonID = e.SurvivorID
		if err := p.readStore.SaveAttribute(ctx, &attr); err != nil {
			return fmt.Errorf("migrate attribute %s for merged person %s: %w", attr.ID, e.MergedID, err)
		}
	}

	// 9. Transfer evidence analyses from merged person to survivor
	analyses, err := p.readStore.GetAnalysesBySubject(ctx, e.MergedID)
	if err != nil {
		return fmt.Errorf("fetch evidence analyses for merged person %s: %w", e.MergedID, err)
	}
	for _, analysis := range analyses {
		analysis.SubjectID = e.SurvivorID
		if err := p.readStore.SaveEvidenceAnalysis(ctx, &analysis); err != nil {
			return fmt.Errorf("migrate evidence analysis %s for merged person %s: %w", analysis.ID, e.MergedID, err)
		}
	}

	// 10. Transfer evidence conflicts from merged person to survivor
	conflicts, err := p.readStore.GetConflictsForSubject(ctx, e.MergedID)
	if err != nil {
		return fmt.Errorf("fetch evidence conflicts for merged person %s: %w", e.MergedID, err)
	}
	for _, conflict := range conflicts {
		conflict.SubjectID = e.SurvivorID
		if err := p.readStore.SaveEvidenceConflict(ctx, &conflict); err != nil {
			return fmt.Errorf("migrate evidence conflict %s for merged person %s: %w", conflict.ID, e.MergedID, err)
		}
	}

	// 11. Transfer research logs from merged person to survivor
	researchLogs, err := p.readStore.GetResearchLogsForSubject(ctx, e.MergedID)
	if err != nil {
		return fmt.Errorf("fetch research logs for merged person %s: %w", e.MergedID, err)
	}
	for _, log := range researchLogs {
		log.SubjectID = e.SurvivorID
		if err := p.readStore.SaveResearchLog(ctx, &log); err != nil {
			return fmt.Errorf("migrate research log %s for merged person %s: %w", log.ID, e.MergedID, err)
		}
	}

	// 12. Transfer proof summaries from merged person to survivor
	summaries, err := p.readStore.GetProofSummariesBySubject(ctx, e.MergedID)
	if err != nil {
		return fmt.Errorf("fetch proof summaries for merged person %s: %w", e.MergedID, err)
	}
	for _, summary := range summaries {
		summary.SubjectID = e.SurvivorID
		if err := p.readStore.SaveProofSummary(ctx, &summary); err != nil {
			return fmt.Errorf("migrate proof summary %s for merged person %s: %w", summary.ID, e.MergedID, err)
		}
	}

	// 13. Delete merged person from read model
	return p.readStore.DeletePerson(ctx, branchID, e.MergedID)
}

// Note projections

func (p *Projector) projectNoteCreated(ctx context.Context, e domain.NoteCreated, version int64) error {
	note := &NoteReadModel{
		ID:           e.NoteID,
		Text:         e.Text,
		MIME:         e.MIME,
		Language:     e.Language,
		Translations: e.Translations,
		GedcomXref:   e.GedcomXref,
		Version:      version,
		UpdatedAt:    e.OccurredAt(),
	}

	return p.readStore.SaveNote(ctx, note)
}

func (p *Projector) projectNoteUpdated(ctx context.Context, e domain.NoteUpdated, version int64) error {
	note, err := p.readStore.GetNote(ctx, e.NoteID)
	if err != nil {
		return err
	}
	if note == nil {
		return nil // Note doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		if key == "text" {
			if v, ok := value.(string); ok {
				note.Text = v
			}
		}
	}

	note.Version = version
	note.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveNote(ctx, note)
}

func (p *Projector) projectNoteDeleted(ctx context.Context, e domain.NoteDeleted) error {
	return p.readStore.DeleteNote(ctx, e.NoteID)
}

// Submitter projections

func (p *Projector) projectSubmitterCreated(ctx context.Context, e domain.SubmitterCreated, version int64) error {
	submitter := &SubmitterReadModel{
		ID:         e.SubmitterID,
		Name:       e.Name,
		Address:    e.Address,
		Phone:      e.Phone,
		Email:      e.Email,
		Language:   e.Language,
		MediaID:    e.MediaID,
		GedcomXref: e.GedcomXref,
		Version:    version,
		UpdatedAt:  e.OccurredAt(),
	}

	return p.readStore.SaveSubmitter(ctx, submitter)
}

func (p *Projector) projectSubmitterUpdated(ctx context.Context, e domain.SubmitterUpdated, version int64) error {
	submitter, err := p.readStore.GetSubmitter(ctx, e.SubmitterID)
	if err != nil {
		return err
	}
	if submitter == nil {
		return nil // Submitter doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "name":
			if v, ok := value.(string); ok {
				submitter.Name = v
			}
		case "address":
			if v, ok := value.(*domain.Address); ok {
				submitter.Address = v
			}
		case "phone":
			if v, ok := value.([]string); ok {
				submitter.Phone = v
			}
		case "email":
			if v, ok := value.([]string); ok {
				submitter.Email = v
			}
		case "language":
			if v, ok := value.(string); ok {
				submitter.Language = v
			}
		case "media_id":
			if v, ok := value.(*uuid.UUID); ok {
				submitter.MediaID = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "SubmitterUpdated", "key", key)
		}
	}

	submitter.Version = version
	submitter.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveSubmitter(ctx, submitter)
}

func (p *Projector) projectSubmitterDeleted(ctx context.Context, e domain.SubmitterDeleted) error {
	return p.readStore.DeleteSubmitter(ctx, e.SubmitterID)
}

// Association projections

func (p *Projector) projectAssociationCreated(ctx context.Context, e domain.AssociationCreated, version int64, branchID domain.BranchID) error {
	// Look up person names for denormalization on the event's branch scope so a
	// branch-local person resolves correctly (falls back to main via the overlay).
	personName := ""
	associateName := ""

	if person, err := p.readStore.GetPerson(ctx, branchID, e.PersonID); err == nil && person != nil {
		personName = person.FullName
	}
	if associate, err := p.readStore.GetPerson(ctx, branchID, e.AssociateID); err == nil && associate != nil {
		associateName = associate.FullName
	}

	association := &AssociationReadModel{
		ID:            e.AssociationID,
		PersonID:      e.PersonID,
		PersonName:    personName,
		AssociateID:   e.AssociateID,
		AssociateName: associateName,
		Role:          e.Role,
		Phrase:        e.Phrase,
		Notes:         e.Notes,
		NoteIDs:       e.NoteIDs,
		GedcomXref:    e.GedcomXref,
		Version:       version,
		UpdatedAt:     e.OccurredAt(),
	}

	return p.readStore.SaveAssociation(ctx, association)
}

func (p *Projector) projectAssociationUpdated(ctx context.Context, e domain.AssociationUpdated, version int64) error {
	association, err := p.readStore.GetAssociation(ctx, e.AssociationID)
	if err != nil {
		return err
	}
	if association == nil {
		return nil // Association doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "role":
			if v, ok := value.(string); ok {
				association.Role = v
			}
		case "phrase":
			if v, ok := value.(string); ok {
				association.Phrase = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				association.Notes = v
			}
		case "note_ids":
			if v, ok := value.([]uuid.UUID); ok {
				association.NoteIDs = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "AssociationUpdated", "key", key)
		}
	}

	association.Version = version
	association.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveAssociation(ctx, association)
}

func (p *Projector) projectAssociationDeleted(ctx context.Context, e domain.AssociationDeleted) error {
	return p.readStore.DeleteAssociation(ctx, e.AssociationID)
}

// LDS Ordinance projections

func (p *Projector) projectLDSOrdinanceCreated(ctx context.Context, e domain.LDSOrdinanceCreated, version int64, branchID domain.BranchID) error {
	var dateSort *time.Time
	var dateRaw string

	if e.Date != nil {
		dateRaw = e.Date.Raw
		t := e.Date.ToTime()
		if !t.IsZero() {
			dateSort = &t
		}
	}

	// Look up person name for denormalization on the event's branch scope so a
	// branch-local person resolves correctly (falls back to main via the overlay).
	personName := ""
	if e.PersonID != nil {
		if person, err := p.readStore.GetPerson(ctx, branchID, *e.PersonID); err == nil && person != nil {
			personName = person.FullName
		}
	}

	ordinance := &LDSOrdinanceReadModel{
		ID:         e.OrdinanceID,
		Type:       e.Type,
		TypeLabel:  e.Type.Label(),
		PersonID:   e.PersonID,
		PersonName: personName,
		FamilyID:   e.FamilyID,
		DateRaw:    dateRaw,
		DateSort:   dateSort,
		Place:      e.Place,
		Temple:     e.Temple,
		Status:     e.Status,
		Version:    version,
		UpdatedAt:  e.OccurredAt(),
	}

	return p.readStore.SaveLDSOrdinance(ctx, ordinance)
}

func (p *Projector) projectLDSOrdinanceUpdated(ctx context.Context, e domain.LDSOrdinanceUpdated, version int64) error {
	ordinance, err := p.readStore.GetLDSOrdinance(ctx, e.OrdinanceID)
	if err != nil {
		return err
	}
	if ordinance == nil {
		return nil // Ordinance doesn't exist in read model, skip
	}

	// Apply changes
	for key, value := range e.Changes {
		switch key {
		case "date":
			if v, ok := value.(string); ok {
				ordinance.DateRaw = v
				gd := domain.ParseGenDate(v)
				t := gd.ToTime()
				if !t.IsZero() {
					ordinance.DateSort = &t
				} else {
					ordinance.DateSort = nil
				}
			}
		case "place":
			if v, ok := value.(string); ok {
				ordinance.Place = v
			}
		case "temple":
			if v, ok := value.(string); ok {
				ordinance.Temple = v
			}
		case "status":
			if v, ok := value.(string); ok {
				ordinance.Status = v
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "LDSOrdinanceUpdated", "key", key)
		}
	}

	ordinance.Version = version
	ordinance.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveLDSOrdinance(ctx, ordinance)
}

func (p *Projector) projectLDSOrdinanceDeleted(ctx context.Context, e domain.LDSOrdinanceDeleted) error {
	return p.readStore.DeleteLDSOrdinance(ctx, e.OrdinanceID)
}

func (p *Projector) projectEvidenceAnalysisCreated(ctx context.Context, e domain.EvidenceAnalysisCreated, version int64) error {
	var citationIDsJSON string
	if len(e.CitationIDs) > 0 {
		if b, err := json.Marshal(e.CitationIDs); err == nil {
			citationIDsJSON = string(b)
		}
	}

	analysis := &EvidenceAnalysisReadModel{
		ID:              e.AnalysisID,
		FactType:        e.FactType,
		SubjectID:       e.SubjectID,
		CitationIDsJSON: citationIDsJSON,
		Conclusion:      e.Conclusion,
		ResearchStatus:  e.ResearchStatus,
		Notes:           e.Notes,
		Version:         version,
		CreatedAt:       e.OccurredAt(),
		UpdatedAt:       e.OccurredAt(),
	}

	return p.readStore.SaveEvidenceAnalysis(ctx, analysis)
}

func (p *Projector) projectEvidenceAnalysisUpdated(ctx context.Context, e domain.EvidenceAnalysisUpdated, version int64) error {
	analysis, err := p.readStore.GetEvidenceAnalysis(ctx, e.AnalysisID)
	if err != nil {
		return err
	}
	if analysis == nil {
		return nil
	}

	for key, value := range e.Changes {
		switch key {
		case "fact_type":
			if v, ok := value.(string); ok {
				analysis.FactType = domain.FactType(v)
			}
		case "subject_id":
			if v, ok := value.(string); ok {
				if id, err := uuid.Parse(v); err == nil {
					analysis.SubjectID = id
				}
			}
		case "conclusion":
			if v, ok := value.(string); ok {
				analysis.Conclusion = v
			}
		case "notes":
			if v, ok := value.(string); ok {
				analysis.Notes = v
			}
		case "research_status":
			if v, ok := value.(string); ok {
				analysis.ResearchStatus = domain.ResearchStatus(v)
			}
		case "citation_ids":
			if b, err := json.Marshal(value); err == nil {
				analysis.CitationIDsJSON = string(b)
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "EvidenceAnalysisUpdated", "key", key)
		}
	}

	analysis.Version = version
	analysis.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveEvidenceAnalysis(ctx, analysis)
}

func (p *Projector) projectEvidenceAnalysisDeleted(ctx context.Context, e domain.EvidenceAnalysisDeleted) error {
	return p.readStore.DeleteEvidenceAnalysis(ctx, e.AnalysisID)
}

func (p *Projector) projectEvidenceConflictDetected(ctx context.Context, e domain.EvidenceConflictDetected, version int64) error {
	var analysisIDsJSON string
	if len(e.AnalysisIDs) > 0 {
		if b, err := json.Marshal(e.AnalysisIDs); err == nil {
			analysisIDsJSON = string(b)
		}
	}

	conflict := &EvidenceConflictReadModel{
		ID:              e.ConflictID,
		FactType:        e.FactType,
		SubjectID:       e.SubjectID,
		AnalysisIDsJSON: analysisIDsJSON,
		Description:     e.Description,
		Status:          e.Status,
		Version:         version,
		CreatedAt:       e.OccurredAt(),
		UpdatedAt:       e.OccurredAt(),
	}

	return p.readStore.SaveEvidenceConflict(ctx, conflict)
}

func (p *Projector) projectEvidenceConflictResolved(ctx context.Context, e domain.EvidenceConflictResolved, version int64) error {
	conflict, err := p.readStore.GetEvidenceConflict(ctx, e.ConflictID)
	if err != nil {
		return err
	}
	if conflict == nil {
		return nil
	}

	conflict.Resolution = e.Resolution
	conflict.Status = e.Status
	conflict.Version = version
	conflict.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveEvidenceConflict(ctx, conflict)
}

func (p *Projector) projectResearchLogCreated(ctx context.Context, e domain.ResearchLogCreated, version int64) error {
	log := &ResearchLogReadModel{
		ID:                e.LogID,
		SubjectID:         e.SubjectID,
		SubjectType:       e.SubjectType,
		Repository:        e.Repository,
		SearchDescription: e.SearchDescription,
		Outcome:           e.Outcome,
		Notes:             e.Notes,
		SearchDate:        e.SearchDate,
		Version:           version,
		CreatedAt:         e.OccurredAt(),
		UpdatedAt:         e.OccurredAt(),
	}

	return p.readStore.SaveResearchLog(ctx, log)
}

func (p *Projector) projectResearchLogUpdated(ctx context.Context, e domain.ResearchLogUpdated, version int64) error {
	log, err := p.readStore.GetResearchLog(ctx, e.LogID)
	if err != nil {
		return err
	}
	if log == nil {
		return nil
	}

	for key, value := range e.Changes {
		switch key {
		case "repository":
			if v, ok := value.(string); ok {
				log.Repository = v
			}
		case "search_description":
			if v, ok := value.(string); ok {
				log.SearchDescription = v
			}
		case "outcome":
			if v, ok := value.(string); ok {
				log.Outcome = domain.ResearchOutcome(v)
			}
		case "notes":
			if v, ok := value.(string); ok {
				log.Notes = v
			}
		case "subject_id":
			if v, ok := value.(string); ok {
				if id, err := uuid.Parse(v); err == nil {
					log.SubjectID = id
				}
			}
		case "subject_type":
			if v, ok := value.(string); ok {
				log.SubjectType = v
			}
		case "search_date":
			if v, ok := value.(string); ok {
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					log.SearchDate = t
				}
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "ResearchLogUpdated", "key", key)
		}
	}

	log.Version = version
	log.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveResearchLog(ctx, log)
}

func (p *Projector) projectResearchLogDeleted(ctx context.Context, e domain.ResearchLogDeleted) error {
	return p.readStore.DeleteResearchLog(ctx, e.LogID)
}

func (p *Projector) projectProofSummaryCreated(ctx context.Context, e domain.ProofSummaryCreated, version int64) error {
	var analysisIDsJSON string
	if len(e.AnalysisIDs) > 0 {
		if b, err := json.Marshal(e.AnalysisIDs); err == nil {
			analysisIDsJSON = string(b)
		}
	}

	summary := &ProofSummaryReadModel{
		ID:              e.SummaryID,
		FactType:        e.FactType,
		SubjectID:       e.SubjectID,
		Conclusion:      e.Conclusion,
		Argument:        e.Argument,
		AnalysisIDsJSON: analysisIDsJSON,
		ResearchStatus:  e.ResearchStatus,
		Version:         version,
		CreatedAt:       e.OccurredAt(),
		UpdatedAt:       e.OccurredAt(),
	}

	return p.readStore.SaveProofSummary(ctx, summary)
}

func (p *Projector) projectProofSummaryUpdated(ctx context.Context, e domain.ProofSummaryUpdated, version int64) error {
	summary, err := p.readStore.GetProofSummary(ctx, e.SummaryID)
	if err != nil {
		return err
	}
	if summary == nil {
		return nil
	}

	for key, value := range e.Changes {
		switch key {
		case "fact_type":
			if v, ok := value.(string); ok {
				summary.FactType = domain.FactType(v)
			}
		case "subject_id":
			if v, ok := value.(string); ok {
				if id, err := uuid.Parse(v); err == nil {
					summary.SubjectID = id
				}
			}
		case "conclusion":
			if v, ok := value.(string); ok {
				summary.Conclusion = v
			}
		case "argument":
			if v, ok := value.(string); ok {
				summary.Argument = v
			}
		case "research_status":
			if v, ok := value.(string); ok {
				summary.ResearchStatus = domain.ResearchStatus(v)
			}
		case "analysis_ids":
			if b, err := json.Marshal(value); err == nil {
				summary.AnalysisIDsJSON = string(b)
			}
		default:
			slog.Warn("projection: ignoring unknown change key", "event", "ProofSummaryUpdated", "key", key)
		}
	}

	summary.Version = version
	summary.UpdatedAt = e.OccurredAt()

	return p.readStore.SaveProofSummary(ctx, summary)
}

func (p *Projector) projectProofSummaryDeleted(ctx context.Context, e domain.ProofSummaryDeleted) error {
	return p.readStore.DeleteProofSummary(ctx, e.SummaryID)
}
