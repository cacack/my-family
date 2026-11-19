package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

func TestSetLivingFacts(t *testing.T) {
	endOf := func(y int) *time.Time {
		d := time.Date(y, time.December, 31, 0, 0, 0, 0, time.UTC)
		return &d
	}
	event := func(fact domain.FactType, date string, negated bool) repository.EventReadModel {
		return repository.EventReadModel{FactType: fact, DateRaw: date, IsNegated: negated}
	}
	tests := []struct {
		name      string
		person    repository.PersonReadModel
		events    []repository.EventReadModel
		wantDeath bool
		wantRef   *time.Time
	}{
		{"nothing", repository.PersonReadModel{}, nil, false, nil},
		{"death date", repository.PersonReadModel{DeathDateRaw: "1950"}, nil, true, nil},
		{"death place only", repository.PersonReadModel{DeathPlace: "Boston"}, nil, true, nil},
		{"death notes only", repository.PersonReadModel{DeathNotes: "died young"}, nil, true, nil},
		{"death type only", repository.PersonReadModel{DeathTypeDetail: "Stillborn"}, nil, true, nil},
		{"burial", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonBurial, "", false)}, true, nil},
		{"cremation", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonCremation, "", false)}, true, nil},
		{"death event", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonDeath, "", false)}, true, nil},
		{"negated burial is no evidence", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonBurial, "", true)}, false, nil},
		{"other event is no evidence", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonCensus, "1900", false)}, false, nil},
		{"birth", repository.PersonReadModel{BirthDateRaw: "1900"}, []repository.EventReadModel{event(domain.FactPersonBaptism, "1950", false)}, false, endOf(1900)},
		{"baptism fallback", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonBaptism, "1901", false)}, false, endOf(1901)},
		{"christening fallback", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonChristening, "1902", false)}, false, endOf(1902)},
		{"negated christening ignored", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonChristening, "1902", true)}, false, nil},
		{"undated baptism ignored", repository.PersonReadModel{}, []repository.EventReadModel{event(domain.FactPersonBaptism, "", false)}, false, nil},
		{"ancient birth is clamped", repository.PersonReadModel{BirthDateRaw: "9000 BC"}, nil, false, &domain.EarliestSortDate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.person
			p.DeathEvidence = !tt.wantDeath // overwritten
			repository.SetLivingFacts(&p, tt.events)
			if p.DeathEvidence != tt.wantDeath {
				t.Errorf("DeathEvidence = %v, want %v", p.DeathEvidence, tt.wantDeath)
			}
			if (p.LivingRefDate == nil) != (tt.wantRef == nil) || (p.LivingRefDate != nil && !p.LivingRefDate.Equal(*tt.wantRef)) {
				t.Errorf("LivingRefDate = %v, want %v", p.LivingRefDate, tt.wantRef)
			}
		})
	}
}

// TestProjector_LifeEventsDriveTheLivingRule proves a person's living-rule
// inputs follow its burials and baptisms as they are added, changed and
// removed, and follow an override and a restriction set by PersonUpdated, on
// every backend.
func TestProjector_LifeEventsDriveTheLivingRule(t *testing.T) {
	for backend, open := range fullNameStores() {
		t.Run(backend, func(t *testing.T) {
			readStore := open(t)
			ctx := context.Background()
			projector := repository.NewProjector(readStore, nil)
			project := func(e domain.Event, version int64) {
				t.Helper()
				if err := projector.Project(ctx, e, version, domain.MainBranchID); err != nil {
					t.Fatalf("Project %s: %v", e.EventType(), err)
				}
			}
			person := domain.NewPerson("Ada", "Lovelace")
			project(domain.NewPersonCreated(person), 1)
			facts := func() domain.LivingFacts {
				t.Helper()
				p, err := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
				if err != nil || p == nil {
					t.Fatalf("GetPerson: %v, %v", p, err)
				}
				return p.LivingFacts()
			}
			if f := facts(); f.DeathEvidence || f.ReferenceDate != nil {
				t.Fatalf("new person facts = %+v, want none", f)
			}

			baptismDate := domain.ParseGenDate("1815")
			baptism := domain.NewLifeEventCreatedFromModel(&domain.LifeEvent{ID: uuid.New(), PersonID: &person.ID,
				FactType: domain.FactPersonBaptism, Date: &baptismDate})
			project(baptism, 1)
			if f := facts(); f.ReferenceDate == nil || f.ReferenceDate.Year() != 1815 {
				t.Errorf("after baptism: reference date = %v, want 1815", f.ReferenceDate)
			}
			project(domain.NewLifeEventUpdated(baptism.EventID, map[string]any{"date": "1820"}), 2)
			if f := facts(); f.ReferenceDate == nil || f.ReferenceDate.Year() != 1820 {
				t.Errorf("after baptism date change: reference date = %v, want 1820", f.ReferenceDate)
			}
			project(domain.NewLifeEventDeleted(baptism.EventID, ""), 3)
			if f := facts(); f.ReferenceDate != nil {
				t.Errorf("after baptism removed: reference date = %v, want none", f.ReferenceDate)
			}

			burial := domain.NewLifeEventCreatedFromModel(&domain.LifeEvent{ID: uuid.New(), PersonID: &person.ID, FactType: domain.FactPersonBurial})
			project(burial, 1)
			if !facts().DeathEvidence {
				t.Error("after burial: no death evidence")
			}
			project(domain.NewLifeEventUpdated(burial.EventID, map[string]any{"is_negated": true}), 2)
			if facts().DeathEvidence {
				t.Error("after burial negated: death evidence remains")
			}

			project(domain.NewPersonUpdated(person.ID, map[string]any{"living_override": "deceased", "restriction": "confidential"}), 2)
			if f := facts(); f.Override != domain.LivingOverrideDeceased {
				t.Errorf("override = %q, want deceased", f.Override)
			}
			project(domain.NewPersonUpdated(person.ID, map[string]any{"living_override": nil, "restriction": nil}), 3)
			p, _ := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
			if p.LivingOverride != domain.LivingOverrideAuto || p.Restriction != "" {
				t.Errorf("after clears: override %q, restriction %q; want both empty", p.LivingOverride, p.Restriction)
			}
		})
	}
}
