package repository

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

func strPtr(s string) *string { return &s }

// TestResolveCoordinates is the effective-coordinate rule (ADR-009 §5): the
// record's own pair when both halves are set, otherwise its place's pair,
// otherwise none; a half-set own pair counts as unset.
func TestResolveCoordinates(t *testing.T) {
	manual := &domain.CoordinateOrigin{Source: domain.CoordinateManual}
	geocoded := &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim", Precision: domain.MatchFull}
	place := &PlaceFactsReadModel{Key: "boston,ma", Latitude: strPtr("N42.36"), Longitude: strPtr("W71.06"), Origin: geocoded}
	unmapped := &PlaceFactsReadModel{Key: "boston,ma", Name: "Boston"}

	tests := []struct {
		name      string
		lat, long *string
		origin    *domain.CoordinateOrigin
		place     *PlaceFactsReadModel
		want      EffectiveCoordinates
		wantOK    bool
	}{
		{"own pair wins over the place's", strPtr("N42.35"), strPtr("W71.05"), manual, place,
			EffectiveCoordinates{Latitude: "N42.35", Longitude: "W71.05", Origin: *manual}, true},
		{"own pair with no origin reads as imported", strPtr("N42.35"), strPtr("W71.05"), nil, nil,
			EffectiveCoordinates{Latitude: "N42.35", Longitude: "W71.05", Origin: domain.CoordinateOrigin{Source: domain.CoordinateImported}}, true},
		{"no own pair falls back to the place's", nil, nil, nil, place,
			EffectiveCoordinates{Latitude: "N42.36", Longitude: "W71.06", Origin: *geocoded, FromPlace: true}, true},
		{"half-set own pair (latitude only) counts as unset", strPtr("N42.35"), nil, manual, place,
			EffectiveCoordinates{Latitude: "N42.36", Longitude: "W71.06", Origin: *geocoded, FromPlace: true}, true},
		{"half-set own pair (empty longitude) counts as unset", strPtr("N42.35"), strPtr(""), manual, place,
			EffectiveCoordinates{Latitude: "N42.36", Longitude: "W71.06", Origin: *geocoded, FromPlace: true}, true},
		{"no own pair and a place with no pair", nil, nil, nil, unmapped, EffectiveCoordinates{}, false},
		{"no own pair and no place facts", nil, nil, nil, nil, EffectiveCoordinates{}, false},
		{"half-set own pair and no place facts", nil, strPtr("W71.05"), nil, nil, EffectiveCoordinates{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveCoordinates(tt.lat, tt.long, tt.origin, tt.place)
			if ok != tt.wantOK || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResolveCoordinates = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestPlaces_Facts shows a place's stated facts on the list: a display name
// replaces the most-used spelling, the pair and version come through, and a
// place with facts that no record names is listed only for a principal who
// sees everything (PL-006).
func TestPlaces_Facts(t *testing.T) {
	uses := []PlaceUse{{Key: "boston,ma", Text: "Boston, MA", Count: 2}}
	origin := &domain.CoordinateOrigin{Source: domain.CoordinateManual}
	facts := []PlaceFactsReadModel{
		{ID: PlaceID("boston,ma"), Key: "boston,ma", Name: "Boston", Latitude: strPtr("N42.36"), Longitude: strPtr("W71.06"), Origin: origin, Version: 2},
		{ID: PlaceID("lost,ma"), Key: "lost,ma", Latitude: strPtr("N42"), Longitude: strPtr("W71"), Origin: origin, Version: 1},
	}
	boston := PlaceReadModel{ID: PlaceID("boston,ma"), Key: "boston,ma", Name: "Boston", Spelling: "Boston, MA", Count: 2,
		Latitude: strPtr("N42.36"), Longitude: strPtr("W71.06"), Origin: origin, Version: 2}
	ma := PlaceReadModel{ID: PlaceID("ma"), Key: "ma", Name: "MA", Spelling: "MA", Count: 2}
	lost := PlaceReadModel{ID: PlaceID("lost,ma"), Key: "lost,ma", Name: "lost,ma", Spelling: "lost,ma",
		Latitude: strPtr("N42"), Longitude: strPtr("W71"), Origin: origin, Version: 1}

	if got, want := Places(uses, facts, Privacy{}), []PlaceReadModel{boston, ma, lost}; !reflect.DeepEqual(got, want) {
		t.Errorf("Places (sees all) =\n  %+v\nwant\n  %+v", got, want)
	}
	viewer := Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: time.Now()}
	if got, want := Places(uses, facts, viewer), []PlaceReadModel{boston, ma}; !reflect.DeepEqual(got, want) {
		t.Errorf("Places (viewer) =\n  %+v\nwant\n  %+v", got, want)
	}
}

// TestMapLocations groups by place key, event type and effective pair: two
// spellings of a place are one pin named by its display name, records with
// no pair of their own appear at the place's pair, a record with its own pair
// keeps its own pin, and records with no effective pair or an unreadable one
// are left out.
func TestMapLocations(t *testing.T) {
	person := func(name string) MapPerson { return MapPerson{ID: uuid.New(), GivenName: name} }
	ann, bob, cy, di, ed, flo := person("Ann"), person("Bob"), person("Cy"), person("Di"), person("Ed"), person("Flo")
	geocoded := &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim", Precision: domain.MatchFull}
	boston := &PlaceFactsReadModel{ID: PlaceID("boston,ma"), Key: "boston,ma", Name: "Boston",
		Latitude: strPtr("N42.36"), Longitude: strPtr("W71.06"), Origin: geocoded}
	uses := []MapUse{
		{Person: ann, EventType: "birth", Text: "Boston, MA", Key: "boston,ma", Place: boston},
		{Person: bob, EventType: "birth", Text: "boston,ma", Key: "boston,ma", Place: boston},
		{Person: cy, EventType: "birth", Text: "Boston, MA", Key: "boston,ma", Place: boston,
			Latitude: strPtr("N42.35"), Longitude: strPtr("W71.05")},
		{Person: di, EventType: "birth", Text: "Salem, MA", Key: "salem,ma"},
		{Person: ed, EventType: "death", Text: "Salem, MA", Key: "salem,ma",
			Latitude: strPtr("N42.52"), Longitude: strPtr("W70.89")},
		{Person: flo, EventType: "death", Text: "Salem, MA", Key: "salem,ma",
			Latitude: strPtr("E42.52"), Longitude: strPtr("W70.89")},
	}
	imported := []domain.CoordinateSource{domain.CoordinateImported}
	want := []MapLocation{
		{PlaceID: PlaceID("boston,ma"), Place: "Boston", Latitude: 42.35, Longitude: -71.05, EventType: "birth",
			Count: 1, Persons: []MapPerson{cy}, Origins: imported},
		{PlaceID: PlaceID("boston,ma"), Place: "Boston", Latitude: 42.36, Longitude: -71.06, EventType: "birth",
			Count: 2, Persons: []MapPerson{ann, bob}, Origins: []domain.CoordinateSource{domain.CoordinateGeocoded}},
		{PlaceID: PlaceID("salem,ma"), Place: "Salem, MA", Latitude: 42.52, Longitude: -70.89, EventType: "death",
			Count: 1, Persons: []MapPerson{ed}, Origins: imported},
	}
	if got := MapLocations(uses); !reflect.DeepEqual(got, want) {
		t.Errorf("MapLocations =\n  %+v\nwant\n  %+v", got, want)
	}
}

// TestMapLocations_NameFromSpellings names a place with no display name by
// its most-used spelling, and gives a text with no key the zero place ID.
func TestMapLocations_NameFromSpellings(t *testing.T) {
	pair := func(p MapPerson, text, key string) MapUse {
		return MapUse{Person: p, EventType: "birth", Text: text, Key: key, Latitude: strPtr("N1"), Longitude: strPtr("E1")}
	}
	a, b, c, d := MapPerson{ID: uuid.New()}, MapPerson{ID: uuid.New()}, MapPerson{ID: uuid.New()}, MapPerson{ID: uuid.New()}
	got := MapLocations([]MapUse{pair(a, "kiel", "kiel"), pair(b, "Kiel", "kiel"), pair(c, "Kiel", "kiel"), pair(d, "", "")})
	if len(got) != 2 {
		t.Fatalf("MapLocations = %+v, want 2 locations", got)
	}
	if got[0].Place != "" || got[0].PlaceID != uuid.Nil {
		t.Errorf("keyless location = %q %s, want no name and the zero ID", got[0].Place, got[0].PlaceID)
	}
	if got[1].Place != "Kiel" || got[1].PlaceID != PlaceID("kiel") || got[1].Count != 3 {
		t.Errorf("kiel location = %+v, want Kiel with 3 persons", got[1])
	}
}
