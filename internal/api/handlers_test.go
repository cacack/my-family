package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupTestServer() *api.Server {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore)
}

func TestHealthCheck(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("Status = %s, want ok", resp["status"])
	}
}

func TestCreatePerson(t *testing.T) {
	server := setupTestServer()
	body := `{"given_name":"John","surname":"Doe","gender":"male","birth_date":"1 JAN 1990"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["given_name"] != "John" {
		t.Errorf("given_name = %v, want John", resp["given_name"])
	}
	if resp["surname"] != "Doe" {
		t.Errorf("surname = %v, want Doe", resp["surname"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

func TestCreatePerson_ValidationError(t *testing.T) {
	server := setupTestServer()
	// Missing required surname
	body := `{"given_name":"John"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListPersons(t *testing.T) {
	server := setupTestServer()

	// Create a person first
	body := `{"given_name":"John","surname":"Doe"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	// List persons
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", resp["total"])
	}
	items := resp["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items count = %d, want 1", len(items))
	}
}

func TestGetPerson(t *testing.T) {
	server := setupTestServer()

	// Create a person first
	body := `{"given_name":"Jane","surname":"Smith"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Get the person
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID, nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["given_name"] != "Jane" {
		t.Errorf("given_name = %v, want Jane", resp["given_name"])
	}
}

func TestGetPerson_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/00000000-0000-0000-0000-000000000001", nil)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdatePerson(t *testing.T) {
	server := setupTestServer()

	// Create a person first
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

	if updateRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var updateResp map[string]any
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if updateResp["given_name"] != "Jane" {
		t.Errorf("given_name = %v, want Jane", updateResp["given_name"])
	}
}

func TestUpdatePerson_VersionConflict(t *testing.T) {
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

	// Update with wrong version
	updateBody := `{"given_name":"Jane","version":999}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", updateRec.Code, http.StatusConflict)
	}
}

func TestDeletePerson(t *testing.T) {
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

	// Delete the person
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/"+personID+"?version=1", nil)
	deleteRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	// Verify person is gone
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID, nil)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("Status = %d after delete, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestSearchPersons(t *testing.T) {
	server := setupTestServer()

	// Create some persons
	for _, name := range []string{"John Smith", "Jane Smith", "Bob Johnson"} {
		parts := strings.Split(name, " ")
		body := `{"given_name":"` + parts[0] + `","surname":"` + parts[1] + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)
	}

	// Search for Smith
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=Smith", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", resp["total"])
	}
}

func TestSearchPersons_QueryTooShort(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=a", nil)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
