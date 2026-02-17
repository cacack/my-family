package gedcom_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupExportTestData(t *testing.T, readStore *memory.ReadModelStore) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	// Create persons
	john := uuid.New()
	jane := uuid.New()
	junior := uuid.New()

	persons := []repository.PersonReadModel{
		{
			ID:           john,
			GivenName:    "John",
			Surname:      "Doe",
			FullName:     "John Doe",
			Gender:       domain.GenderMale,
			BirthDateRaw: "15 JAN 1850",
			BirthPlace:   "Springfield, IL",
			DeathDateRaw: "20 MAR 1920",
			DeathPlace:   "Chicago, IL",
		},
		{
			ID:           jane,
			GivenName:    "Jane",
			Surname:      "Smith",
			FullName:     "Jane Smith",
			Gender:       domain.GenderFemale,
			BirthDateRaw: "ABT 1855",
			BirthPlace:   "Boston, MA",
		},
		{
			ID:           junior,
			GivenName:    "Junior",
			Surname:      "Doe",
			FullName:     "Junior Doe",
			Gender:       domain.GenderMale,
			BirthDateRaw: "1880",
		},
	}

	for _, p := range persons {
		pm := p
		if err := readStore.SavePerson(ctx, &pm); err != nil {
			t.Fatal(err)
		}
	}

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &john,
		Partner1Name:     "John Doe",
		Partner2ID:       &jane,
		Partner2Name:     "Jane Smith",
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "10 JUN 1875",
		MarriagePlace:    "Springfield, IL",
	}
	if err := readStore.SaveFamily(ctx, family); err != nil {
		t.Fatal(err)
	}

	// Link child
	child := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         junior,
		PersonName:       "Junior Doe",
		RelationshipType: domain.ChildBiological,
	}
	if err := readStore.SaveFamilyChild(ctx, child); err != nil {
		t.Fatal(err)
	}

	return john, jane, junior
}

func TestExport(t *testing.T) {
	readStore := memory.NewReadModelStore()
	setupExportTestData(t, readStore)

	exporter := gedcom.NewExporter(readStore)
	ctx := context.Background()

	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	// Verify counts
	if result.PersonsExported != 3 {
		t.Errorf("PersonsExported = %d, want 3", result.PersonsExported)
	}
	if result.FamiliesExported != 1 {
		t.Errorf("FamiliesExported = %d, want 1", result.FamiliesExported)
	}
	if result.BytesWritten == 0 {
		t.Error("BytesWritten should not be 0")
	}

	// Verify GEDCOM structure
	output := buf.String()

	// Check header
	if !strings.HasPrefix(output, "0 HEAD\n") {
		t.Error("Output should start with GEDCOM header")
	}
	if !strings.Contains(output, "1 GEDC\n") {
		t.Error("Output should contain GEDC tag")
	}
	if !strings.Contains(output, "2 VERS 5.5\n") {
		t.Error("Output should contain GEDCOM version 5.5")
	}
	if !strings.Contains(output, "1 CHAR UTF-8\n") {
		t.Error("Output should contain UTF-8 character set")
	}

	// Check trailer
	if !strings.HasSuffix(output, "0 TRLR\n") {
		t.Error("Output should end with TRLR")
	}

	// Check individuals
	if !strings.Contains(output, "INDI\n") {
		t.Error("Output should contain INDI records")
	}
	if !strings.Contains(output, "1 NAME John /Doe/\n") {
		t.Error("Output should contain John Doe name")
	}
	if !strings.Contains(output, "1 SEX M\n") {
		t.Error("Output should contain male sex")
	}
	if !strings.Contains(output, "1 SEX F\n") {
		t.Error("Output should contain female sex")
	}

	// Check birth event
	if !strings.Contains(output, "1 BIRT\n") {
		t.Error("Output should contain BIRT event")
	}
	if !strings.Contains(output, "2 DATE 15 JAN 1850\n") {
		t.Error("Output should contain birth date")
	}
	if !strings.Contains(output, "2 PLAC Springfield, IL\n") {
		t.Error("Output should contain birth place")
	}

	// Check death event
	if !strings.Contains(output, "1 DEAT\n") {
		t.Error("Output should contain DEAT event")
	}

	// Check families
	if !strings.Contains(output, "FAM\n") {
		t.Error("Output should contain FAM records")
	}
	if !strings.Contains(output, "1 HUSB @I") {
		t.Error("Output should contain HUSB reference")
	}
	if !strings.Contains(output, "1 WIFE @I") {
		t.Error("Output should contain WIFE reference")
	}
	if !strings.Contains(output, "1 CHIL @I") {
		t.Error("Output should contain CHIL reference")
	}

	// Check marriage event
	if !strings.Contains(output, "1 MARR\n") {
		t.Error("Output should contain MARR event")
	}
	if !strings.Contains(output, "2 DATE 10 JUN 1875\n") {
		t.Error("Output should contain marriage date")
	}
}

func TestExport_EmptyDatabase(t *testing.T) {
	readStore := memory.NewReadModelStore()
	exporter := gedcom.NewExporter(readStore)
	ctx := context.Background()

	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.PersonsExported != 0 {
		t.Errorf("PersonsExported = %d, want 0", result.PersonsExported)
	}
	if result.FamiliesExported != 0 {
		t.Errorf("FamiliesExported = %d, want 0", result.FamiliesExported)
	}

	output := buf.String()

	// Should still have valid header and trailer
	if !strings.HasPrefix(output, "0 HEAD\n") {
		t.Error("Empty export should still have header")
	}
	if !strings.HasSuffix(output, "0 TRLR\n") {
		t.Error("Empty export should still have trailer")
	}
}

func TestExport_ApproximateDates(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	person := &repository.PersonReadModel{
		ID:           uuid.New(),
		GivenName:    "Test",
		Surname:      "Person",
		FullName:     "Test Person",
		BirthDateRaw: "ABT 1850",
	}
	readStore.SavePerson(ctx, person)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "2 DATE ABT 1850\n") {
		t.Error("Output should preserve approximate dates")
	}
}

func TestExport_DateRanges(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	person := &repository.PersonReadModel{
		ID:           uuid.New(),
		GivenName:    "Test",
		Surname:      "Person",
		FullName:     "Test Person",
		BirthDateRaw: "BET 1850 AND 1860",
	}
	readStore.SavePerson(ctx, person)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "2 DATE BET 1850 AND 1860\n") {
		t.Error("Output should preserve date ranges")
	}
}

func TestExport_Notes(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	person := &repository.PersonReadModel{
		ID:        uuid.New(),
		GivenName: "Test",
		Surname:   "Person",
		FullName:  "Test Person",
		Notes:     "First line of notes.\nSecond line of notes.",
	}
	readStore.SavePerson(ctx, person)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "1 NOTE First line of notes.\n") {
		t.Error("Output should contain NOTE tag")
	}
	if !strings.Contains(output, "2 CONT Second line of notes.\n") {
		t.Error("Output should contain CONT continuation")
	}
}

