package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/demo"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupDemoServer() *api.Server {
	cfg := &config.Config{
		Port:     8080,
		DemoMode: true,
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	branchStore := memory.NewBranchStore()

	server := api.NewServer(cfg, eventStore, readStore, snapshotStore, nil,
		api.WithBranchStore(branchStore),
		api.WithDemoReset(eventStore, readStore, snapshotStore, branchStore),
	)

	// Seed initial demo data (like main.go does)
	cmdHandler := server.CommandHandler()
	if err := demo.SeedDemoData(context.Background(), cmdHandler, readStore); err != nil {
		panic("failed to seed demo data: " + err.Error())
	}

	return server
}

func TestGetAppConfig_DemoMode(t *testing.T) {
	server := setupDemoServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["demo_mode"] != true {
		t.Errorf("demo_mode = %v, want true", resp["demo_mode"])
	}
}

func TestGetAppConfig_NormalMode(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["demo_mode"] != false {
		t.Errorf("demo_mode = %v, want false", resp["demo_mode"])
	}
}

func TestResetDemo(t *testing.T) {
	server := setupDemoServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/reset", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["status"] != "reset" {
		t.Errorf("status = %s, want reset", resp["status"])
	}

	// Verify data was re-seeded: persons list should still have data
	personsReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons", http.NoBody)
	personsRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personsRec, personsReq)

	if personsRec.Code != http.StatusOK {
		t.Errorf("Persons Status = %d, want %d", personsRec.Code, http.StatusOK)
	}

	// The reset clears the branch registry, so re-seeding leaves exactly the one
	// demo branch rather than adding a second.
	branchesReq := httptest.NewRequest(http.MethodGet, "/api/v1/branches", http.NoBody)
	branchesRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(branchesRec, branchesReq)

	if branchesRec.Code != http.StatusOK {
		t.Fatalf("Branches Status = %d, want %d", branchesRec.Code, http.StatusOK)
	}
	var branches struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(branchesRec.Body.Bytes(), &branches); err != nil {
		t.Fatalf("Failed to parse branches response: %v", err)
	}
	if branches.Total != 1 {
		t.Errorf("branches total = %d, want 1", branches.Total)
	}
}

func TestResetDemo_NotAvailableInNormalMode(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/reset", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	// Route is not registered in normal mode, so it should 404 or 405
	if rec.Code == http.StatusOK {
		t.Errorf("Expected non-200 status for demo reset in normal mode, got %d", rec.Code)
	}
}
