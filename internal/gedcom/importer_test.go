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

	result, persons, families, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(sampleGedcom))
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

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	result, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	_, _, families, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	result, _, families, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	result, persons, _, sources, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	_, _, _, sources, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	_, _, families, sources, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	_, _, families, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

	result, _, _, sources, _, repositories, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
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

// Note: The current importer does not yet return event/attribute data structures.
// These tests verify that citations are extracted from life events.
// Future enhancement: Return EventData and AttributeData from importer.

func TestImportBurialEvent(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL Cemetery Records
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 DATE 15 JAN 1850
1 DEAT
2 DATE 20 MAR 1920
1 BURI
2 DATE 23 MAR 1920
2 PLAC Springfield Cemetery, IL
2 SOUR @S1@
3 PAGE Plot 42
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, persons, _, sources, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.PersonsImported != 1 {
		t.Errorf("PersonsImported = %d, want 1", result.PersonsImported)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	// Verify person basic data
	person := persons[0]
	if person.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", person.GivenName)
	}
	if person.BirthDate != "15 JAN 1850" {
		t.Errorf("BirthDate = %s, want '15 JAN 1850'", person.BirthDate)
	}

	// Verify source was imported
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}

	// Note: Current importer extracts citations only from BIRT/DEAT events.
	// Burial event citations require enhancement to extractCitationsFromIndividual.
	_ = citations // Acknowledgement that citations exist but burial not yet tracked
}

func TestImportBaptismEvent(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Mary /Smith/
1 SEX F
1 BIRT
2 DATE 1 JAN 1855
1 BAPM
2 DATE 15 JAN 1855
2 PLAC First Presbyterian Church, Boston, MA
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	person := persons[0]
	if person.GivenName != "Mary" {
		t.Errorf("GivenName = %s, want Mary", person.GivenName)
	}
	if person.BirthDate != "1 JAN 1855" {
		t.Errorf("BirthDate = %s, want '1 JAN 1855'", person.BirthDate)
	}
}

func TestImportCensusEvent(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Robert /Jones/
1 SEX M
1 BIRT
2 DATE 1840
1 CENS
2 DATE 1880
2 PLAC Springfield, IL
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	person := persons[0]
	if person.GivenName != "Robert" {
		t.Errorf("GivenName = %s, want Robert", person.GivenName)
	}
}

func TestImportImmigrationEvent(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Hans /Mueller/
1 SEX M
1 BIRT
2 DATE 1850
2 PLAC Berlin, Germany
1 IMMI
2 DATE 1880
2 PLAC Ellis Island, NY
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	person := persons[0]
	if person.GivenName != "Hans" {
		t.Errorf("GivenName = %s, want Hans", person.GivenName)
	}
	if person.BirthPlace != "Berlin, Germany" {
		t.Errorf("BirthPlace = %s, want 'Berlin, Germany'", person.BirthPlace)
	}
}

func TestImportOccupationAttribute(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME William /Taylor/
1 SEX M
1 BIRT
2 DATE 1830
1 OCCU Blacksmith
2 DATE 1860
2 PLAC Springfield, IL
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	person := persons[0]
	if person.GivenName != "William" {
		t.Errorf("GivenName = %s, want William", person.GivenName)
	}
}

func TestImportResidenceAttribute(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Sarah /Brown/
1 SEX F
1 BIRT
2 DATE 1840
1 RESI
2 DATE 1870
2 PLAC 123 Main St, Springfield, IL
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	person := persons[0]
	if person.GivenName != "Sarah" {
		t.Errorf("GivenName = %s, want Sarah", person.GivenName)
	}
}

func TestImportFamilyMarriageBann(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARB
2 DATE 1 MAY 1875
2 PLAC First Church, Springfield, IL
1 MARR
2 DATE 1 JUN 1875
2 PLAC First Church, Springfield, IL
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, families, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(families) != 1 {
		t.Fatalf("len(families) = %d, want 1", len(families))
	}

	family := families[0]
	if family.MarriageDate != "1 JUN 1875" {
		t.Errorf("MarriageDate = %s, want '1 JUN 1875'", family.MarriageDate)
	}
	if family.MarriagePlace != "First Church, Springfield, IL" {
		t.Errorf("MarriagePlace = %s, want 'First Church, Springfield, IL'", family.MarriagePlace)
	}
}

func TestImportFamilyAnnulment(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1 JUN 1875
1 ANUL
2 DATE 1 JAN 1876
2 PLAC County Court, Springfield, IL
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, families, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(families) != 1 {
		t.Fatalf("len(families) = %d, want 1", len(families))
	}

	family := families[0]
	if family.RelationshipType != domain.RelationMarriage {
		t.Errorf("RelationshipType = %s, want %s", family.RelationshipType, domain.RelationMarriage)
	}
}