func TestExport_SingleParentFamily(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	mother := uuid.New()
	child := uuid.New()

	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        mother,
		GivenName: "Jane",
		Surname:   "Doe",
		FullName:  "Jane Doe",
		Gender:    domain.GenderFemale,
	})
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        child,
		GivenName: "Child",
		Surname:   "Doe",
		FullName:  "Child Doe",
	})

	familyID := uuid.New()
	readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:           familyID,
		Partner2ID:   &mother,
		Partner2Name: "Jane Doe",
	})
	readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:   familyID,
		PersonID:   child,
		PersonName: "Child Doe",
	})

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Should have WIFE but not HUSB
	if !strings.Contains(output, "1 WIFE @I") {
		t.Error("Single-parent family should have WIFE")
	}
	if strings.Contains(output, "1 HUSB @I") {
		t.Error("Single-parent family should not have HUSB when no father")
	}
	if !strings.Contains(output, "1 CHIL @I") {
		t.Error("Family should have CHIL")
	}
}

func TestExport_WithSources(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a source
	sourceID := uuid.New()
	source := &repository.SourceReadModel{
		ID:             sourceID,
		SourceType:     "book",
		Title:          "Parish Records",
		Author:         "John Clerk",
		Publisher:      "County Archive",
		PublishDateRaw: "1850",
		RepositoryName: "State Archive",
		Notes:          "Important source",
	}
	if err := readStore.SaveSource(ctx, source); err != nil {
		t.Fatal(err)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	// Verify source was exported
	if result.SourcesExported != 1 {
		t.Errorf("SourcesExported = %d, want 1", result.SourcesExported)
	}

	output := buf.String()

	// Check source record
	if !strings.Contains(output, "SOUR\n") {
		t.Error("Output should contain SOUR record")
	}
	if !strings.Contains(output, "1 TITL Parish Records\n") {
		t.Error("Output should contain source title")
	}
	if !strings.Contains(output, "1 AUTH John Clerk\n") {
		t.Error("Output should contain source author")
	}
	if !strings.Contains(output, "1 PUBL County Archive\n") {
		t.Error("Output should contain source publisher")
	}
	if !strings.Contains(output, "1 REPO\n") {
		t.Error("Output should contain REPO tag")
	}
	if !strings.Contains(output, "2 NAME State Archive\n") {
		t.Error("Output should contain repository name")
	}
	if !strings.Contains(output, "1 NOTE Important source\n") {
		t.Error("Output should contain source notes")
	}
}

func TestExport_SourceWithXref(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a source with GedcomXref
	sourceID := uuid.New()
	source := &repository.SourceReadModel{
		ID:         sourceID,
		SourceType: "book",
		Title:      "Test Source",
		GedcomXref: "@S100@",
	}
	if err := readStore.SaveSource(ctx, source); err != nil {
		t.Fatal(err)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Should preserve the original XREF
	if !strings.Contains(output, "0 @S100@ SOUR\n") {
		t.Error("Output should preserve original GEDCOM XREF for source")
	}
}

func TestExport_WithCitations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a source
	sourceID := uuid.New()
	source := &repository.SourceReadModel{
		ID:         sourceID,
		SourceType: "book",
		Title:      "Birth Register",
	}
	readStore.SaveSource(ctx, source)

	// Create a person
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "John",
		Surname:      "Doe",
		FullName:     "John Doe",
		BirthDateRaw: "1 JAN 1850",
		BirthPlace:   "Springfield",
	}
	readStore.SavePerson(ctx, person)

	// Create a citation for birth event
	citationID := uuid.New()
	citation := &repository.CitationReadModel{
		ID:            citationID,
		SourceID:      sourceID,
		FactType:      domain.FactPersonBirth,
		FactOwnerID:   personID,
		Page:          "123",
		SourceQuality: "original",
		InformantType: "primary",
		EvidenceType:  "direct",
		QuotedText:    "Born January 1st",
	}
	readStore.SaveCitation(ctx, citation)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	// Verify citation was exported
	if result.CitationsExported != 1 {
		t.Errorf("CitationsExported = %d, want 1", result.CitationsExported)
	}

	output := buf.String()

	// Check citation in birth event
	if !strings.Contains(output, "1 BIRT\n") {
		t.Error("Output should contain BIRT event")
	}
	if !strings.Contains(output, "2 SOUR @S") {
		t.Error("Output should contain source reference in BIRT")
	}
	if !strings.Contains(output, "3 PAGE 123\n") {
		t.Error("Output should contain citation page")
	}
	if !strings.Contains(output, "3 QUAY 3\n") {
		t.Error("Output should contain QUAY 3 for direct/primary evidence")
	}
	if !strings.Contains(output, "3 DATA\n") {
		t.Error("Output should contain DATA tag")
	}
	if !strings.Contains(output, "4 TEXT Born January 1st\n") {
		t.Error("Output should contain quoted text")
	}
}

func TestExport_FamilyCitations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a source
	sourceID := uuid.New()
	source := &repository.SourceReadModel{
		ID:         sourceID,
		SourceType: "archive",
		Title:      "Marriage Records",
	}
	readStore.SaveSource(ctx, source)

	// Create persons
	husbandID := uuid.New()
	wifeID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        husbandID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
	})
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        wifeID,
		GivenName: "Jane",
		Surname:   "Smith",
		FullName:  "Jane Smith",
	})

	// Create family with marriage
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &husbandID,
		Partner2ID:       &wifeID,
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "10 JUN 1875",
		MarriagePlace:    "Chicago",
	}
	readStore.SaveFamily(ctx, family)

	// Create citation for marriage
	citation := &repository.CitationReadModel{
		ID:            uuid.New(),
		SourceID:      sourceID,
		FactType:      domain.FactFamilyMarriage,
		FactOwnerID:   familyID,
		Page:          "456",
		InformantType: "secondary",
		QuotedText:    "Married on June 10th",
	}
	readStore.SaveCitation(ctx, citation)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.CitationsExported != 1 {
		t.Errorf("CitationsExported = %d, want 1", result.CitationsExported)
	}

	output := buf.String()

	// Check citation in marriage event
	if !strings.Contains(output, "1 MARR\n") {
		t.Error("Output should contain MARR event")
	}
	if !strings.Contains(output, "2 SOUR @S") {
		t.Error("Output should contain source reference in MARR")
	}
	if !strings.Contains(output, "3 PAGE 456\n") {
		t.Error("Output should contain citation page")
	}
	if !strings.Contains(output, "3 QUAY 2\n") {
		t.Error("Output should contain QUAY 2 for secondary informant")
	}
}

func TestExport_MultipleCitations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create sources
	source1ID := uuid.New()
	source2ID := uuid.New()
	readStore.SaveSource(ctx, &repository.SourceReadModel{
		ID:         source1ID,
		SourceType: "book",
		Title:      "Source 1",
	})
	readStore.SaveSource(ctx, &repository.SourceReadModel{
		ID:         source2ID,
		SourceType: "book",
		Title:      "Source 2",
	})

	// Create person
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "John",
		Surname:      "Doe",
		FullName:     "John Doe",
		BirthDateRaw: "1850",
	}
	readStore.SavePerson(ctx, person)

	// Create two citations for birth
	readStore.SaveCitation(ctx, &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    source1ID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: personID,
		Page:        "10",
	})
	readStore.SaveCitation(ctx, &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    source2ID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: personID,
		Page:        "20",
	})

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	// Should have exported both citations
	if result.CitationsExported != 2 {
		t.Errorf("CitationsExported = %d, want 2", result.CitationsExported)
	}

	output := buf.String()

	// Should have both page numbers
	if !strings.Contains(output, "3 PAGE 10\n") {
		t.Error("Output should contain first citation page")
	}
	if !strings.Contains(output, "3 PAGE 20\n") {
		t.Error("Output should contain second citation page")
	}
}

