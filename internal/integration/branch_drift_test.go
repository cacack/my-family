package integration_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestBranchDrift_EndToEnd proves the "main moved underneath" counts (#837)
// agree across every storage backend (DB-001), and that the on-branch-entities
// count is the same number a full compare reports as main_change_count — the
// indicator is a cheap preview of compare, so the two must never disagree.
//
// Fixtures use neutral placeholder names only (public repo — no real PII).
func TestBranchDrift_EndToEnd(t *testing.T) {
	forEachBackend(t, runBranchDrift)
}

func runBranchDrift(t *testing.T, server *api.Server) {
	t.Helper()

	touched := createPerson(t, server, "Alex", "Original")
	untouched := createPerson(t, server, "Sam", "Steady")
	branchID := createBranch(t, server, "drift-line")
	touchedPath := "/api/v1/persons/" + touched
	untouchedPath := "/api/v1/persons/" + untouched

	// Before main moves, there is nothing to report.
	drift := mustDo(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/drift", "", http.StatusOK)
	if drift["main_change_count"] != float64(0) || drift["main_change_count_on_branch_entities"] != float64(0) {
		t.Errorf("drift before main moved = %v, want 0 / 0", drift)
	}

	updateSurname(t, server, touchedPath, branchID, "Branchside")
	updateSurname(t, server, touchedPath, "", "Mainside")
	updateSurname(t, server, touchedPath, "", "Mainside Again")
	updateSurname(t, server, untouchedPath, "", "Moved")
	// Another fork is research metadata on main, not a genealogy change.
	createBranch(t, server, "unrelated-line")

	drift = mustDo(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/drift", "", http.StatusOK)
	if drift["main_change_count"] != float64(3) {
		t.Errorf("main_change_count = %v, want 3", drift["main_change_count"])
	}
	if drift["main_change_count_on_branch_entities"] != float64(2) {
		t.Errorf("main_change_count_on_branch_entities = %v, want 2", drift["main_change_count_on_branch_entities"])
	}
	if drift["has_more"] != false {
		t.Errorf("has_more = %v, want false", drift["has_more"])
	}

	compare := mustDo(t, server, http.MethodGet, comparePath(branchID), "", http.StatusOK)
	if compare["main_change_count"] != drift["main_change_count_on_branch_entities"] {
		t.Errorf("compare main_change_count = %v, drift on-branch count = %v; want equal",
			compare["main_change_count"], drift["main_change_count_on_branch_entities"])
	}

	list := mustDo(t, server, http.MethodGet, "/api/v1/branches?include_drift=true", "", http.StatusOK)
	found := false
	for _, raw := range jsonArray(t, list, "items") {
		item, _ := raw.(map[string]any)
		if item["id"] != branchID {
			continue
		}
		found = true
		listed, _ := item["drift"].(map[string]any)
		if listed == nil || listed["main_change_count"] != float64(3) || listed["main_change_count_on_branch_entities"] != float64(2) {
			t.Errorf("listed drift = %v, want 3 / 2", listed)
		}
	}
	if !found {
		t.Errorf("branch %s missing from the list", branchID)
	}

	// A GEDCOM import on main moves the drift by exactly the changes history
	// shows for it: the import's own GedcomImported summary record is not a
	// genealogy change, so it is counted by neither.
	historyBefore := len(jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/history?limit=1000", "", http.StatusOK), "items"))
	importGedcom(t, server, driftImportGedcom)
	historyAfter := len(jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/history?limit=1000", "", http.StatusOK), "items"))
	imported := historyAfter - historyBefore
	if imported == 0 {
		t.Fatal("the import added no history entries; the fixture is not exercising the import")
	}

	drift = mustDo(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/drift", "", http.StatusOK)
	if want := float64(3 + imported); drift["main_change_count"] != want {
		t.Errorf("main_change_count after import = %v, want %v (3 + %d imported changes, no summary record)",
			drift["main_change_count"], want, imported)
	}
	if drift["main_change_count_on_branch_entities"] != float64(2) {
		t.Errorf("main_change_count_on_branch_entities after import = %v, want 2", drift["main_change_count_on_branch_entities"])
	}
}

// driftImportGedcom is a two-person tree with neutral placeholder names.
const driftImportGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Pat /Imported/
1 SEX F
0 @I2@ INDI
1 NAME Lee /Imported/
1 SEX M
0 TRLR
`

// importGedcom imports a GEDCOM file on the mainline.
func importGedcom(t *testing.T, server *api.Server, content string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "drift.ged")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import failed: %d: %s", rec.Code, rec.Body.String())
	}
}
