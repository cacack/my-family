package demo

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// demoStores are the in-memory stores demo mode seeds.
type demoStores struct {
	events    *memory.EventStore
	readModel *memory.ReadModelStore
	snapshots *memory.SnapshotStore
	branches  *memory.BranchStore
}

// seedMemory seeds fresh in-memory stores, wired as the demo-mode server wires them.
func seedMemory(t *testing.T) demoStores {
	t.Helper()
	eventStore := memory.NewEventStore()
	s := demoStores{
		events:    eventStore,
		readModel: memory.NewReadModelStore(),
		snapshots: memory.NewSnapshotStore(eventStore),
		branches:  memory.NewBranchStore(),
	}
	cmdHandler := command.NewHandlerWithBranches(s.events, s.readModel, s.branches, s.snapshots)

	if err := SeedDemoData(context.Background(), cmdHandler, s.readModel); err != nil {
		t.Fatalf("SeedDemoData() failed: %v", err)
	}
	return s
}

func TestSeedDemoData(t *testing.T) {
	readStore := seedMemory(t).readModel

	ctx := context.Background()
	opts := repository.ListOptions{Limit: 100}

	// 13 created persons, 4 GEDCOM-imported Mueller/Schmidt ancestors, 1 duplicate
	persons, total, err := readStore.ListPersons(ctx, opts)
	if err != nil {
		t.Fatalf("ListPersons: %v", err)
	}
	if total != 18 {
		t.Errorf("expected 18 persons, got %d", total)
	}

	// 4 created families, 2 imported
	families, totalFamilies, err := readStore.ListFamilies(ctx, opts)
	if err != nil {
		t.Fatalf("ListFamilies: %v", err)
	}
	if totalFamilies != 6 {
		t.Errorf("expected 6 families, got %d", totalFamilies)
	}

	// Verify sources were created (1 total)
	_, totalSources, err := readStore.ListSources(ctx, opts)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if totalSources != 1 {
		t.Errorf("expected 1 source, got %d", totalSources)
	}

	// Verify citations were created (2 total)
	_, totalCitations, err := readStore.ListCitations(ctx, opts)
	if err != nil {
		t.Fatalf("ListCitations: %v", err)
	}
	if totalCitations != 2 {
		t.Errorf("expected 2 citations, got %d", totalCitations)
	}

	// Verify all persons have [DEMO DATA] marker in notes
	for _, p := range persons {
		if !strings.Contains(p.Notes, "[DEMO DATA]") {
			t.Errorf("person %q notes missing [DEMO DATA] marker, got %q", p.GivenName+" "+p.Surname, p.Notes)
		}
	}

	// Verify family children links
	totalChildren := 0
	for _, f := range families {
		children, err := readStore.GetFamilyChildren(ctx, domain.MainBranchID, f.ID)
		if err != nil {
			t.Fatalf("GetFamilyChildren(%s): %v", f.ID, err)
		}
		totalChildren += len(children)
	}

	// Expected children: 1 (family1) + 3 (family2) + 2 (family3) + 2 (family4)
	// + Anna (Mueller family) + Margarethe (Schmidt family) = 10
	if totalChildren != 10 {
		t.Errorf("expected 10 total family-child links, got %d", totalChildren)
	}
}

