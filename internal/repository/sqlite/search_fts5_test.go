package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// Issue #762: SearchPersons escaping. The FTS5 path is only compiled in when
// mattn/go-sqlite3 is built with the sqlite_fts5 tag (`make test-fts5`); tests
// that need it skip with this reason otherwise, and the LIKE-path counterparts
// run instead, so every build asserts exact result sets.
const noFTS5Reason = "SQLite built without FTS5 (mattn/go-sqlite3 needs -tags sqlite_fts5); " +
	"run `make test-fts5` to exercise the FTS5 search path"

// fts5SpecialInputs is the character list from issue #762 plus apostrophes and
// the names that exposed the escaping bugs.
var fts5SpecialInputs = []string{
	"*", "+", "-", `"`, "(", ")", ":", "^", "'",
	"O'Brien", "Smith-Jones", `"John"`, `'John'`, `"John" AND "Doe"`,
	`John OR Mary`, `NEAR(John Doe)`, `given_name:John`, `^John`, `-John`, `a"b`, `""`,
	// FTS5 stops reading a string at NUL ("unterminated string"), so control
	// characters must never reach the MATCH expression.
	"\x00", "a\x00b", "Jo\x00hn", "\x00\"", "John\x07Doe", "\x1b[0m",
}

func TestEscapeFTS5Query(t *testing.T) {
	tests := []struct {
		query  string
		prefix bool
		want   string
	}{
		{"", false, ""},
		{"   ", false, ""},
		{"", true, ""},
		{"John", false, `"John"`},
		{"John", true, `"John"*`},
		{"  John   Doe ", false, `"John" "Doe"`},
		{"Zac Tho", true, `"Zac" "Tho"*`},
		{"*", false, `"*"`},
		{"+", false, `"+"`},
		{"-", false, `"-"`},
		{`"`, false, `""""`},
		{"(", false, `"("`},
		{")", false, `")"`},
		{":", false, `":"`},
		{"^", false, `"^"`},
		{"'", false, `"'"`},
		{"O'Brien", false, `"O'Brien"`},
		{"O'Brien", true, `"O'Brien"*`},
		{"Smith-Jones", false, `"Smith-Jones"`},
		{`"John"`, false, `"""John"""`},
		{`"John"`, true, `"""John"""*`},
		{`'John'`, false, `"'John'"`},
		{`a"b`, false, `"a""b"`},
		{`John OR Mary`, false, `"John" "OR" "Mary"`},
		{`given_name:John`, false, `"given_name:John"`},
		{"\x00", false, ""},
		{"\x00", true, ""},
		{"a\x00b", false, `"a" "b"`},
		{"Jo\x00hn", true, `"Jo" "hn"*`},
		{"John\x07Doe", false, `"John" "Doe"`},
		{"\x00\"", false, `""""`},
	}
	for _, tt := range tests {
		got := sqlite.EscapeFTS5Query(tt.query, tt.prefix)
		if got != tt.want {
			t.Errorf("EscapeFTS5Query(%q, %v) = %q, want %q", tt.query, tt.prefix, got, tt.want)
		}
	}
}

// openRawFTS5DB opens a scratch database with a bare FTS5 table, skipping when the
// build has no FTS5 module.
func openRawFTS5DB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "fts5.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE VIRTUAL TABLE t USING fts5(given_name, surname)`); err != nil {
		t.Skipf("%s (create fts5 table: %v)", noFTS5Reason, err)
	}
	if _, err := db.Exec(`INSERT INTO t(given_name, surname) VALUES ('John', 'Doe'), ('Mary-Ann', 'O''Brien')`); err != nil {
		t.Fatalf("seed fts5 table: %v", err)
	}
	return db
}

