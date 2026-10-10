package postgres_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestPlaceHierarchy_Parity runs the #895 place-hierarchy scenario against the
// postgres backend; the sqlite package runs the same body (DB-001).
func TestPlaceHierarchy_Parity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	runPlaceHierarchyScenario(t, store)
}

// runPlaceHierarchyScenario is the backend-agnostic body of the #895
// place-hierarchy parity test. Each SQL backend package carries an identical copy
// (there is no shared test harness in this repo); keeping it byte-identical is the
// DB-001 parity guarantee. It proves the top level is the part after the LAST
// comma for places of any depth, that child levels drill down part by part
// whether the place separates parts with "," (as GEDCOM files often do) or ", ",
// and that a parent only matches whole trailing parts ("New England" is not under
// "England"). Names sort bytewise on every backend, so a lowercase name sorts
// after every uppercase one.
func runPlaceHierarchyScenario(t *testing.T, store repository.ReadModelStore) {
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
			{Name: "England", FullName: "England", Count: 4, HasChildren: true},
			{Name: "IL", FullName: "IL", Count: 1, HasChildren: true},
			{Name: "New England", FullName: "New England", Count: 1, HasChildren: false},
			{Name: "Poland", FullName: "Poland", Count: 1, HasChildren: true},
			{Name: "en-Provence", FullName: "en-Provence", Count: 1, HasChildren: true},
		}},
		{"England", []repository.PlaceEntry{
			{Name: "Berkshire", FullName: "Berkshire, England", Count: 1, HasChildren: true},
			{Name: "London", FullName: "London, England", Count: 2, HasChildren: true},
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
		got, err := store.GetPlaceHierarchy(ctx, domain.MainBranchID, tt.parent)
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
}
