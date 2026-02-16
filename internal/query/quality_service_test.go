package query_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// Helper function to create a person read model with specified fields
func createPersonReadModel(id uuid.UUID, givenName, surname string, opts ...func(*repository.PersonReadModel)) repository.PersonReadModel {
	p := repository.PersonReadModel{
		ID:        id,
		GivenName: givenName,
		Surname:   surname,
		FullName:  givenName + " " + surname,
		UpdatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

func withBirthDate(raw string, year int) func(*repository.PersonReadModel) {
	return func(p *repository.PersonReadModel) {
		p.BirthDateRaw = raw
		t := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		p.BirthDateSort = &t
	}
}

func withBirthPlace(place string) func(*repository.PersonReadModel) {
	return func(p *repository.PersonReadModel) {
		p.BirthPlace = place
	}
}

func withDeathDate(raw string, year int) func(*repository.PersonReadModel) {
	return func(p *repository.PersonReadModel) {
		p.DeathDateRaw = raw
		t := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		p.DeathDateSort = &t
	}
}

func withDeathPlace(place string) func(*repository.PersonReadModel) {
	return func(p *repository.PersonReadModel) {
		p.DeathPlace = place
	}
}

func withGender(g domain.Gender) func(*repository.PersonReadModel) {
	return func(p *repository.PersonReadModel) {
		p.Gender = g
	}
}

// TestComputePersonScore_AllFields tests scoring for a person with all fields populated
func TestComputePersonScore_AllFields(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create a person with all fields populated
	personID := uuid.New()
	currentYear := time.Now().Year()
	birthYear := currentYear - 50 // Living person (less than 100 years old)

	person := createPersonReadModel(personID, "John", "Doe",
		withBirthDate("15 MAR "+strconv.Itoa(birthYear), birthYear),
		withBirthPlace("New York, NY"),
	)
	// Living person doesn't need death info
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetPersonQuality(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Living person with birth date and birth place should have score = 100
	// Birth date: 20, Birth place: 15, Death date: 20 (not needed), Death place: 15 (not needed)
	// Total: 70/70 = 100%
	if result.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %.2f, want 100", result.CompletenessScore)
	}
}

// TestComputePersonScore_OnlyBirthDate tests scoring for a person with only birth date
func TestComputePersonScore_OnlyBirthDate(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	personID := uuid.New()
	currentYear := time.Now().Year()
	birthYear := currentYear - 50 // Living person

	person := createPersonReadModel(personID, "John", "Doe",
		withBirthDate("1990", birthYear),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetPersonQuality(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Living person with only birth date:
	// Birth date: 20, Birth place: 0, Death date: 20 (not expected), Death place: 15 (not expected)
	// Total: (20 + 0 + 20 + 15) / 70 * 100 = 55/70 * 100 ~= 78.57
	expectedScore := (20.0 + 0.0 + 20.0 + 15.0) / 70.0 * 100
	if result.CompletenessScore < expectedScore-0.1 || result.CompletenessScore > expectedScore+0.1 {
		t.Errorf("CompletenessScore = %.2f, want ~%.2f", result.CompletenessScore, expectedScore)
	}

	// Should have "Missing birth place" issue
	foundMissingBirthPlace := false
	for _, issue := range result.Issues {
		if issue == "Missing birth place" {
			foundMissingBirthPlace = true
			break
		}
	}
	if !foundMissingBirthPlace {
		t.Error("Expected 'Missing birth place' issue")
	}
}

// TestComputePersonScore_BirthDateAndPlace tests scoring for birth date + place
func TestComputePersonScore_BirthDateAndPlace(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	personID := uuid.New()
	currentYear := time.Now().Year()
	birthYear := currentYear - 50

	person := createPersonReadModel(personID, "John", "Doe",
		withBirthDate("15 MAR "+strconv.Itoa(birthYear), birthYear),
		withBirthPlace("Boston, MA"),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetPersonQuality(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Living person with birth date + place:
	// Birth date: 20, Birth place: 15, Death date: 20 (not expected), Death place: 15 (not expected)
	// Total: 70/70 = 100%
	if result.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %.2f, want 100", result.CompletenessScore)
	}
}

// TestComputePersonScore_LikelyDeceased tests scoring for a person likely deceased
func TestComputePersonScore_LikelyDeceased(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	personID := uuid.New()
	currentYear := time.Now().Year()
	birthYear := currentYear - 150 // > 100 years ago, likely deceased

	person := createPersonReadModel(personID, "John", "Doe",
		withBirthDate("1875", birthYear),
		withBirthPlace("Boston, MA"),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetPersonQuality(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Person born > 100 years ago without death info:
	// Birth date: 20, Birth place: 15, Death date: 0 (expected but missing), Death place: 0 (not expected since no death date)
	// Total: 35/70 = 50%
	expectedScore := (20.0 + 15.0) / 70.0 * 100
	if result.CompletenessScore < expectedScore-0.1 || result.CompletenessScore > expectedScore+0.1 {
		t.Errorf("CompletenessScore = %.2f, want ~%.2f", result.CompletenessScore, expectedScore)
	}

	// Should have "Missing death date (likely deceased)" issue
	foundMissingDeathDate := false
	for _, issue := range result.Issues {
		if issue == "Missing death date (likely deceased)" {
			foundMissingDeathDate = true
			break
		}
	}
	if !foundMissingDeathDate {
		t.Error("Expected 'Missing death date (likely deceased)' issue")
	}
}

// TestComputePersonScore_DeceasedWithDeathDateNoPlace tests death date present but no death place
func TestComputePersonScore_DeceasedWithDeathDateNoPlace(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	personID := uuid.New()
	currentYear := time.Now().Year()
	birthYear := currentYear - 150
	deathYear := currentYear - 80

	person := createPersonReadModel(personID, "John", "Doe",
		withBirthDate("1875", birthYear),
		withBirthPlace("Boston, MA"),
		withDeathDate("1945", deathYear),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetPersonQuality(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Person with birth and death date but no death place:
	// Birth date: 20, Birth place: 15, Death date: 20, Death place: 0
	// Total: 55/70 ~= 78.57%
	expectedScore := (20.0 + 15.0 + 20.0) / 70.0 * 100
	if result.CompletenessScore < expectedScore-0.1 || result.CompletenessScore > expectedScore+0.1 {
		t.Errorf("CompletenessScore = %.2f, want ~%.2f", result.CompletenessScore, expectedScore)
	}

	// Should have "Missing death place" issue
	foundMissingDeathPlace := false
	for _, issue := range result.Issues {
		if issue == "Missing death place" {
			foundMissingDeathPlace = true
			break
		}
	}
	if !foundMissingDeathPlace {
		t.Error("Expected 'Missing death place' issue")
	}
}

// TestComputePersonScore_FullyDocumentedDeceased tests a fully documented deceased person
func TestComputePersonScore_FullyDocumentedDeceased(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	personID := uuid.New()
	currentYear := time.Now().Year()
	birthYear := currentYear - 150
	deathYear := currentYear - 80

	person := createPersonReadModel(personID, "John", "Doe",
		withBirthDate("15 MAR 1875", birthYear),
		withBirthPlace("Boston, MA"),
		withDeathDate("20 DEC 1945", deathYear),
		withDeathPlace("New York, NY"),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetPersonQuality(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Fully documented deceased person should have score = 100
	if result.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %.2f, want 100", result.CompletenessScore)
	}

	// Should have no issues (orphan status is separate)
	// Note: orphan check happens in GetPersonQuality, so we may have that issue
	nonOrphanIssues := 0
	for _, issue := range result.Issues {
		if issue != "No family connections" {
			nonOrphanIssues++
		}
	}
	if nonOrphanIssues > 0 {
		t.Errorf("Expected no issues except possibly orphan, got %d non-orphan issues: %v", nonOrphanIssues, result.Issues)
	}
}

// TestComputePersonScore_TableDriven tests scoring with table-driven tests
func TestComputePersonScore_TableDriven(t *testing.T) {
	currentYear := time.Now().Year()
	livingBirthYear := currentYear - 50
	deceasedBirthYear := currentYear - 150
	deceasedDeathYear := currentYear - 80

	tests := []struct {
		name       string
		person     repository.PersonReadModel
		wantScore  float64
		wantIssues []string
	}{
		{
			name:       "no fields",
			person:     createPersonReadModel(uuid.New(), "John", "Doe"),
			wantScore:  (0 + 0 + 20 + 15) / 70.0 * 100, // No birth info, but living assumed
			wantIssues: []string{"Missing birth date", "Missing birth place"},
		},
		{
			name: "living person - only birth date",
			person: createPersonReadModel(uuid.New(), "John", "Doe",
				withBirthDate("1990", livingBirthYear)),
			wantScore:  (20 + 0 + 20 + 15) / 70.0 * 100,
			wantIssues: []string{"Missing birth place"},
		},
		{
			name: "living person - birth date and place",
			person: createPersonReadModel(uuid.New(), "John", "Doe",
				withBirthDate("1990", livingBirthYear),
				withBirthPlace("Boston, MA")),
			wantScore:  100,
			wantIssues: []string{},
		},
		{
			name: "deceased person - missing death info",
			person: createPersonReadModel(uuid.New(), "John", "Doe",
				withBirthDate("1875", deceasedBirthYear),
				withBirthPlace("Boston, MA")),
			wantScore:  (20 + 15 + 0 + 0) / 70.0 * 100,
			wantIssues: []string{"Missing death date (likely deceased)"},
		},
		{
			name: "deceased person - death date no place",
			person: createPersonReadModel(uuid.New(), "John", "Doe",
				withBirthDate("1875", deceasedBirthYear),
				withBirthPlace("Boston, MA"),
				withDeathDate("1945", deceasedDeathYear)),
			wantScore:  (20 + 15 + 20 + 0) / 70.0 * 100,
			wantIssues: []string{"Missing death place"},
		},
		{
			name: "deceased person - fully documented",
			person: createPersonReadModel(uuid.New(), "John", "Doe",
				withBirthDate("1875", deceasedBirthYear),
				withBirthPlace("Boston, MA"),
				withDeathDate("1945", deceasedDeathYear),
				withDeathPlace("New York, NY")),
			wantScore:  100,
			wantIssues: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readStore := memory.NewReadModelStore()
			service := query.NewQualityService(readStore)
			ctx := context.Background()

			_ = readStore.SavePerson(ctx, &tt.person)

			result, err := service.GetPersonQuality(ctx, tt.person.ID)
			if err != nil {
				t.Fatalf("GetPersonQuality failed: %v", err)
			}

			// Allow small floating point variance
			if result.CompletenessScore < tt.wantScore-0.1 || result.CompletenessScore > tt.wantScore+0.1 {
				t.Errorf("CompletenessScore = %.2f, want ~%.2f", result.CompletenessScore, tt.wantScore)
			}

			// Check issues (excluding orphan which is separate)
			for _, wantIssue := range tt.wantIssues {
				found := false
				for _, gotIssue := range result.Issues {
					if gotIssue == wantIssue {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Missing expected issue: %q, got: %v", wantIssue, result.Issues)
				}
			}
		})
	}
}

// TestGetQualityOverview_EmptyDatabase tests overview with no data
func TestGetQualityOverview_EmptyDatabase(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	result, err := service.GetQualityOverview(ctx)
	if err != nil {
		t.Fatalf("GetQualityOverview failed: %v", err)
	}

	if result.TotalPersons != 0 {
		t.Errorf("TotalPersons = %d, want 0", result.TotalPersons)
	}
	if result.AverageCompleteness != 0 {
		t.Errorf("AverageCompleteness = %.2f, want 0", result.AverageCompleteness)
	}
	if result.RecordsWithIssues != 0 {
		t.Errorf("RecordsWithIssues = %d, want 0", result.RecordsWithIssues)
	}
	if len(result.TopIssues) != 0 {
		t.Errorf("TopIssues length = %d, want 0", len(result.TopIssues))
	}
}

// TestGetQualityOverview_MultiplePersons tests overview with multiple persons
func TestGetQualityOverview_MultiplePersons(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	livingBirthYear := currentYear - 50
	deceasedBirthYear := currentYear - 150
	deceasedDeathYear := currentYear - 80

	// Person 1: Fully documented living (100%)
	p1 := createPersonReadModel(uuid.New(), "John", "Doe",
		withBirthDate("1990", livingBirthYear),
		withBirthPlace("Boston, MA"))
	_ = readStore.SavePerson(ctx, &p1)

	// Person 2: Missing birth place (~78.57%)
	p2 := createPersonReadModel(uuid.New(), "Jane", "Doe",
		withBirthDate("1985", livingBirthYear))
	_ = readStore.SavePerson(ctx, &p2)

	// Person 3: Deceased, fully documented (100%)
	p3 := createPersonReadModel(uuid.New(), "Bob", "Smith",
		withBirthDate("1875", deceasedBirthYear),
		withBirthPlace("New York, NY"),
		withDeathDate("1945", deceasedDeathYear),
		withDeathPlace("Chicago, IL"))
	_ = readStore.SavePerson(ctx, &p3)

	result, err := service.GetQualityOverview(ctx)
	if err != nil {
		t.Fatalf("GetQualityOverview failed: %v", err)
	}

	if result.TotalPersons != 3 {
		t.Errorf("TotalPersons = %d, want 3", result.TotalPersons)
	}

	// Average should be (100 + 78.57 + 100) / 3 ~= 92.86
	expectedAvg := (100 + (55.0 / 70.0 * 100) + 100) / 3
	if result.AverageCompleteness < expectedAvg-1 || result.AverageCompleteness > expectedAvg+1 {
		t.Errorf("AverageCompleteness = %.2f, want ~%.2f", result.AverageCompleteness, expectedAvg)
	}

	// Person 2 has issues
	if result.RecordsWithIssues != 1 {
		t.Errorf("RecordsWithIssues = %d, want 1", result.RecordsWithIssues)
	}

	// Should have "Missing birth place" in top issues
	foundMissingBirthPlace := false
	for _, issue := range result.TopIssues {
		if issue.Issue == "Missing birth place" {
			foundMissingBirthPlace = true
			if issue.Count != 1 {
				t.Errorf("Missing birth place count = %d, want 1", issue.Count)
			}
			break
		}
	}
	if !foundMissingBirthPlace {
		t.Error("Expected 'Missing birth place' in TopIssues")
	}
}

// TestGetQualityOverview_IssueAggregation tests that issues are aggregated correctly
func TestGetQualityOverview_IssueAggregation(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create multiple persons with the same issue
	for i := 0; i < 5; i++ {
		p := createPersonReadModel(uuid.New(), "Person", strconv.Itoa(i))
		// All have missing birth date and place
		_ = readStore.SavePerson(ctx, &p)
	}

	result, err := service.GetQualityOverview(ctx)
	if err != nil {
		t.Fatalf("GetQualityOverview failed: %v", err)
	}

	if result.TotalPersons != 5 {
		t.Errorf("TotalPersons = %d, want 5", result.TotalPersons)
	}

	if result.RecordsWithIssues != 5 {
		t.Errorf("RecordsWithIssues = %d, want 5", result.RecordsWithIssues)
	}

	// Check that "Missing birth date" has count 5
	for _, issue := range result.TopIssues {
		if issue.Issue == "Missing birth date" {
			if issue.Count != 5 {
				t.Errorf("Missing birth date count = %d, want 5", issue.Count)
			}
			break
		}
	}
}

// TestGetPersonQuality_NotFound tests getting quality for non-existent person
func TestGetPersonQuality_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	_, err := service.GetPersonQuality(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestGetPersonQuality_WithSuggestions tests that suggestions are generated
func TestGetPersonQuality_WithSuggestions(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	personID := uuid.New()
	person := createPersonReadModel(personID, "John", "Doe")
	// Missing all info
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetPersonQuality(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Should have suggestions for issues
	if len(result.Suggestions) == 0 {
		t.Error("Expected suggestions to be generated")
	}

	// Check for specific suggestions
	foundBirthDateSuggestion := false
	for _, suggestion := range result.Suggestions {
		if suggestion == "Add birth date from vital records, census, or family sources" {
			foundBirthDateSuggestion = true
			break
		}
	}
	if !foundBirthDateSuggestion {
		t.Errorf("Expected birth date suggestion, got: %v", result.Suggestions)
	}
}

// TestGetPersonQuality_OrphanDetection tests orphan detection
func TestGetPersonQuality_OrphanDetection(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	livingBirthYear := currentYear - 50

	// Create a person without any family connections (orphan)
	orphanID := uuid.New()
	orphan := createPersonReadModel(orphanID, "Orphan", "Person",
		withBirthDate("1990", livingBirthYear),
		withBirthPlace("Boston, MA"))
	_ = readStore.SavePerson(ctx, &orphan)

	result, err := service.GetPersonQuality(ctx, orphanID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Should have "No family connections" issue
	foundOrphanIssue := false
	for _, issue := range result.Issues {
		if issue == "No family connections" {
			foundOrphanIssue = true
			break
		}
	}
	if !foundOrphanIssue {
		t.Errorf("Expected 'No family connections' issue for orphan, got: %v", result.Issues)
	}
}

// TestGetPersonQuality_NotOrphanAsPartner tests that person in family is not orphaned
func TestGetPersonQuality_NotOrphanAsPartner(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	livingBirthYear := currentYear - 50

	// Create person
	personID := uuid.New()
	person := createPersonReadModel(personID, "John", "Doe",
		withBirthDate("1990", livingBirthYear),
		withBirthPlace("Boston, MA"))
	_ = readStore.SavePerson(ctx, &person)

	// Create a family with this person as partner
	familyID := uuid.New()
	family := repository.FamilyReadModel{
		ID:         familyID,
		Partner1ID: &personID,
		UpdatedAt:  time.Now(),
	}
	_ = readStore.SaveFamily(ctx, &family)

	result, err := service.GetPersonQuality(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Should NOT have "No family connections" issue
	for _, issue := range result.Issues {
		if issue == "No family connections" {
			t.Error("Person in family should not be marked as orphan")
		}
	}
}

// TestGetPersonQuality_NotOrphanAsChild tests that person as child in family is not orphaned
func TestGetPersonQuality_NotOrphanAsChild(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	livingBirthYear := currentYear - 50

	// Create parent persons
	parent1ID := uuid.New()
	parent1 := createPersonReadModel(parent1ID, "John", "Doe",
		withBirthDate("1960", livingBirthYear-30),
		withBirthPlace("Boston, MA"))
	_ = readStore.SavePerson(ctx, &parent1)

	// Create child person
	childID := uuid.New()
	child := createPersonReadModel(childID, "Junior", "Doe",
		withBirthDate("1990", livingBirthYear),
		withBirthPlace("Boston, MA"))
	_ = readStore.SavePerson(ctx, &child)

	// Create family
	familyID := uuid.New()
	family := repository.FamilyReadModel{
		ID:         familyID,
		Partner1ID: &parent1ID,
		UpdatedAt:  time.Now(),
	}
	_ = readStore.SaveFamily(ctx, &family)

	// Link child to family
	familyChild := repository.FamilyChildReadModel{
		FamilyID: familyID,
		PersonID: childID,
	}
	_ = readStore.SaveFamilyChild(ctx, &familyChild)

	result, err := service.GetPersonQuality(ctx, childID)
	if err != nil {
		t.Fatalf("GetPersonQuality failed: %v", err)
	}

	// Should NOT have "No family connections" issue
	for _, issue := range result.Issues {
		if issue == "No family connections" {
			t.Error("Child in family should not be marked as orphan")
		}
	}
}

// TestGetStatistics_EmptyDatabase tests statistics with no data
func TestGetStatistics_EmptyDatabase(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	result, err := service.GetStatistics(ctx)
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	if result.TotalPersons != 0 {
		t.Errorf("TotalPersons = %d, want 0", result.TotalPersons)
	}
	if result.TotalFamilies != 0 {
		t.Errorf("TotalFamilies = %d, want 0", result.TotalFamilies)
	}
	if result.DateRange.EarliestBirth != nil {
		t.Errorf("EarliestBirth = %v, want nil", result.DateRange.EarliestBirth)
	}
	if result.DateRange.LatestBirth != nil {
		t.Errorf("LatestBirth = %v, want nil", result.DateRange.LatestBirth)
	}
	if len(result.TopSurnames) != 0 {
		t.Errorf("TopSurnames length = %d, want 0", len(result.TopSurnames))
	}
}

// TestGetStatistics_MultiplePeople tests statistics with multiple persons
func TestGetStatistics_MultiplePeople(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create persons with different attributes
	persons := []struct {
		givenName string
		surname   string
		gender    domain.Gender
		birthYear int
	}{
		{"John", "Smith", domain.GenderMale, 1950},
		{"Jane", "Smith", domain.GenderFemale, 1955},
		{"Bob", "Smith", domain.GenderMale, 1980},
		{"Alice", "Johnson", domain.GenderFemale, 1985},
		{"Charlie", "Brown", domain.GenderUnknown, 1990},
	}

	for _, p := range persons {
		person := createPersonReadModel(uuid.New(), p.givenName, p.surname,
			withBirthDate(strconv.Itoa(p.birthYear), p.birthYear),
			withGender(p.gender))
		_ = readStore.SavePerson(ctx, &person)
	}

	// Create a family
	familyID := uuid.New()
	family := repository.FamilyReadModel{
		ID:        familyID,
		UpdatedAt: time.Now(),
	}
	_ = readStore.SaveFamily(ctx, &family)

	result, err := service.GetStatistics(ctx)
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	// Check total counts
	if result.TotalPersons != 5 {
		t.Errorf("TotalPersons = %d, want 5", result.TotalPersons)
	}
	if result.TotalFamilies != 1 {
		t.Errorf("TotalFamilies = %d, want 1", result.TotalFamilies)
	}

	// Check date range
	if result.DateRange.EarliestBirth == nil || *result.DateRange.EarliestBirth != "1950" {
		t.Errorf("EarliestBirth = %v, want 1950", result.DateRange.EarliestBirth)
	}
	if result.DateRange.LatestBirth == nil || *result.DateRange.LatestBirth != "1990" {
		t.Errorf("LatestBirth = %v, want 1990", result.DateRange.LatestBirth)
	}

	// Check gender distribution
	if result.GenderDistribution.Male != 2 {
		t.Errorf("Male = %d, want 2", result.GenderDistribution.Male)
	}
	if result.GenderDistribution.Female != 2 {
		t.Errorf("Female = %d, want 2", result.GenderDistribution.Female)
	}
	if result.GenderDistribution.Unknown != 1 {
		t.Errorf("Unknown = %d, want 1", result.GenderDistribution.Unknown)
	}

	// Check top surnames - Smith should be first with count 3
	if len(result.TopSurnames) == 0 {
		t.Fatal("Expected TopSurnames to be populated")
	}
	if result.TopSurnames[0].Surname != "Smith" {
		t.Errorf("Top surname = %s, want Smith", result.TopSurnames[0].Surname)
	}
	if result.TopSurnames[0].Count != 3 {
		t.Errorf("Smith count = %d, want 3", result.TopSurnames[0].Count)
	}
}

// TestGetStatistics_TopSurnamesSorted tests that surnames are sorted by count descending
func TestGetStatistics_TopSurnamesSorted(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create persons with different surnames at varying counts
	surnamesCounts := map[string]int{
		"Smith":   5,
		"Johnson": 3,
		"Brown":   1,
		"Davis":   4,
	}

	for surname, count := range surnamesCounts {
		for i := 0; i < count; i++ {
			person := createPersonReadModel(uuid.New(), "Person", surname)
			_ = readStore.SavePerson(ctx, &person)
		}
	}

	result, err := service.GetStatistics(ctx)
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	// Verify sorted by count descending
	if len(result.TopSurnames) < 4 {
		t.Fatalf("Expected at least 4 surnames, got %d", len(result.TopSurnames))
	}

	for i := 1; i < len(result.TopSurnames); i++ {
		if result.TopSurnames[i].Count > result.TopSurnames[i-1].Count {
			t.Errorf("Surnames not sorted by count descending: %s(%d) > %s(%d)",
				result.TopSurnames[i].Surname, result.TopSurnames[i].Count,
				result.TopSurnames[i-1].Surname, result.TopSurnames[i-1].Count)
		}
	}
}

// TestGetStatistics_TopSurnamesLimit tests that only top 10 surnames are returned
func TestGetStatistics_TopSurnamesLimit(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create 15 different surnames
	for i := 0; i < 15; i++ {
		surname := "Surname" + strconv.Itoa(i)
		for j := 0; j <= i; j++ { // Varying counts
			person := createPersonReadModel(uuid.New(), "Person", surname)
			_ = readStore.SavePerson(ctx, &person)
		}
	}

	result, err := service.GetStatistics(ctx)
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	if len(result.TopSurnames) > 10 {
		t.Errorf("TopSurnames should be limited to 10, got %d", len(result.TopSurnames))
	}
}

// ============================================================================
// Discovery Feed tests
// ============================================================================

func withResearchStatus(status domain.ResearchStatus) func(*repository.PersonReadModel) {
	return func(p *repository.PersonReadModel) {
		p.ResearchStatus = status
	}
}

// TestGetDiscoveryFeed_EmptyTree tests the feed with no persons
func TestGetDiscoveryFeed_EmptyTree(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	result, err := service.GetDiscoveryFeed(ctx, 20)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items length = %d, want 0", len(result.Items))
	}
}

// TestGetDiscoveryFeed_MissingBirthDate tests that assessed persons missing birth dates generate suggestions
func TestGetDiscoveryFeed_MissingBirthDate(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create a person that has been assessed (research_status != "unknown") but is missing birth date
	personID := uuid.New()
	person := createPersonReadModel(personID, "John", "Doe",
		withResearchStatus(domain.ResearchStatusProbable),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetDiscoveryFeed(ctx, 20)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	// Should have a missing_data suggestion for birth date
	foundMissingData := false
	for _, item := range result.Items {
		if item.Type == "missing_data" && item.PersonID == personID.String() {
			foundMissingData = true
			if item.Priority != 1 {
				t.Errorf("missing_data priority = %d, want 1", item.Priority)
			}
			break
		}
	}
	if !foundMissingData {
		t.Error("Expected missing_data suggestion for assessed person missing birth date")
	}
}

// TestGetDiscoveryFeed_OrphanedPerson tests that orphaned persons generate suggestions
func TestGetDiscoveryFeed_OrphanedPerson(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	livingBirthYear := currentYear - 50

	// Create a person with no family connections
	personID := uuid.New()
	person := createPersonReadModel(personID, "Lone", "Person",
		withBirthDate("1990", livingBirthYear),
		withBirthPlace("Boston, MA"),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetDiscoveryFeed(ctx, 20)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	// Should have an orphan suggestion
	foundOrphan := false
	for _, item := range result.Items {
		if item.Type == "orphan" && item.PersonID == personID.String() {
			foundOrphan = true
			if item.Priority != 2 {
				t.Errorf("orphan priority = %d, want 2", item.Priority)
			}
			break
		}
	}
	if !foundOrphan {
		t.Error("Expected orphan suggestion for person with no family connections")
	}
}

// TestGetDiscoveryFeed_UnassessedPerson tests that unassessed persons generate suggestions
func TestGetDiscoveryFeed_UnassessedPerson(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	livingBirthYear := currentYear - 50

	// Create a person with default research_status (unknown)
	personID := uuid.New()
	person := createPersonReadModel(personID, "Unknown", "Status",
		withBirthDate("1990", livingBirthYear),
		withBirthPlace("Boston, MA"),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetDiscoveryFeed(ctx, 20)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	// Should have an unassessed suggestion
	foundUnassessed := false
	for _, item := range result.Items {
		if item.Type == "unassessed" && item.PersonID == personID.String() {
			foundUnassessed = true
			if item.Priority != 3 {
				t.Errorf("unassessed priority = %d, want 3", item.Priority)
			}
			break
		}
	}
	if !foundUnassessed {
		t.Error("Expected unassessed suggestion for person with unknown research status")
	}
}

// TestGetDiscoveryFeed_QualityGap tests that low-quality persons generate suggestions
func TestGetDiscoveryFeed_QualityGap(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	deceasedBirthYear := currentYear - 150

	// Create a person with low completeness (has birth date but missing everything else for a deceased person)
	// Birth date: 20, Birth place: 0, Death date: 0, Death place: 0 = 20/70 = 28.6%
	personID := uuid.New()
	person := createPersonReadModel(personID, "Incomplete", "Record",
		withBirthDate("1875", deceasedBirthYear),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetDiscoveryFeed(ctx, 20)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	// Should have a quality_gap suggestion (score is ~28.6%)
	foundQualityGap := false
	for _, item := range result.Items {
		if item.Type == "quality_gap" && item.PersonID == personID.String() {
			foundQualityGap = true
			if item.Priority != 2 {
				t.Errorf("quality_gap priority = %d, want 2", item.Priority)
			}
			break
		}
	}
	if !foundQualityGap {
		t.Error("Expected quality_gap suggestion for person with low completeness score")
	}
}

// TestGetDiscoveryFeed_PriorityOrdering tests that items are sorted by priority
func TestGetDiscoveryFeed_PriorityOrdering(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	deceasedBirthYear := currentYear - 150

	// Create a person that generates priority 1 (missing_data) suggestions
	p1ID := uuid.New()
	p1 := createPersonReadModel(p1ID, "Alice", "Priority1",
		withResearchStatus(domain.ResearchStatusCertain),
		// Missing birth date -> priority 1 missing_data
	)
	_ = readStore.SavePerson(ctx, &p1)

	// Create a person that generates priority 3 (unassessed) suggestion
	p2ID := uuid.New()
	p2 := createPersonReadModel(p2ID, "Bob", "Priority3",
		withBirthDate("1990", currentYear-35),
		withBirthPlace("Boston, MA"),
		// research_status defaults to unknown -> priority 3 unassessed
	)
	_ = readStore.SavePerson(ctx, &p2)

	// Create a person that generates priority 2 (quality_gap) suggestion
	p3ID := uuid.New()
	p3 := createPersonReadModel(p3ID, "Charlie", "Priority2",
		withBirthDate("1875", deceasedBirthYear),
		withResearchStatus(domain.ResearchStatusPossible),
		// Low score, has some data -> priority 2 quality_gap
	)
	_ = readStore.SavePerson(ctx, &p3)

	result, err := service.GetDiscoveryFeed(ctx, 100)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	// Verify priority ordering: all priority 1 before priority 2 before priority 3
	lastPriority := 0
	for _, item := range result.Items {
		if item.Priority < lastPriority {
			t.Errorf("Items not sorted by priority: found priority %d after %d", item.Priority, lastPriority)
			break
		}
		lastPriority = item.Priority
	}
}

// TestGetDiscoveryFeed_LimitParameter tests that the limit parameter works
func TestGetDiscoveryFeed_LimitParameter(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create several persons to generate many suggestions
	for i := 0; i < 10; i++ {
		p := createPersonReadModel(uuid.New(), "Person", strconv.Itoa(i))
		_ = readStore.SavePerson(ctx, &p)
	}

	// Request with limit of 3
	result, err := service.GetDiscoveryFeed(ctx, 3)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	if len(result.Items) > 3 {
		t.Errorf("Items length = %d, want <= 3", len(result.Items))
	}
	// Total should reflect all available suggestions, not just the limited set
	if result.Total <= 3 {
		t.Errorf("Total = %d, expected more than 3 (total available)", result.Total)
	}
}

// TestGetDiscoveryFeed_DefaultLimit tests that zero limit defaults to 20
func TestGetDiscoveryFeed_DefaultLimit(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create 25 persons to exceed default limit
	for i := 0; i < 25; i++ {
		p := createPersonReadModel(uuid.New(), "Person", strconv.Itoa(i))
		_ = readStore.SavePerson(ctx, &p)
	}

	// Request with limit of 0 (should default to 20)
	result, err := service.GetDiscoveryFeed(ctx, 0)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	if len(result.Items) > 20 {
		t.Errorf("Items length = %d, want <= 20 (default limit)", len(result.Items))
	}
}

// TestGetDiscoveryFeed_NoQualityGapForEmptyPerson tests that fully empty persons don't get quality_gap
func TestGetDiscoveryFeed_NoQualityGapForEmptyPerson(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	// Create person with no data at all (hasSomeData = false)
	personID := uuid.New()
	person := createPersonReadModel(personID, "Empty", "Person")
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetDiscoveryFeed(ctx, 20)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	// Should NOT have a quality_gap suggestion for completely empty person
	for _, item := range result.Items {
		if item.Type == "quality_gap" && item.PersonID == personID.String() {
			t.Error("Should not generate quality_gap for person with no data at all")
		}
	}
}

// TestGetDiscoveryFeed_NotOrphanedInFamily tests that persons with families don't get orphan suggestions
func TestGetDiscoveryFeed_NotOrphanedInFamily(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	livingBirthYear := currentYear - 50

	// Create person
	personID := uuid.New()
	person := createPersonReadModel(personID, "Connected", "Person",
		withBirthDate("1990", livingBirthYear),
		withBirthPlace("Boston, MA"),
	)
	_ = readStore.SavePerson(ctx, &person)

	// Create a family with this person
	familyID := uuid.New()
	family := repository.FamilyReadModel{
		ID:         familyID,
		Partner1ID: &personID,
		UpdatedAt:  time.Now(),
	}
	_ = readStore.SaveFamily(ctx, &family)

	result, err := service.GetDiscoveryFeed(ctx, 20)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	// Should NOT have an orphan suggestion
	for _, item := range result.Items {
		if item.Type == "orphan" && item.PersonID == personID.String() {
			t.Error("Person in a family should not get an orphan suggestion")
		}
	}
}

// TestGetDiscoveryFeed_MissingDeathDateForDeceased tests that assessed deceased persons missing death dates get suggestions
func TestGetDiscoveryFeed_MissingDeathDateForDeceased(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewQualityService(readStore)
	ctx := context.Background()

	currentYear := time.Now().Year()
	deceasedBirthYear := currentYear - 150

	// Create an assessed person born >100 years ago with no death date
	personID := uuid.New()
	person := createPersonReadModel(personID, "Old", "Person",
		withBirthDate("1875", deceasedBirthYear),
		withResearchStatus(domain.ResearchStatusProbable),
	)
	_ = readStore.SavePerson(ctx, &person)

	result, err := service.GetDiscoveryFeed(ctx, 20)
	if err != nil {
		t.Fatalf("GetDiscoveryFeed failed: %v", err)
	}

	// Should have a missing_data suggestion for death date
	foundDeathDateSuggestion := false
	for _, item := range result.Items {
		if item.Type == "missing_data" && item.PersonID == personID.String() {
			if item.Title == "Add death date for Old Person" {
				foundDeathDateSuggestion = true
				break
			}
		}
	}
	if !foundDeathDateSuggestion {
		t.Error("Expected missing_data suggestion for death date of assessed deceased person")
	}
}
