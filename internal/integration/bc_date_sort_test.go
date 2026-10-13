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

// TestBCDateSort proves every backend stores a sortable sort key for BC dates
// (#1065): people born BC list before people born AD, in order, a birth-date
// range reaching back before 1 AD finds them, undated people stay apart from
// them, and every BC sort column reads back as the date that was projected.
func TestBCDateSort(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			read := backend.setup(t).read
			projector := repository.NewProjector(read, nil)
			project := func(event domain.Event) {
				t.Helper()
				if err := projector.Project(ctx, event, 1, domain.MainBranchID); err != nil {
					t.Fatalf("project %s: %v", event.EventType(), err)
				}
			}

			// 5000 BC is before PostgreSQL's earliest DATE, so it sorts as that date.
			born := []string{"5000 BC", "1 JAN 1850 BC", "44 BC", "10", "1750"} // ascending
			ids := map[uuid.UUID]string{}
			var bc44 *domain.Person
			for _, date := range append(slices.Clone(born), "", "") { // two undated
				p := domain.NewPerson("Given", date)
				if date != "" {
					gd := domain.ParseGenDate(date)
					p.BirthDate = &gd
				}
				if date == "44 BC" {
					bc44 = p
				}
				ids[p.ID] = date
				project(domain.NewPersonCreated(p))
			}

			t.Run("list sorted by birth date", func(t *testing.T) {
				people, _, err := read.ListPersons(ctx, repository.ListOptions{Sort: "birth_date", Order: "asc", Limit: 50})
				if err != nil {
					t.Fatalf("ListPersons: %v", err)
				}
				assertBornOrder(t, people, ids, born)
			})

			t.Run("search sorted by birth date", func(t *testing.T) {
				// A search needs a query or a filter; this range spans every dated person.
				from := domain.EarliestSortDate
				to := time.Date(3000, time.January, 1, 0, 0, 0, 0, time.UTC)
				people, _, err := read.SearchPersons(ctx, repository.SearchOptions{
					BirthDateFrom: &from, BirthDateTo: &to, Sort: "birth_date", Order: "asc", Limit: 50,
				})
				if err != nil {
					t.Fatalf("SearchPersons: %v", err)
				}
				if got := bornDates(people, ids); !slices.Equal(got, born) {
					t.Errorf("search order %q, want %q", got, born)
				}
			})

			t.Run("birth date range before 1 AD", func(t *testing.T) {
				from := time.Date(-100, time.January, 1, 0, 0, 0, 0, time.UTC)
				to := time.Date(5, time.January, 1, 0, 0, 0, 0, time.UTC)
				people, _, err := read.SearchPersons(ctx, repository.SearchOptions{BirthDateFrom: &from, BirthDateTo: &to, Limit: 50})
				if err != nil {
					t.Fatalf("SearchPersons: %v", err)
				}
				if len(people) != 1 || people[0].ID != bc44.ID {
					t.Errorf("range %v..%v found %v, want only the person born 44 BC", from, to, bornDates(people, ids))
				}
			})

			t.Run("BC sort values read back", func(t *testing.T) {
				assertBCSortDatesRoundTrip(t, ctx, read, project)
			})
		})
	}
}

// assertBornOrder checks the dated people come in born order as one block, with
// the undated people all before or all after it (never interleaved).
func assertBornOrder(t *testing.T, people []repository.PersonReadModel, ids map[uuid.UUID]string, born []string) {
	t.Helper()
	got := bornDates(people, ids)
	dated := slices.DeleteFunc(slices.Clone(got), func(d string) bool { return d == "" })
	if !slices.Equal(dated, born) {
		t.Errorf("dated people in order %q, want %q", dated, born)
	}
	first := slices.Index(got, born[0])
	if first < 0 || !slices.Equal(got[first:first+len(born)], born) {
		t.Errorf("undated people interleave with dated ones: %q", got)
	}
}

// bornDates maps people to the birth date text they were created with.
func bornDates(people []repository.PersonReadModel, ids map[uuid.UUID]string) []string {
	dates := make([]string, 0, len(people))
	for _, p := range people {
		dates = append(dates, ids[p.ID])
	}
	return dates
}

// assertBCSortDatesRoundTrip projects a BC date into every *_date_sort column
// kind (the date-only and RFC 3339 SQLite columns alike) and reads each back.
func assertBCSortDatesRoundTrip(t *testing.T, ctx context.Context, read repository.ReadModelStore,
	project func(domain.Event),
) {
	t.Helper()
	bc := domain.ParseGenDate("@#DJULIAN@ 15 MAR 44 BC")
	want := bc.SortDate()
	check := func(name string, got *time.Time, err error) {
		t.Helper()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		if got == nil || !got.Equal(want) {
			t.Errorf("%s sort date = %v, want %v", name, got, want)
		}
	}

	person := domain.NewPerson("Gaius", "Julius")
	person.DeathDate = &bc
	project(domain.NewPersonCreated(person))
	rm, err := read.GetPerson(ctx, domain.MainBranchID, person.ID)
	check("person death", ptrField(rm, err, func(p *repository.PersonReadModel) *time.Time { return p.DeathDateSort }), err)

	family := domain.NewFamily()
	family.Partner1ID = &person.ID
	family.MarriageDate = &bc
	project(domain.NewFamilyCreated(family))
	fam, err := read.GetFamily(ctx, domain.MainBranchID, family.ID)
	check("family marriage", ptrField(fam, err, func(f *repository.FamilyReadModel) *time.Time { return f.MarriageDateSort }), err)

	source := domain.NewSource("Commentarii", domain.SourceBook)
	source.PublishDate = &bc
	project(domain.NewSourceCreated(source))
	src, err := read.GetSource(ctx, domain.MainBranchID, source.ID)
	check("source publish", ptrField(src, err, func(s *repository.SourceReadModel) *time.Time { return s.PublishDateSort }), err)

	event := domain.NewLifeEvent(person.ID, domain.FactPersonBurial)
	event.Date = &bc
	project(domain.NewLifeEventCreatedFromModel(event))
	ev, err := read.GetEvent(ctx, domain.MainBranchID, event.ID)
	check("life event", ptrField(ev, err, func(e *repository.EventReadModel) *time.Time { return e.DateSort }), err)

	attr := domain.NewAttribute(person.ID, domain.FactPersonOccupation, "Consul")
	attr.Date = &bc
	project(domain.NewAttributeCreatedFromModel(attr))
	at, err := read.GetAttribute(ctx, domain.MainBranchID, attr.ID)
	check("attribute", ptrField(at, err, func(a *repository.AttributeReadModel) *time.Time { return a.DateSort }), err)

	ord := domain.NewLDSOrdinance(domain.LDSBaptism)
	ord.PersonID = &person.ID
	ord.Date = &bc
	project(domain.NewLDSOrdinanceCreated(ord))
	lds, err := read.GetLDSOrdinance(ctx, ord.ID)
	check("LDS ordinance", ptrField(lds, err, func(o *repository.LDSOrdinanceReadModel) *time.Time { return o.DateSort }), err)
}

// ptrField reads a field from a row that may be missing.
func ptrField[T any](row *T, err error, field func(*T) *time.Time) *time.Time {
	if err != nil || row == nil {
		return nil
	}
	return field(row)
}
