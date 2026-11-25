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
var placeEventTypes = []string{"PlaceUpdated", "PlaceAliased", "PlaceUnaliased"}

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
	final, _ := f.eventStore.ReadAll(ctx, 0, 1000)
	if len(final) != len(after)+1 {
		t.Fatalf("log holds %d events, want %d: one more for the clear", len(final), len(after)+1)
	}
	// The clear, written while the display name was set, still records the
	// spelling the key comes from, not the name it removes.
	clearEvent, err := final[len(final)-1].DecodeEvent()
	if err != nil {
		t.Fatal(err)
	}
	if text := clearEvent.(domain.PlaceUpdated).Text; text != "Boston, MA" {
		t.Errorf("clearing event text = %q, want Boston, MA", text)
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
	onBranch := f.handler.WithBranch(branch)
	_, err = onBranch.UpdatePlace(ctx, command.UpdatePlaceInput{ID: repository.PlaceID("boston,ma"), Name: command.Set("Boston")})
	if !errors.Is(err, command.ErrEventTypeNotBranchAware) {
		t.Errorf("UpdatePlace on a branch error = %v, want ErrEventTypeNotBranchAware", err)
	}
	_, err = onBranch.AliasPlace(ctx, command.AliasPlaceInput{ID: repository.PlaceID("boston,ma"), TargetID: repository.PlaceID("ma")})
	if !errors.Is(err, command.ErrEventTypeNotBranchAware) {
		t.Errorf("AliasPlace on a branch error = %v, want ErrEventTypeNotBranchAware", err)
	}
	if after, _ := f.eventStore.ReadAll(ctx, 0, 1000); len(after) != len(before) {
		t.Errorf("log grew from %d to %d events, want nothing appended", len(before), len(after))
	}
	if _, err := f.handler.AliasPlace(ctx, command.AliasPlaceInput{ID: repository.PlaceID("boston,ma"), TargetID: repository.PlaceID("ma")}); err != nil {
		t.Fatal(err)
	}
	before, _ = f.eventStore.ReadAll(ctx, 0, 1000)
	_, err = onBranch.UnaliasPlace(ctx, command.UnaliasPlaceInput{ID: repository.PlaceID("boston,ma"), Version: 1})
	if !errors.Is(err, command.ErrEventTypeNotBranchAware) {
		t.Errorf("UnaliasPlace on a branch error = %v, want ErrEventTypeNotBranchAware", err)
	}
	if after, _ := f.eventStore.ReadAll(ctx, 0, 1000); len(after) != len(before) {
		t.Errorf("log grew from %d to %d events, want nothing appended", len(before), len(after))
	}
}

// TestAliasPlace_OneEventOnTheAbsorbedStream shows that joining two spellings
// appends one PlaceAliased to the absorbed place's stream and nothing else,
// that every record keeps its text (PL-001) and resolves to the target, and
// that separating them appends one PlaceUnaliased.
func TestAliasPlace_OneEventOnTheAbsorbedStream(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	ann, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ann", BirthPlace: "Köln, Germany"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Bob", BirthPlace: "Cologne, Germany"}); err != nil {
		t.Fatal(err)
	}
	koln, cologne := repository.PlaceID("köln,germany"), repository.PlaceID("cologne,germany")
	before, _ := f.eventStore.ReadAll(ctx, 0, 1000)

	result, err := f.handler.AliasPlace(ctx, command.AliasPlaceInput{ID: koln, TargetID: cologne})
	if err != nil || result.Version != 1 {
		t.Fatalf("AliasPlace = %+v, %v; want version 1", result, err)
	}
	after, _ := f.eventStore.ReadAll(ctx, 0, 1000)
	added := after[len(before):]
	if len(added) != 1 || added[0].EventType != "PlaceAliased" || added[0].StreamID != koln || added[0].StreamType != "Place" {
		t.Fatalf("appended %+v, want one PlaceAliased on Köln's stream", added)
	}
	event, err := added[0].DecodeEvent()
	if err != nil {
		t.Fatal(err)
	}
	if e := event.(domain.PlaceAliased); e.Key != "köln,germany" || e.Text != "Köln, Germany" || e.TargetID != cologne ||
		e.TargetKey != "cologne,germany" || e.TargetText != "Cologne, Germany" {
		t.Errorf("event = %+v", e)
	}
	if person, _ := f.readStore.GetPerson(ctx, domain.MainBranchID, ann.ID); person.BirthPlace != "Köln, Germany" {
		t.Errorf("Ann's birth place = %q, want Köln, Germany as recorded (PL-001)", person.BirthPlace)
	}
	if aliases, _ := f.readStore.ListPlaceAliases(ctx); len(aliases) != 1 || aliases[0].ResolvedKey != "cologne,germany" {
		t.Errorf("place_aliases = %+v, want Köln resolving to Cologne", aliases)
	}

	unaliased, err := f.handler.UnaliasPlace(ctx, command.UnaliasPlaceInput{ID: koln, Version: 1})
	if err != nil || unaliased.Version != 2 {
		t.Fatalf("UnaliasPlace = %+v, %v; want version 2", unaliased, err)
	}
	final, _ := f.eventStore.ReadAll(ctx, 0, 1000)
	if len(final) != len(after)+1 || final[len(final)-1].EventType != "PlaceUnaliased" || final[len(final)-1].StreamID != koln {
		t.Errorf("log after UnaliasPlace ends %+v, want one PlaceUnaliased on Köln's stream", final[len(final)-1])
	}
	if aliases, _ := f.readStore.ListPlaceAliases(ctx); len(aliases) != 0 {
		t.Errorf("place_aliases after UnaliasPlace = %+v, want none", aliases)
	}
}

