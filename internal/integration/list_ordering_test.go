package integration_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestPagedListsVisitEveryRowOnce proves every paged list ends its order on id
// (#1176): rows whose sort keys all tie still page through ListAll exactly once
// each, on every backend.
func TestPagedListsVisitEveryRowOnce(t *testing.T) {
	const rows, pageSize = 30, 7
	stamp := time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC)

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			read := backend.setup(t).read
			main := domain.MainBranchID

			want := map[string][]uuid.UUID{}
			for range rows {
				// Identical names, places, dates and timestamps: only the id differs.
				p := &repository.PersonReadModel{
					ID: uuid.New(), GivenName: "Paul", Surname: "von_Hugel", FullName: "Paul von_Hugel",
					BirthPlace: "Vienna", Version: 1, UpdatedAt: stamp,
				}
				if err := read.SavePerson(ctx, main, p); err != nil {
					t.Fatalf("SavePerson: %v", err)
				}
				want["persons"] = append(want["persons"], p.ID)

				// Two burials each, so the DISTINCT join has duplicates to fold.
				for range 2 {
					burial := &repository.EventReadModel{
						ID: uuid.New(), OwnerType: "person", OwnerID: p.ID, FactType: domain.FactPersonBurial,
						Place: "Zentralfriedhof", Version: 1, CreatedAt: stamp,
					}
					if err := read.SaveEvent(ctx, main, burial); err != nil {
						t.Fatalf("SaveEvent: %v", err)
					}
				}

				f := &repository.FamilyReadModel{ID: uuid.New(), Version: 1, UpdatedAt: stamp}
				if err := read.SaveFamily(ctx, main, f); err != nil {
					t.Fatalf("SaveFamily: %v", err)
				}
				want["families"] = append(want["families"], f.ID)

				s := &repository.SubmitterReadModel{ID: uuid.New(), Name: "Same", Version: 1, UpdatedAt: stamp}
				if err := read.SaveSubmitter(ctx, s); err != nil {
					t.Fatalf("SaveSubmitter: %v", err)
				}
				want["submitters"] = append(want["submitters"], s.ID)

				o := &repository.LDSOrdinanceReadModel{
					ID: uuid.New(), Type: domain.LDSBaptism, TypeLabel: "Baptism (LDS)", Version: 1, UpdatedAt: stamp,
				}
				if err := read.SaveLDSOrdinance(ctx, o); err != nil {
					t.Fatalf("SaveLDSOrdinance: %v", err)
				}
				want["lds"] = append(want["lds"], o.ID)
			}

			personsBy := func(sort, order string) func(context.Context, repository.ListOptions) ([]repository.PersonReadModel, int, error) {
				return func(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
					opts.Sort, opts.Order = sort, order
					return read.ListPersons(ctx, opts)
				}
			}
			personLists := map[string]func(context.Context, repository.ListOptions) ([]repository.PersonReadModel, int, error){
				"ListPersons":              read.ListPersons,
				"ListPersons surname desc": personsBy("surname", "desc"),
				"ListPersons given desc":   personsBy("given_name", "desc"),
				"ListPersons birth asc":    personsBy("birth_date", "asc"),
				"ListPersons updated":      personsBy("updated_at", "desc"),
				"GetPersonsBySurname": func(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
					return read.GetPersonsBySurname(ctx, "von_Hugel", opts)
				},
				"GetPersonsByPlace": func(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
					return read.GetPersonsByPlace(ctx, "Vienna", opts)
				},
				"GetPersonsByCemetery": func(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
					return read.GetPersonsByCemetery(ctx, "Zentralfriedhof", opts)
				},
			}
			for name, list := range personLists {
				all, err := repository.ListAll(ctx, pageSize, list)
				assertEachOnce(t, name, err, ids(all, func(p repository.PersonReadModel) uuid.UUID { return p.ID }), want["persons"])
			}

			families, err := repository.ListAll(ctx, pageSize, read.ListFamilies)
			assertEachOnce(t, "ListFamilies", err, ids(families, func(f repository.FamilyReadModel) uuid.UUID { return f.ID }), want["families"])

			subs, err := repository.ListAll(ctx, pageSize, read.ListSubmitters)
			assertEachOnce(t, "ListSubmitters", err, ids(subs, func(s repository.SubmitterReadModel) uuid.UUID { return s.ID }), want["submitters"])

			lds, err := repository.ListAll(ctx, pageSize, read.ListLDSOrdinances)
			assertEachOnce(t, "ListLDSOrdinances", err, ids(lds, func(o repository.LDSOrdinanceReadModel) uuid.UUID { return o.ID }), want["lds"])
		})
	}
}

