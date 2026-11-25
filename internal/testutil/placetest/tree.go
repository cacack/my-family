package placetest

import (
	"context"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// RunPlaceTree is the #1737 scenario: the place browser walks parents over
// places resolved by key, the same on every store (ADR-009 §4). A parent no
// record names directly is a place; an alias moves a place under its target;
// an explicit parent overrides the default one and clearing it restores it;
// the projector ignores a parent or an alias that would close a cycle, in log
// order (PL-004); a place's count is the length of the list of people behind
// it; and a branch sees a place recorded only there. The tree includes a
// multibyte name and two spellings that differ only in letter case. Each
// person has one place, so a count of records is a count of people. store
// must start empty.
func RunPlaceTree(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(store, nil)
	main := domain.MainBranchID
	branch := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Second)
	id := repository.PlaceID

	people := []struct {
		branch      domain.BranchID
		name, birth string
	}{
		{main, "Ann", "Darmstadt, Hessen, Germany"},
		{main, "Ben", "Kassel, Hesse, Germany"},
		{main, "Cid", "Zürich, Schweiz"},
		{main, "Dee", "Boston, MA"},
		{main, "Eli", "boston, ma"},
		{main, "Fox", "Springfield"},
		{main, "Gil", "Chicago, Illinois, USA"},
		{branch, "Hal", "Lyon, France"},
	}
	for _, p := range people {
		if err := store.SavePerson(ctx, p.branch, &repository.PersonReadModel{
			ID: uuid.New(), GivenName: p.name, Surname: "Tree", FullName: p.name + " Tree",
			BirthPlace: p.birth, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePerson(%s): %v", p.name, err)
		}
	}
	project := func(event domain.Event, version int64) {
		t.Helper()
		if err := projector.Project(ctx, event, version, main); err != nil {
			t.Fatalf("project %s: %v", event.EventType(), err)
		}
	}
	parent := func(key, text, parentKey string, version int64) {
		t.Helper()
		project(domain.NewPlaceUpdated(id(key), key, text, map[string]any{"parent": parentKey}), version)
	}

	project(domain.NewPlaceAliased("hesse,germany", "Hesse, Germany", "hessen,germany", "Hessen, Germany",
		id("hesse,germany"), id("hessen,germany")), 1)
	parent("springfield", "Springfield", "illinois,usa", 1)
	// Illinois under Springfield would put it beneath itself: ignored.
	parent("illinois,usa", "Illinois, USA", "springfield", 1)
	// USA as an alias of Chicago would put Chicago beneath itself: ignored.
	project(domain.NewPlaceAliased("usa", "USA", "chicago,illinois,usa", "Chicago, Illinois, USA",
		id("usa"), id("chicago,illinois,usa")), 1)

	if f, err := store.GetPlaceFacts(ctx, id("illinois,usa")); err != nil || f == nil || f.ParentKey != "" || f.Version != 1 {
		t.Errorf("Illinois's facts = %+v, %v; want version 1 and no parent (PL-004)", f, err)
	}
	checkAliases(t, store, "after a cycle-closing alias", []repository.PlaceAliasReadModel{
		{Key: "hesse,germany", TargetKey: "hessen,germany", ResolvedKey: "hessen,germany"},
	})

	mainTree := []string{
		"Germany | Germany | 2",
		"  Hessen | Hessen, Germany | 2",
		"    Darmstadt | Darmstadt, Hessen, Germany | 1",
		"    Kassel | Kassel, Hesse, Germany | 1",
		"MA | MA | 2",
		"  Boston | Boston, MA | 2",
		"Schweiz | Schweiz | 1",
		"  Zürich | Zürich, Schweiz | 1",
		"USA | USA | 2",
		"  Illinois | Illinois, USA | 2",
		"    Chicago | Chicago, Illinois, USA | 1",
		"    Springfield | Springfield | 1",
	}
	checkTree(t, store, main, "main", mainTree)
	checkTree(t, store, branch, "a branch", append([]string{
		"France | France | 1",
		"  Lyon | Lyon, France | 1",
	}, mainTree...))

	// The people behind a place, its children's included.
	checkPeopleAt(t, store, main, id("germany"), "Ann", "Ben")
	checkPeopleAt(t, store, main, id("hesse,germany"), "Ann", "Ben")
	checkPeopleAt(t, store, main, id("usa"), "Fox", "Gil")
	checkPeopleAt(t, store, main, id("illinois,usa"), "Fox", "Gil")
	checkPeopleAt(t, store, main, id("boston,ma"), "Dee", "Eli")
	checkPeopleAt(t, store, main, id("zürich,schweiz"), "Cid")
	checkPeopleAt(t, store, main, id("france"))
	checkPeopleAt(t, store, branch, id("france"), "Hal")

	// The path down to a place, read by an absorbed ID.
	level, err := store.GetPlaceHierarchy(ctx, main, id("hesse,germany"), repository.Privacy{})
	if err != nil {
		t.Fatalf("GetPlaceHierarchy(Hesse): %v", err)
	}
	if len(level.Path) != 2 || level.Path[0].ID != id("germany") || level.Path[1].ID != id("hessen,germany") || len(level.Places) != 2 {
		t.Errorf("GetPlaceHierarchy(Hesse) = %+v, want Hessen's two places under the path Germany, Hessen", level)
	}

	// Clearing Springfield's parent puts it back at the top level.
	parent("springfield", "Springfield", "", 2)
	checkTree(t, store, main, "after clearing a parent", []string{
		"Germany | Germany | 2",
		"  Hessen | Hessen, Germany | 2",
		"    Darmstadt | Darmstadt, Hessen, Germany | 1",
		"    Kassel | Kassel, Hesse, Germany | 1",
		"MA | MA | 2",
		"  Boston | Boston, MA | 2",
		"Schweiz | Schweiz | 1",
		"  Zürich | Zürich, Schweiz | 1",
		"Springfield | Springfield | 1",
		"USA | USA | 1",
		"  Illinois | Illinois, USA | 1",
		"    Chicago | Chicago, Illinois, USA | 1",
	})
}

// checkTree walks the place browser on branch from the top level and compares
// one line per place, indented by depth (name, full name and count), with
// want. At every place the count equals the total of the people behind it,
// and the path of each level ends at the place it was read under.
func checkTree(t *testing.T, store repository.ReadModelStore, branch domain.BranchID, label string, want []string) {
	t.Helper()
	ctx := context.Background()
	var got []string
	var walk func(parent uuid.UUID, indent string)
	walk = func(parent uuid.UUID, indent string) {
		level, err := store.GetPlaceHierarchy(ctx, branch, parent, repository.Privacy{})
		if err != nil {
			t.Fatalf("%s: GetPlaceHierarchy(%s): %v", label, parent, err)
		}
		if parent != uuid.Nil && (len(level.Path) == 0 || level.Path[len(level.Path)-1].ID != parent) {
			t.Errorf("%s: GetPlaceHierarchy(%s) path = %+v, want it to end at the parent", label, parent, level.Path)
		}
		for _, e := range level.Places {
			got = append(got, indent+e.Name+" | "+e.FullName+" | "+strconv.Itoa(e.Count))
			_, total, err := store.GetPersonsByPlace(ctx, e.ID, repository.ListOptions{Limit: 100, BranchID: branch})
			if err != nil {
				t.Fatalf("%s: GetPersonsByPlace(%s): %v", label, e.FullName, err)
			}
			if total != e.Count {
				t.Errorf("%s: %s counts %d but lists %d people", label, e.FullName, e.Count, total)
			}
			if e.HasChildren {
				walk(e.ID, indent+"  ")
			}
		}
	}
	walk(uuid.Nil, "")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: place tree =\n  %q\nwant\n  %q", label, got, want)
	}
}

// checkPeopleAt compares the given names GetPersonsByPlace lists at place on
// branch, and its total, with want.
func checkPeopleAt(t *testing.T, store repository.ReadModelStore, branch domain.BranchID, place uuid.UUID, want ...string) {
	t.Helper()
	persons, total, err := store.GetPersonsByPlace(context.Background(), place, repository.ListOptions{Limit: 100, BranchID: branch})
	if err != nil {
		t.Fatalf("GetPersonsByPlace(%s): %v", place, err)
	}
	got := make([]string, len(persons))
	for i, p := range persons {
		got[i] = p.GivenName
	}
	sort.Strings(got)
	if want == nil {
		want = []string{}
	}
	if !reflect.DeepEqual(got, want) || total != len(want) {
		t.Errorf("GetPersonsByPlace(%s) on %s = %v (total %d), want %v", place, branch, got, total, want)
	}
}