// TestAliasPlace_Rejects covers the command's checks (ADR-009 §3): a target
// that is itself an alias, a place already an alias, the place itself, an
// unknown place or target, a stale version, and unaliasing a place that is no
// alias. None appends anything.
func TestAliasPlace_Rejects(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	for _, place := range []string{"Köln", "Cologne", "Coeln"} {
		if _, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ann", BirthPlace: place}); err != nil {
			t.Fatal(err)
		}
	}
	koln, cologne, coeln := repository.PlaceID("köln"), repository.PlaceID("cologne"), repository.PlaceID("coeln")
	if _, err := f.handler.AliasPlace(ctx, command.AliasPlaceInput{ID: koln, TargetID: cologne}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.eventStore.ReadAll(ctx, 0, 1000)

	for name, tc := range map[string]struct {
		input command.AliasPlaceInput
		want  error
	}{
		"target is an alias": {command.AliasPlaceInput{ID: coeln, TargetID: koln}, command.ErrInvalidInput},
		"place is an alias":  {command.AliasPlaceInput{ID: koln, TargetID: coeln, Version: 1}, command.ErrInvalidInput},
		"the place itself":   {command.AliasPlaceInput{ID: coeln, TargetID: coeln}, command.ErrInvalidInput},
		"closes a cycle":     {command.AliasPlaceInput{ID: cologne, TargetID: koln}, command.ErrInvalidInput},
		"unknown target":     {command.AliasPlaceInput{ID: coeln, TargetID: uuid.New()}, command.ErrInvalidInput},
		"unknown place":      {command.AliasPlaceInput{ID: uuid.New(), TargetID: cologne}, command.ErrPlaceNotFound},
		"stale version":      {command.AliasPlaceInput{ID: coeln, TargetID: cologne, Version: 4}, repository.ErrConcurrencyConflict},
	} {
		if _, err := f.handler.AliasPlace(ctx, tc.input); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", name, err, tc.want)
		}
	}
	if _, err := f.handler.UnaliasPlace(ctx, command.UnaliasPlaceInput{ID: coeln}); !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("unaliasing a place that is no alias: error = %v, want ErrInvalidInput", err)
	}
	if _, err := f.handler.UnaliasPlace(ctx, command.UnaliasPlaceInput{ID: uuid.New()}); !errors.Is(err, command.ErrPlaceNotFound) {
		t.Errorf("unaliasing an unknown place: error = %v, want ErrPlaceNotFound", err)
	}
	if _, err := f.handler.UnaliasPlace(ctx, command.UnaliasPlaceInput{ID: koln}); !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Errorf("stale unalias: error = %v, want ErrConcurrencyConflict", err)
	}
	if after, _ := f.eventStore.ReadAll(ctx, 0, 1000); len(after) != len(before) {
		t.Errorf("log grew from %d to %d events, want nothing appended", len(before), len(after))
	}
}