func TestExport_CitationQualityMapping(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	sourceID := uuid.New()
	readStore.SaveSource(ctx, &repository.SourceReadModel{
		ID:         sourceID,
		SourceType: "book",
		Title:      "Test Source",
	})

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "Test",
		Surname:      "Person",
		FullName:     "Test Person",
		BirthDateRaw: "1850",
		DeathDateRaw: "1920",
	})

	tests := []struct {
		name          string
		evidenceType  domain.EvidenceType
		informantType domain.InformantType
		wantQuality   string
		wantNoQuality bool // QUAY 0 is not output (it's the default)
		factType      domain.FactType
	}{
		{
			name:          "direct and primary",
			evidenceType:  domain.EvidenceDirect,
			informantType: domain.InformantPrimary,
			wantQuality:   "3 QUAY 3",
			factType:      domain.FactPersonBirth,
		},
		{
			name:          "secondary informant",
			informantType: domain.InformantSecondary,
			wantQuality:   "3 QUAY 2",
			factType:      domain.FactPersonDeath,
		},
		{
			name:         "indirect evidence",
			evidenceType: domain.EvidenceIndirect,
			wantQuality:  "3 QUAY 1",
			factType:     domain.FactPersonBirth,
		},
		{
			name:          "negative evidence",
			evidenceType:  domain.EvidenceNegative,
			wantNoQuality: true, // QUAY 0 is the default, not output per GEDCOM spec
			factType:      domain.FactPersonDeath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous citations
			readStore := memory.NewReadModelStore()
			readStore.SaveSource(ctx, &repository.SourceReadModel{
				ID:         sourceID,
				SourceType: "book",
				Title:      "Test Source",
			})
			readStore.SavePerson(ctx, &repository.PersonReadModel{
				ID:           personID,
				GivenName:    "Test",
				Surname:      "Person",
				FullName:     "Test Person",
				BirthDateRaw: "1850",
				DeathDateRaw: "1920",
			})

			citation := &repository.CitationReadModel{
				ID:            uuid.New(),
				SourceID:      sourceID,
				FactType:      tt.factType,
				FactOwnerID:   personID,
				EvidenceType:  tt.evidenceType,
				InformantType: tt.informantType,
			}
			readStore.SaveCitation(ctx, citation)

			exporter := gedcom.NewExporter(readStore)
			buf := &bytes.Buffer{}
			_, err := exporter.Export(ctx, buf)
			if err != nil {
				t.Fatal(err)
			}

			output := buf.String()
			if tt.wantNoQuality {
				// QUAY 0 should NOT be in output (it's the default)
				if strings.Contains(output, "QUAY") {
					t.Errorf("Output should not contain QUAY tag for quality 0, got:\n%s", output)
				}
			} else if !strings.Contains(output, tt.wantQuality) {
				t.Errorf("Output should contain %s, got:\n%s", tt.wantQuality, output)
			}
		})
	}
}

func TestExport_SourceWithMultilineNotes(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	source := &repository.SourceReadModel{
		ID:         uuid.New(),
		SourceType: "book",
		Title:      "Test Source",
		Notes:      "First line\nSecond line\nThird line",
	}
	readStore.SaveSource(ctx, source)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Should have NOTE with CONT
	if !strings.Contains(output, "1 NOTE First line\n") {
		t.Error("Output should contain first line of notes")
	}
	if !strings.Contains(output, "2 CONT Second line\n") {
		t.Error("Output should contain CONT for second line")
	}
	if !strings.Contains(output, "2 CONT Third line\n") {
		t.Error("Output should contain CONT for third line")
	}
}

func TestExport_CitationWithMultilineText(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	sourceID := uuid.New()
	readStore.SaveSource(ctx, &repository.SourceReadModel{
		ID:         sourceID,
		SourceType: "book",
		Title:      "Test Source",
	})

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "Test",
		Surname:      "Person",
		FullName:     "Test Person",
		BirthDateRaw: "1850",
	})

	citation := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: personID,
		QuotedText:  "First line of quote\nSecond line of quote",
	}
	readStore.SaveCitation(ctx, citation)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Should have TEXT with CONT
	if !strings.Contains(output, "4 TEXT First line of quote\n") {
		t.Error("Output should contain first line of quoted text")
	}
	if !strings.Contains(output, "5 CONT Second line of quote\n") {
		t.Error("Output should contain CONT for second line of quoted text")
	}
}

// Event export tests

func TestExport_PersonBurialEvent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "John",
		Surname:      "Doe",
		FullName:     "John Doe",
		DeathDateRaw: "20 MAR 1920",
		DeathPlace:   "Chicago, IL",
	})

	// Add burial event
	event := &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "person",
		OwnerID:   personID,
		FactType:  domain.FactPersonBurial,
		DateRaw:   "23 MAR 1920",
		Place:     "Springfield Cemetery, IL",
	}
	readStore.SaveEvent(ctx, event)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	// Verify event was exported
	if result.EventsExported != 1 {
		t.Errorf("EventsExported = %d, want 1", result.EventsExported)
	}

	output := buf.String()

	// Check burial event
	if !strings.Contains(output, "1 BURI\n") {
		t.Error("Output should contain BURI event")
	}
	if !strings.Contains(output, "2 DATE 23 MAR 1920\n") {
		t.Error("Output should contain burial date")
	}
	if !strings.Contains(output, "2 PLAC Springfield Cemetery, IL\n") {
		t.Error("Output should contain burial place")
	}
}

func TestExport_PersonBaptismEvent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "Mary",
		Surname:      "Smith",
		FullName:     "Mary Smith",
		BirthDateRaw: "1 JAN 1855",
	})

	// Add baptism event
	event := &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "person",
		OwnerID:   personID,
		FactType:  domain.FactPersonBaptism,
		DateRaw:   "15 JAN 1855",
		Place:     "First Presbyterian Church",
	}
	readStore.SaveEvent(ctx, event)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.EventsExported != 1 {
		t.Errorf("EventsExported = %d, want 1", result.EventsExported)
	}

	output := buf.String()

	if !strings.Contains(output, "1 BAPM\n") {
		t.Error("Output should contain BAPM event")
	}
	if !strings.Contains(output, "2 DATE 15 JAN 1855\n") {
		t.Error("Output should contain baptism date")
	}
}

func TestExport_PersonCensusEvent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        personID,
		GivenName: "Robert",
		Surname:   "Jones",
		FullName:  "Robert Jones",
	})

	// Add census event
	event := &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "person",
		OwnerID:   personID,
		FactType:  domain.FactPersonCensus,
		DateRaw:   "1880",
		Place:     "Springfield, IL",
	}
	readStore.SaveEvent(ctx, event)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	if !strings.Contains(output, "1 CENS\n") {
		t.Error("Output should contain CENS event")
	}
	if !strings.Contains(output, "2 DATE 1880\n") {
		t.Error("Output should contain census date")
	}
}

