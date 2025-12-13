package gedcom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
)

const sampleGedcom = `0 HEAD
1 SOUR Test
1 GEDC
2 VERS 5.5
2 FORM LINEAGE-LINKED
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 DATE 15 JAN 1850
2 PLAC Springfield, IL
1 DEAT
2 DATE 20 MAR 1920
2 PLAC Chicago, IL
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
1 BIRT
2 DATE ABT 1855
2 PLAC Boston, MA
0 @I3@ INDI
1 NAME Junior /Doe/
1 SEX M
1 BIRT
2 DATE 1880
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 CHIL @I3@
1 MARR
2 DATE 10 JUN 1875
2 PLAC Springfield, IL
0 TRLR
`

func TestImportBasicGedcom(t *testing.T) {
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, persons, families, err := importer.Import(ctx, strings.NewReader(sampleGedcom))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.PersonsImported != 3 {
		t.Errorf("PersonsImported = %d, want 3", result.PersonsImported)
	}
	if result.FamiliesImported != 1 {
		t.Errorf("FamiliesImported = %d, want 1", result.FamiliesImported)
	}

	// Verify persons
	if len(persons) != 3 {
		t.Fatalf("len(persons) = %d, want 3", len(persons))
	}

	// Find John Doe
	var john *gedcom.PersonData
	for i := range persons {
		if persons[i].GivenName == "John" && persons[i].Surname == "Doe" {
			john = &persons[i]
			break
		}
	}
	if john == nil {
		t.Fatal("John Doe not found")
	}
	if john.Gender != domain.GenderMale {
		t.Errorf("John's gender = %s, want male", john.Gender)
	}
	if john.BirthDate != "15 JAN 1850" {
		t.Errorf("John's birth date = %s, want '15 JAN 1850'", john.BirthDate)
	}
	if john.BirthPlace != "Springfield, IL" {
		t.Errorf("John's birth place = %s, want 'Springfield, IL'", john.BirthPlace)
	}

	// Verify families
	if len(families) != 1 {
		t.Fatalf("len(families) = %d, want 1", len(families))
	}

	fam := families[0]
	if fam.Partner1ID == nil {
		t.Error("Family partner1 should be set")
	}
	if fam.Partner2ID == nil {
		t.Error("Family partner2 should be set")
	}
	if fam.MarriageDate != "10 JUN 1875" {
		t.Errorf("Marriage date = %s, want '10 JUN 1875'", fam.MarriageDate)
	}
	if len(fam.ChildIDs) != 1 {
		t.Errorf("len(ChildIDs) = %d, want 1", len(fam.ChildIDs))
	}
}

func TestImportApproximateDates(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Test /Person/
1 BIRT
2 DATE ABT 1850
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}
	if persons[0].BirthDate != "ABT 1850" {
		t.Errorf("BirthDate = %s, want 'ABT 1850'", persons[0].BirthDate)
	}
}

func TestImportMissingNames(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 SEX M
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, persons, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	// Given name should default to Unknown, surname can be empty
	if persons[0].GivenName != "Unknown" {
		t.Errorf("GivenName = %s, want 'Unknown'", persons[0].GivenName)
	}
	if persons[0].Surname != "" {
		t.Errorf("Surname = %s, want empty string", persons[0].Surname)
	}

	// Should have warnings
	if len(result.Warnings) == 0 {
		t.Error("Expected warnings for missing name")
	}
}

func TestImportSingleParentFamily(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Jane /Doe/
1 SEX F
0 @I2@ INDI
1 NAME Child /Doe/
0 @F1@ FAM
1 WIFE @I1@
1 CHIL @I2@
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, families, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(families) != 1 {
		t.Fatalf("len(families) = %d, want 1", len(families))
	}

	fam := families[0]
	if fam.Partner1ID != nil {
		t.Error("Partner1 should be nil for single-mother family")
	}
	if fam.Partner2ID == nil {
		t.Error("Partner2 (wife) should be set")
	}
	if len(fam.ChildIDs) != 1 {
		t.Errorf("len(ChildIDs) = %d, want 1", len(fam.ChildIDs))
	}
}

func TestImportMissingReference(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Test /Person/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I999@
1 CHIL @I998@
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, families, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have warnings for missing references
	if len(result.Warnings) < 2 {
		t.Errorf("Expected at least 2 warnings, got %d", len(result.Warnings))
	}

	// Family should still be created with partial data
	if len(families) != 1 {
		t.Fatalf("len(families) = %d, want 1", len(families))
	}
	if families[0].Partner1ID == nil {
		t.Error("Partner1 should be set (valid reference)")
	}
	if families[0].Partner2ID != nil {
		t.Error("Partner2 should be nil (invalid reference)")
	}
}

func TestImportDateRanges(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Test /Person/
1 BIRT
2 DATE BET 1850 AND 1860
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if persons[0].BirthDate != "BET 1850 AND 1860" {
		t.Errorf("BirthDate = %s, want 'BET 1850 AND 1860'", persons[0].BirthDate)
	}
}

func TestValidateImportData(t *testing.T) {
	// Empty data should fail
	err := gedcom.ValidateImportData(nil, nil)
	if err == nil {
		t.Error("Expected error for empty import data")
	}

	// Some data should pass
	persons := []gedcom.PersonData{{GivenName: "Test", Surname: "Person"}}
	err = gedcom.ValidateImportData(persons, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