func TestImportMultipleEventsAndAttributes(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL Birth Records
0 @S2@ SOUR
1 TITL Census Records
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 DATE 1 JAN 1850
2 PLAC Springfield, IL
2 SOUR @S1@
3 PAGE 123
1 BAPM
2 DATE 15 JAN 1850
2 PLAC First Church
1 CENS
2 DATE 1880
2 PLAC Springfield, IL
2 SOUR @S2@
3 PAGE 456
1 OCCU Farmer
2 DATE 1880
1 RESI
2 DATE 1880
2 PLAC 100 Main St
1 DEAT
2 DATE 20 MAR 1920
2 PLAC Chicago, IL
1 BURI
2 DATE 23 MAR 1920
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, persons, _, sources, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify counts
	if result.PersonsImported != 1 {
		t.Errorf("PersonsImported = %d, want 1", result.PersonsImported)
	}
	if result.SourcesImported != 2 {
		t.Errorf("SourcesImported = %d, want 2", result.SourcesImported)
	}

	// Verify person
	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}
	person := persons[0]
	if person.BirthDate != "1 JAN 1850" {
		t.Errorf("BirthDate = %s, want '1 JAN 1850'", person.BirthDate)
	}
	if person.DeathDate != "20 MAR 1920" {
		t.Errorf("DeathDate = %s, want '20 MAR 1920'", person.DeathDate)
	}

	// Verify sources
	if len(sources) != 2 {
		t.Fatalf("len(sources) = %d, want 2", len(sources))
	}

	// Verify citations (birth and census events have citations)
	if len(citations) != 2 {
		t.Errorf("len(citations) = %d, want 2 (birth and census)", len(citations))
	}

	// Find birth citation
	var birthCit, censusCit *gedcom.CitationData
	for i := range citations {
		switch citations[i].FactType {
		case string(domain.FactPersonBirth):
			birthCit = &citations[i]
		case string(domain.FactPersonCensus):
			censusCit = &citations[i]
		}
	}
	if birthCit == nil {
		t.Fatal("Birth citation not found")
	}
	if birthCit.Page != "123" {
		t.Errorf("Birth citation page = %s, want '123'", birthCit.Page)
	}
	if censusCit == nil {
		t.Fatal("Census citation not found")
	}
	if censusCit.Page != "456" {
		t.Errorf("Census citation page = %s, want '456'", censusCit.Page)
	}
}

func TestImportChristeningEvent(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME James /Wilson/
1 SEX M
1 BIRT
2 DATE 1 DEC 1860
1 CHR
2 DATE 25 DEC 1860
2 PLAC St. Paul's Cathedral
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	person := persons[0]
	if person.GivenName != "James" {
		t.Errorf("GivenName = %s, want James", person.GivenName)
	}
}

func TestImportEmigrationEvent(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Patrick /O'Brien/
1 SEX M
1 BIRT
2 DATE 1845
2 PLAC Dublin, Ireland
1 EMIG
2 DATE 1860
2 PLAC Liverpool, England
1 IMMI
2 DATE 1860
2 PLAC Boston, MA
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	person := persons[0]
	if person.BirthPlace != "Dublin, Ireland" {
		t.Errorf("BirthPlace = %s, want 'Dublin, Ireland'", person.BirthPlace)
	}
}

func TestImportNaturalizationEvent(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Giuseppe /Romano/
1 SEX M
1 BIRT
2 DATE 1850
2 PLAC Naples, Italy
1 IMMI
2 DATE 1875
2 PLAC New York, NY
1 NATU
2 DATE 1880
2 PLAC Federal Court, New York, NY
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	person := persons[0]
	if person.GivenName != "Giuseppe" {
		t.Errorf("GivenName = %s, want Giuseppe", person.GivenName)
	}
}

func TestImportEventsExtraction(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 DATE 1 JAN 1850
2 PLAC Springfield, IL
1 BAPM
2 DATE 15 JAN 1850
2 PLAC First Church
1 CENS
2 DATE 1880
2 PLAC Springfield, IL
1 EMIG
2 DATE 1890
2 PLAC New York, NY
1 IMMI
2 DATE 1890
2 PLAC Liverpool, England
1 BURI
2 DATE 25 DEC 1920
2 PLAC Springfield Cemetery
2 CAUS Heart failure
1 DEAT
2 DATE 20 DEC 1920
2 PLAC Chicago, IL
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, _, _, _, _, events, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have 5 events (BIRT and DEAT stored on person, so we have BAPM, CENS, EMIG, IMMI, BURI)
	if result.EventsImported != 5 {
		t.Errorf("EventsImported = %d, want 5", result.EventsImported)
	}
	if len(events) != 5 {
		t.Fatalf("len(events) = %d, want 5", len(events))
	}

	// Verify event types
	eventTypes := make(map[domain.FactType]bool)
	for _, e := range events {
		eventTypes[e.FactType] = true
	}

	expectedTypes := []domain.FactType{
		domain.FactPersonBaptism,
		domain.FactPersonCensus,
		domain.FactPersonEmigration,
		domain.FactPersonImmigration,
		domain.FactPersonBurial,
	}
	for _, et := range expectedTypes {
		if !eventTypes[et] {
			t.Errorf("Missing event type: %s", et)
		}
	}

	// Verify burial has cause
	for _, e := range events {
		if e.FactType == domain.FactPersonBurial {
			if e.Cause != "Heart failure" {
				t.Errorf("Burial cause = %q, want %q", e.Cause, "Heart failure")
			}
			if e.Place != "Springfield Cemetery" {
				t.Errorf("Burial place = %q, want %q", e.Place, "Springfield Cemetery")
			}
		}
	}
}

