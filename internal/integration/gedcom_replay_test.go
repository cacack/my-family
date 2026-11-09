package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// GEDCOM import values in the event log (issue #1225).
//
// Import used to write place coordinates and external identifiers straight to
// the read model, so a replay of the log lost them. They now travel in the
// entities' created events, so replaying the log into an empty read model,
// with no carry-over step, reproduces them.

const replayGedcom = `0 HEAD
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
1 DEAT
2 DATE 1 JAN 1920
2 PLAC Salem
3 MAP
4 LATI N42.5195
4 LONG W70.8967
1 RESI
2 DATE 1880
2 PLAC Lowell
3 MAP
4 LATI N42.6334
4 LONG W71.3162
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 EXID F-100
2 TYPE http://example.com/family
0 @R1@ REPO
1 NAME State Archive
1 EXID R-100
2 TYPE http://example.com/repo
0 @S1@ SOUR
1 TITL Census 1880
1 REPO @R1@
1 EXID S-100
2 TYPE http://example.com/source
0 TRLR
`

func TestGedcomImport_ReplayReproducesCoordinatesAndExternalIDs(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(replayGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}

			live := dumpImportedFields(t, ctx, st.read)
			for _, want := range []string{"person-coords", "life-event-coords", "person-exids", "family-exids", "source-exids", "repository-exids"} {
				if !hasKeyPrefix(live, want+"/") {
					t.Fatalf("precondition: import stored no %s; dump = %v", want, live)
				}
			}

			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			assertSameDump(t, live, dumpImportedFields(t, ctx, fresh))
		})
	}
}

// dumpImportedFields renders the main-branch coordinates and EXIDs GEDCOM
// import stores, keyed by kind and entity id. Entities without them are left
// out, so a replay that loses a value shows up as a missing key.
func dumpImportedFields(t *testing.T, ctx context.Context, rs repository.ReadModelStore) map[string]string {
	t.Helper()
	out := map[string]string{}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("dump imported fields: %v", err)
		}
	}
	keep := func(key string, v any, present bool) {
		if present {
			out[key] = canonicalRowJSON(t, v)
		}
	}

	persons, err := repository.ListAll(ctx, 100, rs.ListPersons)
	must(err)
	for _, p := range persons {
		coords := []*string{p.BirthPlaceLat, p.BirthPlaceLong, p.DeathPlaceLat, p.DeathPlaceLong}
		keep("person-coords/"+p.ID.String(), coords, p.BirthPlaceLat != nil || p.BirthPlaceLong != nil || p.DeathPlaceLat != nil || p.DeathPlaceLong != nil)
		ids, err := rs.GetPersonExternalIDs(ctx, domain.MainBranchID, p.ID)
		must(err)
		keep("person-exids/"+p.ID.String(), ids, len(ids) > 0)
	}
	events, err := repository.ListAll(ctx, 100, rs.ListEvents)
	must(err)
	for _, e := range events {
		keep("life-event-coords/"+e.ID.String(), []*string{e.PlaceLat, e.PlaceLong}, e.PlaceLat != nil || e.PlaceLong != nil)
	}
	families, err := repository.ListAll(ctx, 100, rs.ListFamilies)
	must(err)
	for _, f := range families {
		ids, err := rs.GetFamilyExternalIDs(ctx, domain.MainBranchID, f.ID)
		must(err)
		keep("family-exids/"+f.ID.String(), ids, len(ids) > 0)
	}
	sources, err := repository.ListAll(ctx, 100, rs.ListSources)
	must(err)
	for _, s := range sources {
		ids, err := rs.GetSourceExternalIDs(ctx, domain.MainBranchID, s.ID)
		must(err)
		keep("source-exids/"+s.ID.String(), ids, len(ids) > 0)
	}
	repos, err := repository.ListAll(ctx, 100, rs.ListRepositories)
	must(err)
	for _, r := range repos {
		ids, err := rs.GetRepositoryExternalIDs(ctx, r.ID)
		must(err)
		keep("repository-exids/"+r.ID.String(), ids, len(ids) > 0)
	}
	return out
}

