package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupValidationTestServer() (*api.Server, *memory.ReadModelStore) {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	readStore := memory.NewReadModelStore()
	server := api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)
	return server, readStore
}

// addTestPerson creates a person directly in the read store
func addTestPerson(store *memory.ReadModelStore, given, surname, birthDate string) uuid.UUID {
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

// addTestPersonWithDeath creates a person with birth and death dates
func addTestPersonWithDeath(store *memory.ReadModelStore, given, surname, birthDate, deathDate string) uuid.UUID {
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

// ============================================================================
// GetQualityReport Tests
// ============================================================================

// TestGetQualityReport_EmptyDatabase tests GET /quality/report with no data
func TestGetQualityReport_EmptyDatabase(t *testing.T) {
	server, _ := setupValidationTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/report", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.QualityReport
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.TotalIndividuals != 0 {
		t.Errorf("TotalIndividuals = %d, want 0", resp.TotalIndividuals)
	}
	if resp.TotalFamilies != 0 {
		t.Errorf("TotalFamilies = %d, want 0", resp.TotalFamilies)
	}
	if resp.TotalSources != 0 {
		t.Errorf("TotalSources = %d, want 0", resp.TotalSources)
	}
	if resp.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", resp.ErrorCount)
	}
	if resp.WarningCount != 0 {
		t.Errorf("WarningCount = %d, want 0", resp.WarningCount)
	}
}

// TestGetQualityReport_WithData tests GET /quality/report with sample data
func TestGetQualityReport_WithData(t *testing.T) {
	server, readStore := setupValidationTestServer()

	// Add test persons
	addTestPerson(readStore, "John", "Doe", "1950")
	addTestPersonWithDeath(readStore, "Jane", "Doe", "1920", "2000")
	addTestPerson(readStore, "Bob", "Smith", "") // No birth date

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/report", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.QualityReport
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.TotalIndividuals != 3 {
		t.Errorf("TotalIndividuals = %d, want 3", resp.TotalIndividuals)
	}

	// BirthDateCoverage should be 2/3 (approximately 0.67)
	if resp.BirthDateCoverage < 0.6 || resp.BirthDateCoverage > 0.7 {
		t.Errorf("BirthDateCoverage = %.2f, want approximately 0.67", resp.BirthDateCoverage)
	}

	// TopIssues should be a valid array (may or may not have issues)
	if resp.TopIssues == nil {
		t.Error("TopIssues should not be nil")
	}
}

// TestGetQualityReport_ResponseSchema tests that response matches schema
func TestGetQualityReport_ResponseSchema(t *testing.T) {
	server, _ := setupValidationTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/report", http.NoBody)
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
	requiredFields := []string{
		"total_individuals",
		"total_families",
		"total_sources",
		"birth_date_coverage",
		"death_date_coverage",
		"source_coverage",
		"error_count",
		"warning_count",
		"info_count",
		"top_issues",
	}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

// ============================================================================
// GetPersonsDuplicates Tests
// ============================================================================

// TestGetPersonsDuplicates_EmptyDatabase tests GET /validation/duplicates with no data
func TestGetPersonsDuplicates_EmptyDatabase(t *testing.T) {
	server, _ := setupValidationTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/duplicates", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.DuplicatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(resp.Duplicates) != 0 {
		t.Errorf("Duplicates = %d, want 0", len(resp.Duplicates))
	}
	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}
}