// TestUndatedAndYearOneBirthSort proves every backend sorts undated people last
// in both directions and gives 1 JAN 1 AD a real sort value (#1078).
func TestUndatedAndYearOneBirthSort(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			read := backend.setup(t).read
			projector := repository.NewProjector(read, nil)

			ids := map[uuid.UUID]string{}
			var yearOne uuid.UUID
			for _, date := range []string{"", "2", "1 JAN 1", "", "1 BC"} {
				p := domain.NewPerson("Given", "Same")
				if date != "" {
					gd := domain.ParseGenDate(date)
					p.BirthDate = &gd
					dd := gd
					p.DeathDate = &dd
				}
				if date == "1 JAN 1" {
					yearOne = p.ID
				}
				ids[p.ID] = date
				if err := projector.Project(ctx, domain.NewPersonCreated(p), 1, domain.MainBranchID); err != nil {
					t.Fatalf("project: %v", err)
				}
			}
			wantAsc := []string{"1 BC", "1 JAN 1", "2", "", ""}
			wantDesc := []string{"2", "1 JAN 1", "1 BC", "", ""}

			rm, err := read.GetPerson(ctx, domain.MainBranchID, yearOne)
			if err != nil || rm == nil || rm.BirthDateSort == nil {
				t.Fatalf("1 JAN 1 birth sort value = %v (err %v), want non-nil", rm, err)
			}

			// Soundex and fuzzy search sort in Go on SQLite and in SQL on
			// PostgreSQL; every path must agree.
			searches := map[string]repository.SearchOptions{
				"SearchPersons":         {Query: "Same"},
				"SearchPersons soundex": {Query: "Same", Soundex: true},
				"SearchPersons fuzzy":   {Query: "Same", Fuzzy: true},
			}
			for order, want := range map[string][]string{"asc": wantAsc, "desc": wantDesc} {
				people, _, err := read.ListPersons(ctx, repository.ListOptions{Sort: "birth_date", Order: order, Limit: 50})
				if err != nil {
					t.Fatalf("ListPersons: %v", err)
				}
				if got := bornDates(people, ids); !slices.Equal(got, want) {
					t.Errorf("ListPersons %s order %q, want %q", order, got, want)
				}

				// Search alone sorts by death date too.
				for _, sortBy := range []string{"birth_date", "death_date"} {
					for name, opts := range searches {
						opts.Sort, opts.Order, opts.Limit = sortBy, order, 50
						people, _, err := read.SearchPersons(ctx, opts)
						if err != nil {
							t.Fatalf("%s: %v", name, err)
						}
						if got := bornDates(people, ids); !slices.Equal(got, want) {
							t.Errorf("%s %s %s order %q, want %q", name, sortBy, order, got, want)
						}
					}
				}
			}

			from := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
			to := time.Date(1, time.December, 31, 0, 0, 0, 0, time.UTC)
			people, _, err := read.SearchPersons(ctx, repository.SearchOptions{BirthDateFrom: &from, BirthDateTo: &to, Limit: 50})
			if err != nil {
				t.Fatalf("SearchPersons range: %v", err)
			}
			if got := bornDates(people, ids); !slices.Equal(got, []string{"1 JAN 1"}) {
				t.Errorf("birth range over year 1 found %q, want only 1 JAN 1", got)
			}
		})
	}
}

// ids maps rows to their ids.
func ids[T any](rows []T, id func(T) uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, id(r))
	}
	return out
}

// assertEachOnce checks got holds every id of want exactly once.
func assertEachOnce(t *testing.T, name string, err error, got, want []uuid.UUID) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return
	}
	cmp := func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) }
	got, want = slices.Clone(got), slices.Clone(want)
	slices.SortFunc(got, cmp)
	slices.SortFunc(want, cmp)
	if !slices.Equal(got, want) {
		t.Errorf("%s returned %d rows (%d distinct), want each of %d exactly once",
			name, len(got), len(slices.Compact(slices.Clone(got))), len(want))
	}
}
