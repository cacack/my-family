package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestReadModelStore_DeletePersonCascade verifies that deleting a person on main
// removes every dependent the pre-#669 ON DELETE CASCADE foreign keys used to
// clean up: person_names, person_external_ids, pedigree_edges, associations (both
// the person_id and associate_id sides) and attributes. The assertions are kept
// byte-for-byte identical across the memory/sqlite/postgres backends to enforce
// DB-001 parity and would have caught the incomplete/divergent manual cascade.
func TestReadModelStore_DeletePersonCascade(t *testing.T) {
	store := memory.NewReadModelStore()
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
	if err := store.SaveAssociation(ctx, assocSubject); err != nil {
		t.Fatalf("SaveAssociation subject: %v", err)
	}
	// Association where the deleted person is the associate (associate_id side).
	assocAssociate := &repository.AssociationReadModel{ID: uuid.New(), PersonID: uuid.New(), AssociateID: personID, Role: "godparent", Version: 1}
	if err := store.SaveAssociation(ctx, assocAssociate); err != nil {
		t.Fatalf("SaveAssociation associate: %v", err)
	}
	attr := &repository.AttributeReadModel{ID: uuid.New(), PersonID: personID, FactType: domain.FactPersonOccupation, Value: "Mathematician", Version: 1, CreatedAt: time.Now()}
	if err := store.SaveAttribute(ctx, attr); err != nil {
		t.Fatalf("SaveAttribute: %v", err)
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
	a, err := store.GetAssociation(ctx, assocSubject.ID)
	if err != nil {
		t.Fatalf("GetAssociation subject: %v", err)
	}
	if a != nil {
		t.Errorf("association (person_id side) not cascaded: got %+v", a)
	}
	a2, err := store.GetAssociation(ctx, assocAssociate.ID)
	if err != nil {
		t.Fatalf("GetAssociation associate: %v", err)
	}
	if a2 != nil {
		t.Errorf("association (associate_id side) not cascaded: got %+v", a2)
	}
	listed, err := store.ListAssociationsForPerson(ctx, personID)
	if err != nil {
		t.Fatalf("ListAssociationsForPerson: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("associations still listed for person: got %d", len(listed))
	}
	at, err := store.GetAttribute(ctx, attr.ID)
	if err != nil {
		t.Fatalf("GetAttribute: %v", err)
	}
	if at != nil {
		t.Errorf("attributes not cascaded: got %+v", at)
	}
}

// TestReadModelStore_DeleteFamilyCascade verifies that deleting a family on main
// removes its dependents: family_external_ids and family_children. (pedigree_edges
// is keyed by person, not family, so it is deliberately not asserted here.)
func TestReadModelStore_DeleteFamilyCascade(t *testing.T) {
	store := memory.NewReadModelStore()
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
}