// TestGetPersonsDuplicates_WithPotentialDuplicates tests detection of similar names
func TestGetPersonsDuplicates_WithPotentialDuplicates(t *testing.T) {
	server, readStore := setupValidationTestServer()

	// Add persons with similar names - these should match
	addTestPerson(readStore, "John", "Smith", "1950")
	addTestPerson(readStore, "John", "Smith", "1951")
	// Add a different person
	addTestPerson(readStore, "Jane", "Doe", "1960")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/duplicates", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.DuplicatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have at least one potential duplicate pair
	if len(resp.Duplicates) == 0 {
		t.Error("Expected at least one duplicate pair for similar names")
	}

	// If we have duplicates, verify the structure
	if len(resp.Duplicates) > 0 {
		dup := resp.Duplicates[0]
		if dup.Confidence <= 0 || dup.Confidence > 1 {
			t.Errorf("Confidence = %.2f, want between 0 and 1", dup.Confidence)
		}
		if len(dup.MatchReasons) == 0 {
			t.Error("Expected match reasons")
		}
		if dup.Person1Name == "" || dup.Person2Name == "" {
			t.Error("Expected person names to be populated")
		}
	}

	// Total should match the number of pairs
	if resp.Total != len(resp.Duplicates) {
		t.Errorf("Total = %d, want %d", resp.Total, len(resp.Duplicates))
	}
}

// TestGetPersonsDuplicates_Pagination tests pagination parameters
func TestGetPersonsDuplicates_Pagination(t *testing.T) {
	server, readStore := setupValidationTestServer()

	// Add multiple pairs of similar names
	for i := 0; i < 5; i++ {
		addTestPerson(readStore, "John", "Smith", "1950")
		addTestPerson(readStore, "John", "Smith", "1951")
	}

	// Request with limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/duplicates?limit=2&offset=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.DuplicatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should only return up to limit duplicates
	if len(resp.Duplicates) > 2 {
		t.Errorf("Duplicates = %d, want <= 2 (limit)", len(resp.Duplicates))
	}

	// Total should reflect all matches, not just the page
	if resp.Total < len(resp.Duplicates) {
		t.Errorf("Total = %d should be >= Duplicates count %d", resp.Total, len(resp.Duplicates))
	}
}

// TestGetPersonsDuplicates_ResponseSchema tests response structure
func TestGetPersonsDuplicates_ResponseSchema(t *testing.T) {
	server, _ := setupValidationTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/duplicates", http.NoBody)
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
	requiredFields := []string{"duplicates", "total"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

// ============================================================================
// GetValidationIssues Tests
// ============================================================================

// TestGetValidationIssues_EmptyDatabase tests GET /validation/issues with no data
func TestGetValidationIssues_EmptyDatabase(t *testing.T) {
	server, _ := setupValidationTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/validation", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.ValidationIssuesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(resp.Issues) != 0 {
		t.Errorf("Issues = %d, want 0", len(resp.Issues))
	}
	if resp.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", resp.ErrorCount)
	}
	if resp.WarningCount != 0 {
		t.Errorf("WarningCount = %d, want 0", resp.WarningCount)
	}
	if resp.InfoCount != 0 {
		t.Errorf("InfoCount = %d, want 0", resp.InfoCount)
	}
}

// TestGetValidationIssues_WithDateInconsistencies tests detection of date logic issues
func TestGetValidationIssues_WithDateInconsistencies(t *testing.T) {
	server, readStore := setupValidationTestServer()

	// Add a person with death before birth (invalid)
	addTestPersonWithDeath(readStore, "Invalid", "Dates", "2000", "1950")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/validation", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.ValidationIssuesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have at least one issue for death before birth
	if len(resp.Issues) == 0 {
		t.Error("Expected issues for person with death before birth")
	}

	// ErrorCount should reflect the issues
	if resp.ErrorCount == 0 && resp.WarningCount == 0 && resp.InfoCount == 0 {
		t.Error("Expected at least one issue count to be non-zero")
	}
}

