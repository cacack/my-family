package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestMapLocations_AxisChecked_Parity runs the #801 map-coordinate scenario
// against the postgres backend; the sqlite package runs the same body (DB-001).
func TestMapLocations_AxisChecked_Parity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	runMapCoordinateScenario(t, store)
}

// runMapCoordinateScenario is the backend-agnostic body of the #801 parity
// test; each SQL backend package carries an identical copy. A coordinate whose
// direction letter belongs to the other axis, or whose value is out of range,
// is skipped rather than plotted; only the valid birth and death pins remain.
func runMapCoordinateScenario(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()

	str := func(s string) *string { return &s }
	persons := []struct {
		place, lat, lon string
		death           bool
	}{
		{"Boston", "N42.3601", "W71.0589", false},
		{"Swapped Latitude", "E42.3601", "W71.0589", false},
		{"Swapped Longitude", "N42.3601", "S71.0589", false},
		{"Out Of Range", "N95", "E10", false},
		{"Paris", "N48.8566", "E2.3522", true},
		{"Bad Death", "N48.8566", "E200", true},
	}
	for i, p := range persons {
		rm := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: "Person",
			Surname:   "Placeholder",
			FullName:  "Person Placeholder",
			Version:   1,
		}
		if p.death {
			rm.DeathPlace, rm.DeathPlaceLat, rm.DeathPlaceLong = p.place, str(p.lat), str(p.lon)
		} else {
			rm.BirthPlace, rm.BirthPlaceLat, rm.BirthPlaceLong = p.place, str(p.lat), str(p.lon)
		}
		if err := store.SavePerson(ctx, domain.MainBranchID, rm); err != nil {
			t.Fatalf("SavePerson %d: %v", i, err)
		}
	}

	got, err := store.GetMapLocations(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetMapLocations: %v", err)
	}
	type pin struct {
		place, eventType string
		lat, lon         float64
	}
	want := map[pin]bool{
		{"Boston", "birth", 42.3601, -71.0589}: true,
		{"Paris", "death", 48.8566, 2.3522}:    true,
	}
	if len(got) != len(want) {
		t.Fatalf("GetMapLocations = %+v, want only the Boston birth and Paris death pins", got)
	}
	for _, loc := range got {
		if !want[pin{loc.Place, loc.EventType, loc.Latitude, loc.Longitude}] {
			t.Errorf("unexpected pin %+v", loc)
		}
	}
}
