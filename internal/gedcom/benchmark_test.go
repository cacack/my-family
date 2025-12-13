package gedcom_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository/memory"
)

// generateLargeGEDCOM generates a GEDCOM file with the specified number of individuals.
// Creates a multi-generational family tree structure.
func generateLargeGEDCOM(numIndividuals int) []byte {
	var buf bytes.Buffer

	// Header
	buf.WriteString("0 HEAD\n")
	buf.WriteString("1 SOUR TestGenerator\n")
	buf.WriteString("1 GEDC\n")
	buf.WriteString("2 VERS 5.5\n")
	buf.WriteString("2 FORM LINEAGE-LINKED\n")
	buf.WriteString("1 CHAR UTF-8\n")

	// Generate individuals
	for i := 1; i <= numIndividuals; i++ {
		buf.WriteString(fmt.Sprintf("0 @I%d@ INDI\n", i))
		buf.WriteString(fmt.Sprintf("1 NAME Person%d /TestFamily%d/\n", i, i/10))
		if i%2 == 0 {
			buf.WriteString("1 SEX F\n")
		} else {
			buf.WriteString("1 SEX M\n")
		}
		// Add birth event for some individuals
		if i%3 == 0 {
			year := 1800 + (i % 200)
			buf.WriteString(fmt.Sprintf("1 BIRT\n2 DATE 1 JAN %d\n", year))
			buf.WriteString(fmt.Sprintf("2 PLAC City%d, State%d, Country%d\n", i%100, i%50, i%25))
		}
	}

	// Generate families (connect individuals in pairs with children)
	familyCount := numIndividuals / 4 // Roughly 1 family per 4 individuals
	for i := 1; i <= familyCount; i++ {
		husbID := (i * 2) - 1
		wifeID := i * 2
		childID := numIndividuals/2 + i

		if husbID <= numIndividuals && wifeID <= numIndividuals {
			buf.WriteString(fmt.Sprintf("0 @F%d@ FAM\n", i))
			buf.WriteString(fmt.Sprintf("1 HUSB @I%d@\n", husbID))
			buf.WriteString(fmt.Sprintf("1 WIFE @I%d@\n", wifeID))
			if childID <= numIndividuals {
				buf.WriteString(fmt.Sprintf("1 CHIL @I%d@\n", childID))
			}
		}
	}

	// Trailer
	buf.WriteString("0 TRLR\n")

	return buf.Bytes()
}

// TestPerformance_SC001_Import5K tests SC-001: Import 5K GEDCOM within 30 seconds.
func TestPerformance_SC001_Import5K(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Generate 5K individual GEDCOM
	gedcomData := generateLargeGEDCOM(5000)
	t.Logf("Generated GEDCOM size: %d bytes", len(gedcomData))

	// Setup stores
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()

	// Setup command handler
	handler := command.NewHandler(eventStore, readStore)

	ctx := context.Background()

	// Time the import
	start := time.Now()

	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "benchmark.ged",
		FileSize: int64(len(gedcomData)),
		Reader:   bytes.NewReader(gedcomData),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	elapsed := time.Since(start)

	t.Logf("Imported %d persons, %d families in %v", result.PersonsImported, result.FamiliesImported, elapsed)
	if len(result.Warnings) > 0 {
		t.Logf("Warnings: %d", len(result.Warnings))
	}
	if len(result.Errors) > 0 {
		t.Logf("Errors: %d", len(result.Errors))
	}

	// SC-001: Must complete within 30 seconds
	if elapsed > 30*time.Second {
		t.Errorf("SC-001 FAILED: Import took %v, expected < 30s", elapsed)
	} else {
		t.Logf("SC-001 PASSED: Import took %v", elapsed)
	}
}

// TestPerformance_SC005_Search10K tests SC-005: Search within 1 second for 10K tree.
func TestPerformance_SC005_Search10K(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Generate 10K individual GEDCOM and import
	gedcomData := generateLargeGEDCOM(10000)
	t.Logf("Generated GEDCOM size: %d bytes", len(gedcomData))

	// Setup stores
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)

	ctx := context.Background()

	// Import
	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "benchmark.ged",
		FileSize: int64(len(gedcomData)),
		Reader:   bytes.NewReader(gedcomData),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	t.Logf("Imported %d persons", result.PersonsImported)

	// Setup query service
	personService := query.NewPersonService(readStore)

	// Test search performance
	searchQueries := []string{
		"Person1",      // Common prefix
		"TestFamily50", // Surname search
		"Person5000",   // Specific name
	}

	for _, q := range searchQueries {
		start := time.Now()
		searchResult, err := personService.SearchPersons(ctx, query.SearchPersonsInput{
			Query: q,
			Fuzzy: false,
			Limit: 50,
		})
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("search '%s' failed: %v", q, err)
			continue
		}

		t.Logf("Search '%s': %d results in %v", q, len(searchResult.Items), elapsed)

		// SC-005: Must complete within 1 second
		if elapsed > 1*time.Second {
			t.Errorf("SC-005 FAILED: Search '%s' took %v, expected < 1s", q, elapsed)
		}
	}
	t.Log("SC-005 PASSED: All searches completed within 1 second")
}

// TestPerformance_SC006_Export10K tests SC-006: Export within 30 seconds for 10K tree.
func TestPerformance_SC006_Export10K(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Generate 10K individual GEDCOM and import
	gedcomData := generateLargeGEDCOM(10000)

	// Setup stores
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)

	ctx := context.Background()

	// Import
	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "benchmark.ged",
		FileSize: int64(len(gedcomData)),
		Reader:   bytes.NewReader(gedcomData),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	t.Logf("Imported %d persons, %d families", result.PersonsImported, result.FamiliesImported)

	// Setup exporter
	exporter := gedcom.NewExporter(readStore)

	// Time the export
	start := time.Now()

	var buf bytes.Buffer
	exportResult, err := exporter.Export(ctx, &buf)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	t.Logf("SC-006: Export completed in %v (limit: 30s), output size: %d bytes, %d persons, %d families",
		elapsed, buf.Len(), exportResult.PersonsExported, exportResult.FamiliesExported)

	// SC-006: Must complete within 30 seconds
	if elapsed > 30*time.Second {
		t.Errorf("SC-006 FAILED: Export took %v, expected < 30s", elapsed)
	} else {
		t.Logf("SC-006 PASSED: Export took %v", elapsed)
	}
}

// BenchmarkImport1K benchmarks importing 1K individuals.
func BenchmarkImport1K(b *testing.B) {
	gedcomData := generateLargeGEDCOM(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventStore := memory.NewEventStore()
		readStore := memory.NewReadModelStore()
		handler := command.NewHandler(eventStore, readStore)

		_, err := handler.ImportGedcom(context.Background(), command.ImportGedcomInput{
			Filename: "bench.ged",
			FileSize: int64(len(gedcomData)),
			Reader:   bytes.NewReader(gedcomData),
		})
		if err != nil {
			b.Fatalf("import: %v", err)
		}
	}
}

// BenchmarkSearch runs search benchmark.
func BenchmarkSearch(b *testing.B) {
	// Setup with 5K individuals
	gedcomData := generateLargeGEDCOM(5000)
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)

	handler.ImportGedcom(context.Background(), command.ImportGedcomInput{
		Filename: "bench.ged",
		FileSize: int64(len(gedcomData)),
		Reader:   bytes.NewReader(gedcomData),
	})

	personService := query.NewPersonService(readStore)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		personService.SearchPersons(ctx, query.SearchPersonsInput{
			Query: "Person100",
			Fuzzy: false,
			Limit: 50,
		})
	}
}
