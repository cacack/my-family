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

// createRepository is a test helper that POSTs a repository and returns its id and version.
func createRepository(t *testing.T, server *api.Server, name string) (string, int64) {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q}`, name)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/repositories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("createRepository status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("createRepository: failed to parse response: %v", err)
	}
	id, _ := resp["id"].(string)
	version, _ := resp["version"].(float64)
	return id, int64(version)
}

func TestCreateRepository(t *testing.T) {
	server := setupTestServer()
	body := `{"name":"National Archives","address":{"city":"Washington","state":"DC"},"notes":"Primary holdings"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/repositories", strings.NewReader(body))
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
	if resp["name"] != "National Archives" {
		t.Errorf("name = %v, want National Archives", resp["name"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
	addr, ok := resp["address"].(map[string]any)
	if !ok {
		t.Fatalf("address field missing or wrong type: %T", resp["address"])
	}
	if addr["city"] != "Washington" {
		t.Errorf("address.city = %v, want Washington", addr["city"])
	}
}

func TestCreateRepository_ValidationError(t *testing.T) {
	server := setupTestServer()
	// Missing required name.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/repositories", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestListRepositories(t *testing.T) {
	server := setupTestServer()
	createRepository(t, server, "Repository One")
	createRepository(t, server, "Repository Two")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/repositories", http.NoBody)
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
	repositories, ok := resp["repositories"].([]any)
	if !ok {
		t.Fatalf("repositories field missing or wrong type: %T", resp["repositories"])
	}
	if len(repositories) != 2 {
		t.Errorf("repositories count = %d, want 2", len(repositories))
	}
}

func TestGetRepository(t *testing.T) {
	server := setupTestServer()
	id, _ := createRepository(t, server, "Findable Repository")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/"+id, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["name"] != "Findable Repository" {
		t.Errorf("name = %v, want Findable Repository", resp["name"])
	}
}

func TestGetRepository_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/"+uuid.NewString(), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateRepository(t *testing.T) {
	server := setupTestServer()
	id, version := createRepository(t, server, "Original Name")

	body := fmt.Sprintf(`{"name":"Updated Name","version":%d}`, version)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/repositories/"+id, strings.NewReader(body))
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

func TestUpdateRepository_VersionConflict(t *testing.T) {
	server := setupTestServer()
	id, version := createRepository(t, server, "Conflict Repository")

	body := fmt.Sprintf(`{"name":"Stale","version":%d}`, version+99)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/repositories/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestUpdateRepository_NotFound(t *testing.T) {
	server := setupTestServer()
	body := `{"name":"Ghost","version":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/repositories/"+uuid.NewString(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteRepository(t *testing.T) {
	server := setupTestServer()
	id, version := createRepository(t, server, "Deletable Repository")

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/repositories/%s?version=%d", id, version), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/"+id, http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteRepository_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/repositories/"+uuid.NewString()+"?version=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
