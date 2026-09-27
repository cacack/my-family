package api_test

import (
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// ============================================================================
// Branch-scoped snapshots and compare-to-now (#839)
// ============================================================================

// createSnapshotOn creates a snapshot on the scope named by query ("" for main)
// and returns its decoded body.
func createSnapshotOn(t *testing.T, server *api.Server, query, name string) map[string]any {
	t.Helper()
	rec := do(t, server, http.MethodPost, "/api/v1/snapshots"+query, `{"name":"`+name+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST snapshot%s: status = %d, want 201. Body: %s", query, rec.Code, rec.Body.String())
	}
	return decodeJSON(t, rec)
}

// snapshotNames lists the names of the snapshots on a scope.
func snapshotNames(t *testing.T, server *api.Server, query string) []string {
	t.Helper()
	rec := do(t, server, http.MethodGet, "/api/v1/snapshots"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET snapshots%s: status = %d. Body: %s", query, rec.Code, rec.Body.String())
	}
	items, _ := decodeJSON(t, rec)["items"].([]any)
	names := make([]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		name, _ := item["name"].(string)
		names = append(names, name)
	}
	return names
}

// comparisonChanges returns the (action, origin) pairs of a comparison's
// person changes, failing on a non-200.
func comparisonChanges(t *testing.T, server *api.Server, path string) (map[string]any, [][2]string) {
	t.Helper()
	rec := do(t, server, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	changes, _ := body["changes"].([]any)
	var out [][2]string
	for _, raw := range changes {
		c, _ := raw.(map[string]any)
		if c["entity_type"] != "person" {
			continue
		}
		action, _ := c["action"].(string)
		origin, _ := c["origin"].(string)
		out = append(out, [2]string{action, origin})
	}
	return body, out
}

// TestSnapshots_OnBranch: a snapshot taken on a branch marks the branch's view,
// is listed and fetched only on that branch, compares within it (and to now),
// and is refused when compared across branches.
func TestSnapshots_OnBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	untouchedID := createPerson(t, server, "Grace", "Hopper")
	branchID := createBranch(t, server, "Byron theory")
	otherID := createBranch(t, server, "Other theory")
	onBranch := "?branch=" + branchID

	mainSnap := createSnapshotOn(t, server, "", "Main milestone")
	if _, ok := mainSnap["branch_id"]; ok {
		t.Errorf("mainline snapshot carries branch_id: %v", mainSnap)
	}
	before := createSnapshotOn(t, server, onBranch, "Pre-DNA")
	if before["branch_id"] != branchID {
		t.Errorf("branch snapshot branch_id = %v, want %s", before["branch_id"], branchID)
	}
	beforeID, _ := before["id"].(string)
	mainID, _ := mainSnap["id"].(string)

	updateSurname(t, server, personID, onBranch, "Byron")              // the branch's own edit
	updateSurname(t, server, untouchedID, "?branch="+otherID, "Other") // never in this branch's view
	updateSurname(t, server, untouchedID, "", "Murray")                // inherited by the branch's view

	after := createSnapshotOn(t, server, onBranch, "Post-DNA")
	afterID, _ := after["id"].(string)

	// Lists are per scope.
	if got := snapshotNames(t, server, ""); len(got) != 1 || got[0] != "Main milestone" {
		t.Errorf("mainline list = %v, want [Main milestone]", got)
	}
	if got := snapshotNames(t, server, onBranch); len(got) != 2 {
		t.Errorf("branch list = %v, want the two branch snapshots", got)
	}
	if got := snapshotNames(t, server, "?branch="+otherID); len(got) != 0 {
		t.Errorf("other branch list = %v, want empty", got)
	}

	// Get is per scope.
	if rec := do(t, server, http.MethodGet, "/api/v1/snapshots/"+beforeID+onBranch, ""); rec.Code != http.StatusOK {
		t.Errorf("GET branch snapshot on its branch: status = %d, want 200", rec.Code)
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/snapshots/"+beforeID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET branch snapshot on the mainline: status = %d, want 404", rec.Code)
	}

	// Compare within the branch: its own edit and the inherited mainline edit.
	body, changes := comparisonChanges(t, server, "/api/v1/snapshots/"+beforeID+"/compare/"+afterID+onBranch)
	assertEntries(t, "branch compare", changes, [][2]string{{"updated", "branch"}, {"updated", "main"}})
	if body["older_first"] != true {
		t.Errorf("older_first = %v, want true", body["older_first"])
	}

	// Compare to now, after one more branch edit.
	updateSurname(t, server, personID, onBranch, "Byron-King")
	body, changes = comparisonChanges(t, server, "/api/v1/snapshots/"+beforeID+"/compare-current"+onBranch)
	assertEntries(t, "branch compare to now", changes,
		[][2]string{{"updated", "branch"}, {"updated", "main"}, {"updated", "branch"}})
	if snap, _ := body["snapshot"].(map[string]any); snap["id"] != beforeID {
		t.Errorf("compare-current snapshot = %v, want %s", body["snapshot"], beforeID)
	}
	if head, _ := body["head_position"].(float64); head <= 0 {
		t.Errorf("head_position = %v, want the log head", body["head_position"])
	}

	// Compare to now on the mainline: mainline edits only, no origin.
	_, changes = comparisonChanges(t, server, "/api/v1/snapshots/"+mainID+"/compare-current")
	assertEntries(t, "mainline compare to now", changes, [][2]string{{"updated", ""}})

	// Across branches: refused, clearly.
	for _, path := range []string{
		"/api/v1/snapshots/" + mainID + "/compare/" + afterID + onBranch,
		"/api/v1/snapshots/" + beforeID + "/compare/" + afterID,
		"/api/v1/snapshots/" + beforeID + "/compare-current",
		"/api/v1/snapshots/" + beforeID + "/compare-current?branch=" + otherID,
	} {
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusConflict {
			t.Errorf("GET %s: status = %d, want 409. Body: %s", path, rec.Code, rec.Body.String())
			continue
		}
		if code := decodeJSON(t, rec)["code"]; code != "snapshot_branch_mismatch" {
			t.Errorf("GET %s: code = %v, want snapshot_branch_mismatch", path, code)
		}
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/snapshots/"+unknownUUID+"/compare-current", ""); rec.Code != http.StatusNotFound {
		t.Errorf("compare-current of an unknown snapshot: status = %d, want 404", rec.Code)
	}

	// Delete is per scope: not found from the mainline, done from the branch.
	if rec := do(t, server, http.MethodDelete, "/api/v1/snapshots/"+beforeID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE branch snapshot on the mainline: status = %d, want 404", rec.Code)
	}
	if rec := do(t, server, http.MethodDelete, "/api/v1/snapshots/"+beforeID+onBranch, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE branch snapshot on its branch: status = %d, want 204", rec.Code)
	}
	if got := snapshotNames(t, server, onBranch); len(got) != 1 || got[0] != "Post-DNA" {
		t.Errorf("branch list after delete = %v, want [Post-DNA]", got)
	}
}