// TestEscapeFTS5Query_ValidSyntax proves every escaped input is a valid FTS5 MATCH
// expression, with and without the fuzzy prefix operator.
func TestEscapeFTS5Query_ValidSyntax(t *testing.T) {
	db := openRawFTS5DB(t)
	for _, in := range fts5SpecialInputs {
		for _, prefix := range []bool{false, true} {
			q := sqlite.EscapeFTS5Query(in, prefix)
			if q == "" {
				// No tokens: the documented contract is that callers never pass ""
				// to MATCH (searchPersonsFTS returns no results instead).
				if strings.TrimFunc(in, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) != "" {
					t.Errorf("input %q (prefix=%v) unexpectedly escaped to empty", in, prefix)
				}
				continue
			}
			rows, err := db.Query(`SELECT rowid FROM t WHERE t MATCH ?`, q)
			if err != nil {
				t.Errorf("input %q (prefix=%v) -> %q: invalid FTS5: %v", in, prefix, q, err)
				continue
			}
			for rows.Next() {
			}
			if err := rows.Err(); err != nil {
				t.Errorf("input %q (prefix=%v) -> %q: iterate: %v", in, prefix, q, err)
			}
			rows.Close()
		}
	}
}

// TestFTS5_JohnQuirkRootCause pins down the pre-#762 'John' count quirk: the old
// escaper left apostrophes untouched, so 'John' reached MATCH verbatim, which is an
// FTS5 syntax error; the error was swallowed by the LIKE fallback, which searched
// for the literal substring "'john'" and found nothing, while John found John Doe.
// Old per-character quoting likewise broke on a bare quote (`"` -> `"""`) and
// turned O'Brien-style hyphenated names into separate phrases (O"-"Brien).
func TestFTS5_JohnQuirkRootCause(t *testing.T) {
	db := openRawFTS5DB(t)
	// What the pre-#762 per-character escaper produced for each input.
	oldEscaped := map[string]string{
		`'John'`:   `'John'`,     // apostrophes were not escaped at all
		`O'Brien`:  `O'Brien`,    // likewise
		`'`:        `'`,          // likewise
		`"`:        `"""`,        // unterminated string
		`a"b`:      `a"""b`,      // unterminated string
		`Mary-Ann`: `Mary"-"Ann`, // valid, but three phrases rather than one term
	}
	for _, in := range []string{`'John'`, `O'Brien`, `'`, `"`, `a"b`} {
		if _, err := db.Exec(`SELECT rowid FROM t WHERE t MATCH ?`, oldEscaped[in]); err == nil {
			t.Errorf("old escaping of %q (%q) unexpectedly valid FTS5", in, oldEscaped[in])
		}
		if _, err := db.Exec(`SELECT rowid FROM t WHERE t MATCH ?`, sqlite.EscapeFTS5Query(in, false)); err != nil {
			t.Errorf("new escaping of %q (%q) invalid FTS5: %v", in, sqlite.EscapeFTS5Query(in, false), err)
		}
	}
}

// seedSearchPeople saves the fixed population the escaping tests search.
func seedSearchPeople(t *testing.T, store *sqlite.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	people := []struct{ given, surname, nickname string }{
		{"John", "Doe", ""},
		{"Johnny", "Walker", ""},
		{"Mary-Ann", "O'Brien", ""},
		{"Anna", "Smith-Jones", ""},
		{"Zachary", "Thompson", "Zack"},
	}
	for _, p := range people {
		id := uuid.New()
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: id, GivenName: p.given, Surname: p.surname, FullName: p.given + " " + p.surname,
			Version: 1, UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("save person %s: %v", p.given, err)
		}
		if p.nickname != "" {
			if err := store.SavePersonName(ctx, domain.MainBranchID, &repository.PersonNameReadModel{
				ID: uuid.New(), PersonID: id, GivenName: p.given, Surname: p.surname,
				FullName: p.given + " " + p.surname, Nickname: p.nickname, UpdatedAt: time.Now(),
			}); err != nil {
				t.Fatalf("save person name %s: %v", p.nickname, err)
			}
		}
	}
}

type searchCase struct {
	name  string
	query string
	fuzzy bool
	want  []string // full names, any order
}

func runSearchCases(t *testing.T, store *sqlite.ReadModelStore, cases []searchCase) {
	t.Helper()
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: tc.query, Fuzzy: tc.fuzzy, Limit: 100})
			if err != nil {
				t.Fatalf("SearchPersons(%q, fuzzy=%v): %v", tc.query, tc.fuzzy, err)
			}
			got := make([]string, 0, len(results))
			for _, r := range results {
				got = append(got, r.FullName)
			}
			slices.Sort(got)
			want := slices.Clone(tc.want)
			if want == nil {
				want = []string{}
			}
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("SearchPersons(%q, fuzzy=%v) = %q, want %q", tc.query, tc.fuzzy, got, want)
			}
		})
	}
}

