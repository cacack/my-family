package api_test

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestCoordinateOrigin_CreateAndUpdate: a pair written with no origin reads
// as manual, and a geocoded origin is returned with its provider and
// precision (#1563).
func TestCoordinateOrigin_CreateAndUpdate(t *testing.T) {
	server := setupBranchTestServer()
	partner := createPerson(t, server, "Ada", "Lovelace")
	manual := map[string]any{"source": "manual"}
	geocoded := map[string]any{"source": "geocoded", "provider": "Nominatim", "precision": "partial"}

	for _, kind := range []string{"persons", "families"} {
		t.Run(kind, func(t *testing.T) {
			f := coordinateFields[kind][0]
			pair := fmt.Sprintf(`%q:"Boston","%s_latitude":"N42.3601","%s_longitude":"W71.0589"`, f, f, f)
			body := `{"given_name":"Byron",` + pair + `}`
			if kind == "families" {
				body = fmt.Sprintf(`{"partner1_id":%q,%s}`, partner, pair)
			}
			rec := do(t, server, http.MethodPost, "/api/v1/"+kind, body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("POST: status = %d. Body: %s", rec.Code, rec.Body.String())
			}
			created := decodeJSON(t, rec)
			if got := created[f+"_coordinates_origin"]; !reflect.DeepEqual(got, manual) {
				t.Errorf("POST response origin = %v, want %v", got, manual)
			}
			path := "/api/v1/" + kind + "/" + created["id"].(string)
			if got := readRecord(t, server, path)[f+"_coordinates_origin"]; !reflect.DeepEqual(got, manual) {
				t.Errorf("origin after create = %v, want %v", got, manual)
			}

			members := fmt.Sprintf(`"%[1]s_latitude":"N42.36","%[1]s_longitude":"W71.05",`+
				`"%[1]s_coordinates_origin":{"source":"geocoded","provider":"Nominatim","precision":"partial"},`, f)
			if code, body := putRecord(t, server, path, members); code != http.StatusOK {
				t.Fatalf("PUT geocoded: status = %d. Body: %v", code, body)
			}
			if got := readRecord(t, server, path)[f+"_coordinates_origin"]; !reflect.DeepEqual(got, geocoded) {
				t.Errorf("origin after a geocoded update = %v, want %v", got, geocoded)
			}

			members = fmt.Sprintf(`"%[1]s_latitude":null,"%[1]s_longitude":null,`, f)
			if code, body := putRecord(t, server, path, members); code != http.StatusOK {
				t.Fatalf("PUT null: status = %d. Body: %v", code, body)
			}
			if got, ok := readRecord(t, server, path)[f+"_coordinates_origin"]; ok {
				t.Errorf("origin after clearing the pair = %v, want none", got)
			}
		})
	}
}

// TestCoordinateOrigin_Rejected: geocoded without a provider, and an origin
// without a pair, are answered with 400 naming the origin field (#1563).
func TestCoordinateOrigin_Rejected(t *testing.T) {
	server := setupBranchTestServer()
	person := createPerson(t, server, "Ada", "Lovelace")
	family := createFamily(t, server, person, createPerson(t, server, "William", "King"))

	for _, tc := range []struct{ name, members string }{
		{"geocoded without a provider", `%[1]q:"Boston","%[1]s_latitude":"N42.3601","%[1]s_longitude":"W71.0589",` +
			`"%[1]s_coordinates_origin":{"source":"geocoded","precision":"full"}`},
		{"an origin without a pair", `"%[1]s_coordinates_origin":{"source":"manual"}`},
	} {
		for kind, id := range map[string]string{"persons": person, "families": family} {
			prefix := coordinateFields[kind][0]
			members := fmt.Sprintf(tc.members, prefix)
			field := prefix + "_coordinates_origin"
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				code, body := putRecord(t, server, "/api/v1/"+kind+"/"+id, members+",")
				assertFieldRejected(t, "PUT", code, body, field)

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

const originTestGedcom = `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Mary /Imported/
1 BIRT
2 PLAC Boston
3 MAP
4 LATI N42.3601
4 LONG W71.0589
0 TRLR
`

// TestCoordinateOrigin_ImportedAndMap: an imported pair reads as imported,
// and a map location lists the distinct origins of the pairs it groups
// (#1563).
func TestCoordinateOrigin_ImportedAndMap(t *testing.T) {
	server := setupBranchTestServer()
	importGEDCOMText(t, server, originTestGedcom)

	rec := do(t, server, http.MethodPost, "/api/v1/persons",
		`{"given_name":"Byron","birth_place":"Boston","birth_place_latitude":"N42.3601","birth_place_longitude":"W71.0589"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	var imported map[string]any
	for _, item := range readRecord(t, server, "/api/v1/persons")["items"].([]any) {
		if p := item.(map[string]any); p["given_name"] == "Mary" {
			imported = p
		}
	}
	if got, want := imported["birth_place_coordinates_origin"], map[string]any{"source": "imported"}; !reflect.DeepEqual(got, want) {
		t.Errorf("imported origin = %v, want %v", got, want)
	}

	items := readRecord(t, server, "/api/v1/map/locations")["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("map items = %v, want one Boston birth location", items)
	}
	if got, want := items[0].(map[string]any)["origins"], []any{"imported", "manual"}; !reflect.DeepEqual(got, want) {
		t.Errorf("map origins = %v, want %v", got, want)
	}
}

// importGEDCOMText imports content through the API.
// TestNamelessPair_ImportAndMap: a MAP under an empty PLAC is kept on the
// record and reported as an import warning naming the record and event, and
// the map shows no location without a place name (#1807).
func TestNamelessPair_ImportAndMap(t *testing.T) {
	server := setupBranchTestServer()
	result := importGEDCOMText(t, server, strings.TrimSuffix(originTestGedcom, "0 TRLR\n")+
		"0 @I2@ INDI\n1 NAME Ada /Nameless/\n1 BIRT\n2 DATE 1815\n2 PLAC\n3 MAP\n4 LATI N51.5074\n4 LONG W0.1278\n0 TRLR\n")

	want := "Individual @I2@: BIRT coordinates have no place name; kept on the record"
	warnings, _ := result["warnings"].([]any)
	if !slices.ContainsFunc(warnings, func(w any) bool { return w.(map[string]any)["message"] == want }) {
		t.Errorf("import warnings = %v, want %q", warnings, want)
	}
	for _, item := range readRecord(t, server, "/api/v1/persons")["items"].([]any) {
		if p := item.(map[string]any); p["given_name"] == "Ada" && p["birth_place_latitude"] != "N51.5074" {
			t.Errorf("Ada's birth latitude = %v, want the imported N51.5074 kept", p["birth_place_latitude"])
		}
	}
	items := readRecord(t, server, "/api/v1/map/locations")["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["place"] != "Boston" {
		t.Errorf("map items = %v, want only the Boston location", items)
	}
}

func importGEDCOMText(t *testing.T, server *api.Server, content string) map[string]any {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	_, _ = io.WriteString(part, content)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: status %d: %s", rec.Code, rec.Body.String())
	}
	return decodeJSON(t, rec)
}
