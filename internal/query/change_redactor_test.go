package query_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// redactionFixture is a tree with a dead person ("Dora Open") and a living
// one ("Lena Hidden") whose changes reach every event-log surface: both are
// edited between two snapshots, on an open research branch and on the
// mainline behind it (conflicts), with research logs on the branch, and on a
// second branch that was merged (a merge record). The services read through
// the privacy decorator, as the server wires them.
type redactionFixture struct {
	dead, living   uuid.UUID
	before, after  uuid.UUID // snapshots
	review, merged uuid.UUID // branches
	history        *query.HistoryService
	snapshots      *query.SnapshotService
	branches       *query.BranchService
	rollback       *query.RollbackService
}

func newRedactionFixture(t *testing.T) *redactionFixture {
	t.Helper()
	ctx := context.Background()
	events := memory.NewEventStore()
	reads := memory.NewReadModelStore()
	branchStore := memory.NewBranchStore()
	snapshotStore := memory.NewSnapshotStore(events)
	h := command.NewHandlerWithBranches(events, reads, branchStore, snapshotStore)

	dead := mustOK(h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Dora", Surname: "Open", DeathDate: "1 JAN 1900"}))
	living := mustOK(h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Lena", Surname: "Hidden"}))
	before := mustOK(h.CreateSnapshot(ctx, "before", ""))

	rename := func(h *command.Handler, id uuid.UUID, version int64, surname string) int64 {
		t.Helper()
		return mustOK(h.UpdatePerson(ctx, command.UpdatePersonInput{ID: id, Surname: command.Set(surname), Version: version})).Version
	}
	deadV := rename(h, dead.ID, dead.Version, "Open-Again")
	livingV := rename(h, living.ID, living.Version, "Hidden-Again")
	after := mustOK(h.CreateSnapshot(ctx, "after", ""))

	merged := mustOK(h.CreateBranch(ctx, "merged", ""))
	rename(h.WithBranch(merged), dead.ID, deadV, "Mergedopen")
	rename(h.WithBranch(merged), living.ID, livingV, "Mergedhidden")
	review := mustOK(h.CreateBranch(ctx, "review", ""))
	onReview := h.WithBranch(review)
	rename(onReview, dead.ID, deadV, "Branchopen")
	rename(onReview, living.ID, livingV, "Branchhidden")
	for subject, search := range map[uuid.UUID]string{dead.ID: "Dora's open register", living.ID: "Lena's register"} {
		mustOK(onReview.CreateResearchLog(ctx, command.CreateResearchLogInput{
			SubjectID: subject, SubjectType: "person", Repository: "Parish", SearchDescription: search,
			Outcome: string(domain.ResearchOutcomeNotFound), SearchDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}))
	}
	rename(h, dead.ID, deadV, "Mainopen")
	rename(h, living.ID, livingV, "Mainhidden")
	mustOK(h.MergeBranch(ctx, command.MergeBranchInput{BranchID: merged.ID, Resolutions: map[uuid.UUID]command.MergeResolution{
		dead.ID: command.ResolveBranch, living.ID: command.ResolveBranch,
	}}))

	privacy := query.NewPrivacyStore(reads)
	history := query.NewHistoryService(events, privacy)
	history.UseBranchStore(branchStore)
	return &redactionFixture{
		dead: dead.ID, living: living.ID, before: before.ID, after: after.ID, review: review.ID, merged: merged.ID,
		history:   history,
		snapshots: query.NewSnapshotService(snapshotStore, events, history),
		branches:  query.NewBranchService(branchStore, events, history),
		rollback:  query.NewRollbackService(events, privacy),
	}
}

// redactionProbes call each service method that returns event-log output, as
// the principal on ctx, once for each record it can be about. Every such
// method must be here or in unredactedOutputs
// (TestChangeRedactor_EveryEventLogOutputIsProbed).
var redactionProbes = map[string]func(ctx context.Context, f *redactionFixture) (any, error){
	"HistoryService.GetEntityHistory": func(ctx context.Context, f *redactionFixture) (any, error) {
		return both(f, func(id uuid.UUID) (any, error) { return f.history.GetEntityHistory(ctx, "person", id, 100, 0) })
	},
	"HistoryService.GetEntityHistoryOn": func(ctx context.Context, f *redactionFixture) (any, error) {
		return both(f, func(id uuid.UUID) (any, error) {
			return f.history.GetEntityHistoryOn(ctx, domain.BranchID(f.review), "person", id, 100, 0)
		})
	},
	"HistoryService.GetGlobalHistory": func(ctx context.Context, f *redactionFixture) (any, error) {
		return f.history.GetGlobalHistory(ctx, query.GetGlobalHistoryInput{ToTime: time.Now().Add(time.Hour), Limit: 100})
	},
	"SnapshotService.CompareSnapshots": func(ctx context.Context, f *redactionFixture) (any, error) {
		return f.snapshots.CompareSnapshots(ctx, domain.MainBranchID, f.before, f.after)
	},
	"SnapshotService.CompareSnapshotToCurrent": func(ctx context.Context, f *redactionFixture) (any, error) {
		return f.snapshots.CompareSnapshotToCurrent(ctx, domain.MainBranchID, f.before)
	},
	"SnapshotService.CompareSnapshotToPosition": func(ctx context.Context, f *redactionFixture) (any, error) {
		return f.snapshots.CompareSnapshotToPosition(ctx, domain.MainBranchID, f.before, nil)
	},
	"BranchService.CompareBranch": func(ctx context.Context, f *redactionFixture) (any, error) {
		review, err := f.branches.CompareBranch(ctx, f.review)
		if err != nil {
			return nil, err
		}
		merged, err := f.branches.CompareBranch(ctx, f.merged)
		return []any{review, merged}, err
	},
	"BranchService.EvidenceCoverage": func(ctx context.Context, f *redactionFixture) (any, error) {
		return f.branches.EvidenceCoverage(ctx, f.review)
	},
	"BranchService.BranchResearchArchive": func(ctx context.Context, f *redactionFixture) (any, error) {
		return f.branches.BranchResearchArchive(ctx, f.review)
	},
	"RollbackService.GetRestorePoints": func(ctx context.Context, f *redactionFixture) (any, error) {
		return both(f, func(id uuid.UUID) (any, error) { return f.rollback.GetRestorePoints(ctx, "Person", id, 100, 0) })
	},
}

// unredactedOutputs are the methods that return event-log output without the
// redactor, and why that is safe.
var unredactedOutputs = map[string]string{
	"BranchService.PlanMerge":                "the merge command's input; it runs for editors, who see everything",
	"BranchService.LoadMergeReplaySet":       "the merge resume command's input; it runs for editors",
	"RollbackService.GetStateAtVersion":      "the rollback command's input; rollback is owner-only",
	"RollbackService.ComputeRollbackChanges": "the rollback command's input; rollback is owner-only",
}

func both(f *redactionFixture, read func(uuid.UUID) (any, error)) (any, error) {
	dead, err := read(f.dead)
	if err != nil {
		return nil, err
	}
	living, err := read(f.living)
	return []any{dead, living}, err
}

// mentions reports whether v, marshalled, mentions the dead person and the
// living one, by name or ID.
func (f *redactionFixture) mentions(t *testing.T, v any) (dead, living bool) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(raw))
	return strings.Contains(text, "open"),
		strings.Contains(text, "hidden") || strings.Contains(text, "lena") || strings.Contains(text, f.living.String())
}

