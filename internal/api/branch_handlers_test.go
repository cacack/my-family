package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// setupBranchTestServer builds a server with a branch registry wired up, which
// is what a real deployment does (see cmd/myfamily/main.go).
func setupBranchTestServer() *api.Server {
	server, _ := setupBranchTestServerWithEventStore()
	return server
}

// setupBranchTestServerWithEventStore is setupBranchTestServer plus a handle on
// the event store, for the two merge refusals that can only be provoked by
// writing to the log behind the API's back (see their tests).
func setupBranchTestServerWithEventStore() (*api.Server, *memory.EventStore) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	branchStore := memory.NewBranchStore()
	server := api.NewServer(cfg, eventStore, readStore, snapshotStore, nil,
		api.WithBranchStore(branchStore))
	return server, eventStore
}

// do issues a request against the server and returns the recorder.
func do(t *testing.T, server *api.Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, http.NoBody)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	return rec
}

// decodeJSON parses a recorder body into a map, failing the test on error.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("Failed to parse response %q: %v", rec.Body.String(), err)
	}
	return out
}

// createBranch creates a branch over HTTP and returns its id.
func createBranch(t *testing.T, server *api.Server, name string) string {
	t.Helper()
	rec := do(t, server, http.MethodPost, "/api/v1/branches",
		fmt.Sprintf(`{"name":%q}`, name))
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateBranch status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	id, _ := decodeJSON(t, rec)["id"].(string)
	if id == "" {
		t.Fatal("CreateBranch returned no id")
	}
	return id
}

// ============================================================================
// /branches resource
// ============================================================================

func TestCreateBranch(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodPost, "/api/v1/branches",
		`{"name":"Maternal Smith line","description":"Testing the Mary Smith theory"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	resp := decodeJSON(t, rec)
	if resp["name"] != "Maternal Smith line" {
		t.Errorf("name = %v, want Maternal Smith line", resp["name"])
	}
	if resp["description"] != "Testing the Mary Smith theory" {
		t.Errorf("description = %v", resp["description"])
	}
	if resp["status"] != "active" {
		t.Errorf("status = %v, want active", resp["status"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
	if resp["base_position"] == nil {
		t.Error("Expected base_position field")
	}
	if resp["created_at"] == nil {
		t.Error("Expected created_at field")
	}
}

func TestCreateBranch_BasePositionTracksHead(t *testing.T) {
	server := setupBranchTestServer()
	createPerson(t, server, "Ada", "Lovelace")

	rec := do(t, server, http.MethodPost, "/api/v1/branches", `{"name":"After Ada"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}

	// Creating a person writes PersonCreated + NameAdded, so the head is past 0.
	// A base_position of 0 would mean the position source was never wired.
	basePosition, ok := decodeJSON(t, rec)["base_position"].(float64)
	if !ok {
		t.Fatal("base_position missing or not a number")
	}
	if basePosition <= 0 {
		t.Errorf("base_position = %v, want > 0 (branch must fork from the event log head)", basePosition)
	}
}

