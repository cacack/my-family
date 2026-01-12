package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Person rollback tests

func TestGetPersonRestorePoints_Success(t *testing.T) {
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

	// Update the person to create more restore points
	updateBody := `{"given_name":"Jane","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Get restore points
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
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
	if resp["has_more"] == nil {
		t.Error("Expected has_more field in response")
	}

	// Should have at least 2 restore points
	items := resp["items"].([]any)
	if len(items) < 2 {
		t.Errorf("Expected at least 2 restore points, got %d", len(items))
	}

	// Verify first restore point structure
	firstPoint := items[0].(map[string]any)
	if firstPoint["version"] == nil {
		t.Error("Expected version in restore point")
	}
	if firstPoint["timestamp"] == nil {
		t.Error("Expected timestamp in restore point")
	}
	if firstPoint["action"] == nil {
		t.Error("Expected action in restore point")
	}
	if firstPoint["summary"] == nil {
		t.Error("Expected summary in restore point")
	}
}

func TestGetPersonRestorePoints_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/not-a-uuid/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetPersonRestorePoints_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/00000000-0000-0000-0000-000000000001/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetPersonRestorePoints_Pagination(t *testing.T) {
	server := setupTestServer()

	// Create a person and update multiple times
	body := `{"given_name":"John","surname":"Doe"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Update multiple times
	for i := 1; i <= 5; i++ {
		updateBody := `{"notes":"Update ` + string(rune('A'+i-1)) + `","version":` + string(rune('0'+i)) + `}`
		updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
		updateReq.Header.Set("Content-Type", "application/json")
		updateRec := httptest.NewRecorder()
		server.Echo().ServeHTTP(updateRec, updateReq)
	}

	// Get first page with limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/restore-points?limit=2&offset=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	items := resp["items"].([]any)
	if len(items) != 2 {
		t.Errorf("items count = %d, want 2", len(items))
	}
	if resp["has_more"].(bool) != true {
		t.Error("has_more should be true")
	}
}

func TestRollbackPerson_Success(t *testing.T) {
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

	// Update the person (version is 2 because creating a person also creates a primary name)
	updateBody := `{"given_name":"Jane","version":2}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Rollback to version 1 (the PersonCreated event only, before NameAdded)
	rollbackBody := `{"target_version":1}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rollbackRec.Code, http.StatusOK, rollbackRec.Body.String())
	}

	var rollbackResp map[string]any
	if err := json.Unmarshal(rollbackRec.Body.Bytes(), &rollbackResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify response structure
	if rollbackResp["entity_id"] != personID {
		t.Errorf("entity_id = %v, want %v", rollbackResp["entity_id"], personID)
	}
	if rollbackResp["entity_type"] != "Person" {
		t.Errorf("entity_type = %v, want Person", rollbackResp["entity_type"])
	}
	// Version sequence: 1 (PersonCreated) -> 2 (NameAdded) -> 3 (PersonUpdated) -> 4 (rollback)
	if rollbackResp["new_version"].(float64) != 4 {
		t.Errorf("new_version = %v, want 4", rollbackResp["new_version"])
	}
	if rollbackResp["message"] == nil {
		t.Error("Expected message in response")
	}

	// Verify the person was restored
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID, http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	var getResp map[string]any
	json.Unmarshal(getRec.Body.Bytes(), &getResp)
	if getResp["given_name"] != "John" {
		t.Errorf("given_name = %v, want John", getResp["given_name"])
	}
}

