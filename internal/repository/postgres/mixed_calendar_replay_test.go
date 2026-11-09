package postgres_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestReplayRereadsLegacyMixedCalendarRange proves a PersonCreated stored with
// a pre-#1227 reading of a mixed-calendar range (exactly as ParseGenDate stored
// it, without Calendar2) replays into the PostgreSQL read model sorted by its
// real start (#1239).
func TestReplayRereadsLegacyMixedCalendarRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	readStore, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, nil)

	y1740, y5500, y1690 := 1740, 5500, 1690
	for _, tc := range []struct {
		stored   domain.GenDate
		wantYear int
	}{
		{domain.GenDate{Raw: "BET 1740 AND @#DHEBREW@ 5500", Qualifier: domain.DateBet, Year: &y1740, Year2: &y5500, Calendar: domain.CalendarHebrew}, 1740},
		{domain.GenDate{Raw: "BET @#DJULIAN@ 1690 AND @#DGREGORIAN@ 1700", Qualifier: domain.DateBet, Year: &y1690, Calendar: domain.CalendarJulian}, 1690},
	} {
		person := domain.NewPerson("Miriam", "Levi")
		birth := tc.stored
		person.BirthDate = &birth
		stored, err := repository.EncodeEvent(person.ID, "Person", domain.NewPersonCreated(person), 1, 1)
		if err != nil {
			t.Fatalf("EncodeEvent: %v", err)
		}
		decoded, err := stored.DecodeEvent()
		if err != nil {
			t.Fatalf("DecodeEvent: %v", err)
		}
		if err := projector.Project(ctx, decoded, 1, domain.MainBranchID); err != nil {
			t.Fatalf("Project: %v", err)
		}

		rm, err := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
		if err != nil || rm == nil {
			t.Fatalf("GetPerson: %v, %v", rm, err)
		}
		if rm.BirthDateSort == nil || rm.BirthDateSort.Year() != tc.wantYear {
			t.Errorf("%s: BirthDateSort = %v, want %d", tc.stored.Raw, rm.BirthDateSort, tc.wantYear)
		}
	}
}