const (
	johnDoe      = "John Doe"
	johnnyWalker = "Johnny Walker"
	maryAnn      = "Mary-Ann O'Brien"
	annaSJ       = "Anna Smith-Jones"
	zachary      = "Zachary Thompson"
)

// TestSearchPersons_FTS5Escaping asserts exact result sets on the FTS5 path.
func TestSearchPersons_FTS5Escaping(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	if !store.FTS5Enabled() {
		t.Skip(noFTS5Reason)
	}
	seedSearchPeople(t, store)

	runSearchCases(t, store, []searchCase{
		{name: "asterisk", query: "*"},
		{name: "plus", query: "+"},
		{name: "hyphen", query: "-"},
		{name: "double quote", query: `"`},
		{name: "open paren", query: "("},
		{name: "close paren", query: ")"},
		{name: "colon", query: ":"},
		{name: "caret", query: "^"},
		{name: "apostrophe", query: "'"},
		{name: "O'Brien", query: "O'Brien", want: []string{maryAnn}},
		{name: "o'brien lowercase", query: "o'brien", want: []string{maryAnn}},
		{name: "Smith-Jones", query: "Smith-Jones", want: []string{annaSJ}},
		{name: "Mary-Ann", query: "Mary-Ann", want: []string{maryAnn}},
		{name: "parenthesized", query: "(Mary)", want: []string{maryAnn}},
		{name: "bare John", query: "John", want: []string{johnDoe}},
		{name: "double-quoted John", query: `"John"`, want: []string{johnDoe}},
		// The 'John' quirk: before #762 this returned 0 while John returned 1.
		{name: "apostrophe-quoted John", query: "'John'", want: []string{johnDoe}},
		{name: "two terms", query: "John Doe", want: []string{johnDoe}},
		{name: "two terms reversed", query: "Doe John", want: []string{johnDoe}},
		{name: "operators are literal", query: `"John" AND "Doe"`},
		{name: "OR is literal", query: "John OR Mary"},
		{name: "column filter is literal", query: "given_name:John"},
		{name: "alternate name nickname", query: "Zack", want: []string{zachary}},
		{name: "empty query", query: ""},
		{name: "whitespace query", query: "   "},
		{name: "padded query", query: "  O'Brien  ", want: []string{maryAnn}},
		{name: "NUL only", query: "\x00"},
		{name: "NUL only fuzzy", query: "\x00", fuzzy: true},
		{name: "embedded NUL", query: "a\x00b"},
		{name: "embedded NUL fuzzy", query: "Jo\x00hn", fuzzy: true},
		{name: "NUL separates terms", query: "John\x00Doe", want: []string{johnDoe}},
		{name: "fuzzy prefix", query: "Joh", fuzzy: true, want: []string{johnDoe, johnnyWalker}},
		{name: "fuzzy prefix Zac", query: "Zac", fuzzy: true, want: []string{zachary}},
		{name: "fuzzy hyphenated prefix", query: "Smith-Jo", fuzzy: true, want: []string{annaSJ}},
		{name: "fuzzy quoted prefix", query: `"Joh`, fuzzy: true, want: []string{johnDoe, johnnyWalker}},
		{name: "fuzzy apostrophe-quoted", query: "'John'", fuzzy: true, want: []string{johnDoe, johnnyWalker}},
		{name: "fuzzy punctuation only", query: "*", fuzzy: true},
		{name: "fuzzy no match", query: "xyz123notfound", fuzzy: true},
		// A fuzzy search that FTS5 cannot match falls back to LIKE substring matching.
		{name: "fuzzy falls back to LIKE", query: "-", fuzzy: true, want: []string{maryAnn, annaSJ}},
	})
}