func TestExport_PersonImmigrationEvent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        personID,
		GivenName: "Hans",
		Surname:   "Mueller",
		FullName:  "Hans Mueller",
	})

	// Add immigration event
	event := &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "person",
		OwnerID:   personID,
		FactType:  domain.FactPersonImmigration,
		DateRaw:   "1880",
		Place:     "Ellis Island, NY",
	}
	readStore.SaveEvent(ctx, event)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	if !strings.Contains(output, "1 IMMI\n") {
		t.Error("Output should contain IMMI event")
	}
	if !strings.Contains(output, "2 PLAC Ellis Island, NY\n") {
		t.Error("Output should contain immigration place")
	}
}

// Attribute export tests

func TestExport_PersonOccupationAttribute(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        personID,
		GivenName: "William",
		Surname:   "Taylor",
		FullName:  "William Taylor",
	})

	// Add occupation attribute
	attr := &repository.AttributeReadModel{
		ID:       uuid.New(),
		PersonID: personID,
		FactType: domain.FactPersonOccupation,
		Value:    "Blacksmith",
		DateRaw:  "1860",
		Place:    "Springfield, IL",
	}
	readStore.SaveAttribute(ctx, attr)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.AttributesExported != 1 {
		t.Errorf("AttributesExported = %d, want 1", result.AttributesExported)
	}

	output := buf.String()

	if !strings.Contains(output, "1 OCCU Blacksmith\n") {
		t.Error("Output should contain OCCU attribute with value")
	}
	if !strings.Contains(output, "2 DATE 1860\n") {
		t.Error("Output should contain occupation date")
	}
	if !strings.Contains(output, "2 PLAC Springfield, IL\n") {
		t.Error("Output should contain occupation place")
	}
}

func TestExport_PersonResidenceAttribute(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        personID,
		GivenName: "Sarah",
		Surname:   "Brown",
		FullName:  "Sarah Brown",
	})

	// Add residence attribute
	attr := &repository.AttributeReadModel{
		ID:       uuid.New(),
		PersonID: personID,
		FactType: domain.FactPersonResidence,
		Value:    "123 Main St",
		DateRaw:  "1870",
		Place:    "Springfield, IL",
	}
	readStore.SaveAttribute(ctx, attr)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	if !strings.Contains(output, "1 RESI 123 Main St\n") {
		t.Error("Output should contain RESI attribute with value")
	}
}

// Family event export tests

func TestExport_FamilyMarriageBann(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	husbandID := uuid.New()
	wifeID := uuid.New()

	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        husbandID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
	})
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        wifeID,
		GivenName: "Jane",
		Surname:   "Smith",
		FullName:  "Jane Smith",
	})

	familyID := uuid.New()
	readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &husbandID,
		Partner2ID:       &wifeID,
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "1 JUN 1875",
	})

	// Add marriage bann event
	event := &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "family",
		OwnerID:   familyID,
		FactType:  domain.FactFamilyMarriageBann,
		DateRaw:   "1 MAY 1875",
		Place:     "First Church, Springfield, IL",
	}
	readStore.SaveEvent(ctx, event)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.EventsExported != 1 {
		t.Errorf("EventsExported = %d, want 1", result.EventsExported)
	}

	output := buf.String()

	if !strings.Contains(output, "1 MARB\n") {
		t.Error("Output should contain MARB event")
	}
	if !strings.Contains(output, "2 DATE 1 MAY 1875\n") {
		t.Error("Output should contain marriage bann date")
	}
}

func TestExport_FamilyAnnulment(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	husbandID := uuid.New()
	wifeID := uuid.New()

	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        husbandID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
	})
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        wifeID,
		GivenName: "Jane",
		Surname:   "Smith",
		FullName:  "Jane Smith",
	})

	familyID := uuid.New()
	readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &husbandID,
		Partner2ID:       &wifeID,
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "1 JUN 1875",
	})

	// Add annulment event
	event := &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "family",
		OwnerID:   familyID,
		FactType:  domain.FactFamilyAnnulment,
		DateRaw:   "1 JAN 1876",
		Place:     "County Court, Springfield, IL",
	}
	readStore.SaveEvent(ctx, event)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	if !strings.Contains(output, "1 ANUL\n") {
		t.Error("Output should contain ANUL event")
	}
	if !strings.Contains(output, "2 DATE 1 JAN 1876\n") {
		t.Error("Output should contain annulment date")
	}
}

func TestExport_MultipleEventsAndAttributes(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "John",
		Surname:      "Doe",
		FullName:     "John Doe",
		BirthDateRaw: "1 JAN 1850",
		DeathDateRaw: "20 MAR 1920",
	})

	// Add multiple events
	events := []*repository.EventReadModel{
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  domain.FactPersonBaptism,
			DateRaw:   "15 JAN 1850",
		},
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  domain.FactPersonBurial,
			DateRaw:   "23 MAR 1920",
			Place:     "Springfield Cemetery",
		},
	}
	for _, e := range events {
		readStore.SaveEvent(ctx, e)
	}

	// Add multiple attributes
	attributes := []*repository.AttributeReadModel{
		{
			ID:       uuid.New(),
			PersonID: personID,
			FactType: domain.FactPersonOccupation,
			Value:    "Farmer",
			DateRaw:  "1880",
		},
		{
			ID:       uuid.New(),
			PersonID: personID,
			FactType: domain.FactPersonResidence,
			Value:    "100 Main St",
			Place:    "Springfield, IL",
		},
	}
	for _, a := range attributes {
		readStore.SaveAttribute(ctx, a)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.EventsExported != 2 {
		t.Errorf("EventsExported = %d, want 2", result.EventsExported)
	}
	if result.AttributesExported != 2 {
		t.Errorf("AttributesExported = %d, want 2", result.AttributesExported)
	}

	output := buf.String()

	// Verify events
	if !strings.Contains(output, "1 BAPM\n") {
		t.Error("Output should contain BAPM event")
	}
	if !strings.Contains(output, "1 BURI\n") {
		t.Error("Output should contain BURI event")
	}

	// Verify attributes
	if !strings.Contains(output, "1 OCCU Farmer\n") {
		t.Error("Output should contain OCCU attribute")
	}
	if !strings.Contains(output, "1 RESI 100 Main St\n") {
		t.Error("Output should contain RESI attribute")
	}
}

func TestExport_EventWithCause(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "John",
		Surname:      "Doe",
		FullName:     "John Doe",
		DeathDateRaw: "20 MAR 1920",
	})

	// Add burial event with cause
	event := &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "person",
		OwnerID:   personID,
		FactType:  domain.FactPersonBurial,
		DateRaw:   "23 MAR 1920",
		Place:     "Springfield Cemetery",
		Cause:     "Natural causes",
	}
	readStore.SaveEvent(ctx, event)

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	if !strings.Contains(output, "2 CAUS Natural causes\n") {
		t.Error("Output should contain CAUS tag with cause value")
	}
}

func TestExport_AllEventTypes(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        personID,
		GivenName: "Test",
		Surname:   "Person",
		FullName:  "Test Person",
	})

	// Add all person event types
	eventTypes := []domain.FactType{
		domain.FactPersonBurial,
		domain.FactPersonCremation,
		domain.FactPersonBaptism,
		domain.FactPersonChristening,
		domain.FactPersonEmigration,
		domain.FactPersonImmigration,
		domain.FactPersonNaturalization,
		domain.FactPersonCensus,
	}

	for i, factType := range eventTypes {
		event := &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  factType,
			DateRaw:   "1 JAN 18" + string(rune('5'+i%5)) + "0",
		}
		readStore.SaveEvent(ctx, event)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.EventsExported != len(eventTypes) {
		t.Errorf("EventsExported = %d, want %d", result.EventsExported, len(eventTypes))
	}

	output := buf.String()

	// Verify each event type is present in output
	expectedTags := []string{"BURI", "CREM", "BAPM", "CHR", "EMIG", "IMMI", "NATU", "CENS"}
	for _, tag := range expectedTags {
		if !strings.Contains(output, "1 "+tag+"\n") {
			t.Errorf("Output should contain %s event", tag)
		}
	}
}

