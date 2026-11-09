package integration_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Projection failure and repair (issue #845).
//
// A command appends its events and then projects them as a separate step. When
// the projection fails, the command still succeeds (the event is durable), so
// the failure must not be silent: it is recorded as a marker in the read model.
// RebuildReadModel then replays the log into an emptied read model, which must
// bring the missing row back and leave every other row exactly as it was —
// including the fields no event carries (GEDCOM coordinates and EXIDs, brick
// walls). The scenario runs on every backend (DB-001).

var errInjectedProjection = errors.New("injected projection failure")

// failingPersonStore fails the first SavePerson for a person with the given
// given name, as a database error mid-projection would.
type failingPersonStore struct {
	repository.ReadModelStore
	givenName string
	failed    bool
}

func (s *failingPersonStore) SavePerson(ctx context.Context, branchID domain.BranchID, p *repository.PersonReadModel) error {
	if !s.failed && p.GivenName == s.givenName {
		s.failed = true
		return errInjectedProjection
	}
	return s.ReadModelStore.SavePerson(ctx, branchID, p)
}

const rebuildGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 EXID KWCJ-QN7
2 TYPE http://www.familysearch.org/ark
1 BIRT
2 DATE 1 JAN 1850
2 PLAC Boston
3 MAP
4 LATI N42.3601
4 LONG W71.0589
0 TRLR
`

func TestProjectionFailure_RecordedAndRepairedByRebuild(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			failing := &failingPersonStore{ReadModelStore: st.read, givenName: "Broken"}
			h := command.NewHandlerWithBranches(st.events, failing, st.branches, st.snapshots)

			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(rebuildGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			persons, err := repository.ListAll(ctx, 100, st.read.ListPersons)
			if err != nil || len(persons) != 1 {
				t.Fatalf("imported persons = %v, %v; want one", persons, err)
			}
			john := persons[0]
			if john.BirthPlaceLat == nil {
				t.Fatal("precondition: import did not store birth coordinates")
			}

			ada, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
			if err != nil {
				t.Fatalf("create Ada: %v", err)
			}
			broken, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Broken", Surname: "Example"})
			if err != nil {
				t.Fatalf("create Broken: command must succeed once its event is appended, got %v", err)
			}
			if _, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &john.ID, Partner2ID: &ada.ID, RelationshipType: "marriage"}); err != nil {
				t.Fatalf("create family: %v", err)
			}
			branch, err := h.CreateBranch(ctx, "research", "")
			if err != nil {
				t.Fatalf("create branch: %v", err)
			}
			notes := "branch only"
			if _, err := h.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{ID: ada.ID, Notes: &notes, Version: ada.Version}); err != nil {
				t.Fatalf("branch update: %v", err)
			}
			// John's branch row is copied from main before his brick wall is set,
			// so it must not carry the brick wall after the rebuild either.
			johnNotes := "branch edit"
			if _, err := h.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{ID: john.ID, Notes: &johnNotes, Version: john.Version}); err != nil {
				t.Fatalf("branch update of John: %v", err)
			}
			if err := st.read.SetBrickWall(ctx, john.ID, "no parents found"); err != nil {
				t.Fatalf("set brick wall: %v", err)
			}
			if _, err := h.CreateSnapshot(ctx, "before rebuild", ""); err != nil {
				t.Fatalf("create snapshot: %v", err)
			}

			// The failure is visible: the row is missing and a marker names the event.
			if row, err := st.read.GetPerson(ctx, domain.MainBranchID, broken.ID); err != nil || row != nil {
				t.Fatalf("Broken's row = %v, %v; want missing after the injected failure", row, err)
			}
			failures, err := st.read.ListProjectionFailures(ctx, 10)
			if err != nil {
				t.Fatalf("list failures: %v", err)
			}
			if len(failures) != 1 {
				t.Fatalf("projection failures = %+v, want exactly one", failures)
			}
			if f := failures[0]; f.StreamID != broken.ID || f.Version != 1 || f.EventType != "PersonCreated" ||
				!f.BranchID.IsMain() || !strings.Contains(f.Error, errInjectedProjection.Error()) || f.FailedAt.IsZero() {
				t.Errorf("failure marker = %+v, want Broken's PersonCreated v1 on main", f)
			}

			before := dumpReadModel(t, ctx, st, branch.ID)

			result, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots)
			if err != nil {
				t.Fatalf("rebuild: %v", err)
			}
			if result.Events == 0 || result.LastPosition == 0 {
				t.Errorf("rebuild result = %+v, want events replayed", result)
			}

			row, err := st.read.GetPerson(ctx, domain.MainBranchID, broken.ID)
			if err != nil || row == nil || row.GivenName != "Broken" || row.Version != 1 {
				t.Fatalf("Broken's row after rebuild = %+v, %v; want it projected", row, err)
			}
			if failures, err := st.read.ListProjectionFailures(ctx, 10); err != nil || len(failures) != 0 {
				t.Errorf("projection failures after rebuild = %+v, %v; want none", failures, err)
			}

			after := dumpReadModel(t, ctx, st, branch.ID)
			dropPerson(after, broken.ID)
			assertSameDump(t, before, after)

			// A second rebuild changes nothing.
			if _, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots); err != nil {
				t.Fatalf("second rebuild: %v", err)
			}
			again := dumpReadModel(t, ctx, st, branch.ID)
			dropPerson(again, broken.ID)
			assertSameDump(t, before, again)
		})
	}
}

// TestRebuild_ReplayFailureIsRecorded proves a rebuild that cannot project an
// event reports an error and leaves a marker, so the drift stays visible, and
// still replays the events after it.
func TestRebuild_ReplayFailureIsRecorded(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			p, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Broken", Surname: "Example"})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			later, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Later", Surname: "Example"})
			if err != nil {
				t.Fatalf("create later: %v", err)
			}

			failing := &failingPersonStore{ReadModelStore: st.read, givenName: "Broken"}
			result, err := repository.RebuildReadModel(ctx, st.events, failing, st.branches, st.snapshots)
			if !errors.Is(err, errInjectedProjection) {
				t.Fatalf("rebuild error = %v, want the injected failure", err)
			}
			if result.Events != 2 || result.Failed != 1 {
				t.Errorf("rebuild result = %+v, want 2 events replayed, 1 failed", result)
			}
			failures, err := st.read.ListProjectionFailures(ctx, 10)
			if err != nil || len(failures) != 1 || failures[0].StreamID != p.ID {
				t.Fatalf("failures = %+v, %v; want one marker for the person", failures, err)
			}
			if row, err := st.read.GetPerson(ctx, domain.MainBranchID, later.ID); err != nil || row == nil {
				t.Fatalf("event after the failure was not replayed: %v, %v", row, err)
			}

			// Retrying once the cause is gone repairs it.
			if _, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots); err != nil {
				t.Fatalf("retry rebuild: %v", err)
			}
			if row, err := st.read.GetPerson(ctx, domain.MainBranchID, p.ID); err != nil || row == nil {
				t.Fatalf("person after retry = %v, %v; want present", row, err)
			}
		})
	}
}

// dumpReadModel renders the rows the scenario touches as canonical JSON,
// keyed by kind, branch and id.
func dumpReadModel(t *testing.T, ctx context.Context, st stores, branchID uuid.UUID) map[string]string {
	t.Helper()
	out := make(map[string]string)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("dump read model: %v", err)
		}
	}
	for _, scope := range []domain.BranchID{domain.MainBranchID, domain.BranchID(branchID)} {
		label := "main"
		if !scope.IsMain() {
			label = "branch"
		}
		persons, err := repository.ListAllOn(ctx, scope, 100, st.read.ListPersons)
		must(err)
		for _, p := range persons {
			out["person/"+label+"/"+p.ID.String()] = canonicalRowJSON(t, p)
		}
		families, err := repository.ListAllOn(ctx, scope, 100, st.read.ListFamilies)
		must(err)
		for _, f := range families {
			out["family/"+label+"/"+f.ID.String()] = canonicalRowJSON(t, f)
		}
		events, err := repository.ListAllOn(ctx, scope, 100, st.read.ListEvents)
		must(err)
		for _, e := range events {
			out["life-event/"+label+"/"+e.ID.String()] = canonicalRowJSON(t, e)
		}
	}
	mainPersons, err := repository.ListAll(ctx, 100, st.read.ListPersons)
	must(err)
	for _, p := range mainPersons {
		ids, err := st.read.GetPersonExternalIDs(ctx, domain.MainBranchID, p.ID)
		must(err)
		out["person-exids/"+p.ID.String()] = canonicalRowJSON(t, ids)
	}
	walls, err := st.read.GetBrickWalls(ctx, true)
	must(err)
	out["brick-walls"] = canonicalRowJSON(t, walls)
	branches, err := st.branches.List(ctx)
	must(err)
	out["branches"] = canonicalRowJSON(t, branches)
	snaps, err := st.snapshots.List(ctx, domain.MainBranchID)
	must(err)
	out["snapshots"] = canonicalRowJSON(t, snaps)
	return out
}

// dropPerson removes a person's entries (on main, through the branch overlay,
// and its EXIDs) from a dump.
func dropPerson(dump map[string]string, id uuid.UUID) {
	for _, key := range []string{"person/main/", "person/branch/", "person-exids/"} {
		delete(dump, key+id.String())
	}
}

func assertSameDump(t *testing.T, want, got map[string]string) {
	t.Helper()
	keys := make([]string, 0, len(want)+len(got))
	for k := range want {
		keys = append(keys, k)
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if want[k] != got[k] {
			t.Errorf("%s differs after rebuild\n before: %s\n after:  %s", k, want[k], got[k])
		}
	}
}
