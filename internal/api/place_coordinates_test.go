package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// coordinateFields are the coordinate pairs each record accepts (#1604).
var coordinateFields = map[string][]string{
	"persons":  {"birth_place", "death_place"},
	"families": {"marriage_place"},
}

// readRecord GETs path and returns the decoded body, failing on any status but 200.
func readRecord(t *testing.T, server *api.Server, path string) map[string]any {
	t.Helper()
	rec := do(t, server, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d. Body: %s", path, rec.Code, rec.Body.String())
	}
	return decodeJSON(t, rec)
}

// putRecord PUTs members plus the record's current version and returns the response.
func putRecord(t *testing.T, server *api.Server, path, members string) (int, map[string]any) {
	t.Helper()
	body := fmt.Sprintf(`{%s"version":%v}`, members, readRecord(t, server, path)["version"])
	rec := do(t, server, http.MethodPut, path, body)
	return rec.Code, decodeJSON(t, rec)
}

// TestPlaceCoordinates_CreateAndUpdate: create and update accept the
// coordinate pairs the responses return; left out keeps them, null clears them.
func TestPlaceCoordinates_CreateAndUpdate(t *testing.T) {
	server := setupBranchTestServer()
	partner := createPerson(t, server, "Ada", "Lovelace")

	for _, kind := range []string{"persons", "families"} {
		t.Run(kind, func(t *testing.T) {
			var members []string
			for _, f := range coordinateFields[kind] {
				members = append(members, fmt.Sprintf(`"%s_latitude":"N42.3601","%s_longitude":"W71.0589"`, f, f))
			}
			body := `{"given_name":"Byron",` + strings.Join(members, ",") + `}`
			if kind == "families" {
				body = fmt.Sprintf(`{"partner1_id":%q,%s}`, partner, strings.Join(members, ","))
			}
			rec := do(t, server, http.MethodPost, "/api/v1/"+kind, body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("POST: status = %d. Body: %s", rec.Code, rec.Body.String())
			}
			path := "/api/v1/" + kind + "/" + decodeJSON(t, rec)["id"].(string)
			check := func(stage, lat, long string) {
				t.Helper()
				got := readRecord(t, server, path)
				for _, f := range coordinateFields[kind] {
					if got[f+"_latitude"] != nilIfEmpty(lat) || got[f+"_longitude"] != nilIfEmpty(long) {
						t.Errorf("%s: %s = %v, %v; want %q, %q", stage, f, got[f+"_latitude"], got[f+"_longitude"], lat, long)
					}
				}
			}
			check("after create", "N42.3601", "W71.0589")

			for i, f := range coordinateFields[kind] {
				members[i] = fmt.Sprintf(`"%s_latitude":"S33.8688","%s_longitude":"E151.2093",`, f, f)
			}
			if code, body := putRecord(t, server, path, strings.Join(members, "")); code != http.StatusOK {
				t.Fatalf("PUT set: status = %d. Body: %v", code, body)
			}
			check("after set", "S33.8688", "E151.2093")

			if code, body := putRecord(t, server, path, ""); code != http.StatusOK {
				t.Fatalf("PUT omitted: status = %d. Body: %v", code, body)
			}
			check("after an update that left them out", "S33.8688", "E151.2093")

			for i, f := range coordinateFields[kind] {
				members[i] = fmt.Sprintf(`"%s_latitude":null,"%s_longitude":null,`, f, f)
			}
			if code, body := putRecord(t, server, path, strings.Join(members, "")); code != http.StatusOK {
				t.Fatalf("PUT null: status = %d. Body: %v", code, body)
			}
			check("after null", "", "")
		})
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// TestPlaceCoordinates_Rejected: a coordinate on the wrong axis, out of
// range, or half of a pair is answered with 400 naming the field, and
// nothing is stored.
func TestPlaceCoordinates_Rejected(t *testing.T) {
	server := setupBranchTestServer()
	person := createPerson(t, server, "Ada", "Lovelace")
	family := createFamily(t, server, person, createPerson(t, server, "William", "King"))

	for _, tc := range []struct {
		name, members, field string
	}{
		{"latitude with W", `"%[1]s_latitude":"W42.3601","%[1]s_longitude":"W71.0589"`, "_latitude"},
		{"longitude with S", `"%[1]s_latitude":"N42.3601","%[1]s_longitude":"S71.0589"`, "_longitude"},
		{"latitude out of range", `"%[1]s_latitude":"N95","%[1]s_longitude":"W71.0589"`, "_latitude"},
		{"longitude alone", `"%[1]s_longitude":"W71.0589"`, "_latitude"},
		{"latitude alone", `"%[1]s_latitude":"N42.3601"`, "_longitude"},
		{"latitude null beside a longitude", `"%[1]s_latitude":null,"%[1]s_longitude":"W71.0589"`, "_latitude"},
	} {
		for kind, id := range map[string]string{"persons": person, "families": family} {
			prefix := coordinateFields[kind][0]
			members := fmt.Sprintf(tc.members, prefix)
			field := prefix + tc.field
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				path := "/api/v1/" + kind + "/" + id
				before := readRecord(t, server, path)["version"]
				code, body := putRecord(t, server, path, members+",")
				assertFieldRejected(t, "PUT", code, body, field)
				if after := readRecord(t, server, path)["version"]; after != before {
					t.Errorf("version moved %v -> %v, want nothing stored", before, after)
				}

				create := `{"given_name":"Byron",` + members + `}`
				if kind == "families" {
					create = fmt.Sprintf(`{"partner1_id":%q,%s}`, person, members)
				}
				rec := do(t, server, http.MethodPost, "/api/v1/"+kind, create)
				assertFieldRejected(t, "POST", rec.Code, decodeJSON(t, rec), field)
			})
		}
	}
}