func TestImportAttributesExtraction(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME William /Taylor/
1 SEX M
1 BIRT
2 DATE 1830
1 OCCU Blacksmith
2 DATE 1860
2 PLAC Springfield, IL
1 RESI
2 DATE 1870
2 PLAC 123 Main St, Springfield, IL
1 EDUC College Graduate
1 RELI Methodist
1 TITL Esquire
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, _, _, _, _, _, attributes, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have 5 attributes (OCCU, RESI as events, plus EDUC, RELI, TITL as tags)
	if result.AttributesImported != 5 {
		t.Errorf("AttributesImported = %d, want 5", result.AttributesImported)
	}
	if len(attributes) != 5 {
		t.Fatalf("len(attributes) = %d, want 5", len(attributes))
	}

	// Verify attribute types
	attrTypes := make(map[domain.FactType]string)
	for _, a := range attributes {
		attrTypes[a.FactType] = a.Value
	}

	if attrTypes[domain.FactPersonOccupation] != "Blacksmith" {
		t.Errorf("Occupation = %q, want %q", attrTypes[domain.FactPersonOccupation], "Blacksmith")
	}
	if attrTypes[domain.FactPersonEducation] != "College Graduate" {
		t.Errorf("Education = %q, want %q", attrTypes[domain.FactPersonEducation], "College Graduate")
	}
	if attrTypes[domain.FactPersonReligion] != "Methodist" {
		t.Errorf("Religion = %q, want %q", attrTypes[domain.FactPersonReligion], "Methodist")
	}
	if attrTypes[domain.FactPersonTitle] != "Esquire" {
		t.Errorf("Title = %q, want %q", attrTypes[domain.FactPersonTitle], "Esquire")
	}
}

func TestImportFamilyEventsExtraction(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 ENGA
2 DATE 1 JAN 1875
2 PLAC Springfield, IL
1 MARB
2 DATE 1 MAY 1875
2 PLAC First Church
1 MARL
2 DATE 15 MAY 1875
2 PLAC County Clerk
1 MARR
2 DATE 1 JUN 1875
2 PLAC First Church
1 DIV
2 DATE 1 JAN 1880
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, _, _, _, _, events, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have 4 family events (MARR stored on family, so we have ENGA, MARB, MARL, DIV)
	if result.EventsImported != 4 {
		t.Errorf("EventsImported = %d, want 4", result.EventsImported)
	}

	// Verify event types
	eventTypes := make(map[domain.FactType]bool)
	for _, e := range events {
		eventTypes[e.FactType] = true
		if e.OwnerType != "family" {
			t.Errorf("Event %s has OwnerType = %q, want %q", e.FactType, e.OwnerType, "family")
		}
	}

	expectedTypes := []domain.FactType{
		domain.FactFamilyEngagement,
		domain.FactFamilyMarriageBann,
		domain.FactFamilyMarriageLicense,
		domain.FactFamilyDivorce,
	}
	for _, et := range expectedTypes {
		if !eventTypes[et] {
			t.Errorf("Missing event type: %s", et)
		}
	}
}

func TestImportEventsCitations(t *testing.T) {
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL Church Records
0 @S2@ SOUR
1 TITL Cemetery Records
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BAPM
2 DATE 15 JAN 1850
2 PLAC First Church
2 SOUR @S1@
3 PAGE Baptism register, page 42
1 BURI
2 DATE 25 DEC 1920
2 PLAC Springfield Cemetery
2 SOUR @S2@
3 PAGE Plot 123
3 QUAY 3
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, _, _, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have 2 citations (one for baptism, one for burial)
	if len(citations) != 2 {
		t.Fatalf("len(citations) = %d, want 2", len(citations))
	}

	// Find citations by type
	var baptismCit, burialCit *gedcom.CitationData
	for i := range citations {
		switch citations[i].FactType {
		case string(domain.FactPersonBaptism):
			baptismCit = &citations[i]
		case string(domain.FactPersonBurial):
			burialCit = &citations[i]
		}
	}

	if baptismCit == nil {
		t.Fatal("Baptism citation not found")
	}
	if baptismCit.Page != "Baptism register, page 42" {
		t.Errorf("Baptism citation page = %q, want %q", baptismCit.Page, "Baptism register, page 42")
	}

	if burialCit == nil {
		t.Fatal("Burial citation not found")
	}
	if burialCit.Page != "Plot 123" {
		t.Errorf("Burial citation page = %q, want %q", burialCit.Page, "Plot 123")
	}
	if burialCit.Quality != "direct" {
		t.Errorf("Burial citation quality = %q, want %q", burialCit.Quality, "direct")
	}
}

