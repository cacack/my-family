package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// setupAhnentafelTestData creates a complete 3-generation pedigree:
// Subject (1) -> Father (2), Mother (3)
// Father (2) -> Paternal Grandfather (4), Paternal Grandmother (5)
// Mother (3) -> Maternal Grandfather (6), Maternal Grandmother (7)
func setupAhnentafelTestData(t *testing.T, readStore *memory.ReadModelStore) (subject, father, mother, paternalGF, paternalGM, maternalGF, maternalGM uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	subject = uuid.New()
	father = uuid.New()
	mother = uuid.New()
	paternalGF = uuid.New()
	paternalGM = uuid.New()
	maternalGF = uuid.New()
	maternalGM = uuid.New()

	// Save persons with full data
	persons := []repository.PersonReadModel{
		{ID: subject, GivenName: "Jane", Surname: "Smith", FullName: "Jane Smith", Gender: domain.GenderFemale, BirthDateRaw: "15 MAR 1990", BirthPlace: "Boston, MA"},
		{ID: father, GivenName: "John", Surname: "Smith", FullName: "John Smith", Gender: domain.GenderMale, BirthDateRaw: "10 JUN 1965", BirthPlace: "New York, NY"},
		{ID: mother, GivenName: "Mary", Surname: "Johnson", FullName: "Mary Johnson", Gender: domain.GenderFemale, BirthDateRaw: "22 SEP 1968", BirthPlace: "Chicago, IL"},
		{ID: paternalGF, GivenName: "George", Surname: "Smith", FullName: "George Smith", Gender: domain.GenderMale, BirthDateRaw: "5 JAN 1940", BirthPlace: "Philadelphia, PA", DeathDateRaw: "12 DEC 2015", DeathPlace: "New York, NY"},
		{ID: paternalGM, GivenName: "Elizabeth", Surname: "Brown", FullName: "Elizabeth Brown", Gender: domain.GenderFemale, BirthDateRaw: "18 APR 1942", BirthPlace: "Baltimore, MD"},
		{ID: maternalGF, GivenName: "Robert", Surname: "Johnson", FullName: "Robert Johnson", Gender: domain.GenderMale, BirthDateRaw: "3 NOV 1938", BirthPlace: "Detroit, MI"},
		{ID: maternalGM, GivenName: "Helen", Surname: "Davis", FullName: "Helen Davis", Gender: domain.GenderFemale, BirthDateRaw: "27 JUL 1941", BirthPlace: "Cleveland, OH"},
	}

	for _, p := range persons {
		pm := p
		if err := readStore.SavePerson(ctx, &pm); err != nil {
			t.Fatal(err)
		}
	}

	// Create pedigree edges
	// Subject's parents
	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   subject,
		FatherID:   &father,
		FatherName: "John Smith",
		MotherID:   &mother,
		MotherName: "Mary Johnson",
	}); err != nil {
		t.Fatal(err)
	}

	// Father's parents (paternal grandparents)
	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   father,
		FatherID:   &paternalGF,
		FatherName: "George Smith",
		MotherID:   &paternalGM,
		MotherName: "Elizabeth Brown",
	}); err != nil {
		t.Fatal(err)
	}

	// Mother's parents (maternal grandparents)
	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   mother,
		FatherID:   &maternalGF,
		FatherName: "Robert Johnson",
		MotherID:   &maternalGM,
		MotherName: "Helen Davis",
	}); err != nil {
		t.Fatal(err)
	}

	return
}

