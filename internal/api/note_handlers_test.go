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

// createNote is a test helper that POSTs a note and returns its id and version.
func createNote(t *testing.T, server *api.Server, text string) (string, int64) {
	t.Helper()
	body := fmt.Sprintf(`{"text":%q}`, text)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("createNote status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("createNote: failed to parse response: %v", err)
	}
	id, _ := resp["id"].(string)
	version, _ := resp["version"].(float64)
	return id, int64(version)
}

func TestCreateNote(t *testing.T) {
	server := setupTestServer()
	body := `{"text":"A research note about the Doe family","gedcom_xref":"@N1@"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notes", strings.NewReader(body))
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
	if resp["text"] != "A research note about the Doe family" {
		t.Errorf("text = %v, want the note text", resp["text"])
	}
	if resp["gedcom_xref"] != "@N1@" {
		t.Errorf("gedcom_xref = %v, want @N1@", resp["gedcom_xref"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

func TestCreateNote_MalformedJSON(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notes", strings.NewReader(`{"text":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListNotes(t *testing.T) {
	server := setupTestServer()
	createNote(t, server, "first note")
	createNote(t, server, "second note")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes", http.NoBody)
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
	notes, ok := resp["notes"].([]any)
	if !ok {
		t.Fatalf("notes field missing or wrong type: %T", resp["notes"])
	}
	if len(notes) != 2 {
		t.Errorf("notes count = %d, want 2", len(notes))
	}
}

func TestGetNote(t *testing.T) {
	server := setupTestServer()
	id, _ := createNote(t, server, "findable note")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes/"+id, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["text"] != "findable note" {
		t.Errorf("text = %v, want findable note", resp["text"])
	}
}

func TestGetNote_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes/"+uuid.NewString(), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateNote(t *testing.T) {
	server := setupTestServer()
	id, version := createNote(t, server, "original text")

	body := fmt.Sprintf(`{"text":"updated text","version":%d}`, version)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notes/"+id, strings.NewReader(body))
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
	if resp["text"] != "updated text" {
		t.Errorf("text = %v, want updated text", resp["text"])
	}
	if resp["version"].(float64) <= float64(version) {
		t.Errorf("version = %v, want greater than %d", resp["version"], version)
	}
}

func TestUpdateNote_VersionConflict(t *testing.T) {
	server := setupTestServer()
	id, version := createNote(t, server, "conflict note")

	body := fmt.Sprintf(`{"text":"stale update","version":%d}`, version+99)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notes/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestUpdateNote_NotFound(t *testing.T) {
	server := setupTestServer()
	body := `{"text":"x","version":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notes/"+uuid.NewString(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteNote(t *testing.T) {
	server := setupTestServer()
	id, version := createNote(t, server, "deletable note")

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/notes/%s?version=%d", id, version), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Confirm it is gone.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/notes/"+id, http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteNote_VersionConflict(t *testing.T) {
	server := setupTestServer()
	id, version := createNote(t, server, "guarded note")

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/notes/%s?version=%d", id, version+99), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestDeleteNote_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/notes/"+uuid.NewString()+"?version=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
