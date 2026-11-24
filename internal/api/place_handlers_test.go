package api_test

import (
	"net/http"
	"slices"
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