func TestGetAhnentafel_CompleteTree(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	subject, father, mother, paternalGF, paternalGM, maternalGF, maternalGM := setupAhnentafelTestData(t, readStore)

	ctx := context.Background()
	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       subject,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have 7 entries: subject + 2 parents + 4 grandparents
	if result.TotalEntries != 7 {
		t.Errorf("TotalEntries = %d, want 7", result.TotalEntries)
	}

	if result.MaxGeneration != 2 {
		t.Errorf("MaxGeneration = %d, want 2", result.MaxGeneration)
	}

	// Verify entries are sorted by Ahnentafel number
	for i := 1; i < len(result.Entries); i++ {
		if result.Entries[i].Number <= result.Entries[i-1].Number {
			t.Errorf("Entries not sorted: [%d].Number=%d <= [%d].Number=%d",
				i, result.Entries[i].Number, i-1, result.Entries[i-1].Number)
		}
	}

	// Build map for easier verification
	byNumber := make(map[int]query.AhnentafelEntry)
	for _, e := range result.Entries {
		byNumber[e.Number] = e
	}

	// Verify Ahnentafel numbering
	testCases := []struct {
		number     int
		expectedID uuid.UUID
		name       string
		generation int
	}{
		{1, subject, "Jane Smith (subject)", 0},
		{2, father, "John Smith (father)", 1},
		{3, mother, "Mary Johnson (mother)", 1},
		{4, paternalGF, "George Smith (paternal grandfather)", 2},
		{5, paternalGM, "Elizabeth Brown (paternal grandmother)", 2},
		{6, maternalGF, "Robert Johnson (maternal grandfather)", 2},
		{7, maternalGM, "Helen Davis (maternal grandmother)", 2},
	}

	for _, tc := range testCases {
		entry, ok := byNumber[tc.number]
		if !ok {
			t.Errorf("Missing entry for Ahnentafel number %d (%s)", tc.number, tc.name)
			continue
		}
		if entry.ID != tc.expectedID {
			t.Errorf("Entry %d: ID = %v, want %v (%s)", tc.number, entry.ID, tc.expectedID, tc.name)
		}
		if entry.Generation != tc.generation {
			t.Errorf("Entry %d: Generation = %d, want %d (%s)", tc.number, entry.Generation, tc.generation, tc.name)
		}
	}
}

func TestGetAhnentafel_NumberingFormula(t *testing.T) {
	// Specifically verify the Ahnentafel formula: father=2N, mother=2N+1
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	subject, father, mother, paternalGF, paternalGM, maternalGF, maternalGM := setupAhnentafelTestData(t, readStore)

	ctx := context.Background()
	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       subject,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	byNumber := make(map[int]query.AhnentafelEntry)
	for _, e := range result.Entries {
		byNumber[e.Number] = e
	}

	// Subject is 1
	if byNumber[1].ID != subject {
		t.Error("Subject should be at position 1")
	}

	// Father of 1 = 2*1 = 2
	if byNumber[2].ID != father {
		t.Error("Father should be at position 2 (2*1)")
	}

	// Mother of 1 = 2*1 + 1 = 3
	if byNumber[3].ID != mother {
		t.Error("Mother should be at position 3 (2*1+1)")
	}

	// Father of 2 = 2*2 = 4 (paternal grandfather)
	if byNumber[4].ID != paternalGF {
		t.Error("Paternal grandfather should be at position 4 (2*2)")
	}

	// Mother of 2 = 2*2 + 1 = 5 (paternal grandmother)
	if byNumber[5].ID != paternalGM {
		t.Error("Paternal grandmother should be at position 5 (2*2+1)")
	}

	// Father of 3 = 2*3 = 6 (maternal grandfather)
	if byNumber[6].ID != maternalGF {
		t.Error("Maternal grandfather should be at position 6 (2*3)")
	}

	// Mother of 3 = 2*3 + 1 = 7 (maternal grandmother)
	if byNumber[7].ID != maternalGM {
		t.Error("Maternal grandmother should be at position 7 (2*3+1)")
	}
}

func TestGetAhnentafel_MissingAncestors(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	ctx := context.Background()

	// Create incomplete tree: subject -> father only (no mother)
	// Father -> paternal grandmother only (no paternal grandfather)
	subject := uuid.New()
	father := uuid.New()
	paternalGM := uuid.New()

	persons := []repository.PersonReadModel{
		{ID: subject, GivenName: "Child", Surname: "Test", FullName: "Child Test", Gender: domain.GenderMale},
		{ID: father, GivenName: "Father", Surname: "Test", FullName: "Father Test", Gender: domain.GenderMale},
		{ID: paternalGM, GivenName: "Grandma", Surname: "Test", FullName: "Grandma Test", Gender: domain.GenderFemale},
	}

	for _, p := range persons {
		pm := p
		if err := readStore.SavePerson(ctx, &pm); err != nil {
			t.Fatal(err)
		}
	}

	// Subject has only father (no mother)
	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   subject,
		FatherID:   &father,
		FatherName: "Father Test",
		// MotherID is nil
	}); err != nil {
		t.Fatal(err)
	}

	// Father has only mother (no father)
	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   father,
		MotherID:   &paternalGM,
		MotherName: "Grandma Test",
		// FatherID is nil
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       subject,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have 3 entries: subject, father, paternal grandmother
	if result.TotalEntries != 3 {
		t.Errorf("TotalEntries = %d, want 3", result.TotalEntries)
	}

	byNumber := make(map[int]query.AhnentafelEntry)
	for _, e := range result.Entries {
		byNumber[e.Number] = e
	}

	// Verify present entries
	if _, ok := byNumber[1]; !ok {
		t.Error("Subject (1) should be present")
	}
	if _, ok := byNumber[2]; !ok {
		t.Error("Father (2) should be present")
	}
	if _, ok := byNumber[5]; !ok {
		t.Error("Paternal grandmother (5) should be present")
	}

	// Verify gaps (missing ancestors)
	if _, ok := byNumber[3]; ok {
		t.Error("Mother (3) should NOT be present (gap expected)")
	}
	if _, ok := byNumber[4]; ok {
		t.Error("Paternal grandfather (4) should NOT be present (gap expected)")
	}
}

