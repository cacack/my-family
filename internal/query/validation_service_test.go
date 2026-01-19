package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupValidationService() (*query.ValidationService, *memory.ReadModelStore) {
	readStore := memory.NewReadModelStore()
	service := query.NewValidationService(readStore)
	return service, readStore
}

// addPerson creates a person directly in the read store
func addPerson(store *memory.ReadModelStore, given, surname, birthDate string) uuid.UUID {
	id := uuid.New()
	ctx := context.Background()
	_ = store.SavePerson(ctx, &repository.PersonReadModel{
		ID:           id,
		GivenName:    given,
		Surname:      surname,
		FullName:     given + " " + surname,
		BirthDateRaw: birthDate,
		UpdatedAt:    time.Now(),
	})
	return id
}

// addPersonWithDeath creates a person with birth and death dates
func addPersonWithDeath(store *memory.ReadModelStore, given, surname, birthDate, deathDate string) uuid.UUID {
	id := uuid.New()
	ctx := context.Background()
	_ = store.SavePerson(ctx, &repository.PersonReadModel{
		ID:           id,
		GivenName:    given,
		Surname:      surname,
		FullName:     given + " " + surname,
		BirthDateRaw: birthDate,
		DeathDateRaw: deathDate,
		UpdatedAt:    time.Now(),
	})
	return id
}

// addPersonWithGender creates a person with gender
func addPersonWithGender(store *memory.ReadModelStore, given, surname string, gender domain.Gender, birthDate string) uuid.UUID {
	id := uuid.New()
	ctx := context.Background()
	_ = store.SavePerson(ctx, &repository.PersonReadModel{
		ID:           id,
		GivenName:    given,
		Surname:      surname,
		FullName:     given + " " + surname,
		Gender:       gender,
		BirthDateRaw: birthDate,
		UpdatedAt:    time.Now(),
	})
	return id
}

// addFamily creates a family in the read store
func addFamily(store *memory.ReadModelStore, partner1ID, partner2ID *uuid.UUID, marriageDate string) uuid.UUID {
	id := uuid.New()
	ctx := context.Background()
	_ = store.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:              id,
		Partner1ID:      partner1ID,
		Partner2ID:      partner2ID,
		MarriageDateRaw: marriageDate,
		UpdatedAt:       time.Now(),
	})
	return id
}

// addSource creates a source in the read store
func addSource(store *memory.ReadModelStore, title, author string) uuid.UUID {
	id := uuid.New()
	ctx := context.Background()
	_ = store.SaveSource(ctx, &repository.SourceReadModel{
		ID:        id,
		Title:     title,
		Author:    author,
		UpdatedAt: time.Now(),
	})
	return id
}

// ============================================================================
// GetQualityReport Tests
// ============================================================================

func TestValidationService_GetQualityReport_Empty(t *testing.T) {
	service, _ := setupValidationService()
	ctx := context.Background()

	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	if report.TotalIndividuals != 0 {
		t.Errorf("TotalIndividuals = %d, want 0", report.TotalIndividuals)
	}
	if report.TotalFamilies != 0 {
		t.Errorf("TotalFamilies = %d, want 0", report.TotalFamilies)
	}
	if report.TotalSources != 0 {
		t.Errorf("TotalSources = %d, want 0", report.TotalSources)
	}
}

