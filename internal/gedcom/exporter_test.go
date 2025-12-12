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
