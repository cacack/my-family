package memory_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

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

// TestBranchScenario_AggregateIsolation drives the ADR-005 copy-on-write overlay
// through the browse and map aggregates (sub-issue A of #676, #756) against the
// in-memory backend: a mainline browse must be unaffected by the existence of branch
// shadow rows, a branch browse must see its own shadows instead of main's while
// omitting tombstoned people, and the main-only brick-wall writers must stay pinned to
// the main row. The scenario body (runBranchAggregateScenario) is an identical copy of
// the sqlite and postgres versions so all three backends prove the same behavior
// (DB-001). Fixtures use neutral placeholder names and invented places only (public
// repo — no real PII).
func TestBranchScenario_AggregateIsolation(t *testing.T) {
	readStore := memory.NewReadModelStore()
	branchStore := memory.NewBranchStore()
	runBranchAggregateScenario(t, readStore, branchStore)
}

// mainAggregateSnapshot is every mainline browse / map / cemetery / brick-wall read
// that runBranchAggregateScenario compares before and after branch rows exist.
type mainAggregateSnapshot struct {
	surnames    []repository.SurnameEntry
	letters     []repository.LetterCount
	byLetter    []repository.SurnameEntry
	places      []repository.PlaceEntry
	locations   []repository.MapLocation
	bySurname   []repository.PersonReadModel
	bySurnameN  int
	byPlace     []repository.PersonReadModel
	byPlaceN    int
	byCemetery  []repository.PersonReadModel
	byCemeteryN int
	brickWalls  []repository.BrickWallEntry
}

