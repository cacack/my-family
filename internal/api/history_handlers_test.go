package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGetGlobalHistory(t *testing.T) {
	server := setupTestServer()

	// Create some entities to generate history
	body := `{"given_name":"John","surname":"Doe"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Get global history
	req = httptest.NewRequest(http.MethodGet, "/api/v1/history", nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify response structure
	if resp["items"] == nil {
		t.Error("Expected items field in response")
	}
	if resp["total"] == nil {
		t.Error("Expected total field in response")
	}
	if resp["limit"] == nil {
		t.Error("Expected limit field in response")
	}
	if resp["offset"] == nil {
		t.Error("Expected offset field in response")
	}
	if resp["has_more"] == nil {
		t.Error("Expected has_more field in response")
	}

	// Verify we have at least one change entry (PersonCreated)
	items := resp["items"].([]any)
	if len(items) < 1 {
		t.Error("Expected at least 1 history entry")
	}

	// Verify first entry structure
	firstEntry := items[0].(map[string]any)
	if firstEntry["id"] == nil {
		t.Error("Expected id in change entry")
	}
	if firstEntry["timestamp"] == nil {
		t.Error("Expected timestamp in change entry")
	}
	if firstEntry["entity_type"] == nil {
		t.Error("Expected entity_type in change entry")
	}
	if firstEntry["entity_id"] == nil {
		t.Error("Expected entity_id in change entry")
	}
	if firstEntry["action"] == nil {
		t.Error("Expected action in change entry")
	}

	// Verify the entry is for the person we created
	if firstEntry["entity_type"] != "person" {
		t.Errorf("entity_type = %v, want person", firstEntry["entity_type"])
	}
	if firstEntry["action"] != "created" {
		t.Errorf("action = %v, want created", firstEntry["action"])
	}
}

func TestGetGlobalHistory_WithPagination(t *testing.T) {
	server := setupTestServer()

	// Create multiple entities
	for i := 0; i < 5; i++ {
		body := `{"given_name":"Person` + string(rune('A'+i)) + `","surname":"Test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)
	}

	// Test with limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?limit=2&offset=0", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have exactly 2 items
	items := resp["items"].([]any)
	if len(items) != 2 {
		t.Errorf("items count = %d, want 2", len(items))
	}

	// Should indicate there are more
	if resp["has_more"].(bool) != true {
		t.Error("has_more should be true")
	}

	// Verify limit and offset are returned
	if int(resp["limit"].(float64)) != 2 {
		t.Errorf("limit = %v, want 2", resp["limit"])
	}
	if int(resp["offset"].(float64)) != 0 {
		t.Errorf("offset = %v, want 0", resp["offset"])
	}
}

func TestGetGlobalHistory_WithEntityTypeFilter(t *testing.T) {
	server := setupTestServer()

	// Create a person and a family
	personBody := `{"given_name":"John","surname":"Doe"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	familyBody := `{"relationship_type":"marriage"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/families", strings.NewReader(familyBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Get history filtered by person
	req = httptest.NewRequest(http.MethodGet, "/api/v1/history?entity_type=person", nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// All entries should be for persons
	items := resp["items"].([]any)
	for _, item := range items {
		entry := item.(map[string]any)
		if entry["entity_type"] != "person" {
			t.Errorf("entity_type = %v, want person", entry["entity_type"])
		}
	}
}

func TestGetGlobalHistory_InvalidEntityType(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?entity_type=invalid", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetGlobalHistory_WithTimeFilters(t *testing.T) {
	server := setupTestServer()

	// Create a person
	body := `{"given_name":"John","surname":"Doe"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Get history with time range
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	params := url.Values{}
	params.Add("from", yesterday.Format(time.RFC3339))
	params.Add("to", tomorrow.Format(time.RFC3339))

	req = httptest.NewRequest(http.MethodGet, "/api/v1/history?"+params.Encode(), nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have entries
	items := resp["items"].([]any)
	if len(items) < 1 {
		t.Error("Expected at least 1 entry in time range")
	}
}

func TestGetGlobalHistory_InvalidTimeFormat(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?from=invalid-date", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetPersonHistory(t *testing.T) {
	server := setupTestServer()

	// Create a person
	body := `{"given_name":"John","surname":"Doe"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Update the person to generate more history
	updateBody := `{"given_name":"Jane","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Get person history
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have at least 2 entries (created + updated)
	items := resp["items"].([]any)
	if len(items) < 2 {
		t.Errorf("Expected at least 2 history entries, got %d", len(items))
	}

	// All entries should be for this person
	for _, item := range items {
		entry := item.(map[string]any)
		if entry["entity_id"] != personID {
			t.Errorf("entity_id = %v, want %v", entry["entity_id"], personID)
		}
	}
}

func TestGetPersonHistory_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/00000000-0000-0000-0000-000000000001/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetPersonHistory_InvalidUUID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/not-a-uuid/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetFamilyHistory(t *testing.T) {
	server := setupTestServer()

	// Create a person to be a partner
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create a family with the person as partner
	body := `{"partner1_id":"` + personID + `","relationship_type":"marriage"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/families", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("Failed to create family: status=%d, body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	familyID := createResp["id"].(string)

	// Get family history
	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/"+familyID+"/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have at least 1 entry (created)
	items := resp["items"].([]any)
	if len(items) < 1 {
		t.Error("Expected at least 1 history entry")
	}

	// Entry should be for this family
	firstEntry := items[0].(map[string]any)
	if firstEntry["entity_id"] != familyID {
		t.Errorf("entity_id = %v, want %v", firstEntry["entity_id"], familyID)
	}
	if firstEntry["entity_type"] != "family" {
		t.Errorf("entity_type = %v, want family", firstEntry["entity_type"])
	}
}

func TestGetFamilyHistory_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/00000000-0000-0000-0000-000000000001/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetFamilyHistory_InvalidUUID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/not-a-uuid/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetSourceHistory(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"title":"Test Source","author":"Test Author","publication_date":"2000"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)

	// Get source history
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/"+sourceID+"/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have at least 1 entry (created)
	items := resp["items"].([]any)
	if len(items) < 1 {
		t.Error("Expected at least 1 history entry")
	}

	// Entry should be for this source
	firstEntry := items[0].(map[string]any)
	if firstEntry["entity_id"] != sourceID {
		t.Errorf("entity_id = %v, want %v", firstEntry["entity_id"], sourceID)
	}
	if firstEntry["entity_type"] != "source" {
		t.Errorf("entity_type = %v, want source", firstEntry["entity_type"])
	}
}

func TestGetSourceHistory_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/00000000-0000-0000-0000-000000000001/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetSourceHistory_InvalidUUID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/not-a-uuid/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHistoryResponseFormat(t *testing.T) {
	server := setupTestServer()

	// Create a person with a specific name
	body := `{"given_name":"Alice","surname":"Johnson","gender":"female"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Get history
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/history", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	items := resp["items"].([]any)
	entry := items[0].(map[string]any)

	// Verify timestamp is in RFC3339 format
	timestamp := entry["timestamp"].(string)
	_, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t.Errorf("timestamp not in RFC3339 format: %v", err)
	}

	// Verify entity_name is present and meaningful
	entityName := entry["entity_name"].(string)
	if entityName == "" {
		t.Error("entity_name should not be empty")
	}
	// Should contain person's name
	if !strings.Contains(entityName, "Alice") && !strings.Contains(entityName, "Johnson") {
		t.Errorf("entity_name = %v, expected to contain Alice or Johnson", entityName)
	}
}
