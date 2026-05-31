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

// createAssociation is a test helper that POSTs an association between two
// existing persons and returns its id and version.
func createAssociation(t *testing.T, server *api.Server, personID, associateID, role string) (string, int64) {
	t.Helper()
	body := fmt.Sprintf(`{"person_id":%q,"associate_id":%q,"role":%q}`, personID, associateID, role)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/associations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("createAssociation status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("createAssociation: failed to parse response: %v", err)
	}
	id, _ := resp["id"].(string)
	version, _ := resp["version"].(float64)
	return id, int64(version)
}

func TestCreateAssociation(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")
	associateID := createPerson(t, server, "Jane", "Smith")

	body := fmt.Sprintf(`{"person_id":%q,"associate_id":%q,"role":"godparent","phrase":"Baptismal godparent"}`, personID, associateID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/associations", strings.NewReader(body))
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
	if resp["role"] != "godparent" {
		t.Errorf("role = %v, want godparent", resp["role"])
	}
	if resp["person_id"] != personID {
		t.Errorf("person_id = %v, want %s", resp["person_id"], personID)
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

// TestCreateAssociation_UnknownPerson verifies the handler rejects an
// association referencing a person that does not exist.
func TestCreateAssociation_UnknownPerson(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")

	body := fmt.Sprintf(`{"person_id":%q,"associate_id":%q,"role":"witness"}`, personID, uuid.NewString())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/associations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestCreateAssociation_SelfReference verifies a person cannot be associated
// with themselves.
func TestCreateAssociation_SelfReference(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")

	body := fmt.Sprintf(`{"person_id":%q,"associate_id":%q,"role":"witness"}`, personID, personID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/associations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestListAssociations(t *testing.T) {
	server := setupTestServer()
	p1 := createPerson(t, server, "John", "Doe")
	p2 := createPerson(t, server, "Jane", "Smith")
	createAssociation(t, server, p1, p2, "godparent")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/associations", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", resp["total"])
	}
	associations, ok := resp["associations"].([]any)
	if !ok {
		t.Fatalf("associations field missing or wrong type: %T", resp["associations"])
	}
	if len(associations) != 1 {
		t.Errorf("associations count = %d, want 1", len(associations))
	}
}

// TestListAssociationsForPerson verifies the bidirectional per-person lookup:
// an association should surface for both the person and the associate.
func TestListAssociationsForPerson(t *testing.T) {
	server := setupTestServer()
	p1 := createPerson(t, server, "John", "Doe")
	p2 := createPerson(t, server, "Jane", "Smith")
	createAssociation(t, server, p1, p2, "godparent")

	for _, id := range []string{p1, p2} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+id+"/associations", http.NoBody)
		rec := httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("person %s: Status = %d, want %d. Body: %s", id, rec.Code, http.StatusOK, rec.Body.String())
		}
		// The per-person sub-resource returns a bare array, not a paginated envelope.
		var associations []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &associations); err != nil {
			t.Fatalf("person %s: failed to parse response: %v", id, err)
		}
		if len(associations) != 1 {
			t.Errorf("person %s: associations count = %d, want 1", id, len(associations))
		}
	}
}

func TestGetAssociation(t *testing.T) {
	server := setupTestServer()
	p1 := createPerson(t, server, "John", "Doe")
	p2 := createPerson(t, server, "Jane", "Smith")
	id, _ := createAssociation(t, server, p1, p2, "witness")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/associations/"+id, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["role"] != "witness" {
		t.Errorf("role = %v, want witness", resp["role"])
	}
}

func TestGetAssociation_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/associations/"+uuid.NewString(), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateAssociation(t *testing.T) {
	server := setupTestServer()
	p1 := createPerson(t, server, "John", "Doe")
	p2 := createPerson(t, server, "Jane", "Smith")
	id, version := createAssociation(t, server, p1, p2, "witness")

	body := fmt.Sprintf(`{"role":"godparent","version":%d}`, version)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/associations/"+id, strings.NewReader(body))
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
	if resp["role"] != "godparent" {
		t.Errorf("role = %v, want godparent", resp["role"])
	}
	if resp["version"].(float64) <= float64(version) {
		t.Errorf("version = %v, want greater than %d", resp["version"], version)
	}
}

func TestUpdateAssociation_VersionConflict(t *testing.T) {
	server := setupTestServer()
	p1 := createPerson(t, server, "John", "Doe")
	p2 := createPerson(t, server, "Jane", "Smith")
	id, version := createAssociation(t, server, p1, p2, "witness")

	body := fmt.Sprintf(`{"role":"godparent","version":%d}`, version+99)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/associations/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestUpdateAssociation_NotFound(t *testing.T) {
	server := setupTestServer()
	body := `{"role":"witness","version":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/associations/"+uuid.NewString(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteAssociation(t *testing.T) {
	server := setupTestServer()
	p1 := createPerson(t, server, "John", "Doe")
	p2 := createPerson(t, server, "Jane", "Smith")
	id, version := createAssociation(t, server, p1, p2, "witness")

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/associations/%s?version=%d", id, version), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/associations/"+id, http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteAssociation_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/associations/"+uuid.NewString()+"?version=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
