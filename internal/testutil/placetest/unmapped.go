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

// RunUnmappedPlaces is the #1561 scenario: ListUnmappedPlaces lists every
// place with a record of the five kinds that has no effective coordinates,
// grouped by place key, with the pairs other records there carry, the same
// on every store. "Köln" and "KÖLN" are one place, which no SQL LOWER would
// make them on SQLite; a viewer gets nothing that only hidden records give;
// a place's own pair takes its records off the list; and a place named only
// on a research branch is listed on that branch alone (ADR-009 §1, §6, §9,
// §10). store must start empty.
func RunUnmappedPlaces(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	main := domain.MainBranchID
	str := func(s string) *string { return &s }

	type place struct {
		text      string
		lat, long *string
	}
	person := func(branch domain.BranchID, name string, birth, death place, restriction domain.Restriction, dead bool) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if err := store.SavePerson(ctx, branch, &repository.PersonReadModel{
			ID: id, GivenName: name, Surname: "Unmapped", FullName: name + " Unmapped",
			BirthPlace: birth.text, BirthPlaceLat: birth.lat, BirthPlaceLong: birth.long,
			DeathPlace: death.text, DeathPlaceLat: death.lat, DeathPlaceLong: death.long,
			Restriction: restriction, DeathEvidence: dead, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePerson(%s): %v", name, err)
		}
		return id
	}
	attribute := func(owner uuid.UUID, p place) {
		t.Helper()
		if err := store.SaveAttribute(ctx, main, &repository.AttributeReadModel{
			ID: uuid.New(), PersonID: owner, FactType: domain.FactPersonResidence,
			Place: p.text, PlaceLat: p.lat, PlaceLong: p.long, Version: 1, CreatedAt: now,
		}); err != nil {
			t.Fatalf("SaveAttribute(%s): %v", p.text, err)
		}
	}

	// Ann and Bob are dead and shown; Liv is living and Pam private.
	ann := person(main, "Ann", place{"Boston, MA", nil, nil}, place{" boston, ma ", nil, nil}, "", true)
	bob := person(main, "Bob", place{"Boston,MA", str("N42.36"), str("W71.06")}, place{"Boston MA", nil, nil}, "", true)
	liv := person(main, "Liv", place{"Livingston, MA", nil, nil}, place{}, "", false)
	attribute(liv, place{"Boston, MA", nil, nil})
	person(main, "Pam", place{"Boston, MA", str("N42.30"), str("W71.00")}, place{}, "confidential", true)

	family := uuid.New()
	if err := store.SaveFamily(ctx, main, &repository.FamilyReadModel{
		ID: family, Partner1ID: &ann, Partner2ID: &bob, MarriagePlace: "Köln, Deutschland", Version: 1, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveFamily: %v", err)
	}
	if err := store.SaveEvent(ctx, main, &repository.EventReadModel{
		ID: uuid.New(), OwnerType: "person", OwnerID: ann, FactType: domain.FactPersonBurial,
		Place: "KÖLN, deutschland", PlaceLat: str("N50.94"), PlaceLong: str("E6.96"), Version: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	attribute(bob, place{"Köln, Deutschland", nil, nil})

	entry := func(key, name string, count int, suggestions ...repository.CoordinateSuggestion) repository.UnmappedPlace {
		return repository.UnmappedPlace{ID: repository.PlaceID(key), Key: key, Name: name, Spelling: name, Count: count, Suggestions: suggestions}
	}
	bobsPair := repository.CoordinateSuggestion{Latitude: "N42.36", Longitude: "W71.06", Count: 1}
	pamsPair := repository.CoordinateSuggestion{Latitude: "N42.30", Longitude: "W71.00", Count: 1}
	koeln := entry("köln,deutschland", "Köln, Deutschland", 2, repository.CoordinateSuggestion{Latitude: "N50.94", Longitude: "E6.96", Count: 1})
	bostonNoComma := entry("boston ma", "Boston MA", 1)
	livingston := entry("livingston,ma", "Livingston, MA", 1)

	// Everyone: Ann's birth and death and Liv's residence lack a pair in
	// Boston, where Bob and Pam suggest theirs; "Boston MA" is another place.
	all := []repository.UnmappedPlace{
		entry("boston,ma", "Boston, MA", 3, pamsPair, bobsPair), koeln, bostonNoComma, livingston,
	}
	checkUnmapped(t, store, main, repository.Privacy{}, all)

	// A viewer sees neither Liv nor Pam: no Livingston, no residence of
	// Liv's in the count and no pair of Pam's among the suggestions.
	viewer := repository.Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: time.Now()}
	checkUnmapped(t, store, main, viewer, []repository.UnmappedPlace{
		entry("boston,ma", "Boston, MA", 2, bobsPair), koeln, bostonNoComma,
	})

	// A place named only on a branch is listed there, not on main.
	branch := domain.BranchID(uuid.New())
	person(branch, "Bea", place{"Branchville, MA", nil, nil}, place{}, "", true)
	checkUnmapped(t, store, branch, repository.Privacy{}, []repository.UnmappedPlace{
		all[0], all[1], all[2], entry("branchville,ma", "Branchville, MA", 1), all[3],
	})
	checkUnmapped(t, store, main, repository.Privacy{}, all)

	// Setting a place's pair maps every record there that has none.
	projector := repository.NewProjector(store, nil)
	if err := projector.Project(ctx, domain.NewPlaceUpdated(repository.PlaceID("boston ma"), "boston ma", "Boston MA",
		map[string]any{"latitude": "N42.3", "longitude": "W71.1", "coordinates_origin": map[string]any{"source": "manual"}}), 1, main); err != nil {
		t.Fatalf("project PlaceUpdated: %v", err)
	}
	checkUnmapped(t, store, main, repository.Privacy{}, []repository.UnmappedPlace{all[0], koeln, livingston})
}

// checkUnmapped compares ListUnmappedPlaces on branch under privacy with want.
func checkUnmapped(t *testing.T, store repository.ReadModelStore, branch domain.BranchID, privacy repository.Privacy, want []repository.UnmappedPlace) {
	t.Helper()
	got, err := store.ListUnmappedPlaces(context.Background(), branch, privacy)
	if err != nil {
		t.Fatalf("ListUnmappedPlaces(%s, %+v): %v", branch, privacy, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListUnmappedPlaces(%s, %+v) =\n  %+v\nwant\n  %+v", branch, privacy, got, want)
	}
}
