package api

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// Every converter that builds a Person or Family response carries the place
// coordinates, and omits them when unset (#1278).

var (
	personCoordinateKeys = []string{"birth_place_latitude", "birth_place_longitude", "death_place_latitude", "death_place_longitude"}
	familyCoordinateKeys = []string{"marriage_place_latitude", "marriage_place_longitude"}
)

func TestConverters_PersonPlaceCoordinates(t *testing.T) {
	bLat, bLong, dLat, dLong, empty := "N42.3601", "W71.0589", "N39.7817", "W89.6501", ""
	want := map[string]string{
		"birth_place_latitude": bLat, "birth_place_longitude": bLong,
		"death_place_latitude": dLat, "death_place_longitude": dLong,
	}
	qp := query.Person{ID: uuid.New(), BirthPlaceLat: &bLat, BirthPlaceLong: &bLong, DeathPlaceLat: &dLat, DeathPlaceLong: &dLong}
	rm := repository.PersonReadModel{ID: uuid.New(), BirthPlaceLat: &bLat, BirthPlaceLong: &bLong, DeathPlaceLat: &dLat, DeathPlaceLong: &dLong}

	assertJSONCoordinates(t, "query person", convertQueryPersonToGenerated(qp), personCoordinateKeys, want)
	assertJSONCoordinates(t, "query person detail", convertQueryPersonDetailToGenerated(&query.PersonDetail{Person: qp}), personCoordinateKeys, want)
	assertJSONCoordinates(t, "read model person", convertReadModelPersonToGenerated(rm), personCoordinateKeys, want)

	assertJSONCoordinates(t, "query person without", convertQueryPersonToGenerated(query.Person{ID: uuid.New()}), personCoordinateKeys, nil)
	assertJSONCoordinates(t, "query person detail without", convertQueryPersonDetailToGenerated(&query.PersonDetail{}), personCoordinateKeys, nil)
	assertJSONCoordinates(t, "read model person without",
		convertReadModelPersonToGenerated(repository.PersonReadModel{ID: uuid.New(), BirthPlaceLat: &empty}), personCoordinateKeys, nil)
}

func TestConverters_FamilyPlaceCoordinates(t *testing.T) {
	lat, long, empty := "N39.7817", "W89.6501", ""
	want := map[string]string{"marriage_place_latitude": lat, "marriage_place_longitude": long}
	qf := query.Family{ID: uuid.New(), MarriagePlaceLat: &lat, MarriagePlaceLong: &long}
	rm := repository.FamilyReadModel{ID: uuid.New(), MarriagePlaceLat: &lat, MarriagePlaceLong: &long}

	assertJSONCoordinates(t, "query family", convertQueryFamilyToGenerated(qf), familyCoordinateKeys, want)
	assertJSONCoordinates(t, "query family detail", convertQueryFamilyDetailToGenerated(query.FamilyDetail{Family: qf}), familyCoordinateKeys, want)
	assertJSONCoordinates(t, "query family as detail", convertQueryFamilyToFamilyDetail(qf), familyCoordinateKeys, want)
	assertJSONCoordinates(t, "read model family", convertReadModelFamilyToGenerated(rm), familyCoordinateKeys, want)

	assertJSONCoordinates(t, "query family without", convertQueryFamilyToGenerated(query.Family{}), familyCoordinateKeys, nil)
	assertJSONCoordinates(t, "query family detail without", convertQueryFamilyDetailToGenerated(query.FamilyDetail{}), familyCoordinateKeys, nil)
	assertJSONCoordinates(t, "query family as detail without", convertQueryFamilyToFamilyDetail(query.Family{}), familyCoordinateKeys, nil)
	assertJSONCoordinates(t, "read model family without",
		convertReadModelFamilyToGenerated(repository.FamilyReadModel{ID: uuid.New(), MarriagePlaceLat: &empty}), familyCoordinateKeys, nil)
}

// assertJSONCoordinates encodes resp as the API would and checks each key:
// present with its wanted value, or absent when want lacks it.
func assertJSONCoordinates(t *testing.T, label string, resp any, keys []string, want map[string]string) {
	t.Helper()
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s: unmarshal: %v", label, err)
	}
	for _, key := range keys {
		value, present := got[key]
		wantValue, wantPresent := want[key]
		switch {
		case wantPresent && value != wantValue:
			t.Errorf("%s: %s = %v, want %q", label, key, value, wantValue)
		case !wantPresent && present:
			t.Errorf("%s: %s = %v, want it omitted", label, key, value)
		}
	}
}