func TestImportAncestryVendor(t *testing.T) {
	// Test GEDCOM file from Ancestry.com with _APID tags
	gedcomData := `0 HEAD
1 SOUR Ancestry.com
2 VERS 1.0
2 NAME Ancestry.com Family Trees
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Smith/
1 SEX M
1 BIRT
2 DATE 15 MAR 1850
2 PLAC Springfield, Illinois, USA
2 SOUR @S1@
3 PAGE Page 42
3 _APID 1,7602::2771226
1 DEAT
2 DATE 10 JUN 1920
2 SOUR @S2@
3 PAGE Entry 103
3 _APID 1,9024::12345678
0 @S1@ SOUR
1 TITL Illinois Deaths
0 @S2@ SOUR
1 TITL 1920 Census
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, _, _, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify vendor detection
	if result.Vendor != "ancestry" {
		t.Errorf("Vendor = %q, want %q", result.Vendor, "ancestry")
	}

	// Verify citations with AncestryAPID
	if len(citations) != 2 {
		t.Fatalf("len(citations) = %d, want 2", len(citations))
	}

	// Find birth citation
	var birthCit, deathCit *gedcom.CitationData
	for i := range citations {
		switch citations[i].FactType {
		case string(domain.FactPersonBirth):
			birthCit = &citations[i]
		case string(domain.FactPersonDeath):
			deathCit = &citations[i]
		}
	}

	if birthCit == nil {
		t.Fatal("Birth citation not found")
	}
	if birthCit.AncestryAPID == nil {
		t.Fatal("Birth citation AncestryAPID is nil")
	}
	if birthCit.AncestryAPID.Database != "7602" {
		t.Errorf("Birth AncestryAPID.Database = %q, want %q", birthCit.AncestryAPID.Database, "7602")
	}
	if birthCit.AncestryAPID.Record != "2771226" {
		t.Errorf("Birth AncestryAPID.Record = %q, want %q", birthCit.AncestryAPID.Record, "2771226")
	}
	if birthCit.AncestryAPID.RawValue != "1,7602::2771226" {
		t.Errorf("Birth AncestryAPID.RawValue = %q, want %q", birthCit.AncestryAPID.RawValue, "1,7602::2771226")
	}

	if deathCit == nil {
		t.Fatal("Death citation not found")
	}
	if deathCit.AncestryAPID == nil {
		t.Fatal("Death citation AncestryAPID is nil")
	}
	if deathCit.AncestryAPID.Database != "9024" {
		t.Errorf("Death AncestryAPID.Database = %q, want %q", deathCit.AncestryAPID.Database, "9024")
	}
	if deathCit.AncestryAPID.Record != "12345678" {
		t.Errorf("Death AncestryAPID.Record = %q, want %q", deathCit.AncestryAPID.Record, "12345678")
	}
}

func TestImportFamilySearchVendor(t *testing.T) {
	// Test GEDCOM file from FamilySearch with _FSFTID tags
	gedcomData := `0 HEAD
1 SOUR FamilySearch
2 VERS 3.0
2 NAME FamilySearch Family Tree
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 DATE 15 JAN 1850
1 _FSFTID KWCJ-QN7
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
1 _FSFTID ABCD-123
0 @I3@ INDI
1 NAME Child /Doe/
1 SEX M
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify vendor detection
	if result.Vendor != "familysearch" {
		t.Errorf("Vendor = %q, want %q", result.Vendor, "familysearch")
	}

	// Verify persons with FamilySearchID
	if len(persons) != 3 {
		t.Fatalf("len(persons) = %d, want 3", len(persons))
	}

	// Find persons by name
	var john, jane, child *gedcom.PersonData
	for i := range persons {
		switch persons[i].GivenName {
		case "John":
			john = &persons[i]
		case "Jane":
			jane = &persons[i]
		case "Child":
			child = &persons[i]
		}
	}

	if john == nil {
		t.Fatal("John not found")
	}
	if john.FamilySearchID != "KWCJ-QN7" {
		t.Errorf("John.FamilySearchID = %q, want %q", john.FamilySearchID, "KWCJ-QN7")
	}

	if jane == nil {
		t.Fatal("Jane not found")
	}
	if jane.FamilySearchID != "ABCD-123" {
		t.Errorf("Jane.FamilySearchID = %q, want %q", jane.FamilySearchID, "ABCD-123")
	}

	if child == nil {
		t.Fatal("Child not found")
	}
	if child.FamilySearchID != "" {
		t.Errorf("Child.FamilySearchID = %q, want empty string", child.FamilySearchID)
	}
}

