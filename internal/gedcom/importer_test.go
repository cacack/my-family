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

	result, persons, families, _, _, _, err := importer.Import(ctx, strings.NewReader(sampleGedcom))
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

	_, persons, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	result, persons, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	_, _, families, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	result, _, families, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	_, persons, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

func TestImportSources(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL 1850 US Census
1 AUTH US Census Bureau
1 PUBL Government Printing Office
1 NOTE This is a test source
0 @I1@ INDI
1 NAME John /Doe/
1 BIRT
2 DATE 1850
2 SOUR @S1@
3 PAGE 123
3 QUAY 3
3 DATA
4 TEXT Born January 1850
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, persons, _, sources, citations, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify source import
	if result.SourcesImported != 1 {
		t.Errorf("SourcesImported = %d, want 1", result.SourcesImported)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}

	src := sources[0]
	if src.Title != "1850 US Census" {
		t.Errorf("Source title = %s, want '1850 US Census'", src.Title)
	}
	if src.Author != "US Census Bureau" {
		t.Errorf("Source author = %s, want 'US Census Bureau'", src.Author)
	}
	if src.Publisher != "Government Printing Office" {
		t.Errorf("Source publisher = %s, want 'Government Printing Office'", src.Publisher)
	}
	if src.Notes != "This is a test source" {
		t.Errorf("Source notes = %s, want 'This is a test source'", src.Notes)
	}

	// Verify citation import
	if result.CitationsImported != 1 {
		t.Errorf("CitationsImported = %d, want 1", result.CitationsImported)
	}
	if len(citations) != 1 {
		t.Fatalf("len(citations) = %d, want 1", len(citations))
	}

	cit := citations[0]
	if cit.SourceXref != "@S1@" {
		t.Errorf("Citation source xref = %s, want '@S1@'", cit.SourceXref)
	}
	if cit.FactType != string(domain.FactPersonBirth) {
		t.Errorf("Citation fact type = %s, want '%s'", cit.FactType, domain.FactPersonBirth)
	}
	if cit.FactOwnerID != persons[0].ID {
		t.Error("Citation should be linked to person")
	}
	if cit.Page != "123" {
		t.Errorf("Citation page = %s, want '123'", cit.Page)
	}
	if cit.Quality != "direct" {
		t.Errorf("Citation quality = %s, want 'direct' (from QUAY 3)", cit.Quality)
	}
	if cit.QuotedText != "Born January 1850" {
		t.Errorf("Citation quoted text = %s, want 'Born January 1850'", cit.QuotedText)
	}
}

func TestImportCitationsForMultipleEvents(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL Birth Certificate
0 @S2@ SOUR
1 TITL Death Certificate
0 @I1@ INDI
1 NAME Jane /Smith/
1 BIRT
2 DATE 1860
2 SOUR @S1@
3 PAGE Birth page
1 DEAT
2 DATE 1940
2 SOUR @S2@
3 PAGE Death page
3 QUAY 2
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, _, sources, citations, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(sources) != 2 {
		t.Fatalf("len(sources) = %d, want 2", len(sources))
	}
	if len(citations) != 2 {
		t.Fatalf("len(citations) = %d, want 2", len(citations))
	}

	// Find birth citation
	var birthCit, deathCit *gedcom.CitationData
	for i := range citations {
		if citations[i].FactType == string(domain.FactPersonBirth) {
			birthCit = &citations[i]
		} else if citations[i].FactType == string(domain.FactPersonDeath) {
			deathCit = &citations[i]
		}
	}

	if birthCit == nil {
		t.Fatal("Birth citation not found")
	}
	if birthCit.Page != "Birth page" {
		t.Errorf("Birth citation page = %s, want 'Birth page'", birthCit.Page)
	}

	if deathCit == nil {
		t.Fatal("Death citation not found")
	}
	if deathCit.Page != "Death page" {
		t.Errorf("Death citation page = %s, want 'Death page'", deathCit.Page)
	}
	if deathCit.Quality != "secondary" {
		t.Errorf("Death citation quality = %s, want 'secondary' (from QUAY 2)", deathCit.Quality)
	}
}

func TestImportFamilyCitations(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL Marriage Record
0 @I1@ INDI
1 NAME John /Doe/
0 @I2@ INDI
1 NAME Jane /Smith/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1880
2 SOUR @S1@
3 PAGE Marriage register, page 45
3 QUAY 3
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, families, sources, citations, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if len(citations) != 1 {
		t.Fatalf("len(citations) = %d, want 1", len(citations))
	}

	cit := citations[0]
	if cit.FactType != string(domain.FactFamilyMarriage) {
		t.Errorf("Citation fact type = %s, want '%s'", cit.FactType, domain.FactFamilyMarriage)
	}
	if cit.FactOwnerID != families[0].ID {
		t.Error("Citation should be linked to family")
	}
	if cit.Page != "Marriage register, page 45" {
		t.Errorf("Citation page = %s, want 'Marriage register, page 45'", cit.Page)
	}
}