// TestChangeRedactor_EveryEventLogSurface reads every event-log surface as a
// viewer who may not see living people and as an editor: the viewer sees the
// dead person's changes and none of the living one's, the editor both.
func TestChangeRedactor_EveryEventLogSurface(t *testing.T) {
	f := newRedactionFixture(t)
	settings := repository.DefaultTreeSettings()
	settings.ViewersSeeLiving = false
	strictViewer := repository.ContextWithTreeSettings(
		repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleViewer}), settings)
	editor := repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleEditor})

	for name, probe := range redactionProbes {
		t.Run(name, func(t *testing.T) {
			got, err := probe(strictViewer, f)
			if err != nil {
				t.Fatal(err)
			}
			if dead, living := f.mentions(t, got); !dead || living {
				t.Errorf("viewer without living sees the dead person %v and the living one %v; want only the dead: %+v", dead, living, got)
			}
			got, err = probe(editor, f)
			if err != nil {
				t.Fatal(err)
			}
			if dead, living := f.mentions(t, got); !dead || !living {
				t.Errorf("editor sees the dead person %v and the living one %v; want both", dead, living)
			}
		})
	}
}

// TestChangeRedactor_EveryEventLogOutputIsProbed fails when a method of a
// service that reads the event log returns event-log output (change entries,
// merge conflicts and records, restore points, changed facts, archived
// research, raw events or rebuilt state) without being probed by
// TestChangeRedactor_EveryEventLogSurface or listed in unredactedOutputs, so
// a new surface cannot skip the redactor unnoticed (ADR-008 §5b).
func TestChangeRedactor_EveryEventLogOutputIsProbed(t *testing.T) {
	outputs := []reflect.Type{
		reflect.TypeFor[query.ChangeEntry](), reflect.TypeFor[query.MergeConflict](), reflect.TypeFor[query.MergeRecord](),
		reflect.TypeFor[query.RestorePoint](), reflect.TypeFor[query.ChangedFact](), reflect.TypeFor[query.ArchivedResearchLog](),
		reflect.TypeFor[query.ArchivedEvidenceAnalysis](), reflect.TypeFor[query.ArchivedProofSummary](),
		reflect.TypeFor[repository.StoredEvent](), reflect.TypeFor[query.EntityState](), reflect.TypeFor[query.RollbackChanges](),
	}
	services := []reflect.Type{
		reflect.TypeFor[*query.HistoryService](), reflect.TypeFor[*query.SnapshotService](),
		reflect.TypeFor[*query.BranchService](), reflect.TypeFor[*query.RollbackService](),
	}
	seen := map[string]bool{}
	for _, svc := range services {
		for i := range svc.NumMethod() {
			m := svc.Method(i)
			name := svc.Elem().Name() + "." + m.Name
			returnsOutput := false
			for j := range m.Type.NumOut() {
				returnsOutput = returnsOutput || reaches(m.Type.Out(j), outputs, map[reflect.Type]bool{})
			}
			if !returnsOutput {
				continue
			}
			seen[name] = true
			_, probed := redactionProbes[name]
			_, exempt := unredactedOutputs[name]
			if !probed && !exempt {
				t.Errorf("%s returns event-log output but is not probed for redaction: redact it (changeRedactor) and add it to redactionProbes, or say in unredactedOutputs why it need not be", name)
			}
		}
	}
	for name := range redactionProbes {
		if !seen[name] {
			t.Errorf("redactionProbes lists %s, which returns no event-log output", name)
		}
	}
	for name := range unredactedOutputs {
		if !seen[name] {
			t.Errorf("unredactedOutputs lists %s, which returns no event-log output", name)
		}
	}
}

// reaches reports whether a value of type typ can hold one of targets.
func reaches(typ reflect.Type, targets []reflect.Type, visited map[reflect.Type]bool) bool {
	if visited[typ] {
		return false
	}
	visited[typ] = true
	for _, target := range targets {
		if typ == target {
			return true
		}
	}
	switch typ.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return reaches(typ.Elem(), targets, visited)
	case reflect.Map:
		return reaches(typ.Key(), targets, visited) || reaches(typ.Elem(), targets, visited)
	case reflect.Struct:
		for i := range typ.NumField() {
			if reaches(typ.Field(i).Type, targets, visited) {
				return true
			}
		}
	}
	return false
}