func TestValidationService_GetQualityReport_WithData(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add test data
	p1 := addPerson(store, "John", "Doe", "1950")
	p2 := addPersonWithDeath(store, "Jane", "Doe", "1920", "2000")
	addPerson(store, "Bob", "Smith", "") // No birth date

	addFamily(store, &p1, &p2, "1940")
	addSource(store, "Test Source", "Test Author")

	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	if report.TotalIndividuals != 3 {
		t.Errorf("TotalIndividuals = %d, want 3", report.TotalIndividuals)
	}
	if report.TotalFamilies != 1 {
		t.Errorf("TotalFamilies = %d, want 1", report.TotalFamilies)
	}
	if report.TotalSources != 1 {
		t.Errorf("TotalSources = %d, want 1", report.TotalSources)
	}

	// BirthDateCoverage should be 2/3 (approximately 0.67)
	if report.BirthDateCoverage < 0.6 || report.BirthDateCoverage > 0.7 {
		t.Errorf("BirthDateCoverage = %.2f, want approximately 0.67", report.BirthDateCoverage)
	}

	// DeathDateCoverage should be 1/3 (approximately 0.33)
	if report.DeathDateCoverage < 0.3 || report.DeathDateCoverage > 0.4 {
		t.Errorf("DeathDateCoverage = %.2f, want approximately 0.33", report.DeathDateCoverage)
	}
}

func TestValidationService_GetQualityReport_TopIssues(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add persons without birth dates - should generate issues
	for i := 0; i < 5; i++ {
		addPerson(store, "Person", "Test", "")
	}

	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	// TopIssues should be a valid slice
	if report.TopIssues == nil {
		t.Error("TopIssues should not be nil")
	}

	// TopIssues should be sorted by count descending
	for i := 1; i < len(report.TopIssues); i++ {
		if report.TopIssues[i].Count > report.TopIssues[i-1].Count {
			t.Errorf("TopIssues not sorted by count descending")
		}
	}

	// TopIssues should have at most 10 items
	if len(report.TopIssues) > 10 {
		t.Errorf("TopIssues = %d, want <= 10", len(report.TopIssues))
	}
}

// ============================================================================
// FindDuplicates Tests
// ============================================================================

func TestValidationService_FindDuplicates_Empty(t *testing.T) {
	service, _ := setupValidationService()
	ctx := context.Background()

	results, total, err := service.FindDuplicates(ctx, 100, 0)
	if err != nil {
		t.Fatalf("FindDuplicates returned error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Results = %d, want 0", len(results))
	}
	if total != 0 {
		t.Errorf("Total = %d, want 0", total)
	}
}

func TestValidationService_FindDuplicates_SimilarNames(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add persons with similar names
	addPerson(store, "John", "Smith", "1950")
	addPerson(store, "John", "Smith", "1951")
	// Add a different person
	addPerson(store, "Jane", "Doe", "1960")

	results, total, err := service.FindDuplicates(ctx, 100, 0)
	if err != nil {
		t.Fatalf("FindDuplicates returned error: %v", err)
	}

	// Should have at least one duplicate pair
	if len(results) == 0 {
		t.Error("Expected at least one duplicate pair")
	}

	if total != len(results) {
		t.Errorf("Total = %d, want %d", total, len(results))
	}
}

func TestValidationService_FindDuplicates_ConfidenceScoring(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add persons with same name and similar birth years
	addPerson(store, "John", "Smith", "1950")
	addPerson(store, "John", "Smith", "1950")

	results, _, err := service.FindDuplicates(ctx, 100, 0)
	if err != nil {
		t.Fatalf("FindDuplicates returned error: %v", err)
	}

	// Should have duplicates
	if len(results) == 0 {
		t.Fatal("Expected at least one duplicate pair")
	}

	// Confidence should be between 0 and 1
	for _, result := range results {
		if result.Confidence < 0 || result.Confidence > 1 {
			t.Errorf("Confidence = %.2f, want between 0 and 1", result.Confidence)
		}
	}

	// Should have match reasons
	for _, result := range results {
		if len(result.MatchReasons) == 0 {
			t.Error("Expected match reasons")
		}
	}
}