// TestSeedDemoData_NavSurfaces checks that every global-nav page backed by seeded
// data has something to render under DEMO_MODE (#495).
func TestSeedDemoData_NavSurfaces(t *testing.T) {
	stores := seedMemory(t)
	eventStore, readStore := stores.events, stores.readModel
	ctx := context.Background()
	main := domain.MainBranchID
	browse := query.NewBrowseService(readStore)

	t.Run("browse surnames", func(t *testing.T) {
		res, err := browse.GetSurnameIndex(ctx, query.GetSurnameIndexInput{BranchID: main})
		if err != nil {
			t.Fatalf("GetSurnameIndex: %v", err)
		}
		if len(res.Items) < 2 {
			t.Errorf("expected at least 2 surnames, got %d", len(res.Items))
		}
	})

	t.Run("browse places", func(t *testing.T) {
		res, err := browse.GetPlaceHierarchy(ctx, query.GetPlaceHierarchyInput{BranchID: main})
		if err != nil {
			t.Fatalf("GetPlaceHierarchy: %v", err)
		}
		if len(res.Items) < 2 {
			t.Errorf("expected at least 2 places, got %d", len(res.Items))
		}
	})

	t.Run("browse cemeteries", func(t *testing.T) {
		res, err := browse.GetCemeteryIndex(ctx, main)
		if err != nil {
			t.Fatalf("GetCemeteryIndex: %v", err)
		}
		if len(res.Items) < 1 {
			t.Error("expected at least 1 cemetery")
		}
	})

	t.Run("browse brick walls", func(t *testing.T) {
		res, err := browse.GetBrickWalls(ctx, false)
		if err != nil {
			t.Fatalf("GetBrickWalls: %v", err)
		}
		if len(res.Items) < 1 {
			t.Error("expected at least 1 brick wall")
		}
	})

	t.Run("map", func(t *testing.T) {
		res, err := browse.GetMapLocations(ctx, main)
		if err != nil {
			t.Fatalf("GetMapLocations: %v", err)
		}
		if len(res.Items) < 2 {
			t.Errorf("expected at least 2 geocoded locations, got %d", len(res.Items))
		}
	})

	t.Run("evidence", func(t *testing.T) {
		evidence := query.NewEvidenceQueryService(readStore)
		in := query.ListInput{Limit: 100}
		analyses, err := evidence.ListEvidenceAnalyses(ctx, in)
		if err != nil {
			t.Fatalf("ListEvidenceAnalyses: %v", err)
		}
		conflicts, err := evidence.ListEvidenceConflicts(ctx, in)
		if err != nil {
			t.Fatalf("ListEvidenceConflicts: %v", err)
		}
		logs, err := evidence.ListResearchLogs(ctx, in)
		if err != nil {
			t.Fatalf("ListResearchLogs: %v", err)
		}
		summaries, err := evidence.ListProofSummaries(ctx, in)
		if err != nil {
			t.Fatalf("ListProofSummaries: %v", err)
		}
		if analyses.Total < 1 || conflicts.Total < 1 || logs.Total < 1 || summaries.Total < 1 {
			t.Errorf("expected at least 1 of each evidence record, got analyses=%d conflicts=%d logs=%d summaries=%d",
				analyses.Total, conflicts.Total, logs.Total, summaries.Total)
		}
	})

	t.Run("history", func(t *testing.T) {
		res, err := query.NewHistoryService(eventStore, readStore).GetGlobalHistory(ctx, query.GetGlobalHistoryInput{Limit: 10})
		if err != nil {
			t.Fatalf("GetGlobalHistory: %v", err)
		}
		if len(res.Entries) == 0 {
			t.Error("expected history entries")
		}
	})

	t.Run("quality", func(t *testing.T) {
		validation := query.NewValidationService(readStore)
		_, dupTotal, err := validation.FindDuplicates(ctx, 100, 0)
		if err != nil {
			t.Fatalf("FindDuplicates: %v", err)
		}
		if dupTotal < 1 {
			t.Error("expected at least 1 potential duplicate pair")
		}
		issues, err := validation.GetValidationIssues(ctx, "", 0, 0)
		if err != nil {
			t.Fatalf("GetValidationIssues: %v", err)
		}
		if issues.ErrorCount < 1 || issues.WarningCount < 1 || issues.InfoCount < 1 {
			t.Errorf("expected a validation issue of every severity, got errors=%d warnings=%d info=%d",
				issues.ErrorCount, issues.WarningCount, issues.InfoCount)
		}
	})

	t.Run("repositories", func(t *testing.T) {
		res, err := query.NewRepositoryService(readStore).ListRepositories(ctx, query.ListRepositoriesInput{})
		if err != nil {
			t.Fatalf("ListRepositories: %v", err)
		}
		if res.Total < 1 {
			t.Error("expected at least 1 repository")
		}
	})

	t.Run("snapshots", func(t *testing.T) {
		snapshots, err := stores.snapshots.List(ctx, main)
		if err != nil {
			t.Fatalf("List snapshots: %v", err)
		}
		if len(snapshots) < 1 {
			t.Error("expected at least 1 snapshot")
		}
	})

	t.Run("branches", func(t *testing.T) {
		branches, err := stores.branches.List(ctx)
		if err != nil {
			t.Fatalf("List branches: %v", err)
		}
		if len(branches) < 1 {
			t.Error("expected at least 1 research branch")
		}
	})

	t.Run("relationship", func(t *testing.T) {
		william := findPerson(t, readStore, "William", "Thompson")
		emily := findPerson(t, readStore, "Emily", "Thompson")
		res, err := query.NewRelationshipService(readStore).GetRelationship(ctx, main, william, emily)
		if err != nil {
			t.Fatalf("GetRelationship: %v", err)
		}
		if !res.IsRelated {
			t.Error("expected William and Emily Thompson to be related")
		}
	})
}

func findPerson(t *testing.T, readStore repository.ReadModelStore, given, surname string) uuid.UUID {
	t.Helper()
	persons, _, err := readStore.ListPersons(context.Background(), repository.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("ListPersons: %v", err)
	}
	for _, p := range persons {
		if p.GivenName == given && p.Surname == surname {
			return p.ID
		}
	}
	t.Fatalf("person %s %s not found", given, surname)
	return uuid.Nil
}
