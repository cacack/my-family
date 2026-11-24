package memory_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestMapLocations_CoordinateUpdates runs the #1604 scenario against the memory
// backend; the other backend packages run the same body (DB-001).
func TestMapLocations_CoordinateUpdates(t *testing.T) {
	runCoordinateUpdateScenario(t, memory.NewReadModelStore(), memory.NewBranchStore())
}

// runCoordinateUpdateScenario is the backend-agnostic body of the #1604 map
// test; each backend package carries an identical copy (DB-001). Coordinates
// set by a PersonUpdated reach GetMapLocations on the scope they were written
// to, and a cleared pair leaves the map.
func runCoordinateUpdateScenario(t *testing.T, readStore repository.ReadModelStore, branchStore repository.BranchStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, branchStore)

	branch, err := domain.NewBranch("coordinates", "placing the birth", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	if err := projector.Project(ctx, domain.NewBranchCreated(branch), 1, domain.MainBranchID); err != nil {
		t.Fatalf("project BranchCreated: %v", err)
	}
	branchID := domain.BranchID(branch.ID)

	person := domain.NewPerson("Robin", "Placeholder")
	person.BirthPlace, person.DeathPlace = "Westland, Realm", "Eastland, Realm"
	if err := projector.Project(ctx, domain.NewPersonCreated(person), 1, domain.MainBranchID); err != nil {
		t.Fatalf("project PersonCreated: %v", err)
	}
	pins := func(scope domain.BranchID) []string {
		t.Helper()
		locations, err := readStore.GetMapLocations(ctx, scope, repository.Privacy{})
		if err != nil {
			t.Fatalf("GetMapLocations(%s): %v", scope, err)
		}
		var got []string
		for _, loc := range locations {
			got = append(got, fmt.Sprintf("%s %s %.4f %.4f", loc.EventType, loc.Place, loc.Latitude, loc.Longitude))
		}
		slices.Sort(got)
		return got
	}

	// Written on the branch: the branch maps it, main does not.
	birth := map[string]any{"birth_place_lat": "N42.3601", "birth_place_long": "W71.0589"}
	if err := projector.Project(ctx, domain.NewPersonUpdated(person.ID, birth), 2, branchID); err != nil {
		t.Fatalf("project branch PersonUpdated: %v", err)
	}
	if got, want := pins(branchID), []string{"birth Westland, Realm 42.3601 -71.0589"}; !slices.Equal(got, want) {
		t.Errorf("branch pins = %v, want %v", got, want)
	}
	if got := pins(domain.MainBranchID); len(got) != 0 {
		t.Errorf("main pins after a branch write = %v, want none", got)
	}

	// Written on main.
	death := map[string]any{"death_place_lat": "S33.8688", "death_place_long": "E151.2093"}
	if err := projector.Project(ctx, domain.NewPersonUpdated(person.ID, death), 2, domain.MainBranchID); err != nil {
		t.Fatalf("project main PersonUpdated: %v", err)
	}
	if got, want := pins(domain.MainBranchID), []string{"death Eastland, Realm -33.8688 151.2093"}; !slices.Equal(got, want) {
		t.Errorf("main pins = %v, want %v", got, want)
	}

	// A cleared pair leaves the map.
	cleared := map[string]any{"death_place_lat": nil, "death_place_long": nil}
	if err := projector.Project(ctx, domain.NewPersonUpdated(person.ID, cleared), 3, domain.MainBranchID); err != nil {
		t.Fatalf("project clearing PersonUpdated: %v", err)
	}
	if got := pins(domain.MainBranchID); len(got) != 0 {
		t.Errorf("main pins after a clear = %v, want none", got)
	}
}
