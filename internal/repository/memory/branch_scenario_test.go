package memory_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestBranchScenario_EndToEnd drives the full ADR-005 branch lifecycle through the
// Projector against the in-memory backend: create a branch, seed main, edit and
// delete under a branch, then delete the branch. The SAME assertions run verbatim
// against the sqlite and postgres backends (DB-001) so all three prove identical
// overlay / tombstone / fallback / purge behavior. Fixtures use neutral placeholder
// names only (public repo — no real PII).
func TestBranchScenario_EndToEnd(t *testing.T) {
	readStore := memory.NewReadModelStore()
	branchStore := memory.NewBranchStore()
	runBranchScenario(t, readStore, branchStore)
}

// runBranchScenario is the backend-agnostic scenario body. Each backend package
// carries an identical copy (there is no shared test harness in this repo); keeping
// the assertions byte-identical is the DB-001 parity guarantee.
func runBranchScenario(t *testing.T, readStore repository.ReadModelStore, branchStore repository.BranchStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, branchStore)

	// --- Step 1: BranchCreated -> branch appears in the registry as active. ---
	branch, err := domain.NewBranch("research-line", "exploring an alternate lineage", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	if err := projector.Project(ctx, domain.NewBranchCreated(branch), 1, domain.MainBranchID); err != nil {
		t.Fatalf("project BranchCreated: %v", err)
	}
	reg, err := branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("registry Get after create: %v", err)
	}
	if reg.Status != domain.BranchStatusActive {
		t.Fatalf("registry status after create = %s, want active", reg.Status)
	}

	// --- Step 2: seed a Person + Family on main, then edit the person on the branch. ---
	edited := domain.NewPerson("Alex", "Original") // gets a branch-scoped edit below
	untouched := domain.NewPerson("Sam", "Steady") // stays main-only -> proves fallback
	family := domain.NewFamily()                   // never edited on the branch -> fallback
	for i, ev := range []domain.Event{
		domain.NewPersonCreated(edited),
		domain.NewPersonCreated(untouched),
		domain.NewFamilyCreated(family),
	} {
		if err := projector.Project(ctx, ev, int64(i+1), domain.MainBranchID); err != nil {
			t.Fatalf("seed main event %d: %v", i, err)
		}
	}
	// A Person edit projected under the branch id writes a branch-scoped row
	// (copy-on-write over main).
	if err := projector.Project(ctx, domain.NewPersonUpdated(edited.ID, map[string]any{"surname": "Revised"}), 4, domain.BranchID(branch.ID)); err != nil {
		t.Fatalf("project branch edit: %v", err)
	}

	// --- Step 3: branch query returns the branch row for edited entities and falls
	// back to main for untouched ones. ---
	if got, _ := readStore.GetPerson(ctx, domain.BranchID(branch.ID), edited.ID); got == nil || got.Surname != "Revised" {
		t.Fatalf("branch Get edited: want Revised, got %+v", got)
	}
	if got, _ := readStore.GetPerson(ctx, domain.BranchID(branch.ID), untouched.ID); got == nil || got.Surname != "Steady" {
		t.Fatalf("branch Get untouched (fallback): want Steady, got %+v", got)
	}
	if got, _ := readStore.GetFamily(ctx, domain.BranchID(branch.ID), family.ID); got == nil {
		t.Fatal("branch Get family (fallback): want main family, got nil")
	}
	// Main is unaffected by the branch edit.
	if got, _ := readStore.GetPerson(ctx, domain.MainBranchID, edited.ID); got == nil || got.Surname != "Original" {
		t.Fatalf("main Get edited: want Original (untouched), got %+v", got)
	}

	// --- Step 6 (structural non-N+1): a single ListPersons call resolves the whole
	// overlay list -- branch edit + main fallback -- rather than a per-entity Get
	// loop by the caller. Asserting the resolved contents come back from one store
	// call is the anti-N+1 guarantee. ---
	list, total, err := readStore.ListPersons(ctx, repository.ListOptions{Limit: 100, BranchID: domain.BranchID(branch.ID)})
	if err != nil {
		t.Fatalf("ListPersons branch: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("branch ListPersons: want 2 resolved in one call, got total=%d len=%d", total, len(list))
	}
	surnames := map[string]bool{}
	for _, p := range list {
		surnames[p.Surname] = true
	}
	if !surnames["Revised"] || !surnames["Steady"] {
		t.Fatalf("branch ListPersons overlay: want {Revised, Steady}, got %+v", surnames)
	}

	// --- Step 4: delete a Person on the branch -> tombstone hides it on the branch
	// while main still returns it. ---
	if err := projector.Project(ctx, domain.NewPersonDeleted(untouched.ID, "pruned on branch"), 5, domain.BranchID(branch.ID)); err != nil {
		t.Fatalf("project branch delete: %v", err)
	}
	if got, _ := readStore.GetPerson(ctx, domain.BranchID(branch.ID), untouched.ID); got != nil {
		t.Fatalf("branch Get after tombstone: want nil, got %+v", got)
	}
	if got, _ := readStore.GetPerson(ctx, domain.MainBranchID, untouched.ID); got == nil || got.Surname != "Steady" {
		t.Fatalf("main Get after branch tombstone: want Steady, got %+v", got)
	}
	if _, total, _ := readStore.ListPersons(ctx, repository.ListOptions{Limit: 100, BranchID: domain.BranchID(branch.ID)}); total != 1 {
		t.Fatalf("branch ListPersons after tombstone: want 1 (edited only), got %d", total)
	}

	// --- Step 5: BranchDeleted -> PurgeBranch drops the branch's overlay rows and the
	// registry is archived; branch queries revert to main. ---
	if err := projector.Project(ctx, domain.NewBranchDeleted(branch.ID), 6, domain.MainBranchID); err != nil {
		t.Fatalf("project BranchDeleted: %v", err)
	}
	reg, err = branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("registry Get after delete: %v", err)
	}
	if reg.Status != domain.BranchStatusArchived {
		t.Fatalf("registry status after delete = %s, want archived", reg.Status)
	}
	// Overlay purged: the branch edit is gone, the tombstone is gone, so both persons
	// resolve to their main rows again.
	if got, _ := readStore.GetPerson(ctx, domain.BranchID(branch.ID), edited.ID); got == nil || got.Surname != "Original" {
		t.Fatalf("branch Get edited after purge: want main fallback Original, got %+v", got)
	}
	if got, _ := readStore.GetPerson(ctx, domain.BranchID(branch.ID), untouched.ID); got == nil || got.Surname != "Steady" {
		t.Fatalf("branch Get untouched after purge: want main fallback Steady, got %+v", got)
	}
	if _, total, _ := readStore.ListPersons(ctx, repository.ListOptions{Limit: 100, BranchID: domain.BranchID(branch.ID)}); total != 2 {
		t.Fatalf("branch ListPersons after purge: want 2 (both fall back to main), got %d", total)
	}
	// Main is entirely intact throughout.
	if _, total, _ := readStore.ListPersons(ctx, repository.ListOptions{Limit: 100, BranchID: domain.MainBranchID}); total != 2 {
		t.Fatalf("main ListPersons after purge: want 2, got %d", total)
	}
}
