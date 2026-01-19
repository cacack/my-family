package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupMergeTestServer() (*api.Server, *memory.ReadModelStore, *memory.EventStore) {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	readStore := memory.NewReadModelStore()
	server := api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)
	return server, readStore, eventStore
}

// createMergeTestPerson creates a person via API and returns the ID and version
func createMergeTestPerson(t *testing.T, server *api.Server, givenName, surname string, opts ...string) (string, int64) {
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
	version := int64(resp["version"].(float64))
	return resp["id"].(string), version
}

// TestMergePersons_Success tests successful merge of two persons
func TestMergePersons_Success(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	// Create two persons
	survivorID, survivorVersion := createMergeTestPerson(t, server, "John", "Doe", "birth_date", "1950")
	mergedID, mergedVersion := createMergeTestPerson(t, server, "Johnny", "Doe", "death_date", "2020")

	// Merge persons
	body := `{
		"survivor_id": "` + survivorID + `",
		"merged_id": "` + mergedID + `",
		"survivor_version": ` + intToString(survivorVersion) + `,
		"merged_version": ` + intToString(mergedVersion) + `
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("MergePersons failed: %d - %s", rec.Code, rec.Body.String())
	}

	var resp api.MergePersonsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Person.Id.String() != survivorID {
		t.Errorf("Person.ID = %s, want %s", resp.Person.Id.String(), survivorID)
	}

	// Verify merged person was deleted
	req = httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+mergedID, http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Merged person should be deleted, got status %d", rec.Code)
	}
}

// TestMergePersons_WithFieldResolution tests merge with field resolution
func TestMergePersons_WithFieldResolution(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	survivorID, survivorVersion := createMergeTestPerson(t, server, "John", "Doe", "birth_date", "1950")
	mergedID, mergedVersion := createMergeTestPerson(t, server, "Johnny", "Doe", "birth_date", "1951")

	// Merge with field resolution preferring merged values
	body := `{
		"survivor_id": "` + survivorID + `",
		"merged_id": "` + mergedID + `",
		"survivor_version": ` + intToString(survivorVersion) + `,
		"merged_version": ` + intToString(mergedVersion) + `,
		"field_resolution": {
			"birth_date": "merged"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("MergePersons with field resolution failed: %d - %s", rec.Code, rec.Body.String())
	}

	var resp api.MergePersonsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check that birth_date was taken from merged person
	if resp.Person.BirthDate == nil || resp.Person.BirthDate.Year == nil || *resp.Person.BirthDate.Year != 1951 {
		birthYear := 0
		if resp.Person.BirthDate != nil && resp.Person.BirthDate.Year != nil {
			birthYear = *resp.Person.BirthDate.Year
		}
		t.Errorf("BirthDate.Year = %d, want 1951", birthYear)
	}
}

// TestMergePersons_SamePerson tests that merging a person with themselves fails
func TestMergePersons_SamePerson(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	personID, version := createMergeTestPerson(t, server, "John", "Doe")

	body := `{
		"survivor_id": "` + personID + `",
		"merged_id": "` + personID + `",
		"survivor_version": ` + intToString(version) + `,
		"merged_version": ` + intToString(version) + `
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestMergePersons_SurvivorNotFound tests merge when survivor doesn't exist
func TestMergePersons_SurvivorNotFound(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	mergedID, mergedVersion := createMergeTestPerson(t, server, "John", "Doe")
	nonExistentID := uuid.New().String()

	body := `{
		"survivor_id": "` + nonExistentID + `",
		"merged_id": "` + mergedID + `",
		"survivor_version": 0,
		"merged_version": ` + intToString(mergedVersion) + `
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestMergePersons_MergedNotFound tests merge when merged person doesn't exist
func TestMergePersons_MergedNotFound(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	survivorID, survivorVersion := createMergeTestPerson(t, server, "John", "Doe")
	nonExistentID := uuid.New().String()

	body := `{
		"survivor_id": "` + survivorID + `",
		"merged_id": "` + nonExistentID + `",
		"survivor_version": ` + intToString(survivorVersion) + `,
		"merged_version": 0
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestMergePersons_VersionConflict tests merge with wrong version
func TestMergePersons_VersionConflict(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	survivorID, _ := createMergeTestPerson(t, server, "John", "Doe")
	mergedID, mergedVersion := createMergeTestPerson(t, server, "Johnny", "Doe")

	// Use wrong survivor version
	body := `{
		"survivor_id": "` + survivorID + `",
		"merged_id": "` + mergedID + `",
		"survivor_version": 999,
		"merged_version": ` + intToString(mergedVersion) + `
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

// TestDismissDuplicate_Success tests successful dismissal
func TestDismissDuplicate_Success(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	person1ID, _ := createMergeTestPerson(t, server, "John", "Doe")
	person2ID, _ := createMergeTestPerson(t, server, "Johnny", "Doe")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/"+person1ID+"/"+person2ID+"/dismiss", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestDismissDuplicate_Person1NotFound tests dismissal when person1 doesn't exist
func TestDismissDuplicate_Person1NotFound(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	person2ID, _ := createMergeTestPerson(t, server, "John", "Doe")
	nonExistentID := uuid.New().String()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/"+nonExistentID+"/"+person2ID+"/dismiss", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestDismissDuplicate_Person2NotFound tests dismissal when person2 doesn't exist
func TestDismissDuplicate_Person2NotFound(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	person1ID, _ := createMergeTestPerson(t, server, "John", "Doe")
	nonExistentID := uuid.New().String()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/"+person1ID+"/"+nonExistentID+"/dismiss", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestBatchMergePersons_Success tests successful batch merge
func TestBatchMergePersons_Success(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	// Create pairs of persons to merge
	survivor1ID, survivor1Version := createMergeTestPerson(t, server, "John", "Doe")
	merged1ID, merged1Version := createMergeTestPerson(t, server, "Johnny", "Doe")
	survivor2ID, survivor2Version := createMergeTestPerson(t, server, "Jane", "Smith")
	merged2ID, merged2Version := createMergeTestPerson(t, server, "Janey", "Smith")

	body := `{
		"merges": [
			{
				"survivor_id": "` + survivor1ID + `",
				"merged_id": "` + merged1ID + `",
				"survivor_version": ` + intToString(survivor1Version) + `,
				"merged_version": ` + intToString(merged1Version) + `
			},
			{
				"survivor_id": "` + survivor2ID + `",
				"merged_id": "` + merged2ID + `",
				"survivor_version": ` + intToString(survivor2Version) + `,
				"merged_version": ` + intToString(merged2Version) + `
			}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("BatchMergePersons failed: %d - %s", rec.Code, rec.Body.String())
	}

	var resp api.BatchMergeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Total)
	}
	if resp.Successful != 2 {
		t.Errorf("Successful = %d, want 2", resp.Successful)
	}
	if resp.Failed != 0 {
		t.Errorf("Failed = %d, want 0", resp.Failed)
	}
	if len(resp.Results) != 2 {
		t.Errorf("Results length = %d, want 2", len(resp.Results))
	}
}

// TestBatchMergePersons_PartialSuccess tests batch merge with some failures
func TestBatchMergePersons_PartialSuccess(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	survivorID, survivorVersion := createMergeTestPerson(t, server, "John", "Doe")
	mergedID, mergedVersion := createMergeTestPerson(t, server, "Johnny", "Doe")
	nonExistentID := uuid.New().String()

	body := `{
		"merges": [
			{
				"survivor_id": "` + survivorID + `",
				"merged_id": "` + mergedID + `",
				"survivor_version": ` + intToString(survivorVersion) + `,
				"merged_version": ` + intToString(mergedVersion) + `
			},
			{
				"survivor_id": "` + nonExistentID + `",
				"merged_id": "` + mergedID + `",
				"survivor_version": 0,
				"merged_version": 0
			}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("BatchMergePersons failed: %d - %s", rec.Code, rec.Body.String())
	}

	var resp api.BatchMergeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Successful != 1 {
		t.Errorf("Successful = %d, want 1", resp.Successful)
	}
	if resp.Failed != 1 {
		t.Errorf("Failed = %d, want 1", resp.Failed)
	}
}

// TestBatchMergePersons_EmptyMerges tests batch merge with empty array
func TestBatchMergePersons_EmptyMerges(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	body := `{"merges": []}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestBatchMergePersons_TooManyMerges tests batch merge limit
func TestBatchMergePersons_TooManyMerges(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	// Build a request with 101 merges
	merges := make([]string, 101)
	for i := 0; i < 101; i++ {
		id1 := uuid.New().String()
		id2 := uuid.New().String()
		merges[i] = `{"survivor_id":"` + id1 + `","merged_id":"` + id2 + `","survivor_version":0,"merged_version":0}`
	}
	body := `{"merges":[` + strings.Join(merges, ",") + `]}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestBatchDismissDuplicates_Success tests successful batch dismissal
func TestBatchDismissDuplicates_Success(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	person1ID, _ := createMergeTestPerson(t, server, "John", "Doe")
	person2ID, _ := createMergeTestPerson(t, server, "Johnny", "Doe")
	person3ID, _ := createMergeTestPerson(t, server, "Jane", "Smith")
	person4ID, _ := createMergeTestPerson(t, server, "Janey", "Smith")

	body := `{
		"dismissals": [
			{"person1_id": "` + person1ID + `", "person2_id": "` + person2ID + `"},
			{"person1_id": "` + person3ID + `", "person2_id": "` + person4ID + `"}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/dismiss/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("BatchDismissDuplicates failed: %d - %s", rec.Code, rec.Body.String())
	}

	var resp api.BatchDismissResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Total)
	}
	if resp.Successful != 2 {
		t.Errorf("Successful = %d, want 2", resp.Successful)
	}
	if resp.Failed != 0 {
		t.Errorf("Failed = %d, want 0", resp.Failed)
	}
}

// TestBatchDismissDuplicates_PartialSuccess tests batch dismissal with some failures
func TestBatchDismissDuplicates_PartialSuccess(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	person1ID, _ := createMergeTestPerson(t, server, "John", "Doe")
	person2ID, _ := createMergeTestPerson(t, server, "Johnny", "Doe")
	nonExistentID := uuid.New().String()

	body := `{
		"dismissals": [
			{"person1_id": "` + person1ID + `", "person2_id": "` + person2ID + `"},
			{"person1_id": "` + nonExistentID + `", "person2_id": "` + person2ID + `"}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/dismiss/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("BatchDismissDuplicates failed: %d - %s", rec.Code, rec.Body.String())
	}

	var resp api.BatchDismissResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Successful != 1 {
		t.Errorf("Successful = %d, want 1", resp.Successful)
	}
	if resp.Failed != 1 {
		t.Errorf("Failed = %d, want 1", resp.Failed)
	}
}

// TestBatchDismissDuplicates_EmptyDismissals tests batch dismissal with empty array
func TestBatchDismissDuplicates_EmptyDismissals(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	body := `{"dismissals": []}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/dismiss/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestBatchDismissDuplicates_TooManyDismissals tests batch dismissal limit
func TestBatchDismissDuplicates_TooManyDismissals(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	// Build a request with 101 dismissals
	dismissals := make([]string, 101)
	for i := 0; i < 101; i++ {
		id1 := uuid.New().String()
		id2 := uuid.New().String()
		dismissals[i] = `{"person1_id":"` + id1 + `","person2_id":"` + id2 + `"}`
	}
	body := `{"dismissals":[` + strings.Join(dismissals, ",") + `]}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/dismiss/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestMergePersons_ResponseSchema tests that response matches schema
func TestMergePersons_ResponseSchema(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	survivorID, survivorVersion := createMergeTestPerson(t, server, "John", "Doe")
	mergedID, mergedVersion := createMergeTestPerson(t, server, "Johnny", "Doe")

	body := `{
		"survivor_id": "` + survivorID + `",
		"merged_id": "` + mergedID + `",
		"survivor_version": ` + intToString(survivorVersion) + `,
		"merged_version": ` + intToString(mergedVersion) + `
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	if _, ok := raw["person"]; !ok {
		t.Error("Missing required field: person")
	}
	if _, ok := raw["merge_summary"]; !ok {
		t.Error("Missing required field: merge_summary")
	}

	// Check merge_summary structure
	if summary, ok := raw["merge_summary"].(map[string]interface{}); ok {
		requiredFields := []string{"merged_person_name", "fields_updated", "families_updated", "citations_transferred"}
		for _, field := range requiredFields {
			if _, ok := summary[field]; !ok {
				t.Errorf("merge_summary missing field: %s", field)
			}
		}
	}
}

// TestMergeEndpoints_ContentType tests content type headers
func TestMergeEndpoints_ContentType(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	person1ID, _ := createMergeTestPerson(t, server, "John", "Doe")
	person2ID, _ := createMergeTestPerson(t, server, "Johnny", "Doe")

	// Test dismiss endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/"+person1ID+"/"+person2ID+"/dismiss", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// 204 No Content should not have Content-Type
	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Test batch dismiss endpoint
	person3ID, _ := createMergeTestPerson(t, server, "Jane", "Smith")
	person4ID, _ := createMergeTestPerson(t, server, "Janey", "Smith")
	body := `{"dismissals": [{"person1_id": "` + person3ID + `", "person2_id": "` + person4ID + `"}]}`

	req = httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/dismiss/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}
}

// TestDismissDuplicate_InvalidUUID tests dismissal with invalid UUIDs
func TestDismissDuplicate_InvalidUUID(t *testing.T) {
	server, _, _ := setupMergeTestServer()

	person1ID, _ := createMergeTestPerson(t, server, "John", "Doe")

	// Test invalid person2 UUID
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/duplicates/"+person1ID+"/invalid-uuid/dismiss", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d for invalid UUID", rec.Code, http.StatusBadRequest)
	}
}

// intToString converts int64 to string for JSON
func intToString(i int64) string {
	return strconv.FormatInt(i, 10)
}
