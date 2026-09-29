package sqlite_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// SQLite name search is a case-insensitive substring match (plain queries) or
// pg_trgm-style trigram similarity (fuzzy queries), the same semantics as the
// PostgreSQL store (ADR-002, DB-005, #822). User input is only ever bound as a
// parameter, so punctuation and FTS-style operators are literal text (#762).

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

// TestSearchPersons_LikeEscaping asserts exact result sets for plain queries.
// LIKE is a case-insensitive substring match on the raw input, so punctuation is
// literal and 'John' only matches a name containing the quotes.
func TestSearchPersons_LikeEscaping(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
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
	})
}

// TestSearchPersons_Fuzzy asserts exact result sets for fuzzy queries, which use
// trigram similarity (pg_trgm's `%` at its default 0.3 threshold) against the
// person's names and every alternate name, including nicknames.
func TestSearchPersons_Fuzzy(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	seedSearchPeople(t, store)

	runSearchCases(t, store, []searchCase{
		// "Joh" shares three of John's five trigrams (0.5) and three of Johnny's
		// seven (0.375).
		{name: "prefix", query: "Joh", fuzzy: true, want: []string{johnDoe, johnnyWalker}},
		{name: "prefix Zac", query: "Zac", fuzzy: true, want: []string{zachary}},
		// A misspelling a substring match cannot find.
		{name: "misspelled surname", query: "Thomson", fuzzy: true, want: []string{zachary}},
		{name: "misspelled given name", query: "Johm", fuzzy: true, want: []string{johnDoe, johnnyWalker}},
		// "Zackk" is close to the nickname Zack (0.57) but not to Zachary (0.27):
		// matched on the alternate name alone.
		{name: "nickname", query: "Zackk", fuzzy: true, want: []string{zachary}},
		{name: "apostrophe", query: "O'Brian", fuzzy: true, want: []string{maryAnn}},
		{name: "hyphenated", query: "Smith-Jones", fuzzy: true, want: []string{annaSJ}},
		{name: "case-insensitive", query: "JOHN", fuzzy: true, want: []string{johnDoe, johnnyWalker}},
		// Too few shared trigrams: "Jon" vs "John" is 0.29, under the threshold.
		{name: "below threshold", query: "Jon", fuzzy: true},
		{name: "punctuation only", query: "*", fuzzy: true},
		{name: "NUL only", query: "\x00", fuzzy: true},
		{name: "no match", query: "xyz123notfound", fuzzy: true},
	})
}

// TestSearchPersons_FuzzyOrderAndLimit checks fuzzy results are ordered by
// similarity (best first by default, worst first with order=asc), honor an
// explicit name sort, and are cut to the limit only after ordering.
func TestSearchPersons_FuzzyOrderAndLimit(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	seedSearchPeople(t, store)
	ctx := context.Background()

	names := func(opts repository.SearchOptions) []string {
		t.Helper()
		opts.Query, opts.Fuzzy = "John", true
		results, err := store.SearchPersons(ctx, opts)
		if err != nil {
			t.Fatalf("SearchPersons: %v", err)
		}
		var got []string
		for _, r := range results {
			got = append(got, r.FullName)
		}
		return got
	}

	// John Doe's given name is an exact match (1.0); Johnny scores 0.44.
	if got, want := names(repository.SearchOptions{Limit: 10}), []string{johnDoe, johnnyWalker}; !slices.Equal(got, want) {
		t.Errorf("relevance order = %q, want %q", got, want)
	}
	if got, want := names(repository.SearchOptions{Limit: 10, Order: "asc"}), []string{johnnyWalker, johnDoe}; !slices.Equal(got, want) {
		t.Errorf("relevance asc order = %q, want %q", got, want)
	}
	if got, want := names(repository.SearchOptions{Limit: 1}), []string{johnDoe}; !slices.Equal(got, want) {
		t.Errorf("limit 1 = %q, want the best match %q", got, want)
	}
	if got, want := names(repository.SearchOptions{Limit: 10, Sort: "name", Order: "desc"}), []string{johnnyWalker, johnDoe}; !slices.Equal(got, want) {
		t.Errorf("name desc order = %q, want %q", got, want)
	}
}

