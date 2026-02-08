package demo

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestSeedDemoData(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)

	err := SeedDemoData(context.Background(), cmdHandler)
	if err != nil {
		t.Fatalf("SeedDemoData() failed: %v", err)
	}

	ctx := context.Background()
	opts := repository.ListOptions{Limit: 100}

	// Verify persons were created (13 total)
	persons, total, err := readStore.ListPersons(ctx, opts)
	if err != nil {
		t.Fatalf("ListPersons: %v", err)
	}
	if total != 13 {
		t.Errorf("expected 13 persons, got %d", total)
	}

	// Verify families were created (4 total)
	families, totalFamilies, err := readStore.ListFamilies(ctx, opts)
	if err != nil {
		t.Fatalf("ListFamilies: %v", err)
	}
	if totalFamilies != 4 {
		t.Errorf("expected 4 families, got %d", totalFamilies)
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
		children, err := readStore.GetFamilyChildren(ctx, f.ID)
		if err != nil {
			t.Fatalf("GetFamilyChildren(%s): %v", f.ID, err)
		}
		totalChildren += len(children)
	}

	// Expected children: 1 (family1) + 3 (family2) + 2 (family3) + 2 (family4) = 8
	if totalChildren != 8 {
		t.Errorf("expected 8 total family-child links, got %d", totalChildren)
	}
}
