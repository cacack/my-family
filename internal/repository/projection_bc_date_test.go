package repository_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestProjector_BCDateKeepsSortDate proves a BC birth date is stored with its
// BC sort date on every backend and reads back unchanged, alongside the raw
// date text (#1065).
func TestProjector_BCDateKeepsSortDate(t *testing.T) {
	for backend, readStore := range fullNameStores(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			projector := repository.NewProjector(readStore, nil)

			person := domain.NewPerson("Gaius", "Julius")
			bc := domain.ParseGenDate("@#DJULIAN@ 15 MAR 44 BC")
			person.BirthDate = &bc
			ad := domain.ParseGenDate("1 JAN 1850")
			person.DeathDate = &ad
			if err := projector.Project(ctx, domain.NewPersonCreated(person), 1, domain.MainBranchID); err != nil {
				t.Fatalf("Project: %v", err)
			}

			rm, err := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
			if err != nil || rm == nil {
				t.Fatalf("GetPerson: %v, %v", rm, err)
			}
			if want := bc.SortDate(); rm.BirthDateSort == nil || !rm.BirthDateSort.Equal(want) {
				t.Errorf("BirthDateSort = %v, want %v", rm.BirthDateSort, want)
			}
			if rm.BirthDateRaw != bc.Raw {
				t.Errorf("BirthDateRaw = %q, want %q", rm.BirthDateRaw, bc.Raw)
			}
			if rm.DeathDateSort == nil || rm.DeathDateSort.Year() != 1850 {
				t.Errorf("DeathDateSort = %v, want 1850", rm.DeathDateSort)
			}
		})
	}
}