func TestImportAncestryFamilyCitations(t *testing.T) {
	// Test Ancestry APID extraction from family events
	gedcomData := `0 HEAD
1 SOUR Ancestry.com
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
0 @S1@ SOUR
1 TITL Marriage Records
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 10 JUN 1875
2 SOUR @S1@
3 PAGE Marriage register, page 45
3 _APID 1,5678::87654321
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, _, _, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have 1 citation from marriage event
	if len(citations) != 1 {
		t.Fatalf("len(citations) = %d, want 1", len(citations))
	}

	cit := citations[0]
	if cit.FactType != string(domain.FactFamilyMarriage) {
		t.Errorf("Citation fact type = %q, want %q", cit.FactType, domain.FactFamilyMarriage)
	}
	if cit.AncestryAPID == nil {
		t.Fatal("Marriage citation AncestryAPID is nil")
	}
	if cit.AncestryAPID.Database != "5678" {
		t.Errorf("AncestryAPID.Database = %q, want %q", cit.AncestryAPID.Database, "5678")
	}
	if cit.AncestryAPID.Record != "87654321" {
		t.Errorf("AncestryAPID.Record = %q, want %q", cit.AncestryAPID.Record, "87654321")
	}
}

func TestImportUnknownVendor(t *testing.T) {
	// Test GEDCOM file from unknown vendor
	gedcomData := `0 HEAD
1 SOUR MyGenealogy
2 VERS 1.0
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Test /Person/
1 SEX M
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Unknown vendor should return empty string
	if result.Vendor != "" {
		t.Errorf("Vendor = %q, want empty string for unknown vendor", result.Vendor)
	}
}

func TestImportCitationWithoutAPID(t *testing.T) {
	// Test that citations without APID have nil AncestryAPID
	gedcomData := `0 HEAD
1 SOUR Test
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Test /Person/
1 SEX M
1 BIRT
2 DATE 1850
2 SOUR @S1@
3 PAGE Page 1
0 @S1@ SOUR
1 TITL Test Source
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, _, _, citations, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(citations) != 1 {
		t.Fatalf("len(citations) = %d, want 1", len(citations))
	}

	cit := citations[0]
	if cit.AncestryAPID != nil {
		t.Errorf("Citation AncestryAPID should be nil when no _APID tag is present")
	}
}

func TestImportMultipleNames(t *testing.T) {
	// Test GEDCOM with multiple name variants (birth name, married name, immigrant name)
	gedcomData := `0 HEAD
1 SOUR Test
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Maria /Schmidt/
2 TYPE birth
2 NICK Mitzi
1 NAME Mary /Smith/
2 TYPE married
1 NAME Mary Elizabeth /Smith/
2 TYPE aka
2 NPFX Mrs.
2 NSFX Sr.
1 SEX F
1 BIRT
2 DATE 1850
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	p := persons[0]

	// Primary name fields should be from first name
	if p.GivenName != "Maria" {
		t.Errorf("GivenName = %q, want %q", p.GivenName, "Maria")
	}
	if p.Surname != "Schmidt" {
		t.Errorf("Surname = %q, want %q", p.Surname, "Schmidt")
	}
	if p.NameType != domain.NameTypeBirth {
		t.Errorf("NameType = %q, want %q", p.NameType, domain.NameTypeBirth)
	}
	if p.Nickname != "Mitzi" {
		t.Errorf("Nickname = %q, want %q", p.Nickname, "Mitzi")
	}

	// Check all names are captured
	if len(p.Names) != 3 {
		t.Fatalf("len(p.Names) = %d, want 3", len(p.Names))
	}

	// First name should be primary
	if !p.Names[0].IsPrimary {
		t.Error("First name should be primary")
	}
	if p.Names[0].GivenName != "Maria" {
		t.Errorf("Names[0].GivenName = %q, want %q", p.Names[0].GivenName, "Maria")
	}
	if p.Names[0].Surname != "Schmidt" {
		t.Errorf("Names[0].Surname = %q, want %q", p.Names[0].Surname, "Schmidt")
	}
	if p.Names[0].NameType != domain.NameTypeBirth {
		t.Errorf("Names[0].NameType = %q, want %q", p.Names[0].NameType, domain.NameTypeBirth)
	}
	if p.Names[0].Nickname != "Mitzi" {
		t.Errorf("Names[0].Nickname = %q, want %q", p.Names[0].Nickname, "Mitzi")
	}

	// Second name (married name)
	if p.Names[1].IsPrimary {
		t.Error("Second name should not be primary")
	}
	if p.Names[1].GivenName != "Mary" {
		t.Errorf("Names[1].GivenName = %q, want %q", p.Names[1].GivenName, "Mary")
	}
	if p.Names[1].Surname != "Smith" {
		t.Errorf("Names[1].Surname = %q, want %q", p.Names[1].Surname, "Smith")
	}
	if p.Names[1].NameType != domain.NameTypeMarried {
		t.Errorf("Names[1].NameType = %q, want %q", p.Names[1].NameType, domain.NameTypeMarried)
	}

	// Third name (AKA)
	if p.Names[2].IsPrimary {
		t.Error("Third name should not be primary")
	}
	if p.Names[2].GivenName != "Mary Elizabeth" {
		t.Errorf("Names[2].GivenName = %q, want %q", p.Names[2].GivenName, "Mary Elizabeth")
	}
	if p.Names[2].Surname != "Smith" {
		t.Errorf("Names[2].Surname = %q, want %q", p.Names[2].Surname, "Smith")
	}
	if p.Names[2].NameType != domain.NameTypeAKA {
		t.Errorf("Names[2].NameType = %q, want %q", p.Names[2].NameType, domain.NameTypeAKA)
	}
	if p.Names[2].NamePrefix != "Mrs." {
		t.Errorf("Names[2].NamePrefix = %q, want %q", p.Names[2].NamePrefix, "Mrs.")
	}
	if p.Names[2].NameSuffix != "Sr." {
		t.Errorf("Names[2].NameSuffix = %q, want %q", p.Names[2].NameSuffix, "Sr.")
	}
}