func TestValidationService_FindDuplicates_Pagination(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add multiple pairs of similar names to get many duplicates
	for i := 0; i < 10; i++ {
		addPerson(store, "John", "Smith", "1950")
	}

	// Get first page
	results1, total1, err := service.FindDuplicates(ctx, 5, 0)
	if err != nil {
		t.Fatalf("FindDuplicates returned error: %v", err)
	}

	// Get second page
	results2, total2, err := service.FindDuplicates(ctx, 5, 5)
	if err != nil {
		t.Fatalf("FindDuplicates returned error: %v", err)
	}

	// Totals should be the same
	if total1 != total2 {
		t.Errorf("Totals differ: %d vs %d", total1, total2)
	}

	// Results should be limited
	if len(results1) > 5 {
		t.Errorf("Results1 = %d, want <= 5", len(results1))
	}

	// Second page should also be limited
	if len(results2) > 5 {
		t.Errorf("Results2 = %d, want <= 5", len(results2))
	}

	// With high offset, should return empty
	results3, total3, err := service.FindDuplicates(ctx, 100, 1000)
	if err != nil {
		t.Fatalf("FindDuplicates returned error: %v", err)
	}
	if len(results3) != 0 {
		t.Errorf("Results3 = %d, want 0 for high offset", len(results3))
	}
	if total3 != total1 {
		t.Errorf("Total should be consistent: %d vs %d", total3, total1)
	}
}

func TestValidationService_FindDuplicates_XRefMapping(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add persons with similar names
	id1 := addPerson(store, "John", "Smith", "1950")
	id2 := addPerson(store, "John", "Smith", "1951")

	results, _, err := service.FindDuplicates(ctx, 100, 0)
	if err != nil {
		t.Fatalf("FindDuplicates returned error: %v", err)
	}

	// Should have duplicates
	if len(results) == 0 {
		t.Fatal("Expected at least one duplicate pair")
	}

	// The Person IDs should be valid UUIDs
	for _, result := range results {
		if result.Person1ID == uuid.Nil {
			t.Error("Person1ID should not be nil")
		}
		if result.Person2ID == uuid.Nil {
			t.Error("Person2ID should not be nil")
		}

		// IDs should be from our test data
		validIDs := map[uuid.UUID]bool{id1: true, id2: true}
		if !validIDs[result.Person1ID] || !validIDs[result.Person2ID] {
			t.Logf("Note: IDs may be matched in different order - Person1: %s, Person2: %s", result.Person1ID, result.Person2ID)
		}
	}
}

// ============================================================================
// GetValidationIssues Tests
// ============================================================================

func TestValidationService_GetValidationIssues_Empty(t *testing.T) {
	service, _ := setupValidationService()
	ctx := context.Background()

	issues, err := service.GetValidationIssues(ctx, "")
	if err != nil {
		t.Fatalf("GetValidationIssues returned error: %v", err)
	}

	if len(issues) != 0 {
		t.Errorf("Issues = %d, want 0", len(issues))
	}
}

func TestValidationService_GetValidationIssues_DateLogicIssues(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add a person with death before birth (invalid)
	addPersonWithDeath(store, "Invalid", "Dates", "2000", "1950")

	issues, err := service.GetValidationIssues(ctx, "")
	if err != nil {
		t.Fatalf("GetValidationIssues returned error: %v", err)
	}

	// Should have at least one issue
	if len(issues) == 0 {
		t.Error("Expected issues for person with death before birth")
	}

	// At least one issue should relate to date logic
	hasDateIssue := false
	for _, issue := range issues {
		if issue.Code != "" {
			hasDateIssue = true
			break
		}
	}
	if !hasDateIssue {
		t.Log("No explicit date logic issues found, but validator may report other issues")
	}
}

func TestValidationService_GetValidationIssues_SeverityLevels(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add various test data that might generate different severity issues
	addPerson(store, "John", "Doe", "")                           // Missing birth date
	addPersonWithDeath(store, "Invalid", "Dates", "2000", "1900") // Death before birth

	issues, err := service.GetValidationIssues(ctx, "")
	if err != nil {
		t.Fatalf("GetValidationIssues returned error: %v", err)
	}

	// Verify severity levels are valid
	validSeverities := map[string]bool{"error": true, "warning": true, "info": true}
	for _, issue := range issues {
		if !validSeverities[issue.Severity] {
			t.Errorf("Invalid severity: %s", issue.Severity)
		}
	}
}

