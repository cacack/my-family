package placetest

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// RunCoordinateOrigins is the #1563 scenario: each coordinate pair's origin
// is stored and read back on every store, a pair with none stays none (read
// as imported), an update sets or clears it, and a map location lists the
// distinct origins of the pairs it groups. store must start empty.
func RunCoordinateOrigins(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(store, nil)
	main := domain.MainBranchID
	str := func(s string) *string { return &s }
	manual := &domain.CoordinateOrigin{Source: domain.CoordinateManual}
	geocoded := &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim", Precision: domain.MatchPartial}

	// Imported (no origin) birth beside a manual death.
	ada := domain.NewPerson("Ada", "Origin")
	ada.BirthPlace, ada.DeathPlace = "Boston, MA", "Salem, MA"
	created := domain.NewPersonCreated(ada)
	created.BirthPlaceLat, created.BirthPlaceLong = str("N42.3601"), str("W71.0589")
	created.DeathPlaceLat, created.DeathPlaceLong, created.DeathPlaceOrigin = str("N42.5195"), str("W70.8967"), manual
	if err := projector.Project(ctx, created, 1, main); err != nil {
		t.Fatalf("project PersonCreated: %v", err)
	}
	// A second birth in Boston, typed in: the location groups two origins.
	bo := domain.NewPerson("Bo", "Origin")
	bo.BirthPlace = "Boston, MA"
	second := domain.NewPersonCreated(bo)
	second.BirthPlaceLat, second.BirthPlaceLong, second.BirthPlaceOrigin = str("N42.3601"), str("W71.0589"), manual
	if err := projector.Project(ctx, second, 1, main); err != nil {
		t.Fatalf("project second PersonCreated: %v", err)
	}

	family := domain.NewFamily()
	family.Partner1ID = &ada.ID
	family.MarriagePlace = "Boston, MA"
	famCreated := domain.NewFamilyCreated(family)
	famCreated.MarriagePlaceLat, famCreated.MarriagePlaceLong, famCreated.MarriagePlaceOrigin = str("N42.3601"), str("W71.0589"), geocoded
	if err := projector.Project(ctx, famCreated, 1, main); err != nil {
		t.Fatalf("project FamilyCreated: %v", err)
	}

	origins := func() (birth, death *domain.CoordinateOrigin) {
		t.Helper()
		p, err := store.GetPerson(ctx, main, ada.ID)
		if err != nil || p == nil {
			t.Fatalf("GetPerson = %v, %v", p, err)
		}
		return p.BirthPlaceOrigin, p.DeathPlaceOrigin
	}
	if birth, death := origins(); birth != nil || !reflect.DeepEqual(death, manual) {
		t.Errorf("person origins = %v, %v; want none and manual", birth, death)
	}
	persons, _, err := store.ListPersons(ctx, repository.ListOptions{Limit: 10})
	if err != nil || len(persons) != 2 {
		t.Fatalf("ListPersons = %d, %v; want 2", len(persons), err)
	}
	if listed := persons[slices.IndexFunc(persons, func(p repository.PersonReadModel) bool { return p.ID == bo.ID })]; !reflect.DeepEqual(listed.BirthPlaceOrigin, manual) {
		t.Errorf("listed birth origin = %v, want manual", listed.BirthPlaceOrigin)
	}
	f, err := store.GetFamily(ctx, main, family.ID)
	if err != nil || f == nil {
		t.Fatalf("GetFamily = %v, %v", f, err)
	}
	if !reflect.DeepEqual(f.MarriagePlaceOrigin, geocoded) {
		t.Errorf("marriage origin = %v, want %v", f.MarriagePlaceOrigin, geocoded)
	}

	checkMapOrigins(t, store, map[string][]domain.CoordinateSource{
		"birth Boston, MA": {domain.CoordinateImported, domain.CoordinateManual},
		"death Salem, MA":  {domain.CoordinateManual},
	})

	// An update sets the birth origin and clears the death one.
	update := domain.NewPersonUpdated(ada.ID, map[string]any{
		"birth_place_origin": geocoded.ChangeValue(),
		"death_place_origin": nil,
	})
	if err := projector.Project(ctx, update, 2, main); err != nil {
		t.Fatalf("project PersonUpdated: %v", err)
	}
	if birth, death := origins(); !reflect.DeepEqual(birth, geocoded) || death != nil {
		t.Errorf("person origins after update = %v, %v; want geocoded and none", birth, death)
	}
	famUpdate := domain.NewFamilyUpdated(family.ID, map[string]any{"marriage_place_origin": manual.ChangeValue()})
	if err := projector.Project(ctx, famUpdate, 2, main); err != nil {
		t.Fatalf("project FamilyUpdated: %v", err)
	}
	if f, _ := store.GetFamily(ctx, main, family.ID); f == nil || !reflect.DeepEqual(f.MarriagePlaceOrigin, manual) {
		t.Errorf("marriage origin after update = %v, want manual", f)
	}
}

// checkMapOrigins checks the origins of each main map location, keyed by
// event type and place.
func checkMapOrigins(t *testing.T, store repository.ReadModelStore, want map[string][]domain.CoordinateSource) {
	t.Helper()
	locations, err := store.GetMapLocations(context.Background(), domain.MainBranchID, repository.Privacy{})
	if err != nil {
		t.Fatalf("GetMapLocations: %v", err)
	}
	got := map[string][]domain.CoordinateSource{}
	for _, loc := range locations {
		got[loc.EventType+" "+loc.Place] = loc.Origins
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("map origins = %v, want %v", got, want)
	}
}