// TestUpdatePlace_Parent sets and clears a place's explicit parent (ADR-009
// §4): the event records the parent's key, the same parent again appends
// nothing, and a clear records "".
func TestUpdatePlace_Parent(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	for _, place := range []string{"Springfield", "Chicago, Illinois, USA"} {
		if _, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ann", BirthPlace: place}); err != nil {
			t.Fatal(err)
		}
	}
	springfield, illinois := repository.PlaceID("springfield"), repository.PlaceID("illinois,usa")

	lastParent := func() any {
		t.Helper()
		events, _ := f.eventStore.ReadAll(ctx, 0, 1000)
		event, err := events[len(events)-1].DecodeEvent()
		if err != nil {
			t.Fatal(err)
		}
		return event.(domain.PlaceUpdated).Changes["parent"]
	}

	result, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: springfield, Parent: command.Set(illinois)})
	if err != nil {
		t.Fatalf("set parent: %v", err)
	}
	if got := lastParent(); got != "illinois,usa" || result.Version != 1 {
		t.Errorf("set parent recorded %v at version %d, want illinois,usa at 1", got, result.Version)
	}
	if facts, _ := f.readStore.GetPlaceFacts(ctx, springfield); facts == nil || facts.ParentKey != "illinois,usa" {
		t.Errorf("facts = %+v, want parent illinois,usa", facts)
	}

	same, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: springfield, Version: 1, Parent: command.Set(illinois)})
	if err != nil || same.Version != 1 {
		t.Errorf("same parent again = %+v, %v; want version 1 and nothing appended", same, err)
	}

	cleared, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: springfield, Version: 1, Parent: command.Clear[uuid.UUID]()})
	if err != nil {
		t.Fatalf("clear parent: %v", err)
	}
	if got := lastParent(); got != "" || cleared.Version != 2 {
		t.Errorf("clear parent recorded %v at version %d, want \"\" at 2", got, cleared.Version)
	}
	if facts, _ := f.readStore.GetPlaceFacts(ctx, springfield); facts == nil || facts.ParentKey != "" {
		t.Errorf("facts after clear = %+v, want no parent", facts)
	}
}

// TestUpdatePlace_RejectsParent covers the parent checks (ADR-009 §4): a
// parent that is not a place in use, the place itself, and a parent that
// would put the place beneath itself, counted through default parents and
// aliases. None appends anything.
func TestUpdatePlace_RejectsParent(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	for _, place := range []string{"Springfield", "Chicago, Illinois, USA", "IL"} {
		if _, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ann", BirthPlace: place}); err != nil {
			t.Fatal(err)
		}
	}
	id := repository.PlaceID
	if _, err := f.handler.AliasPlace(ctx, command.AliasPlaceInput{ID: id("il"), TargetID: id("illinois,usa")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: id("springfield"), Parent: command.Set(id("il"))}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.eventStore.ReadAll(ctx, 0, 1000)

	for name, tc := range map[string]struct {
		place, parent uuid.UUID
		version       int64
	}{
		"unknown parent":              {id("springfield"), uuid.New(), 1},
		"the place itself":            {id("usa"), id("usa"), 0},
		"its own alias":               {id("illinois,usa"), id("il"), 0},
		"under its default child":     {id("usa"), id("chicago,illinois,usa"), 0},
		"under its explicit child":    {id("illinois,usa"), id("springfield"), 0},
		"under a child through alias": {id("usa"), id("springfield"), 0},
	} {
		_, err := f.handler.UpdatePlace(ctx, command.UpdatePlaceInput{ID: tc.place, Version: tc.version, Parent: command.Set(tc.parent)})
		if !errors.Is(err, command.ErrInvalidInput) {
			t.Errorf("%s: error = %v, want ErrInvalidInput", name, err)
		}
	}
	if after, _ := f.eventStore.ReadAll(ctx, 0, 1000); len(after) != len(before) {
		t.Errorf("log grew from %d to %d events, want nothing appended", len(before), len(after))
	}
}

// TestAliasPlace_RejectsParentCycle: an alias that would put its target
// beneath itself in the place hierarchy is rejected (ADR-009 §4).
func TestAliasPlace_RejectsParentCycle(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	for _, place := range []string{"Berlin, Germany", "Germany"} {
		if _, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ann", BirthPlace: place}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.handler.AliasPlace(ctx, command.AliasPlaceInput{ID: repository.PlaceID("germany"), TargetID: repository.PlaceID("berlin,germany")})
	if !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("aliasing Germany to Berlin: error = %v, want ErrInvalidInput", err)
	}
}
