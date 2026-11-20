package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
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

// TestProjector_MainLifeEventsRefreshBranchCopies proves a branch's own copy
// of a person follows a death recorded and removed on main, which the branch
// sees through its overlay, while a branch without a copy is not given one.
func TestProjector_MainLifeEventsRefreshBranchCopies(t *testing.T) {
	ctx := context.Background()
	rs, branches := memory.NewReadModelStore(), memory.NewBranchStore()
	copied, _ := domain.NewBranch("copied", "", 0)
	plain, _ := domain.NewBranch("plain", "", 0)
	for _, b := range []*domain.Branch{copied, plain} {
		if err := branches.Create(ctx, b); err != nil {
			t.Fatalf("create branch: %v", err)
		}
	}
	projector := repository.NewProjector(rs, branches)
	project := func(e domain.Event, version int64, branchID domain.BranchID) {
		t.Helper()
		if err := projector.Project(ctx, e, version, branchID); err != nil {
			t.Fatalf("Project %s: %v", e.EventType(), err)
		}
	}
	person := domain.NewPerson("Ada", "Lovelace")
	project(domain.NewPersonCreated(person), 1, domain.MainBranchID)
	project(domain.NewPersonUpdated(person.ID, map[string]any{"notes": "branch copy"}), 2, domain.BranchID(copied.ID))

	evidence := func(b *domain.Branch) bool {
		t.Helper()
		p, err := rs.GetPerson(ctx, domain.BranchID(b.ID), person.ID)
		if err != nil || p == nil {
			t.Fatalf("GetPerson on %s: %v, %v", b.Name, p, err)
		}
		return p.DeathEvidence
	}
	burial := domain.NewLifeEventCreatedFromModel(&domain.LifeEvent{ID: uuid.New(), PersonID: &person.ID, FactType: domain.FactPersonBurial})
	project(burial, 1, domain.MainBranchID)
	for _, b := range []*domain.Branch{copied, plain} {
		if !evidence(b) {
			t.Errorf("after main's burial: branch %s has no death evidence", b.Name)
		}
	}
	project(domain.NewLifeEventDeleted(burial.EventID, ""), 2, domain.MainBranchID)
	for _, b := range []*domain.Branch{copied, plain} {
		if evidence(b) {
			t.Errorf("after main's burial removed: branch %s keeps death evidence", b.Name)
		}
	}
	if p, _ := rs.GetPerson(ctx, domain.BranchID(copied.ID), person.ID); p == nil || p.Notes != "branch copy" {
		t.Errorf("branch copy = %+v, want its own notes kept", p)
	}
}

// personAccessLog is a read model that records the branches whose person rows
// are read or written.
type personAccessLog struct {
	*memory.ReadModelStore
	reads, writes map[domain.BranchID]int
}

func (s *personAccessLog) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	s.reads[branchID]++
	return s.ReadModelStore.GetPerson(ctx, branchID, id)
}

func (s *personAccessLog) SavePerson(ctx context.Context, branchID domain.BranchID, p *repository.PersonReadModel) error {
	s.writes[branchID]++
	return s.ReadModelStore.SavePerson(ctx, branchID, p)
}

// TestProjector_MainLifeEventsSkipClosedBranches proves a life event on main
// refreshes the living rule on an open branch's copy of the person but neither
// reads nor writes the person on a branch that was merged or abandoned, whose
// copy was purged when it closed.
func TestProjector_MainLifeEventsSkipClosedBranches(t *testing.T) {
	ctx := context.Background()
	rs := &personAccessLog{ReadModelStore: memory.NewReadModelStore(), reads: map[domain.BranchID]int{}, writes: map[domain.BranchID]int{}}
	branches := memory.NewBranchStore()
	projector := repository.NewProjector(rs, branches)
	project := func(e domain.Event, version int64, branchID domain.BranchID) {
		t.Helper()
		if err := projector.Project(ctx, e, version, branchID); err != nil {
			t.Fatalf("Project %s: %v", e.EventType(), err)
		}
	}
	person := domain.NewPerson("Ada", "Lovelace")
	project(domain.NewPersonCreated(person), 1, domain.MainBranchID)
	open, _ := domain.NewBranch("open", "", 0)
	merged, _ := domain.NewBranch("merged", "", 0)
	abandoned, _ := domain.NewBranch("abandoned", "", 0)
	for _, b := range []*domain.Branch{open, merged, abandoned} {
		project(domain.NewBranchCreated(b), 0, domain.MainBranchID)
		project(domain.NewPersonUpdated(person.ID, map[string]any{"notes": "copy on " + b.Name}), 2, domain.BranchID(b.ID))
	}
	project(domain.NewBranchMerged(merged.ID, 0, 0, "", nil), 0, domain.MainBranchID)
	project(domain.NewBranchDeleted(abandoned.ID), 0, domain.MainBranchID)
	clear(rs.reads)
	clear(rs.writes)

	burial := domain.NewLifeEventCreatedFromModel(&domain.LifeEvent{ID: uuid.New(), PersonID: &person.ID, FactType: domain.FactPersonBurial})
	project(burial, 1, domain.MainBranchID)

	if rs.writes[domain.BranchID(open.ID)] != 1 {
		t.Errorf("open branch's copy written %d times, want once", rs.writes[domain.BranchID(open.ID)])
	}
	for _, b := range []*domain.Branch{merged, abandoned} {
		if n, m := rs.reads[domain.BranchID(b.ID)], rs.writes[domain.BranchID(b.ID)]; n != 0 || m != 0 {
			t.Errorf("%s branch: person read %d and written %d times, want neither", b.Name, n, m)
		}
	}
	if p, _ := rs.GetPerson(ctx, domain.BranchID(open.ID), person.ID); p == nil || !p.DeathEvidence {
		t.Errorf("open branch's copy = %+v, want death evidence", p)
	}
}
