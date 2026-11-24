package api_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/repository"
)

// unmappedGEDCOM places a record of each of the five kinds: Ann's birth,
// burial and marriage in Boston without a pair, her death there with one,
// her residence in "Boston MA" (no comma: another place) and Bob's birth in
// Salem.
const unmappedGEDCOM = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Ann /Unmapped/
1 SEX F
1 BIRT
2 DATE 1850
2 PLAC Boston, MA
1 DEAT
2 DATE 1900
2 PLAC Boston,MA
3 MAP
4 LATI N42.36
4 LONG W71.06
1 BURI
2 PLAC boston, ma
1 RESI
2 PLAC Boston MA
0 @I2@ INDI
1 NAME Bob /Unmapped/
1 SEX M
1 BIRT
2 DATE 1848
2 PLAC Salem, MA
0 @F1@ FAM
1 HUSB @I2@
1 WIFE @I1@
1 MARR
2 PLAC Boston, MA
0 TRLR
`

// TestListUnmappedPlaces drives listUnmappedPlaces on every store: each
// place with records of any of the five kinds lacking coordinates, counted
// and sorted by count then name, with the pairs other records there carry;
// setting the place's pair through updatePlace takes it off the list.
func TestListUnmappedPlaces(t *testing.T) {
	forEachAPIBackend(t, func(t *testing.T, server *api.Server) {
		importGEDCOMText(t, server, unmappedGEDCOM)

		entry := func(key, name string, count int, suggestions ...any) map[string]any {
			if suggestions == nil {
				suggestions = []any{}
			}
			return map[string]any{"id": repository.PlaceID(key).String(), "key": key, "name": name, "spelling": name,
				"count": float64(count), "suggestions": suggestions}
		}
		boston := entry("boston,ma", "Boston, MA", 3, map[string]any{"latitude": "N42.36", "longitude": "W71.06", "count": float64(1)})
		want := map[string]any{"items": []any{boston, entry("boston ma", "Boston MA", 1), entry("salem,ma", "Salem, MA", 1)}, "total": float64(3)}
		if got := readRecord(t, server, "/api/v1/places/unmapped"); !reflect.DeepEqual(got, want) {
			t.Errorf("listUnmappedPlaces =\n  %v\nwant\n  %v", got, want)
		}

		rec := do(t, server, http.MethodPut, "/api/v1/places/"+repository.PlaceID("boston,ma").String(),
			`{"version":0,"latitude":"N42.36","longitude":"W71.06"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("updatePlace: %d %s", rec.Code, rec.Body)
		}
		want = map[string]any{"items": []any{entry("boston ma", "Boston MA", 1), entry("salem,ma", "Salem, MA", 1)}, "total": float64(2)}
		if got := readRecord(t, server, "/api/v1/places/unmapped"); !reflect.DeepEqual(got, want) {
			t.Errorf("after updatePlace, listUnmappedPlaces =\n  %v\nwant\n  %v", got, want)
		}
	})
}