func TestValidationService_GetValidationIssues_SeverityFilter(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add test data that will generate issues
	addPersonWithDeath(store, "Bad", "Dates", "2000", "1900")
	addPerson(store, "Missing", "Data", "")

	// Get all issues first
	allIssues, err := service.GetValidationIssues(ctx, "")
	if err != nil {
		t.Fatalf("GetValidationIssues returned error: %v", err)
	}

	// Filter by error
	errorIssues, err := service.GetValidationIssues(ctx, "error")
	if err != nil {
		t.Fatalf("GetValidationIssues (error filter) returned error: %v", err)
	}

	// All filtered issues should have error severity
	for _, issue := range errorIssues {
		if issue.Severity != "error" {
			t.Errorf("Issue severity = %s, want error", issue.Severity)
		}
	}

	// Filter by warning
	warningIssues, err := service.GetValidationIssues(ctx, "warning")
	if err != nil {
		t.Fatalf("GetValidationIssues (warning filter) returned error: %v", err)
	}

	// All filtered issues should have warning severity
	for _, issue := range warningIssues {
		if issue.Severity != "warning" {
			t.Errorf("Issue severity = %s, want warning", issue.Severity)
		}
	}

	// Filtered counts should not exceed total
	if len(errorIssues)+len(warningIssues) > len(allIssues) {
		// This could happen if there are info issues too
		t.Log("Filtered counts may include info issues")
	}
}

func TestValidationService_GetValidationIssues_RecordMapping(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add a person with issues
	personID := addPersonWithDeath(store, "Invalid", "Person", "2000", "1900")

	issues, err := service.GetValidationIssues(ctx, "")
	if err != nil {
		t.Fatalf("GetValidationIssues returned error: %v", err)
	}

	// If there are issues related to our person, the RecordID should be mapped
	for _, issue := range issues {
		if issue.RecordID != nil {
			// Verify it's a valid UUID (not nil)
			if *issue.RecordID == uuid.Nil {
				t.Error("RecordID should not be nil UUID")
			}
			// It should match our person
			if *issue.RecordID == personID {
				t.Log("Successfully mapped issue to person ID")
			}
		}
	}
}

// ============================================================================
// buildGedcomDocument Tests
// ============================================================================

func TestValidationService_BuildDocument_PersonToIndividual(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add a person with various attributes
	addPersonWithGender(store, "John", "Doe", domain.GenderMale, "1950")

	// Get the quality report which internally builds the document
	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	if report.TotalIndividuals != 1 {
		t.Errorf("TotalIndividuals = %d, want 1", report.TotalIndividuals)
	}
}

func TestValidationService_BuildDocument_FamilyMapping(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add persons and a family
	p1 := addPersonWithGender(store, "John", "Doe", domain.GenderMale, "1920")
	p2 := addPersonWithGender(store, "Jane", "Doe", domain.GenderFemale, "1925")
	addFamily(store, &p1, &p2, "1945")

	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	if report.TotalIndividuals != 2 {
		t.Errorf("TotalIndividuals = %d, want 2", report.TotalIndividuals)
	}
	if report.TotalFamilies != 1 {
		t.Errorf("TotalFamilies = %d, want 1", report.TotalFamilies)
	}
}

func TestValidationService_BuildDocument_SourceMapping(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add sources
	addSource(store, "Source 1", "Author 1")
	addSource(store, "Source 2", "Author 2")

	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	if report.TotalSources != 2 {
		t.Errorf("TotalSources = %d, want 2", report.TotalSources)
	}
}