func assertFieldRejected(t *testing.T, op string, code int, body map[string]any, field string) {
	t.Helper()
	details, _ := body["details"].(map[string]any)
	message, _ := body["message"].(string)
	if code != http.StatusBadRequest || details["field"] != field || !strings.Contains(message, field) {
		t.Errorf("%s: status = %d, body = %v; want 400 naming %s", op, code, body, field)
	}
}

// TestPlaceCoordinates_MapLocations: a birth place given coordinates is
// returned by the map; written with ?branch= it is returned for that branch
// and not for main.
func TestPlaceCoordinates_MapLocations(t *testing.T) {
	server := setupBranchTestServer()
	person := createPerson(t, server, "Ada", "Lovelace")
	branch := createBranch(t, server, "Placing")
	path := "/api/v1/persons/" + person
	set := `"birth_place":"Boston","birth_place_latitude":"N42.3601","birth_place_longitude":"W71.0589",`

	places := func(query string) []string {
		t.Helper()
		var got []string
		for _, item := range readRecord(t, server, "/api/v1/map/locations"+query)["items"].([]any) {
			got = append(got, item.(map[string]any)["place"].(string))
		}
		return got
	}

	rec := do(t, server, http.MethodPut, path+"?branch="+branch,
		fmt.Sprintf(`{%s"version":%v}`, set, readRecord(t, server, path)["version"]))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT on branch: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	if got := places("?branch=" + branch); len(got) != 1 || got[0] != "Boston" {
		t.Errorf("branch map = %v, want Boston", got)
	}
	if got := places(""); len(got) != 0 {
		t.Errorf("main map = %v, want nothing", got)
	}

	if code, body := putRecord(t, server, path, set); code != http.StatusOK {
		t.Fatalf("PUT on main: status = %d. Body: %v", code, body)
	}
	if got := places(""); len(got) != 1 || got[0] != "Boston" {
		t.Errorf("main map = %v, want Boston", got)
	}
}
