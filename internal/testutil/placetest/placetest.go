// Package placetest is the shared place-matching suite every
// repository.ReadModelStore runs, so the backends cannot drift (DB-001): the
// place browser, the per-place person list, the cemetery index and the
// per-cemetery list all read the place matching key (ADR-009 §1, PL-002).
package placetest

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// matchingPerson is one person of RunMatching: born at birth or died at death,
// and buried (or cremated) at burial unless it is "", reburied there if
// reburied.
type matchingPerson struct {
	name, birth, death, burial string
	cremated, reburied         bool
}

// matchingPeople are RunMatching's persons, three spellings of Boston, two of
// Darmstadt, and a York that is not New York.
var matchingPeople = []matchingPerson{
	{name: "Ann", birth: "Boston, MA", burial: "Boston, MA"},
	{name: "Bob", birth: " BOSTON ,ma ", burial: " BOSTON ,ma "},
	{name: "Cat", death: "Boston,MA", burial: "Boston,MA", cremated: true},
	{name: "Dan", burial: "Boston, MA"},
	{name: "Eve", birth: "Darmstadt,,,Germany", burial: "Darmstadt,,,Germany", reburied: true},
	{name: "Fay", birth: "Darmstadt, Germany", burial: "Darmstadt, Germany"},
	{name: "Gus", birth: "New York, NY", burial: "New York, NY"},
	{name: "Hal", birth: "York"},
}

// listCase is a place and the given names of the persons listed under it.
type listCase struct {
	place string
	want  []string
}

// RunMatching is the #1733 scenario: spellings with one PlaceKey are one place
// in each of the four place reads, and a place lists only the records at or
// under it. store must start empty.
func RunMatching(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	burials := seedMatching(t, store)

	// The place browser: each spelling family is one entry, counting records.
	browser := []struct {
		parent string
		want   []repository.PlaceEntry
	}{
		{"", []repository.PlaceEntry{
			{Name: "Germany", FullName: "Germany", Count: 2, HasChildren: true},
			{Name: "MA", FullName: "MA", Count: 3, HasChildren: true},
			{Name: "NY", FullName: "NY", Count: 1, HasChildren: true},
			{Name: "York", FullName: "York", Count: 1},
		}},
		{"MA", []repository.PlaceEntry{{Name: "Boston", FullName: "Boston, MA", Count: 3}}},
		{" ma ", []repository.PlaceEntry{{Name: "Boston", FullName: "Boston,  ma ", Count: 3}}},
		{"Germany", []repository.PlaceEntry{{Name: "Darmstadt", FullName: "Darmstadt, Germany", Count: 2}}},
	}
	for _, tt := range browser {
		got, err := store.GetPlaceHierarchy(ctx, domain.MainBranchID, tt.parent, repository.Privacy{})
		if err != nil {
			t.Fatalf("GetPlaceHierarchy(%q): %v", tt.parent, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("GetPlaceHierarchy(%q) =\n  %+v\nwant\n  %+v", tt.parent, got, tt.want)
		}
	}

	// The per-place person list: every spelling finds the same persons, and a
	// place lists only records at or under it ("York" is not "New York, NY").
	boston := []string{"Ann", "Bob", "Cat"}
	darmstadt := []string{"Eve", "Fay"}
	checkLists(t, "GetPersonsByPlace", store.GetPersonsByPlace, []listCase{
		{"Boston, MA", boston},
		{" BOSTON ,ma ", boston},
		{"Boston,MA", boston},
		{"MA", boston},
		{"Darmstadt,,,Germany", darmstadt},
		{"Darmstadt, Germany", darmstadt},
		{"germany", darmstadt},
		{"York", []string{"Hal"}},
		{"New York, NY", []string{"Gus"}},
		{"Boston", []string{}},
		{" , ", []string{}},
	})

	// The cemetery index: one entry per key, named by the spelling the most
	// owners used. Darmstadt's two spellings tie (Eve's reburial weighs once),
	// so the bytewise smaller one names it.
	wantIndex := []repository.CemeteryEntry{
		{Place: "Boston, MA", Count: 4},
		{Place: "Darmstadt, Germany", Count: 2},
		{Place: "New York, NY", Count: 1},
	}
	index, err := store.GetCemeteryIndex(ctx, domain.MainBranchID, repository.Privacy{})
	if err != nil {
		t.Fatalf("GetCemeteryIndex: %v", err)
	}
	if !reflect.DeepEqual(index, wantIndex) {
		t.Errorf("GetCemeteryIndex = %+v, want %+v", index, wantIndex)
	}

	// The per-cemetery list: every spelling finds the same persons.
	buried := []string{"Ann", "Bob", "Cat", "Dan"}
	checkLists(t, "GetPersonsByCemetery", store.GetPersonsByCemetery, []listCase{
		{"Boston, MA", buried},
		{" BOSTON ,ma ", buried},
		{"Boston,MA", buried},
		{"Darmstadt,,,Germany", darmstadt},
		{"Darmstadt, Germany", darmstadt},
		{"York", []string{}},
		{"NY", []string{}},
		{"", []string{}},
	})

	// The burials keep their recorded text (PL-001).
	for id, place := range burials {
		if e, err := store.GetEvent(ctx, domain.MainBranchID, id); err != nil || e == nil || e.Place != place {
			t.Errorf("GetEvent(%s) = %+v, %v; want place %q as recorded (PL-001)", id, e, err, place)
		}
	}
}

// seedMatching saves matchingPeople and their burials, and returns each
// burial's event ID with its recorded place.
func seedMatching(t *testing.T, store repository.ReadModelStore) map[uuid.UUID]string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	burials := make(map[uuid.UUID]string)
	for _, p := range matchingPeople {
		id := uuid.New()
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: id, GivenName: p.name, Surname: "Placetest", FullName: p.name + " Placetest",
			BirthPlace: p.birth, DeathPlace: p.death, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePerson(%s): %v", p.name, err)
		}
		if p.burial == "" {
			continue
		}
		fact := domain.FactPersonBurial
		if p.cremated {
			fact = domain.FactPersonCremation
		}
		times := 1
		if p.reburied {
			times = 2
		}
		for range times {
			eventID := uuid.New()
			burials[eventID] = p.burial
			if err := store.SaveEvent(ctx, domain.MainBranchID, &repository.EventReadModel{
				ID: eventID, OwnerType: "person", OwnerID: id, FactType: fact,
				Place: p.burial, Version: 1, CreatedAt: now,
			}); err != nil {
				t.Fatalf("SaveEvent(%s): %v", p.name, err)
			}
		}
	}
	return burials
}

