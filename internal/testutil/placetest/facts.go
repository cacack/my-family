package placetest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// RunPlaceFacts is the #1735 scenario: a PlaceUpdated projects the place's
// name and pair, with its origin, into the places table on every store; the
// place list shows them; the map places every record without a pair of its
// own at its place's pair, while a record with its own pair keeps its own
// pin; a branch reads main's facts; a clear removes them; and the map and
// list of a principal who may not see a place's records show nothing of it
// (ADR-009 §2, §5, §6, §9, §10). store must start empty.
func RunPlaceFacts(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(store, nil)
	main := domain.MainBranchID
	str := func(s string) *string { return &s }
	now := time.Now().UTC().Truncate(time.Second)

	person := func(branch domain.BranchID, name, birth string, lat, long *string, dead bool) repository.MapPerson {
		t.Helper()
		id := uuid.New()
		if err := store.SavePerson(ctx, branch, &repository.PersonReadModel{
			ID: id, GivenName: name, Surname: "Facts", FullName: name + " Facts",
			BirthPlace: birth, BirthPlaceLat: lat, BirthPlaceLong: long, DeathEvidence: dead,
			Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePerson(%s): %v", name, err)
		}
		return repository.MapPerson{ID: id, GivenName: name, Surname: "Facts", LivingFacts: domain.LivingFacts{DeathEvidence: dead}}
	}
	// Ann and Bob record Boston with no pair of their own, in two spellings;
	// Cy records a pair of his own there. Liv is living, in Livingston.
	// Dee is in Salem, which has no facts.
	ann := person(main, "Ann", "Boston, MA", nil, nil, true)
	bob := person(main, "Bob", " boston ,ma ", nil, nil, true)
	cy := person(main, "Cy", "Boston, MA", str("N42.35"), str("W71.05"), true)
	liv := person(main, "Liv", "Livingston, MA", nil, nil, false)
	person(main, "Dee", "Salem, MA", nil, nil, true)

	boston, livingston := repository.PlaceID("boston,ma"), repository.PlaceID("livingston,ma")
	geocoded := map[string]any{"source": "geocoded", "provider": "Nominatim", "precision": "full"}
	update := func(id uuid.UUID, key, text string, version int64, changes map[string]any) {
		t.Helper()
		if err := projector.Project(ctx, domain.NewPlaceUpdated(id, key, text, changes), version, main); err != nil {
			t.Fatalf("project PlaceUpdated(%s): %v", key, err)
		}
	}
	update(boston, "boston,ma", "Boston, MA", 1, map[string]any{"latitude": "N42.36", "longitude": "W71.06", "coordinates_origin": geocoded})
	update(boston, "boston,ma", "Boston, MA", 2, map[string]any{"name": "Boston"})
	update(livingston, "livingston,ma", "Livingston, MA", 1, map[string]any{"latitude": "N42.4", "longitude": "W71.3", "coordinates_origin": map[string]any{"source": "manual"}})

	geocodedOrigin := &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim", Precision: domain.MatchFull}
	manualOrigin := &domain.CoordinateOrigin{Source: domain.CoordinateManual}
	bostonFacts := repository.PlaceFactsReadModel{ID: boston, Key: "boston,ma", Name: "Boston",
		Latitude: str("N42.36"), Longitude: str("W71.06"), Origin: geocodedOrigin, Version: 2}
	livingstonFacts := repository.PlaceFactsReadModel{ID: livingston, Key: "livingston,ma",
		Latitude: str("N42.4"), Longitude: str("W71.3"), Origin: manualOrigin, Version: 1}

	got, err := store.GetPlaceFacts(ctx, boston)
	if err != nil || got == nil {
		t.Fatalf("GetPlaceFacts(boston) = %v, %v", got, err)
	}
	checkFacts(t, "GetPlaceFacts(boston)", []repository.PlaceFactsReadModel{*got}, []repository.PlaceFactsReadModel{bostonFacts})
	if none, err := store.GetPlaceFacts(ctx, repository.PlaceID("salem,ma")); err != nil || none != nil {
		t.Errorf("GetPlaceFacts(salem) = %+v, %v; want none", none, err)
	}
	all, err := store.ListPlaceFacts(ctx, main)
	if err != nil {
		t.Fatalf("ListPlaceFacts: %v", err)
	}
	checkFacts(t, "ListPlaceFacts", all, []repository.PlaceFactsReadModel{bostonFacts, livingstonFacts})

	// The place list shows the display name and the pair.
	places, err := store.ListPlaces(ctx, main, repository.Privacy{})
	if err != nil {
		t.Fatalf("ListPlaces: %v", err)
	}
	wantBoston := repository.PlaceReadModel{ID: boston, Key: "boston,ma", Name: "Boston", Spelling: "Boston, MA", Count: 3,
		Latitude: str("N42.36"), Longitude: str("W71.06"), Origin: geocodedOrigin, Version: 2}
	if p := findPlace(places, boston); p == nil || !reflect.DeepEqual(*p, wantBoston) {
		t.Errorf("ListPlaces boston = %+v, want %+v", p, wantBoston)
	}

	// The map: Ann and Bob at Boston's pair under one pin, Cy at his own,
	// Liv at Livingston's, and Dee nowhere.
	imported := []domain.CoordinateSource{domain.CoordinateImported}
	bostonPin := repository.MapLocation{PlaceID: boston, Place: "Boston", Latitude: 42.36, Longitude: -71.06, EventType: "birth",
		Count: 2, Persons: []repository.MapPerson{bob, ann}, Origins: []domain.CoordinateSource{domain.CoordinateGeocoded}}
	cyPin := repository.MapLocation{PlaceID: boston, Place: "Boston", Latitude: 42.35, Longitude: -71.05, EventType: "birth",
		Count: 1, Persons: []repository.MapPerson{cy}, Origins: imported}
	livPin := repository.MapLocation{PlaceID: livingston, Place: "Livingston, MA", Latitude: 42.4, Longitude: -71.3, EventType: "birth",
		Count: 1, Persons: []repository.MapPerson{liv}, Origins: []domain.CoordinateSource{domain.CoordinateManual}}
	checkMap(t, store, main, repository.Privacy{}, []repository.MapLocation{cyPin, bostonPin, livPin})

	// A viewer who does not see the living sees nothing of Livingston:
	// neither its pin nor the place with its facts.
	viewer := repository.Privacy{HideLiving: true, LivingCutoff: time.Now()}
	checkMap(t, store, main, viewer, []repository.MapLocation{cyPin, bostonPin})
	if places, err := store.ListPlaces(ctx, main, viewer); err != nil || findPlace(places, livingston) != nil {
		t.Errorf("viewer ListPlaces holds livingston = %v (err %v), want it absent", findPlace(places, livingston), err)
	}

	// A branch reads main's facts: Bea, recorded only on the branch, is at
	// Boston's pair beside Ann and Bob.
	branch := domain.BranchID(uuid.New())
	bea := person(branch, "Bea", "Boston, MA", nil, nil, true)
	onBranch := bostonPin
	onBranch.Count, onBranch.Persons = 3, []repository.MapPerson{bob, ann, bea}
	if ann.ID.String() > bea.ID.String() {
		onBranch.Persons = []repository.MapPerson{bob, bea, ann}
	}
	checkMap(t, store, branch, repository.Privacy{}, []repository.MapLocation{cyPin, onBranch, livPin})

	// Clearing Boston's pair leaves Ann and Bob unmapped and Cy where he was.
	update(boston, "boston,ma", "Boston", 3, map[string]any{"latitude": nil, "longitude": nil, "coordinates_origin": nil})
	checkMap(t, store, main, repository.Privacy{}, []repository.MapLocation{cyPin, livPin})
}

// checkFacts compares place facts, ignoring UpdatedAt, which each store
// keeps at its own precision.
func checkFacts(t *testing.T, what string, got, want []repository.PlaceFactsReadModel) {
	t.Helper()
	got = append([]repository.PlaceFactsReadModel(nil), got...)
	for i := range got {
		if got[i].UpdatedAt.IsZero() {
			t.Errorf("%s: %s has no UpdatedAt", what, got[i].Key)
		}
		got[i].UpdatedAt = time.Time{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s =\n  %+v\nwant\n  %+v", what, got, want)
	}
}

// checkMap compares GetMapLocations on branch under privacy with want.
func checkMap(t *testing.T, store repository.ReadModelStore, branch domain.BranchID, privacy repository.Privacy, want []repository.MapLocation) {
	t.Helper()
	got, err := store.GetMapLocations(context.Background(), branch, privacy)
	if err != nil {
		t.Fatalf("GetMapLocations(%s): %v", branch, err)
	}
	for i := range got {
		for j := range got[i].Persons {
			// The living rule's reference date is the store's to derive.
			got[i].Persons[j].LivingFacts.ReferenceDate = nil
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetMapLocations(%s, %+v) =\n  %+v\nwant\n  %+v", branch, privacy, got, want)
	}
}

// findPlace returns the place with id in places, or nil.
func findPlace(places []repository.PlaceReadModel, id uuid.UUID) *repository.PlaceReadModel {
	for i := range places {
		if places[i].ID == id {
			return &places[i]
		}
	}
	return nil
}