func hasKeyPrefix(m map[string]string, prefix string) bool {
	for k := range m {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// TestRebuild_BackfillsValuesOnlyTheReadModelHeld seeds a database the way an
// import made before #1225 left it: created events without coordinates or
// EXIDs, and those values written straight to the read model. A rebuild must
// log them before its reset and keep them, and leave every row's version in
// step with its stream so the next edit is not refused (#1251).
func TestRebuild_BackfillsValuesOnlyTheReadModelHeld(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			src := setupMemory(t)
			srcHandler := command.NewHandlerWithBranches(src.events, src.read, src.branches, src.snapshots)
			if _, err := srcHandler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(replayGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			want := dumpImportedFields(t, ctx, src.read)

			st := backend.setup(t)
			copyLogWithoutImportedValues(t, ctx, src.events, st.events)
			replayLog(t, ctx, st.events, st.read)
			if got := dumpImportedFields(t, ctx, st.read); len(got) != 0 {
				t.Fatalf("precondition: the copied log still carries imported values: %v", got)
			}
			copyImportedFields(t, ctx, src.read, st.read)
			assertSameDump(t, want, dumpImportedFields(t, ctx, st.read))

			result, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots)
			if err != nil {
				t.Fatalf("rebuild: %v", err)
			}
			// John (birth and death coordinates, an EXID), his residence (a
			// life event with coordinates), the family, the source and the
			// repository.
			if result.Backfilled != 5 || result.DroppedImported != 0 {
				t.Errorf("Backfilled = %d, DroppedImported = %d; want 5 and 0", result.Backfilled, result.DroppedImported)
			}
			assertSameDump(t, want, dumpImportedFields(t, ctx, st.read))

			again, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots)
			if err != nil || again.Backfilled != 0 || again.DroppedImported != 0 {
				t.Errorf("second rebuild = %+v, %v; want nothing left to backfill or drop", again, err)
			}
			assertSameDump(t, want, dumpImportedFields(t, ctx, st.read))

			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			persons, err := repository.ListAll(ctx, 100, st.read.ListPersons)
			if err != nil {
				t.Fatalf("list persons: %v", err)
			}
			for _, p := range persons {
				surname := p.Surname + "-edited"
				if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: p.ID, Surname: &surname, Version: p.Version}); err != nil {
					t.Errorf("update %s at read-model version %d: %v", p.GivenName, p.Version, err)
				}
			}
		})
	}
}

// copyLogWithoutImportedValues appends every event of from's log to to, with
// the coordinates and EXIDs that #1225 added to the created events removed.
func copyLogWithoutImportedValues(t *testing.T, ctx context.Context, from, to repository.EventStore) {
	t.Helper()
	var after int64
	for {
		page, err := from.ReadAll(ctx, after, 500)
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		if len(page) == 0 {
			return
		}
		for _, stored := range page {
			decoded, err := stored.DecodeEvent()
			if err != nil {
				t.Fatalf("decode %s: %v", stored.EventType, err)
			}
			switch e := decoded.(type) {
			case domain.PersonCreated:
				e.BirthPlaceLat, e.BirthPlaceLong, e.DeathPlaceLat, e.DeathPlaceLong, e.ExternalIDs = nil, nil, nil, nil, nil
				decoded = e
			case domain.LifeEventCreated:
				e.PlaceLat, e.PlaceLong = nil, nil
				decoded = e
			case domain.FamilyCreated:
				e.ExternalIDs = nil
				decoded = e
			case domain.SourceCreated:
				e.ExternalIDs = nil
				decoded = e
			case domain.RepositoryCreated:
				e.ExternalIDs = nil
				decoded = e
			}
			if err := to.Append(ctx, stored.StreamID, stored.StreamType, []domain.Event{decoded}, stored.Version-1, repository.MainScope); err != nil {
				t.Fatalf("append %s: %v", stored.EventType, err)
			}
			after = stored.Position
		}
	}
}

// copyImportedFields writes from's main-branch coordinates and EXIDs straight
// onto to's rows, as import did before #1225.
func copyImportedFields(t *testing.T, ctx context.Context, from, to repository.ReadModelStore) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("copy imported fields: %v", err)
		}
	}
	persons, err := repository.ListAll(ctx, 100, from.ListPersons)
	must(err)
	for _, p := range persons {
		row, err := to.GetPerson(ctx, domain.MainBranchID, p.ID)
		must(err)
		row.BirthPlaceLat, row.BirthPlaceLong, row.DeathPlaceLat, row.DeathPlaceLong = p.BirthPlaceLat, p.BirthPlaceLong, p.DeathPlaceLat, p.DeathPlaceLong
		must(to.SavePerson(ctx, domain.MainBranchID, row))
		ids, err := from.GetPersonExternalIDs(ctx, domain.MainBranchID, p.ID)
		must(err)
		must(to.ReplacePersonExternalIDs(ctx, domain.MainBranchID, p.ID, ids))
	}
	events, err := repository.ListAll(ctx, 100, from.ListEvents)
	must(err)
	for _, e := range events {
		row, err := to.GetEvent(ctx, domain.MainBranchID, e.ID)
		must(err)
		row.PlaceLat, row.PlaceLong = e.PlaceLat, e.PlaceLong
		must(to.SaveEvent(ctx, domain.MainBranchID, row))
	}
	families, err := repository.ListAll(ctx, 100, from.ListFamilies)
	must(err)
	for _, f := range families {
		ids, err := from.GetFamilyExternalIDs(ctx, domain.MainBranchID, f.ID)
		must(err)
		must(to.ReplaceFamilyExternalIDs(ctx, domain.MainBranchID, f.ID, ids))
	}
	sources, err := repository.ListAll(ctx, 100, from.ListSources)
	must(err)
	for _, s := range sources {
		ids, err := from.GetSourceExternalIDs(ctx, domain.MainBranchID, s.ID)
		must(err)
		must(to.ReplaceSourceExternalIDs(ctx, domain.MainBranchID, s.ID, ids))
	}
	repos, err := repository.ListAll(ctx, 100, from.ListRepositories)
	must(err)
	for _, r := range repos {
		ids, err := from.GetRepositoryExternalIDs(ctx, r.ID)
		must(err)
		must(to.ReplaceRepositoryExternalIDs(ctx, r.ID, ids))
	}
}
