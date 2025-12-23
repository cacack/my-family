package gedcom_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestIntegration_ImportExportRoundTrip(t *testing.T) {
	testFiles := []struct {
		name        string
		path        string
		minPersons  int
		minFamilies int
		checkNames  []string // Names that should survive round-trip
	}{
		{
			name:        "minimal",
			path:        "../../testdata/gedcom-5.5/minimal.ged",
			minPersons:  1,
			minFamilies: 0,
			checkNames:  []string{"John", "Doe"},
		},
		{
			name:        "comprehensive",
			path:        "../../testdata/gedcom-5.5/comprehensive.ged",
			minPersons:  13,
			minFamilies: 4,
			checkNames:  []string{"John", "Smith", "Mary", "Johnson", "Robert", "Emily", "Adopted"},
		},
	}

	for _, tc := range testFiles {
		t.Run(tc.name, func(t *testing.T) {
			// Skip if file doesn't exist
			absPath, _ := filepath.Abs(tc.path)
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				t.Skipf("Test file not found: %s", absPath)
			}

			// Read the file
			data, err := os.ReadFile(absPath)
			if err != nil {
				t.Fatalf("Failed to read test file: %v", err)
			}

			ctx := context.Background()

			// Import
			importer := gedcom.NewImporter()
			result, persons, families, _, _, err := importer.Import(ctx, bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Import failed: %v", err)
			}

			if result.PersonsImported < tc.minPersons {
				t.Errorf("PersonsImported = %d, want >= %d", result.PersonsImported, tc.minPersons)
			}
			if result.FamiliesImported < tc.minFamilies {
				t.Errorf("FamiliesImported = %d, want >= %d", result.FamiliesImported, tc.minFamilies)
			}

			// Store in read model (simulating what the command handler does)
			readStore := memory.NewReadModelStore()
			for _, p := range persons {
				pm := &repository.PersonReadModel{
					ID:           p.ID,
					GivenName:    p.GivenName,
					Surname:      p.Surname,
					FullName:     p.GivenName + " " + p.Surname,
					Gender:       p.Gender,
					BirthDateRaw: p.BirthDate,
					BirthPlace:   p.BirthPlace,
					DeathDateRaw: p.DeathDate,
					DeathPlace:   p.DeathPlace,
					Notes:        p.Notes,
				}
				if err := readStore.SavePerson(ctx, pm); err != nil {
					t.Fatalf("Failed to save person: %v", err)
				}
			}

			for _, f := range families {
				fm := &repository.FamilyReadModel{
					ID:               f.ID,
					RelationshipType: f.RelationshipType,
					MarriageDateRaw:  f.MarriageDate,
					MarriagePlace:    f.MarriagePlace,
				}
				if f.Partner1ID != nil {
					fm.Partner1ID = f.Partner1ID
				}
				if f.Partner2ID != nil {
					fm.Partner2ID = f.Partner2ID
				}
				if err := readStore.SaveFamily(ctx, fm); err != nil {
					t.Fatalf("Failed to save family: %v", err)
				}
			}

			// Export
			exporter := gedcom.NewExporter(readStore)
			buf := &bytes.Buffer{}
			exportResult, err := exporter.Export(ctx, buf)
			if err != nil {
				t.Fatalf("Export failed: %v", err)
			}

			if exportResult.PersonsExported != result.PersonsImported {
				t.Errorf("Export count mismatch: exported %d, imported %d",
					exportResult.PersonsExported, result.PersonsImported)
			}

			// Verify GEDCOM structure
			output := buf.String()
			if !strings.HasPrefix(output, "0 HEAD\n") {
				t.Error("Export should start with GEDCOM header")
			}
			if !strings.HasSuffix(output, "0 TRLR\n") {
				t.Error("Export should end with TRLR")
			}

			// Check that key names survived round-trip
			for _, name := range tc.checkNames {
				if !strings.Contains(output, name) {
					t.Errorf("Expected name '%s' not found in export", name)
				}
			}
		})
	}
}

func TestIntegration_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	path := "../../testdata/gedcom-5.5/royal92.ged"
	absPath, _ := filepath.Abs(path)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		t.Skipf("Test file not found: %s", absPath)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	ctx := context.Background()
	importer := gedcom.NewImporter()
	result, persons, families, _, _, err := importer.Import(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// royal92.ged has ~3000 individuals and ~1400 families
	if result.PersonsImported < 3000 {
		t.Errorf("PersonsImported = %d, want >= 3000", result.PersonsImported)
	}
	if result.FamiliesImported < 1400 {
		t.Errorf("FamiliesImported = %d, want >= 1400", result.FamiliesImported)
	}

	// Verify some expected names exist
	foundVictoria := false
	foundElizabeth := false
	for _, p := range persons {
		if strings.Contains(p.GivenName, "Victoria") {
			foundVictoria = true
		}
		if strings.Contains(p.GivenName, "Elizabeth") {
			foundElizabeth = true
		}
	}
	if !foundVictoria {
		t.Error("Expected to find 'Victoria' in royal92.ged")
	}
	if !foundElizabeth {
		t.Error("Expected to find 'Elizabeth' in royal92.ged")
	}

	// Verify families have relationships
	marriageCount := 0
	for _, f := range families {
		if f.RelationshipType == domain.RelationMarriage {
			marriageCount++
		}
	}
	if marriageCount < 500 {
		t.Errorf("Expected at least 500 marriages, got %d", marriageCount)
	}
}