func TestGetAhnentafel_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	ctx := context.Background()
	_, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID: uuid.New(), // Non-existent person
	})

	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetAhnentafel_MaxGenerations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	subject, _, _, _, _, _, _ := setupAhnentafelTestData(t, readStore)

	ctx := context.Background()

	// Request only 1 generation (parents only, no grandparents)
	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       subject,
		MaxGenerations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have 3 entries: subject + 2 parents (no grandparents due to limit)
	if result.TotalEntries != 3 {
		t.Errorf("TotalEntries = %d, want 3", result.TotalEntries)
	}

	if result.MaxGeneration != 1 {
		t.Errorf("MaxGeneration = %d, want 1", result.MaxGeneration)
	}

	// Verify no grandparents (numbers 4-7)
	for _, entry := range result.Entries {
		if entry.Number > 3 {
			t.Errorf("Entry %d should not exist with MaxGenerations=1", entry.Number)
		}
	}
}

func TestGetAhnentafel_NoParents(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	ctx := context.Background()

	// Create a person with no parents
	orphan := uuid.New()
	if err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        orphan,
		GivenName: "Orphan",
		Surname:   "Test",
		FullName:  "Orphan Test",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       orphan,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have only 1 entry (the subject)
	if result.TotalEntries != 1 {
		t.Errorf("TotalEntries = %d, want 1", result.TotalEntries)
	}

	if result.MaxGeneration != 0 {
		t.Errorf("MaxGeneration = %d, want 0", result.MaxGeneration)
	}

	if result.Entries[0].Number != 1 {
		t.Errorf("Subject should be at position 1, got %d", result.Entries[0].Number)
	}
}

func TestGetAhnentafel_DefaultMaxGenerations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	subject, _, _, _, _, _, _ := setupAhnentafelTestData(t, readStore)

	ctx := context.Background()

	// Request with 0 max generations (should default to 5)
	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       subject,
		MaxGenerations: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should include all ancestors (test data only has 2 generations)
	if result.TotalEntries != 7 {
		t.Errorf("TotalEntries = %d, want 7", result.TotalEntries)
	}
}

func TestGetAhnentafel_PersonDataFields(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	subject, _, _, paternalGF, _, _, _ := setupAhnentafelTestData(t, readStore)

	ctx := context.Background()
	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       subject,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Find paternal grandfather (number 4) - has death date/place in test data
	var pgfEntry *query.AhnentafelEntry
	for i := range result.Entries {
		if result.Entries[i].ID == paternalGF {
			pgfEntry = &result.Entries[i]
			break
		}
	}

	if pgfEntry == nil {
		t.Fatal("Paternal grandfather entry not found")
	}

	// Verify all fields are populated correctly
	if pgfEntry.Number != 4 {
		t.Errorf("Number = %d, want 4", pgfEntry.Number)
	}
	if pgfEntry.Generation != 2 {
		t.Errorf("Generation = %d, want 2", pgfEntry.Generation)
	}
	if pgfEntry.GivenName != "George" {
		t.Errorf("GivenName = %s, want George", pgfEntry.GivenName)
	}
	if pgfEntry.Surname != "Smith" {
		t.Errorf("Surname = %s, want Smith", pgfEntry.Surname)
	}
	if pgfEntry.Gender != string(domain.GenderMale) {
		t.Errorf("Gender = %s, want %s", pgfEntry.Gender, domain.GenderMale)
	}
	if pgfEntry.BirthDate == nil {
		t.Error("BirthDate should be set")
	}
	if pgfEntry.BirthPlace == nil || *pgfEntry.BirthPlace != "Philadelphia, PA" {
		t.Error("BirthPlace should be 'Philadelphia, PA'")
	}
	if pgfEntry.DeathDate == nil {
		t.Error("DeathDate should be set")
	}
	if pgfEntry.DeathPlace == nil || *pgfEntry.DeathPlace != "New York, NY" {
		t.Error("DeathPlace should be 'New York, NY'")
	}
}

