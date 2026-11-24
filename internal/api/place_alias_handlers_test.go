package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/storage"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

// forEachAPIBackend runs test against a server on fresh stores of each
// backend. PostgreSQL skips without Docker (see pgtest).
func forEachAPIBackend(t *testing.T, test func(t *testing.T, server *api.Server)) {
	t.Helper()
	backends := []struct {
		name string
		open func(t *testing.T) (*storage.Stores, error)
	}{
		{"Memory", func(*testing.T) (*storage.Stores, error) { return storage.OpenMemory(), nil }},
		{"SQLite", func(t *testing.T) (*storage.Stores, error) {
			return storage.OpenSQLite(t.TempDir()+"/myfamily.db", storage.AccessServe)
		}},
		{"Postgres", func(t *testing.T) (*storage.Stores, error) {
			return storage.OpenPostgres(pgtest.NewDatabase(t), storage.AccessServe)
		}},
	}
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st, err := backend.open(t)
			if err != nil {
				t.Fatalf("open stores: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })
			cfg := &config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeOpen}
			test(t, api.NewServer(cfg, st.Events, st.ReadModel, st.Snapshots, nil, api.WithBranchStore(st.Branches)))
		})
	}
}

// TestPlaces_AliasAndUnalias drives aliasPlace and unaliasPlace end to end on
// every store (ADR-009 §3): after an alias the list shows the target with the
// absorbed key among its aliases, every record, resolved place and map
// location carries the target's ID and reads its facts, each record keeps its
// spelling and the GEDCOM export writes the same PLAC lines (PL-001), and a
// GET of the absorbed place returns its own facts and the ID it resolves to.
// After an unalias both places are back.
func TestPlaces_AliasAndUnalias(t *testing.T) {
	forEachAPIBackend(t, func(t *testing.T, server *api.Server) {
		create := func(path, body string) string {
			t.Helper()
			rec := do(t, server, http.MethodPost, path, body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("POST %s: %d %s", path, rec.Code, rec.Body)
			}
			return decodeJSON(t, rec)["id"].(string)
		}
		ann := create("/api/v1/persons", `{"given_name":"Ann","surname":"Lee","birth_place":"Köln, Germany"}`)
		create("/api/v1/persons", `{"given_name":"Bob","surname":"Lee","birth_place":"Cologne, Germany"}`)
		family := create("/api/v1/families", `{"partner1_id":"`+ann+`","marriage_place":"Köln, Germany"}`)

		koln, cologne := repository.PlaceID("köln,germany").String(), repository.PlaceID("cologne,germany").String()
		if rec := do(t, server, http.MethodPut, "/api/v1/places/"+cologne,
			`{"version":0,"name":"Cologne","latitude":"N50.94","longitude":"E6.96"}`); rec.Code != http.StatusOK {
			t.Fatalf("updatePlace Cologne: %d %s", rec.Code, rec.Body)
		}
		if rec := do(t, server, http.MethodPut, "/api/v1/places/"+koln, `{"version":0,"latitude":"N1","longitude":"E1"}`); rec.Code != http.StatusOK {
			t.Fatalf("updatePlace Köln: %d %s", rec.Code, rec.Body)
		}
		plac := func() []string {
			t.Helper()
			rec := do(t, server, http.MethodGet, "/api/v1/gedcom/export", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("export: %d %s", rec.Code, rec.Body)
			}
			var lines []string
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if strings.Contains(line, " PLAC ") {
					lines = append(lines, strings.TrimRight(line, "\r"))
				}
			}
			return lines
		}
		exported := plac()
		if len(exported) != 3 {
			t.Fatalf("exported PLAC lines = %q, want the two births and the marriage", exported)
		}

		aliasPath := "/api/v1/places/" + koln + "/alias"
		rec := do(t, server, http.MethodPut, aliasPath, `{"version":1,"target_id":"`+cologne+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("aliasPlace: %d %s", rec.Code, rec.Body)
		}
		// The absorbed place reads as itself: its own pair, its version and
		// the ID it resolves to.
		for _, absorbed := range []map[string]any{decodeJSON(t, rec), decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/places/"+koln, ""))} {
			if absorbed["id"] != koln || absorbed["resolves_to"] != cologne || absorbed["latitude"] != "N1" || absorbed["version"] != float64(2) {
				t.Errorf("absorbed place = %v, want Köln at its own pair, version 2, resolving to Cologne", absorbed)
			}
		}

		list := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/places?limit=100", ""))
		var keys []string
		var target map[string]any
		for _, item := range list["items"].([]any) {
			m := item.(map[string]any)
			keys = append(keys, m["key"].(string))
			if m["id"] == cologne {
				target = m
			}
		}
		if want := []string{"cologne,germany", "germany"}; !slices.Equal(keys, want) {
			t.Errorf("listPlaces keys = %v, want %v: Köln is no entry of its own", keys, want)
		}
		if aliases, _ := target["aliases"].([]any); target["count"] != float64(3) || len(aliases) != 1 ||
			aliases[0].(map[string]any)["id"] != koln || aliases[0].(map[string]any)["name"] != "Köln, Germany" {
			t.Errorf("Cologne = %v, want a count of 3 with Köln among its aliases", target)
		}

		annBody := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/persons/"+ann, ""))
		if r, _ := annBody["birth_place_resolved"].(map[string]any); annBody["birth_place"] != "Köln, Germany" ||
			r["id"] != cologne || r["name"] != "Cologne" || r["latitude"] != "N50.94" {
			t.Errorf("Ann = %v / %v, want Köln, Germany as recorded resolving to Cologne", annBody["birth_place"], r)
		}
		familyBody := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/families/"+family, ""))
		if r, _ := familyBody["marriage_place_resolved"].(map[string]any); familyBody["marriage_place"] != "Köln, Germany" || r["id"] != cologne {
			t.Errorf("family marriage place = %v / %v, want Köln, Germany resolving to Cologne", familyBody["marriage_place"], r)
		}
		pins := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/map/locations", ""))["items"].([]any)
		if len(pins) != 1 || pins[0].(map[string]any)["place_id"] != cologne || pins[0].(map[string]any)["count"] != float64(2) {
			t.Errorf("map locations = %v, want Ann and Bob under one Cologne pin", pins)
		}
		if got := plac(); !slices.Equal(got, exported) {
			t.Errorf("PLAC lines after the alias = %q, want %q as before (PL-001)", got, exported)
		}

		// The command's checks answer 400 and a stale version 409.
		if rec := do(t, server, http.MethodPut, "/api/v1/places/"+cologne+"/alias", `{"version":1,"target_id":"`+koln+`"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("aliasPlace to an alias: %d %s, want 400", rec.Code, rec.Body)
		}
		if rec := do(t, server, http.MethodDelete, aliasPath+"?version=1", ""); rec.Code != http.StatusConflict {
			t.Errorf("stale unaliasPlace: %d %s, want 409", rec.Code, rec.Body)
		}

		rec = do(t, server, http.MethodDelete, aliasPath+"?version=2", "")
		if body := decodeJSON(t, rec); rec.Code != http.StatusOK || body["resolves_to"] != nil || body["version"] != float64(3) {
			t.Fatalf("unaliasPlace: %d %v", rec.Code, body)
		}
		if rec := do(t, server, http.MethodDelete, aliasPath+"?version=3", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("unaliasPlace of a place that is no alias: %d %s, want 400", rec.Code, rec.Body)
		}
		list = decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/places?limit=100", ""))
		if list["total"] != float64(3) {
			t.Errorf("listPlaces after the unalias = %v, want Cologne, Germany and Köln", list["items"])
		}
		annBody = decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/persons/"+ann, ""))
		if r, _ := annBody["birth_place_resolved"].(map[string]any); r["id"] != koln || r["latitude"] != "N1" {
			t.Errorf("Ann's resolved birth place after the unalias = %v, want Köln at its own pair", r)
		}

		// History: the alias and the unalias name the place they join and
		// separate.
		var aliasOf []string
		for _, item := range decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/history?entity_type=place&limit=100", ""))["items"].([]any) {
			if e := item.(map[string]any); e["entity_id"] == koln {
				if c, ok := e["changes"].(map[string]any)["alias_of"].(map[string]any); ok {
					aliasOf = append(aliasOf, fmt.Sprintf("%v -> %v", c["old_value"], c["new_value"]))
				}
			}
		}
		slices.Sort(aliasOf)
		if want := []string{"<nil> -> Cologne, Germany", "Cologne, Germany -> <nil>"}; !slices.Equal(aliasOf, want) {
			t.Errorf("Köln's alias history = %q, want %q", aliasOf, want)
		}
	})
}