func TestExport_AllAttributeTypes(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        personID,
		GivenName: "Test",
		Surname:   "Person",
		FullName:  "Test Person",
	})

	// Add all attribute types
	attrConfigs := []struct {
		factType domain.FactType
		value    string
	}{
		{domain.FactPersonOccupation, "Farmer"},
		{domain.FactPersonResidence, "123 Main St"},
		{domain.FactPersonEducation, "Grammar School"},
		{domain.FactPersonReligion, "Protestant"},
		{domain.FactPersonTitle, "Dr."},
	}

	for _, cfg := range attrConfigs {
		attr := &repository.AttributeReadModel{
			ID:       uuid.New(),
			PersonID: personID,
			FactType: cfg.factType,
			Value:    cfg.value,
		}
		readStore.SaveAttribute(ctx, attr)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.AttributesExported != len(attrConfigs) {
		t.Errorf("AttributesExported = %d, want %d", result.AttributesExported, len(attrConfigs))
	}

	output := buf.String()

	// Verify each attribute type is present in output
	expectedTags := []string{
		"1 OCCU Farmer\n",
		"1 RESI 123 Main St\n",
		"1 EDUC Grammar School\n",
		"1 RELI Protestant\n",
		"1 TITL Dr.\n",
	}
	for _, expected := range expectedTags {
		if !strings.Contains(output, expected) {
			t.Errorf("Output should contain %q", expected)
		}
	}
}

func TestExport_AllFamilyEventTypes(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	husbandID := uuid.New()
	wifeID := uuid.New()

	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        husbandID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
	})
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        wifeID,
		GivenName: "Jane",
		Surname:   "Smith",
		FullName:  "Jane Smith",
	})

	familyID := uuid.New()
	readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &husbandID,
		Partner2ID:       &wifeID,
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "1 JUN 1875",
	})

	// Add family event types (excluding marriage/divorce which are core)
	eventTypes := []domain.FactType{
		domain.FactFamilyMarriageBann,
		domain.FactFamilyMarriageContract,
		domain.FactFamilyMarriageLicense,
		domain.FactFamilyMarriageSettlement,
		domain.FactFamilyAnnulment,
		domain.FactFamilyEngagement,
	}

	for i, factType := range eventTypes {
		event := &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: "family",
			OwnerID:   familyID,
			FactType:  factType,
			DateRaw:   "1 JUN 187" + string(rune('0'+i)),
		}
		readStore.SaveEvent(ctx, event)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	if result.EventsExported != len(eventTypes) {
		t.Errorf("EventsExported = %d, want %d", result.EventsExported, len(eventTypes))
	}

	output := buf.String()

	// Verify each family event type is present in output
	expectedTags := []string{"MARB", "MARC", "MARL", "MARS", "ANUL", "ENGA"}
	for _, tag := range expectedTags {
		if !strings.Contains(output, "1 "+tag+"\n") {
			t.Errorf("Output should contain %s event", tag)
		}
	}
}

func TestExport_MultipleNames(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a person with multiple names
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "Maria",
		Surname:      "Schmidt",
		FullName:     "Maria Schmidt",
		Gender:       domain.GenderFemale,
		BirthDateRaw: "1850",
	}
	if err := readStore.SavePerson(ctx, person); err != nil {
		t.Fatal(err)
	}

	// Add multiple names
	names := []repository.PersonNameReadModel{
		{
			ID:        uuid.New(),
			PersonID:  personID,
			GivenName: "Maria",
			Surname:   "Schmidt",
			FullName:  "Maria Schmidt",
			Nickname:  "Mitzi",
			NameType:  domain.NameTypeBirth,
			IsPrimary: true,
		},
		{
			ID:        uuid.New(),
			PersonID:  personID,
			GivenName: "Mary",
			Surname:   "Smith",
			FullName:  "Mary Smith",
			NameType:  domain.NameTypeMarried,
			IsPrimary: false,
		},
		{
			ID:            uuid.New(),
			PersonID:      personID,
			GivenName:     "Mary Elizabeth",
			Surname:       "Smith",
			FullName:      "Mary Elizabeth Smith",
			NamePrefix:    "Mrs.",
			NameSuffix:    "Sr.",
			SurnamePrefix: "von",
			NameType:      domain.NameTypeAKA,
			IsPrimary:     false,
		},
	}
	for _, n := range names {
		nm := n
		if err := readStore.SavePersonName(ctx, &nm); err != nil {
			t.Fatal(err)
		}
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Verify all three names are exported
	if !strings.Contains(output, "1 NAME Maria /Schmidt/") {
		t.Error("Output should contain birth name 'Maria /Schmidt/'")
	}
	if !strings.Contains(output, "1 NAME Mary /Smith/") {
		t.Error("Output should contain married name 'Mary /Smith/'")
	}
	if !strings.Contains(output, "1 NAME Mary Elizabeth /Smith/") {
		t.Error("Output should contain AKA name 'Mary Elizabeth /Smith/'")
	}

	// Verify TYPE tags (birth should NOT have TYPE tag, others should)
	if strings.Contains(output, "2 TYPE birth") {
		t.Error("Birth type should not be exported as it's the default")
	}
	if !strings.Contains(output, "2 TYPE married") {
		t.Error("Output should contain '2 TYPE married'")
	}
	if !strings.Contains(output, "2 TYPE aka") {
		t.Error("Output should contain '2 TYPE aka'")
	}

	// Verify name components
	if !strings.Contains(output, "2 NICK Mitzi") {
		t.Error("Output should contain nickname 'Mitzi'")
	}
	if !strings.Contains(output, "2 NPFX Mrs.") {
		t.Error("Output should contain prefix 'Mrs.'")
	}
	if !strings.Contains(output, "2 NSFX Sr.") {
		t.Error("Output should contain suffix 'Sr.'")
	}
	if !strings.Contains(output, "2 SPFX von") {
		t.Error("Output should contain surname prefix 'von'")
	}
}

func TestExport_PrimaryNameFirst(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a person
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "Johann",
		Surname:      "Muller",
		FullName:     "Johann Muller",
		Gender:       domain.GenderMale,
		BirthDateRaw: "1850",
	}
	if err := readStore.SavePerson(ctx, person); err != nil {
		t.Fatal(err)
	}

	// Add multiple names - note that secondary name is added first but has IsPrimary=false
	names := []repository.PersonNameReadModel{
		{
			ID:        uuid.New(),
			PersonID:  personID,
			GivenName: "John",
			Surname:   "Miller",
			FullName:  "John Miller",
			NameType:  domain.NameTypeImmigrant,
			IsPrimary: false,
		},
		{
			ID:        uuid.New(),
			PersonID:  personID,
			GivenName: "Johann",
			Surname:   "Muller",
			FullName:  "Johann Muller",
			NameType:  domain.NameTypeBirth,
			IsPrimary: true,
		},
	}
	for _, n := range names {
		nm := n
		if err := readStore.SavePersonName(ctx, &nm); err != nil {
			t.Fatal(err)
		}
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Find the positions of both name tags
	birthNamePos := strings.Index(output, "1 NAME Johann /Muller/")
	immigrantNamePos := strings.Index(output, "1 NAME John /Miller/")

	if birthNamePos == -1 {
		t.Error("Output should contain birth name 'Johann /Muller/'")
	}
	if immigrantNamePos == -1 {
		t.Error("Output should contain immigrant name 'John /Miller/'")
	}

	// Primary (birth) name should appear before immigrant name
	if birthNamePos > immigrantNamePos {
		t.Error("Primary name should appear before secondary names")
	}
}

