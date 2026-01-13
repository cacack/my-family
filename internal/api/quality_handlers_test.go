package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupQualityTestServer() (*api.Server, *memory.ReadModelStore) {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	server := api.NewServer(cfg, eventStore, readStore, nil)
	return server, readStore
}

// createQualityTestPerson creates a person via API and returns the ID
func createQualityTestPerson(t *testing.T, server *api.Server, givenName, surname string, opts ...string) string {
	body := `{"given_name":"` + givenName + `","surname":"` + surname + `"`

	// Parse optional fields from opts
	for i := 0; i < len(opts); i += 2 {
		if i+1 < len(opts) {
			body += `,"` + opts[i] + `":"` + opts[i+1] + `"`
		}
	}
	body += `}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreatePerson failed: %d - %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	return resp["id"].(string)
}

// TestGetQualityOverview_EmptyDatabase tests GET /quality/overview with no data
func TestGetQualityOverview_EmptyDatabase(t *testing.T) {
	server, _ := setupQualityTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/overview", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.QualityOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.TotalPersons != 0 {
		t.Errorf("TotalPersons = %d, want 0", resp.TotalPersons)
	}
	if resp.AverageCompleteness != 0 {
		t.Errorf("AverageCompleteness = %.2f, want 0", resp.AverageCompleteness)
	}
	if resp.RecordsWithIssues != 0 {
		t.Errorf("RecordsWithIssues = %d, want 0", resp.RecordsWithIssues)
	}
	if resp.TopIssues == nil {
		t.Error("TopIssues should not be nil")
	}
}

// TestGetQualityOverview_WithData tests GET /quality/overview with persons
func TestGetQualityOverview_WithData(t *testing.T) {
	server, _ := setupQualityTestServer()

	// Create some test persons
	currentYear := time.Now().Year()
	birthYear := currentYear - 50

	// Fully documented person
	createQualityTestPerson(t, server, "John", "Doe",
		"birth_date", strconv.Itoa(birthYear),
		"birth_place", "New York, NY")

	// Person with missing info
	createQualityTestPerson(t, server, "Jane", "Doe")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/overview", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.QualityOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.TotalPersons != 2 {
		t.Errorf("TotalPersons = %d, want 2", resp.TotalPersons)
	}

	// At least one person has issues
	if resp.RecordsWithIssues < 1 {
		t.Errorf("RecordsWithIssues = %d, want >= 1", resp.RecordsWithIssues)
	}

	// Average should be between 0 and 100
	if resp.AverageCompleteness < 0 || resp.AverageCompleteness > 100 {
		t.Errorf("AverageCompleteness = %.2f, want between 0 and 100", resp.AverageCompleteness)
	}
}

// TestGetQualityOverview_ResponseSchema tests that response matches schema
func TestGetQualityOverview_ResponseSchema(t *testing.T) {
	server, _ := setupQualityTestServer()

	// Create a person to have some data
	createQualityTestPerson(t, server, "John", "Doe")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/overview", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify JSON structure
	var raw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Check required fields exist
	requiredFields := []string{"total_persons", "average_completeness", "records_with_issues", "top_issues"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify top_issues is an array
	topIssues, ok := raw["top_issues"].([]interface{})
	if !ok {
		t.Errorf("top_issues should be an array")
	}

	// If there are issues, verify their structure
	if len(topIssues) > 0 {
		issue, ok := topIssues[0].(map[string]interface{})
		if !ok {
			t.Error("Issue should be an object")
		} else {
			if _, ok := issue["issue"]; !ok {
				t.Error("Issue should have 'issue' field")
			}
			if _, ok := issue["count"]; !ok {
				t.Error("Issue should have 'count' field")
			}
		}
	}
}

// TestGetPersonQuality_ValidUUID tests GET /quality/persons/:id with valid UUID
func TestGetPersonQuality_ValidUUID(t *testing.T) {
	server, _ := setupQualityTestServer()

	// Create a person
	personID := createQualityTestPerson(t, server, "John", "Doe",
		"birth_date", "1990",
		"birth_place", "Boston, MA")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/persons/"+personID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.PersonQualityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.PersonID != personID {
		t.Errorf("PersonID = %s, want %s", resp.PersonID, personID)
	}

	if resp.CompletenessScore < 0 || resp.CompletenessScore > 100 {
		t.Errorf("CompletenessScore = %.2f, want between 0 and 100", resp.CompletenessScore)
	}

	// Issues and Suggestions should be non-nil arrays
	if resp.Issues == nil {
		t.Error("Issues should not be nil")
	}
	if resp.Suggestions == nil {
		t.Error("Suggestions should not be nil")
	}
}

// TestGetPersonQuality_InvalidUUID tests GET /quality/persons/:id with invalid UUID
func TestGetPersonQuality_InvalidUUID(t *testing.T) {
	server, _ := setupQualityTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/persons/invalid-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestGetPersonQuality_NonExistentUUID tests GET /quality/persons/:id with non-existent UUID
func TestGetPersonQuality_NonExistentUUID(t *testing.T) {
	server, _ := setupQualityTestServer()

	// Use a valid UUID format but non-existent
	nonExistentID := uuid.New().String()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/persons/"+nonExistentID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestGetPersonQuality_ResponseSchema tests that response matches PersonQualityResponse schema
func TestGetPersonQuality_ResponseSchema(t *testing.T) {
	server, _ := setupQualityTestServer()

	personID := createQualityTestPerson(t, server, "John", "Doe")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/persons/"+personID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Check required fields
	requiredFields := []string{"person_id", "completeness_score", "issues", "suggestions"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

// TestGetPersonQuality_WithIssues tests that issues and suggestions are populated
func TestGetPersonQuality_WithIssues(t *testing.T) {
	server, _ := setupQualityTestServer()

	// Create a person with missing info to generate issues
	personID := createQualityTestPerson(t, server, "John", "Doe")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/persons/"+personID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp api.PersonQualityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Person with no birth info should have issues
	if len(resp.Issues) == 0 {
		t.Error("Expected issues for person with missing data")
	}

	// Should have suggestions for each issue
	if len(resp.Suggestions) == 0 {
		t.Error("Expected suggestions for person with issues")
	}
}

// TestGetStatistics_EmptyDatabase tests GET /statistics with no data
func TestGetStatistics_EmptyDatabase(t *testing.T) {
	server, _ := setupQualityTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/statistics", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.StatisticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.TotalPersons != 0 {
		t.Errorf("TotalPersons = %d, want 0", resp.TotalPersons)
	}
	if resp.TotalFamilies != 0 {
		t.Errorf("TotalFamilies = %d, want 0", resp.TotalFamilies)
	}
	if resp.DateRange != nil {
		t.Errorf("DateRange = %v, want nil", resp.DateRange)
	}
}

// TestGetStatistics_WithData tests GET /statistics with persons and families
func TestGetStatistics_WithData(t *testing.T) {
	server, readStore := setupQualityTestServer()
	ctx := httptest.NewRequest(http.MethodGet, "/", http.NoBody).Context()

	// Create persons directly via read store for more control
	persons := []struct {
		givenName string
		surname   string
		gender    domain.Gender
		birthYear int
	}{
		{"John", "Smith", domain.GenderMale, 1950},
		{"Jane", "Smith", domain.GenderFemale, 1955},
		{"Bob", "Johnson", domain.GenderMale, 1980},
	}

	for _, p := range persons {
		person := repository.PersonReadModel{
			ID:           uuid.New(),
			GivenName:    p.givenName,
			Surname:      p.surname,
			FullName:     p.givenName + " " + p.surname,
			Gender:       p.gender,
			BirthDateRaw: strconv.Itoa(p.birthYear),
			UpdatedAt:    time.Now(),
		}
		_ = readStore.SavePerson(ctx, &person)
	}

	// Create a family
	family := repository.FamilyReadModel{
		ID:        uuid.New(),
		UpdatedAt: time.Now(),
	}
	_ = readStore.SaveFamily(ctx, &family)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/statistics", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.StatisticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.TotalPersons != 3 {
		t.Errorf("TotalPersons = %d, want 3", resp.TotalPersons)
	}
	if resp.TotalFamilies != 1 {
		t.Errorf("TotalFamilies = %d, want 1", resp.TotalFamilies)
	}

	// Check gender distribution
	if resp.GenderDistribution.Male != 2 {
		t.Errorf("Male = %d, want 2", resp.GenderDistribution.Male)
	}
	if resp.GenderDistribution.Female != 1 {
		t.Errorf("Female = %d, want 1", resp.GenderDistribution.Female)
	}

	// Check top surnames - Smith should be first
	if len(resp.TopSurnames) == 0 {
		t.Fatal("Expected TopSurnames to be populated")
	}
	if resp.TopSurnames[0].Surname != "Smith" {
		t.Errorf("Top surname = %s, want Smith", resp.TopSurnames[0].Surname)
	}
	if resp.TopSurnames[0].Count != 2 {
		t.Errorf("Smith count = %d, want 2", resp.TopSurnames[0].Count)
	}

	// Check date range
	if resp.DateRange == nil {
		t.Fatal("DateRange should not be nil")
	}
	if resp.DateRange.EarliestBirth == nil || *resp.DateRange.EarliestBirth != "1950" {
		t.Errorf("EarliestBirth = %v, want 1950", resp.DateRange.EarliestBirth)
	}
	if resp.DateRange.LatestBirth == nil || *resp.DateRange.LatestBirth != "1980" {
		t.Errorf("LatestBirth = %v, want 1980", resp.DateRange.LatestBirth)
	}
}

// TestGetStatistics_ResponseSchema tests that response matches StatisticsResponse schema
func TestGetStatistics_ResponseSchema(t *testing.T) {
	server, _ := setupQualityTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/statistics", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Check required fields
	requiredFields := []string{"total_persons", "total_families", "top_surnames", "gender_distribution"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify gender_distribution structure
	genderDist, ok := raw["gender_distribution"].(map[string]interface{})
	if !ok {
		t.Error("gender_distribution should be an object")
	} else {
		for _, field := range []string{"male", "female", "unknown"} {
			if _, ok := genderDist[field]; !ok {
				t.Errorf("gender_distribution missing field: %s", field)
			}
		}
	}
}

// TestGetStatistics_TopSurnamesSorted tests that top surnames are sorted by count
func TestGetStatistics_TopSurnamesSorted(t *testing.T) {
	server, readStore := setupQualityTestServer()
	ctx := httptest.NewRequest(http.MethodGet, "/", http.NoBody).Context()

	// Create persons with different surnames
	surnamesCounts := map[string]int{
		"Smith":   5,
		"Johnson": 3,
		"Brown":   1,
		"Davis":   4,
	}

	for surname, count := range surnamesCounts {
		for i := 0; i < count; i++ {
			person := repository.PersonReadModel{
				ID:        uuid.New(),
				GivenName: "Person",
				Surname:   surname,
				FullName:  "Person " + surname,
				UpdatedAt: time.Now(),
			}
			_ = readStore.SavePerson(ctx, &person)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/statistics", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp api.StatisticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify surnames are sorted by count descending
	for i := 1; i < len(resp.TopSurnames); i++ {
		if resp.TopSurnames[i].Count > resp.TopSurnames[i-1].Count {
			t.Errorf("Surnames not sorted by count descending: %s(%d) > %s(%d)",
				resp.TopSurnames[i].Surname, resp.TopSurnames[i].Count,
				resp.TopSurnames[i-1].Surname, resp.TopSurnames[i-1].Count)
		}
	}
}

// TestGetQualityOverview_TopIssuesSorted tests that top issues are sorted by count
func TestGetQualityOverview_TopIssuesSorted(t *testing.T) {
	server, _ := setupQualityTestServer()

	// Create multiple persons with issues
	for i := 0; i < 5; i++ {
		createQualityTestPerson(t, server, "Person", strconv.Itoa(i))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/overview", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp api.QualityOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify issues are sorted by count descending
	for i := 1; i < len(resp.TopIssues); i++ {
		if resp.TopIssues[i].Count > resp.TopIssues[i-1].Count {
			t.Errorf("Issues not sorted by count descending: %s(%d) > %s(%d)",
				resp.TopIssues[i].Issue, resp.TopIssues[i].Count,
				resp.TopIssues[i-1].Issue, resp.TopIssues[i-1].Count)
		}
	}
}

// TestGetPersonQuality_FullyDocumented tests quality for a fully documented person
func TestGetPersonQuality_FullyDocumented(t *testing.T) {
	server, _ := setupQualityTestServer()

	currentYear := time.Now().Year()
	birthYear := currentYear - 50

	// Create a fully documented living person
	personID := createQualityTestPerson(t, server, "John", "Doe",
		"birth_date", strconv.Itoa(birthYear),
		"birth_place", "Boston, MA")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/persons/"+personID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp api.PersonQualityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Fully documented person should have high score (may not be 100 due to orphan status)
	if resp.CompletenessScore < 80 {
		t.Errorf("CompletenessScore = %.2f, want >= 80 for documented person", resp.CompletenessScore)
	}
}

// TestQualityEndpoints_ContentType verifies all endpoints return JSON
func TestQualityEndpoints_ContentType(t *testing.T) {
	server, _ := setupQualityTestServer()

	// Create a person for person-specific endpoint
	personID := createQualityTestPerson(t, server, "John", "Doe")

	endpoints := []string{
		"/api/v1/quality/overview",
		"/api/v1/quality/persons/" + personID,
		"/api/v1/statistics",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint, http.NoBody)
			rec := httptest.NewRecorder()
			server.Echo().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
			}

			contentType := rec.Header().Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				t.Errorf("Content-Type = %s, want application/json", contentType)
			}
		})
	}
}