func TestRollbackPerson_InvalidBody(t *testing.T) {
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

	// Update the person
	updateBody := `{"given_name":"Jane","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Try to rollback with invalid JSON
	rollbackBody := `{invalid json`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}

func TestRollbackPerson_InvalidVersion(t *testing.T) {
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

	// Update the person
	updateBody := `{"given_name":"Jane","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Try to rollback to version 0 (invalid)
	rollbackBody := `{"target_version":0}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}

	// Try to rollback to version higher than current
	rollbackBody = `{"target_version":100}`
	rollbackReq = httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}

func TestRollbackPerson_InvalidID(t *testing.T) {
	server := setupTestServer()

	rollbackBody := `{"target_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/not-a-uuid/rollback", strings.NewReader(rollbackBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRollbackPerson_NotFound(t *testing.T) {
	server := setupTestServer()

	rollbackBody := `{"target_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/00000000-0000-0000-0000-000000000001/rollback", strings.NewReader(rollbackBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRollbackPerson_NoChanges(t *testing.T) {
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

	// Try to rollback to current version (version 2 - after person and name created)
	rollbackBody := `{"target_version":2}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}

// Family rollback tests

func TestGetFamilyRestorePoints_Success(t *testing.T) {
	server := setupTestServer()

	// Create a person to use as partner
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create a family
	body := `{"partner1_id":"` + personID + `","relationship_type":"marriage","marriage_place":"New York"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/families", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	familyID := createResp["id"].(string)

	// Get restore points
	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/"+familyID+"/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGetFamilyRestorePoints_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/not-a-uuid/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetFamilyRestorePoints_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/00000000-0000-0000-0000-000000000001/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRollbackFamily_Success(t *testing.T) {
	server := setupTestServer()

	// Create a person to use as partner
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create a family
	body := `{"partner1_id":"` + personID + `","relationship_type":"marriage","marriage_place":"New York"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/families", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	familyID := createResp["id"].(string)

	// Update the family
	updateBody := `{"marriage_place":"Boston","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/families/"+familyID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Rollback to version 1
	rollbackBody := `{"target_version":1}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/families/"+familyID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rollbackRec.Code, http.StatusOK, rollbackRec.Body.String())
	}
}

func TestRollbackFamily_InvalidID(t *testing.T) {
	server := setupTestServer()

	rollbackBody := `{"target_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/families/not-a-uuid/rollback", strings.NewReader(rollbackBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRollbackFamily_NotFound(t *testing.T) {
	server := setupTestServer()

	rollbackBody := `{"target_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/families/00000000-0000-0000-0000-000000000001/rollback", strings.NewReader(rollbackBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRollbackFamily_InvalidBody(t *testing.T) {
	server := setupTestServer()

	// Create a family
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	body := `{"partner1_id":"` + personID + `","relationship_type":"marriage"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/families", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	familyID := createResp["id"].(string)

	// Update to have version 2
	updateBody := `{"marriage_place":"Boston","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/families/"+familyID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Try to rollback with invalid JSON
	rollbackBody := `{invalid`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/families/"+familyID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}

func TestRollbackFamily_InvalidVersion(t *testing.T) {
	server := setupTestServer()

	// Create a person to use as partner
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create a family with at least one partner
	body := `{"partner1_id":"` + personID + `","relationship_type":"marriage"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/families", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	familyID := createResp["id"].(string)

	// Try to rollback to version 0
	rollbackBody := `{"target_version":0}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/families/"+familyID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}

// Source rollback tests

func TestGetSourceRestorePoints_Success(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"title":"1900 Census","author":"US Census Bureau"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)

	// Get restore points
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/"+sourceID+"/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGetSourceRestorePoints_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/not-a-uuid/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetSourceRestorePoints_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/00000000-0000-0000-0000-000000000001/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRollbackSource_Success(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"title":"1900 Census","author":"US Census Bureau"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)

	// Update the source
	updateBody := `{"title":"1900 Federal Census","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/sources/"+sourceID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Rollback to version 1
	rollbackBody := `{"target_version":1}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources/"+sourceID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rollbackRec.Code, http.StatusOK, rollbackRec.Body.String())
	}
}

func TestRollbackSource_InvalidID(t *testing.T) {
	server := setupTestServer()

	rollbackBody := `{"target_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/not-a-uuid/rollback", strings.NewReader(rollbackBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRollbackSource_NotFound(t *testing.T) {
	server := setupTestServer()

	rollbackBody := `{"target_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/00000000-0000-0000-0000-000000000001/rollback", strings.NewReader(rollbackBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRollbackSource_InvalidBody(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"title":"Test Source"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)

	// Update to have version 2
	updateBody := `{"title":"Updated Source","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/sources/"+sourceID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Try to rollback with invalid JSON
	rollbackBody := `{invalid`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources/"+sourceID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}

func TestRollbackSource_InvalidVersion(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"title":"Test Source"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)

	// Try to rollback to version 0
	rollbackBody := `{"target_version":0}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources/"+sourceID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}

// Citation rollback tests

func TestGetCitationRestorePoints_Success(t *testing.T) {
	server := setupTestServer()

	// Create a source first
	sourceBody := `{"title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	// Create a person for the citation
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create a citation
	body := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `","page":"10"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	citationID := createResp["id"].(string)

	// Get restore points
	req := httptest.NewRequest(http.MethodGet, "/api/v1/citations/"+citationID+"/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGetCitationRestorePoints_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/citations/not-a-uuid/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetCitationRestorePoints_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/citations/00000000-0000-0000-0000-000000000001/restore-points", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRollbackCitation_Success(t *testing.T) {
	server := setupTestServer()

	// Create a source first
	sourceBody := `{"title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	// Create a person for the citation
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create a citation
	body := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `","page":"10"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	citationID := createResp["id"].(string)

	// Update the citation
	updateBody := `{"page":"20","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/citations/"+citationID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Rollback to version 1
	rollbackBody := `{"target_version":1}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations/"+citationID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rollbackRec.Code, http.StatusOK, rollbackRec.Body.String())
	}
}

func TestRollbackCitation_InvalidID(t *testing.T) {
	server := setupTestServer()

	rollbackBody := `{"target_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/citations/not-a-uuid/rollback", strings.NewReader(rollbackBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRollbackCitation_NotFound(t *testing.T) {
	server := setupTestServer()

	rollbackBody := `{"target_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/citations/00000000-0000-0000-0000-000000000001/rollback", strings.NewReader(rollbackBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRollbackCitation_InvalidBody(t *testing.T) {
	server := setupTestServer()

	// Create a source first
	sourceBody := `{"title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	// Create a person for the citation
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create a citation
	body := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	citationID := createResp["id"].(string)

	// Update to have version 2
	updateBody := `{"page":"20","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/citations/"+citationID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	// Try to rollback with invalid JSON
	rollbackBody := `{invalid`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations/"+citationID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}

func TestRollbackCitation_InvalidVersion(t *testing.T) {
	server := setupTestServer()

	// Create a source first
	sourceBody := `{"title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	// Create a person for the citation
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create a citation
	body := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	citationID := createResp["id"].(string)

	// Try to rollback to version 0
	rollbackBody := `{"target_version":0}`
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations/"+citationID+"/rollback", strings.NewReader(rollbackBody))
	rollbackReq.Header.Set("Content-Type", "application/json")
	rollbackRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rollbackRec, rollbackReq)

	if rollbackRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rollbackRec.Code, http.StatusBadRequest)
	}
}
