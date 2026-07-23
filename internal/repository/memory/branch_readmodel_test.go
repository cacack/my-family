package memory_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// These tests exercise the ADR-005 copy-on-write overlay + tombstone semantics
// of the in-memory ReadModelStore for the branch-scoped slice entities. The
// memory backend is the reference implementation the SQL backends mirror.

func personRM(id uuid.UUID, given, surname string) *repository.PersonReadModel {
	return &repository.PersonReadModel{ID: id, GivenName: given, Surname: surname, FullName: given + " " + surname, Version: 1}
}

// TestBranchOverlayPersonPrecedence: a branch row shadows the main row for the
// same id, while main keeps its own value.
func TestBranchOverlayPersonPrecedence(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, personRM(id, "Ada", "Main")); err != nil {
		t.Fatalf("SavePerson main: %v", err)
	}
	if err := store.SavePerson(ctx, branch, personRM(id, "Ada", "Branch")); err != nil {
		t.Fatalf("SavePerson branch: %v", err)
	}

	got, _ := store.GetPerson(ctx, branch, id)
	if got == nil || got.Surname != "Branch" {
		t.Fatalf("branch Get: want surname Branch, got %+v", got)
	}
	main, _ := store.GetPerson(ctx, domain.MainBranchID, id)
	if main == nil || main.Surname != "Main" {
		t.Fatalf("main Get: want surname Main, got %+v", main)
	}

	// List reflects the same precedence.
	branchList, total, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: branch})
	if total != 1 || len(branchList) != 1 || branchList[0].Surname != "Branch" {
		t.Fatalf("branch List: want 1 Branch, got total=%d list=%+v", total, branchList)
	}
	mainList, _, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: domain.MainBranchID})
	if len(mainList) != 1 || mainList[0].Surname != "Main" {
		t.Fatalf("main List: want 1 Main, got %+v", mainList)
	}
}

// TestBranchFallbackToMain: an entity the branch has not overridden resolves to
// the main row via both Get and List.
func TestBranchFallbackToMain(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, personRM(id, "Grace", "Hopper")); err != nil {
		t.Fatalf("SavePerson main: %v", err)
	}

	got, _ := store.GetPerson(ctx, branch, id)
	if got == nil || got.Surname != "Hopper" {
		t.Fatalf("branch Get fallback: want Hopper, got %+v", got)
	}
	branchList, total, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: branch})
	if total != 1 || len(branchList) != 1 || branchList[0].Surname != "Hopper" {
		t.Fatalf("branch List fallback: want 1 Hopper, got total=%d list=%+v", total, branchList)
	}
}

// TestBranchTombstoneSuppression: deleting on a non-main branch writes a
// tombstone that hides the main row for that branch, without touching main.
func TestBranchTombstoneSuppression(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, personRM(id, "Alan", "Turing")); err != nil {
		t.Fatalf("SavePerson main: %v", err)
	}
	if err := store.DeletePerson(ctx, branch, id); err != nil {
		t.Fatalf("DeletePerson branch: %v", err)
	}

	if got, _ := store.GetPerson(ctx, branch, id); got != nil {
		t.Fatalf("branch Get after tombstone: want nil, got %+v", got)
	}
	if main, _ := store.GetPerson(ctx, domain.MainBranchID, id); main == nil || main.Surname != "Turing" {
		t.Fatalf("main Get after branch tombstone: want Turing, got %+v", main)
	}
	branchList, total, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: branch})
	if total != 0 || len(branchList) != 0 {
		t.Fatalf("branch List after tombstone: want empty, got total=%d list=%+v", total, branchList)
	}
	mainList, _, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: domain.MainBranchID})
	if len(mainList) != 1 {
		t.Fatalf("main List after branch tombstone: want 1, got %+v", mainList)
	}
}

// TestBranchMainDeleteIsRealRemoval: a delete scoped to main is an actual
// removal (unchanged pre-branch behavior), not a tombstone.
func TestBranchMainDeleteIsRealRemoval(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, personRM(id, "Ada", "Lovelace")); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	if err := store.DeletePerson(ctx, domain.MainBranchID, id); err != nil {
		t.Fatalf("DeletePerson main: %v", err)
	}
	if got, _ := store.GetPerson(ctx, domain.MainBranchID, id); got != nil {
		t.Fatalf("main Get after main delete: want nil, got %+v", got)
	}
}

// TestBranchOnlyEntityInvisibleOnMain: an entity created only on a branch is not
// visible on main.
func TestBranchOnlyEntityInvisibleOnMain(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, branch, personRM(id, "Only", "Branch")); err != nil {
		t.Fatalf("SavePerson branch: %v", err)
	}
	if got, _ := store.GetPerson(ctx, domain.MainBranchID, id); got != nil {
		t.Fatalf("main Get of branch-only entity: want nil, got %+v", got)
	}
	if got, _ := store.GetPerson(ctx, branch, id); got == nil {
		t.Fatal("branch Get of branch-only entity: want present, got nil")
	}
	mainList, _, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: domain.MainBranchID})
	if len(mainList) != 0 {
		t.Fatalf("main List of branch-only entity: want empty, got %+v", mainList)
	}
}

