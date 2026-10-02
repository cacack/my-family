package memory_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// searchFixture saves a small tree with alternate names, dates and places and
// returns the store plus each person's id keyed by given name.
func searchFixture(t *testing.T) (*memory.ReadModelStore, map[string]uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	store := memory.NewReadModelStore()
	day := func(y int) *time.Time {
		d := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		return &d
	}
	ids := map[string]uuid.UUID{}
	for _, p := range []struct {
		given, surname, birthPlace string
		birth, death               *time.Time
	}{
		{"John", "Smith", "London, England", day(1850), day(1910)},
		{"Zachary", "Thompson", "MÜNCHEN, Bayern", day(1870), nil},
		{"Edmund", "Blackwood", "Boston", nil, day(1900)},
	} {
		id := uuid.New()
		ids[p.given] = id
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: id, GivenName: p.given, Surname: p.surname, FullName: p.given + " " + p.surname,
			BirthPlace: p.birthPlace, BirthDateSort: p.birth, DeathDateSort: p.death,
			Version: 1, UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("save %s: %v", p.given, err)
		}
	}
	for _, n := range []repository.PersonNameReadModel{
		{PersonID: ids["Zachary"], GivenName: "Zachary", Surname: "Thompson", Nickname: "Zack"},
		// FullName carries the prefix the SQL full_name column leaves out.
		{PersonID: ids["Edmund"], GivenName: "Edmund", Surname: "Harrow", FullName: "Dr. Edmund Harrow"},
	} {
		n.ID = uuid.New()
		n.UpdatedAt = time.Now()
		if err := store.SavePersonName(ctx, domain.MainBranchID, &n); err != nil {
			t.Fatalf("save name %s: %v", n.Surname, err)
		}
	}
	return store, ids
}

func searchNames(t *testing.T, store *memory.ReadModelStore, ids map[string]uuid.UUID, opts repository.SearchOptions) []string {
	t.Helper()
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	results, err := store.SearchPersons(context.Background(), opts)
	if err != nil {
		t.Fatalf("SearchPersons(%+v): %v", opts, err)
	}
	var got []string
	for _, r := range results {
		for given, id := range ids {
			if id == r.ID {
				got = append(got, given)
			}
		}
	}
	return got
}

// TestSearchPersons_MatchesSQLSemantics checks the memory store matches names
// the way the SQL stores do (DB-005): ILIKE-style substrings, pg_trgm fuzzy
// similarity, Soundex, alternate names and nicknames, and place filters.
func TestSearchPersons_MatchesSQLSemantics(t *testing.T) {
	store, ids := searchFixture(t)
	tests := []struct {
		name string
		opts repository.SearchOptions
		want []string
	}{
		{"plain substring", repository.SearchOptions{Query: "mith"}, []string{"John"}},
		{"plain is trimmed", repository.SearchOptions{Query: "  smith  "}, []string{"John"}},
		{"plain alternate surname", repository.SearchOptions{Query: "Harrow"}, []string{"Edmund"}},
		{"plain nickname", repository.SearchOptions{Query: "zack"}, []string{"Zachary"}},
		{"plain ignores alternate-name prefix", repository.SearchOptions{Query: "Dr."}, nil},
		{"plain wildcard", repository.SearchOptions{Query: "Sm_th"}, []string{"John"}},
		{"plain no match", repository.SearchOptions{Query: "Smyth"}, nil},
		{"fuzzy person", repository.SearchOptions{Query: "Smyth", Fuzzy: true}, []string{"John"}},
		{"fuzzy alternate surname", repository.SearchOptions{Query: "Harow", Fuzzy: true}, []string{"Edmund"}},
		{"fuzzy nickname", repository.SearchOptions{Query: "Zackk", Fuzzy: true}, []string{"Zachary"}},
		{"fuzzy below threshold", repository.SearchOptions{Query: "Jon", Fuzzy: true}, nil},
		{"fuzzy wins over soundex", repository.SearchOptions{Query: "Smyth", Fuzzy: true, Soundex: true}, []string{"John"}},
		{"soundex person", repository.SearchOptions{Query: "Smyth", Soundex: true}, []string{"John"}},
		{"soundex alternate surname", repository.SearchOptions{Query: "Harrau", Soundex: true}, []string{"Edmund"}},
		{"soundex nickname", repository.SearchOptions{Query: "Zak", Soundex: true}, []string{"Zachary"}},
		{"place folds case", repository.SearchOptions{BirthPlace: " münchen "}, []string{"Zachary"}},
		{"place with query", repository.SearchOptions{Query: "Smith", BirthPlace: "boston"}, nil},
		{"death place", repository.SearchOptions{DeathPlace: "nowhere"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := searchNames(t, store, ids, tt.opts)
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

// TestSearchPersons_Sort checks the name, birth and death sorts in both
// directions; people without a date sort last either way.
func TestSearchPersons_Sort(t *testing.T) {
	store, ids := searchFixture(t)
	all := repository.SearchOptions{Query: "o"} // every fixture name contains an "o"
	tests := []struct {
		sort, order string
		want        []string
	}{
		{"name", "asc", []string{"Edmund", "John", "Zachary"}},
		{"name", "desc", []string{"Zachary", "John", "Edmund"}},
		{"birth_date", "asc", []string{"John", "Zachary", "Edmund"}},
		{"birth_date", "desc", []string{"Zachary", "John", "Edmund"}},
		{"death_date", "asc", []string{"Edmund", "John", "Zachary"}},
		{"death_date", "desc", []string{"John", "Edmund", "Zachary"}},
	}
	for _, tt := range tests {
		t.Run(tt.sort+"/"+tt.order, func(t *testing.T) {
			opts := all
			opts.Sort, opts.Order = tt.sort, tt.order
			if got := searchNames(t, store, ids, opts); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
