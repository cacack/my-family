package repository_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// legacyMixedCalendarRange is one mixed-calendar range exactly as ParseGenDate
// stored it before #1227, with the reading replay must project.
type legacyMixedCalendarRange struct {
	stored    domain.GenDate
	cal, cal2 string
	year      int
	year2     int
}

// legacyMixedCalendarRanges returns the two stored shapes: an unescaped start
// took the end's Hebrew calendar for the whole date (so it sorted as Hebrew
// 1740, about 2021 BC), and an escaped Julian start lost its Gregorian end.
func legacyMixedCalendarRanges() map[string]legacyMixedCalendarRange {
	y1740, y5500, y1690 := 1740, 5500, 1690
	return map[string]legacyMixedCalendarRange{
		"gregorian-to-hebrew": {
			stored: domain.GenDate{Raw: "BET 1740 AND @#DHEBREW@ 5500", Qualifier: domain.DateBet,
				Year: &y1740, Year2: &y5500, Calendar: domain.CalendarHebrew},
			cal: domain.CalendarGregorian, cal2: domain.CalendarHebrew, year: 1740, year2: 5500,
		},
		"julian-to-gregorian": {
			stored: domain.GenDate{Raw: "BET @#DJULIAN@ 1690 AND @#DGREGORIAN@ 1700", Qualifier: domain.DateBet,
				Year: &y1690, Calendar: domain.CalendarJulian},
			cal: domain.CalendarJulian, cal2: domain.CalendarGregorian, year: 1690, year2: 1700,
		},
	}
}

// TestProjector_ReplayRereadsLegacyMixedCalendarRange proves replaying an
// event stored with the legacy reading of a mixed-calendar range projects it
// with its real start on every backend, without rewriting the event (#1239).
func TestProjector_ReplayRereadsLegacyMixedCalendarRange(t *testing.T) {
	for name, tc := range legacyMixedCalendarRanges() {
		for backend, open := range fullNameStores() {
			t.Run(name+"/"+backend, func(t *testing.T) {
				readStore := open(t)
				ctx := context.Background()
				projector := repository.NewProjector(readStore, nil)

				person := domain.NewPerson("Miriam", "Levi")
				birth := tc.stored
				person.BirthDate = &birth
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
				if bd := created.BirthDate; bd.Calendar != tc.cal || bd.Calendar2 != tc.cal2 ||
					bd.Year == nil || *bd.Year != tc.year || bd.Year2 == nil || *bd.Year2 != tc.year2 {
					t.Errorf("decoded BirthDate = %+v, want %s %d to %s %d", bd, tc.cal, tc.year, tc.cal2, tc.year2)
				}
				if err := projector.Project(ctx, decoded, 1, domain.MainBranchID); err != nil {
					t.Fatalf("Project: %v", err)
				}

				rm, err := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
				if err != nil || rm == nil {
					t.Fatalf("GetPerson: %v, %v", rm, err)
				}
				if rm.BirthDateSort == nil || rm.BirthDateSort.Year() != tc.year {
					t.Errorf("BirthDateSort = %v, want %d", rm.BirthDateSort, tc.year)
				}
				if rm.BirthDateRaw != tc.stored.Raw {
					t.Errorf("BirthDateRaw = %q, want the stored text %q", rm.BirthDateRaw, tc.stored.Raw)
				}
				if rm.DeathDateSort == nil || rm.DeathDateSort.Year() != 1800 {
					t.Errorf("DeathDateSort = %v, want 1800", rm.DeathDateSort)
				}
			})
		}
	}
}
