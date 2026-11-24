package command_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// placeEventTypes are every place event type (ADR-009 §2). PL-005 holds them
// all main-only.
var placeEventTypes = []string{"PlaceUpdated"}

// TestUpdatePlace_OneEventOnThePlaceStream shows that setting a place's pair
// appends exactly one event, on the place's own stream, and none on any
// person or family stream, while every record there reads the place's pair
// beside its own.
func TestUpdatePlace_OneEventOnThePlaceStream(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	ada, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston, MA"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Bo", DeathPlace: " boston ,ma "}); err != nil {
		t.Fatal(err)
	}
	before, err := f.eventStore.ReadAll(ctx, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}

	boston := repository.PlaceID("boston,ma")
	origin := &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim", Precision: domain.MatchFull}
	result, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{
		ID: boston, Name: command.Set(" Boston "),
		Coordinates: command.CoordinatesPatch{Latitude: command.Set("n42.3601 "), Longitude: command.Set("W71.0589"), Origin: origin},
	})
	if err != nil {
		t.Fatalf("UpdatePlace: %v", err)
	}
	if result.Version != 1 {
		t.Errorf("version = %d, want 1", result.Version)
	}

	after, err := f.eventStore.ReadAll(ctx, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	added := after[len(before):]
	if len(added) != 1 || added[0].EventType != "PlaceUpdated" || added[0].StreamID != boston || added[0].StreamType != "Place" {
		t.Fatalf("appended %+v, want one PlaceUpdated on the place's stream", added)
	}
	event, err := added[0].DecodeEvent()
	if err != nil {
		t.Fatal(err)
	}
	placeUpdated := event.(domain.PlaceUpdated)
	if placeUpdated.Key != "boston,ma" || placeUpdated.Text != "Boston, MA" {
		t.Errorf("event key and text = %q, %q; want boston,ma and Boston, MA", placeUpdated.Key, placeUpdated.Text)
	}

	facts, _ := f.readStore.GetPlaceFacts(ctx, boston)
	if facts == nil || facts.Name != "Boston" || *facts.Latitude != "N42.3601" || *facts.Longitude != "W71.0589" || *facts.Origin != *origin || facts.Version != 1 {
		t.Fatalf("facts = %+v, want Boston at N42.3601 W71.0589, geocoded, version 1", facts)
	}
	person, _ := f.readStore.GetPerson(ctx, domain.MainBranchID, ada.ID)
	if person.BirthPlace != "Boston, MA" || person.BirthPlaceLat != nil {
		t.Errorf("person after UpdatePlace = %q %v; want its text and no own pair (PL-001)", person.BirthPlace, person.BirthPlaceLat)
	}

	// The same facts again change nothing and append nothing.
	same, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: boston, Version: 1, Name: command.Set("Boston"),
		Coordinates: command.CoordinatesPatch{Latitude: command.Set("N42.3601"), Longitude: command.Set("W71.0589")}})
	if err != nil || same.Version != 1 {
		t.Errorf("unchanged UpdatePlace = %+v, %v; want version 1", same, err)
	}

	// Clearing the name and the pair removes them.
	cleared, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: boston, Version: 1, Name: command.Clear[string](),
		Coordinates: command.CoordinatesPatch{Latitude: command.Clear[string](), Longitude: command.Clear[string]()}})
	if err != nil || cleared.Version != 2 {
		t.Fatalf("clearing UpdatePlace = %+v, %v; want version 2", cleared, err)
	}
	facts, _ = f.readStore.GetPlaceFacts(ctx, boston)
	if facts == nil || facts.Name != "" || facts.Latitude != nil || facts.Longitude != nil || facts.Origin != nil {
		t.Errorf("facts after clear = %+v, want none set", facts)
	}
	if final, _ := f.eventStore.ReadAll(ctx, 0, 1000); len(final) != len(after)+1 {
		t.Errorf("log holds %d events, want %d: one more for the clear", len(final), len(after)+1)
	}
}

func TestUpdatePlace_Rejects(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	if _, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston, MA"}); err != nil {
		t.Fatal(err)
	}
	boston := repository.PlaceID("boston,ma")
	pair := command.CoordinatesPatch{Latitude: command.Set("N42"), Longitude: command.Set("W71")}

	if _, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: uuid.New(), Name: command.Set("x")}); !errors.Is(err, command.ErrPlaceNotFound) {
		t.Errorf("unknown place error = %v, want ErrPlaceNotFound", err)
	}
	if _, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: boston, Version: 3, Coordinates: pair}); !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Errorf("stale version error = %v, want ErrConcurrencyConflict", err)
	}
	for name, coords := range map[string]command.CoordinatesPatch{
		"half a pair":         {Latitude: command.Set("N42")},
		"latitude on E/W":     {Latitude: command.Set("E42"), Longitude: command.Set("W71")},
		"origin with no pair": {Origin: &domain.CoordinateOrigin{Source: domain.CoordinateManual}},
		"geocoded, no provider": {Latitude: command.Set("N42"), Longitude: command.Set("W71"),
			Origin: &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded}},
	} {
		_, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: boston, Coordinates: coords})
		var coordErr *command.InvalidCoordinateError
		if !errors.Is(err, command.ErrInvalidInput) || !errors.As(err, &coordErr) {
			t.Errorf("%s: error = %v, want an InvalidCoordinateError", name, err)
		}
	}
	if facts, _ := f.readStore.GetPlaceFacts(ctx, boston); facts != nil {
		t.Errorf("facts after rejected updates = %+v, want none", facts)
	}
}

// TestPlaceEvents_MainOnly is PL-005's command half: no place event type is
// on the BR-006 allowlist, and each place command on a branch fails with
// ErrEventTypeNotBranchAware, appending nothing.
func TestPlaceEvents_MainOnly(t *testing.T) {
	for _, eventType := range placeEventTypes {
		if slices.Contains(command.BranchAwareEventTypes(), eventType) {
			t.Errorf("%s is branch-aware; place events are main-only (PL-005)", eventType)
		}
	}

	f := newBranchFixture()
	ctx := context.Background()
	if _, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston, MA"}); err != nil {
		t.Fatal(err)
	}
	branch, err := f.handler.CreateBranch(ctx, "places", "")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.eventStore.ReadAll(ctx, 0, 1000)
	_, err = f.handler.WithBranch(branch).UpdatePlace(ctx, command.UpdatePlaceInput{ID: repository.PlaceID("boston,ma"), Name: command.Set("Boston")})
	if !errors.Is(err, command.ErrEventTypeNotBranchAware) {
		t.Errorf("UpdatePlace on a branch error = %v, want ErrEventTypeNotBranchAware", err)
	}
	if after, _ := f.eventStore.ReadAll(ctx, 0, 1000); len(after) != len(before) {
		t.Errorf("log grew from %d to %d events, want nothing appended", len(before), len(after))
	}
}
