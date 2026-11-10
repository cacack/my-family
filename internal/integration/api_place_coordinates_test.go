package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
)

// Place coordinates in person and family API responses (issue #1278).
//
// Import keeps BIRT/DEAT/MARR PLAC MAP coordinates in the read model (#1225,
// #1265); the person and family reads must return them, on the mainline and on
// a branch, and omit them for records that have none.

const placeCoordinatesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Mapped/
1 SEX M
1 BIRT
2 DATE 1850
2 PLAC Boston
3 MAP
4 LATI N42.3601
4 LONG W71.0589
1 DEAT
2 DATE 1910
2 PLAC Springfield
3 MAP
4 LATI N39.7817
4 LONG W89.6501
0 @I2@ INDI
1 NAME Mary /Plain/
1 SEX F
1 BIRT
2 DATE 1855
2 PLAC Salem
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1875
2 PLAC Springfield
3 MAP
4 LATI N39.7817
4 LONG W89.6501
0 @F2@ FAM
1 WIFE @I2@
1 MARR
2 DATE 1880
2 PLAC Salem
0 TRLR
`

var (
	personCoordinateFields = []string{"birth_place_latitude", "birth_place_longitude", "death_place_latitude", "death_place_longitude"}
	familyCoordinateFields = []string{"marriage_place_latitude", "marriage_place_longitude"}
)

func TestAPI_PlaceCoordinatesInPersonAndFamilyResponses(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(placeCoordinatesGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			server := newServer(t, st)
			john, mary, mapped, plain := importedIDs(t, ctx, st.read)

			johnPath, maryPath := "/api/v1/persons/"+john, "/api/v1/persons/"+mary
			mappedPath, plainPath := "/api/v1/families/"+mapped, "/api/v1/families/"+plain
			wantJohn := map[string]string{
				"birth_place_latitude": "N42.3601", "birth_place_longitude": "W71.0589",
				"death_place_latitude": "N39.7817", "death_place_longitude": "W89.6501",
			}
			wantMarriage := map[string]string{"marriage_place_latitude": "N39.7817", "marriage_place_longitude": "W89.6501"}

			assertAll := func(scope, branchID string) {
				t.Helper()
				assertCoordinates(t, scope+" person detail", getEntity(t, server, johnPath, branchID), personCoordinateFields, wantJohn)
				assertCoordinates(t, scope+" person detail without coordinates", getEntity(t, server, maryPath, branchID), personCoordinateFields, nil)
				assertCoordinates(t, scope+" family detail", getEntity(t, server, mappedPath, branchID), familyCoordinateFields, wantMarriage)
				assertCoordinates(t, scope+" family detail without coordinates", getEntity(t, server, plainPath, branchID), familyCoordinateFields, nil)

				persons := listItems(t, server, scoped("/api/v1/persons", branchID))
				assertCoordinates(t, scope+" person list", persons[john], personCoordinateFields, wantJohn)
				assertCoordinates(t, scope+" person list without coordinates", persons[mary], personCoordinateFields, nil)
				families := listItems(t, server, scoped("/api/v1/families", branchID))
				assertCoordinates(t, scope+" family list", families[mapped], familyCoordinateFields, wantMarriage)
				assertCoordinates(t, scope+" family list without coordinates", families[plain], familyCoordinateFields, nil)
			}

			assertAll("main", "")

			branch := createBranch(t, server, "coordinates")
			assertAll("branch", branch)

			// A branch edit copies the row into the branch overlay; the
			// coordinates must come along with it.
			updateSurname(t, server, johnPath, branch, "Remapped")
			mustDo(t, server, http.MethodPut, scoped(mappedPath, branch),
				fmt.Sprintf(`{"marriage_place":"Springfield","version":%d}`, entityVersion(t, server, mappedPath, branch)),
				http.StatusOK)
			assertAll("edited branch", branch)
		})
	}
}

// importedIDs returns the imported John and Mary person ids and the ids of the
// family with marriage coordinates and the one without.
func importedIDs(t *testing.T, ctx context.Context, rs repository.ReadModelStore) (john, mary, mapped, plain string) {
	t.Helper()
	persons, err := repository.ListAll(ctx, 100, rs.ListPersons)
	if err != nil || len(persons) != 2 {
		t.Fatalf("list persons = %d, %v; want two", len(persons), err)
	}
	for _, p := range persons {
		switch p.GivenName {
		case "John":
			john = p.ID.String()
		case "Mary":
			mary = p.ID.String()
		}
	}
	families, err := repository.ListAll(ctx, 100, rs.ListFamilies)
	if err != nil || len(families) != 2 {
		t.Fatalf("list families = %d, %v; want two", len(families), err)
	}
	for _, f := range families {
		if f.Partner2ID != nil && f.Partner1ID != nil {
			mapped = f.ID.String()
		} else {
			plain = f.ID.String()
		}
	}
	if john == "" || mary == "" || mapped == "" || plain == "" {
		t.Fatalf("imported ids: john=%q mary=%q mapped=%q plain=%q", john, mary, mapped, plain)
	}
	return john, mary, mapped, plain
}

// listItems reads a list endpoint and indexes its items by id.
func listItems(t *testing.T, server *api.Server, path string) map[string]map[string]any {
	t.Helper()
	items := map[string]map[string]any{}
	for _, item := range jsonArray(t, mustDo(t, server, http.MethodGet, path, "", http.StatusOK), "items") {
		m := item.(map[string]any)
		items[m["id"].(string)] = m
	}
	return items
}

// assertCoordinates checks that resp carries exactly the wanted coordinate
// fields: each field in want with its value, every other field absent.
func assertCoordinates(t *testing.T, label string, resp map[string]any, fields []string, want map[string]string) {
	t.Helper()
	if resp == nil {
		t.Errorf("%s: no response", label)
		return
	}
	for _, field := range fields {
		got, present := resp[field]
		wantValue, wantPresent := want[field]
		switch {
		case wantPresent && got != wantValue:
			t.Errorf("%s: %s = %v, want %q", label, field, got, wantValue)
		case !wantPresent && present:
			t.Errorf("%s: %s = %v, want it omitted", label, field, got)
		}
	}
}