func TestImportMultipleNamesWithImmigrantType(t *testing.T) {
	// Test GEDCOM with immigrant and professional name types
	gedcomData := `0 HEAD
1 SOUR Test
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Johann /Muller/
2 TYPE birth
2 SPFX von
1 NAME John /Miller/
2 TYPE immigrant
1 NAME John /Miller/
2 TYPE religious
2 NPFX Rev.
1 NAME John /Miller/
2 TYPE professional
1 SEX M
1 BIRT
2 DATE 1850
2 PLAC Bavaria, Germany
1 IMMI
2 DATE 1875
2 PLAC New York, NY
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	p := persons[0]

	// Check all names are captured
	if len(p.Names) != 4 {
		t.Fatalf("len(p.Names) = %d, want 4", len(p.Names))
	}

	// First name (birth name with surname prefix)
	if p.Names[0].GivenName != "Johann" {
		t.Errorf("Names[0].GivenName = %q, want %q", p.Names[0].GivenName, "Johann")
	}
	if p.Names[0].Surname != "Muller" {
		t.Errorf("Names[0].Surname = %q, want %q", p.Names[0].Surname, "Muller")
	}
	if p.Names[0].SurnamePrefix != "von" {
		t.Errorf("Names[0].SurnamePrefix = %q, want %q", p.Names[0].SurnamePrefix, "von")
	}
	if p.Names[0].NameType != domain.NameTypeBirth {
		t.Errorf("Names[0].NameType = %q, want %q", p.Names[0].NameType, domain.NameTypeBirth)
	}

	// Second name (immigrant/anglicized name)
	if p.Names[1].GivenName != "John" {
		t.Errorf("Names[1].GivenName = %q, want %q", p.Names[1].GivenName, "John")
	}
	if p.Names[1].Surname != "Miller" {
		t.Errorf("Names[1].Surname = %q, want %q", p.Names[1].Surname, "Miller")
	}
	if p.Names[1].NameType != domain.NameTypeImmigrant {
		t.Errorf("Names[1].NameType = %q, want %q", p.Names[1].NameType, domain.NameTypeImmigrant)
	}

	// Third name (religious name)
	if p.Names[2].GivenName != "John" {
		t.Errorf("Names[2].GivenName = %q, want %q", p.Names[2].GivenName, "John")
	}
	if p.Names[2].NamePrefix != "Rev." {
		t.Errorf("Names[2].NamePrefix = %q, want %q", p.Names[2].NamePrefix, "Rev.")
	}
	if p.Names[2].NameType != domain.NameTypeReligious {
		t.Errorf("Names[2].NameType = %q, want %q", p.Names[2].NameType, domain.NameTypeReligious)
	}

	// Fourth name (professional name)
	if p.Names[3].GivenName != "John" {
		t.Errorf("Names[3].GivenName = %q, want %q", p.Names[3].GivenName, "John")
	}
	if p.Names[3].NameType != domain.NameTypeProfessional {
		t.Errorf("Names[3].NameType = %q, want %q", p.Names[3].NameType, domain.NameTypeProfessional)
	}
}

func TestImportEventWithStructuredAddress(t *testing.T) {
	// Test GEDCOM with structured address on a BAPM event (baptism)
	// BAPM goes through extractEventsFromIndividual, not as an attribute
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Smith/
1 SEX M
1 BIRT
2 DATE 1850
1 BAPM
2 DATE 15 JAN 1850
2 PLAC First Methodist Church
2 ADDR 123 Church Street
3 ADR1 123 Church Street
3 ADR2 Building A
3 CITY Chicago
3 STAE IL
3 POST 60601
3 CTRY USA
1 BURI
2 DATE 1920
2 PLAC Oak Cemetery
2 ADDR
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, _, _, _, _, events, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}

	// Should have two events: BAPM and BURI
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}

	// Find the baptism event
	var bapmEvent *gedcom.EventData
	for i := range events {
		if events[i].FactType == domain.FactPersonBaptism {
			bapmEvent = &events[i]
			break
		}
	}
	if bapmEvent == nil {
		t.Fatal("Baptism event not found")
	}

	// Check address was parsed
	if bapmEvent.Address == nil {
		t.Fatal("Baptism event address should not be nil")
	}
	if bapmEvent.Address.Line1 != "123 Church Street" {
		t.Errorf("Address.Line1 = %s, want '123 Church Street'", bapmEvent.Address.Line1)
	}
	if bapmEvent.Address.Line2 != "Building A" {
		t.Errorf("Address.Line2 = %s, want 'Building A'", bapmEvent.Address.Line2)
	}
	if bapmEvent.Address.City != "Chicago" {
		t.Errorf("Address.City = %s, want 'Chicago'", bapmEvent.Address.City)
	}
	if bapmEvent.Address.State != "IL" {
		t.Errorf("Address.State = %s, want 'IL'", bapmEvent.Address.State)
	}

	person := persons[0]
	if person.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", person.GivenName)
	}
}

func TestImportFamilyEventWithAddress(t *testing.T) {
	// Test GEDCOM with family event that has an address
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Smith/
1 SEX M
0 @I2@ INDI
1 NAME Jane /Doe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 10 JUN 1880
2 PLAC First Baptist Church
1 DIV
2 DATE 1890
2 PLAC City Court
2 ADDR 100 Court Street
3 ADR1 100 Court Street
3 CITY Springfield
3 STAE IL
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, _, _, _, _, events, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have one family event (DIV - divorce)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	event := events[0]
	if event.FactType != domain.FactFamilyDivorce {
		t.Errorf("FactType = %s, want %s", event.FactType, domain.FactFamilyDivorce)
	}

	// Check address was parsed
	if event.Address == nil {
		t.Fatal("Event address should not be nil")
	}
	if event.Address.Line1 != "100 Court Street" {
		t.Errorf("Address.Line1 = %s, want '100 Court Street'", event.Address.Line1)
	}
	if event.Address.City != "Springfield" {
		t.Errorf("Address.City = %s, want 'Springfield'", event.Address.City)
	}
}

func TestImportRepositoryWithAddress(t *testing.T) {
	// Test GEDCOM with repository containing structured address
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @R1@ REPO
1 NAME Family History Library
1 ADDR 35 N West Temple St
2 ADR1 35 N West Temple St
2 ADR2 Suite 100
2 CITY Salt Lake City
2 STAE UT
2 POST 84150
2 CTRY USA
1 PHON 801-240-2584
1 EMAIL familyhistory@familysearch.org
1 WWW https://www.familysearch.org
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, _, _, _, repos, _, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}

	repo := repos[0]
	if repo.Name != "Family History Library" {
		t.Errorf("Name = %s, want 'Family History Library'", repo.Name)
	}
	// Address should be combined from ADR1 and ADR2
	if repo.Address != "35 N West Temple St, Suite 100" {
		t.Errorf("Address = %s, want '35 N West Temple St, Suite 100'", repo.Address)
	}
	if repo.City != "Salt Lake City" {
		t.Errorf("City = %s, want 'Salt Lake City'", repo.City)
	}
	if repo.State != "UT" {
		t.Errorf("State = %s, want 'UT'", repo.State)
	}
	if repo.PostalCode != "84150" {
		t.Errorf("PostalCode = %s, want '84150'", repo.PostalCode)
	}
	if repo.Country != "USA" {
		t.Errorf("Country = %s, want 'USA'", repo.Country)
	}
	if repo.Phone != "801-240-2584" {
		t.Errorf("Phone = %s, want '801-240-2584'", repo.Phone)
	}
	if repo.Email != "familyhistory@familysearch.org" {
		t.Errorf("Email = %s, want 'familyhistory@familysearch.org'", repo.Email)
	}
	if repo.Website != "https://www.familysearch.org" {
		t.Errorf("Website = %s, want 'https://www.familysearch.org'", repo.Website)
	}
}

func TestImportMediaObject(t *testing.T) {
	// Test GEDCOM with OBJE (media object) records
	// Note: In GEDCOM, MEDI is a child of FORM, not a sibling like TYPE would be
	gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @M1@ OBJE
1 FILE /photos/family_portrait.jpg
2 FORM image/jpeg
3 MEDI PHOTO
2 TITL Family Portrait 1920
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, _, _, _, _, _, _, _, _, _, _, media, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have 1 media object
	if result.MediaImported != 1 {
		t.Errorf("MediaImported = %d, want 1", result.MediaImported)
	}
	if len(media) != 1 {
		t.Fatalf("len(media) = %d, want 1", len(media))
	}

	m := media[0]
	if m.GedcomXref != "@M1@" {
		t.Errorf("GedcomXref = %s, want @M1@", m.GedcomXref)
	}
	// Title comes from FILE-level TITL since there's no OBJE-level title parsing
	if m.Title != "Family Portrait 1920" {
		t.Errorf("Title = %s, want 'Family Portrait 1920'", m.Title)
	}
	if m.MediaType != domain.MediaPhoto {
		t.Errorf("MediaType = %s, want %s", m.MediaType, domain.MediaPhoto)
	}
	if m.Format != "image/jpeg" {
		t.Errorf("Format = %s, want 'image/jpeg'", m.Format)
	}

	// Check files slice
	if len(m.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(m.Files))
	}
	if m.Files[0].Path != "/photos/family_portrait.jpg" {
		t.Errorf("Files[0].Path = %s, want '/photos/family_portrait.jpg'", m.Files[0].Path)
	}
	if m.Files[0].Format != "image/jpeg" {
		t.Errorf("Files[0].Format = %s, want 'image/jpeg'", m.Files[0].Format)
	}
	if m.Files[0].Title != "Family Portrait 1920" {
		t.Errorf("Files[0].Title = %s, want 'Family Portrait 1920'", m.Files[0].Title)
	}
	if m.Files[0].MediaType != "PHOTO" {
		t.Errorf("Files[0].MediaType = %s, want 'PHOTO'", m.Files[0].MediaType)
	}
}

func TestImportMediaObjectWithMultipleFiles(t *testing.T) {
	// Test GEDCOM with OBJE that has multiple FILE records (GEDCOM 7.0 style)
	// Note: MEDI is a child of FORM, not a sibling like TYPE
	gedcomData := `0 HEAD
1 GEDC
2 VERS 7.0
1 CHAR UTF-8
0 @M1@ OBJE
1 FILE /photos/highres.jpg
2 FORM image/jpeg
3 MEDI PHOTO
2 TITL High Resolution
1 FILE /photos/thumbnail.jpg
2 FORM image/jpeg
3 MEDI PHOTO
2 TITL Thumbnail
0 TRLR
`
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, _, _, _, _, _, _, _, _, _, _, _, media, err := importer.Import(ctx, strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(media) != 1 {
		t.Fatalf("len(media) = %d, want 1", len(media))
	}

	m := media[0]
	// Title comes from first file since there's no top-level TITL parsing
	if m.Title != "High Resolution" {
		t.Errorf("Title = %s, want 'High Resolution'", m.Title)
	}

	// Should have 2 files
	if len(m.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(m.Files))
	}

	// First file (high resolution)
	if m.Files[0].Path != "/photos/highres.jpg" {
		t.Errorf("Files[0].Path = %s, want '/photos/highres.jpg'", m.Files[0].Path)
	}
	if m.Files[0].Title != "High Resolution" {
		t.Errorf("Files[0].Title = %s, want 'High Resolution'", m.Files[0].Title)
	}

	// Second file (thumbnail)
	if m.Files[1].Path != "/photos/thumbnail.jpg" {
		t.Errorf("Files[1].Path = %s, want '/photos/thumbnail.jpg'", m.Files[1].Path)
	}
	if m.Files[1].Title != "Thumbnail" {
		t.Errorf("Files[1].Title = %s, want 'Thumbnail'", m.Files[1].Title)
	}
}

func TestImportMediaObjectMediaTypes(t *testing.T) {
	// Test mapping of GEDCOM MEDI values to domain MediaType
	// Note: In GEDCOM, MEDI is a child of FORM (level 3), not a sibling of FILE (level 2)
	testCases := []struct {
		medi     string
		expected domain.MediaType
	}{
		{"PHOTO", domain.MediaPhoto},
		{"photo", domain.MediaPhoto},
		{"AUDIO", domain.MediaAudio},
		{"audio", domain.MediaAudio},
		{"VIDEO", domain.MediaVideo},
		{"video", domain.MediaVideo},
		{"DOCUMENT", domain.MediaDocument},
		{"document", domain.MediaDocument},
	}

	for _, tc := range testCases {
		t.Run(tc.medi, func(t *testing.T) {
			gedcomData := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @M1@ OBJE
1 FILE /test.jpg
2 FORM image/jpeg
3 MEDI ` + tc.medi + `
2 TITL Test
0 TRLR
`
			importer := gedcom.NewImporter()
			ctx := context.Background()

			_, _, _, _, _, _, _, _, _, _, _, _, media, err := importer.Import(ctx, strings.NewReader(gedcomData))
			if err != nil {
				t.Fatalf("Import failed: %v", err)
			}

			if len(media) != 1 {
				t.Fatalf("len(media) = %d, want 1", len(media))
			}

			if media[0].MediaType != tc.expected {
				t.Errorf("MediaType = %s, want %s", media[0].MediaType, tc.expected)
			}
		})
	}
}
