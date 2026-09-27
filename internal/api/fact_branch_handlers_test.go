package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// ============================================================================
// Branch scope on the person/family facts (sub-issue B of #676, issue #757)
// ============================================================================

// setupFactBranchTestServer is setupBranchTestServer plus a handle on the read
// model, for seeding life events (which have no create endpoint of their own).
func setupFactBranchTestServer() (*api.Server, *memory.ReadModelStore) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	server := api.NewServer(cfg, eventStore, readStore, snapshotStore, nil,
		api.WithBranchStore(memory.NewBranchStore()))
	return server, readStore
}

// TestBrowseCemeteries_BranchScope is the end-to-end check that the cemetery
// index follows ?branch=: a person deleted on the branch takes their burial with
// them there, while the mainline index still counts them.
func TestBrowseCemeteries_BranchScope(t *testing.T) {
	server, readStore := setupFactBranchTestServer()
	kept := createPerson(t, server, "Ada", "Lovelace")
	dropped := createPerson(t, server, "Grace", "Hopper")
	for _, id := range []string{kept, dropped} {
		if err := readStore.SaveEvent(context.Background(), domain.MainBranchID, &repository.EventReadModel{
			ID: uuid.New(), OwnerType: "person", OwnerID: uuid.MustParse(id),
			FactType: domain.FactPersonBurial, Place: "Oak Grove", Version: 1, CreatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("seed burial: %v", err)
		}
	}

	branchID := createBranch(t, server, "Hopper theory")
	if rec := do(t, server, http.MethodDelete, "/api/v1/persons/"+dropped+"?branch="+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete person on branch: status = %d, want 204. Body: %s", rec.Code, rec.Body.String())
	}

	oakGrove := func(path string) int {
		t.Helper()
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
		}
		items, _ := decodeJSON(t, rec)["items"].([]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if item["place"] == "Oak Grove" {
				count, _ := item["count"].(float64)
				return int(count)
			}
		}
		return 0
	}
	if got := oakGrove("/api/v1/browse/cemeteries"); got != 2 {
		t.Errorf("Mainline cemetery count = %d, want 2", got)
	}
	if got := oakGrove("/api/v1/browse/cemeteries?branch=" + branchID); got != 1 {
		t.Errorf("Branch cemetery count = %d, want 1 (the deleted person's burial is tombstoned)", got)
	}

	// The per-cemetery list agrees with the index on both scopes.
	listTotal := func(path string) int {
		t.Helper()
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
		}
		total, _ := decodeJSON(t, rec)["total"].(float64)
		return int(total)
	}
	if got := listTotal("/api/v1/browse/cemeteries/Oak%20Grove/persons"); got != 2 {
		t.Errorf("Mainline cemetery person list total = %d, want 2", got)
	}
	if got := listTotal("/api/v1/browse/cemeteries/Oak%20Grove/persons?branch=" + branchID); got != 1 {
		t.Errorf("Branch cemetery person list total = %d, want 1", got)
	}
}

