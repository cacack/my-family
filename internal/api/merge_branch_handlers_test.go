package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

// Person merge honors ?branch= (#834): the merge lands on the branch only, the
// response is the survivor as the branch sees it, and a non-active branch
// refuses the write.

func TestMergePersons_OnBranch(t *testing.T) {
	server := setupBranchTestServer()
	survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
	mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat", "birth_place", "Riverton")
	branchID := createBranch(t, server, "same person")

	body := fmt.Sprintf(`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`, survivorID, mergedID, sv, mv)
	rec := do(t, server, http.MethodPost, "/api/v1/persons/merge?branch="+branchID, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("branch merge status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	person, _ := decodeJSON(t, rec)["person"].(map[string]any)
	if person["birth_place"] != "Riverton" {
		t.Errorf("response person = %v, want the branch's survivor with the merged birth place", person)
	}

	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+mergedID+"?branch="+branchID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("branch GET merged person = %d, want 404", rec.Code)
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+mergedID, ""); rec.Code != http.StatusOK {
		t.Errorf("main GET merged person = %d, want 200 - the branch merge leaked onto main", rec.Code)
	}
}

func TestBatchMergePersons_OnBranch(t *testing.T) {
	server := setupBranchTestServer()
	survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
	mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
	branchID := createBranch(t, server, "batch")

	body := fmt.Sprintf(`{"merges":[{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}]}`, survivorID, mergedID, sv, mv)
	rec := do(t, server, http.MethodPost, "/api/v1/persons/merge/batch?branch="+branchID, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("branch batch merge status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if got := decodeJSON(t, rec)["successful"]; got != float64(1) {
		t.Errorf("successful = %v, want 1", got)
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+mergedID+"?branch="+branchID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("branch GET merged person = %d, want 404", rec.Code)
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+mergedID, ""); rec.Code != http.StatusOK {
		t.Errorf("main GET merged person = %d, want 200", rec.Code)
	}
}

func TestMergePersons_BranchScopeRefusals(t *testing.T) {
	server := setupBranchTestServer()
	survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
	mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
	body := fmt.Sprintf(`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`, survivorID, mergedID, sv, mv)
	batch := `{"merges":[` + body + `]}`

	unknown := "00000000-0000-0000-0000-00000000abcd"
	if rec := do(t, server, http.MethodPost, "/api/v1/persons/merge?branch="+unknown, body); rec.Code != http.StatusNotFound {
		t.Errorf("merge on an unknown branch = %d, want 404", rec.Code)
	}
	if rec := do(t, server, http.MethodPost, "/api/v1/persons/merge/batch?branch="+unknown, batch); rec.Code != http.StatusNotFound {
		t.Errorf("batch merge on an unknown branch = %d, want 404", rec.Code)
	}

	branchID := createBranch(t, server, "archived")
	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete branch = %d, want 204. Body: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, server, http.MethodPost, "/api/v1/persons/merge?branch="+branchID, body); rec.Code != http.StatusConflict {
		t.Errorf("merge on an archived branch = %d, want 409", rec.Code)
	}
}