// TestSearchPersons_LikeEscaping asserts exact result sets on the LIKE path taken
// when the build has no FTS5. LIKE is a case-insensitive substring match on the raw
// input, so punctuation is literal and 'John' only matches a name containing the
// quotes.
func TestSearchPersons_LikeEscaping(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	if store.FTS5Enabled() {
		t.Skip("SQLite built with FTS5: SearchPersons takes the FTS5 path (see TestSearchPersons_FTS5Escaping)")
	}
	seedSearchPeople(t, store)

	runSearchCases(t, store, []searchCase{
		{name: "asterisk", query: "*"},
		{name: "plus", query: "+"},
		{name: "hyphen", query: "-", want: []string{maryAnn, annaSJ}},
		{name: "double quote", query: `"`},
		{name: "open paren", query: "("},
		{name: "close paren", query: ")"},
		{name: "colon", query: ":"},
		{name: "caret", query: "^"},
		{name: "apostrophe", query: "'", want: []string{maryAnn}},
		{name: "O'Brien", query: "O'Brien", want: []string{maryAnn}},
		{name: "Smith-Jones", query: "Smith-Jones", want: []string{annaSJ}},
		{name: "Mary-Ann", query: "Mary-Ann", want: []string{maryAnn}},
		{name: "parenthesized", query: "(Mary)"},
		{name: "bare John", query: "John", want: []string{johnDoe, johnnyWalker}},
		{name: "double-quoted John", query: `"John"`},
		{name: "apostrophe-quoted John", query: "'John'"},
		{name: "two terms", query: "John Doe", want: []string{johnDoe}},
		{name: "operators are literal", query: `"John" AND "Doe"`},
		{name: "alternate name nickname", query: "Zack", want: []string{zachary}},
		{name: "empty query", query: ""},
		{name: "whitespace query", query: "   "},
		{name: "padded query", query: "  O'Brien  ", want: []string{maryAnn}},
		{name: "fuzzy prefix", query: "Joh", fuzzy: true, want: []string{johnDoe, johnnyWalker}},
		{name: "fuzzy prefix Zac", query: "Zac", fuzzy: true, want: []string{zachary}},
		{name: "fuzzy no match", query: "xyz123notfound", fuzzy: true},
	})
}

// TestSearchPersons_WhitespaceQueryIsFiltersOnly verifies a whitespace-only query
// counts as no query, so date filters still apply, identically on the FTS5 and
// LIKE paths and in line with the PostgreSQL and memory stores (issue #762).
func TestSearchPersons_WhitespaceQueryIsFiltersOnly(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	born := func(y int) *time.Time {
		d := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		return &d
	}
	for _, p := range []struct {
		given string
		birth *time.Time
	}{{"Early", born(1800)}, {"Late", born(1900)}} {
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: uuid.New(), GivenName: p.given, Surname: "Test", FullName: p.given + " Test",
			BirthDateSort: p.birth, Version: 1, UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("save person %s: %v", p.given, err)
		}
	}
	for _, q := range []string{"", "  ", " \t "} {
		for _, fuzzy := range []bool{false, true} {
			results, err := store.SearchPersons(ctx, repository.SearchOptions{
				Query: q, Fuzzy: fuzzy, BirthDateFrom: born(1850), Limit: 10,
			})
			if err != nil {
				t.Fatalf("SearchPersons(%q, fuzzy=%v): %v", q, fuzzy, err)
			}
			if len(results) != 1 || results[0].GivenName != "Late" {
				t.Errorf("SearchPersons(%q, fuzzy=%v) with birth filter = %d results, want only Late", q, fuzzy, len(results))
			}
		}
	}
}

// TestSearchPersons_FTS5ErrorSurfaced verifies a failing FTS5 query is returned as
// an error instead of being masked by LIKE results (the #762 fallback decision).
func TestSearchPersons_FTS5ErrorSurfaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "surface.db")
	db, err := sqlite.OpenDB(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	if !store.FTS5Enabled() {
		t.Skip(noFTS5Reason)
	}
	seedSearchPeople(t, store)

	// Break the FTS5 index out from under the store: the MATCH query now fails.
	if _, err := db.Exec(`DROP TABLE person_names_fts`); err != nil {
		t.Fatalf("drop person_names_fts: %v", err)
	}
	results, err := store.SearchPersons(context.Background(), repository.SearchOptions{Query: "John", Limit: 10})
	if err == nil {
		t.Fatalf("expected FTS5 error to surface, got %d results and nil error", len(results))
	}
}
