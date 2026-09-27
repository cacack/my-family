package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestReadModelStore_DeletePersonCascade verifies that deleting a person on main
// removes every dependent the pre-#669 ON DELETE CASCADE foreign keys used to
// clean up: person_names, person_external_ids, pedigree_edges, associations (both
// the person_id and associate_id sides) and attributes — plus the person's own life
// events, which #757 added to the cascade. The assertions are kept
// byte-for-byte identical across the memory/sqlite/postgres backends to enforce
// DB-001 parity and would have caught the incomplete/divergent manual cascade.
func TestReadModelStore_DeletePersonCascade(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	main := domain.MainBranchID
	personID := uuid.New()

	if err := store.SavePerson(ctx, main, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Surname: "Lovelace", Version: 1}); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	if err := store.SavePersonName(ctx, main, &repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Ada", Surname: "Lovelace"}); err != nil {
		t.Fatalf("SavePersonName: %v", err)
	}
	if err := store.ReplacePersonExternalIDs(ctx, main, personID, []repository.PersonExternalIDReadModel{{Value: "X1"}}); err != nil {
		t.Fatalf("ReplacePersonExternalIDs: %v", err)
	}
	if err := store.SavePedigreeEdge(ctx, main, &repository.PedigreeEdge{PersonID: personID}); err != nil {
		t.Fatalf("SavePedigreeEdge: %v", err)
	}
	// The person is also a child of a family: family_children referenced persons(id)
	// ON DELETE CASCADE on the child side, so that row must be cleaned up too.
	childFamilyID := uuid.New()
	if err := store.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: childFamilyID, RelationshipType: domain.RelationMarriage, Version: 1}); err != nil {
		t.Fatalf("SaveFamily: %v", err)
	}
	if err := store.SaveFamilyChild(ctx, main, &repository.FamilyChildReadModel{FamilyID: childFamilyID, PersonID: personID, RelationshipType: domain.ChildBiological}); err != nil {
		t.Fatalf("SaveFamilyChild: %v", err)
	}
	// Association where the deleted person is the subject (person_id side).
	assocSubject := &repository.AssociationReadModel{ID: uuid.New(), PersonID: personID, AssociateID: uuid.New(), Role: "witness", Version: 1}
	if err := store.SaveAssociation(ctx, domain.MainBranchID, assocSubject); err != nil {
		t.Fatalf("SaveAssociation subject: %v", err)
	}
	// Association where the deleted person is the associate (associate_id side).
	assocAssociate := &repository.AssociationReadModel{ID: uuid.New(), PersonID: uuid.New(), AssociateID: personID, Role: "godparent", Version: 1}
	if err := store.SaveAssociation(ctx, domain.MainBranchID, assocAssociate); err != nil {
		t.Fatalf("SaveAssociation associate: %v", err)
	}
	attr := &repository.AttributeReadModel{ID: uuid.New(), PersonID: personID, FactType: domain.FactPersonOccupation, Value: "Mathematician", Version: 1, CreatedAt: time.Now()}
	if err := store.SaveAttribute(ctx, domain.MainBranchID, attr); err != nil {
		t.Fatalf("SaveAttribute: %v", err)
	}
	burial := &repository.EventReadModel{ID: uuid.New(), OwnerType: "person", OwnerID: personID, FactType: domain.FactPersonBurial, Place: "Restland", Version: 1, CreatedAt: time.Now()}
	if err := store.SaveEvent(ctx, main, burial); err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	// A bystander's life event must survive the cascade.
	bystander := &repository.EventReadModel{ID: uuid.New(), OwnerType: "person", OwnerID: uuid.New(), FactType: domain.FactPersonBurial, Place: "Restland", Version: 1, CreatedAt: time.Now()}
	if err := store.SaveEvent(ctx, main, bystander); err != nil {
		t.Fatalf("SaveEvent bystander: %v", err)
	}

	if err := store.DeletePerson(ctx, main, personID); err != nil {
		t.Fatalf("DeletePerson: %v", err)
	}

	// No dependent row may survive as an orphan. Every read is error-checked so a
	// failing query cannot make an assertion pass vacuously on an empty result.
	names, err := store.GetPersonNames(ctx, main, personID)
	if err != nil {
		t.Fatalf("GetPersonNames: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("person_names not cascaded: got %d", len(names))
	}
	ids, err := store.GetPersonExternalIDs(ctx, main, personID)
	if err != nil {
		t.Fatalf("GetPersonExternalIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("person_external_ids not cascaded: got %d", len(ids))
	}
	edge, err := store.GetPedigreeEdge(ctx, main, personID)
	if err != nil {
		t.Fatalf("GetPedigreeEdge: %v", err)
	}
	if edge != nil {
		t.Errorf("pedigree_edges not cascaded: got %+v", edge)
	}
	kids, err := store.GetFamilyChildren(ctx, main, childFamilyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren: %v", err)
	}
	for _, k := range kids {
		if k.PersonID == personID {
			t.Errorf("family_children not cascaded: deleted person still a child of %s", childFamilyID)
		}
	}
	a, err := store.GetAssociation(ctx, domain.MainBranchID, assocSubject.ID)
	if err != nil {
		t.Fatalf("GetAssociation subject: %v", err)
	}
	if a != nil {
		t.Errorf("association (person_id side) not cascaded: got %+v", a)
	}
	a2, err := store.GetAssociation(ctx, domain.MainBranchID, assocAssociate.ID)
	if err != nil {
		t.Fatalf("GetAssociation associate: %v", err)
	}
	if a2 != nil {
		t.Errorf("association (associate_id side) not cascaded: got %+v", a2)
	}
	listed, err := store.ListAssociationsForPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("ListAssociationsForPerson: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("associations still listed for person: got %d", len(listed))
	}
	at, err := store.GetAttribute(ctx, domain.MainBranchID, attr.ID)
	if err != nil {
		t.Fatalf("GetAttribute: %v", err)
	}
	if at != nil {
		t.Errorf("attributes not cascaded: got %+v", at)
	}
	ev, err := store.GetEvent(ctx, main, burial.ID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if ev != nil {
		t.Errorf("life_events not cascaded: got %+v", ev)
	}
	kept, err := store.GetEvent(ctx, main, bystander.ID)
	if err != nil {
		t.Fatalf("GetEvent bystander: %v", err)
	}
	if kept == nil {
		t.Error("cascade removed another person's life event")
	}
	cemeteries, err := store.GetCemeteryIndex(ctx, main)
	if err != nil {
		t.Fatalf("GetCemeteryIndex: %v", err)
	}
	if len(cemeteries) != 1 || cemeteries[0].Count != 1 {
		t.Errorf("cemetery index still counts the deleted person: got %+v", cemeteries)
	}
}

// TestReadModelStore_DeleteFamilyCascade verifies that deleting a family on main
// removes its dependents: family_external_ids, family_children and the family's
// own life events (#757). (pedigree_edges is keyed by person, not family, so it is
// deliberately not asserted here.)
func TestReadModelStore_DeleteFamilyCascade(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	main := domain.MainBranchID
	familyID := uuid.New()

	if err := store.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: familyID, RelationshipType: domain.RelationMarriage, Version: 1}); err != nil {
		t.Fatalf("SaveFamily: %v", err)
	}
	if err := store.ReplaceFamilyExternalIDs(ctx, main, familyID, []repository.FamilyExternalIDReadModel{{Value: "F-1"}}); err != nil {
		t.Fatalf("ReplaceFamilyExternalIDs: %v", err)
	}
	if err := store.SaveFamilyChild(ctx, main, &repository.FamilyChildReadModel{FamilyID: familyID, PersonID: uuid.New(), RelationshipType: domain.ChildBiological}); err != nil {
		t.Fatalf("SaveFamilyChild: %v", err)
	}
	marriage := &repository.EventReadModel{ID: uuid.New(), OwnerType: "family", OwnerID: familyID, FactType: domain.FactFamilyMarriage, Place: "Chapel", Version: 1, CreatedAt: time.Now()}
	if err := store.SaveEvent(ctx, main, marriage); err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}

	if err := store.DeleteFamily(ctx, main, familyID); err != nil {
		t.Fatalf("DeleteFamily: %v", err)
	}

	ids, err := store.GetFamilyExternalIDs(ctx, main, familyID)
	if err != nil {
		t.Fatalf("GetFamilyExternalIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("family_external_ids not cascaded: got %d", len(ids))
	}
	kids, err := store.GetFamilyChildren(ctx, main, familyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren: %v", err)
	}
	if len(kids) != 0 {
		t.Errorf("family_children not cascaded: got %d", len(kids))
	}
	events, err := store.ListEventsForFamily(ctx, main, familyID)
	if err != nil {
		t.Fatalf("ListEventsForFamily: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("family life_events not cascaded: got %d", len(events))
	}
}

// TestReadModelStore_BranchDeleteCascadesFacts verifies the branch half of the
// #757 cascade: deleting a person or family ON A BRANCH tombstones, on that branch
// only, every life event, attribute and association the owner has — including rows
// that exist only on main and rows the branch itself added — while main keeps all of
// them. The assertions are byte-for-byte identical across backends (DB-001).
func TestReadModelStore_BranchDeleteCascadesFacts(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	main := domain.MainBranchID
	branch := domain.BranchID(uuid.New())
	personID, otherID, familyID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	for _, p := range []*repository.PersonReadModel{
		{ID: personID, GivenName: "Ada", Surname: "Lovelace", Version: 1},
		{ID: otherID, GivenName: "Sam", Surname: "Steady", Version: 1},
	} {
		if err := store.SavePerson(ctx, main, p); err != nil {
			t.Fatalf("SavePerson: %v", err)
		}
	}
	if err := store.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: familyID, Partner1ID: &personID, RelationshipType: domain.RelationMarriage, Version: 1}); err != nil {
		t.Fatalf("SaveFamily: %v", err)
	}
	mainEvent := &repository.EventReadModel{ID: uuid.New(), OwnerType: "person", OwnerID: personID, FactType: domain.FactPersonBurial, Place: "Restland", Version: 1, CreatedAt: now}
	familyEvent := &repository.EventReadModel{ID: uuid.New(), OwnerType: "family", OwnerID: familyID, FactType: domain.FactFamilyMarriage, Place: "Chapel", Version: 1, CreatedAt: now}
	for _, e := range []*repository.EventReadModel{mainEvent, familyEvent} {
		if err := store.SaveEvent(ctx, main, e); err != nil {
			t.Fatalf("SaveEvent main: %v", err)
		}
	}
	attr := &repository.AttributeReadModel{ID: uuid.New(), PersonID: personID, FactType: domain.FactPersonOccupation, Value: "Mathematician", Version: 1, CreatedAt: now}
	if err := store.SaveAttribute(ctx, main, attr); err != nil {
		t.Fatalf("SaveAttribute main: %v", err)
	}
	assoc := &repository.AssociationReadModel{ID: uuid.New(), PersonID: otherID, AssociateID: personID, Role: "witness", Version: 1, UpdatedAt: now}
	if err := store.SaveAssociation(ctx, main, assoc); err != nil {
		t.Fatalf("SaveAssociation main: %v", err)
	}
	// A branch-only life event for the person, and a branch shadow of the main one.
	branchEvent := &repository.EventReadModel{ID: uuid.New(), OwnerType: "person", OwnerID: personID, FactType: domain.FactPersonBaptism, Place: "Font", Version: 1, CreatedAt: now}
	if err := store.SaveEvent(ctx, branch, branchEvent); err != nil {
		t.Fatalf("SaveEvent branch: %v", err)
	}
	shadow := *mainEvent
	shadow.Place = "Westland"
	if err := store.SaveEvent(ctx, branch, &shadow); err != nil {
		t.Fatalf("SaveEvent branch shadow: %v", err)
	}

	if err := store.DeletePerson(ctx, branch, personID); err != nil {
		t.Fatalf("DeletePerson branch: %v", err)
	}
	if err := store.DeleteFamily(ctx, branch, familyID); err != nil {
		t.Fatalf("DeleteFamily branch: %v", err)
	}

	// The branch sees none of the owners' facts.
	branchEvents, err := store.ListEventsForPerson(ctx, branch, personID)
	if err != nil {
		t.Fatalf("ListEventsForPerson branch: %v", err)
	}
	if len(branchEvents) != 0 {
		t.Errorf("branch person life events not tombstoned: got %+v", branchEvents)
	}
	branchFamilyEvents, err := store.ListEventsForFamily(ctx, branch, familyID)
	if err != nil {
		t.Fatalf("ListEventsForFamily branch: %v", err)
	}
	if len(branchFamilyEvents) != 0 {
		t.Errorf("branch family life events not tombstoned: got %+v", branchFamilyEvents)
	}
	for _, id := range []uuid.UUID{mainEvent.ID, familyEvent.ID, branchEvent.ID} {
		got, err := store.GetEvent(ctx, branch, id)
		if err != nil {
			t.Fatalf("GetEvent branch: %v", err)
		}
		if got != nil {
			t.Errorf("GetEvent(branch, %s) = %+v, want tombstoned", id, got)
		}
	}
	branchAttr, err := store.GetAttribute(ctx, branch, attr.ID)
	if err != nil {
		t.Fatalf("GetAttribute branch: %v", err)
	}
	if branchAttr != nil {
		t.Errorf("branch attribute not tombstoned: got %+v", branchAttr)
	}
	branchAssocs, err := store.ListAssociationsForPerson(ctx, branch, otherID)
	if err != nil {
		t.Fatalf("ListAssociationsForPerson branch: %v", err)
	}
	if len(branchAssocs) != 0 {
		t.Errorf("branch association (associate_id side) not tombstoned: got %+v", branchAssocs)
	}
	branchCemeteries, err := store.GetCemeteryIndex(ctx, branch)
	if err != nil {
		t.Fatalf("GetCemeteryIndex branch: %v", err)
	}
	if len(branchCemeteries) != 0 {
		t.Errorf("branch cemetery index still counts the deleted person: got %+v", branchCemeteries)
	}

	// Main keeps every row, with main's values.
	mainEvents, err := store.ListEventsForPerson(ctx, main, personID)
	if err != nil {
		t.Fatalf("ListEventsForPerson main: %v", err)
	}
	if len(mainEvents) != 1 || mainEvents[0].Place != "Restland" {
		t.Errorf("main person life events changed by a branch delete: got %+v", mainEvents)
	}
	mainFamilyEvents, err := store.ListEventsForFamily(ctx, main, familyID)
	if err != nil {
		t.Fatalf("ListEventsForFamily main: %v", err)
	}
	if len(mainFamilyEvents) != 1 {
		t.Errorf("main family life events changed by a branch delete: got %+v", mainFamilyEvents)
	}
	mainAttr, err := store.GetAttribute(ctx, main, attr.ID)
	if err != nil {
		t.Fatalf("GetAttribute main: %v", err)
	}
	if mainAttr == nil || mainAttr.Value != "Mathematician" {
		t.Errorf("main attribute changed by a branch delete: got %+v", mainAttr)
	}
	mainAssocs, err := store.ListAssociationsForPerson(ctx, main, otherID)
	if err != nil {
		t.Fatalf("ListAssociationsForPerson main: %v", err)
	}
	if len(mainAssocs) != 1 {
		t.Errorf("main association changed by a branch delete: got %+v", mainAssocs)
	}
	mainCemeteries, err := store.GetCemeteryIndex(ctx, main)
	if err != nil {
		t.Fatalf("GetCemeteryIndex main: %v", err)
	}
	if len(mainCemeteries) != 1 || mainCemeteries[0].Place != "Restland" || mainCemeteries[0].Count != 1 {
		t.Errorf("main cemetery index changed by a branch delete: got %+v", mainCemeteries)
	}
}