// runBranchAggregateScenario is the backend-agnostic aggregate-isolation scenario for
// sub-issue A of #676 (#756). Each backend package carries an identical copy (there is
// no shared test harness in this repo); keeping the assertions byte-identical is the
// DB-001 parity guarantee.
//
// Shape: two persons on main, both buried in the same cemetery, the first flagged as a
// brick wall. The first is then shadowed on a branch with a different surname and birth
// place (the shadow is copied from main, so it carries the brick-wall fields too); the
// second is tombstoned on the branch. Then:
//
//  1. every mainline browse / map / cemetery / brick-wall read is byte-for-byte what it
//     was before the branch rows existed -- the leak regression, and the point of #756;
//  2. the branch-scoped reads show the shadow's surname and place, not main's, and omit
//     the tombstoned person entirely;
//  3. GetPersonsByCemetery -- the one aggregate where a branch-scoped `persons` side
//     joins a mainline `life_events` side -- resolves the person through the overlay:
//     with real burial rows on both sides of the join, the branch sees the shadow's
//     surname and not the mainline spelling, and the tombstoned person disappears;
//  4. SetBrickWall / ResolveBrickWall are main-pinned: they leave the shadow alone.
//
// Deliberately NOT asserted: GetPlaceHierarchy below the top level. The memory backend
// ignores the `parent` argument and returns whole place strings where the SQL backends
// return hierarchy levels -- the pre-existing place-parsing divergence tracked as #763,
// not something #756 introduced. Place fixtures are single-component so the top-level
// call does agree on all three backends.
func runBranchAggregateScenario(t *testing.T, readStore repository.ReadModelStore, branchStore repository.BranchStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, branchStore)

	strPtr := func(s string) *string { return &s }
	now := time.Now().UTC().Truncate(time.Second)
	const cemetery = "Restland Cemetery"

	// --- Step 1: two persons on main, with distinct surnames, distinct places and
	// distinct coordinates so every aggregate can tell them apart. Seeded through
	// SavePerson rather than the projector because PersonCreated carries no
	// coordinates and the map aggregate needs them. ---
	shadowed := &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Robin", Surname: "Mainline", FullName: "Robin Mainline",
		BirthPlace:    "Northland",
		BirthPlaceLat: strPtr("N42.3601"), BirthPlaceLong: strPtr("W71.0589"),
		Version: 1, UpdatedAt: now,
	}
	tombstoned := &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Sky", Surname: "Steadfast", FullName: "Sky Steadfast",
		BirthPlace:    "Southland",
		BirthPlaceLat: strPtr("N40.7128"), BirthPlaceLong: strPtr("W74.0060"),
		Version: 1, UpdatedAt: now,
	}
	for _, p := range []*repository.PersonReadModel{shadowed, tombstoned} {
		if err := readStore.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatalf("seed main person %s: %v", p.Surname, err)
		}
	}
	// Both are buried in the same cemetery. `life_events` has no branch_id yet
	// (sub-issue B of #676, #757), so these rows are the permanently-mainline half of
	// the GetPersonsByCemetery join and give it a non-empty source to resolve against.
	for _, p := range []*repository.PersonReadModel{shadowed, tombstoned} {
		if err := readStore.SaveEvent(ctx, &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   p.ID,
			FactType:  domain.FactPersonBurial,
			DateRaw:   "1899",
			Place:     cemetery,
			Version:   1,
			CreatedAt: now,
		}); err != nil {
			t.Fatalf("seed burial for %s: %v", p.Surname, err)
		}
	}
	// Flag the first as a brick wall BEFORE the branch exists, so the shadow copied
	// from main in step 3 carries the brick-wall fields and an unscoped GetBrickWalls
	// would report the person twice.
	if err := readStore.SetBrickWall(ctx, shadowed.ID, "needs a vital record"); err != nil {
		t.Fatalf("SetBrickWall on main: %v", err)
	}

	// --- Step 2: snapshot every mainline aggregate BEFORE any branch row exists.
	// This is the regression baseline for step 4. ---
	readMain := func(label string) mainAggregateSnapshot {
		t.Helper()
		var s mainAggregateSnapshot
		var err error
		mainOpts := repository.ListOptions{Limit: 100, BranchID: domain.MainBranchID}
		if s.surnames, s.letters, err = readStore.GetSurnameIndex(ctx, domain.MainBranchID); err != nil {
			t.Fatalf("%s: main GetSurnameIndex: %v", label, err)
		}
		if s.byLetter, err = readStore.GetSurnamesByLetter(ctx, domain.MainBranchID, "M"); err != nil {
			t.Fatalf("%s: main GetSurnamesByLetter: %v", label, err)
		}
		if s.places, err = readStore.GetPlaceHierarchy(ctx, domain.MainBranchID, ""); err != nil {
			t.Fatalf("%s: main GetPlaceHierarchy: %v", label, err)
		}
		if s.locations, err = readStore.GetMapLocations(ctx, domain.MainBranchID); err != nil {
			t.Fatalf("%s: main GetMapLocations: %v", label, err)
		}
		if s.bySurname, s.bySurnameN, err = readStore.GetPersonsBySurname(ctx, "Mainline", mainOpts); err != nil {
			t.Fatalf("%s: main GetPersonsBySurname: %v", label, err)
		}
		// "land" is a substring of all three fixture places, so this is the multi-row
		// case: main sees both persons, the branch sees only its shadow.
		if s.byPlace, s.byPlaceN, err = readStore.GetPersonsByPlace(ctx, "land", mainOpts); err != nil {
			t.Fatalf("%s: main GetPersonsByPlace: %v", label, err)
		}
		if s.byCemetery, s.byCemeteryN, err = readStore.GetPersonsByCemetery(ctx, cemetery, mainOpts); err != nil {
			t.Fatalf("%s: main GetPersonsByCemetery: %v", label, err)
		}
		if s.brickWalls, err = readStore.GetBrickWalls(ctx, true); err != nil {
			t.Fatalf("%s: main GetBrickWalls: %v", label, err)
		}
		return s
	}
	before := readMain("before branch")

	// Sanity: the baseline sees both main persons, both burials and the brick wall, so
	// step 4 is comparing real data rather than two empty results.
	if len(before.surnames) != 2 || len(before.places) != 2 || len(before.locations) != 2 {
		t.Fatalf("main baseline: want 2 surnames / 2 places / 2 map locations, got %d/%d/%d",
			len(before.surnames), len(before.places), len(before.locations))
	}
	if before.bySurnameN != 1 || len(before.brickWalls) != 1 {
		t.Fatalf("main baseline: want 1 Mainline person and 1 brick wall, got %d and %d",
			before.bySurnameN, len(before.brickWalls))
	}
	if before.byPlaceN != 2 || len(before.byPlace) != 2 {
		t.Fatalf("main baseline: want both persons matching place %q, got total=%d len=%d",
			"land", before.byPlaceN, len(before.byPlace))
	}
	if before.byCemeteryN != 2 || len(before.byCemetery) != 2 {
		t.Fatalf("main baseline: want both buried persons at %q, got total=%d len=%d",
			cemetery, before.byCemeteryN, len(before.byCemetery))
	}

	// --- Step 3: create the branch, shadow the first person (copy-on-write from
	// main, then change surname and birth place), and tombstone the second. ---
	branch, err := domain.NewBranch("aggregate-scope", "browse and map aggregates must not leak", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	if err := projector.Project(ctx, domain.NewBranchCreated(branch), 1, domain.MainBranchID); err != nil {
		t.Fatalf("project BranchCreated: %v", err)
	}
	branchID := domain.BranchID(branch.ID)

	mainRow, err := readStore.GetPerson(ctx, domain.MainBranchID, shadowed.ID)
	if err != nil || mainRow == nil {
		t.Fatalf("read main row to copy onto the branch: %v (person=%+v)", err, mainRow)
	}
	if mainRow.BrickWallSince == nil {
		t.Fatal("main row has no brick_wall_since; the shadow would not exercise the GetBrickWalls leak")
	}
	shadow := *mainRow
	shadow.Surname = "Branchline"
	shadow.FullName = "Robin Branchline"
	shadow.BirthPlace = "Westland"
	shadow.BirthPlaceLat = strPtr("N45.0000")
	shadow.BirthPlaceLong = strPtr("W93.0000")
	shadow.Version = mainRow.Version + 1
	if err := readStore.SavePerson(ctx, branchID, &shadow); err != nil {
		t.Fatalf("save branch shadow: %v", err)
	}
	if err := readStore.DeletePerson(ctx, branchID, tombstoned.ID); err != nil {
		t.Fatalf("tombstone on branch: %v", err)
	}

	// --- Step 4: THE LEAK REGRESSION. Every mainline aggregate must be identical to
	// the pre-branch snapshot: the shadow must not appear as an extra person, place,
	// map pin, cemetery occupant or brick wall, and the tombstone must not remove
	// anyone from main. ---
	after := readMain("after branch")
	for _, c := range []struct {
		name          string
		before, after any
	}{
		{"GetSurnameIndex surnames", before.surnames, after.surnames},
		{"GetSurnameIndex letters", before.letters, after.letters},
		{"GetSurnamesByLetter", before.byLetter, after.byLetter},
		{"GetPlaceHierarchy", before.places, after.places},
		{"GetMapLocations", before.locations, after.locations},
		{"GetPersonsBySurname rows", before.bySurname, after.bySurname},
		{"GetPersonsBySurname total", before.bySurnameN, after.bySurnameN},
		{"GetPersonsByPlace rows", before.byPlace, after.byPlace},
		{"GetPersonsByPlace total", before.byPlaceN, after.byPlaceN},
		{"GetPersonsByCemetery rows", before.byCemetery, after.byCemetery},
		{"GetPersonsByCemetery total", before.byCemeteryN, after.byCemeteryN},
		{"GetBrickWalls", before.brickWalls, after.brickWalls},
	} {
		if !reflect.DeepEqual(c.before, c.after) {
			t.Errorf("main %s changed once branch rows existed (BR-003 leak): before = %+v, after = %+v",
				c.name, c.before, c.after)
		}
	}

	// --- Step 5: the branch view resolves the overlay -- shadow instead of main's
	// row, tombstoned person gone -- one set-based read per aggregate. ---
	branchOpts := repository.ListOptions{Limit: 100, BranchID: branchID}

	branchSurnames, branchLetters, err := readStore.GetSurnameIndex(ctx, branchID)
	if err != nil {
		t.Fatalf("branch GetSurnameIndex: %v", err)
	}
	if want := []repository.SurnameEntry{{Surname: "Branchline", Count: 1}}; !reflect.DeepEqual(branchSurnames, want) {
		t.Errorf("branch GetSurnameIndex surnames = %+v, want %+v", branchSurnames, want)
	}
	if want := []repository.LetterCount{{Letter: "B", Count: 1}}; !reflect.DeepEqual(branchLetters, want) {
		t.Errorf("branch GetSurnameIndex letters = %+v, want %+v", branchLetters, want)
	}

	for _, c := range []struct {
		letter string
		want   int
	}{{"B", 1}, {"M", 0}, {"S", 0}} {
		got, err := readStore.GetSurnamesByLetter(ctx, branchID, c.letter)
		if err != nil {
			t.Fatalf("branch GetSurnamesByLetter(%s): %v", c.letter, err)
		}
		if len(got) != c.want {
			t.Errorf("branch GetSurnamesByLetter(%s) = %+v, want %d entries", c.letter, got, c.want)
		}
	}

	branchPlaces, err := readStore.GetPlaceHierarchy(ctx, branchID, "")
	if err != nil {
		t.Fatalf("branch GetPlaceHierarchy: %v", err)
	}
	if len(branchPlaces) != 1 || branchPlaces[0].Name != "Westland" || branchPlaces[0].Count != 1 {
		t.Errorf("branch GetPlaceHierarchy = %+v, want only the shadowed place Westland", branchPlaces)
	}

	branchLocations, err := readStore.GetMapLocations(ctx, branchID)
	if err != nil {
		t.Fatalf("branch GetMapLocations: %v", err)
	}
	if len(branchLocations) != 1 || branchLocations[0].Place != "Westland" ||
		branchLocations[0].EventType != "birth" || branchLocations[0].Count != 1 {
		t.Errorf("branch GetMapLocations = %+v, want one birth pin at Westland", branchLocations)
	}

	for _, c := range []struct {
		surname string
		want    int
	}{{"Branchline", 1}, {"Mainline", 0}, {"Steadfast", 0}} {
		rows, total, err := readStore.GetPersonsBySurname(ctx, c.surname, branchOpts)
		if err != nil {
			t.Fatalf("branch GetPersonsBySurname(%s): %v", c.surname, err)
		}
		if total != c.want || len(rows) != c.want {
			t.Errorf("branch GetPersonsBySurname(%s): want %d, got total=%d len=%d",
				c.surname, c.want, total, len(rows))
		}
	}

	for _, c := range []struct {
		place string
		want  int
	}{{"Westland", 1}, {"Northland", 0}, {"Southland", 0}, {"land", 1}} {
		rows, total, err := readStore.GetPersonsByPlace(ctx, c.place, branchOpts)
		if err != nil {
			t.Fatalf("branch GetPersonsByPlace(%s): %v", c.place, err)
		}
		if total != c.want || len(rows) != c.want {
			t.Errorf("branch GetPersonsByPlace(%s): want %d, got total=%d len=%d",
				c.place, c.want, total, len(rows))
		}
	}

	// --- Step 6: the cross-scope join. GetPersonsByCemetery reads `life_events` on
	// main (no branch_id there yet, #757) but resolves the PERSON through the overlay.
	// Under branch scope the branch must therefore see the shadow's surname -- showing
	// the mainline spelling here is the regression a user would notice -- and must not
	// see the person the branch tombstoned. ---
	branchCemetery, branchCemeteryN, err := readStore.GetPersonsByCemetery(ctx, cemetery, branchOpts)
	if err != nil {
		t.Fatalf("branch GetPersonsByCemetery: %v", err)
	}
	if branchCemeteryN != 1 || len(branchCemetery) != 1 {
		t.Fatalf("branch GetPersonsByCemetery(%q): want only the shadowed person (the other is tombstoned on the branch), got total=%d len=%d",
			cemetery, branchCemeteryN, len(branchCemetery))
	}
	if branchCemetery[0].ID != shadowed.ID {
		t.Fatalf("branch GetPersonsByCemetery returned person %s, want the shadowed %s",
			branchCemetery[0].ID, shadowed.ID)
	}
	if branchCemetery[0].Surname != "Branchline" {
		t.Errorf("branch GetPersonsByCemetery surname = %q, want the shadow's %q (the mainline spelling leaked through the join)",
			branchCemetery[0].Surname, "Branchline")
	}
	// The tombstoned person is still buried there on main -- the branch dropped them,
	// mainline did not.
	mainCemetery, mainCemeteryN, err := readStore.GetPersonsByCemetery(ctx, cemetery,
		repository.ListOptions{Limit: 100, BranchID: domain.MainBranchID})
	if err != nil {
		t.Fatalf("main GetPersonsByCemetery: %v", err)
	}
	foundTombstoned := false
	for _, p := range mainCemetery {
		if p.ID == tombstoned.ID {
			foundTombstoned = true
		}
	}
	if mainCemeteryN != 2 || !foundTombstoned {
		t.Errorf("main GetPersonsByCemetery(%q): want both burials including the branch-tombstoned person, got total=%d rows=%+v",
			cemetery, mainCemeteryN, mainCemetery)
	}

	// --- Step 7: brick walls are main-pinned. A second SetBrickWall on a person that
	// HAS a branch shadow must rewrite main's row and leave the shadow's note as it
	// was when the branch copied it. ---
	if err := readStore.SetBrickWall(ctx, shadowed.ID, "still unsourced"); err != nil {
		t.Fatalf("SetBrickWall after shadow: %v", err)
	}
	walls, err := readStore.GetBrickWalls(ctx, false)
	if err != nil {
		t.Fatalf("GetBrickWalls after set: %v", err)
	}
	if len(walls) != 1 || walls[0].PersonID != shadowed.ID || walls[0].Note != "still unsourced" {
		t.Fatalf("GetBrickWalls after set = %+v, want exactly one entry carrying main's new note", walls)
	}
	shadowPerson, err := readStore.GetPerson(ctx, branchID, shadowed.ID)
	if err != nil || shadowPerson == nil {
		t.Fatalf("branch GetPerson after SetBrickWall: %v (person=%+v)", err, shadowPerson)
	}
	if shadowPerson.Surname != "Branchline" {
		t.Fatalf("branch GetPerson resolved to %q, want the shadow row Branchline", shadowPerson.Surname)
	}
	if shadowPerson.BrickWallNote != "needs a vital record" {
		t.Errorf("SetBrickWall wrote the branch shadow row: note = %q, want the copied %q",
			shadowPerson.BrickWallNote, "needs a vital record")
	}

	// ResolveBrickWall is main-pinned for the same reason.
	if err := readStore.ResolveBrickWall(ctx, shadowed.ID); err != nil {
		t.Fatalf("ResolveBrickWall: %v", err)
	}
	if walls, err = readStore.GetBrickWalls(ctx, false); err != nil || len(walls) != 0 {
		t.Errorf("GetBrickWalls after resolve = %+v (err=%v), want none unresolved", walls, err)
	}
	shadowPerson, err = readStore.GetPerson(ctx, branchID, shadowed.ID)
	if err != nil || shadowPerson == nil {
		t.Fatalf("branch GetPerson after ResolveBrickWall: %v (person=%+v)", err, shadowPerson)
	}
	if shadowPerson.BrickWallResolvedAt != nil {
		t.Errorf("ResolveBrickWall wrote the branch shadow row: resolved_at = %v", shadowPerson.BrickWallResolvedAt)
	}
}
