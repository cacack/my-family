package integration_test

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestSearchParity runs one fixed set of name searches against every backend
// and expects the same people from each (DB-005, #822). SQLite search is a
// substring scan and a Go port of pg_trgm similarity rather than PostgreSQL's
// tsvector + trigram indexes (ADR-002), so this is what keeps the two in step:
// if either backend's matching drifts, a query here returns a different set.
//
// Queries are the ones #822 names: a whole name, a prefix, apostrophe and
// hyphen names, a multi-word query, and fuzzy against non-fuzzy.
func TestSearchParity(t *testing.T) {
	forEachBackend(t, runSearchParity)
}

func runSearchParity(t *testing.T, server *api.Server) {
	ids := map[string]string{}
	for _, p := range []struct{ given, surname string }{
		{"John", "Smith"},
		{"Alice", "Johnson"},
		{"Patrick", "O'Brien"},
		{"Mary", "Smith-Jones"},
		{"Zachary", "Thompson"},
		{"Edmund", "Blackwood"},
	} {
		ids[p.given] = createPerson(t, server, p.given, p.surname)
	}
	mustDo(t, server, http.MethodPost, "/api/v1/persons/"+ids["Zachary"]+"/names",
		`{"given_name":"Zachary","surname":"Thompson","nickname":"Zack","name_type":"aka"}`, http.StatusCreated)
	mustDo(t, server, http.MethodPost, "/api/v1/persons/"+ids["Edmund"]+"/names",
		`{"given_name":"Edmund","surname":"Harrow","name_type":"married"}`, http.StatusCreated)

	tests := []struct {
		query string
		fuzzy bool
		want  []string // given names
	}{
		// Plain search: a case-insensitive substring of a name, so a whole name
		// also finds longer names that contain it.
		{"John", false, []string{"John", "Alice"}},
		{"john", false, []string{"John", "Alice"}},
		{"Joh", false, []string{"John", "Alice"}},
		{"O'Brien", false, []string{"Patrick"}},
		{"Smith-Jones", false, []string{"Mary"}},
		{"Smith", false, []string{"John", "Mary"}},
		{"John Smith", false, []string{"John"}},
		{"Zack", false, []string{"Zachary"}},  // alternate name's nickname
		{"Harrow", false, []string{"Edmund"}}, // alternate name's surname
		{"Smyth", false, nil},
		{"xyzzy", false, nil},

		// Fuzzy search: pg_trgm similarity >= 0.3 against any name.
		{"Smyth", true, []string{"John"}},
		{"Johnsen", true, []string{"John", "Alice"}},
		{"O'Brian", true, []string{"Patrick"}},
		{"Tompson", true, []string{"Zachary"}},
		{"Zackk", true, []string{"Zachary"}},
		{"Harow", true, []string{"Edmund"}},
		// "Jon" misses John (0.29) but reaches Johnson (0.33): the shared
		// trailing "on " trigram counts. A prefix rule would get this backwards.
		{"Jon", true, []string{"Alice"}},
		{"xyzzy", true, nil},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/fuzzy=%v", tt.query, tt.fuzzy), func(t *testing.T) {
			path := "/api/v1/search?q=" + url.QueryEscape(tt.query)
			if tt.fuzzy {
				path += "&fuzzy=true"
			}
			got := itemIDs(t, mustDo(t, server, http.MethodGet, path, "", http.StatusOK))
			want := make([]string, 0, len(tt.want))
			for _, given := range tt.want {
				want = append(want, ids[given])
			}
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("search %q (fuzzy=%v) = %v, want %v", tt.query, tt.fuzzy, namesOf(ids, got), tt.want)
			}
		})
	}
}

// namesOf maps person ids back to the given names they were created with, so a
// failure reads as names rather than UUIDs.
func namesOf(ids map[string]string, got []string) []string {
	names := make([]string, 0, len(got))
	for _, id := range got {
		name := id
		for given, pid := range ids {
			if pid == id {
				name = given
			}
		}
		names = append(names, name)
	}
	return names
}
