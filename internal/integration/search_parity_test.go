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
// hyphen names, a multi-word query, and fuzzy against non-fuzzy. Beyond those:
// non-ASCII case (SQLite's own LOWER/LIKE fold only ASCII), ILIKE's wildcards
// and backslash escape, padded place filters, and an alternate name with a
// prefix and suffix (the SQL full_name columns leave both out).
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
		{"Ana", "ÑÚÑEZ"},
	} {
		ids[p.given] = createPerson(t, server, p.given, p.surname)
	}
	for _, p := range []struct{ given, surname, birthPlace string }{
		{"Hans", "Müller", "London, England"},
		{"Karl", "MÜLLER", "MÜNCHEN, Bayern"},
	} {
		resp := mustDo(t, server, http.MethodPost, "/api/v1/persons",
			fmt.Sprintf(`{"given_name":%q,"surname":%q,"gender":"unknown","birth_place":%q}`, p.given, p.surname, p.birthPlace),
			http.StatusCreated)
		ids[p.given] = mustString(t, resp, "id")
	}
	mustDo(t, server, http.MethodPost, "/api/v1/persons/"+ids["Zachary"]+"/names",
		`{"given_name":"Zachary","surname":"Thompson","nickname":"Zack","name_type":"aka"}`, http.StatusCreated)
	mustDo(t, server, http.MethodPost, "/api/v1/persons/"+ids["Edmund"]+"/names",
		`{"given_name":"Edmund","surname":"Harrow","name_type":"married"}`, http.StatusCreated)
	mustDo(t, server, http.MethodPost, "/api/v1/persons/"+ids["Hans"]+"/names",
		`{"given_name":"Johann","surname":"Beethoven","name_prefix":"Dr.","surname_prefix":"van","name_suffix":"Jr.","name_type":"aka"}`,
		http.StatusCreated)

	tests := []struct {
		query string
		fuzzy bool
		extra string   // further query parameters
		want  []string // given names
	}{
		// Plain search: a case-insensitive substring of a name, so a whole name
		// also finds longer names that contain it.
		{"John", false, "", []string{"John", "Alice"}},
		{"john", false, "", []string{"John", "Alice"}},
		{"Joh", false, "", []string{"John", "Alice", "Hans"}}, // Hans's alternate name Johann
		{"O'Brien", false, "", []string{"Patrick"}},
		{"Smith-Jones", false, "", []string{"Mary"}},
		{"Smith", false, "", []string{"John", "Mary"}},
		{"John Smith", false, "", []string{"John"}},
		{"Zack", false, "", []string{"Zachary"}},  // alternate name's nickname
		{"Harrow", false, "", []string{"Edmund"}}, // alternate name's surname
		{"Smyth", false, "", nil},
		{"xyzzy", false, "", nil},

		// Case folds for every letter, whichever side is upper-case.
		{"müller", false, "", []string{"Hans", "Karl"}},
		{"MÜLLER", false, "", []string{"Hans", "Karl"}},
		{"Müller", false, "", []string{"Hans", "Karl"}},
		{"ñúñez", false, "", []string{"Ana"}},
		{"ÑÚÑEZ", false, "", []string{"Ana"}},

		// ILIKE: % and _ are wildcards and backslash escapes the next character.
		{"Sm_th", false, "", []string{"John", "Mary"}},
		{"Jo%son", false, "", []string{"Alice"}},
		{`O\'Brien`, false, "", []string{"Patrick"}},
		{`Smith\-Jones`, false, "", []string{"Mary"}},

		// An alternate name's prefix and suffix are not part of its full name.
		{"Johann Beethoven", false, "", []string{"Hans"}},
		{"Dr.", false, "", nil},
		{"van Beethoven", false, "", nil},

		// Place filters fold case like names and are trimmed; a blank one is no filter.
		{"Müller", false, "&birth_place=m%C3%BCnchen", []string{"Karl"}},
		{"Müller", false, "&birth_place=%20London", []string{"Hans"}},
		{"Müller", false, "&birth_place=%20", []string{"Hans", "Karl"}},
		{"Müller", true, "&birth_place=%20london%20", []string{"Hans"}},

		// Fuzzy search: pg_trgm similarity >= 0.3 against any name.
		{"Smyth", true, "", []string{"John"}},
		{"Johnsen", true, "", []string{"John", "Alice"}},
		{"O'Brian", true, "", []string{"Patrick"}},
		{"Tompson", true, "", []string{"Zachary"}},
		{"Zackk", true, "", []string{"Zachary"}},
		{"Harow", true, "", []string{"Edmund"}},
		{"MULLER", true, "", []string{"Hans", "Karl"}}, // shares "  m", "lle", "ler" and "er " (0.4)
		{"müller", true, "", []string{"Hans", "Karl"}},
		{"Beethoven", true, "", []string{"Hans"}},
		// Scored against "Johann Beethoven", not "Dr. Johann van Beethoven Jr.".
		{"Dr van Jr", true, "", nil},
		// "Jon" misses John (0.29) but reaches Johnson (0.33): the shared
		// trailing "on " trigram counts. A prefix rule would get this backwards.
		{"Jon", true, "", []string{"Alice"}},
		{"xyzzy", true, "", nil},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/fuzzy=%v%s", tt.query, tt.fuzzy, tt.extra), func(t *testing.T) {
			path := "/api/v1/search?q=" + url.QueryEscape(tt.query) + tt.extra
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
				t.Errorf("search %q (fuzzy=%v%s) = %v, want %v", tt.query, tt.fuzzy, tt.extra, namesOf(ids, got), tt.want)
			}
		})
	}
}

// TestSearchParity_FuzzyLimit checks that when more people match a fuzzy query
// than the limit returns, both SQL backends return the same people in the same
// order: best score first, ties by surname, given name and id (DB-005). Without
// the tie-break PostgreSQL picks among equal scores arbitrarily. Plain search
// has no such guarantee (PostgreSQL ranks by ts_rank, SQLite by name; see
// ADR-002), and the in-memory demo store keeps relevance results in insertion
// order, so both are out of scope here.
func TestSearchParity_FuzzyLimit(t *testing.T) {
	for _, b := range backends {
		if b.name == "Memory" {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			server := newServer(t, b.setup(t))
			ids := map[string]string{"John": createPerson(t, server, "John", "Smith")}
			// 25 people whose given names score the same against "John", with
			// surnames in the reverse order of their given names, so neither
			// creation order nor given-name order is the tie-break.
			for i := range 25 {
				given := "John" + string(rune('a'+i))
				ids[given] = createPerson(t, server, given, "Zed"+string(rune('z'-i)))
			}
			got := itemIDs(t, mustDo(t, server, http.MethodGet, "/api/v1/search?q=John&fuzzy=true&limit=5", "", http.StatusOK))
			// John scores 1; each Johnx scores 4/7 on its given name.
			want := []string{ids["John"], ids["Johny"], ids["Johnx"], ids["Johnw"], ids["Johnv"]}
			if !slices.Equal(got, want) {
				t.Errorf("fuzzy John, limit 5 = %v, want [John Johny Johnx Johnw Johnv]", namesOf(ids, got))
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
