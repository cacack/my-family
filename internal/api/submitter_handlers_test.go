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

// createSubmitter is a test helper that POSTs a submitter and returns its id and version.
func createSubmitter(t *testing.T, server *api.Server, name string) (string, int64) {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q}`, name)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/submitters", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("createSubmitter status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("createSubmitter: failed to parse response: %v", err)
	}
	id, _ := resp["id"].(string)
	version, _ := resp["version"].(float64)
	return id, int64(version)
}

func TestCreateSubmitter(t *testing.T) {
	server := setupTestServer()
	body := `{"name":"Jane Researcher","email":["jane@example.com"],"phone":["555-0100"],"language":"English"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/submitters", strings.NewReader(body))
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
	if resp["name"] != "Jane Researcher" {
		t.Errorf("name = %v, want Jane Researcher", resp["name"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

func TestCreateSubmitter_ValidationError(t *testing.T) {
	server := setupTestServer()
	// Missing required name.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/submitters", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestListSubmitters(t *testing.T) {
	server := setupTestServer()
	createSubmitter(t, server, "Submitter One")
	createSubmitter(t, server, "Submitter Two")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/submitters", http.NoBody)
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
	submitters, ok := resp["submitters"].([]any)
	if !ok {
		t.Fatalf("submitters field missing or wrong type: %T", resp["submitters"])
	}
	if len(submitters) != 2 {
		t.Errorf("submitters count = %d, want 2", len(submitters))
	}
}

func TestGetSubmitter(t *testing.T) {
	server := setupTestServer()
	id, _ := createSubmitter(t, server, "Findable Submitter")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/submitters/"+id, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["name"] != "Findable Submitter" {
		t.Errorf("name = %v, want Findable Submitter", resp["name"])
	}
}

func TestGetSubmitter_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/submitters/"+uuid.NewString(), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateSubmitter(t *testing.T) {
	server := setupTestServer()
	id, version := createSubmitter(t, server, "Original Name")

	body := fmt.Sprintf(`{"name":"Updated Name","version":%d}`, version)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/submitters/"+id, strings.NewReader(body))
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
	if resp["name"] != "Updated Name" {
		t.Errorf("name = %v, want Updated Name", resp["name"])
	}
	if resp["version"].(float64) <= float64(version) {
		t.Errorf("version = %v, want greater than %d", resp["version"], version)
	}
}

func TestUpdateSubmitter_VersionConflict(t *testing.T) {
	server := setupTestServer()
	id, version := createSubmitter(t, server, "Conflict Submitter")

	body := fmt.Sprintf(`{"name":"Stale","version":%d}`, version+99)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/submitters/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestUpdateSubmitter_NotFound(t *testing.T) {
	server := setupTestServer()
	body := `{"name":"Ghost","version":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/submitters/"+uuid.NewString(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteSubmitter(t *testing.T) {
	server := setupTestServer()
	id, version := createSubmitter(t, server, "Deletable Submitter")

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/submitters/%s?version=%d", id, version), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/submitters/"+id, http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteSubmitter_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/submitters/"+uuid.NewString()+"?version=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