func TestGetAhnentafel_SortOrder(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	subject, _, _, _, _, _, _ := setupAhnentafelTestData(t, readStore)

	ctx := context.Background()
	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       subject,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Entries should be sorted in ascending order by Ahnentafel number
	expectedOrder := []int{1, 2, 3, 4, 5, 6, 7}
	if len(result.Entries) != len(expectedOrder) {
		t.Fatalf("Expected %d entries, got %d", len(expectedOrder), len(result.Entries))
	}

	for i, entry := range result.Entries {
		if entry.Number != expectedOrder[i] {
			t.Errorf("Entry[%d].Number = %d, want %d", i, entry.Number, expectedOrder[i])
		}
	}
}

func TestGetAhnentafel_CycleDetection(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	ctx := context.Background()

	// Create circular reference (shouldn't happen, but should handle gracefully)
	person1 := uuid.New()
	person2 := uuid.New()

	if err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        person1,
		GivenName: "Person1",
		Surname:   "Test",
		FullName:  "Person1 Test",
		Gender:    domain.GenderMale,
	}); err != nil {
		t.Fatal(err)
	}

	if err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        person2,
		GivenName: "Person2",
		Surname:   "Test",
		FullName:  "Person2 Test",
		Gender:    domain.GenderMale,
	}); err != nil {
		t.Fatal(err)
	}

	// Create circular edge
	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   person1,
		FatherID:   &person2,
		FatherName: "Person2 Test",
	}); err != nil {
		t.Fatal(err)
	}

	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   person2,
		FatherID:   &person1,
		FatherName: "Person1 Test",
	}); err != nil {
		t.Fatal(err)
	}

	// Should not infinite loop due to cycle detection in pedigree service
	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       person1,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have at least the subject entry
	if result.TotalEntries < 1 {
		t.Error("Should have at least the subject entry")
	}

	// Due to cycle detection, we should have exactly 2 entries (person1 and person2)
	if result.TotalEntries != 2 {
		t.Errorf("TotalEntries = %d, want 2 (cycle should be detected)", result.TotalEntries)
	}
}

func TestGetAhnentafel_PartialGrandparents(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	ctx := context.Background()

	// Subject with both parents, but only paternal grandparents
	subject := uuid.New()
	father := uuid.New()
	mother := uuid.New()
	paternalGF := uuid.New()
	paternalGM := uuid.New()

	persons := []repository.PersonReadModel{
		{ID: subject, GivenName: "Subject", Surname: "Test", FullName: "Subject Test"},
		{ID: father, GivenName: "Father", Surname: "Test", FullName: "Father Test"},
		{ID: mother, GivenName: "Mother", Surname: "Test", FullName: "Mother Test"},
		{ID: paternalGF, GivenName: "PaternalGF", Surname: "Test", FullName: "PaternalGF Test"},
		{ID: paternalGM, GivenName: "PaternalGM", Surname: "Test", FullName: "PaternalGM Test"},
	}

	for _, p := range persons {
		pm := p
		if err := readStore.SavePerson(ctx, &pm); err != nil {
			t.Fatal(err)
		}
	}

	// Subject's parents
	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   subject,
		FatherID:   &father,
		FatherName: "Father Test",
		MotherID:   &mother,
		MotherName: "Mother Test",
	}); err != nil {
		t.Fatal(err)
	}

	// Only father has parents (mother has none)
	if err := readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   father,
		FatherID:   &paternalGF,
		FatherName: "PaternalGF Test",
		MotherID:   &paternalGM,
		MotherName: "PaternalGM Test",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       subject,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have 5 entries: subject, father, mother, paternal grandfather, paternal grandmother
	if result.TotalEntries != 5 {
		t.Errorf("TotalEntries = %d, want 5", result.TotalEntries)
	}

	byNumber := make(map[int]query.AhnentafelEntry)
	for _, e := range result.Entries {
		byNumber[e.Number] = e
	}

	// Verify present entries
	expectedPresent := []int{1, 2, 3, 4, 5}
	for _, num := range expectedPresent {
		if _, ok := byNumber[num]; !ok {
			t.Errorf("Entry %d should be present", num)
		}
	}

	// Verify gaps (maternal grandparents missing)
	expectedMissing := []int{6, 7}
	for _, num := range expectedMissing {
		if _, ok := byNumber[num]; ok {
			t.Errorf("Entry %d should NOT be present (maternal grandparents missing)", num)
		}
	}
}

func TestNewAhnentafelService(t *testing.T) {
	readStore := memory.NewReadModelStore()
	pedigreeSvc := query.NewPedigreeService(readStore)
	svc := query.NewAhnentafelService(pedigreeSvc)

	if svc == nil {
		t.Error("NewAhnentafelService should return non-nil service")
	}
}