func TestCreateBranch_ValidationError(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":""}`},
		{"name too long", fmt.Sprintf(`{"name":%q}`, strings.Repeat("x", 101))},
		{"description too long", fmt.Sprintf(`{"name":"ok","description":%q}`, strings.Repeat("x", 501))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupBranchTestServer()
			rec := do(t, server, http.MethodPost, "/api/v1/branches", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestListBranches(t *testing.T) {
	server := setupBranchTestServer()

	rec := do(t, server, http.MethodGet, "/api/v1/branches", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if total := decodeJSON(t, rec)["total"]; total != float64(0) {
		t.Errorf("total = %v, want 0", total)
	}

	createBranch(t, server, "one")
	createBranch(t, server, "two")

	rec = do(t, server, http.MethodGet, "/api/v1/branches", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON(t, rec)
	if resp["total"] != float64(2) {
		t.Errorf("total = %v, want 2", resp["total"])
	}
	items, _ := resp["items"].([]any)
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

func TestGetBranch(t *testing.T) {
	server := setupBranchTestServer()
	id := createBranch(t, server, "Maternal line")

	rec := do(t, server, http.MethodGet, "/api/v1/branches/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON(t, rec)
	if resp["id"] != id {
		t.Errorf("id = %v, want %s", resp["id"], id)
	}
	if resp["name"] != "Maternal line" {
		t.Errorf("name = %v, want Maternal line", resp["name"])
	}
}

func TestGetBranch_NotFound(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodGet, "/api/v1/branches/"+unknownUUID, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteBranch_Archives(t *testing.T) {
	server := setupBranchTestServer()
	id := createBranch(t, server, "Dead end")

	rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+id, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("Status = %d, want 204. Body: %s", rec.Code, rec.Body.String())
	}

	// The record is retained with status archived - delete is not erasure (ES-002).
	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GetBranch after delete: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if status := decodeJSON(t, rec)["status"]; status != "archived" {
		t.Errorf("status = %v, want archived", status)
	}
}

func TestDeleteBranch_NotFound(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+unknownUUID, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteBranch_AlreadyArchived(t *testing.T) {
	server := setupBranchTestServer()
	id := createBranch(t, server, "Dead end")

	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+id, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("First delete: status = %d, want 204", rec.Code)
	}

	rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+id, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("Second delete: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
}

// ============================================================================
// Branch isolation over HTTP - the acceptance criterion of #670
// ============================================================================

func TestBranchIsolation_PersonEdit(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	branchID := createBranch(t, server, "Byron theory")

	// Read the mainline version so we can thread it into the update.
	rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID, "")
	version, ok := decodeJSON(t, rec)["version"].(float64)
	if !ok {
		t.Fatalf("version missing from person payload: %s", rec.Body.String())
	}

	// Edit on the branch only.
	rec = do(t, server, http.MethodPut,
		fmt.Sprintf("/api/v1/persons/%s?branch=%s", personID, branchID),
		fmt.Sprintf(`{"surname":"Byron","version":%d}`, int64(version)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Branch update: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if surname := decodeJSON(t, rec)["surname"]; surname != "Byron" {
		t.Errorf("Branch update returned surname = %v, want Byron", surname)
	}

	// The branch view shows the edit.
	rec = do(t, server, http.MethodGet,
		fmt.Sprintf("/api/v1/persons/%s?branch=%s", personID, branchID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Branch read: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if surname := decodeJSON(t, rec)["surname"]; surname != "Byron" {
		t.Errorf("Branch read surname = %v, want Byron", surname)
	}

	// Main is untouched.
	rec = do(t, server, http.MethodGet, "/api/v1/persons/"+personID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Main read: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if surname := decodeJSON(t, rec)["surname"]; surname != "Lovelace" {
		t.Errorf("Main read surname = %v, want Lovelace - the branch edit leaked", surname)
	}
}

func TestBranchIsolation_PersonCreate(t *testing.T) {
	server := setupBranchTestServer()
	branchID := createBranch(t, server, "Speculative")

	rec := do(t, server, http.MethodPost, "/api/v1/persons?branch="+branchID,
		`{"given_name":"Hypothetical","surname":"Ancestor"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Branch create: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	newID, _ := decodeJSON(t, rec)["id"].(string)

	// Visible on the branch...
	rec = do(t, server, http.MethodGet,
		fmt.Sprintf("/api/v1/persons/%s?branch=%s", newID, branchID), "")
	if rec.Code != http.StatusOK {
		t.Errorf("Branch read: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	// ...and absent from main.
	rec = do(t, server, http.MethodGet, "/api/v1/persons/"+newID, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("Main read: status = %d, want 404 - the branch-only person leaked to main. Body: %s",
			rec.Code, rec.Body.String())
	}

	// The mainline list does not include it either.
	rec = do(t, server, http.MethodGet, "/api/v1/persons", "")
	if total := decodeJSON(t, rec)["total"]; total != float64(0) {
		t.Errorf("Mainline list total = %v, want 0", total)
	}
}

func TestBranchIsolation_Family(t *testing.T) {
	server := setupBranchTestServer()
	p1 := createPerson(t, server, "John", "Smith")
	p2 := createPerson(t, server, "Mary", "Jones")
	branchID := createBranch(t, server, "Marriage theory")

	rec := do(t, server, http.MethodPost, "/api/v1/families?branch="+branchID,
		fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q}`, p1, p2))
	if rec.Code != http.StatusCreated {
		t.Fatalf("Branch family create: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	familyID, _ := decodeJSON(t, rec)["id"].(string)

	rec = do(t, server, http.MethodGet,
		fmt.Sprintf("/api/v1/families/%s?branch=%s", familyID, branchID), "")
	if rec.Code != http.StatusOK {
		t.Errorf("Branch family read: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, server, http.MethodGet, "/api/v1/families/"+familyID, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("Main family read: status = %d, want 404 - the branch family leaked. Body: %s",
			rec.Code, rec.Body.String())
	}
}

func TestBranchScope_UnaffectedWhenOmitted(t *testing.T) {
	// Regression guard: with no ?branch= the API behaves exactly as before.
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")

	for _, path := range []string{
		"/api/v1/persons",
		"/api/v1/persons/" + personID,
		"/api/v1/persons/" + personID + "/names",
		"/api/v1/pedigree/" + personID,
	} {
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
		}
	}
}

// TestBranchChildLinks_FullLifecycle covers AddChildToFamily /
// RemoveChildFromFamily under a branch scope. This test previously pinned four
// documented limitations (second link 409, branch-only unlink 400, and the
// createFamily / deleteFamily cases below); all of them were symptoms of the
// command layer reading the mainline read model and are fixed now that command
// reads carry the handler's branch scope.
func TestBranchChildLinks_FullLifecycle(t *testing.T) {
	server := setupBranchTestServer()
	p1 := createPerson(t, server, "John", "Smith")
	p2 := createPerson(t, server, "Mary", "Jones")
	child1 := createPerson(t, server, "Kid", "One")
	child2 := createPerson(t, server, "Kid", "Two")
	familyID := createFamily(t, server, p1, p2)
	branchID := createBranch(t, server, "children theory")

	childrenPath := fmt.Sprintf("/api/v1/families/%s/children?branch=%s", familyID, branchID)

	// Two sequential links on the same family within one branch both succeed:
	// the expected family version now comes from the branch's own row.
	for _, child := range []string{child1, child2} {
		rec := do(t, server, http.MethodPost, childrenPath, fmt.Sprintf(`{"person_id":%q}`, child))
		if rec.Code != http.StatusCreated {
			t.Fatalf("Branch link of %s: status = %d, want 201. Body: %s", child, rec.Code, rec.Body.String())
		}
	}

	// A branch-only link can be undone on the branch: the unlink command resolves
	// the child's family through the branch overlay.
	rec := do(t, server, http.MethodDelete,
		fmt.Sprintf("/api/v1/families/%s/children/%s?branch=%s", familyID, child1, branchID), "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("Branch unlink of a branch-only link: status = %d, want 204. Body: %s",
			rec.Code, rec.Body.String())
	}

	// The branch sees exactly the surviving link.
	rec = do(t, server, http.MethodGet, fmt.Sprintf("/api/v1/families/%s?branch=%s", familyID, branchID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Branch family read: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if children, _ := decodeJSON(t, rec)["children"].([]any); len(children) != 1 {
		t.Errorf("Branch family children = %d, want 1", len(children))
	}

	// Main never saw any of it.
	rec = do(t, server, http.MethodGet, "/api/v1/families/"+familyID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Main family read: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if children, _ := decodeJSON(t, rec)["children"].([]any); len(children) != 0 {
		t.Errorf("Main family children = %d, want 0 - the branch link leaked", len(children))
	}
}

// TestBranchFamily_BranchOnlyPartnerAndDelete covers the two remaining
// limitations that the mainline-read gap used to impose on the family endpoints:
// a branch-only person could not be a partner, and a family whose children were
// unlinked only on the branch could not be deleted there.
func TestBranchFamily_BranchOnlyPartnerAndDelete(t *testing.T) {
	server := setupBranchTestServer()
	branchID := createBranch(t, server, "new lineage")

	// A person created on the branch can be a partner in a branch family.
	rec := do(t, server, http.MethodPost, "/api/v1/persons?branch="+branchID,
		`{"given_name":"Branch","surname":"Only","gender":"unknown"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Branch CreatePerson: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	branchPerson, _ := decodeJSON(t, rec)["id"].(string)

	rec = do(t, server, http.MethodPost, "/api/v1/families?branch="+branchID,
		fmt.Sprintf(`{"partner1_id":%q,"relationship_type":"marriage"}`, branchPerson))
	if rec.Code != http.StatusCreated {
		t.Fatalf("Branch CreateFamily with a branch-only partner: status = %d, want 201. Body: %s",
			rec.Code, rec.Body.String())
	}

	// A family whose only child is unlinked on the branch deletes on the branch.
	p1 := createPerson(t, server, "John", "Smith")
	p2 := createPerson(t, server, "Mary", "Jones")
	child := createPerson(t, server, "Kid", "One")
	familyID := createFamily(t, server, p1, p2)
	if rec := do(t, server, http.MethodPost, "/api/v1/families/"+familyID+"/children",
		fmt.Sprintf(`{"person_id":%q}`, child)); rec.Code != http.StatusCreated {
		t.Fatalf("Main link: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}

	branch2 := createBranch(t, server, "prune the family")
	if rec := do(t, server, http.MethodDelete,
		fmt.Sprintf("/api/v1/families/%s/children/%s?branch=%s", familyID, child, branch2), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Branch unlink: status = %d, want 204. Body: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, server, http.MethodDelete,
		fmt.Sprintf("/api/v1/families/%s?branch=%s", familyID, branch2), "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("Branch DeleteFamily after a branch-only unlink: status = %d, want 204. Body: %s",
			rec.Code, rec.Body.String())
	}

	// Main keeps both the family and the child link.
	rec = do(t, server, http.MethodGet, "/api/v1/families/"+familyID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Main family read: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if children, _ := decodeJSON(t, rec)["children"].([]any); len(children) != 1 {
		t.Errorf("Main family children = %d, want 1 - the branch delete leaked", len(children))
	}
}

// ============================================================================
// Scope resolution errors
// ============================================================================

// unknownUUID is a well-formed UUID that no fixture ever creates.
const unknownUUID = "00000000-0000-4000-8000-0000000000ff"

func TestBranchScope_UnknownBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"read person", http.MethodGet, "/api/v1/persons/" + personID + "?branch=" + unknownUUID, ""},
		{"list persons", http.MethodGet, "/api/v1/persons?branch=" + unknownUUID, ""},
		{"read names", http.MethodGet, "/api/v1/persons/" + personID + "/names?branch=" + unknownUUID, ""},
		{"read pedigree", http.MethodGet, "/api/v1/pedigree/" + personID + "?branch=" + unknownUUID, ""},
		{"create person", http.MethodPost, "/api/v1/persons?branch=" + unknownUUID, `{"given_name":"X"}`},
		{"delete person", http.MethodDelete, "/api/v1/persons/" + personID + "?branch=" + unknownUUID, ""},
		{"create family", http.MethodPost, "/api/v1/families?branch=" + unknownUUID, `{}`},
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

func TestBranchScope_ArchivedBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	branchID := createBranch(t, server, "Abandoned")

	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete branch: status = %d, want 204", rec.Code)
	}

	// A read of a terminal branch is 404: archiving purged its overlay rows, so
	// there is no isolated view left to return.
	rec := do(t, server, http.MethodGet,
		fmt.Sprintf("/api/v1/persons/%s?branch=%s", personID, branchID), "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("Archived read: status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
	}

	// A write to a terminal branch is 409: it is read-only per ADR-005.
	rec = do(t, server, http.MethodPut,
		fmt.Sprintf("/api/v1/persons/%s?branch=%s", personID, branchID),
		`{"surname":"Byron","version":1}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("Archived write: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, server, http.MethodPost, "/api/v1/persons?branch="+branchID, `{"given_name":"X"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("Archived create: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
}

func TestBranchScope_MalformedUUID(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodGet, "/api/v1/persons?branch=not-a-uuid", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400. Body: %s", rec.Code, rec.Body.String())
	}
}

// ============================================================================
// Compare
// ============================================================================

func TestCompareBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	branchID := createBranch(t, server, "Byron theory")

	rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID, "")
	version, _ := decodeJSON(t, rec)["version"].(float64)

	rec = do(t, server, http.MethodPut,
		fmt.Sprintf("/api/v1/persons/%s?branch=%s", personID, branchID),
		fmt.Sprintf(`{"surname":"Byron","version":%d}`, int64(version)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Branch update: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/compare", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Compare: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON(t, rec)
	branch, _ := resp["branch"].(map[string]any)
	if branch["id"] != branchID {
		t.Errorf("branch.id = %v, want %s", branch["id"], branchID)
	}
	if resp["base_position"] == nil {
		t.Error("Expected base_position")
	}

	branchChanges, _ := resp["branch_changes"].([]any)
	if len(branchChanges) != 1 {
		t.Fatalf("len(branch_changes) = %d, want 1. Body: %s", len(branchChanges), rec.Body.String())
	}
	entry, _ := branchChanges[0].(map[string]any)
	if entry["entity_id"] != personID {
		t.Errorf("branch_changes[0].entity_id = %v, want %s", entry["entity_id"], personID)
	}
	if entry["action"] != "updated" {
		t.Errorf("branch_changes[0].action = %v, want updated", entry["action"])
	}
	if resp["branch_change_count"] != float64(1) {
		t.Errorf("branch_change_count = %v, want 1", resp["branch_change_count"])
	}

	// Main saw no work after the fork, so its side is empty and nothing overlaps.
	if mainChanges, _ := resp["main_changes"].([]any); len(mainChanges) != 0 {
		t.Errorf("len(main_changes) = %d, want 0", len(mainChanges))
	}
	overlap, ok := resp["overlapping_stream_ids"].([]any)
	if !ok {
		t.Fatalf("overlapping_stream_ids missing or null: %s", rec.Body.String())
	}
	if len(overlap) != 0 {
		t.Errorf("len(overlapping_stream_ids) = %d, want 0", len(overlap))
	}
	if resp["has_more"] != false {
		t.Errorf("has_more = %v, want false", resp["has_more"])
	}

	// A clean branch still carries the verdict, as [] rather than null.
	conflicts, ok := resp["conflicts"].([]any)
	if !ok {
		t.Fatalf("conflicts missing or null: %s", rec.Body.String())
	}
	if len(conflicts) != 0 {
		t.Errorf("len(conflicts) = %d, want 0", len(conflicts))
	}
}

func TestCompareBranch_ContestedStreamShowsOnBothSides(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	branchID := createBranch(t, server, "Byron theory")

	rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID, "")
	version, _ := decodeJSON(t, rec)["version"].(float64)

	// Same person edited on both sides.
	rec = do(t, server, http.MethodPut,
		fmt.Sprintf("/api/v1/persons/%s?branch=%s", personID, branchID),
		fmt.Sprintf(`{"surname":"Byron","version":%d}`, int64(version)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Branch update: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, server, http.MethodPut, "/api/v1/persons/"+personID,
		fmt.Sprintf(`{"surname":"King","version":%d}`, int64(version)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Main update: status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/compare", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Compare: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON(t, rec)
	if mainChanges, _ := resp["main_changes"].([]any); len(mainChanges) != 1 {
		t.Errorf("len(main_changes) = %d, want 1. Body: %s", len(mainChanges), rec.Body.String())
	}
	overlap, _ := resp["overlapping_stream_ids"].([]any)
	if len(overlap) != 1 || overlap[0] != personID {
		t.Errorf("overlapping_stream_ids = %v, want [%s]", overlap, personID)
	}

	// Overlap is the hint; conflicts is the verdict. Both sides set surname to a
	// different value, so this overlap really is a conflict.
	conflicts, _ := resp["conflicts"].([]any)
	if len(conflicts) != 1 {
		t.Fatalf("len(conflicts) = %d, want 1. Body: %s", len(conflicts), rec.Body.String())
	}
	conflict, _ := conflicts[0].(map[string]any)
	if conflict["stream_id"] != personID {
		t.Errorf("conflicts[0].stream_id = %v, want %s", conflict["stream_id"], personID)
	}
	if conflict["kind"] != "edit_edit" {
		t.Errorf("conflicts[0].kind = %v, want edit_edit", conflict["kind"])
	}
	// Lower-case, the same vocabulary ChangeEntry.entity_type uses — this response
	// carries both, so they must agree.
	if conflict["entity_type"] != "person" {
		t.Errorf("conflicts[0].entity_type = %v, want person", conflict["entity_type"])
	}
	// And because the type is lower-case, name resolution actually fires: handed
	// the capitalized stream type it fell through to a default and every conflict
	// came back unnamed, which would have left the review UI showing only UUIDs.
	// nil as well as "": conflict is a map[string]any, so an absent or null
	// field decodes to nil and `nil == ""` is false — the assertion would pass
	// on exactly the regression it exists to catch.
	if conflict["entity_name"] == nil || conflict["entity_name"] == "" {
		t.Errorf("conflicts[0].entity_name is empty; want the person's resolved name. Body: %s", rec.Body.String())
	}
	fields, _ := conflict["fields"].([]any)
	if len(fields) != 1 || fields[0] != "surname" {
		t.Errorf("conflicts[0].fields = %v, want [surname]", conflict["fields"])
	}
	if conflict["detail"] == nil || conflict["detail"] == "" {
		t.Error("Expected a human-readable detail on the conflict")
	}
}

func TestCompareBranch_NotFound(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodGet, "/api/v1/branches/"+unknownUUID+"/compare", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
	}
}

// ============================================================================
// Merge
// ============================================================================

// forkAndEditPerson creates a person on main, forks a branch, and renames the
// person on the branch. It returns the person id, the branch id and the
// person's pre-branch version — the setup nearly every merge test needs.
func forkAndEditPerson(t *testing.T, server *api.Server, branchSurname string) (personID, branchID string, version int64) {
	t.Helper()
	personID = createPerson(t, server, "Ada", "Lovelace")
	branchID = createBranch(t, server, "Byron theory")

	rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID, "")
	raw, _ := decodeJSON(t, rec)["version"].(float64)
	version = int64(raw)

	rec = do(t, server, http.MethodPut,
		fmt.Sprintf("/api/v1/persons/%s?branch=%s", personID, branchID),
		fmt.Sprintf(`{"surname":%q,"version":%d}`, branchSurname, version))
	if rec.Code != http.StatusOK {
		t.Fatalf("Branch update: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	return personID, branchID, version
}

// mainSurname reads a person from the mainline view.
func mainSurname(t *testing.T, server *api.Server, personID string) string {
	t.Helper()
	rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GetPerson: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	surname, _ := decodeJSON(t, rec)["surname"].(string)
	return surname
}

func TestMergeBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID, branchID, _ := forkAndEditPerson(t, server, "Byron")

	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge",
		`{"note":"Confirmed by the 1881 census"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Merge: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON(t, rec)
	branch, _ := resp["branch"].(map[string]any)
	if branch["status"] != "merged" {
		t.Errorf("branch.status = %v, want merged", branch["status"])
	}
	if branch["merged_at"] == nil {
		t.Error("Expected branch.merged_at on a merged branch")
	}
	if branch["merge_note"] != "Confirmed by the 1881 census" {
		t.Errorf("branch.merge_note = %v", branch["merge_note"])
	}
	if resp["merged_at_position"] == nil {
		t.Error("Expected merged_at_position")
	}
	if resp["replayed_event_count"] != float64(1) {
		t.Errorf("replayed_event_count = %v, want 1", resp["replayed_event_count"])
	}
	// Required by the schema, so [] and never null even though nothing was skipped.
	skipped, ok := resp["skipped_stream_ids"].([]any)
	if !ok {
		t.Fatalf("skipped_stream_ids missing or null: %s", rec.Body.String())
	}
	if len(skipped) != 0 {
		t.Errorf("len(skipped_stream_ids) = %d, want 0", len(skipped))
	}

	// The point of the whole exercise: the branch's research is now on main.
	if got := mainSurname(t, server, personID); got != "Byron" {
		t.Errorf("Main surname after merge = %q, want Byron", got)
	}
}

func TestMergeBranch_GetBranchExposesMergeRecord(t *testing.T) {
	server := setupBranchTestServer()
	_, branchID, _ := forkAndEditPerson(t, server, "Byron")

	if rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge",
		`{"note":"Promoted after review"}`); rec.Code != http.StatusOK {
		t.Fatalf("Merge: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	rec := do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GetBranch: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON(t, rec)
	if resp["status"] != "merged" {
		t.Errorf("status = %v, want merged", resp["status"])
	}
	if resp["merged_at"] == nil {
		t.Error("Expected merged_at")
	}
	if resp["merge_note"] != "Promoted after review" {
		t.Errorf("merge_note = %v, want Promoted after review", resp["merge_note"])
	}
}

func TestMergeBranch_EmptyBodyIsAllowed(t *testing.T) {
	server := setupBranchTestServer()
	_, branchID, _ := forkAndEditPerson(t, server, "Byron")

	// Note and resolutions are both optional, so a bare POST must work.
	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Merge: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	branch, _ := decodeJSON(t, rec)["branch"].(map[string]any)
	if branch["merge_note"] != nil {
		t.Errorf("merge_note = %v, want absent", branch["merge_note"])
	}
}

func TestMergeBranch_ResolutionToMainSkipsTheStream(t *testing.T) {
	server := setupBranchTestServer()
	personID, branchID, _ := forkAndEditPerson(t, server, "Byron")

	// Resolving a non-conflicting stream to main is the only supported way to
	// leave an entity behind (ADR-005: no cherry-pick).
	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"main"}]}`, personID))
	if rec.Code != http.StatusOK {
		t.Fatalf("Merge: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON(t, rec)
	if resp["replayed_event_count"] != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0", resp["replayed_event_count"])
	}
	skipped, _ := resp["skipped_stream_ids"].([]any)
	if len(skipped) != 1 || skipped[0] != personID {
		t.Errorf("skipped_stream_ids = %v, want [%s]", resp["skipped_stream_ids"], personID)
	}
	if got := mainSurname(t, server, personID); got != "Lovelace" {
		t.Errorf("Main surname = %q, want Lovelace (the branch change was dropped)", got)
	}
}

func TestMergeBranch_UnresolvedConflicts(t *testing.T) {
	server := setupBranchTestServer()
	personID, branchID, version := forkAndEditPerson(t, server, "Byron")

	// Main renames the same person differently, which is an edit_edit conflict.
	rec := do(t, server, http.MethodPut, "/api/v1/persons/"+personID,
		fmt.Sprintf(`{"surname":"King","version":%d}`, version))
	if rec.Code != http.StatusOK {
		t.Fatalf("Main update: status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON(t, rec)
	if resp["code"] != "merge_conflicts" {
		t.Errorf("code = %v, want merge_conflicts", resp["code"])
	}
	conflicts, ok := resp["conflicts"].([]any)
	if !ok || len(conflicts) != 1 {
		t.Fatalf("conflicts = %v, want one entry. Body: %s", resp["conflicts"], rec.Body.String())
	}
	conflict, _ := conflicts[0].(map[string]any)
	if conflict["stream_id"] != personID {
		t.Errorf("conflicts[0].stream_id = %v, want %s", conflict["stream_id"], personID)
	}
	if conflict["kind"] != "edit_edit" {
		t.Errorf("conflicts[0].kind = %v, want edit_edit", conflict["kind"])
	}
	// An edit_edit accepts either side, and the array is required by the schema.
	supported, ok := conflict["supported_resolutions"].([]any)
	if !ok || len(supported) != 2 {
		t.Errorf("conflicts[0].supported_resolutions = %v, want [branch main]", conflict["supported_resolutions"])
	}

	// Refused means nothing was written: main is untouched and the branch lives.
	if got := mainSurname(t, server, personID); got != "King" {
		t.Errorf("Main surname = %q, want King (merge must not have run)", got)
	}
	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, "")
	if status := decodeJSON(t, rec)["status"]; status != "active" {
		t.Errorf("branch status = %v, want active", status)
	}
}

func TestMergeBranch_ResolvedConflictMerges(t *testing.T) {
	server := setupBranchTestServer()
	personID, branchID, version := forkAndEditPerson(t, server, "Byron")

	rec := do(t, server, http.MethodPut, "/api/v1/persons/"+personID,
		fmt.Sprintf(`{"surname":"King","version":%d}`, version))
	if rec.Code != http.StatusOK {
		t.Fatalf("Main update: status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge",
		fmt.Sprintf(`{"note":"Branch wins","resolutions":[{"stream_id":%q,"resolution":"branch"}]}`, personID))
	if rec.Code != http.StatusOK {
		t.Fatalf("Merge: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if got := mainSurname(t, server, personID); got != "Byron" {
		t.Errorf("Main surname = %q, want Byron (branch side won)", got)
	}
}

func TestMergeBranch_BranchNotActive(t *testing.T) {
	t.Run("archived", func(t *testing.T) {
		server := setupBranchTestServer()
		_, branchID, _ := forkAndEditPerson(t, server, "Byron")
		if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("Delete: status = %d, want 204", rec.Code)
		}

		rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("Merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
		}
		if code := decodeJSON(t, rec)["code"]; code != "branch_not_active" {
			t.Errorf("code = %v, want branch_not_active", code)
		}
	})

	t.Run("already merged", func(t *testing.T) {
		server := setupBranchTestServer()
		_, branchID, _ := forkAndEditPerson(t, server, "Byron")
		if rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`); rec.Code != http.StatusOK {
			t.Fatalf("First merge: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
		}

		rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("Second merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
		}
		if code := decodeJSON(t, rec)["code"]; code != "branch_not_active" {
			t.Errorf("code = %v, want branch_not_active", code)
		}
	})
}

// lostClaimEventStore makes the active->merged compare-and-set fail the way a
// concurrent merge makes it fail. The CAS reads the branch stream's version and
// then appends at it, so losing the race cannot be staged by seeding events
// beforehand - the loser would simply read the newer version. Failing the
// BranchMerged append instead reproduces the one thing that matters here: the
// command sees repository.ErrConcurrencyConflict on the claim. The genuine race
// is covered at the command layer by TestMergeBranch_ConcurrentClaimLoses.
type lostClaimEventStore struct {
	repository.EventStore
}

func (s *lostClaimEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string,
	events []domain.Event, expectedVersion int64, scope repository.AppendScope,
) error {
	for _, event := range events {
		if event.EventType() == "BranchMerged" {
			return repository.ErrConcurrencyConflict
		}
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

func TestMergeBranch_AlreadyClaimed(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	base := memory.NewEventStore()
	server := api.NewServer(cfg, &lostClaimEventStore{base}, memory.NewReadModelStore(),
		memory.NewSnapshotStore(base), nil, api.WithBranchStore(memory.NewBranchStore()))

	_, branchID, _ := forkAndEditPerson(t, server, "Byron")

	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeJSON(t, rec)["code"]; code != "merge_already_claimed" {
		t.Errorf("code = %v, want merge_already_claimed", code)
	}

	// The loser writes nothing to main.
	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, "")
	if status := decodeJSON(t, rec)["status"]; status != "active" {
		t.Errorf("branch status = %v, want active", status)
	}
}

func TestMergeBranch_TooLarge(t *testing.T) {
	server, eventStore := setupBranchTestServerWithEventStore()
	_, branchID, _ := forkAndEditPerson(t, server, "Byron")

	rec := do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, "")
	basePosition, _ := decodeJSON(t, rec)["base_position"].(float64)

	// Push the branch past the 1000-event comparison cap so its conflict scan
	// comes back truncated. Written straight to the log on a stream of its own:
	// 1000 HTTP round trips would buy nothing and cost seconds.
	bulkID := uuid.New()
	edits := make([]domain.Event, 1000)
	for i := range edits {
		edits[i] = domain.NewPersonUpdated(bulkID, map[string]any{"note": i})
	}
	err := eventStore.Append(context.Background(), bulkID, "person", edits, -1,
		repository.AppendScope{
			BranchID:     domain.BranchID(uuid.MustParse(branchID)),
			BasePosition: int64(basePosition),
		})
	if err != nil {
		t.Fatalf("Seeding an oversized branch: %v", err)
	}

	rec = do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeJSON(t, rec)["code"]; code != "branch_too_large" {
		t.Errorf("code = %v, want branch_too_large", code)
	}
}

func TestMergeBranch_BadRequest(t *testing.T) {
	tests := []struct {
		name string
		// body is a template: %s is replaced with the branch's person id.
		body string
	}{
		{"malformed json", `{"note":`},
		{"note too long", fmt.Sprintf(`{"note":%q}`, strings.Repeat("x", 1001))},
		{"unknown resolution value", `{"resolutions":[{"stream_id":"%s","resolution":"sideways"}]}`},
		{"resolution for an untouched stream",
			`{"resolutions":[{"stream_id":"` + unknownUUID + `","resolution":"branch"}]}`},
		{"duplicate stream id",
			`{"resolutions":[{"stream_id":"%s","resolution":"branch"},{"stream_id":"%s","resolution":"main"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupBranchTestServer()
			personID, branchID, _ := forkAndEditPerson(t, server, "Byron")

			body := tt.body
			if strings.Contains(body, "%s") {
				body = strings.ReplaceAll(body, "%s", personID)
			}

			rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want 400. Body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestMergeBranch_NotFound(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+unknownUUID+"/merge", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
	}
}

// ============================================================================
// Degraded mode: no branch registry configured
// ============================================================================

func TestBranches_NoBranchStore(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"list", http.MethodGet, "/api/v1/branches", "", http.StatusServiceUnavailable},
		{"create", http.MethodPost, "/api/v1/branches", `{"name":"x"}`, http.StatusServiceUnavailable},
		{"get", http.MethodGet, "/api/v1/branches/" + unknownUUID, "", http.StatusServiceUnavailable},
		{"delete", http.MethodDelete, "/api/v1/branches/" + unknownUUID, "", http.StatusServiceUnavailable},
		{"compare", http.MethodGet, "/api/v1/branches/" + unknownUUID + "/compare", "", http.StatusServiceUnavailable},
		{"merge", http.MethodPost, "/api/v1/branches/" + unknownUUID + "/merge", `{}`, http.StatusServiceUnavailable},
		// A branch scope cannot resolve when no branch can exist.
		{"scoped read", http.MethodGet, "/api/v1/persons?branch=" + unknownUUID, "", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupTestServer() // no WithBranchStore
			rec := do(t, server, tt.method, tt.path, tt.body)
			if rec.Code != tt.want {
				t.Errorf("Status = %d, want %d. Body: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

// TestMergeBranch_MainDeletedRejectsBranchResolution is the HTTP face of the
// merge's one genuinely unhonorable resolution.
//
// Main deletes the person, the branch keeps editing it, and the caller asks for
// "branch" — keep my version. The replay cannot honor that (the *Updated
// projections skip an absent row), so the API refuses with 400 rather than
// returning 200 for a merge that left main deleted.
func TestMergeBranch_MainDeletedRejectsBranchResolution(t *testing.T) {
	server := setupBranchTestServer()
	personID, branchID, version := forkAndEditPerson(t, server, "Byron")

	rec := do(t, server, http.MethodDelete,
		fmt.Sprintf("/api/v1/persons/%s?version=%d", personID, version), "")
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("Main delete: status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	// The conflict advertises only the resolution that would work.
	rec = do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Unresolved merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
	conflicts, _ := decodeJSON(t, rec)["conflicts"].([]any)
	if len(conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %v", conflicts)
	}
	conflict, _ := conflicts[0].(map[string]any)
	if conflict["kind"] != "delete_edit" {
		t.Errorf("kind = %v, want delete_edit", conflict["kind"])
	}
	supported, _ := conflict["supported_resolutions"].([]any)
	if len(supported) != 1 || supported[0] != "main" {
		t.Errorf("supported_resolutions = %v, want [main]", conflict["supported_resolutions"])
	}

	// Asking for the unsupported side is a 400, not a hollow 200.
	rec = do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"branch"}]}`, personID))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Merge: status = %d, want 400. Body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeJSON(t, rec)["code"]; code != "invalid_resolution" {
		t.Errorf("code = %v, want invalid_resolution", code)
	}

	// Nothing was written: the branch is still mergeable the supported way.
	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, "")
	if status := decodeJSON(t, rec)["status"]; status != "active" {
		t.Errorf("branch status = %v, want active", status)
	}
	rec = do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"main"}]}`, personID))
	if rec.Code != http.StatusOK {
		t.Fatalf("Merge resolved to main: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
}

// failingReplayEventStore lets the claim land and then fails the replay onto
// main, which is the one merge outcome that is NOT "nothing happened": the
// branch is already merged and main is partially updated. Distinguished from
// the claim by scope — the claim writes the branch's own scope, the replay
// writes MainScope.
type failingReplayEventStore struct {
	repository.EventStore
}

func (s *failingReplayEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string,
	events []domain.Event, expectedVersion int64, scope repository.AppendScope,
) error {
	if scope.BranchID == domain.MainBranchID && len(events) > 0 && events[0].EventType() != "PersonCreated" {
		return errors.New("simulated storage failure during replay")
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// TestMergeBranch_PartiallyApplied pins the status/code for the one response a
// client must never retry. It is deliberately NOT in the 409 family: every 409
// means nothing was written, and folding this one in would tell a client it is
// safe to retry when the branch is already terminal and main is half-updated.
func TestMergeBranch_PartiallyApplied(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	base := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	branchStore := memory.NewBranchStore()
	server := api.NewServer(cfg, &failingReplayEventStore{base}, readStore,
		memory.NewSnapshotStore(base), nil, api.WithBranchStore(branchStore))

	_, branchID, _ := forkAndEditPerson(t, server, "Byron")

	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Merge: status = %d, want 500. Body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeJSON(t, rec)["code"]; code != "merge_partially_applied" {
		t.Errorf("code = %v, want merge_partially_applied", code)
	}

	// The claim did land, so the branch really is terminal — which is why a
	// retry would be misleading rather than harmless.
	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, "")
	if status := decodeJSON(t, rec)["status"]; status != "merged" {
		t.Errorf("branch status = %v, want merged", status)
	}
}

// stalePlanEventStore lands a real mainline write in the window between
// PlanMerge capturing main's stream versions and MergeBranch re-checking them
// (#698).
//
// It counts main-scoped GetStreamVersion calls, which is what makes the timing
// precise: the plan's capture is the first, and the pre-claim staleness check is
// the second. Appending on the way into that second call means the check reads a
// version the plan never saw — a genuine race, not a simulated one.
type stalePlanEventStore struct {
	repository.EventStore
	mainVersionReads int
}

func (s *stalePlanEventStore) GetStreamVersion(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID) (int64, error) {
	if branchID.IsMain() {
		s.mainVersionReads++
		if s.mainVersionReads == 2 {
			// Straight through to the embedded store — this wrapper overrides
			// only GetStreamVersion, so there is nothing of its own to skip.
			rival := domain.NewPersonUpdated(streamID, map[string]any{"given_name": "Augusta"})
			if err := s.Append(ctx, streamID, "person", []domain.Event{rival}, -1, repository.MainScope); err != nil {
				return 0, err
			}
		}
	}
	return s.EventStore.GetStreamVersion(ctx, streamID, branchID)
}

// TestMergeBranch_PlanStale pins the sentinel→status mapping for a stale plan.
//
// It is deliberately a 409 and not the 500 its neighbour above returns: the
// refusal happens before the claim, so nothing was written and the branch is
// still active — the client SHOULD re-compare and retry. Getting the two
// backwards would tell a client with a half-merged mainline that a retry is
// safe, which is why the handler's switch checks merge_partially_applied first.
func TestMergeBranch_PlanStale(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	base := memory.NewEventStore()
	server := api.NewServer(cfg, &stalePlanEventStore{EventStore: base}, memory.NewReadModelStore(),
		memory.NewSnapshotStore(base), nil, api.WithBranchStore(memory.NewBranchStore()))

	_, branchID, _ := forkAndEditPerson(t, server, "Byron")

	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeJSON(t, rec)["code"]; code != "merge_plan_stale" {
		t.Errorf("code = %v, want merge_plan_stale", code)
	}

	// Nothing was written, so the branch is still mergeable against a fresh plan.
	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, "")
	if status := decodeJSON(t, rec)["status"]; status != "active" {
		t.Errorf("branch status = %v, want active", status)
	}
}