// TestBranchSearchPersonsScope: SearchPersons resolves the overlay so a branch
// override is found by its branch value, and main by its main value.
func TestBranchSearchPersonsScope(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	_ = store.SavePerson(ctx, domain.MainBranchID, personRM(id, "Katherine", "Johnson"))
	_ = store.SavePerson(ctx, branch, personRM(id, "Katherine", "Coleman"))

	branchHits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Coleman", Limit: 10, BranchID: branch})
	if len(branchHits) != 1 || branchHits[0].Surname != "Coleman" {
		t.Fatalf("branch Search: want Coleman, got %+v", branchHits)
	}
	// The branch override should not surface under the main surname on the branch.
	if hits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Johnson", Limit: 10, BranchID: branch}); len(hits) != 0 {
		t.Fatalf("branch Search for main-only surname: want empty, got %+v", hits)
	}
	mainHits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Johnson", Limit: 10, BranchID: domain.MainBranchID})
	if len(mainHits) != 1 || mainHits[0].Surname != "Johnson" {
		t.Fatalf("main Search: want Johnson, got %+v", mainHits)
	}
}

// TestBranchPersonNamesCopyOnWrite: a branch edit to a person's name bucket
// forks main's bucket (copy-on-write) and leaves main untouched.
func TestBranchPersonNamesCopyOnWrite(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	mainName := &repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Main", Surname: "Name"}
	if err := store.SavePersonName(ctx, domain.MainBranchID, mainName); err != nil {
		t.Fatalf("SavePersonName main: %v", err)
	}

	// Adding a name on the branch seeds from main then appends.
	branchName := &repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Branch", Surname: "Name"}
	if err := store.SavePersonName(ctx, branch, branchName); err != nil {
		t.Fatalf("SavePersonName branch: %v", err)
	}

	branchNames, _ := store.GetPersonNames(ctx, branch, personID)
	if len(branchNames) != 2 {
		t.Fatalf("branch names: want 2 (COW seed + new), got %d: %+v", len(branchNames), branchNames)
	}
	mainNames, _ := store.GetPersonNames(ctx, domain.MainBranchID, personID)
	if len(mainNames) != 1 {
		t.Fatalf("main names after branch edit: want 1 (untouched), got %d", len(mainNames))
	}

	// GetPersonName resolves within the branch overlay.
	if got, _ := store.GetPersonName(ctx, branch, branchName.ID); got == nil {
		t.Fatal("GetPersonName branch: want the branch name, got nil")
	}
	if got, _ := store.GetPersonName(ctx, domain.MainBranchID, branchName.ID); got != nil {
		t.Fatalf("GetPersonName main: branch-only name must be invisible, got %+v", got)
	}
}

// TestBranchFamilyChildrenOverlay: branch children mutations copy-on-write over
// main; a branch child delete does not affect main.
func TestBranchFamilyChildrenOverlay(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	familyID := uuid.New()
	child1 := uuid.New()
	child2 := uuid.New()

	_ = store.SaveFamilyChild(ctx, domain.MainBranchID, &repository.FamilyChildReadModel{FamilyID: familyID, PersonID: child1})

	// Branch adds a second child (COW seeds child1 from main).
	_ = store.SaveFamilyChild(ctx, branch, &repository.FamilyChildReadModel{FamilyID: familyID, PersonID: child2})
	branchKids, _ := store.GetFamilyChildren(ctx, branch, familyID)
	if len(branchKids) != 2 {
		t.Fatalf("branch children: want 2, got %d: %+v", len(branchKids), branchKids)
	}
	mainKids, _ := store.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if len(mainKids) != 1 {
		t.Fatalf("main children after branch add: want 1, got %d", len(mainKids))
	}

	// Branch removes child1; main keeps it.
	_ = store.DeleteFamilyChild(ctx, branch, familyID, child1)
	branchKids, _ = store.GetFamilyChildren(ctx, branch, familyID)
	if len(branchKids) != 1 || branchKids[0].PersonID != child2 {
		t.Fatalf("branch children after delete: want [child2], got %+v", branchKids)
	}
	mainKids, _ = store.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if len(mainKids) != 1 || mainKids[0].PersonID != child1 {
		t.Fatalf("main children after branch delete: want [child1], got %+v", mainKids)
	}
}