func TestExport_PlaceCoordinates(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a person with birth and death place coordinates
	personID := uuid.New()
	birthLat := "N42.3601"
	birthLong := "W71.0589"
	deathLat := "N41.8781"
	deathLong := "W87.6298"

	person := &repository.PersonReadModel{
		ID:             personID,
		GivenName:      "John",
		Surname:        "Doe",
		FullName:       "John Doe",
		Gender:         domain.GenderMale,
		BirthDateRaw:   "15 JAN 1850",
		BirthPlace:     "Boston, MA, USA",
		BirthPlaceLat:  &birthLat,
		BirthPlaceLong: &birthLong,
		DeathDateRaw:   "20 MAR 1920",
		DeathPlace:     "Chicago, IL, USA",
		DeathPlaceLat:  &deathLat,
		DeathPlaceLong: &deathLong,
	}
	if err := readStore.SavePerson(ctx, person); err != nil {
		t.Fatal(err)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Verify birth place with MAP structure
	if !strings.Contains(output, "2 PLAC Boston, MA, USA\n") {
		t.Error("Output should contain birth place")
	}
	if !strings.Contains(output, "3 MAP\n") {
		t.Error("Output should contain MAP tag for birth place")
	}
	if !strings.Contains(output, "4 LATI N42.3601\n") {
		t.Error("Output should contain LATI tag with birth latitude")
	}
	if !strings.Contains(output, "4 LONG W71.0589\n") {
		t.Error("Output should contain LONG tag with birth longitude")
	}

	// Verify death place with MAP structure
	if !strings.Contains(output, "2 PLAC Chicago, IL, USA\n") {
		t.Error("Output should contain death place")
	}
	if !strings.Contains(output, "4 LATI N41.8781\n") {
		t.Error("Output should contain LATI tag with death latitude")
	}
	if !strings.Contains(output, "4 LONG W87.6298\n") {
		t.Error("Output should contain LONG tag with death longitude")
	}
}

func TestExport_MarriagePlaceCoordinates(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create two persons and a family with marriage place coordinates
	johnID := uuid.New()
	janeID := uuid.New()
	familyID := uuid.New()

	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        johnID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Gender:    domain.GenderMale,
	})
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        janeID,
		GivenName: "Jane",
		Surname:   "Smith",
		FullName:  "Jane Smith",
		Gender:    domain.GenderFemale,
	})

	marriageLat := "N39.7817"
	marriageLong := "W89.6501"

	family := &repository.FamilyReadModel{
		ID:                familyID,
		Partner1ID:        &johnID,
		Partner2ID:        &janeID,
		RelationshipType:  domain.RelationMarriage,
		MarriageDateRaw:   "15 JUN 1875",
		MarriagePlace:     "Springfield, IL, USA",
		MarriagePlaceLat:  &marriageLat,
		MarriagePlaceLong: &marriageLong,
	}
	if err := readStore.SaveFamily(ctx, family); err != nil {
		t.Fatal(err)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Verify marriage place with MAP structure
	if !strings.Contains(output, "2 PLAC Springfield, IL, USA\n") {
		t.Error("Output should contain marriage place")
	}
	if !strings.Contains(output, "3 MAP\n") {
		t.Error("Output should contain MAP tag for marriage place")
	}
	if !strings.Contains(output, "4 LATI N39.7817\n") {
		t.Error("Output should contain LATI tag with marriage latitude")
	}
	if !strings.Contains(output, "4 LONG W89.6501\n") {
		t.Error("Output should contain LONG tag with marriage longitude")
	}
}

func TestExport_PlaceWithoutCoordinates(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a person with place but no coordinates
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:         personID,
		GivenName:  "John",
		Surname:    "Doe",
		FullName:   "John Doe",
		BirthPlace: "Unknown City",
	}
	if err := readStore.SavePerson(ctx, person); err != nil {
		t.Fatal(err)
	}

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Verify place is present but no MAP structure
	if !strings.Contains(output, "2 PLAC Unknown City\n") {
		t.Error("Output should contain birth place")
	}
	// MAP should NOT appear after this place
	placeIdx := strings.Index(output, "2 PLAC Unknown City\n")
	nextLineIdx := placeIdx + len("2 PLAC Unknown City\n")
	if nextLineIdx < len(output) {
		nextSection := output[nextLineIdx:]
		// Find next tag at level 2 or higher (end of place subordinates)
		lines := strings.Split(nextSection, "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], "3 MAP") {
			t.Error("Output should NOT contain MAP tag when coordinates are missing")
		}
	}
}

// Progress callback tests

func TestExportWithProgress_CallsCallback(t *testing.T) {
	readStore := memory.NewReadModelStore()
	setupExportTestData(t, readStore)

	exporter := gedcom.NewExporter(readStore)
	ctx := context.Background()

	// Track progress callbacks
	var progressCalls []gedcom.ExportProgress
	callback := func(progress gedcom.ExportProgress) error {
		progressCalls = append(progressCalls, progress)
		return nil
	}

	buf := &bytes.Buffer{}
	result, err := exporter.ExportWithProgress(ctx, buf, callback)
	if err != nil {
		t.Fatal(err)
	}

	// Verify export succeeded
	if result.PersonsExported != 3 {
		t.Errorf("PersonsExported = %d, want 3", result.PersonsExported)
	}

	// Verify progress callbacks were called
	if len(progressCalls) == 0 {
		t.Fatal("Expected progress callbacks to be called")
	}

	// Verify we got callbacks for different phases
	phases := make(map[string]bool)
	for _, p := range progressCalls {
		phases[p.Phase] = true
	}

	// Should have persons and families phases (sources might be empty)
	if !phases["persons"] {
		t.Error("Expected 'persons' phase in progress callbacks")
	}
	if !phases["families"] {
		t.Error("Expected 'families' phase in progress callbacks")
	}
	if !phases["complete"] {
		t.Error("Expected 'complete' phase in progress callbacks")
	}

	// Last callback should be complete with 100%
	lastProgress := progressCalls[len(progressCalls)-1]
	if lastProgress.Phase != "complete" {
		t.Errorf("Last phase = %s, want 'complete'", lastProgress.Phase)
	}
	if lastProgress.Percentage != 100.0 {
		t.Errorf("Final percentage = %f, want 100.0", lastProgress.Percentage)
	}
}