// TestSnapshots_BranchScopeErrors: the standard scope errors — unknown branch
// 404, a write to an archived branch 409, a read of one 404, a malformed id 400.
func TestSnapshots_BranchScopeErrors(t *testing.T) {
	server := setupBranchTestServer()
	archived := createBranch(t, server, "Abandoned")
	snap := createSnapshotOn(t, server, "?branch="+archived, "Before abandoning")
	snapID, _ := snap["id"].(string)
	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+archived, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete branch: status = %d", rec.Code)
	}

	reads := []string{
		"/api/v1/snapshots",
		"/api/v1/snapshots/" + snapID,
		"/api/v1/snapshots/" + snapID + "/compare-current",
		"/api/v1/snapshots/" + snapID + "/compare/" + snapID,
	}
	for _, path := range reads {
		if rec := do(t, server, http.MethodGet, path+"?branch="+unknownUUID, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s on unknown branch: status = %d, want 404", path, rec.Code)
		}
		if rec := do(t, server, http.MethodGet, path+"?branch="+archived, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s on archived branch: status = %d, want 404", path, rec.Code)
		}
		if rec := do(t, server, http.MethodGet, path+"?branch=not-a-uuid", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s with malformed branch: status = %d, want 400", path, rec.Code)
		}
	}
	if rec := do(t, server, http.MethodPost, "/api/v1/snapshots?branch="+archived, `{"name":"Too late"}`); rec.Code != http.StatusConflict {
		t.Errorf("POST snapshot on archived branch: status = %d, want 409", rec.Code)
	}
	if rec := do(t, server, http.MethodDelete, "/api/v1/snapshots/"+snapID+"?branch="+archived, ""); rec.Code != http.StatusConflict {
		t.Errorf("DELETE snapshot on archived branch: status = %d, want 409", rec.Code)
	}
	if rec := do(t, server, http.MethodPost, "/api/v1/snapshots?branch="+unknownUUID, `{"name":"Nowhere"}`); rec.Code != http.StatusNotFound {
		t.Errorf("POST snapshot on unknown branch: status = %d, want 404", rec.Code)
	}
}
