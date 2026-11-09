package postgres_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestReplayRereadsLegacyMixedCalendarRange proves a PersonCreated stored with
// the pre-#1227 reading of "BET 1740 AND @#DHEBREW@ 5500" (Hebrew calendar,
// no end date) replays into the PostgreSQL read model sorted in 1740 (#1239).
func TestReplayRereadsLegacyMixedCalendarRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	readStore, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, nil)

	year := 1740
	person := domain.NewPerson("Miriam", "Levi")
	person.BirthDate = &domain.GenDate{Raw: "BET 1740 AND @#DHEBREW@ 5500", Qualifier: domain.DateBet, Year: &year, Calendar: domain.CalendarHebrew}
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
	if rm.BirthDateSort == nil || rm.BirthDateSort.Year() != 1740 {
		t.Errorf("BirthDateSort = %v, want 1740", rm.BirthDateSort)
	}
}