// TestSearchPersons_FuzzyFiltersAndBranches checks fuzzy matching applies the
// date/place filters and reads the branch overlay: a person created on a branch
// is found only there, and one deleted on a branch is found only on the mainline.
func TestSearchPersons_FuzzyFiltersAndBranches(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	born := func(y int) *time.Time {
		d := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		return &d
	}
	save := func(branch domain.BranchID, id uuid.UUID, given string, birth *time.Time, place string) {
		t.Helper()
		if err := store.SavePerson(ctx, branch, &repository.PersonReadModel{
			ID: id, GivenName: given, Surname: "Harrow", FullName: given + " Harrow",
			BirthDateSort: birth, BirthPlace: place, Version: 1, UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("save %s: %v", given, err)
		}
	}
	early, late, doomed := uuid.New(), uuid.New(), uuid.New()
	save(domain.MainBranchID, early, "Edmund", born(1800), "Boston")
	save(domain.MainBranchID, late, "Edmond", born(1900), "Salem")
	save(domain.MainBranchID, doomed, "Edmunt", born(1850), "Boston")

	branch := domain.BranchID(uuid.New())
	added := uuid.New()
	save(branch, added, "Edmundo", born(1880), "Boston")
	if err := store.DeletePerson(ctx, branch, doomed); err != nil {
		t.Fatalf("delete on branch: %v", err)
	}

	search := func(opts repository.SearchOptions) []uuid.UUID {
		t.Helper()
		opts.Query, opts.Fuzzy, opts.Limit = "Edmund", true, 10
		results, err := store.SearchPersons(ctx, opts)
		if err != nil {
			t.Fatalf("SearchPersons: %v", err)
		}
		var ids []uuid.UUID
		for _, r := range results {
			ids = append(ids, r.ID)
		}
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		return ids
	}
	sorted := func(ids ...uuid.UUID) []uuid.UUID {
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		return ids
	}

	if got, want := search(repository.SearchOptions{}), sorted(early, late, doomed); !slices.Equal(got, want) {
		t.Errorf("mainline = %v, want %v", got, want)
	}
	if got, want := search(repository.SearchOptions{BirthDateFrom: born(1840)}), sorted(late, doomed); !slices.Equal(got, want) {
		t.Errorf("born after 1840 = %v, want %v", got, want)
	}
	if got, want := search(repository.SearchOptions{BirthPlace: "boston"}), sorted(early, doomed); !slices.Equal(got, want) {
		t.Errorf("born in Boston = %v, want %v", got, want)
	}
	if got, want := search(repository.SearchOptions{BranchID: branch}), sorted(early, late, added); !slices.Equal(got, want) {
		t.Errorf("branch = %v, want %v", got, want)
	}
}

// TestNewReadModelStore_DropsLegacyFTS5 opens a database carrying the FTS5
// search index an earlier version created: the store drops its tables and
// triggers, so person saves no longer maintain an index nothing reads (#822).
func TestNewReadModelStore_DropsLegacyFTS5(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sqlite.OpenDB(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if _, err := sqlite.NewReadModelStore(db); err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	// Recreate what the pre-#822 store built when FTS5 was compiled in.
	for _, stmt := range []string{
		`CREATE VIRTUAL TABLE persons_fts USING fts5(given_name, surname, content='persons', content_rowid='rowid')`,
		`CREATE VIRTUAL TABLE person_names_fts USING fts5(given_name, surname, nickname, content='person_names', content_rowid='rowid')`,
		`CREATE TRIGGER persons_fts_insert AFTER INSERT ON persons BEGIN
			INSERT INTO persons_fts(rowid, given_name, surname) VALUES (NEW.rowid, NEW.given_name, NEW.surname);
		END`,
		`CREATE TRIGGER person_names_fts_insert AFTER INSERT ON person_names BEGIN
			INSERT INTO person_names_fts(rowid, given_name, surname, nickname) VALUES (NEW.rowid, NEW.given_name, NEW.surname, COALESCE(NEW.nickname, ''));
		END`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed legacy FTS5 object: %v", err)
		}
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("reopen read model store: %v", err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name LIKE '%\_fts%' ESCAPE '\'`).Scan(&left); err != nil {
		t.Fatalf("count FTS objects: %v", err)
	}
	if left != 0 {
		t.Errorf("%d legacy FTS5 objects left after opening the store, want 0", left)
	}
	seedSearchPeople(t, store)
	runSearchCases(t, store, []searchCase{{name: "search after upgrade", query: "O'Brien", want: []string{maryAnn}}})
}

// TestSearchPersons_WhitespaceQueryIsFiltersOnly verifies a whitespace-only query
// counts as no query, so date filters still apply, for plain and fuzzy search
// alike and in line with the PostgreSQL and memory stores (issue #762).
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
