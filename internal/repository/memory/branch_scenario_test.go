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
	cemeteries  []repository.CemeteryEntry
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
//  3. GetPersonsByCemetery -- where the `persons` overlay joins the `life_events`
//     overlay (#757) -- resolves the person through the overlay: with real burial rows
//     on both sides of the join, the branch sees the shadow's surname and not the
//     mainline spelling, and the tombstoned person disappears, as they do from the
//     branch's cemetery index (their burial was tombstoned by the DeletePerson cascade);
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
	// Both are buried in the same cemetery, on main. The branch never edits these
	// rows directly; the tombstone in step 3 hides the second one through the
	// DeletePerson cascade (#757).
	for _, p := range []*repository.PersonReadModel{shadowed, tombstoned} {
		if err := readStore.SaveEvent(ctx, domain.MainBranchID, &repository.EventReadModel{
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
		if s.cemeteries, err = readStore.GetCemeteryIndex(ctx, domain.MainBranchID); err != nil {
			t.Fatalf("%s: main GetCemeteryIndex: %v", label, err)
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
		{"GetCemeteryIndex", before.cemeteries, after.cemeteries},
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

	// --- Step 6: the cemetery pair. GetPersonsByCemetery joins the `life_events`
	// overlay to the `persons` overlay (#757). Under branch scope the branch must see
	// the shadow's surname -- showing the mainline spelling here is the regression a
	// user would notice -- and must not see the person the branch tombstoned; the
	// cemetery index must count the same single person. ---
	if want, got := []repository.CemeteryEntry{{Place: cemetery, Count: 2}}, before.cemeteries; !reflect.DeepEqual(got, want) {
		t.Errorf("main GetCemeteryIndex = %+v, want %+v", got, want)
	}
	branchIndex, err := readStore.GetCemeteryIndex(ctx, branchID)
	if err != nil {
		t.Fatalf("branch GetCemeteryIndex: %v", err)
	}
	if want := []repository.CemeteryEntry{{Place: cemetery, Count: 1}}; !reflect.DeepEqual(branchIndex, want) {
		t.Errorf("branch GetCemeteryIndex = %+v, want %+v (the tombstoned person's burial must not count)", branchIndex, want)
	}
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

// TestBranchScenario_FactOverlay drives the ADR-005 copy-on-write overlay through
// the person/family fact tables (sub-issue B of #676, #757) — life events,
// attributes, associations and the cemetery index — against the in-memory backend. The
// scenario body (runBranchFactScenario) is an identical copy of the other
// backends' so all three prove the same behavior (DB-001). Fixtures use neutral
// placeholder names and invented places only (public repo — no real PII).
func TestBranchScenario_FactOverlay(t *testing.T) {
	runBranchFactScenario(t, memory.NewReadModelStore(), memory.NewBranchStore())
}

// runBranchFactScenario is the backend-agnostic person/family fact scenario for
// sub-issue B of #676 (#757). Each backend package carries an identical copy
// (there is no shared test harness in this repo); keeping the assertions
// byte-identical is the DB-001 parity guarantee.
//
// Shape: two persons and their family on main, each fact table seeded through the
// projector. A branch then edits, deletes and adds a row of each, re-owns an
// attribute, and deletes one person and the family. Every step asserts that main
// is untouched, that the branch resolves shadow-over-main with tombstones
// honoured (single-row, per-owner and paged reads, and the cemetery index), and,
// last, that deleting the branch purges its fact rows so the branch id resolves
// to main again.
func runBranchFactScenario(t *testing.T, readStore repository.ReadModelStore, branchStore repository.BranchStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, branchStore)
	main := domain.MainBranchID
	mainOpts := repository.ListOptions{Limit: 100, BranchID: main}

	project := func(label string, branchID domain.BranchID, events ...domain.Event) {
		t.Helper()
		for i, ev := range events {
			if err := projector.Project(ctx, ev, int64(i+2), branchID); err != nil {
				t.Fatalf("%s: project %s: %v", label, ev.EventType(), err)
			}
		}
	}

	// --- Step 1: seed main. ---
	subject := domain.NewPerson("Alex", "Original")
	partner := domain.NewPerson("Sam", "Steady")
	family := domain.NewFamilyWithPartners(&subject.ID, &partner.ID)
	birth := domain.NewLifeEvent(subject.ID, domain.FactPersonBirth)
	birth.Place = "Northland"
	burial := domain.NewLifeEvent(subject.ID, domain.FactPersonBurial)
	burial.Place = "Restland Cemetery"
	marriage := domain.NewFamilyLifeEvent(family.ID, domain.FactFamilyMarriage)
	marriage.Place = "Chapel"
	occupation := domain.NewAttribute(subject.ID, domain.FactPersonOccupation, "Farmer")
	witness := domain.NewAssociation(subject.ID, partner.ID, "witness")
	project("seed main", main,
		domain.NewPersonCreated(subject),
		domain.NewPersonCreated(partner),
		domain.NewFamilyCreated(family),
		domain.NewLifeEventCreatedFromModel(birth),
		domain.NewLifeEventCreatedFromModel(burial),
		domain.NewLifeEventCreatedFromModel(marriage),
		domain.NewAttributeCreatedFromModel(occupation),
		domain.NewAssociationCreated(witness),
	)

	branch, err := domain.NewBranch("fact-scope", "person and family facts must fork", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	project("create branch", main, domain.NewBranchCreated(branch))
	branchID := domain.BranchID(branch.ID)
	branchOpts := repository.ListOptions{Limit: 100, BranchID: branchID}

	// mainView asserts main's facts are exactly what step 1 seeded.
	mainView := func(label string) {
		t.Helper()
		if got, err := readStore.GetEvent(ctx, main, birth.ID); err != nil || got == nil || got.Place != "Northland" {
			t.Errorf("%s: main GetEvent(birth) = %+v (err=%v), want place Northland", label, got, err)
		}
		if got, err := readStore.ListEventsForPerson(ctx, main, subject.ID); err != nil || len(got) != 2 {
			t.Errorf("%s: main ListEventsForPerson(subject) = %d rows (err=%v), want 2", label, len(got), err)
		}
		if got, err := readStore.ListEventsForPerson(ctx, main, partner.ID); err != nil || len(got) != 0 {
			t.Errorf("%s: main ListEventsForPerson(partner) = %d rows (err=%v), want 0", label, len(got), err)
		}
		if got, err := readStore.ListEventsForFamily(ctx, main, family.ID); err != nil || len(got) != 1 {
			t.Errorf("%s: main ListEventsForFamily = %d rows (err=%v), want 1", label, len(got), err)
		}
		if _, total, err := readStore.ListEvents(ctx, mainOpts); err != nil || total != 3 {
			t.Errorf("%s: main ListEvents total = %d (err=%v), want 3", label, total, err)
		}
		if got, err := readStore.GetAttribute(ctx, main, occupation.ID); err != nil || got == nil || got.Value != "Farmer" || got.PersonID != subject.ID {
			t.Errorf("%s: main GetAttribute = %+v (err=%v), want the subject's Farmer", label, got, err)
		}
		if got, err := readStore.ListAttributesForPerson(ctx, main, partner.ID); err != nil || len(got) != 0 {
			t.Errorf("%s: main ListAttributesForPerson(partner) = %d rows (err=%v), want 0", label, len(got), err)
		}
		if _, total, err := readStore.ListAttributes(ctx, mainOpts); err != nil || total != 1 {
			t.Errorf("%s: main ListAttributes total = %d (err=%v), want 1", label, total, err)
		}
		if got, err := readStore.GetAssociation(ctx, main, witness.ID); err != nil || got == nil || got.Role != "witness" {
			t.Errorf("%s: main GetAssociation = %+v (err=%v), want role witness", label, got, err)
		}
		if got, err := readStore.ListAssociationsForPerson(ctx, main, subject.ID); err != nil || len(got) != 1 {
			t.Errorf("%s: main ListAssociationsForPerson(subject) = %d rows (err=%v), want 1", label, len(got), err)
		}
		if _, total, err := readStore.ListAssociations(ctx, mainOpts); err != nil || total != 1 {
			t.Errorf("%s: main ListAssociations total = %d (err=%v), want 1", label, total, err)
		}
		want := []repository.CemeteryEntry{{Place: "Restland Cemetery", Count: 1}}
		if got, err := readStore.GetCemeteryIndex(ctx, main); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: main GetCemeteryIndex = %+v (err=%v), want %+v", label, got, err, want)
		}
	}
	mainView("baseline")

	// Before the branch writes anything it resolves entirely to main.
	if got, err := readStore.GetEvent(ctx, branchID, birth.ID); err != nil || got == nil || got.Place != "Northland" {
		t.Fatalf("fresh branch GetEvent(birth) = %+v (err=%v), want main's row", got, err)
	}

	// --- Step 2: the branch edits, deletes and adds one row of each fact table. ---
	baptism := domain.NewLifeEvent(partner.ID, domain.FactPersonBaptism)
	baptism.Place = "Font"
	trade := domain.NewAttribute(partner.ID, domain.FactPersonOccupation, "Smith")
	mentor := domain.NewAssociation(partner.ID, subject.ID, "mentor")
	project("branch edits", branchID,
		domain.NewLifeEventUpdated(birth.ID, map[string]any{"place": "Westland"}),
		domain.NewLifeEventDeleted(burial.ID, "branch hypothesis"),
		domain.NewLifeEventCreatedFromModel(baptism),
		domain.NewAttributeUpdated(occupation.ID, map[string]any{"value": "Miller"}),
		domain.NewAttributeCreatedFromModel(trade),
		domain.NewAssociationUpdated(witness.ID, map[string]any{"role": "godparent"}),
		domain.NewAssociationCreated(mentor),
	)
	mainView("after branch edits")

	// Life events: shadow wins, tombstone hides, branch-only row appears, the
	// untouched family event falls back to main.
	if got, err := readStore.GetEvent(ctx, branchID, birth.ID); err != nil || got == nil || got.Place != "Westland" {
		t.Errorf("branch GetEvent(birth) = %+v (err=%v), want the shadow's Westland", got, err)
	}
	if got, err := readStore.GetEvent(ctx, branchID, burial.ID); err != nil || got != nil {
		t.Errorf("branch GetEvent(burial) = %+v (err=%v), want tombstoned", got, err)
	}
	if got, err := readStore.GetEvent(ctx, main, baptism.ID); err != nil || got != nil {
		t.Errorf("main GetEvent(branch-only baptism) = %+v (err=%v), want absent", got, err)
	}
	if got, err := readStore.ListEventsForPerson(ctx, branchID, subject.ID); err != nil || len(got) != 1 || got[0].ID != birth.ID || got[0].Place != "Westland" {
		t.Errorf("branch ListEventsForPerson(subject) = %+v (err=%v), want only the Westland birth", got, err)
	}
	if got, err := readStore.ListEventsForPerson(ctx, branchID, partner.ID); err != nil || len(got) != 1 || got[0].ID != baptism.ID {
		t.Errorf("branch ListEventsForPerson(partner) = %+v (err=%v), want the branch-only baptism", got, err)
	}
	if got, err := readStore.ListEventsForFamily(ctx, branchID, family.ID); err != nil || len(got) != 1 || got[0].ID != marriage.ID {
		t.Errorf("branch ListEventsForFamily = %+v (err=%v), want main's marriage via fallback", got, err)
	}
	if rows, total, err := readStore.ListEvents(ctx, branchOpts); err != nil || total != 3 || len(rows) != 3 {
		t.Errorf("branch ListEvents = total %d, %d rows (err=%v), want 3 (shadowed birth, marriage, baptism)", total, len(rows), err)
	}
	if got, err := readStore.GetCemeteryIndex(ctx, branchID); err != nil || len(got) != 0 {
		t.Errorf("branch GetCemeteryIndex = %+v (err=%v), want empty (the burial is tombstoned)", got, err)
	}
	// Paging walks the branch view without repeating or dropping a row.
	page1, _, err := readStore.ListEvents(ctx, repository.ListOptions{Limit: 2, BranchID: branchID})
	if err != nil {
		t.Fatalf("branch ListEvents page 1: %v", err)
	}
	page2, _, err := readStore.ListEvents(ctx, repository.ListOptions{Limit: 2, Offset: 2, BranchID: branchID})
	if err != nil {
		t.Fatalf("branch ListEvents page 2: %v", err)
	}
	seen := map[uuid.UUID]bool{}
	for _, e := range append(page1, page2...) {
		seen[e.ID] = true
	}
	if len(page1) != 2 || len(page2) != 1 || len(seen) != 3 {
		t.Errorf("branch ListEvents paging = %d + %d rows, %d distinct, want 2 + 1 covering 3", len(page1), len(page2), len(seen))
	}

	// Attributes.
	if got, err := readStore.GetAttribute(ctx, branchID, occupation.ID); err != nil || got == nil || got.Value != "Miller" {
		t.Errorf("branch GetAttribute(occupation) = %+v (err=%v), want the shadow's Miller", got, err)
	}
	if got, err := readStore.ListAttributesForPerson(ctx, branchID, partner.ID); err != nil || len(got) != 1 || got[0].ID != trade.ID {
		t.Errorf("branch ListAttributesForPerson(partner) = %+v (err=%v), want the branch-only Smith", got, err)
	}
	if _, total, err := readStore.ListAttributes(ctx, branchOpts); err != nil || total != 2 {
		t.Errorf("branch ListAttributes total = %d (err=%v), want 2", total, err)
	}

	// Associations: the subject appears in both, as subject and as associate.
	if got, err := readStore.GetAssociation(ctx, branchID, witness.ID); err != nil || got == nil || got.Role != "godparent" {
		t.Errorf("branch GetAssociation(witness) = %+v (err=%v), want the shadow's godparent", got, err)
	}
	if got, err := readStore.ListAssociationsForPerson(ctx, branchID, subject.ID); err != nil || len(got) != 2 {
		t.Errorf("branch ListAssociationsForPerson(subject) = %d rows (err=%v), want 2", len(got), err)
	}
	if rows, total, err := readStore.ListAssociations(ctx, branchOpts); err != nil || total != 2 || len(rows) != 2 {
		t.Errorf("branch ListAssociations = total %d, %d rows (err=%v), want 2", total, len(rows), err)
	}

	// --- Step 3: re-own an attribute on the branch (the shape PersonMerged
	// writes). The per-owner list must follow the WINNING row: the subject loses it
	// on the branch while main still lists it under the subject. ---
	moved, err := readStore.GetAttribute(ctx, branchID, occupation.ID)
	if err != nil || moved == nil {
		t.Fatalf("branch GetAttribute before re-own: %+v (err=%v)", moved, err)
	}
	moved.PersonID = partner.ID
	if err := readStore.SaveAttribute(ctx, branchID, moved); err != nil {
		t.Fatalf("branch SaveAttribute re-own: %v", err)
	}
	if got, err := readStore.ListAttributesForPerson(ctx, branchID, subject.ID); err != nil || len(got) != 0 {
		t.Errorf("branch ListAttributesForPerson(subject) after re-own = %+v (err=%v), want none", got, err)
	}
	if got, err := readStore.ListAttributesForPerson(ctx, branchID, partner.ID); err != nil || len(got) != 2 {
		t.Errorf("branch ListAttributesForPerson(partner) after re-own = %d rows (err=%v), want 2", len(got), err)
	}
	mainView("after re-own")

	// A save after a delete clears the tombstone.
	mainBurial, err := readStore.GetEvent(ctx, main, burial.ID)
	if err != nil || mainBurial == nil {
		t.Fatalf("main GetEvent(burial): %+v (err=%v)", mainBurial, err)
	}
	if err := readStore.SaveEvent(ctx, branchID, mainBurial); err != nil {
		t.Fatalf("branch SaveEvent over tombstone: %v", err)
	}
	if got, err := readStore.GetEvent(ctx, branchID, burial.ID); err != nil || got == nil {
		t.Errorf("branch GetEvent(burial) after re-save = %+v (err=%v), want it back", got, err)
	}
	want := []repository.CemeteryEntry{{Place: "Restland Cemetery", Count: 1}}
	if got, err := readStore.GetCemeteryIndex(ctx, branchID); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("branch GetCemeteryIndex after re-save = %+v (err=%v), want %+v", got, err, want)
	}

	// --- Step 4: cascade. Deleting the partner and the family on the branch
	// tombstones every fact they own or appear in, on the branch only. ---
	project("branch deletes", branchID,
		domain.NewPersonDeleted(partner.ID, "branch hypothesis"),
		domain.NewFamilyDeleted(family.ID, "branch hypothesis"),
	)
	mainView("after branch deletes")
	if got, err := readStore.ListEventsForPerson(ctx, branchID, partner.ID); err != nil || len(got) != 0 {
		t.Errorf("branch ListEventsForPerson(deleted partner) = %+v (err=%v), want none", got, err)
	}
	if got, err := readStore.ListAttributesForPerson(ctx, branchID, partner.ID); err != nil || len(got) != 0 {
		t.Errorf("branch ListAttributesForPerson(deleted partner) = %+v (err=%v), want none (including the re-owned one)", got, err)
	}
	if got, err := readStore.ListAssociationsForPerson(ctx, branchID, subject.ID); err != nil || len(got) != 0 {
		t.Errorf("branch ListAssociationsForPerson(subject) after partner delete = %+v (err=%v), want none", got, err)
	}
	if got, err := readStore.ListEventsForFamily(ctx, branchID, family.ID); err != nil || len(got) != 0 {
		t.Errorf("branch ListEventsForFamily(deleted family) = %+v (err=%v), want none", got, err)
	}
	if got, err := readStore.ListEventsForPerson(ctx, branchID, subject.ID); err != nil || len(got) != 2 {
		t.Errorf("branch ListEventsForPerson(subject) after partner delete = %d rows (err=%v), want 2 (untouched)", len(got), err)
	}

	// --- Step 5: deleting the branch purges its fact rows; the branch id then
	// resolves to main exactly. ---
	project("delete branch", main, domain.NewBranchDeleted(branch.ID))
	mainView("after purge")
	if got, err := readStore.GetEvent(ctx, branchID, birth.ID); err != nil || got == nil || got.Place != "Northland" {
		t.Errorf("purged branch GetEvent(birth) = %+v (err=%v), want main's Northland", got, err)
	}
	if got, err := readStore.GetEvent(ctx, branchID, baptism.ID); err != nil || got != nil {
		t.Errorf("purged branch GetEvent(baptism) = %+v (err=%v), want absent", got, err)
	}
	if _, total, err := readStore.ListEvents(ctx, branchOpts); err != nil || total != 3 {
		t.Errorf("purged branch ListEvents total = %d (err=%v), want main's 3", total, err)
	}
	if got, err := readStore.GetAttribute(ctx, branchID, occupation.ID); err != nil || got == nil || got.Value != "Farmer" {
		t.Errorf("purged branch GetAttribute = %+v (err=%v), want main's Farmer", got, err)
	}
	if _, total, err := readStore.ListAttributes(ctx, branchOpts); err != nil || total != 1 {
		t.Errorf("purged branch ListAttributes total = %d (err=%v), want main's 1", total, err)
	}
	if got, err := readStore.GetAssociation(ctx, branchID, witness.ID); err != nil || got == nil || got.Role != "witness" {
		t.Errorf("purged branch GetAssociation = %+v (err=%v), want main's witness", got, err)
	}
	if _, total, err := readStore.ListAssociations(ctx, branchOpts); err != nil || total != 1 {
		t.Errorf("purged branch ListAssociations total = %d (err=%v), want main's 1", total, err)
	}
}

// TestBranchScenario_EvidenceOverlay drives the ADR-005 copy-on-write overlay
// through the evidence tables (sub-issue C of #676, #758) — sources, source
// external IDs, citations and notes — against the memory backend. The scenario
// body (runBranchEvidenceScenario) is an identical copy of the other backends'
// so all three prove the same behavior (DB-001). Fixtures use neutral
// placeholder names only (public repo — no real PII).
func TestBranchScenario_EvidenceOverlay(t *testing.T) {
	runBranchEvidenceScenario(t, memory.NewReadModelStore(), memory.NewBranchStore())
}

// runBranchEvidenceScenario is the backend-agnostic evidence scenario for
// sub-issue C of #676 (#758): sources, source external IDs, citations and notes.
// Each backend package carries an identical copy (there is no shared test
// harness in this repo); keeping the assertions byte-identical is the DB-001
// parity guarantee.
//
// Shape: one person, two cited sources (one with external IDs) and a note on
// main, seeded through the projector. A branch then retitles both sources, cites
// the retitled one (the citation must denormalize the BRANCH title — the
// Source-before-Citation ordering constraint), edits and re-points citations,
// replaces external IDs, edits and adds notes, and finally deletes a source
// (cascading to its external IDs and citations on the branch only). Every step
// asserts that main is untouched and that the branch resolves shadow-over-main
// with tombstones honoured, including SearchSources, which must resolve the
// overlay before matching. Last, deleting the branch purges its evidence rows so
// the branch id resolves to main again.
func runBranchEvidenceScenario(t *testing.T, readStore repository.ReadModelStore, branchStore repository.BranchStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, branchStore)
	main := domain.MainBranchID
	mainOpts := repository.ListOptions{Limit: 100, BranchID: main}

	project := func(label string, branchID domain.BranchID, events ...domain.Event) {
		t.Helper()
		for i, ev := range events {
			if err := projector.Project(ctx, ev, int64(i+2), branchID); err != nil {
				t.Fatalf("%s: project %s: %v", label, ev.EventType(), err)
			}
		}
	}
	searchIDs := func(branchID domain.BranchID, q string) []uuid.UUID {
		t.Helper()
		got, err := readStore.SearchSources(ctx, branchID, q, 10)
		if err != nil {
			t.Fatalf("SearchSources(%q): %v", q, err)
		}
		ids := make([]uuid.UUID, 0, len(got))
		for _, s := range got {
			ids = append(ids, s.ID)
		}
		return ids
	}

	// --- Step 1: seed main. ---
	subject := domain.NewPerson("Alex", "Original")
	census := domain.NewSource("Census 1880", domain.SourceCensus)
	census.Author = "Enumerator"
	register := domain.NewSource("Parish Register", domain.SourceChurch)
	birthCite := domain.NewCitation(census.ID, domain.FactPersonBirth, subject.ID)
	birthCite.Page = "12"
	deathCite := domain.NewCitation(register.ID, domain.FactPersonDeath, subject.ID)
	note := domain.NewNote("Seen in the register")
	project("seed main", main,
		domain.NewPersonCreated(subject),
		domain.NewSourceCreated(census),
		domain.NewSourceCreated(register),
		domain.NewCitationCreated(birthCite),
		domain.NewCitationCreated(deathCite),
		domain.NewNoteCreated(note),
	)
	if err := readStore.ReplaceSourceExternalIDs(ctx, main, census.ID, []repository.SourceExternalIDReadModel{
		{Value: "MAIN-1", Type: "http://example.org/ids"},
	}); err != nil {
		t.Fatalf("seed main external ids: %v", err)
	}

	branch, err := domain.NewBranch("evidence-scope", "evidence must fork", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	project("create branch", main, domain.NewBranchCreated(branch))
	branchID := domain.BranchID(branch.ID)
	branchOpts := repository.ListOptions{Limit: 100, BranchID: branchID}

	// mainView asserts main's evidence is exactly what step 1 seeded.
	mainView := func(label string) {
		t.Helper()
		if got, err := readStore.GetSource(ctx, main, census.ID); err != nil || got == nil || got.Title != "Census 1880" || got.CitationCount != 1 {
			t.Errorf("%s: main GetSource(census) = %+v (err=%v), want Census 1880 with 1 citation", label, got, err)
		}
		if got, err := readStore.GetSource(ctx, main, register.ID); err != nil || got == nil || got.Title != "Parish Register" || got.CitationCount != 1 {
			t.Errorf("%s: main GetSource(register) = %+v (err=%v), want Parish Register with 1 citation", label, got, err)
		}
		if _, total, err := readStore.ListSources(ctx, mainOpts); err != nil || total != 2 {
			t.Errorf("%s: main ListSources total = %d (err=%v), want 2", label, total, err)
		}
		if got := searchIDs(main, "parish"); !reflect.DeepEqual(got, []uuid.UUID{register.ID}) {
			t.Errorf("%s: main SearchSources(parish) = %v, want [register]", label, got)
		}
		if got := searchIDs(main, "enumerator"); !reflect.DeepEqual(got, []uuid.UUID{census.ID}) {
			t.Errorf("%s: main SearchSources(enumerator) = %v, want [census]", label, got)
		}
		if got, err := readStore.GetSourceExternalIDs(ctx, main, census.ID); err != nil || len(got) != 1 || got[0].Value != "MAIN-1" {
			t.Errorf("%s: main GetSourceExternalIDs = %+v (err=%v), want [MAIN-1]", label, got, err)
		}
		if got, err := readStore.GetCitation(ctx, main, birthCite.ID); err != nil || got == nil || got.Page != "12" || got.SourceTitle != "Census 1880" {
			t.Errorf("%s: main GetCitation(birth) = %+v (err=%v), want page 12 of Census 1880", label, got, err)
		}
		if got, err := readStore.GetCitationsForSource(ctx, main, register.ID); err != nil || len(got) != 1 || got[0].ID != deathCite.ID {
			t.Errorf("%s: main GetCitationsForSource(register) = %+v (err=%v), want [death]", label, got, err)
		}
		if got, err := readStore.GetCitationsForPerson(ctx, main, subject.ID); err != nil || len(got) != 2 {
			t.Errorf("%s: main GetCitationsForPerson = %d rows (err=%v), want 2", label, len(got), err)
		}
		if got, err := readStore.GetCitationsForFact(ctx, main, domain.FactPersonBirth, subject.ID); err != nil || len(got) != 1 || got[0].ID != birthCite.ID {
			t.Errorf("%s: main GetCitationsForFact(birth) = %+v (err=%v), want [birth]", label, got, err)
		}
		if _, total, err := readStore.ListCitations(ctx, mainOpts); err != nil || total != 2 {
			t.Errorf("%s: main ListCitations total = %d (err=%v), want 2", label, total, err)
		}
		if got, err := readStore.GetNote(ctx, main, note.ID); err != nil || got == nil || got.Text != "Seen in the register" {
			t.Errorf("%s: main GetNote = %+v (err=%v), want the seeded text", label, got, err)
		}
		if _, total, err := readStore.ListNotes(ctx, mainOpts); err != nil || total != 1 {
			t.Errorf("%s: main ListNotes total = %d (err=%v), want 1", label, total, err)
		}
	}
	mainView("baseline")

	// Before the branch writes anything it resolves entirely to main.
	if got, err := readStore.GetSource(ctx, branchID, census.ID); err != nil || got == nil || got.Title != "Census 1880" {
		t.Fatalf("fresh branch GetSource(census) = %+v (err=%v), want main's row", got, err)
	}
	if got, err := readStore.GetSourceExternalIDs(ctx, branchID, census.ID); err != nil || len(got) != 1 || got[0].Value != "MAIN-1" {
		t.Fatalf("fresh branch GetSourceExternalIDs = %+v (err=%v), want main's bucket", got, err)
	}

	// --- Step 2: the branch retitles both sources, THEN cites the census: the new
	// citation must carry the branch's title. It also edits a citation and a note,
	// adds a source and a note, and replaces the census's external IDs. ---
	burialCite := domain.NewCitation(census.ID, domain.FactPersonBurial, subject.ID)
	diary := domain.NewSource("Family Diary", domain.SourceBook)
	branchNote := domain.NewNote("Branch-only note")
	project("branch edits", branchID,
		domain.NewSourceUpdated(census.ID, map[string]any{"title": "Census 1880 (Revised)"}),
		domain.NewSourceUpdated(register.ID, map[string]any{"title": "Baptism Roll"}),
		domain.NewCitationCreated(burialCite),
		domain.NewCitationUpdated(birthCite.ID, map[string]any{"page": "99"}),
		domain.NewSourceCreated(diary),
		domain.NewNoteUpdated(note.ID, map[string]any{"text": "Revised on the branch"}),
		domain.NewNoteCreated(branchNote),
	)
	if err := readStore.ReplaceSourceExternalIDs(ctx, branchID, census.ID, []repository.SourceExternalIDReadModel{
		{Value: "BRANCH-1", Type: "http://example.org/ids"},
		{Value: "BRANCH-2", Type: "http://example.org/ids"},
	}); err != nil {
		t.Fatalf("branch ReplaceSourceExternalIDs: %v", err)
	}
	mainView("after branch edits")

	if got, err := readStore.GetSource(ctx, branchID, census.ID); err != nil || got == nil || got.Title != "Census 1880 (Revised)" || got.CitationCount != 2 {
		t.Errorf("branch GetSource(census) = %+v (err=%v), want the revised title with 2 citations", got, err)
	}
	if got, err := readStore.GetCitation(ctx, branchID, burialCite.ID); err != nil || got == nil || got.SourceTitle != "Census 1880 (Revised)" {
		t.Errorf("branch GetCitation(burial) = %+v (err=%v), want the BRANCH source title denormalized", got, err)
	}
	if got, err := readStore.GetCitation(ctx, main, burialCite.ID); err != nil || got != nil {
		t.Errorf("main GetCitation(branch-only burial) = %+v (err=%v), want absent", got, err)
	}
	if got, err := readStore.GetCitation(ctx, branchID, birthCite.ID); err != nil || got == nil || got.Page != "99" {
		t.Errorf("branch GetCitation(birth) = %+v (err=%v), want the shadow's page 99", got, err)
	}
	if got, err := readStore.GetCitationsForSource(ctx, branchID, census.ID); err != nil || len(got) != 2 {
		t.Errorf("branch GetCitationsForSource(census) = %d rows (err=%v), want 2", len(got), err)
	}
	if got, err := readStore.GetCitationsForPerson(ctx, branchID, subject.ID); err != nil || len(got) != 3 {
		t.Errorf("branch GetCitationsForPerson = %d rows (err=%v), want 3", len(got), err)
	}
	if got, err := readStore.GetCitationsForFact(ctx, branchID, domain.FactPersonBirth, subject.ID); err != nil || len(got) != 1 || got[0].Page != "99" {
		t.Errorf("branch GetCitationsForFact(birth) = %+v (err=%v), want the page-99 shadow only", got, err)
	}
	if rows, total, err := readStore.ListCitations(ctx, branchOpts); err != nil || total != 3 || len(rows) != 3 {
		t.Errorf("branch ListCitations = total %d, %d rows (err=%v), want 3", total, len(rows), err)
	}
	if rows, total, err := readStore.ListSources(ctx, branchOpts); err != nil || total != 3 || len(rows) != 3 {
		t.Errorf("branch ListSources = total %d, %d rows (err=%v), want 3", total, len(rows), err)
	}
	// SearchSources resolves the overlay FIRST: the census matches once (its main
	// row and branch shadow are one source), the register's main title no longer
	// matches on the branch, and its branch title matches on the branch only.
	if got := searchIDs(branchID, "1880"); !reflect.DeepEqual(got, []uuid.UUID{census.ID}) {
		t.Errorf("branch SearchSources(1880) = %v, want [census] exactly once", got)
	}
	if got := searchIDs(branchID, "parish"); len(got) != 0 {
		t.Errorf("branch SearchSources(parish) = %v, want none (the branch retitled it)", got)
	}
	if got := searchIDs(branchID, "baptism"); !reflect.DeepEqual(got, []uuid.UUID{register.ID}) {
		t.Errorf("branch SearchSources(baptism) = %v, want [register]", got)
	}
	if got := searchIDs(main, "baptism"); len(got) != 0 {
		t.Errorf("main SearchSources(baptism) = %v, want none", got)
	}
	if got := searchIDs(branchID, "diary"); !reflect.DeepEqual(got, []uuid.UUID{diary.ID}) {
		t.Errorf("branch SearchSources(diary) = %v, want [diary]", got)
	}
	if got := searchIDs(branchID, "enumerator"); !reflect.DeepEqual(got, []uuid.UUID{census.ID}) {
		t.Errorf("branch SearchSources(enumerator) = %v, want [census] (author is unchanged)", got)
	}
	if got, err := readStore.GetSourceExternalIDs(ctx, branchID, census.ID); err != nil || len(got) != 2 || got[0].Value != "BRANCH-1" || got[1].Value != "BRANCH-2" {
		t.Errorf("branch GetSourceExternalIDs = %+v (err=%v), want [BRANCH-1 BRANCH-2]", got, err)
	}
	if got, err := readStore.GetNote(ctx, branchID, note.ID); err != nil || got == nil || got.Text != "Revised on the branch" {
		t.Errorf("branch GetNote = %+v (err=%v), want the shadow's text", got, err)
	}
	if got, err := readStore.GetNote(ctx, main, branchNote.ID); err != nil || got != nil {
		t.Errorf("main GetNote(branch-only) = %+v (err=%v), want absent", got, err)
	}
	if rows, total, err := readStore.ListNotes(ctx, branchOpts); err != nil || total != 2 || len(rows) != 2 {
		t.Errorf("branch ListNotes = total %d, %d rows (err=%v), want 2", total, len(rows), err)
	}

	// --- Step 3: re-point the death citation at the census on the branch. The
	// counts move and the title re-denormalizes, all through the branch; the
	// per-source list follows the WINNING row. ---
	project("branch re-point", branchID,
		domain.NewCitationUpdated(deathCite.ID, map[string]any{"source_id": census.ID.String()}),
	)
	mainView("after re-point")
	if got, err := readStore.GetCitation(ctx, branchID, deathCite.ID); err != nil || got == nil || got.SourceID != census.ID || got.SourceTitle != "Census 1880 (Revised)" {
		t.Errorf("branch GetCitation(death) after re-point = %+v (err=%v), want the census with its branch title", got, err)
	}
	if got, err := readStore.GetCitationsForSource(ctx, branchID, register.ID); err != nil || len(got) != 0 {
		t.Errorf("branch GetCitationsForSource(register) after re-point = %+v (err=%v), want none", got, err)
	}
	if got, err := readStore.GetSource(ctx, branchID, register.ID); err != nil || got == nil || got.CitationCount != 0 {
		t.Errorf("branch GetSource(register) after re-point = %+v (err=%v), want 0 citations", got, err)
	}
	if got, err := readStore.GetSource(ctx, branchID, census.ID); err != nil || got == nil || got.CitationCount != 3 {
		t.Errorf("branch GetSource(census) after re-point = %+v (err=%v), want 3 citations", got, err)
	}

	// --- Step 4: deletes. A citation and a note are tombstoned; deleting the census
	// on the branch cascades to its external IDs and every citation of it the
	// branch sees (the re-pointed death citation and the branch-only burial one),
	// on the branch only. ---
	project("branch deletes", branchID,
		domain.NewCitationDeleted(birthCite.ID, "branch hypothesis"),
		domain.NewNoteDeleted(note.ID, "branch hypothesis"),
		domain.NewSourceDeleted(census.ID, "branch hypothesis"),
	)
	mainView("after branch deletes")
	if got, err := readStore.GetSource(ctx, branchID, census.ID); err != nil || got != nil {
		t.Errorf("branch GetSource(census) after delete = %+v (err=%v), want tombstoned", got, err)
	}
	if got, err := readStore.GetSourceExternalIDs(ctx, branchID, census.ID); err != nil || len(got) != 0 {
		t.Errorf("branch GetSourceExternalIDs after source delete = %+v (err=%v), want none", got, err)
	}
	for _, id := range []uuid.UUID{birthCite.ID, deathCite.ID, burialCite.ID} {
		if got, err := readStore.GetCitation(ctx, branchID, id); err != nil || got != nil {
			t.Errorf("branch GetCitation(%s) after deletes = %+v (err=%v), want tombstoned", id, got, err)
		}
	}
	if got, err := readStore.GetCitationsForPerson(ctx, branchID, subject.ID); err != nil || len(got) != 0 {
		t.Errorf("branch GetCitationsForPerson after deletes = %+v (err=%v), want none", got, err)
	}
	if _, total, err := readStore.ListCitations(ctx, branchOpts); err != nil || total != 0 {
		t.Errorf("branch ListCitations total after deletes = %d (err=%v), want 0", total, err)
	}
	if got := searchIDs(branchID, "1880"); len(got) != 0 {
		t.Errorf("branch SearchSources(1880) after delete = %v, want none", got)
	}
	if _, total, err := readStore.ListSources(ctx, branchOpts); err != nil || total != 2 {
		t.Errorf("branch ListSources total after delete = %d (err=%v), want 2 (register shadow, diary)", total, err)
	}
	if got, err := readStore.GetNote(ctx, branchID, note.ID); err != nil || got != nil {
		t.Errorf("branch GetNote after delete = %+v (err=%v), want tombstoned", got, err)
	}
	if _, total, err := readStore.ListNotes(ctx, branchOpts); err != nil || total != 1 {
		t.Errorf("branch ListNotes total after delete = %d (err=%v), want 1", total, err)
	}

	// A save after a delete clears the tombstone.
	mainNote, err := readStore.GetNote(ctx, main, note.ID)
	if err != nil || mainNote == nil {
		t.Fatalf("main GetNote: %+v (err=%v)", mainNote, err)
	}
	if err := readStore.SaveNote(ctx, branchID, mainNote); err != nil {
		t.Fatalf("branch SaveNote over tombstone: %v", err)
	}
	if got, err := readStore.GetNote(ctx, branchID, note.ID); err != nil || got == nil {
		t.Errorf("branch GetNote after re-save = %+v (err=%v), want it back", got, err)
	}

	// --- Step 5: deleting the branch purges its evidence rows; the branch id then
	// resolves to main exactly. ---
	project("delete branch", main, domain.NewBranchDeleted(branch.ID))
	mainView("after purge")
	if got, err := readStore.GetSource(ctx, branchID, census.ID); err != nil || got == nil || got.Title != "Census 1880" || got.CitationCount != 1 {
		t.Errorf("purged branch GetSource(census) = %+v (err=%v), want main's row", got, err)
	}
	if got, err := readStore.GetSourceExternalIDs(ctx, branchID, census.ID); err != nil || len(got) != 1 || got[0].Value != "MAIN-1" {
		t.Errorf("purged branch GetSourceExternalIDs = %+v (err=%v), want main's [MAIN-1]", got, err)
	}
	if got, err := readStore.GetCitation(ctx, branchID, burialCite.ID); err != nil || got != nil {
		t.Errorf("purged branch GetCitation(burial) = %+v (err=%v), want absent", got, err)
	}
	if got, err := readStore.GetCitation(ctx, branchID, birthCite.ID); err != nil || got == nil || got.Page != "12" {
		t.Errorf("purged branch GetCitation(birth) = %+v (err=%v), want main's page 12", got, err)
	}
	if _, total, err := readStore.ListSources(ctx, branchOpts); err != nil || total != 2 {
		t.Errorf("purged branch ListSources total = %d (err=%v), want main's 2", total, err)
	}
	if _, total, err := readStore.ListCitations(ctx, branchOpts); err != nil || total != 2 {
		t.Errorf("purged branch ListCitations total = %d (err=%v), want main's 2", total, err)
	}
	if got, err := readStore.GetNote(ctx, branchID, note.ID); err != nil || got == nil || got.Text != "Seen in the register" {
		t.Errorf("purged branch GetNote = %+v (err=%v), want main's text", got, err)
	}
	if _, total, err := readStore.ListNotes(ctx, branchOpts); err != nil || total != 1 {
		t.Errorf("purged branch ListNotes total = %d (err=%v), want main's 1", total, err)
	}
	if got := searchIDs(branchID, "parish"); !reflect.DeepEqual(got, []uuid.UUID{register.ID}) {
		t.Errorf("purged branch SearchSources(parish) = %v, want [register]", got)
	}
}

// TestBranchScenario_MediaOverlay runs the #759 media scenario against the
// in-memory backend; the scenario body is identical across all three backends.
func TestBranchScenario_MediaOverlay(t *testing.T) {
	readStore := memory.NewReadModelStore()
	stored := func(t *testing.T, branch domain.BranchID, id uuid.UUID) ([]byte, []byte, bool) {
		t.Helper()
		return memory.StoredMediaBytes(readStore, branch, id)
	}
	runBranchMediaScenario(t, readStore, memory.NewBranchStore(), stored)
}

// storedMediaBytes reports the bytes physically stored on the (id, branch) media
// row, and whether that row exists at all (tombstones included). It reads the
// backend's storage directly, which is what lets the scenario prove the blob
// rule rather than infer it from the resolved reads.
type storedMediaBytes func(t *testing.T, branch domain.BranchID, id uuid.UUID) (file, thumb []byte, present bool)

// runBranchMediaScenario is the backend-agnostic media scenario for sub-issue D
// of #676 (#759). Each backend package carries an identical copy (there is no
// shared test harness in this repo); keeping the assertions byte-identical is
// the DB-001 parity guarantee.
//
// Shape: two people and three media items on main, seeded through the
// projector. A branch retitles one item and must store NO copy of its bytes
// while still reading main's through GetMediaWithData and GetMediaThumbnail;
// uploads its own item (which owns its bytes); re-links, deletes and
// cascade-deletes items on the branch only. A second branch then keeps a live
// shadow of an item main deletes, which must keep that shadow's bytes until the
// branch is purged. Last, purging the first branch resolves it back to main.
func runBranchMediaScenario(t *testing.T, readStore repository.ReadModelStore, branchStore repository.BranchStore, stored storedMediaBytes) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, branchStore)
	main := domain.MainBranchID

	project := func(label string, branchID domain.BranchID, events ...domain.Event) {
		t.Helper()
		for i, ev := range events {
			if err := projector.Project(ctx, ev, int64(i+2), branchID); err != nil {
				t.Fatalf("%s: project %s: %v", label, ev.EventType(), err)
			}
		}
	}
	newMedia := func(title string, owner uuid.UUID, file, thumb string) *domain.Media {
		m := domain.NewMedia(title, "person", owner)
		m.MimeType = "image/jpeg"
		m.MediaType = domain.MediaPhoto
		m.Filename = title + ".jpg"
		m.FileData = []byte(file)
		m.ThumbnailData = []byte(thumb)
		m.FileSize = int64(len(file))
		return m
	}
	listIDs := func(branchID domain.BranchID, owner uuid.UUID) map[uuid.UUID]bool {
		t.Helper()
		got, total, err := readStore.ListMediaForEntity(ctx, "person", owner, repository.ListOptions{Limit: 100, BranchID: branchID})
		if err != nil {
			t.Fatalf("ListMediaForEntity: %v", err)
		}
		if total != len(got) {
			t.Errorf("ListMediaForEntity total = %d, want %d (the page size)", total, len(got))
		}
		ids := make(map[uuid.UUID]bool, len(got))
		for _, m := range got {
			if m.FileData != nil || m.ThumbnailData != nil {
				t.Errorf("ListMediaForEntity returned bytes for %s; it must read metadata only", m.ID)
			}
			ids[m.ID] = true
		}
		return ids
	}
	wantBytes := func(label string, branchID domain.BranchID, id uuid.UUID, file, thumb string) {
		t.Helper()
		got, err := readStore.GetMediaWithData(ctx, branchID, id)
		if err != nil || got == nil {
			t.Fatalf("%s: GetMediaWithData = %+v (err=%v), want the item", label, got, err)
		}
		if string(got.FileData) != file || string(got.ThumbnailData) != thumb {
			t.Errorf("%s: GetMediaWithData bytes = %q/%q, want %q/%q", label, got.FileData, got.ThumbnailData, file, thumb)
		}
		th, err := readStore.GetMediaThumbnail(ctx, branchID, id)
		if err != nil || string(th) != thumb {
			t.Errorf("%s: GetMediaThumbnail = %q (err=%v), want %q", label, th, err, thumb)
		}
	}
	wantGone := func(label string, branchID domain.BranchID, id uuid.UUID) {
		t.Helper()
		if got, err := readStore.GetMedia(ctx, branchID, id); err != nil || got != nil {
			t.Errorf("%s: GetMedia = %+v (err=%v), want absent", label, got, err)
		}
		if got, err := readStore.GetMediaWithData(ctx, branchID, id); err != nil || got != nil {
			t.Errorf("%s: GetMediaWithData = %+v (err=%v), want absent", label, got, err)
		}
		if th, err := readStore.GetMediaThumbnail(ctx, branchID, id); err != nil || th != nil {
			t.Errorf("%s: GetMediaThumbnail = %q (err=%v), want nil", label, th, err)
		}
	}
	wantNoCopy := func(label string, branchID domain.BranchID, id uuid.UUID) {
		t.Helper()
		file, thumb, present := stored(t, branchID, id)
		if !present {
			t.Fatalf("%s: no stored row for the branch; expected a metadata shadow", label)
		}
		if file != nil || thumb != nil {
			t.Errorf("%s: branch row stores %d file / %d thumbnail bytes, want none (blobs are shared, never copied)",
				label, len(file), len(thumb))
		}
	}

	// --- Step 1: seed main. ---
	alex := domain.NewPerson("Alex", "Original")
	blair := domain.NewPerson("Blair", "Original")
	portrait := newMedia("portrait", alex.ID, "PORTRAIT-FILE", "PORTRAIT-THUMB")
	letter := newMedia("letter", alex.ID, "LETTER-FILE", "LETTER-THUMB")
	deed := newMedia("deed", blair.ID, "DEED-FILE", "DEED-THUMB")
	project("seed main", main,
		domain.NewPersonCreated(alex),
		domain.NewPersonCreated(blair),
		domain.NewMediaCreated(portrait),
		domain.NewMediaCreated(letter),
		domain.NewMediaCreated(deed),
	)

	branch, err := domain.NewBranch("media-scope", "media metadata must fork", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	project("create branch", main, domain.NewBranchCreated(branch))
	branchID := domain.BranchID(branch.ID)

	mainView := func(label string) {
		t.Helper()
		if got, err := readStore.GetMedia(ctx, main, portrait.ID); err != nil || got == nil || got.Title != "portrait" || got.FileData != nil {
			t.Errorf("%s: main GetMedia(portrait) = %+v (err=%v), want the seeded title and no bytes", label, got, err)
		}
		wantBytes(label+": main portrait", main, portrait.ID, "PORTRAIT-FILE", "PORTRAIT-THUMB")
		if file, _, present := stored(t, main, portrait.ID); !present || string(file) != "PORTRAIT-FILE" {
			t.Errorf("%s: main portrait row stores %q (present=%v), want its own bytes", label, file, present)
		}
	}
	mainView("baseline")
	if got := listIDs(main, alex.ID); len(got) != 2 || !got[portrait.ID] || !got[letter.ID] {
		t.Errorf("baseline: main ListMediaForEntity(alex) = %v, want portrait and letter", got)
	}

	// Before the branch writes anything it resolves to main and stores nothing.
	wantBytes("unwritten branch", branchID, portrait.ID, "PORTRAIT-FILE", "PORTRAIT-THUMB")
	if _, _, present := stored(t, branchID, portrait.ID); present {
		t.Errorf("unwritten branch: a branch row exists before any branch write")
	}

	// --- Step 2: a branch METADATA edit stores no copy of the bytes. ---
	project("branch retitle", branchID, domain.NewMediaUpdated(portrait.ID, map[string]any{
		"title": "portrait (branch reading)", "crop_left": 5,
	}))
	wantNoCopy("after branch retitle", branchID, portrait.ID)
	if got, err := readStore.GetMedia(ctx, branchID, portrait.ID); err != nil || got == nil ||
		got.Title != "portrait (branch reading)" || got.CropLeft == nil || *got.CropLeft != 5 || got.FileData != nil {
		t.Errorf("branch GetMedia(portrait) = %+v (err=%v), want the branch title and crop, no bytes", got, err)
	}
	wantBytes("branch portrait after retitle", branchID, portrait.ID, "PORTRAIT-FILE", "PORTRAIT-THUMB")
	if got, err := readStore.GetMediaWithData(ctx, branchID, portrait.ID); err != nil || got == nil || got.Title != "portrait (branch reading)" {
		t.Errorf("branch GetMediaWithData(portrait) = %+v (err=%v), want the branch metadata beside main's bytes", got, err)
	}
	mainView("after branch retitle")

	// Even a caller that hands SaveMedia the full record, bytes included, cannot
	// copy them into the shadow row.
	full, err := readStore.GetMediaWithData(ctx, branchID, portrait.ID)
	if err != nil || full == nil {
		t.Fatalf("branch GetMediaWithData(portrait): %+v (err=%v)", full, err)
	}
	full.Description = "annotated on the branch"
	if err := readStore.SaveMedia(ctx, branchID, full); err != nil {
		t.Fatalf("branch SaveMedia with bytes: %v", err)
	}
	wantNoCopy("after branch SaveMedia with bytes", branchID, portrait.ID)
	wantBytes("branch portrait after full save", branchID, portrait.ID, "PORTRAIT-FILE", "PORTRAIT-THUMB")

	// --- Step 3: a branch upload is a new id that owns its bytes. ---
	scan := newMedia("scan", alex.ID, "SCAN-FILE", "SCAN-THUMB")
	project("branch upload", branchID, domain.NewMediaCreated(scan))
	if file, thumb, present := stored(t, branchID, scan.ID); !present || string(file) != "SCAN-FILE" || string(thumb) != "SCAN-THUMB" {
		t.Errorf("branch upload row stores %q/%q (present=%v), want its own bytes", file, thumb, present)
	}
	project("branch retitle upload", branchID, domain.NewMediaUpdated(scan.ID, map[string]any{"title": "scan (retitled)"}))
	wantBytes("branch upload after retitle", branchID, scan.ID, "SCAN-FILE", "SCAN-THUMB")
	if got, err := readStore.GetMedia(ctx, main, scan.ID); err != nil || got != nil {
		t.Errorf("main GetMedia(branch upload) = %+v (err=%v), want absent", got, err)
	}
	if got := listIDs(branchID, alex.ID); len(got) != 3 || !got[scan.ID] {
		t.Errorf("branch ListMediaForEntity(alex) = %v, want portrait, letter and scan", got)
	}
	if got := listIDs(main, alex.ID); len(got) != 2 || got[scan.ID] {
		t.Errorf("main ListMediaForEntity(alex) after branch upload = %v, want portrait and letter only", got)
	}

	// --- Step 4: re-link an item on the branch; the entity filter follows the
	// winning row. ---
	relinked, err := readStore.GetMedia(ctx, branchID, letter.ID)
	if err != nil || relinked == nil {
		t.Fatalf("branch GetMedia(letter): %+v (err=%v)", relinked, err)
	}
	relinked.EntityID = blair.ID
	if err := readStore.SaveMedia(ctx, branchID, relinked); err != nil {
		t.Fatalf("branch re-link: %v", err)
	}
	wantNoCopy("after branch re-link", branchID, letter.ID)
	if got := listIDs(branchID, alex.ID); got[letter.ID] {
		t.Errorf("branch ListMediaForEntity(alex) still lists the re-linked letter: %v", got)
	}
	if got := listIDs(branchID, blair.ID); len(got) != 2 || !got[letter.ID] || !got[deed.ID] {
		t.Errorf("branch ListMediaForEntity(blair) = %v, want letter and deed", got)
	}
	if got := listIDs(main, alex.ID); !got[letter.ID] {
		t.Errorf("main ListMediaForEntity(alex) lost the letter after a branch re-link: %v", got)
	}
	wantBytes("branch letter after re-link", branchID, letter.ID, "LETTER-FILE", "LETTER-THUMB")

	// --- Step 5: a branch tombstone hides the item and never touches main's bytes. ---
	project("branch delete", branchID, domain.NewMediaDeleted(portrait.ID, "branch hypothesis"))
	wantGone("branch after delete", branchID, portrait.ID)
	wantNoCopy("branch tombstone", branchID, portrait.ID)
	mainView("after branch delete")

	// --- Step 6: deleting a person on the branch cascades to the media the
	// branch shows for them (the re-linked letter and the deed), branch only. ---
	project("branch delete person", branchID, domain.NewPersonDeleted(blair.ID, "branch hypothesis"))
	wantGone("branch letter after cascade", branchID, letter.ID)
	wantGone("branch deed after cascade", branchID, deed.ID)
	wantBytes("main letter after branch cascade", main, letter.ID, "LETTER-FILE", "LETTER-THUMB")
	wantBytes("main deed after branch cascade", main, deed.ID, "DEED-FILE", "DEED-THUMB")
	if _, _, present := stored(t, branchID, deed.ID); !present {
		t.Errorf("branch cascade wrote no tombstone for the deed")
	}
	wantNoCopy("branch deed tombstone", branchID, deed.ID)

	// --- Step 7: main deletes an item a second branch still shows through a live
	// shadow; the shadow keeps the shared bytes. ---
	other, err := domain.NewBranch("media-keeper", "keeps a shadow", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	project("create second branch", main, domain.NewBranchCreated(other))
	otherID := domain.BranchID(other.ID)
	project("second branch retitle", otherID, domain.NewMediaUpdated(letter.ID, map[string]any{"title": "letter (kept)"}))
	wantNoCopy("second branch shadow", otherID, letter.ID)

	project("main delete letter", main, domain.NewMediaDeleted(letter.ID, "mainline cleanup"))
	wantGone("main letter after main delete", main, letter.ID)
	if got := listIDs(main, alex.ID); got[letter.ID] {
		t.Errorf("main ListMediaForEntity(alex) still lists the deleted letter: %v", got)
	}
	wantBytes("second branch letter after main delete", otherID, letter.ID, "LETTER-FILE", "LETTER-THUMB")
	if got, err := readStore.GetMedia(ctx, otherID, letter.ID); err != nil || got == nil || got.Title != "letter (kept)" {
		t.Errorf("second branch GetMedia(letter) after main delete = %+v (err=%v), want its shadow", got, err)
	}
	fresh, err := domain.NewBranch("media-fresh", "no rows of its own", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	project("create fresh branch", main, domain.NewBranchCreated(fresh))
	wantGone("fresh branch letter after main delete", domain.BranchID(fresh.ID), letter.ID)

	// A metadata edit the fresh branch read before main's delete but saves after
	// it (two concurrent requests) carries no bytes.
	freshID := domain.BranchID(fresh.ID)
	staleDeed, err := readStore.GetMedia(ctx, freshID, deed.ID)
	if err != nil || staleDeed == nil {
		t.Fatalf("fresh branch GetMedia(deed) = %+v (err=%v), want main's row", staleDeed, err)
	}
	staleDeed.Title = "deed (stale edit)"

	// Main deletes an item no branch shows live (the first branch only holds a
	// tombstone of the deed): a real removal.
	project("main delete deed", main, domain.NewMediaDeleted(deed.ID, "mainline cleanup"))
	if _, _, present := stored(t, main, deed.ID); present {
		t.Errorf("main deed row survived a delete no live shadow needed")
	}

	// The late save lands on an id with no row anywhere: it must store nothing,
	// never a shadow with no bytes to show.
	if err := readStore.SaveMedia(ctx, freshID, staleDeed); err != nil {
		t.Fatalf("stale byte-less SaveMedia: %v", err)
	}
	if file, _, present := stored(t, freshID, deed.ID); present {
		t.Errorf("stale byte-less branch save stored a row (%d file bytes) for an item main deleted", len(file))
	}
	wantGone("fresh branch deed after stale save", freshID, deed.ID)

	// Purging the second branch releases the letter's main row and its bytes.
	project("delete second branch", main, domain.NewBranchDeleted(other.ID))
	if _, _, present := stored(t, main, letter.ID); present {
		t.Errorf("main letter tombstone survived the purge of the last branch that needed it")
	}
	if _, _, present := stored(t, otherID, letter.ID); present {
		t.Errorf("second branch letter shadow survived its purge")
	}

	// --- Step 8: purging the first branch resolves it back to main. ---
	project("delete branch", main, domain.NewBranchDeleted(branch.ID))
	mainView("after purge")
	for _, id := range []uuid.UUID{portrait.ID, letter.ID, deed.ID, scan.ID} {
		if _, _, present := stored(t, branchID, id); present {
			t.Errorf("purged branch still stores a row for %s", id)
		}
	}
	if got, err := readStore.GetMedia(ctx, branchID, portrait.ID); err != nil || got == nil || got.Title != "portrait" {
		t.Errorf("purged branch GetMedia(portrait) = %+v (err=%v), want main's row", got, err)
	}
	wantBytes("purged branch portrait", branchID, portrait.ID, "PORTRAIT-FILE", "PORTRAIT-THUMB")
	wantGone("purged branch upload", branchID, scan.ID)
	if got := listIDs(branchID, alex.ID); len(got) != 1 || !got[portrait.ID] {
		t.Errorf("purged branch ListMediaForEntity(alex) = %v, want main's portrait only", got)
	}
}

// TestBranchScenario_GPSOverlay runs the #760 GPS artifact scenario against the
// in-memory backend; the scenario body is identical across all three backends.
func TestBranchScenario_GPSOverlay(t *testing.T) {
	runBranchGPSScenario(t, memory.NewReadModelStore(), memory.NewBranchStore())
}

// runBranchGPSScenario is the backend-agnostic GPS artifact scenario for
// sub-issue E of #676 (#760): evidence analyses, evidence conflicts, research
// logs and proof summaries. Each backend package carries an identical copy
// (there is no shared test harness in this repo); keeping the assertions
// byte-identical is the DB-001 parity guarantee.
//
// Shape: a person and a family on main with two disagreeing birth analyses, the
// open evidence conflict between them, a research log and a proof summary, all
// seeded through the projector. A branch then edits an analysis, adds one,
// RESOLVES main's conflict (ListUnresolvedConflicts must resolve the overlay
// before it filters on status, so the branch no longer lists the conflict while
// main still does), edits and adds research logs, and re-points the proof
// summary at the family (the per-subject and per-fact lists match the winning
// row). Deletes follow: branch tombstones of a main analysis and of main's
// conflict, then the person itself, which cascades every GPS artifact about the
// person on the branch only, then the family's proof and a branch-only log.
// Every step asserts main is untouched. Last, deleting the branch purges its GPS
// rows so the branch id resolves to main again.
func runBranchGPSScenario(t *testing.T, readStore repository.ReadModelStore, branchStore repository.BranchStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, branchStore)
	main := domain.MainBranchID

	project := func(label string, branchID domain.BranchID, events ...domain.Event) {
		t.Helper()
		for i, ev := range events {
			if err := projector.Project(ctx, ev, int64(i+2), branchID); err != nil {
				t.Fatalf("%s: project %s: %v", label, ev.EventType(), err)
			}
		}
	}
	type lister func(repository.ListOptions) (int, error)
	totals := func(branchID domain.BranchID) [4]int {
		t.Helper()
		opts := repository.ListOptions{Limit: 100, BranchID: branchID}
		var out [4]int
		for i, list := range []lister{
			func(o repository.ListOptions) (int, error) {
				_, n, err := readStore.ListEvidenceAnalyses(ctx, o)
				return n, err
			},
			func(o repository.ListOptions) (int, error) {
				_, n, err := readStore.ListEvidenceConflicts(ctx, o)
				return n, err
			},
			func(o repository.ListOptions) (int, error) {
				_, n, err := readStore.ListResearchLogs(ctx, o)
				return n, err
			},
			func(o repository.ListOptions) (int, error) {
				_, n, err := readStore.ListProofSummaries(ctx, o)
				return n, err
			},
		} {
			n, err := list(opts)
			if err != nil {
				t.Fatalf("list GPS artifacts: %v", err)
			}
			out[i] = n
		}
		return out
	}
	unresolved := func(branchID domain.BranchID) []uuid.UUID {
		t.Helper()
		got, err := readStore.ListUnresolvedConflicts(ctx, branchID)
		if err != nil {
			t.Fatalf("ListUnresolvedConflicts: %v", err)
		}
		ids := make([]uuid.UUID, 0, len(got))
		for _, c := range got {
			ids = append(ids, c.ID)
		}
		return ids
	}
	analysesForFact := func(branchID domain.BranchID, factType domain.FactType, subject uuid.UUID) int {
		t.Helper()
		got, err := readStore.GetAnalysesForFact(ctx, branchID, factType, subject)
		if err != nil {
			t.Fatalf("GetAnalysesForFact: %v", err)
		}
		return len(got)
	}
	analysesBySubject := func(branchID domain.BranchID, subject uuid.UUID) int {
		t.Helper()
		got, err := readStore.GetAnalysesBySubject(ctx, branchID, subject)
		if err != nil {
			t.Fatalf("GetAnalysesBySubject: %v", err)
		}
		return len(got)
	}
	conflictsFor := func(branchID domain.BranchID, subject uuid.UUID) []repository.EvidenceConflictReadModel {
		t.Helper()
		got, err := readStore.GetConflictsForSubject(ctx, branchID, subject)
		if err != nil {
			t.Fatalf("GetConflictsForSubject: %v", err)
		}
		return got
	}
	logsFor := func(branchID domain.BranchID, subject uuid.UUID) int {
		t.Helper()
		got, err := readStore.GetResearchLogsForSubject(ctx, branchID, subject)
		if err != nil {
			t.Fatalf("GetResearchLogsForSubject: %v", err)
		}
		return len(got)
	}
	proofsBySubject := func(branchID domain.BranchID, subject uuid.UUID) int {
		t.Helper()
		got, err := readStore.GetProofSummariesBySubject(ctx, branchID, subject)
		if err != nil {
			t.Fatalf("GetProofSummariesBySubject: %v", err)
		}
		return len(got)
	}
	proofsForFact := func(branchID domain.BranchID, factType domain.FactType, subject uuid.UUID) int {
		t.Helper()
		got, err := readStore.GetProofSummariesForFact(ctx, branchID, factType, subject)
		if err != nil {
			t.Fatalf("GetProofSummariesForFact: %v", err)
		}
		return len(got)
	}

	// --- Step 1: seed main. ---
	subject := domain.NewPerson("Alex", "Original")
	partner := domain.NewPerson("Sam", "Partner")
	family := domain.NewFamilyWithPartners(&subject.ID, &partner.ID)
	early := domain.NewEvidenceAnalysis(domain.FactPersonBirth, subject.ID, "Born 1815")
	late := domain.NewEvidenceAnalysis(domain.FactPersonBirth, subject.ID, "Born 1816")
	conflict := domain.NewEvidenceConflict(domain.FactPersonBirth, subject.ID, []uuid.UUID{early.ID, late.ID}, "Birth year disagrees")
	searchDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	log := domain.NewResearchLog(subject.ID, "person", "County Archive", "Baptisms 1810-1820", domain.ResearchOutcomeNotFound, searchDate)
	proof := domain.NewProofSummary(domain.FactPersonBirth, subject.ID, "Born 1815", "The census and the register agree")
	project("seed main", main,
		domain.NewPersonCreated(subject),
		domain.NewPersonCreated(partner),
		domain.NewFamilyCreated(family),
		domain.NewEvidenceAnalysisCreated(early),
		domain.NewEvidenceAnalysisCreated(late),
		domain.NewEvidenceConflictDetected(conflict),
		domain.NewResearchLogCreated(log),
		domain.NewProofSummaryCreated(proof),
	)

	branch, err := domain.NewBranch("gps-scope", "gps artifacts must fork", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	project("create branch", main, domain.NewBranchCreated(branch))
	branchID := domain.BranchID(branch.ID)

	// mainView asserts main's GPS artifacts are exactly what step 1 seeded.
	mainView := func(label string) {
		t.Helper()
		if got := totals(main); got != [4]int{2, 1, 1, 1} {
			t.Errorf("%s: main GPS totals = %v, want [2 1 1 1]", label, got)
		}
		if got, err := readStore.GetEvidenceAnalysis(ctx, main, early.ID); err != nil || got == nil || got.Conclusion != "Born 1815" {
			t.Errorf("%s: main GetEvidenceAnalysis(early) = %+v (err=%v), want Born 1815", label, got, err)
		}
		if got := analysesForFact(main, domain.FactPersonBirth, subject.ID); got != 2 {
			t.Errorf("%s: main GetAnalysesForFact(birth) = %d, want 2", label, got)
		}
		if got := analysesBySubject(main, subject.ID); got != 2 {
			t.Errorf("%s: main GetAnalysesBySubject = %d, want 2", label, got)
		}
		if got := unresolved(main); !reflect.DeepEqual(got, []uuid.UUID{conflict.ID}) {
			t.Errorf("%s: main ListUnresolvedConflicts = %v, want [conflict]", label, got)
		}
		if got := conflictsFor(main, subject.ID); len(got) != 1 || got[0].Status != domain.ConflictStatusOpen {
			t.Errorf("%s: main GetConflictsForSubject = %+v, want the open conflict", label, got)
		}
		if got, err := readStore.GetResearchLog(ctx, main, log.ID); err != nil || got == nil || got.Notes != "" || !got.SearchDate.Equal(searchDate) {
			t.Errorf("%s: main GetResearchLog = %+v (err=%v), want the seeded log", label, got, err)
		}
		if got := logsFor(main, subject.ID); got != 1 {
			t.Errorf("%s: main GetResearchLogsForSubject(person) = %d, want 1", label, got)
		}
		if got := logsFor(main, family.ID); got != 0 {
			t.Errorf("%s: main GetResearchLogsForSubject(family) = %d, want 0", label, got)
		}
		if got := proofsBySubject(main, subject.ID); got != 1 {
			t.Errorf("%s: main GetProofSummariesBySubject(person) = %d, want 1", label, got)
		}
		if got := proofsForFact(main, domain.FactPersonBirth, subject.ID); got != 1 {
			t.Errorf("%s: main GetProofSummariesForFact(birth) = %d, want 1", label, got)
		}
	}
	mainView("baseline")

	// Before the branch writes anything it resolves entirely to main.
	if got := totals(branchID); got != [4]int{2, 1, 1, 1} {
		t.Fatalf("fresh branch GPS totals = %v, want main's [2 1 1 1]", got)
	}
	if got := unresolved(branchID); !reflect.DeepEqual(got, []uuid.UUID{conflict.ID}) {
		t.Fatalf("fresh branch ListUnresolvedConflicts = %v, want main's conflict", got)
	}

	// --- Step 2: the branch edits, adds, resolves and re-points. ---
	death := domain.NewEvidenceAnalysis(domain.FactPersonDeath, subject.ID, "Died 1852")
	familyLog := domain.NewResearchLog(family.ID, "family", "Parish Chest", "Marriage banns", domain.ResearchOutcomeFound, searchDate)
	project("branch edits", branchID,
		domain.NewEvidenceAnalysisUpdated(early.ID, map[string]any{"conclusion": "Born 1814"}),
		domain.NewEvidenceAnalysisCreated(death),
		domain.NewEvidenceConflictResolved(conflict.ID, "The register wins", domain.ConflictStatusResolved),
		domain.NewResearchLogUpdated(log.ID, map[string]any{"notes": "Checked twice on the branch"}),
		domain.NewResearchLogCreated(familyLog),
		domain.NewProofSummaryUpdated(proof.ID, map[string]any{
			"subject_id": family.ID.String(), "fact_type": string(domain.FactFamilyMarriage),
		}),
	)
	mainView("after branch edits")

	if got := totals(branchID); got != [4]int{3, 1, 2, 1} {
		t.Errorf("branch GPS totals = %v, want [3 1 2 1]", got)
	}
	if got, err := readStore.GetEvidenceAnalysis(ctx, branchID, early.ID); err != nil || got == nil || got.Conclusion != "Born 1814" {
		t.Errorf("branch GetEvidenceAnalysis(early) = %+v (err=%v), want the shadow's Born 1814", got, err)
	}
	if got, err := readStore.GetEvidenceAnalysis(ctx, main, death.ID); err != nil || got != nil {
		t.Errorf("main GetEvidenceAnalysis(branch-only death) = %+v (err=%v), want absent", got, err)
	}
	if got := analysesForFact(branchID, domain.FactPersonBirth, subject.ID); got != 2 {
		t.Errorf("branch GetAnalysesForFact(birth) = %d, want 2", got)
	}
	if got := analysesForFact(branchID, domain.FactPersonDeath, subject.ID); got != 1 {
		t.Errorf("branch GetAnalysesForFact(death) = %d, want 1", got)
	}
	if got := analysesForFact(main, domain.FactPersonDeath, subject.ID); got != 0 {
		t.Errorf("main GetAnalysesForFact(death) = %d, want 0", got)
	}
	// The branch resolved main's conflict: resolve-then-filter, so it is not
	// listed as unresolved on the branch while main still lists it.
	if got := unresolved(branchID); len(got) != 0 {
		t.Errorf("branch ListUnresolvedConflicts = %v, want none (the branch resolved it)", got)
	}
	if got := conflictsFor(branchID, subject.ID); len(got) != 1 || got[0].Status != domain.ConflictStatusResolved || got[0].Resolution != "The register wins" {
		t.Errorf("branch GetConflictsForSubject = %+v, want the resolved shadow", got)
	}
	if got, err := readStore.GetResearchLog(ctx, branchID, log.ID); err != nil || got == nil || got.Notes != "Checked twice on the branch" {
		t.Errorf("branch GetResearchLog = %+v (err=%v), want the shadow's notes", got, err)
	}
	if got := logsFor(branchID, family.ID); got != 1 {
		t.Errorf("branch GetResearchLogsForSubject(family) = %d, want 1", got)
	}
	// The re-pointed proof lists under its new subject and fact only.
	if got := proofsBySubject(branchID, subject.ID); got != 0 {
		t.Errorf("branch GetProofSummariesBySubject(person) = %d, want 0 (re-pointed)", got)
	}
	if got := proofsBySubject(branchID, family.ID); got != 1 {
		t.Errorf("branch GetProofSummariesBySubject(family) = %d, want 1", got)
	}
	if got := proofsForFact(branchID, domain.FactFamilyMarriage, family.ID); got != 1 {
		t.Errorf("branch GetProofSummariesForFact(marriage) = %d, want 1", got)
	}
	if got := proofsForFact(branchID, domain.FactPersonBirth, subject.ID); got != 0 {
		t.Errorf("branch GetProofSummariesForFact(birth) = %d, want 0 (re-pointed)", got)
	}

	// --- Step 3: branch deletes. A tombstone hides main's analysis on the branch
	// only; then deleting the person cascades every GPS artifact about the person
	// on the branch, leaving the family's artifacts and main alone. ---
	project("branch delete analysis", branchID, domain.NewEvidenceAnalysisDeleted(late.ID, "branch hypothesis"))
	mainView("after branch analysis delete")
	if got, err := readStore.GetEvidenceAnalysis(ctx, branchID, late.ID); err != nil || got != nil {
		t.Errorf("branch GetEvidenceAnalysis(late) after delete = %+v (err=%v), want tombstoned", got, err)
	}
	if got := analysesForFact(branchID, domain.FactPersonBirth, subject.ID); got != 1 {
		t.Errorf("branch GetAnalysesForFact(birth) after delete = %d, want 1", got)
	}
	if got := totals(branchID); got != [4]int{2, 1, 2, 1} {
		t.Errorf("branch GPS totals after analysis delete = %v, want [2 1 2 1]", got)
	}
	// Evidence conflicts have no delete event; the store method tombstones too.
	if err := readStore.DeleteEvidenceConflict(ctx, branchID, conflict.ID); err != nil {
		t.Fatalf("branch DeleteEvidenceConflict: %v", err)
	}
	mainView("after branch conflict delete")
	if got, err := readStore.GetEvidenceConflict(ctx, branchID, conflict.ID); err != nil || got != nil {
		t.Errorf("branch GetEvidenceConflict after delete = %+v (err=%v), want tombstoned", got, err)
	}
	if got := conflictsFor(branchID, subject.ID); len(got) != 0 {
		t.Errorf("branch GetConflictsForSubject after delete = %+v, want none", got)
	}

	project("branch delete person", branchID, domain.NewPersonDeleted(subject.ID, "branch hypothesis"))
	mainView("after branch person delete")
	if got := totals(branchID); got != [4]int{0, 0, 1, 1} {
		t.Errorf("branch GPS totals after person delete = %v, want [0 0 1 1] (only the family's artifacts)", got)
	}
	if got := analysesBySubject(branchID, subject.ID); got != 0 {
		t.Errorf("branch GetAnalysesBySubject after person delete = %d, want 0", got)
	}
	if got := conflictsFor(branchID, subject.ID); len(got) != 0 {
		t.Errorf("branch GetConflictsForSubject after person delete = %+v, want none", got)
	}
	if got := logsFor(branchID, subject.ID); got != 0 {
		t.Errorf("branch GetResearchLogsForSubject(person) after person delete = %d, want 0", got)
	}
	if got := logsFor(branchID, family.ID); got != 1 {
		t.Errorf("branch GetResearchLogsForSubject(family) after person delete = %d, want 1", got)
	}
	if got := proofsBySubject(branchID, family.ID); got != 1 {
		t.Errorf("branch GetProofSummariesBySubject(family) after person delete = %d, want 1", got)
	}

	// Explicit deletes of the family's artifacts: a tombstone over main's
	// (re-pointed) proof summary and the removal of a branch-only research log.
	project("branch delete proof and log", branchID,
		domain.NewProofSummaryDeleted(proof.ID, "branch hypothesis"),
		domain.NewResearchLogDeleted(familyLog.ID, "branch hypothesis"),
	)
	mainView("after branch proof and log delete")
	if got := totals(branchID); got != [4]int{0, 0, 0, 0} {
		t.Errorf("branch GPS totals after every delete = %v, want [0 0 0 0]", got)
	}
	if got, err := readStore.GetProofSummary(ctx, branchID, proof.ID); err != nil || got != nil {
		t.Errorf("branch GetProofSummary after delete = %+v (err=%v), want tombstoned", got, err)
	}
	if got, err := readStore.GetResearchLog(ctx, branchID, familyLog.ID); err != nil || got != nil {
		t.Errorf("branch GetResearchLog(branch-only) after delete = %+v (err=%v), want gone", got, err)
	}

	// A save after a delete clears the tombstone.
	mainEarly, err := readStore.GetEvidenceAnalysis(ctx, main, early.ID)
	if err != nil || mainEarly == nil {
		t.Fatalf("main GetEvidenceAnalysis(early): %+v (err=%v)", mainEarly, err)
	}
	if err := readStore.SaveEvidenceAnalysis(ctx, branchID, mainEarly); err != nil {
		t.Fatalf("branch SaveEvidenceAnalysis over tombstone: %v", err)
	}
	if got, err := readStore.GetEvidenceAnalysis(ctx, branchID, early.ID); err != nil || got == nil || got.Conclusion != "Born 1815" {
		t.Errorf("branch GetEvidenceAnalysis after re-save = %+v (err=%v), want it back", got, err)
	}

	// --- Step 4: deleting the branch purges its GPS rows; the branch id then
	// resolves to main exactly. ---
	project("delete branch", main, domain.NewBranchDeleted(branch.ID))
	mainView("after purge")
	if got := totals(branchID); got != [4]int{2, 1, 1, 1} {
		t.Errorf("purged branch GPS totals = %v, want main's [2 1 1 1]", got)
	}
	if got := unresolved(branchID); !reflect.DeepEqual(got, []uuid.UUID{conflict.ID}) {
		t.Errorf("purged branch ListUnresolvedConflicts = %v, want main's conflict", got)
	}
	if got, err := readStore.GetEvidenceAnalysis(ctx, branchID, death.ID); err != nil || got != nil {
		t.Errorf("purged branch GetEvidenceAnalysis(death) = %+v (err=%v), want absent", got, err)
	}
	if got := proofsBySubject(branchID, subject.ID); got != 1 {
		t.Errorf("purged branch GetProofSummariesBySubject(person) = %d, want main's 1", got)
	}
}