// TestReadModelStore_DeleteSourceCascade verifies the #758 source cascade that
// replaces the dropped sources(id) foreign keys: deleting a source removes (main)
// or tombstones (branch) its external identifiers and every citation of it on
// that same branch, and never touches another branch's rows. The assertions are
// byte-for-byte identical across the memory/sqlite/postgres backends (DB-001).
func TestReadModelStore_DeleteSourceCascade(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	main := domain.MainBranchID
	branch := domain.BranchID(uuid.New())
	otherBranch := domain.BranchID(uuid.New())
	sourceID, keptSourceID, ownerID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	for _, src := range []*repository.SourceReadModel{
		{ID: sourceID, SourceType: domain.SourceCensus, Title: "Census 1880", CitationCount: 1, Version: 1, UpdatedAt: now},
		{ID: keptSourceID, SourceType: domain.SourceBook, Title: "Family Bible", CitationCount: 1, Version: 1, UpdatedAt: now},
	} {
		if err := store.SaveSource(ctx, main, src); err != nil {
			t.Fatalf("SaveSource main: %v", err)
		}
	}
	if err := store.ReplaceSourceExternalIDs(ctx, main, sourceID, []repository.SourceExternalIDReadModel{{Value: "MAIN-1", Type: "http://example.org/ids"}}); err != nil {
		t.Fatalf("ReplaceSourceExternalIDs main: %v", err)
	}
	mainCite := &repository.CitationReadModel{ID: uuid.New(), SourceID: sourceID, SourceTitle: "Census 1880", FactType: domain.FactPersonBirth, FactOwnerID: ownerID, Version: 1, CreatedAt: now}
	keptCite := &repository.CitationReadModel{ID: uuid.New(), SourceID: keptSourceID, SourceTitle: "Family Bible", FactType: domain.FactPersonDeath, FactOwnerID: ownerID, Version: 1, CreatedAt: now}
	for _, c := range []*repository.CitationReadModel{mainCite, keptCite} {
		if err := store.SaveCitation(ctx, main, c); err != nil {
			t.Fatalf("SaveCitation main: %v", err)
		}
	}
	// A branch-only citation of the source, and one on a sibling branch that the
	// cascade must not touch.
	branchCite := &repository.CitationReadModel{ID: uuid.New(), SourceID: sourceID, SourceTitle: "Census 1880", FactType: domain.FactPersonBurial, FactOwnerID: ownerID, Version: 1, CreatedAt: now}
	if err := store.SaveCitation(ctx, branch, branchCite); err != nil {
		t.Fatalf("SaveCitation branch: %v", err)
	}
	siblingCite := &repository.CitationReadModel{ID: uuid.New(), SourceID: sourceID, SourceTitle: "Census 1880", FactType: domain.FactPersonBaptism, FactOwnerID: ownerID, Version: 1, CreatedAt: now}
	if err := store.SaveCitation(ctx, otherBranch, siblingCite); err != nil {
		t.Fatalf("SaveCitation sibling branch: %v", err)
	}

	// --- Branch delete: tombstones the source, its external IDs and every citation
	// of it the branch sees, on that branch only. ---
	if err := store.DeleteSource(ctx, branch, sourceID); err != nil {
		t.Fatalf("DeleteSource branch: %v", err)
	}
	if got, err := store.GetSource(ctx, branch, sourceID); err != nil || got != nil {
		t.Errorf("branch GetSource after delete = %+v (err=%v), want tombstoned", got, err)
	}
	if got, err := store.GetSourceExternalIDs(ctx, branch, sourceID); err != nil || len(got) != 0 {
		t.Errorf("branch source external ids not tombstoned: got %+v (err=%v)", got, err)
	}
	if got, err := store.GetCitationsForSource(ctx, branch, sourceID); err != nil || len(got) != 0 {
		t.Errorf("branch citations of the source not tombstoned: got %+v (err=%v)", got, err)
	}
	for _, id := range []uuid.UUID{mainCite.ID, branchCite.ID} {
		if got, err := store.GetCitation(ctx, branch, id); err != nil || got != nil {
			t.Errorf("branch GetCitation(%s) = %+v (err=%v), want tombstoned", id, got, err)
		}
	}
	if got, err := store.GetCitation(ctx, branch, keptCite.ID); err != nil || got == nil {
		t.Errorf("branch cascade removed another source's citation: got %+v (err=%v)", got, err)
	}
	// Main keeps everything.
	if got, err := store.GetSource(ctx, main, sourceID); err != nil || got == nil {
		t.Errorf("main GetSource after branch delete = %+v (err=%v), want kept", got, err)
	}
	if got, err := store.GetSourceExternalIDs(ctx, main, sourceID); err != nil || len(got) != 1 {
		t.Errorf("main source external ids after branch delete = %+v (err=%v), want 1", got, err)
	}
	if got, err := store.GetCitationsForSource(ctx, main, sourceID); err != nil || len(got) != 1 || got[0].ID != mainCite.ID {
		t.Errorf("main citations of the source after branch delete = %+v (err=%v), want [main]", got, err)
	}
	// The sibling branch is untouched.
	if got, err := store.GetCitationsForSource(ctx, otherBranch, sourceID); err != nil || len(got) != 2 {
		t.Errorf("sibling branch citations of the source = %d rows (err=%v), want 2 (main's + its own)", len(got), err)
	}
	if got, err := store.GetSource(ctx, otherBranch, sourceID); err != nil || got == nil {
		t.Errorf("sibling branch GetSource = %+v (err=%v), want main's row", got, err)
	}

	// Deleting a citation on the branch never touches another branch's rows.
	if err := store.DeleteCitation(ctx, branch, siblingCite.ID); err != nil {
		t.Fatalf("DeleteCitation branch (sibling's id): %v", err)
	}
	if got, err := store.GetCitation(ctx, otherBranch, siblingCite.ID); err != nil || got == nil {
		t.Errorf("branch DeleteCitation reached the sibling branch: got %+v (err=%v)", got, err)
	}

	// --- Main delete: removes the source, its external IDs and its main
	// citations; the other source and its citation survive. ---
	if err := store.DeleteSource(ctx, main, sourceID); err != nil {
		t.Fatalf("DeleteSource main: %v", err)
	}
	if got, err := store.GetSource(ctx, main, sourceID); err != nil || got != nil {
		t.Errorf("main GetSource after delete = %+v (err=%v), want gone", got, err)
	}
	if got, err := store.GetSourceExternalIDs(ctx, main, sourceID); err != nil || len(got) != 0 {
		t.Errorf("main source external ids not cascaded: got %+v (err=%v)", got, err)
	}
	if got, err := store.GetCitation(ctx, main, mainCite.ID); err != nil || got != nil {
		t.Errorf("main citation of the source not cascaded: got %+v (err=%v)", got, err)
	}
	if got, err := store.GetCitation(ctx, main, keptCite.ID); err != nil || got == nil {
		t.Errorf("main cascade removed another source's citation: got %+v (err=%v)", got, err)
	}
	if _, total, err := store.ListSources(ctx, repository.ListOptions{Limit: 10}); err != nil || total != 1 {
		t.Errorf("main ListSources total after delete = %d (err=%v), want 1", total, err)
	}
	// The sibling branch's own citation of the deleted source is its own row:
	// main's cascade leaves other branches' rows alone.
	if got, err := store.GetCitation(ctx, otherBranch, siblingCite.ID); err != nil || got == nil {
		t.Errorf("main DeleteSource reached the sibling branch's citation: got %+v (err=%v)", got, err)
	}
}

