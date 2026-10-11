package sqlite_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestSearchPersonsTotal_Parity runs the #902 search-total scenario against the
// sqlite backend; the other backend packages run the same body (DB-001).
func TestSearchPersonsTotal_Parity(t *testing.T) {
	readStore, _, cleanup := setupBranchScenarioStores(t)
	defer cleanup()
	runSearchTotalScenario(t, readStore)
}

// TestSearchPersonsSoundex_TotalBeyondThousandCandidates guards against a
// candidate cap: Soundex matches stored after the first 1000 people must still
// be counted and reachable.
func TestSearchPersonsSoundex_TotalBeyondThousandCandidates(t *testing.T) {
	store, _, cleanup := setupBranchScenarioStores(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	save := func(given, surname string) {
		t.Helper()
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: uuid.New(), GivenName: given, Surname: surname, FullName: given + " " + surname,
			Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}
	for i := range 1000 {
		save(fmt.Sprintf("Filler%04d", i), "Zzyzx")
	}
	const matches = 3
	for i := range matches {
		save(fmt.Sprintf("Late%d", i), "Pagetest")
	}

	page, total, err := store.SearchPersons(ctx, repository.SearchOptions{Query: "Pagetest", Soundex: true, Limit: 10})
	if err != nil {
		t.Fatalf("SearchPersons: %v", err)
	}
	if total != matches || len(page) != matches {
		t.Fatalf("total = %d, page = %d, want %d and %d", total, len(page), matches, matches)
	}
}

// runSearchTotalScenario is the backend-agnostic body of the #902 search
// total. Each backend package carries an identical copy (DB-001). It proves
// that, for every match mode and sort, SearchPersons reports the full match
// count whatever the limit and offset, and that paging by offset reaches every
// match exactly once. Fixtures use neutral placeholder names only.
func runSearchTotalScenario(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// 25 people match by their own name and live in Testville; birth years
	// repeat so date sorts have ties to break.
	const direct, viaAltName = 25, 2
	for i := range direct {
		born := time.Date(1900+i%5, 1, 1, 0, 0, 0, 0, time.UTC)
		given := fmt.Sprintf("Person%02d", i)
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: uuid.New(), GivenName: given, Surname: "Pagetest", FullName: given + " Pagetest",
			BirthDateRaw: born.Format("2006"), BirthDateSort: &born, BirthPlace: "Testville",
			Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}
	// Two more match only through an alternate name.
	for i := range viaAltName {
		id := uuid.New()
		given := fmt.Sprintf("Alias%d", i)
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: id, GivenName: given, Surname: "Other", FullName: given + " Other", Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save person: %v", err)
		}
		if err := store.SavePersonName(ctx, domain.MainBranchID, &repository.PersonNameReadModel{
			ID: uuid.New(), PersonID: id, GivenName: given, Surname: "Pagetest", FullName: given + " Pagetest",
			NameType: domain.NameTypeAKA, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save person name: %v", err)
		}
	}
	// And one matches nothing.
	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Unrelated", Surname: "Nobody", FullName: "Unrelated Nobody", Version: 1, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save person: %v", err)
	}

	modes := []struct {
		name string
		opts repository.SearchOptions
		want int
	}{
		{"substring", repository.SearchOptions{Query: "Pagetest"}, direct + viaAltName},
		{"fuzzy", repository.SearchOptions{Query: "Pagetest", Fuzzy: true}, direct + viaAltName},
		{"soundex", repository.SearchOptions{Query: "Pagetest", Soundex: true}, direct + viaAltName},
		{"filters only", repository.SearchOptions{BirthPlace: "Testville"}, direct},
	}
	sorts := []struct{ sort, order string }{
		{"relevance", "desc"},
		{"name", "asc"},
		{"birth_date", "desc"},
	}
	const pageSize = 10
	for _, mode := range modes {
		for _, s := range sorts {
			t.Run(mode.name+"/"+s.sort, func(t *testing.T) {
				seen := make(map[uuid.UUID]bool)
				for offset := 0; offset < mode.want+pageSize; offset += pageSize {
					opts := mode.opts
					opts.Sort, opts.Order = s.sort, s.order
					opts.Limit, opts.Offset = pageSize, offset
					page, total, err := store.SearchPersons(ctx, opts)
					if err != nil {
						t.Fatalf("SearchPersons(offset %d): %v", offset, err)
					}
					if total != mode.want {
						t.Fatalf("offset %d: total = %d, want %d", offset, total, mode.want)
					}
					if want := min(pageSize, max(mode.want-offset, 0)); len(page) != want {
						t.Fatalf("offset %d: page has %d people, want %d", offset, len(page), want)
					}
					for _, p := range page {
						if seen[p.ID] {
							t.Fatalf("offset %d: %s %s appears on two pages", offset, p.GivenName, p.Surname)
						}
						seen[p.ID] = true
					}
				}
				if len(seen) != mode.want {
					t.Errorf("pages reached %d people, want %d", len(seen), mode.want)
				}
			})
		}
	}
}
