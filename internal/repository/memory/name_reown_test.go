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

// TestSavePersonNameReownMovesName checks that saving a name under a new
// person (what PersonMerged does, #834) takes it out of its old owner's
// bucket, on main and on a branch, and that the branch re-own leaves main's
// bucket alone.
func TestSavePersonNameReownMovesName(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	branch := domain.BranchID(uuid.New())
	oldOwner, newOwner, mainOwner := uuid.New(), uuid.New(), uuid.New()

	onMain := &repository.PersonNameReadModel{ID: uuid.New(), PersonID: oldOwner, GivenName: "Ann", Surname: "Old"}
	if err := store.SavePersonName(ctx, domain.MainBranchID, onMain); err != nil {
		t.Fatalf("save on main: %v", err)
	}

	// Branch re-own of a mainline name.
	moved := *onMain
	moved.PersonID = newOwner
	if err := store.SavePersonName(ctx, branch, &moved); err != nil {
		t.Fatalf("re-own on branch: %v", err)
	}
	assertNameCount(t, store, branch, oldOwner, 0)
	assertNameCount(t, store, branch, newOwner, 1)
	assertNameCount(t, store, domain.MainBranchID, oldOwner, 1)
	assertNameCount(t, store, domain.MainBranchID, newOwner, 0)

	// Main re-own.
	onMainMoved := *onMain
	onMainMoved.PersonID = mainOwner
	if err := store.SavePersonName(ctx, domain.MainBranchID, &onMainMoved); err != nil {
		t.Fatalf("re-own on main: %v", err)
	}
	assertNameCount(t, store, domain.MainBranchID, oldOwner, 0)
	assertNameCount(t, store, domain.MainBranchID, mainOwner, 1)

	// A stale hint (the name deleted since) is harmless.
	if err := store.DeletePersonName(ctx, domain.MainBranchID, onMain.ID); err != nil {
		t.Fatalf("delete on main: %v", err)
	}
	if err := store.SavePersonName(ctx, domain.MainBranchID, onMain); err != nil {
		t.Fatalf("re-save on main: %v", err)
	}
	assertNameCount(t, store, domain.MainBranchID, oldOwner, 1)
	assertNameCount(t, store, domain.MainBranchID, mainOwner, 0)

	// Purging the branch drops its hints with its rows.
	if err := store.PurgeBranch(ctx, branch); err != nil {
		t.Fatalf("purge branch: %v", err)
	}
	assertNameCount(t, store, branch, newOwner, 0)
	assertNameCount(t, store, branch, oldOwner, 1)
}

// TestSavePersonNameScalesLinearly guards the name-save path the in-memory
// read model uses for every NameAdded (a GEDCOM import saves one per name):
// looking for a re-owned name's previous owner must not scan every person's
// bucket on each save. 20000 saves took well over a minute when it did, and
// take milliseconds when it does not; the bound is loose on purpose.
func TestSavePersonNameScalesLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("timing guard")
	}
	ctx := context.Background()
	store := memory.NewReadModelStore()
	start := time.Now()
	for i := range 20000 {
		name := &repository.PersonNameReadModel{ID: uuid.New(), PersonID: uuid.New(), GivenName: "Given", Surname: "Surname", IsPrimary: i%2 == 0}
		if err := store.SavePersonName(ctx, domain.MainBranchID, name); err != nil {
			t.Fatalf("save name %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("20000 name saves took %v; SavePersonName is no longer O(1) per save", elapsed)
	}
}

func assertNameCount(t *testing.T, store *memory.ReadModelStore, branch domain.BranchID, personID uuid.UUID, want int) {
	t.Helper()
	names, err := store.GetPersonNames(context.Background(), branch, personID)
	if err != nil {
		t.Fatalf("GetPersonNames: %v", err)
	}
	if len(names) != want {
		t.Errorf("GetPersonNames(%s, %s) = %d names, want %d", branch, personID, len(names), want)
	}
}
