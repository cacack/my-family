package repository_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// legacyMixedCalendarRange is "BET 1740 AND @#DHEBREW@ 5500" as ParseGenDate
// stored it before #1227: the end's Hebrew escape taken as the whole date's
// calendar and no end date, so its start sorted as Hebrew 1740 (about 2021 BC).
func legacyMixedCalendarRange() *domain.GenDate {
	year := 1740
	return &domain.GenDate{Raw: "BET 1740 AND @#DHEBREW@ 5500", Qualifier: domain.DateBet, Year: &year, Calendar: domain.CalendarHebrew}
}

// TestProjector_ReplayRereadsLegacyMixedCalendarRange proves replaying an
// event stored with the legacy reading of a mixed-calendar range projects it
// with its Gregorian start on every backend, without rewriting the event
// (#1239).
func TestProjector_ReplayRereadsLegacyMixedCalendarRange(t *testing.T) {
	for backend, readStore := range fullNameStores(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			projector := repository.NewProjector(readStore, nil)

			person := domain.NewPerson("Miriam", "Levi")
			person.BirthDate = legacyMixedCalendarRange()
			death := domain.ParseGenDate("1 JAN 1800")
			person.DeathDate = &death
			stored, err := repository.EncodeEvent(person.ID, "Person", domain.NewPersonCreated(person), 1, 1)
			if err != nil {
				t.Fatalf("EncodeEvent: %v", err)
			}
			decoded, err := stored.DecodeEvent()
			if err != nil {
				t.Fatalf("DecodeEvent: %v", err)
			}
			created := decoded.(domain.PersonCreated)
			if bd := created.BirthDate; bd.Calendar != domain.CalendarGregorian || bd.Calendar2 != domain.CalendarHebrew ||
				bd.Year == nil || *bd.Year != 1740 || bd.Year2 == nil || *bd.Year2 != 5500 {
				t.Errorf("decoded BirthDate = %+v, want Gregorian 1740 to Hebrew 5500", bd)
			}
			if err := projector.Project(ctx, decoded, 1, domain.MainBranchID); err != nil {
				t.Fatalf("Project: %v", err)
			}

			rm, err := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
			if err != nil || rm == nil {
				t.Fatalf("GetPerson: %v, %v", rm, err)
			}
			if rm.BirthDateSort == nil || rm.BirthDateSort.Year() != 1740 {
				t.Errorf("BirthDateSort = %v, want 1740", rm.BirthDateSort)
			}
			if rm.BirthDateRaw != "BET 1740 AND @#DHEBREW@ 5500" {
				t.Errorf("BirthDateRaw = %q, want the stored text", rm.BirthDateRaw)
			}
			if rm.DeathDateSort == nil || rm.DeathDateSort.Year() != 1800 {
				t.Errorf("DeathDateSort = %v, want 1800", rm.DeathDateSort)
			}
		})
	}
}
