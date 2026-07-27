package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

// setupBranchTestServer builds a server with a branch registry wired up, which
// is what a real deployment does (see cmd/myfamily/main.go).
func setupBranchTestServer() *api.Server {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	branchStore := memory.NewBranchStore()
	return api.NewServer(cfg, eventStore, readStore, snapshotStore, nil,
		api.WithBranchStore(branchStore))
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
	// A base_position of 0 would mean the MaxPositionReader was never wired.
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
}

func TestCompareBranch_NotFound(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodGet, "/api/v1/branches/"+unknownUUID+"/compare", "")
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