// TestAssociations_BranchScope walks the association CRUD surface on a branch:
// every write lands on the branch only and every read follows the scope.
func TestAssociations_BranchScope(t *testing.T) {
	server := setupBranchTestServer()
	subject := createPerson(t, server, "Ada", "Lovelace")
	associate := createPerson(t, server, "Charles", "Babbage")
	branchID := createBranch(t, server, "Patronage")
	onBranch := "?branch=" + branchID

	rec := do(t, server, http.MethodPost, "/api/v1/associations"+onBranch,
		fmt.Sprintf(`{"person_id":%q,"associate_id":%q,"role":"witness"}`, subject, associate))
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create association on branch: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	created := decodeJSON(t, rec)
	assocID, _ := created["id"].(string)
	if created["role"] != "witness" {
		t.Errorf("Created association role = %v, want witness", created["role"])
	}

	status := func(method, path, body string) int {
		t.Helper()
		return do(t, server, method, path, body).Code
	}
	total := func(path string) int {
		t.Helper()
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
		}
		n, _ := decodeJSON(t, rec)["total"].(float64)
		return int(n)
	}

	// Branch-only: invisible on main.
	if got := status(http.MethodGet, "/api/v1/associations/"+assocID, ""); got != http.StatusNotFound {
		t.Errorf("Mainline GET of a branch association: status = %d, want 404", got)
	}
	if got := status(http.MethodGet, "/api/v1/associations/"+assocID+onBranch, ""); got != http.StatusOK {
		t.Errorf("Branch GET of a branch association: status = %d, want 200", got)
	}
	if got := total("/api/v1/associations"); got != 0 {
		t.Errorf("Mainline association list total = %d, want 0", got)
	}
	if got := total("/api/v1/associations" + onBranch); got != 1 {
		t.Errorf("Branch association list total = %d, want 1", got)
	}
	perPerson := func(path string) int {
		t.Helper()
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
		}
		var items []any
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
			t.Fatalf("GET %s: decode array: %v", path, err)
		}
		return len(items)
	}
	if got := perPerson("/api/v1/persons/" + associate + "/associations"); got != 0 {
		t.Errorf("Mainline per-person associations = %d, want 0", got)
	}
	if got := perPerson("/api/v1/persons/" + associate + "/associations" + onBranch); got != 1 {
		t.Errorf("Branch per-person associations = %d, want 1", got)
	}

	// Update and delete on the branch.
	version, _ := created["version"].(float64)
	rec = do(t, server, http.MethodPut, "/api/v1/associations/"+assocID+onBranch,
		fmt.Sprintf(`{"role":"godparent","version":%d}`, int64(version)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Update association on branch: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	updated := decodeJSON(t, rec)
	if updated["role"] != "godparent" {
		t.Errorf("Updated association role = %v, want godparent", updated["role"])
	}
	newVersion, _ := updated["version"].(float64)
	if got := status(http.MethodDelete, fmt.Sprintf("/api/v1/associations/%s?branch=%s&version=%d", assocID, branchID, int64(newVersion)), ""); got != http.StatusNoContent {
		t.Fatalf("Delete association on branch: status = %d, want 204", got)
	}
	if got := status(http.MethodGet, "/api/v1/associations/"+assocID+onBranch, ""); got != http.StatusNotFound {
		t.Errorf("Branch GET after delete: status = %d, want 404", got)
	}

	// A person the branch does not have 404s on the branch-scoped per-person list.
	if got := status(http.MethodGet, "/api/v1/persons/"+unknownUUID+"/associations"+onBranch, ""); got != http.StatusNotFound {
		t.Errorf("Branch per-person list for an unknown person: status = %d, want 404", got)
	}
}

// TestFactBranchScope_UnknownBranch pins the shared 404 for a branch id that was
// never created, on every operation #757 scoped.
func TestFactBranchScope_UnknownBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	scope := "?branch=" + unknownUUID

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"browseCemeteries", http.MethodGet, "/api/v1/browse/cemeteries" + scope, ""},
		{"listAssociations", http.MethodGet, "/api/v1/associations" + scope, ""},
		{"createAssociation", http.MethodPost, "/api/v1/associations" + scope,
			fmt.Sprintf(`{"person_id":%q,"associate_id":%q,"role":"witness"}`, personID, personID)},
		{"getAssociation", http.MethodGet, "/api/v1/associations/" + unknownUUID + scope, ""},
		{"updateAssociation", http.MethodPut, "/api/v1/associations/" + unknownUUID + scope, `{"role":"x","version":1}`},
		{"deleteAssociation", http.MethodDelete, "/api/v1/associations/" + unknownUUID + scope + "&version=1", ""},
		{"listAssociationsForPerson", http.MethodGet, "/api/v1/persons/" + personID + "/associations" + scope, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, server, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("Status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestFactBranchScope_ArchivedBranch pins the terminal-branch contract on the
// #757 operations: reads 404 (the overlay is purged), writes 409 (read-only).
func TestFactBranchScope_ArchivedBranch(t *testing.T) {
	server := setupBranchTestServer()
	subject := createPerson(t, server, "Ada", "Lovelace")
	associate := createPerson(t, server, "Charles", "Babbage")
	branchID := createBranch(t, server, "Abandoned")
	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete branch: status = %d, want 204", rec.Code)
	}
	scope := "?branch=" + branchID

	for _, path := range []string{
		"/api/v1/browse/cemeteries" + scope,
		"/api/v1/associations" + scope,
		"/api/v1/persons/" + subject + "/associations" + scope,
	} {
		if rec := do(t, server, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404. Body: %s", path, rec.Code, rec.Body.String())
		}
	}
	rec := do(t, server, http.MethodPost, "/api/v1/associations"+scope,
		fmt.Sprintf(`{"person_id":%q,"associate_id":%q,"role":"witness"}`, subject, associate))
	if rec.Code != http.StatusConflict {
		t.Errorf("Archived create association: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
}
