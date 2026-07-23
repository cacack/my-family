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

	// No dependent row may survive as an orphan.
	if names, _ := store.GetPersonNames(ctx, main, personID); len(names) != 0 {
		t.Errorf("person_names not cascaded: got %d", len(names))
	}
	if ids, _ := store.GetPersonExternalIDs(ctx, main, personID); len(ids) != 0 {
		t.Errorf("person_external_ids not cascaded: got %d", len(ids))
	}
	if edge, _ := store.GetPedigreeEdge(ctx, main, personID); edge != nil {
		t.Errorf("pedigree_edges not cascaded: got %+v", edge)
	}
	if a, _ := store.GetAssociation(ctx, assocSubject.ID); a != nil {
		t.Errorf("association (person_id side) not cascaded: got %+v", a)
	}
	if a, _ := store.GetAssociation(ctx, assocAssociate.ID); a != nil {
		t.Errorf("association (associate_id side) not cascaded: got %+v", a)
	}
	if got, _ := store.ListAssociationsForPerson(ctx, personID); len(got) != 0 {
		t.Errorf("associations still listed for person: got %d", len(got))
	}
	if at, _ := store.GetAttribute(ctx, attr.ID); at != nil {
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

	if ids, _ := store.GetFamilyExternalIDs(ctx, main, familyID); len(ids) != 0 {
		t.Errorf("family_external_ids not cascaded: got %d", len(ids))
	}
	if kids, _ := store.GetFamilyChildren(ctx, main, familyID); len(kids) != 0 {
		t.Errorf("family_children not cascaded: got %d", len(kids))
	}
}
