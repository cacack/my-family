package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/repository"
)

// TestPlaces_ListAndGet drives listPlaces and getPlace end to end: each place
// carries its ID (UUIDv5 of its key), its key, its most-used spelling and its
// count of uses at or under it; the list pages; a place named only on a
// branch is listed and found there alone, under the ID it has everywhere.
func TestPlaces_ListAndGet(t *testing.T) {
	server := setupBranchTestServer()
	for _, body := range []string{
		`{"given_name":"Ann","surname":"Lee","birth_place":"Boston, MA"}`,
		`{"given_name":"Bob","surname":"Lee","birth_place":"boston,ma","death_place":"Salem, MA"}`,
	} {
		if rec := do(t, server, http.MethodPost, "/api/v1/persons", body); rec.Code != http.StatusCreated {
			t.Fatalf("create person: %d %s", rec.Code, rec.Body)
		}
	}
	branchID := createBranch(t, server, "Worcester theory")
	if rec := do(t, server, http.MethodPost, "/api/v1/persons?branch="+branchID,
		`{"given_name":"Cy","surname":"Lee","birth_place":"Worcester, MA"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create person on branch: %d %s", rec.Code, rec.Body)
	}

	type place struct {
		key, name string
		count     float64
	}
	list := func(path string) ([]place, float64) {
		t.Helper()
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
		}
		body := decodeJSON(t, rec)
		var out []place
		for _, item := range body["items"].([]any) {
			m := item.(map[string]any)
			if m["id"] != repository.PlaceID(m["key"].(string)).String() {
				t.Errorf("GET %s: %v has an ID that is not its key's", path, m)
			}
			out = append(out, place{m["key"].(string), m["name"].(string), m["count"].(float64)})
		}
		return out, body["total"].(float64)
	}

	main := []place{{"boston,ma", "Boston, MA", 2}, {"ma", "MA", 3}, {"salem,ma", "Salem, MA", 1}}
	if got, total := list("/api/v1/places"); !slices.Equal(got, main) || total != 3 {
		t.Errorf("listPlaces = %v (total %v), want %v", got, total, main)
	}
	if got, total := list("/api/v1/places?limit=1&offset=1"); !slices.Equal(got, main[1:2]) || total != 3 {
		t.Errorf("listPlaces page 2 = %v (total %v), want %v", got, total, main[1:2])
	}
	onBranch := []place{{"boston,ma", "Boston, MA", 2}, {"ma", "MA", 4}, {"salem,ma", "Salem, MA", 1}, {"worcester,ma", "Worcester, MA", 1}}
	if got, total := list("/api/v1/places?branch=" + branchID); !slices.Equal(got, onBranch) || total != 4 {
		t.Errorf("listPlaces on the branch = %v (total %v), want %v", got, total, onBranch)
	}

	worcester := "/api/v1/places/" + repository.PlaceID("worcester,ma").String()
	if rec := do(t, server, http.MethodGet, worcester, ""); rec.Code != http.StatusNotFound {
		t.Errorf("getPlace on main of a branch-only place: %d %s, want 404", rec.Code, rec.Body)
	}
	rec := do(t, server, http.MethodGet, worcester+"?branch="+branchID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("getPlace on the branch: %d %s", rec.Code, rec.Body)
	}
	if got := decodeJSON(t, rec); got["key"] != "worcester,ma" || got["name"] != "Worcester, MA" || got["count"] != float64(1) {
		t.Errorf("getPlace on the branch = %v", got)
	}
	if rec := do(t, server, http.MethodGet, worcester+"?branch="+unknownUUID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("getPlace on an unknown branch: %d, want 404", rec.Code)
	}
}

// TestPlaces_UpdateAndEffectiveCoordinates drives updatePlace end to end
// (ADR-009 §2, §5, §9): setting a place's name and pair returns the place
// with them; a person's and a family's reads keep their own pair and add the
// resolved place with its pair beside it; the map places the records with no
// pair of their own at the place's pair, under its ID and name, while a record
// with its own pair keeps its own pin; and the change is one history entry
// about the place.
func TestPlaces_UpdateAndEffectiveCoordinates(t *testing.T) {
	server := setupBranchTestServer()
	create := func(path, body string) string {
		t.Helper()
		rec := do(t, server, http.MethodPost, path, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST %s: %d %s", path, rec.Code, rec.Body)
		}
		return decodeJSON(t, rec)["id"].(string)
	}
	ann := create("/api/v1/persons", `{"given_name":"Ann","surname":"Lee","birth_place":"Boston, MA"}`)
	cy := create("/api/v1/persons", `{"given_name":"Cy","surname":"Lee","birth_place":"boston,ma",
		"birth_place_latitude":"N42.35","birth_place_longitude":"W71.05"}`)
	family := create("/api/v1/families", `{"partner1_id":"`+ann+`","marriage_place":"Boston, MA"}`)

	boston := repository.PlaceID("boston,ma").String()
	path := "/api/v1/places/" + boston
	rec := do(t, server, http.MethodPut, path,
		`{"version":0,"name":"Boston","latitude":"N42.3601","longitude":"W71.0589","coordinates_origin":{"source":"geocoded","provider":"Nominatim","precision":"full"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("updatePlace: %d %s", rec.Code, rec.Body)
	}
	place := decodeJSON(t, rec)
	if place["name"] != "Boston" || place["latitude"] != "N42.3601" || place["longitude"] != "W71.0589" ||
		place["version"] != float64(1) || place["count"] != float64(3) {
		t.Errorf("updatePlace = %v", place)
	}
	if origin, _ := place["coordinates_origin"].(map[string]any); origin["source"] != "geocoded" {
		t.Errorf("updatePlace origin = %v, want geocoded", place["coordinates_origin"])
	}

	resolved := func(body map[string]any, field string) map[string]any {
		t.Helper()
		r, ok := body[field].(map[string]any)
		if !ok || r["id"] != boston || r["name"] != "Boston" || r["latitude"] != "N42.3601" || r["longitude"] != "W71.0589" {
			t.Errorf("%s = %v, want Boston with its pair", field, body[field])
		}
		return r
	}
	annBody := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/persons/"+ann, ""))
	resolved(annBody, "birth_place_resolved")
	if _, ok := annBody["birth_place_latitude"]; ok {
		t.Errorf("Ann has an own latitude %v, want none: the place's pair is not copied", annBody["birth_place_latitude"])
	}
	cyBody := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/persons/"+cy, ""))
	resolved(cyBody, "birth_place_resolved")
	if cyBody["birth_place_latitude"] != "N42.35" || cyBody["birth_place_longitude"] != "W71.05" {
		t.Errorf("Cy's own pair = %v %v, want it unchanged", cyBody["birth_place_latitude"], cyBody["birth_place_longitude"])
	}
	list := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/persons", ""))
	for _, item := range list["items"].([]any) {
		resolved(item.(map[string]any), "birth_place_resolved")
	}
	resolved(decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/families/"+family, "")), "marriage_place_resolved")
	families := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/families", ""))
	resolved(families["items"].([]any)[0].(map[string]any), "marriage_place_resolved")

	// The map: Ann at the place's pair, Cy at his own, both under Boston.
	type pin struct {
		placeID, place string
		lat, lon       float64
		names          string
	}
	var pins []pin
	for _, item := range decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/map/locations", ""))["items"].([]any) {
		m := item.(map[string]any)
		var names []string
		for _, p := range m["persons"].([]any) {
			names = append(names, p.(map[string]any)["given_name"].(string))
		}
		pins = append(pins, pin{m["place_id"].(string), m["place"].(string), m["latitude"].(float64), m["longitude"].(float64), strings.Join(names, ",")})
	}
	wantPins := []pin{{boston, "Boston", 42.35, -71.05, "Cy"}, {boston, "Boston", 42.3601, -71.0589, "Ann"}}
	if !slices.Equal(pins, wantPins) {
		t.Errorf("map locations = %v, want %v", pins, wantPins)
	}

	history := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/history?entity_type=place", ""))
	entries, _ := history["items"].([]any)
	if len(entries) != 1 {
		t.Fatalf("place history = %v, want one entry", history)
	}
	if e := entries[0].(map[string]any); e["entity_id"] != boston || e["entity_name"] != "Boston" || e["action"] != "updated" {
		t.Errorf("place history entry = %v", e)
	}

	// A stale version conflicts, a bad pair names its field, and an ID no
	// record names is not found.
	if rec := do(t, server, http.MethodPut, path, `{"version":0,"name":"Old Boston"}`); rec.Code != http.StatusConflict {
		t.Errorf("stale updatePlace: %d %s, want 409", rec.Code, rec.Body)
	}
	rec = do(t, server, http.MethodPut, path, `{"version":1,"latitude":"E42","longitude":"W71"}`)
	if details, _ := decodeJSON(t, rec)["details"].(map[string]any); rec.Code != http.StatusBadRequest || details["field"] != "latitude" {
		t.Errorf("bad latitude: %d %s, want 400 naming latitude", rec.Code, rec.Body)
	}
	if rec := do(t, server, http.MethodPut, "/api/v1/places/"+repository.PlaceID("nowhere").String(), `{"version":0,"name":"x"}`); rec.Code != http.StatusNotFound {
		t.Errorf("updatePlace of an unused place: %d %s, want 404", rec.Code, rec.Body)
	}
}