// TestReadModelStore_DeleteCascadesMedia verifies the #759 media cascade:
// media has no foreign key to its owner, so DeletePerson, DeleteFamily and
// DeleteSource delete (main) or tombstone (branch) the owner's media by hand, on
// that branch only. A branch cascade never touches main's rows or bytes, and a
// main cascade keeps the bytes a sibling branch's live shadow still borrows.
// The assertions are byte-for-byte identical across the memory/sqlite/postgres
// backends (DB-001).
func TestReadModelStore_DeleteCascadesMedia(t *testing.T) {
	store, cleanup := newMediaCascadeStore(t)
	defer cleanup()
	ctx := context.Background()

	main := domain.MainBranchID
	now := time.Now()
	newMedia := func(entityType string, owner uuid.UUID, bytes string) *repository.MediaReadModel {
		return &repository.MediaReadModel{
			ID: uuid.New(), EntityType: entityType, EntityID: owner, Title: entityType + " media",
			MimeType: "image/jpeg", MediaType: domain.MediaPhoto, Filename: "m.jpg",
			FileSize: int64(len(bytes)), FileData: []byte(bytes), ThumbnailData: []byte(bytes + "-thumb"),
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	for _, tc := range []struct {
		entityType string
		del        func(branchID domain.BranchID, id uuid.UUID) error
	}{
		{"person", func(b domain.BranchID, id uuid.UUID) error { return store.DeletePerson(ctx, b, id) }},
		{"family", func(b domain.BranchID, id uuid.UUID) error { return store.DeleteFamily(ctx, b, id) }},
		{"source", func(b domain.BranchID, id uuid.UUID) error { return store.DeleteSource(ctx, b, id) }},
	} {
		t.Run(tc.entityType, func(t *testing.T) {
			branch := domain.BranchID(uuid.New())
			sibling := domain.BranchID(uuid.New())
			owner, otherOwner := uuid.New(), uuid.New()

			owned := newMedia(tc.entityType, owner, "OWNED")
			kept := newMedia(tc.entityType, otherOwner, "KEPT")
			for _, m := range []*repository.MediaReadModel{owned, kept} {
				if err := store.SaveMedia(ctx, main, m); err != nil {
					t.Fatalf("SaveMedia main: %v", err)
				}
			}
			branchOnly := newMedia(tc.entityType, owner, "BRANCH-ONLY")
			if err := store.SaveMedia(ctx, branch, branchOnly); err != nil {
				t.Fatalf("SaveMedia branch: %v", err)
			}
			// The sibling keeps a live metadata shadow of the owned item.
			shadow := *owned
			shadow.FileData, shadow.ThumbnailData = nil, nil
			shadow.Title = "sibling reading"
			if err := store.SaveMedia(ctx, sibling, &shadow); err != nil {
				t.Fatalf("SaveMedia sibling: %v", err)
			}

			// --- Branch delete: the owner's media (main's and the branch's own)
			// is tombstoned on the branch only. ---
			if err := tc.del(branch, owner); err != nil {
				t.Fatalf("branch delete: %v", err)
			}
			for _, id := range []uuid.UUID{owned.ID, branchOnly.ID} {
				if got, err := store.GetMediaWithData(ctx, branch, id); err != nil || got != nil {
					t.Errorf("branch GetMediaWithData(%s) after cascade = %+v (err=%v), want tombstoned", id, got, err)
				}
			}
			if got, _, err := store.ListMediaForEntity(ctx, tc.entityType, owner, repository.ListOptions{Limit: 10, BranchID: branch}); err != nil || len(got) != 0 {
				t.Errorf("branch ListMediaForEntity(owner) after cascade = %d items (err=%v), want 0", len(got), err)
			}
			if got, err := store.GetMedia(ctx, branch, kept.ID); err != nil || got == nil {
				t.Errorf("branch cascade removed another owner's media: %+v (err=%v)", got, err)
			}
			if got, err := store.GetMediaWithData(ctx, main, owned.ID); err != nil || got == nil || string(got.FileData) != "OWNED" {
				t.Errorf("main GetMediaWithData(owned) after branch cascade = %+v (err=%v), want it and its bytes", got, err)
			}
			if got, err := store.GetMediaWithData(ctx, sibling, owned.ID); err != nil || got == nil || got.Title != "sibling reading" || string(got.FileData) != "OWNED" {
				t.Errorf("sibling GetMediaWithData(owned) after branch cascade = %+v (err=%v), want its shadow over main's bytes", got, err)
			}

			// --- Main delete: main's media for the owner goes, but the sibling's
			// live shadow keeps reading the shared bytes. ---
			if err := tc.del(main, owner); err != nil {
				t.Fatalf("main delete: %v", err)
			}
			if got, err := store.GetMedia(ctx, main, owned.ID); err != nil || got != nil {
				t.Errorf("main GetMedia(owned) after cascade = %+v (err=%v), want gone", got, err)
			}
			if got, _, err := store.ListMediaForEntity(ctx, tc.entityType, owner, repository.ListOptions{Limit: 10}); err != nil || len(got) != 0 {
				t.Errorf("main ListMediaForEntity(owner) after cascade = %d items (err=%v), want 0", len(got), err)
			}
			if got, err := store.GetMediaWithData(ctx, main, kept.ID); err != nil || got == nil || string(got.FileData) != "KEPT" {
				t.Errorf("main cascade removed another owner's media: %+v (err=%v)", got, err)
			}
			if got, err := store.GetMediaWithData(ctx, sibling, owned.ID); err != nil || got == nil || string(got.FileData) != "OWNED" || string(got.ThumbnailData) != "OWNED-thumb" {
				t.Errorf("sibling GetMediaWithData(owned) after main cascade = %+v (err=%v), want the shared bytes kept", got, err)
			}

			// Once the sibling deletes its shadow too, nothing is left anywhere.
			if err := store.DeleteMedia(ctx, sibling, owned.ID); err != nil {
				t.Fatalf("sibling DeleteMedia: %v", err)
			}
			if got, err := store.GetMediaWithData(ctx, sibling, owned.ID); err != nil || got != nil {
				t.Errorf("sibling GetMediaWithData(owned) after its delete = %+v (err=%v), want gone", got, err)
			}
		})
	}
}

// newMediaCascadeStore returns this backend's store for the media cascade test.
func newMediaCascadeStore(t *testing.T) (repository.ReadModelStore, func()) {
	t.Helper()
	return setupTestReadModelDB(t)
}

// TestReadModelStore_DeleteCascadesGPS verifies that DeletePerson and
// DeleteFamily cascade to every GPS artifact about the deleted subject (#760):
// on a branch each artifact the branch sees (main's and its own) is tombstoned
// on that branch only, and on main main's artifacts are deleted. Another
// subject's artifacts and a sibling branch's own rows are never touched. The
// body is identical across the memory/sqlite/postgres backends (DB-001).
func TestReadModelStore_DeleteCascadesGPS(t *testing.T) {
	store, cleanup := newGPSCascadeStore(t)
	defer cleanup()
	ctx := context.Background()

	main := domain.MainBranchID
	now := time.Now().UTC().Truncate(time.Second)

	for _, tc := range []struct {
		name string
		del  func(branchID domain.BranchID, id uuid.UUID) error
	}{
		{"person", func(b domain.BranchID, id uuid.UUID) error { return store.DeletePerson(ctx, b, id) }},
		{"family", func(b domain.BranchID, id uuid.UUID) error { return store.DeleteFamily(ctx, b, id) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			branch := domain.BranchID(uuid.New())
			sibling := domain.BranchID(uuid.New())
			subject, other := uuid.New(), uuid.New()

			analysis := &repository.EvidenceAnalysisReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: subject,
				Conclusion: "Born 1815", Version: 1, CreatedAt: now, UpdatedAt: now}
			otherAnalysis := &repository.EvidenceAnalysisReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: other,
				Conclusion: "Born 1820", Version: 1, CreatedAt: now, UpdatedAt: now}
			conflict := &repository.EvidenceConflictReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: subject,
				Description: "Disagreement", Status: domain.ConflictStatusOpen, Version: 1, CreatedAt: now, UpdatedAt: now}
			log := &repository.ResearchLogReadModel{ID: uuid.New(), SubjectID: subject, SubjectType: tc.name, Repository: "Archive",
				SearchDescription: "Baptisms", Outcome: domain.ResearchOutcomeNotFound, SearchDate: now, Version: 1, CreatedAt: now, UpdatedAt: now}
			proof := &repository.ProofSummaryReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: subject,
				Conclusion: "Born 1815", Argument: "Census", Version: 1, CreatedAt: now, UpdatedAt: now}
			branchProof := &repository.ProofSummaryReadModel{ID: uuid.New(), FactType: domain.FactPersonDeath, SubjectID: subject,
				Conclusion: "Died 1852", Argument: "Burial register", Version: 1, CreatedAt: now, UpdatedAt: now}

			for _, save := range []error{
				store.SaveEvidenceAnalysis(ctx, main, analysis),
				store.SaveEvidenceAnalysis(ctx, main, otherAnalysis),
				store.SaveEvidenceConflict(ctx, main, conflict),
				store.SaveResearchLog(ctx, main, log),
				store.SaveProofSummary(ctx, main, proof),
				store.SaveProofSummary(ctx, branch, branchProof),
			} {
				if save != nil {
					t.Fatalf("seed: %v", save)
				}
			}
			// The sibling keeps its own edit of the subject's research log.
			siblingLog := *log
			siblingLog.Notes = "sibling reading"
			if err := store.SaveResearchLog(ctx, sibling, &siblingLog); err != nil {
				t.Fatalf("SaveResearchLog sibling: %v", err)
			}

			visible := func(b domain.BranchID) (analyses, conflicts, logs, proofs int) {
				t.Helper()
				a, err := store.GetAnalysesBySubject(ctx, b, subject)
				if err != nil {
					t.Fatalf("GetAnalysesBySubject: %v", err)
				}
				c, err := store.GetConflictsForSubject(ctx, b, subject)
				if err != nil {
					t.Fatalf("GetConflictsForSubject: %v", err)
				}
				l, err := store.GetResearchLogsForSubject(ctx, b, subject)
				if err != nil {
					t.Fatalf("GetResearchLogsForSubject: %v", err)
				}
				p, err := store.GetProofSummariesBySubject(ctx, b, subject)
				if err != nil {
					t.Fatalf("GetProofSummariesBySubject: %v", err)
				}
				return len(a), len(c), len(l), len(p)
			}
			otherKept := func(label string, b domain.BranchID) {
				t.Helper()
				if got, err := store.GetEvidenceAnalysis(ctx, b, otherAnalysis.ID); err != nil || got == nil {
					t.Errorf("%s: cascade removed another subject's analysis: %+v (err=%v)", label, got, err)
				}
			}

			// --- Branch delete: every artifact the branch sees about the subject
			// is tombstoned on the branch only. ---
			if err := tc.del(branch, subject); err != nil {
				t.Fatalf("branch delete: %v", err)
			}
			if a, c, l, p := visible(branch); a+c+l+p != 0 {
				t.Errorf("branch artifacts after cascade = %d/%d/%d/%d, want none", a, c, l, p)
			}
			if got, err := store.GetProofSummary(ctx, branch, branchProof.ID); err != nil || got != nil {
				t.Errorf("branch GetProofSummary(branch-only) after cascade = %+v (err=%v), want tombstoned", got, err)
			}
			if got, err := store.ListUnresolvedConflicts(ctx, branch); err != nil || len(got) != 0 {
				t.Errorf("branch ListUnresolvedConflicts after cascade = %d (err=%v), want 0", len(got), err)
			}
			otherKept("branch", branch)
			if a, c, l, p := visible(main); a != 1 || c != 1 || l != 1 || p != 1 {
				t.Errorf("main artifacts after branch cascade = %d/%d/%d/%d, want 1/1/1/1", a, c, l, p)
			}
			if got, err := store.GetResearchLog(ctx, sibling, log.ID); err != nil || got == nil || got.Notes != "sibling reading" {
				t.Errorf("sibling GetResearchLog after branch cascade = %+v (err=%v), want its own edit", got, err)
			}

			// --- Main delete: main's artifacts about the subject go; another
			// subject's stay, and the sibling's own row is not touched. ---
			if err := tc.del(main, subject); err != nil {
				t.Fatalf("main delete: %v", err)
			}
			if a, c, l, p := visible(main); a+c+l+p != 0 {
				t.Errorf("main artifacts after cascade = %d/%d/%d/%d, want none", a, c, l, p)
			}
			if got, err := store.ListUnresolvedConflicts(ctx, main); err != nil || len(got) != 0 {
				t.Errorf("main ListUnresolvedConflicts after cascade = %d (err=%v), want 0", len(got), err)
			}
			otherKept("main", main)
			if got, err := store.GetResearchLog(ctx, sibling, log.ID); err != nil || got == nil || got.Notes != "sibling reading" {
				t.Errorf("sibling GetResearchLog after main cascade = %+v (err=%v), want its own edit", got, err)
			}
		})
	}
}

// newGPSCascadeStore returns this backend's store for the GPS cascade test.
func newGPSCascadeStore(t *testing.T) (repository.ReadModelStore, func()) {
	t.Helper()
	return newMediaCascadeStore(t)
}
