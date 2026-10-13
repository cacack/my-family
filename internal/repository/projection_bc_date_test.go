package repository_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestProjector_BCDateHasNoSortDate proves a BC birth date is stored with no
// sort date on every backend, so SQLite (whose text sort column cannot hold a
// negative year) and memory agree, while the raw date text is kept.
func TestProjector_BCDateHasNoSortDate(t *testing.T) {
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
			if rm.BirthDateSort != nil {
				t.Errorf("BirthDateSort = %v, want nil for a BC date", rm.BirthDateSort)
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