// TestBranchPersonExternalIDsOverlayTombstone: a branch Replace shadows main; an
// empty Replace on the branch is a tombstone that hides main's identifiers.
func TestBranchPersonExternalIDsOverlayTombstone(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	_ = store.ReplacePersonExternalIDs(ctx, domain.MainBranchID, personID, []repository.PersonExternalIDReadModel{{Value: "MAIN-1"}})

	// Branch overrides with its own set.
	_ = store.ReplacePersonExternalIDs(ctx, branch, personID, []repository.PersonExternalIDReadModel{{Value: "BR-1"}, {Value: "BR-2"}})
	branchIDs, _ := store.GetPersonExternalIDs(ctx, branch, personID)
	if len(branchIDs) != 2 || branchIDs[0].Value != "BR-1" {
		t.Fatalf("branch ext ids: want [BR-1, BR-2], got %+v", branchIDs)
	}
	mainIDs, _ := store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID)
	if len(mainIDs) != 1 || mainIDs[0].Value != "MAIN-1" {
		t.Fatalf("main ext ids after branch override: want [MAIN-1], got %+v", mainIDs)
	}

	// Empty Replace on the branch is a tombstone: branch sees none, main intact.
	_ = store.ReplacePersonExternalIDs(ctx, branch, personID, nil)
	branchIDs, _ = store.GetPersonExternalIDs(ctx, branch, personID)
	if len(branchIDs) != 0 {
		t.Fatalf("branch ext ids after tombstone: want empty, got %+v", branchIDs)
	}
	mainIDs, _ = store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID)
	if len(mainIDs) != 1 {
		t.Fatalf("main ext ids after branch tombstone: want [MAIN-1], got %+v", mainIDs)
	}
}

// TestBranchPedigreeEdgeOverlayTombstone: pedigree edges follow the single-row
// overlay + tombstone contract.
func TestBranchPedigreeEdgeOverlayTombstone(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()
	mainFather := uuid.New()
	branchFather := uuid.New()

	_ = store.SavePedigreeEdge(ctx, domain.MainBranchID, &repository.PedigreeEdge{PersonID: personID, FatherID: &mainFather})

	// Fallback before any branch write.
	if e, _ := store.GetPedigreeEdge(ctx, branch, personID); e == nil || e.FatherID == nil || *e.FatherID != mainFather {
		t.Fatalf("branch edge fallback: want mainFather, got %+v", e)
	}

	// Branch override.
	_ = store.SavePedigreeEdge(ctx, branch, &repository.PedigreeEdge{PersonID: personID, FatherID: &branchFather})
	if e, _ := store.GetPedigreeEdge(ctx, branch, personID); e == nil || *e.FatherID != branchFather {
		t.Fatalf("branch edge override: want branchFather, got %+v", e)
	}
	if e, _ := store.GetPedigreeEdge(ctx, domain.MainBranchID, personID); e == nil || *e.FatherID != mainFather {
		t.Fatalf("main edge after branch override: want mainFather, got %+v", e)
	}

	// Branch tombstone hides the main edge for the branch only.
	_ = store.DeletePedigreeEdge(ctx, branch, personID)
	if e, _ := store.GetPedigreeEdge(ctx, branch, personID); e != nil {
		t.Fatalf("branch edge after tombstone: want nil, got %+v", e)
	}
	if e, _ := store.GetPedigreeEdge(ctx, domain.MainBranchID, personID); e == nil {
		t.Fatal("main edge after branch tombstone: want present, got nil")
	}
}

// TestBranchDeletePersonCascadesTombstones: deleting a person on a branch also
// tombstones its names and external IDs for that branch, while main keeps them.
func TestBranchDeletePersonCascadesTombstones(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	_ = store.SavePerson(ctx, domain.MainBranchID, personRM(personID, "Cascade", "Main"))
	_ = store.SavePersonName(ctx, domain.MainBranchID, &repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Cascade", Surname: "Main"})
	_ = store.ReplacePersonExternalIDs(ctx, domain.MainBranchID, personID, []repository.PersonExternalIDReadModel{{Value: "X"}})

	_ = store.DeletePerson(ctx, branch, personID)

	if names, _ := store.GetPersonNames(ctx, branch, personID); len(names) != 0 {
		t.Fatalf("branch names after cascade tombstone: want empty, got %+v", names)
	}
	if ids, _ := store.GetPersonExternalIDs(ctx, branch, personID); len(ids) != 0 {
		t.Fatalf("branch ext ids after cascade tombstone: want empty, got %+v", ids)
	}
	// Main retains everything.
	if names, _ := store.GetPersonNames(ctx, domain.MainBranchID, personID); len(names) != 1 {
		t.Fatalf("main names after branch cascade: want 1, got %+v", names)
	}
	if ids, _ := store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID); len(ids) != 1 {
		t.Fatalf("main ext ids after branch cascade: want 1, got %+v", ids)
	}
}
