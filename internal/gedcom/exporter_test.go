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
			name:         "negative evidence",
			evidenceType: domain.EvidenceNegative,
			wantQuality:  "3 QUAY 0",
			factType:     domain.FactPersonDeath,
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
			if !strings.Contains(output, tt.wantQuality) {
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
