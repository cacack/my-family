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

func setupSnapshotTestServer() *api.Server {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	return api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)
}

func TestCreateSnapshot(t *testing.T) {
	server := setupSnapshotTestServer()
	body := `{"name":"Pre-DNA results","description":"Research state before DNA test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body))
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
	if resp["name"] != "Pre-DNA results" {
		t.Errorf("name = %v, want Pre-DNA results", resp["name"])
	}
	if resp["description"] != "Research state before DNA test" {
		t.Errorf("description = %v, want Research state before DNA test", resp["description"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
	if resp["position"] == nil {
		t.Error("Expected position field")
	}
	if resp["created_at"] == nil {
		t.Error("Expected created_at field")
	}
}

func TestCreateSnapshot_NoDescription(t *testing.T) {
	server := setupSnapshotTestServer()
	body := `{"name":"Milestone"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body))
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
	if resp["name"] != "Milestone" {
		t.Errorf("name = %v, want Milestone", resp["name"])
	}
	// Description should be absent or null
	if resp["description"] != nil && resp["description"] != "" {
		t.Errorf("description = %v, want empty or nil", resp["description"])
	}
}

func TestCreateSnapshot_ValidationError(t *testing.T) {
	server := setupSnapshotTestServer()
	// Empty name should fail
	body := `{"name":"","description":"Test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestListSnapshots(t *testing.T) {
	server := setupSnapshotTestServer()

	// Create some snapshots
	for _, name := range []string{"First", "Second", "Third"} {
		body := `{"name":"` + name + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)
	}

	// List snapshots
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["total"].(float64) != 3 {
		t.Errorf("total = %v, want 3", resp["total"])
	}
	items := resp["items"].([]any)
	if len(items) != 3 {
		t.Errorf("items count = %d, want 3", len(items))
	}

	// Verify order (newest first, so Third should be first)
	firstItem := items[0].(map[string]any)
	if firstItem["name"] != "Third" {
		t.Errorf("first item name = %v, want Third (newest)", firstItem["name"])
	}
}

func TestListSnapshots_Empty(t *testing.T) {
	server := setupSnapshotTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", resp["total"])
	}
	items := resp["items"].([]any)
	if len(items) != 0 {
		t.Errorf("items count = %d, want 0", len(items))
	}
}

func TestGetSnapshot(t *testing.T) {
	server := setupSnapshotTestServer()

	// Create a snapshot
	body := `{"name":"Test Snapshot","description":"Test description"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	snapshotID := createResp["id"].(string)

	// Get the snapshot
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots/"+snapshotID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["id"] != snapshotID {
		t.Errorf("id = %v, want %v", resp["id"], snapshotID)
	}
	if resp["name"] != "Test Snapshot" {
		t.Errorf("name = %v, want Test Snapshot", resp["name"])
	}
	if resp["description"] != "Test description" {
		t.Errorf("description = %v, want Test description", resp["description"])
	}
}

func TestGetSnapshot_NotFound(t *testing.T) {
	server := setupSnapshotTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestGetSnapshot_InvalidUUID(t *testing.T) {
	server := setupSnapshotTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots/not-a-uuid", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestDeleteSnapshot(t *testing.T) {
	server := setupSnapshotTestServer()

	// Create a snapshot
	body := `{"name":"To Delete"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	snapshotID := createResp["id"].(string)

	// Delete the snapshot
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/snapshots/"+snapshotID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Verify it's gone
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots/"+snapshotID, http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("Status after delete = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteSnapshot_NotFound(t *testing.T) {
	server := setupSnapshotTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/snapshots/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestCompareSnapshots(t *testing.T) {
	server := setupSnapshotTestServer()

	// Create first snapshot
	body1 := `{"name":"First Snapshot"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec1, req1)

	var createResp1 map[string]any
	json.Unmarshal(rec1.Body.Bytes(), &createResp1)
	snapshot1ID := createResp1["id"].(string)

	// Create second snapshot
	body2 := `{"name":"Second Snapshot"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec2, req2)

	var createResp2 map[string]any
	json.Unmarshal(rec2.Body.Bytes(), &createResp2)
	snapshot2ID := createResp2["id"].(string)

	// Compare snapshots
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots/"+snapshot1ID+"/compare/"+snapshot2ID, http.NoBody)
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
	if resp["snapshot1"] == nil {
		t.Error("Expected snapshot1 field")
	}
	if resp["snapshot2"] == nil {
		t.Error("Expected snapshot2 field")
	}
	if resp["changes"] == nil {
		t.Error("Expected changes field")
	}
	if resp["total_count"] == nil {
		t.Error("Expected total_count field")
	}
	if resp["has_more"] == nil {
		t.Error("Expected has_more field")
	}
	if resp["older_first"] == nil {
		t.Error("Expected older_first field")
	}
}

func TestCompareSnapshots_NotFound(t *testing.T) {
	server := setupSnapshotTestServer()

	// Create one snapshot
	body := `{"name":"Existing Snapshot"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec1, req1)

	var createResp map[string]any
	json.Unmarshal(rec1.Body.Bytes(), &createResp)
	existingID := createResp["id"].(string)

	// Compare with non-existent snapshot
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots/"+existingID+"/compare/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestCompareSnapshots_InvalidUUID(t *testing.T) {
	server := setupSnapshotTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots/not-a-uuid/compare/also-not-uuid", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateSnapshot_InvalidJSON(t *testing.T) {
	server := setupSnapshotTestServer()
	body := `{"name": invalid json`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