// TestGetValidationIssues_SeverityFilter tests filtering by severity
func TestGetValidationIssues_SeverityFilter(t *testing.T) {
	server, readStore := setupValidationTestServer()

	// Add persons that will generate various issues
	addTestPerson(readStore, "John", "Doe", "")                       // Missing birth date
	addTestPerson(readStore, "Jane", "Doe", "")                       // Missing birth date
	addTestPersonWithDeath(readStore, "Bad", "Dates", "2000", "1900") // Death before birth

	// Get all issues first
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/validation", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var allResp api.ValidationIssuesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &allResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Test with severity filter (if there are issues)
	if allResp.ErrorCount > 0 {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/quality/validation?severity=error", http.NoBody)
		rec = httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		var filteredResp api.ValidationIssuesResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &filteredResp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// All returned issues should be errors
		for _, issue := range filteredResp.Issues {
			if issue.Severity != "error" {
				t.Errorf("Issue severity = %s, want error", issue.Severity)
			}
		}
	}
}

// TestGetValidationIssues_ResponseSchema tests response structure
func TestGetValidationIssues_ResponseSchema(t *testing.T) {
	server, _ := setupValidationTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/validation", http.NoBody)
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
	requiredFields := []string{"issues", "error_count", "warning_count", "info_count"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

// TestGetValidationIssues_IssueStructure tests the structure of validation issues
func TestGetValidationIssues_IssueStructure(t *testing.T) {
	server, readStore := setupValidationTestServer()

	// Add a person with issues
	addTestPersonWithDeath(readStore, "Test", "Person", "2000", "1900")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/validation", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp api.ValidationIssuesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// If there are issues, verify the structure
	for _, issue := range resp.Issues {
		if issue.Code == "" {
			t.Error("Issue code should not be empty")
		}
		if issue.Message == "" {
			t.Error("Issue message should not be empty")
		}
		validSeverities := map[api.ValidationIssueSeverity]bool{
			"error":   true,
			"warning": true,
			"info":    true,
		}
		if !validSeverities[issue.Severity] {
			t.Errorf("Invalid severity: %s", issue.Severity)
		}
	}
}

// ============================================================================
// Content-Type Tests
// ============================================================================

// TestValidationEndpoints_ContentType verifies all endpoints return JSON
func TestValidationEndpoints_ContentType(t *testing.T) {
	server, _ := setupValidationTestServer()

	endpoints := []string{
		"/api/v1/quality/report",
		"/api/v1/persons/duplicates",
		"/api/v1/quality/validation",
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
			if contentType != "application/json" && contentType != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %s, want application/json", contentType)
			}
		})
	}
}

// TestGetQualityReport_CoverageBounds tests coverage values are within valid range
func TestGetQualityReport_CoverageBounds(t *testing.T) {
	server, readStore := setupValidationTestServer()

	// Add test data
	addTestPerson(readStore, "John", "Doe", "1950")
	addTestPersonWithDeath(readStore, "Jane", "Doe", "1920", "2000")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality/report", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp api.QualityReport
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Coverage values should be between 0 and 1
	if resp.BirthDateCoverage < 0 || resp.BirthDateCoverage > 1 {
		t.Errorf("BirthDateCoverage = %.2f, want between 0 and 1", resp.BirthDateCoverage)
	}
	if resp.DeathDateCoverage < 0 || resp.DeathDateCoverage > 1 {
		t.Errorf("DeathDateCoverage = %.2f, want between 0 and 1", resp.DeathDateCoverage)
	}
	if resp.SourceCoverage < 0 || resp.SourceCoverage > 1 {
		t.Errorf("SourceCoverage = %.2f, want between 0 and 1", resp.SourceCoverage)
	}
}

// TestGetPersonsDuplicates_HighOffset tests pagination with high offset
func TestGetPersonsDuplicates_HighOffset(t *testing.T) {
	server, readStore := setupValidationTestServer()

	// Add some test data
	addTestPerson(readStore, "John", "Smith", "1950")
	addTestPerson(readStore, "John", "Smith", "1951")

	// Request with offset higher than total
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/duplicates?offset=1000", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp api.DuplicatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should return empty duplicates but total should still reflect actual count
	if len(resp.Duplicates) != 0 {
		t.Errorf("Duplicates = %d, want 0 for high offset", len(resp.Duplicates))
	}
}
