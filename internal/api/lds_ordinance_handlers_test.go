package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/google/uuid"
)

// createIndividualOrdinance is a test helper that POSTs an individual (BAPL)
// ordinance for the given person and returns its id and version.
func createIndividualOrdinance(t *testing.T, server *api.Server, personID string) (string, int64) {
	t.Helper()
	body := fmt.Sprintf(`{"type":"BAPL","person_id":%q,"temple":"SLAKE","status":"COMPLETED","date":"1 JAN 1990"}`, personID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lds-ordinances", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("createIndividualOrdinance status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("createIndividualOrdinance: failed to parse response: %v", err)
	}
	id, _ := resp["id"].(string)
	version, _ := resp["version"].(float64)
	return id, int64(version)
}

func TestCreateLDSOrdinance(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")

	body := fmt.Sprintf(`{"type":"BAPL","person_id":%q,"temple":"SLAKE","status":"COMPLETED"}`, personID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lds-ordinances", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["type"] != "BAPL" {
		t.Errorf("type = %v, want BAPL", resp["type"])
	}
	if resp["person_id"] != personID {
		t.Errorf("person_id = %v, want %s", resp["person_id"], personID)
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

// TestCreateLDSOrdinance_MissingPersonForIndividual verifies that an individual
// ordinance type without a person_id is rejected.
func TestCreateLDSOrdinance_MissingPersonForIndividual(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lds-ordinances", strings.NewReader(`{"type":"BAPL"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestCreateLDSOrdinance_InvalidType verifies an unknown ordinance type is rejected.
func TestCreateLDSOrdinance_InvalidType(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lds-ordinances", strings.NewReader(`{"type":"NOPE"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestListLDSOrdinances(t *testing.T) {
	server := setupTestServer()
	p1 := createPerson(t, server, "John", "Doe")
	p2 := createPerson(t, server, "Jane", "Smith")
	createIndividualOrdinance(t, server, p1)
	createIndividualOrdinance(t, server, p2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lds-ordinances", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", resp["total"])
	}
	ordinances, ok := resp["lds_ordinances"].([]any)
	if !ok {
		t.Fatalf("lds_ordinances field missing or wrong type: %T", resp["lds_ordinances"])
	}
	if len(ordinances) != 2 {
		t.Errorf("lds_ordinances count = %d, want 2", len(ordinances))
	}
}

func TestGetLDSOrdinance(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")
	id, _ := createIndividualOrdinance(t, server, personID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lds-ordinances/"+id, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["type"] != "BAPL" {
		t.Errorf("type = %v, want BAPL", resp["type"])
	}
}

func TestGetLDSOrdinance_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lds-ordinances/"+uuid.NewString(), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateLDSOrdinance(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")
	id, version := createIndividualOrdinance(t, server, personID)

	body := fmt.Sprintf(`{"temple":"PROVO","status":"BIC","version":%d}`, version)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lds-ordinances/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["temple"] != "PROVO" {
		t.Errorf("temple = %v, want PROVO", resp["temple"])
	}
	if resp["version"].(float64) <= float64(version) {
		t.Errorf("version = %v, want greater than %d", resp["version"], version)
	}
}

func TestUpdateLDSOrdinance_VersionConflict(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")
	id, version := createIndividualOrdinance(t, server, personID)

	body := fmt.Sprintf(`{"temple":"PROVO","version":%d}`, version+99)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lds-ordinances/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestUpdateLDSOrdinance_NotFound(t *testing.T) {
	server := setupTestServer()
	body := `{"temple":"PROVO","version":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lds-ordinances/"+uuid.NewString(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteLDSOrdinance(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")
	id, version := createIndividualOrdinance(t, server, personID)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/lds-ordinances/%s?version=%d", id, version), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/lds-ordinances/"+id, http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteLDSOrdinance_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/lds-ordinances/"+uuid.NewString()+"?version=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestListLDSOrdinancesForPerson verifies the per-person sub-resource lists
// individual ordinances for an existing person.
func TestListLDSOrdinancesForPerson(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")
	createIndividualOrdinance(t, server, personID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/lds-ordinances", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("ordinance count = %d, want 1", len(items))
	}
}

func TestListLDSOrdinancesForPerson_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+uuid.NewString()+"/lds-ordinances", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestListLDSOrdinancesForFamily verifies the per-family sub-resource lists
// spouse-sealing (SLGS) ordinances for an existing family.
func TestListLDSOrdinancesForFamily(t *testing.T) {
	server := setupTestServer()
	p1 := createPerson(t, server, "John", "Doe")
	p2 := createPerson(t, server, "Jane", "Smith")
	familyID := createFamily(t, server, p1, p2)

	body := fmt.Sprintf(`{"type":"SLGS","family_id":%q,"temple":"SLAKE","status":"COMPLETED"}`, familyID)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/lds-ordinances", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create SLGS status = %d, want %d. Body: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/"+familyID+"/lds-ordinances", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("ordinance count = %d, want 1", len(items))
	}
}

func TestListLDSOrdinancesForFamily_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/"+uuid.NewString()+"/lds-ordinances", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