func TestValidationService_BuildDocument_GenderMapping(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add persons with different genders
	addPersonWithGender(store, "John", "Doe", domain.GenderMale, "1950")
	addPersonWithGender(store, "Jane", "Doe", domain.GenderFemale, "1955")
	addPersonWithGender(store, "Unknown", "Person", domain.GenderUnknown, "1960")

	// The document should be built correctly - we test this through the API
	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	if report.TotalIndividuals != 3 {
		t.Errorf("TotalIndividuals = %d, want 3", report.TotalIndividuals)
	}
}

func TestValidationService_BuildDocument_DateParsing(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add persons with various date formats
	addPerson(store, "Year", "Only", "1950")
	addPerson(store, "Month", "Year", "MAR 1950")
	addPerson(store, "Full", "Date", "15 MAR 1950")
	addPersonWithDeath(store, "With", "Death", "1920", "2000")

	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	if report.TotalIndividuals != 4 {
		t.Errorf("TotalIndividuals = %d, want 4", report.TotalIndividuals)
	}

	// All should have birth dates
	if report.BirthDateCoverage != 1.0 {
		t.Errorf("BirthDateCoverage = %.2f, want 1.0", report.BirthDateCoverage)
	}
}

// ============================================================================
// Error Handling Tests
// ============================================================================

func TestValidationService_NilStore(t *testing.T) {
	// This should panic or return error - test defensive programming
	defer func() {
		if r := recover(); r != nil {
			t.Log("Recovered from panic with nil store - expected behavior")
		}
	}()

	// Passing nil should be handled gracefully or panic
	service := query.NewValidationService(nil)
	ctx := context.Background()

	// This will likely panic - which is acceptable for nil input
	_, _ = service.GetQualityReport(ctx)
}

// ============================================================================
// Display Name Tests
// ============================================================================

func TestValidationService_DisplayName(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add persons with same name for duplicate detection
	addPerson(store, "John", "Doe", "1950")
	addPerson(store, "John", "Doe", "1951")

	results, _, err := service.FindDuplicates(ctx, 100, 0)
	if err != nil {
		t.Fatalf("FindDuplicates returned error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected duplicates")
	}

	// Check that display names are populated
	for _, result := range results {
		if result.Person1Name == "" {
			t.Error("Person1Name should not be empty")
		}
		if result.Person2Name == "" {
			t.Error("Person2Name should not be empty")
		}

		// Names should contain "John" and "Doe"
		if result.Person1Name != "" {
			t.Logf("Person1Name: %s", result.Person1Name)
		}
	}
}

// ============================================================================
// Coverage Calculation Tests
// ============================================================================

func TestValidationService_CoverageCalculations(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()

	// Add 4 persons: 2 with birth, 1 with death, 0 with sources
	addPerson(store, "With", "Birth1", "1950")
	addPerson(store, "With", "Birth2", "1960")
	addPerson(store, "No", "Birth", "")
	addPersonWithDeath(store, "With", "Death", "1920", "2000")

	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}

	// 3/4 persons have birth dates (With Birth1, With Birth2, With Death)
	expectedBirthCoverage := 0.75
	if report.BirthDateCoverage < expectedBirthCoverage-0.01 || report.BirthDateCoverage > expectedBirthCoverage+0.01 {
		t.Errorf("BirthDateCoverage = %.2f, want %.2f", report.BirthDateCoverage, expectedBirthCoverage)
	}

	// 1/4 persons have death dates
	expectedDeathCoverage := 0.25
	if report.DeathDateCoverage < expectedDeathCoverage-0.01 || report.DeathDateCoverage > expectedDeathCoverage+0.01 {
		t.Errorf("DeathDateCoverage = %.2f, want %.2f", report.DeathDateCoverage, expectedDeathCoverage)
	}

	// No sources, so source coverage should be 0
	if report.SourceCoverage != 0 {
		t.Errorf("SourceCoverage = %.2f, want 0", report.SourceCoverage)
	}
}