func TestImportNameComponents(t *testing.T) {
	// Test GEDCOM with full name components
	gedcomData := `0 HEAD
1 SOUR Test
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Dr. John Fitzgerald /von Doe/ Jr.
2 NPFX Dr.
2 GIVN John Fitzgerald
2 SPFX von
2 SURN Doe
2 NSFX Jr.
2 NICK Jack
2 TYPE birth
1 SEX M
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	p := persons[0]
	if p.GivenName != "John Fitzgerald" {
		t.Errorf("GivenName = %q, want %q", p.GivenName, "John Fitzgerald")
	}
	if p.Surname != "Doe" {
		t.Errorf("Surname = %q, want %q", p.Surname, "Doe")
	}
	if p.NamePrefix != "Dr." {
		t.Errorf("NamePrefix = %q, want %q", p.NamePrefix, "Dr.")
	}
	if p.NameSuffix != "Jr." {
		t.Errorf("NameSuffix = %q, want %q", p.NameSuffix, "Jr.")
	}
	if p.SurnamePrefix != "von" {
		t.Errorf("SurnamePrefix = %q, want %q", p.SurnamePrefix, "von")
	}
	if p.Nickname != "Jack" {
		t.Errorf("Nickname = %q, want %q", p.Nickname, "Jack")
	}
	if p.NameType != domain.NameTypeBirth {
		t.Errorf("NameType = %q, want %q", p.NameType, domain.NameTypeBirth)
	}
}

func TestImportPedigreeTypes(t *testing.T) {
	// Test GEDCOM with PEDI in FAMC links
	gedcomData := `0 HEAD
1 SOUR Test
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
0 @I3@ INDI
1 NAME Bio /Doe/
1 SEX M
1 FAMC @F1@
2 PEDI birth
0 @I4@ INDI
1 NAME Adopted /Doe/
1 SEX F
1 FAMC @F1@
2 PEDI adopted
0 @I5@ INDI
1 NAME Foster /Doe/
1 SEX M
1 FAMC @F1@
2 PEDI foster
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 CHIL @I3@
1 CHIL @I4@
1 CHIL @I5@
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, families, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(families) != 1 {
		t.Fatalf("len(families) = %d, want 1", len(families))
	}

	fam := families[0]
	if len(fam.ChildIDs) != 3 {
		t.Fatalf("len(ChildIDs) = %d, want 3", len(fam.ChildIDs))
	}
	if len(fam.ChildRelTypes) != 3 {
		t.Fatalf("len(ChildRelTypes) = %d, want 3", len(fam.ChildRelTypes))
	}

	// Verify the relationship types
	if fam.ChildRelTypes[0] != domain.ChildBiological {
		t.Errorf("First child rel type = %q, want %q", fam.ChildRelTypes[0], domain.ChildBiological)
	}
	if fam.ChildRelTypes[1] != domain.ChildAdopted {
		t.Errorf("Second child rel type = %q, want %q", fam.ChildRelTypes[1], domain.ChildAdopted)
	}
	if fam.ChildRelTypes[2] != domain.ChildFoster {
		t.Errorf("Third child rel type = %q, want %q", fam.ChildRelTypes[2], domain.ChildFoster)
	}
}

func TestImportRepositories(t *testing.T) {
	// Test GEDCOM with repository
	gedcomData := `0 HEAD
1 SOUR Test
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @R1@ REPO
1 NAME Family History Library
1 ADDR 35 N West Temple St
2 CITY Salt Lake City
2 STAE UT
2 POST 84150
2 CTRY USA
0 @S1@ SOUR
1 TITL Birth Certificate
1 REPO @R1@
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, _, sources, _, repositories, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.RepositoriesImported != 1 {
		t.Errorf("RepositoriesImported = %d, want 1", result.RepositoriesImported)
	}

	if len(repositories) != 1 {
		t.Fatalf("len(repositories) = %d, want 1", len(repositories))
	}

	repo := repositories[0]
	if repo.Name != "Family History Library" {
		t.Errorf("Repository name = %q, want %q", repo.Name, "Family History Library")
	}
	if repo.GedcomXref != "@R1@" {
		t.Errorf("Repository xref = %q, want %q", repo.GedcomXref, "@R1@")
	}
	if repo.City != "Salt Lake City" {
		t.Errorf("Repository city = %q, want %q", repo.City, "Salt Lake City")
	}
	if repo.State != "UT" {
		t.Errorf("Repository state = %q, want %q", repo.State, "UT")
	}

	// Verify source is linked to repository
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	src := sources[0]
	if src.RepositoryID == nil {
		t.Error("Source should be linked to repository")
	} else if *src.RepositoryID != repo.ID {
		t.Errorf("Source repository ID = %v, want %v", *src.RepositoryID, repo.ID)
	}
}