func TestExportWithProgress_NoCallback(t *testing.T) {
	readStore := memory.NewReadModelStore()
	setupExportTestData(t, readStore)

	exporter := gedcom.NewExporter(readStore)
	ctx := context.Background()

	buf := &bytes.Buffer{}
	result, err := exporter.ExportWithProgress(ctx, buf, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify export still works without callback
	if result.PersonsExported != 3 {
		t.Errorf("PersonsExported = %d, want 3", result.PersonsExported)
	}
}

func TestExportWithProgress_CallbackCancellation(t *testing.T) {
	readStore := memory.NewReadModelStore()
	setupExportTestData(t, readStore)

	exporter := gedcom.NewExporter(readStore)
	ctx := context.Background()

	// Callback that returns an error to cancel export
	callCount := 0
	cancelErr := context.Canceled
	callback := func(_ gedcom.ExportProgress) error {
		callCount++
		if callCount >= 2 {
			return cancelErr
		}
		return nil
	}

	buf := &bytes.Buffer{}
	_, err := exporter.ExportWithProgress(ctx, buf, callback)

	// Should return the cancellation error
	if err != cancelErr {
		t.Errorf("Expected cancellation error, got %v", err)
	}
}

func TestExportWithProgress_ProgressPercentage(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Add more data for better progress tracking
	for i := 0; i < 30; i++ {
		person := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: "Person",
			Surname:   "Test",
		}
		readStore.SavePerson(ctx, person)
	}

	exporter := gedcom.NewExporter(readStore)

	var lastPercentage float64 = -1
	callback := func(progress gedcom.ExportProgress) error {
		// Percentage should be monotonically increasing or equal
		if progress.Percentage < lastPercentage {
			t.Errorf("Percentage decreased from %f to %f", lastPercentage, progress.Percentage)
		}
		// Percentage should be in valid range
		if progress.Percentage < 0 || progress.Percentage > 100 {
			t.Errorf("Percentage %f out of range [0, 100]", progress.Percentage)
		}
		lastPercentage = progress.Percentage
		return nil
	}

	buf := &bytes.Buffer{}
	_, err := exporter.ExportWithProgress(ctx, buf, callback)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExport_LDSOrdinances(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a person
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:        personID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Gender:    domain.GenderMale,
	}
	if err := readStore.SavePerson(ctx, person); err != nil {
		t.Fatal(err)
	}

	// Create individual LDS ordinances
	baptism := &repository.LDSOrdinanceReadModel{
		ID:        uuid.New(),
		Type:      domain.LDSBaptism,
		TypeLabel: "Baptism (LDS)",
		PersonID:  &personID,
		DateRaw:   "15 JAN 1880",
		Temple:    "SL",
		Status:    "COMPLETED",
	}
	if err := readStore.SaveLDSOrdinance(ctx, baptism); err != nil {
		t.Fatal(err)
	}

	confirmation := &repository.LDSOrdinanceReadModel{
		ID:        uuid.New(),
		Type:      domain.LDSConfirmation,
		TypeLabel: "Confirmation (LDS)",
		PersonID:  &personID,
		DateRaw:   "15 JAN 1880",
		Temple:    "SL",
		Status:    "COMPLETED",
	}
	if err := readStore.SaveLDSOrdinance(ctx, confirmation); err != nil {
		t.Fatal(err)
	}

	endowment := &repository.LDSOrdinanceReadModel{
		ID:        uuid.New(),
		Type:      domain.LDSEndowment,
		TypeLabel: "Endowment",
		PersonID:  &personID,
		DateRaw:   "20 FEB 1885",
		Temple:    "LOGAN",
		Status:    "COMPLETED",
		Place:     "Logan, Utah",
	}
	if err := readStore.SaveLDSOrdinance(ctx, endowment); err != nil {
		t.Fatal(err)
	}

	sealingChild := &repository.LDSOrdinanceReadModel{
		ID:        uuid.New(),
		Type:      domain.LDSSealingChild,
		TypeLabel: "Sealing to Parents",
		PersonID:  &personID,
		DateRaw:   "20 MAR 1885",
		Temple:    "MANTI",
		Status:    "COMPLETED",
	}
	if err := readStore.SaveLDSOrdinance(ctx, sealingChild); err != nil {
		t.Fatal(err)
	}

	// Create a family for spouse sealing
	familyID := uuid.New()
	partner2ID := uuid.New()
	partner2 := &repository.PersonReadModel{
		ID:        partner2ID,
		GivenName: "Jane",
		Surname:   "Smith",
		FullName:  "Jane Smith",
		Gender:    domain.GenderFemale,
	}
	if err := readStore.SavePerson(ctx, partner2); err != nil {
		t.Fatal(err)
	}

	family := &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &personID,
		Partner1Name:     "John Doe",
		Partner2ID:       &partner2ID,
		Partner2Name:     "Jane Smith",
		RelationshipType: domain.RelationMarriage,
	}
	if err := readStore.SaveFamily(ctx, family); err != nil {
		t.Fatal(err)
	}

	sealingSpouse := &repository.LDSOrdinanceReadModel{
		ID:        uuid.New(),
		Type:      domain.LDSSealingSpouse,
		TypeLabel: "Sealing to Spouse",
		FamilyID:  &familyID,
		DateRaw:   "25 DEC 1885",
		Temple:    "SL",
		Status:    "COMPLETED",
	}
	if err := readStore.SaveLDSOrdinance(ctx, sealingSpouse); err != nil {
		t.Fatal(err)
	}

	// Export
	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify ordinances were exported
	if result.LDSOrdinancesExported != 5 {
		t.Errorf("LDSOrdinancesExported = %d, want 5", result.LDSOrdinancesExported)
	}

	output := buf.String()

	// Verify individual ordinances in output
	if !strings.Contains(output, "1 BAPL") {
		t.Error("Output should contain BAPL tag")
	}
	if !strings.Contains(output, "1 CONL") {
		t.Error("Output should contain CONL tag")
	}
	if !strings.Contains(output, "1 ENDL") {
		t.Error("Output should contain ENDL tag")
	}
	if !strings.Contains(output, "1 SLGC") {
		t.Error("Output should contain SLGC tag")
	}

	// Verify family ordinance in output
	if !strings.Contains(output, "1 SLGS") {
		t.Error("Output should contain SLGS tag")
	}

	// Verify specific date and temple codes
	if !strings.Contains(output, "2 DATE 15 JAN 1880") {
		t.Error("Output should contain baptism date")
	}
	if !strings.Contains(output, "2 TEMP SL") {
		t.Error("Output should contain SL temple code")
	}
	if !strings.Contains(output, "2 TEMP LOGAN") {
		t.Error("Output should contain LOGAN temple code")
	}
	if !strings.Contains(output, "2 TEMP MANTI") {
		t.Error("Output should contain MANTI temple code")
	}
	if !strings.Contains(output, "2 STAT COMPLETED") {
		t.Error("Output should contain COMPLETED status")
	}
}