// checkLists runs list for each case and compares the given names it returns.
// Each person must come back with its places exactly as recorded: the key is
// stored beside the text, never in place of it (PL-001).
func checkLists(t *testing.T, name string,
	list func(context.Context, string, repository.ListOptions) ([]repository.PersonReadModel, int, error), cases []listCase) {
	t.Helper()
	recorded := make(map[string]matchingPerson)
	for _, p := range matchingPeople {
		recorded[p.name] = p
	}
	for _, tt := range cases {
		persons, total, err := list(context.Background(), tt.place, repository.ListOptions{Limit: 100})
		if err != nil {
			t.Fatalf("%s(%q): %v", name, tt.place, err)
		}
		got := make([]string, len(persons))
		for i, p := range persons {
			got[i] = p.GivenName
			if r := recorded[p.GivenName]; p.BirthPlace != r.birth || p.DeathPlace != r.death {
				t.Errorf("%s's places = %q, %q; want %q, %q as recorded (PL-001)", p.GivenName, p.BirthPlace, p.DeathPlace, r.birth, r.death)
			}
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, tt.want) || total != len(tt.want) {
			t.Errorf("%s(%q) = %v (total %d), want %v", name, tt.place, got, total, tt.want)
		}
	}
}

// RunHierarchy is the #895 place-hierarchy scenario. store must start empty. It
// proves the top level is the part after the LAST
// comma for places of any depth, that child levels drill down part by part
// whether the place separates parts with "," (as GEDCOM files often do) or ", ",
// and that a parent only matches whole trailing parts ("New England" is not under
// "England"). Names sort bytewise on every backend, so a lowercase name sorts
// after every uppercase one. Empty or whitespace-only parts and stray spaces
// (#925) are skipped, so no level is empty and every node that reports
// children has some.
func RunHierarchy(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()

	// One person per place; the birth/death split exercises both UNION legs.
	births := []string{
		"Buckingham House,St. James Park,London,England",
		"Windsor Castle,Berkshire,England",
		"England",
		"Springfield, IL",
		"New England",
		"Aix,en-Provence",
		". James,,England",
		"Leeds,England, ",
		"Darmstadt,,,Germany",
		" Bonn , ,Germany",
	}
	deaths := []string{
		"Kensington Palace, London, England",
		"Warsaw,Poland",
	}
	n := 0
	save := func(birth, death string) {
		n++
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID:         uuid.New(),
			GivenName:  "Person",
			Surname:    "Placeholder",
			FullName:   "Person Placeholder",
			BirthPlace: birth,
			DeathPlace: death,
			Version:    1,
		}); err != nil {
			t.Fatalf("SavePerson %d: %v", n, err)
		}
	}
	for _, p := range births {
		save(p, "")
	}
	for _, p := range deaths {
		save("", p)
	}

	tests := []struct {
		parent string
		want   []repository.PlaceEntry
	}{
		{"", []repository.PlaceEntry{
			{Name: "England", FullName: "England", Count: 6, HasChildren: true},
			{Name: "Germany", FullName: "Germany", Count: 2, HasChildren: true},
			{Name: "IL", FullName: "IL", Count: 1, HasChildren: true},
			{Name: "New England", FullName: "New England", Count: 1, HasChildren: false},
			{Name: "Poland", FullName: "Poland", Count: 1, HasChildren: true},
			{Name: "en-Provence", FullName: "en-Provence", Count: 1, HasChildren: true},
		}},
		{"England", []repository.PlaceEntry{
			{Name: ". James", FullName: ". James, England", Count: 1, HasChildren: false},
			{Name: "Berkshire", FullName: "Berkshire, England", Count: 1, HasChildren: true},
			{Name: "Leeds", FullName: "Leeds, England", Count: 1, HasChildren: false},
			{Name: "London", FullName: "London, England", Count: 2, HasChildren: true},
		}},
		{"Germany", []repository.PlaceEntry{
			{Name: "Bonn", FullName: "Bonn, Germany", Count: 1, HasChildren: false},
			{Name: "Darmstadt", FullName: "Darmstadt, Germany", Count: 1, HasChildren: false},
		}},
		{"London, England", []repository.PlaceEntry{
			{Name: "Kensington Palace", FullName: "Kensington Palace, London, England", Count: 1, HasChildren: false},
			{Name: "St. James Park", FullName: "St. James Park, London, England", Count: 1, HasChildren: true},
		}},
		{"St. James Park, London, England", []repository.PlaceEntry{
			{Name: "Buckingham House", FullName: "Buckingham House, St. James Park, London, England", Count: 1, HasChildren: false},
		}},
		{"IL", []repository.PlaceEntry{
			{Name: "Springfield", FullName: "Springfield, IL", Count: 1, HasChildren: false},
		}},
		{"Poland", []repository.PlaceEntry{
			{Name: "Warsaw", FullName: "Warsaw, Poland", Count: 1, HasChildren: false},
		}},
		{"New England", nil},
		{"Nowhere", nil},
	}
	for _, tt := range tests {
		got, err := store.GetPlaceHierarchy(ctx, domain.MainBranchID, tt.parent, repository.Privacy{})
		if err != nil {
			t.Fatalf("GetPlaceHierarchy(%q): %v", tt.parent, err)
		}
		if len(got) == 0 && len(tt.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("GetPlaceHierarchy(%q) =\n  %+v\nwant\n  %+v", tt.parent, got, tt.want)
		}
		// Each child's full_name must find its persons, including places stored
		// without a space after the commas (every place here is one person).
		for _, e := range got {
			if tt.parent == "" {
				continue
			}
			_, total, err := store.GetPersonsByPlace(ctx, e.FullName, repository.ListOptions{Limit: 100})
			if err != nil {
				t.Fatalf("GetPersonsByPlace(%q): %v", e.FullName, err)
			}
			if total != e.Count {
				t.Errorf("GetPersonsByPlace(%q) total = %d, want %d", e.FullName, total, e.Count)
			}
		}
	}

	// Walk the whole tree: no level is blank, and every node that reports
	// children returns some when asked for them by its full_name.
	var walk func(parent string)
	walk = func(parent string) {
		got, err := store.GetPlaceHierarchy(ctx, domain.MainBranchID, parent, repository.Privacy{})
		if err != nil {
			t.Fatalf("GetPlaceHierarchy(%q): %v", parent, err)
		}
		for _, e := range got {
			if strings.TrimSpace(e.Name) == "" {
				t.Errorf("GetPlaceHierarchy(%q) returned a blank level %+v", parent, e)
			}
			if !e.HasChildren {
				continue
			}
			children, err := store.GetPlaceHierarchy(ctx, domain.MainBranchID, e.FullName, repository.Privacy{})
			if err != nil {
				t.Fatalf("GetPlaceHierarchy(%q): %v", e.FullName, err)
			}
			if len(children) == 0 {
				t.Errorf("%q reports has_children but GetPlaceHierarchy(%q) is empty", e.FullName, e.FullName)
			}
			walk(e.FullName)
		}
	}
	walk("")
}
