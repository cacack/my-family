package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

// ============================================================================
// Branch scope on the evidence (sub-issue C of #676, issue #758)
// ============================================================================

// TestSourcesAndCitations_BranchScope walks the source and citation surface on a
// branch: writes land on the branch only, reads (including search) follow the
// scope, and a citation created on the branch carries the branch's source title.
func TestSourcesAndCitations_BranchScope(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")

	rec := do(t, server, http.MethodPost, "/api/v1/sources", `{"title":"Census 1880","source_type":"census"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create main source: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	source := decodeJSON(t, rec)
	sourceID, _ := source["id"].(string)
	sourceVersion, _ := source["version"].(float64)

	branchID := createBranch(t, server, "Census reading")
	onBranch := "?branch=" + branchID

	status := func(method, path, body string) int {
		t.Helper()
		return do(t, server, method, path, body).Code
	}
	getJSON := func(path string) map[string]any {
		t.Helper()
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
		}
		return decodeJSON(t, rec)
	}
	total := func(path string) int {
		t.Helper()
		n, _ := getJSON(path)["total"].(float64)
		return int(n)
	}

	// Retitle on the branch only.
	rec = do(t, server, http.MethodPut, "/api/v1/sources/"+sourceID+onBranch,
		fmt.Sprintf(`{"title":"Census 1880 (Revised)","version":%d}`, int64(sourceVersion)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Update source on branch: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if got := getJSON("/api/v1/sources/" + sourceID)["title"]; got != "Census 1880" {
		t.Errorf("Mainline source title = %v, want Census 1880", got)
	}
	if got := getJSON("/api/v1/sources/" + sourceID + onBranch)["title"]; got != "Census 1880 (Revised)" {
		t.Errorf("Branch source title = %v, want the revised title", got)
	}
	if got := total("/api/v1/sources/search?q=revised"); got != 0 {
		t.Errorf("Mainline search for the branch title = %d hits, want 0", got)
	}
	if got := total("/api/v1/sources/search?q=revised&branch=" + branchID); got != 1 {
		t.Errorf("Branch search for the branch title = %d hits, want 1", got)
	}

	// Cite it on the branch: the citation denormalizes the branch title.
	rec = do(t, server, http.MethodPost, "/api/v1/citations"+onBranch,
		fmt.Sprintf(`{"source_id":%q,"fact_type":"person_birth","fact_owner_id":%q,"page":"12"}`, sourceID, personID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create citation on branch: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	citation := decodeJSON(t, rec)
	citationID, _ := citation["id"].(string)
	if citation["source_title"] != "Census 1880 (Revised)" {
		t.Errorf("Branch citation source_title = %v, want the branch title", citation["source_title"])
	}
	if got := status(http.MethodGet, "/api/v1/citations/"+citationID, ""); got != http.StatusNotFound {
		t.Errorf("Mainline GET of a branch citation: status = %d, want 404", got)
	}
	if got := status(http.MethodGet, "/api/v1/citations/"+citationID+onBranch, ""); got != http.StatusOK {
		t.Errorf("Branch GET of a branch citation: status = %d, want 200", got)
	}
	if got := status(http.MethodGet, "/api/v1/citations/"+citationID+"/format"+onBranch, ""); got != http.StatusOK {
		t.Errorf("Branch format of a branch citation: status = %d, want 200", got)
	}
	if got := total("/api/v1/sources/" + sourceID + "/citations"); got != 0 {
		t.Errorf("Mainline citations for source = %d, want 0", got)
	}
	if got := total("/api/v1/sources/" + sourceID + "/citations" + onBranch); got != 1 {
		t.Errorf("Branch citations for source = %d, want 1", got)
	}
	if got := total("/api/v1/persons/" + personID + "/citations"); got != 0 {
		t.Errorf("Mainline citations for person = %d, want 0", got)
	}
	if got := total("/api/v1/persons/" + personID + "/citations" + onBranch); got != 1 {
		t.Errorf("Branch citations for person = %d, want 1", got)
	}

	// Update, then delete the citation on the branch.
	citationVersion, _ := citation["version"].(float64)
	rec = do(t, server, http.MethodPut, "/api/v1/citations/"+citationID+onBranch,
		fmt.Sprintf(`{"page":"13","version":%d}`, int64(citationVersion)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Update citation on branch: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	updated := decodeJSON(t, rec)
	if updated["page"] != "13" {
		t.Errorf("Updated citation page = %v, want 13", updated["page"])
	}
	newVersion, _ := updated["version"].(float64)
	if got := status(http.MethodDelete, fmt.Sprintf("/api/v1/citations/%s?branch=%s&version=%d", citationID, branchID, int64(newVersion)), ""); got != http.StatusNoContent {
		t.Fatalf("Delete citation on branch: status = %d, want 204", got)
	}
	if got := status(http.MethodGet, "/api/v1/citations/"+citationID+onBranch, ""); got != http.StatusNotFound {
		t.Errorf("Branch GET of a deleted citation: status = %d, want 404", got)
	}

	// A branch-only source, and deleting main's source on the branch only.
	rec = do(t, server, http.MethodPost, "/api/v1/sources"+onBranch, `{"title":"Family Diary","source_type":"book"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create source on branch: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	if got := total("/api/v1/sources"); got != 1 {
		t.Errorf("Mainline source list total = %d, want 1", got)
	}
	if got := total("/api/v1/sources" + onBranch); got != 2 {
		t.Errorf("Branch source list total = %d, want 2", got)
	}
	branchVersion, _ := getJSON("/api/v1/sources/" + sourceID + onBranch)["version"].(float64)
	if got := status(http.MethodDelete, fmt.Sprintf("/api/v1/sources/%s?branch=%s&version=%d", sourceID, branchID, int64(branchVersion)), ""); got != http.StatusNoContent {
		t.Fatalf("Delete source on branch: status = %d, want 204", got)
	}
	if got := status(http.MethodGet, "/api/v1/sources/"+sourceID+onBranch, ""); got != http.StatusNotFound {
		t.Errorf("Branch GET of a deleted source: status = %d, want 404", got)
	}
	if got := status(http.MethodGet, "/api/v1/sources/"+sourceID, ""); got != http.StatusOK {
		t.Errorf("Mainline GET after branch delete: status = %d, want 200", got)
	}
}

// TestNotes_BranchScope walks the note CRUD surface on a branch.
func TestNotes_BranchScope(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodPost, "/api/v1/notes", `{"text":"main text"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create main note: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	note := decodeJSON(t, rec)
	noteID, _ := note["id"].(string)
	version, _ := note["version"].(float64)

	branchID := createBranch(t, server, "Notes")
	onBranch := "?branch=" + branchID
	text := func(path string) any {
		t.Helper()
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
		}
		return decodeJSON(t, rec)["text"]
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

	rec = do(t, server, http.MethodPut, "/api/v1/notes/"+noteID+onBranch, fmt.Sprintf(`{"text":"branch text","version":%d}`, int64(version)))
	if rec.Code != http.StatusOK {
		t.Fatalf("Update note on branch: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if got := text("/api/v1/notes/" + noteID); got != "main text" {
		t.Errorf("Mainline note text = %v, want main text", got)
	}
	if got := text("/api/v1/notes/" + noteID + onBranch); got != "branch text" {
		t.Errorf("Branch note text = %v, want branch text", got)
	}

	rec = do(t, server, http.MethodPost, "/api/v1/notes"+onBranch, `{"text":"branch only"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create note on branch: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	if got := total("/api/v1/notes"); got != 1 {
		t.Errorf("Mainline note total = %d, want 1", got)
	}
	if got := total("/api/v1/notes" + onBranch); got != 2 {
		t.Errorf("Branch note total = %d, want 2", got)
	}

	branchNote := do(t, server, http.MethodGet, "/api/v1/notes/"+noteID+onBranch, "")
	branchVersion, _ := decodeJSON(t, branchNote)["version"].(float64)
	if rec := do(t, server, http.MethodDelete, fmt.Sprintf("/api/v1/notes/%s?branch=%s&version=%d", noteID, branchID, int64(branchVersion)), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete note on branch: status = %d, want 204. Body: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/notes/"+noteID+onBranch, ""); rec.Code != http.StatusNotFound {
		t.Errorf("Branch GET of a deleted note: status = %d, want 404", rec.Code)
	}
	if got := text("/api/v1/notes/" + noteID); got != "main text" {
		t.Errorf("Mainline note after branch delete = %v, want main text", got)
	}
}

// TestEvidenceBranchScope_UnknownBranch pins the shared 404 for a branch id that
// was never created, on every operation #758 scoped.
func TestEvidenceBranchScope_UnknownBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	scope := "?branch=" + unknownUUID

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"listSources", http.MethodGet, "/api/v1/sources" + scope, ""},
		{"createSource", http.MethodPost, "/api/v1/sources" + scope, `{"title":"x","source_type":"book"}`},
		{"searchSources", http.MethodGet, "/api/v1/sources/search?q=census&branch=" + unknownUUID, ""},
		{"getSource", http.MethodGet, "/api/v1/sources/" + unknownUUID + scope, ""},
		{"updateSource", http.MethodPut, "/api/v1/sources/" + unknownUUID + scope, `{"title":"x","version":1}`},
		{"deleteSource", http.MethodDelete, "/api/v1/sources/" + unknownUUID + scope + "&version=1", ""},
		{"getCitationsForSource", http.MethodGet, "/api/v1/sources/" + unknownUUID + "/citations" + scope, ""},
		{"createCitation", http.MethodPost, "/api/v1/citations" + scope,
			fmt.Sprintf(`{"source_id":%q,"fact_type":"person_birth","fact_owner_id":%q}`, unknownUUID, personID)},
		{"getCitation", http.MethodGet, "/api/v1/citations/" + unknownUUID + scope, ""},
		{"updateCitation", http.MethodPut, "/api/v1/citations/" + unknownUUID + scope, `{"page":"1","version":1}`},
		{"deleteCitation", http.MethodDelete, "/api/v1/citations/" + unknownUUID + scope + "&version=1", ""},
		{"formatCitation", http.MethodGet, "/api/v1/citations/" + unknownUUID + "/format" + scope, ""},
		{"getCitationsForPerson", http.MethodGet, "/api/v1/persons/" + personID + "/citations" + scope, ""},
		{"listNotes", http.MethodGet, "/api/v1/notes" + scope, ""},
		{"createNote", http.MethodPost, "/api/v1/notes" + scope, `{"text":"x"}`},
		{"getNote", http.MethodGet, "/api/v1/notes/" + unknownUUID + scope, ""},
		{"updateNote", http.MethodPut, "/api/v1/notes/" + unknownUUID + scope, `{"text":"x","version":1}`},
		{"deleteNote", http.MethodDelete, "/api/v1/notes/" + unknownUUID + scope + "&version=1", ""},
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

// TestEvidenceBranchScope_ArchivedBranch pins the terminal-branch contract on the
// #758 operations: reads 404 (the overlay is purged), writes 409 (read-only).
func TestEvidenceBranchScope_ArchivedBranch(t *testing.T) {
	server := setupBranchTestServer()
	branchID := createBranch(t, server, "Abandoned")
	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete branch: status = %d, want 204", rec.Code)
	}
	scope := "?branch=" + branchID

	for _, path := range []string{
		"/api/v1/sources" + scope,
		"/api/v1/sources/search?q=census&branch=" + branchID,
		"/api/v1/notes" + scope,
	} {
		if rec := do(t, server, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404. Body: %s", path, rec.Code, rec.Body.String())
		}
	}
	for _, write := range []struct{ path, body string }{
		{"/api/v1/sources" + scope, `{"title":"x","source_type":"book"}`},
		{"/api/v1/notes" + scope, `{"text":"x"}`},
	} {
		if rec := do(t, server, http.MethodPost, write.path, write.body); rec.Code != http.StatusConflict {
			t.Errorf("POST %s on an archived branch: status = %d, want 409. Body: %s", write.path, rec.Code, rec.Body.String())
		}
	}
}