func TestExport_LDSOrdinances_RoundTrip(t *testing.T) {
	// Test that LDS ordinances survive import->export round trip
	inputGedcom := `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BAPL
2 DATE 15 JAN 1880
2 TEMP SL
2 STAT COMPLETED
1 CONL
2 DATE 15 JAN 1880
2 TEMP SL
2 STAT COMPLETED
1 ENDL
2 DATE 20 FEB 1885
2 TEMP LOGAN
2 STAT COMPLETED
1 SLGC
2 DATE 20 MAR 1885
2 TEMP MANTI
2 STAT COMPLETED
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 SLGS
2 DATE 25 DEC 1885
2 TEMP SL
2 STAT COMPLETED
0 TRLR
`

	// Import
	importer := gedcom.NewImporter()
	ctx := context.Background()
	result, persons, families, _, _, _, _, _, _, _, _, ldsOrdinances, _, err := importer.Import(ctx, strings.NewReader(inputGedcom))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.LDSOrdinancesImported != 5 {
		t.Errorf("LDSOrdinancesImported = %d, want 5", result.LDSOrdinancesImported)
	}

	// Build read store from imported data
	readStore := memory.NewReadModelStore()
	for _, p := range persons {
		person := &repository.PersonReadModel{
			ID:        p.ID,
			GivenName: p.GivenName,
			Surname:   p.Surname,
			FullName:  p.GivenName + " " + p.Surname,
			Gender:    p.Gender,
		}
		readStore.SavePerson(ctx, person)
	}

	for _, f := range families {
		fam := &repository.FamilyReadModel{
			ID:         f.ID,
			Partner1ID: f.Partner1ID,
			Partner2ID: f.Partner2ID,
		}
		readStore.SaveFamily(ctx, fam)
	}

	for _, ord := range ldsOrdinances {
		ldsOrd := &repository.LDSOrdinanceReadModel{
			ID:        ord.ID,
			Type:      ord.Type,
			TypeLabel: ord.Type.Label(),
			PersonID:  ord.PersonID,
			FamilyID:  ord.FamilyID,
			DateRaw:   ord.Date,
			Temple:    ord.Temple,
			Status:    ord.Status,
			Place:     ord.Place,
		}
		readStore.SaveLDSOrdinance(ctx, ldsOrd)
	}

	// Export
	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	exportResult, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if exportResult.LDSOrdinancesExported != 5 {
		t.Errorf("LDSOrdinancesExported = %d, want 5", exportResult.LDSOrdinancesExported)
	}

	output := buf.String()

	// Verify all ordinance types are present
	expectedTags := []string{"1 BAPL", "1 CONL", "1 ENDL", "1 SLGC", "1 SLGS"}
	for _, tag := range expectedTags {
		if !strings.Contains(output, tag) {
			t.Errorf("Output should contain %s tag", tag)
		}
	}

	// Verify temple codes preserved
	expectedTemples := []string{"TEMP SL", "TEMP LOGAN", "TEMP MANTI"}
	for _, temple := range expectedTemples {
		if !strings.Contains(output, temple) {
			t.Errorf("Output should contain %s", temple)
		}
	}
}

func TestExport_NegatedEvents(t *testing.T) {
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// Create a person
	personID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "Alice",
		Surname:      "Living",
		FullName:     "Alice Living",
		Gender:       domain.GenderFemale,
		BirthDateRaw: "15 JAN 1990",
	})

	// Create a negated death event (NO DEAT)
	negDeathID := uuid.New()
	readStore.SaveEvent(ctx, &repository.EventReadModel{
		ID:        negDeathID,
		OwnerType: "person",
		OwnerID:   personID,
		FactType:  domain.FactPersonDeath,
		IsNegated: true,
		Version:   1,
	})

	// Create persons for family
	husbandID := uuid.New()
	wifeID := uuid.New()
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        husbandID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Gender:    domain.GenderMale,
	})
	readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        wifeID,
		GivenName: "Jane",
		Surname:   "Smith",
		FullName:  "Jane Smith",
		Gender:    domain.GenderFemale,
	})

	// Create a family
	familyID := uuid.New()
	readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &husbandID,
		Partner2ID:       &wifeID,
		RelationshipType: domain.RelationUnknown,
	})

	// Create a negated marriage event (NO MARR)
	negMarrID := uuid.New()
	readStore.SaveEvent(ctx, &repository.EventReadModel{
		ID:        negMarrID,
		OwnerType: "family",
		OwnerID:   familyID,
		FactType:  domain.FactFamilyMarriage,
		IsNegated: true,
		Version:   1,
	})

	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	result, err := exporter.Export(ctx, buf)
	if err != nil {
		t.Fatal(err)
	}

	// Should have exported the negated events
	if result.EventsExported != 2 {
		t.Errorf("EventsExported = %d, want 2", result.EventsExported)
	}

	output := buf.String()

	// Check for NO DEAT on the individual
	if !strings.Contains(output, "1 NO DEAT\n") {
		t.Error("Output should contain 'NO DEAT' for negated death event")
	}

	// Check for NO MARR on the family
	if !strings.Contains(output, "1 NO MARR\n") {
		t.Error("Output should contain 'NO MARR' for negated marriage event")
	}
}

func TestExport_NegatedEventRoundTrip(t *testing.T) {
	// Import GEDCOM with NO tags, then export and verify NO tags are preserved.
	gedcomInput := `0 HEAD
1 GEDC
2 VERS 7.0
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Alice /Living/
1 SEX F
1 BIRT
2 DATE 15 JAN 1990
1 NO DEAT
0 @I2@ INDI
1 NAME John /Doe/
1 SEX M
0 @I3@ INDI
1 NAME Jane /Smith/
1 SEX F
0 @F1@ FAM
1 HUSB @I2@
1 WIFE @I3@
1 NO MARR
0 TRLR
`
	// Step 1: Import
	importer := gedcom.NewImporter()
	ctx := context.Background()

	_, persons, families, _, _, _, events, _, _, _, _, _, _, err := importer.Import(ctx, strings.NewReader(gedcomInput))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify import produced negated events
	if len(events) != 2 {
		t.Fatalf("Import should produce 2 negated events, got %d", len(events))
	}

	// Step 2: Store in read model
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
		}
		readStore.SavePerson(ctx, pm)
	}

	for _, f := range families {
		fm := &repository.FamilyReadModel{
			ID:               f.ID,
			Partner1ID:       f.Partner1ID,
			Partner2ID:       f.Partner2ID,
			RelationshipType: f.RelationshipType,
			MarriageDateRaw:  f.MarriageDate,
			MarriagePlace:    f.MarriagePlace,
		}
		readStore.SaveFamily(ctx, fm)
	}

	for _, e := range events {
		em := &repository.EventReadModel{
			ID:          e.ID,
			OwnerType:   e.OwnerType,
			OwnerID:     e.OwnerID,
			FactType:    e.FactType,
			DateRaw:     e.Date,
			Place:       e.Place,
			Description: e.Description,
			IsNegated:   e.IsNegated,
			Version:     1,
		}
		readStore.SaveEvent(ctx, em)
	}

	// Step 3: Export
	exporter := gedcom.NewExporter(readStore)
	buf := &bytes.Buffer{}
	_, err = exporter.Export(ctx, buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	// Step 4: Verify NO tags survived the round trip
	if !strings.Contains(output, "1 NO DEAT\n") {
		t.Error("Round-trip should preserve 'NO DEAT' tag")
	}
	if !strings.Contains(output, "1 NO MARR\n") {
		t.Error("Round-trip should preserve 'NO MARR' tag")
	}

	// Verify normal birth still works
	if !strings.Contains(output, "1 BIRT\n") {
		t.Error("Round-trip should preserve normal BIRT event")
	}
	if !strings.Contains(output, "2 DATE 15 JAN 1990\n") {
		t.Error("Round-trip should preserve birth date")
	}
}
