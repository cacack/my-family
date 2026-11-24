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

// RunPlaceAliases is the #1736 scenario: PlaceAliased joins a spelling to
// another place in the place_aliases table on every store. The list of places
// shows the target with the absorbed keys among its aliases and counts their
// uses once; an absorbed place keeps its own facts and names the place it
// resolves to; only the whole key is joined; the map pins every record of
// either spelling at the target, with its facts, on main and on a branch; a
// viewer sees no alias only hidden records use; an alias that would close a
// cycle is ignored; and PlaceUnaliased gives the place back (ADR-009 §3, §10,
// PL-004). store must start empty.
func RunPlaceAliases(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(store, nil)
	main := domain.MainBranchID
	str := func(s string) *string { return &s }
	now := time.Now().UTC().Truncate(time.Second)

	person := func(branch domain.BranchID, name, birth string, dead bool) repository.MapPerson {
		t.Helper()
		id := uuid.New()
		if err := store.SavePerson(ctx, branch, &repository.PersonReadModel{
			ID: id, GivenName: name, Surname: "Alias", FullName: name + " Alias",
			BirthPlace: birth, DeathEvidence: dead, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePerson(%s): %v", name, err)
		}
		return repository.MapPerson{ID: id, GivenName: name, Surname: "Alias", LivingFacts: domain.LivingFacts{DeathEvidence: dead}}
	}
	// Ann and Bob record one city in two spellings; Cy a place under Ann's
	// spelling; Liv, who is living, a third spelling.
	ann := person(main, "Ann", "Köln, Germany", true)
	bob := person(main, "Bob", "Cologne, Germany", true)
	person(main, "Cy", "Mülheim, Köln, Germany", true)
	liv := person(main, "Liv", "Coeln, Germany", false)

	cologne, koln, coeln := "cologne,germany", "köln,germany", "coeln,germany"
	id := repository.PlaceID
	project := func(event domain.Event, version int64) {
		t.Helper()
		if err := projector.Project(ctx, event, version, main); err != nil {
			t.Fatalf("project %s: %v", event.EventType(), err)
		}
	}
	manual := map[string]any{"source": "manual"}
	project(domain.NewPlaceUpdated(id(cologne), cologne, "Cologne, Germany", map[string]any{
		"name": "Cologne", "latitude": "N50.94", "longitude": "E6.96", "coordinates_origin": manual}), 1)
	project(domain.NewPlaceUpdated(id(koln), koln, "Köln, Germany", map[string]any{
		"latitude": "N1", "longitude": "E1", "coordinates_origin": manual}), 1)
	project(domain.NewPlaceAliased(koln, "Köln, Germany", cologne, "Cologne, Germany", id(koln), id(cologne)), 2)
	project(domain.NewPlaceAliased(coeln, "Coeln, Germany", cologne, "Cologne, Germany", id(coeln), id(cologne)), 1)

	checkAliases(t, store, "after two aliases", []repository.PlaceAliasReadModel{
		{Key: coeln, TargetKey: cologne, ResolvedKey: cologne},
		{Key: koln, TargetKey: cologne, ResolvedKey: cologne},
	})

	manualOrigin := &domain.CoordinateOrigin{Source: domain.CoordinateManual}
	resolvesTo := func(key string) *uuid.UUID { v := id(key); return &v }
	place := func(key, name string, count int) repository.PlaceReadModel {
		return repository.PlaceReadModel{ID: id(key), Key: key, Name: name, Spelling: name, Count: count}
	}
	alias := func(key, name string) repository.PlaceAlias {
		return repository.PlaceAlias{ID: id(key), Key: key, Name: name}
	}
	// Cologne counts Bob, Ann, Cy (under Köln) and Liv once each; Köln keeps
	// its own pair, ignored, and Mülheim stays under its own key.
	cologneAll := repository.PlaceReadModel{ID: id(cologne), Key: cologne, Name: "Cologne", Spelling: "Cologne, Germany", Count: 4,
		Latitude: str("N50.94"), Longitude: str("E6.96"), Origin: manualOrigin, Version: 1,
		Aliases: []repository.PlaceAlias{alias(coeln, "Coeln, Germany"), alias(koln, "Köln, Germany")}}
	coelnPlace := place(coeln, "Coeln, Germany", 1)
	coelnPlace.Version, coelnPlace.ResolvesTo = 1, resolvesTo(cologne)
	kolnPlace := place(koln, "Köln, Germany", 2)
	kolnPlace.Latitude, kolnPlace.Longitude, kolnPlace.Origin, kolnPlace.Version = str("N1"), str("E1"), manualOrigin, 2
	kolnPlace.ResolvesTo = resolvesTo(cologne)
	mulheim := place("mülheim,köln,germany", "Mülheim, Köln, Germany", 1)
	checkPlaces(t, store, main, repository.Privacy{}, []repository.PlaceReadModel{
		coelnPlace, cologneAll, place("germany", "Germany", 4), kolnPlace, mulheim,
	})

	// A viewer who does not see the living sees no Coeln: not as a place,
	// not as an alias, not in a count.
	viewer := repository.Privacy{HideLiving: true, LivingCutoff: time.Now()}
	cologneViewer := cologneAll
	cologneViewer.Count, cologneViewer.Aliases = 3, []repository.PlaceAlias{alias(koln, "Köln, Germany")}
	checkPlaces(t, store, main, viewer, []repository.PlaceReadModel{
		cologneViewer, place("germany", "Germany", 3), kolnPlace, mulheim,
	})

	// The map: every spelling at Cologne's pair under one Cologne pin, and
	// Köln's own pair unused.
	cologneOrigins := []domain.CoordinateSource{domain.CoordinateManual}
	pin := func(name string, lat, lon float64, key string, persons ...repository.MapPerson) repository.MapLocation {
		return repository.MapLocation{PlaceID: id(key), Place: name, Latitude: lat, Longitude: lon, EventType: "birth",
			Count: len(persons), Persons: persons, Origins: cologneOrigins}
	}
	checkMap(t, store, main, repository.Privacy{}, []repository.MapLocation{pin("Cologne", 50.94, 6.96, cologne, liv, bob, ann)})
	checkMap(t, store, main, viewer, []repository.MapLocation{pin("Cologne", 50.94, 6.96, cologne, bob, ann)})

	// A branch reads main's aliases: Bea, recorded only there as Köln, is at
	// Cologne too.
	branch := domain.BranchID(uuid.New())
	bea := person(branch, "Bea", "Köln, Germany", true)
	onBranch := []repository.MapPerson{liv, bob, ann, bea}
	if bea.ID.String() < ann.ID.String() {
		onBranch = []repository.MapPerson{liv, bob, bea, ann}
	}
	checkMap(t, store, branch, repository.Privacy{}, []repository.MapLocation{pin("Cologne", 50.94, 6.96, cologne, onBranch...)})

	// Cologne as an alias of Köln would close a cycle: the projector ignores
	// it, though the event still moves Cologne's stream version.
	project(domain.NewPlaceAliased(cologne, "Cologne, Germany", koln, "Köln, Germany", id(cologne), id(koln)), 2)
	checkAliases(t, store, "after a cycle", []repository.PlaceAliasReadModel{
		{Key: coeln, TargetKey: cologne, ResolvedKey: cologne},
		{Key: koln, TargetKey: cologne, ResolvedKey: cologne},
	})

	// Unaliasing Köln gives it back, with its own pair, and leaves Coeln.
	project(domain.NewPlaceUnaliased(id(koln), koln, "Köln, Germany"), 3)
	checkAliases(t, store, "after an unalias", []repository.PlaceAliasReadModel{{Key: coeln, TargetKey: cologne, ResolvedKey: cologne}})
	cologneAfter := cologneAll
	cologneAfter.Count, cologneAfter.Version, cologneAfter.Aliases = 2, 2, []repository.PlaceAlias{alias(coeln, "Coeln, Germany")}
	kolnAfter := kolnPlace
	kolnAfter.Version, kolnAfter.ResolvesTo = 3, nil
	checkPlaces(t, store, main, repository.Privacy{}, []repository.PlaceReadModel{
		coelnPlace, cologneAfter, place("germany", "Germany", 4), kolnAfter, mulheim,
	})
	kolnPin := pin("Köln, Germany", 1, 1, koln, ann)
	checkMap(t, store, main, repository.Privacy{}, []repository.MapLocation{pin("Cologne", 50.94, 6.96, cologne, liv, bob), kolnPin})
}

// checkAliases compares ListPlaceAliases with want.
func checkAliases(t *testing.T, store repository.ReadModelStore, when string, want []repository.PlaceAliasReadModel) {
	t.Helper()
	got, err := store.ListPlaceAliases(context.Background())
	if err != nil {
		t.Fatalf("ListPlaceAliases %s: %v", when, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListPlaceAliases %s =\n  %+v\nwant\n  %+v", when, got, want)
	}
}
