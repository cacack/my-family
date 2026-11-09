package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
)

// duplicateCount reads GET /persons/duplicates (with an optional query) and
// returns its total.
func duplicateCount(t *testing.T, server *api.Server, query string) int {
	t.Helper()
	rec := do(t, server, http.MethodGet, "/api/v1/persons/duplicates"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GetPersonsDuplicates status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	var resp api.DuplicatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode duplicates: %v", err)
	}
	return resp.Total
}

// dismissedList reads GET /persons/duplicates/dismissed (with an optional query).
func dismissedList(t *testing.T, server *api.Server, query string) api.DismissedDuplicatesResponse {
	t.Helper()
	rec := do(t, server, http.MethodGet, "/api/v1/persons/duplicates/dismissed"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ListDismissedDuplicates status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	var resp api.DismissedDuplicatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dismissed: %v", err)
	}
	return resp
}

// TestDismissDuplicate_PersistsAcrossReloads is the #1036 regression: a
// dismissed pair stays out of the duplicate results on the next load, is
// listed as dismissed, and comes back once restored.
func TestDismissDuplicate_PersistsAcrossReloads(t *testing.T) {
	server := setupBranchTestServer()
	father, _ := createMergeTestPerson(t, server, "John", "Doe", "birth_date", "1850")
	son, _ := createMergeTestPerson(t, server, "John", "Doe", "birth_date", "1850")
	if n := duplicateCount(t, server, ""); n != 1 {
		t.Fatalf("duplicates before dismissing = %d, want 1", n)
	}

	path := "/api/v1/persons/duplicates/" + father + "/" + son + "/dismiss"
	if rec := do(t, server, http.MethodPost, path, `{"reason":"father and son"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("DismissDuplicate status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	if n := duplicateCount(t, server, ""); n != 0 {
		t.Errorf("duplicates after dismissing = %d, want 0", n)
	}
	list := dismissedList(t, server, "")
	if list.Total != 1 || len(list.Dismissals) != 1 {
		t.Fatalf("dismissed list = %+v, want one pair", list)
	}
	got := list.Dismissals[0]
	ids := map[string]bool{got.Person1Id.String(): true, got.Person2Id.String(): true}
	if !ids[father] || !ids[son] || got.Person1Name != "John Doe" || got.Reason == nil || *got.Reason != "father and son" {
		t.Errorf("dismissed pair = %+v", got)
	}

	// Restoring takes either order of the two ids.
	restore := "/api/v1/persons/duplicates/" + son + "/" + father + "/dismiss"
	if rec := do(t, server, http.MethodDelete, restore, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("RestoreDuplicate status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	if n := duplicateCount(t, server, ""); n != 1 {
		t.Errorf("duplicates after restoring = %d, want 1", n)
	}
	if list := dismissedList(t, server, ""); list.Total != 0 {
		t.Errorf("dismissed after restoring = %d, want 0", list.Total)
	}
	if rec := do(t, server, http.MethodDelete, restore, ""); rec.Code != http.StatusNotFound {
		t.Errorf("second restore status = %d, want 404", rec.Code)
	}
}

func TestDismissDuplicate_SamePerson(t *testing.T) {
	server := setupBranchTestServer()
	id, _ := createMergeTestPerson(t, server, "John", "Doe")
	rec := do(t, server, http.MethodPost, "/api/v1/persons/duplicates/"+id+"/"+id+"/dismiss", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	rec = do(t, server, http.MethodDelete, "/api/v1/persons/duplicates/"+id+"/"+id+"/dismiss", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("restore status = %d, want 400", rec.Code)
	}
}

// The batch endpoint persists each dismissal the same way.
func TestBatchDismissDuplicates_Persists(t *testing.T) {
	server := setupBranchTestServer()
	a1, _ := createMergeTestPerson(t, server, "Ann", "Roe", "birth_date", "1900")
	a2, _ := createMergeTestPerson(t, server, "Ann", "Roe", "birth_date", "1900")
	b1, _ := createMergeTestPerson(t, server, "Bob", "Poe", "birth_date", "1910")
	b2, _ := createMergeTestPerson(t, server, "Bob", "Poe", "birth_date", "1910")
	if n := duplicateCount(t, server, ""); n != 2 {
		t.Fatalf("duplicates before dismissing = %d, want 2", n)
	}

	body := `{"dismissals":[{"person1_id":"` + a1 + `","person2_id":"` + a2 + `"},` +
		`{"person1_id":"` + b1 + `","person2_id":"` + b2 + `","reason":"brothers"},` +
		`{"person1_id":"` + b1 + `","person2_id":"` + uuid.New().String() + `"}]}`
	rec := do(t, server, http.MethodPost, "/api/v1/persons/duplicates/dismiss/batch", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("BatchDismissDuplicates status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	var resp api.BatchDismissResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Successful != 2 || resp.Failed != 1 {
		t.Errorf("batch = %+v, want 2 successful and 1 failed", resp)
	}
	if n := duplicateCount(t, server, ""); n != 0 {
		t.Errorf("duplicates after batch dismissing = %d, want 0", n)
	}
	if list := dismissedList(t, server, ""); list.Total != 2 {
		t.Errorf("dismissed after batch = %d, want 2", list.Total)
	}
}

// A dismissal made with ?branch= stays on the branch until it is merged.
func TestDismissDuplicate_OnBranch(t *testing.T) {
	server := setupBranchTestServer()
	a, _ := createMergeTestPerson(t, server, "Cal", "Moe", "birth_date", "1800")
	b, _ := createMergeTestPerson(t, server, "Cal", "Moe", "birth_date", "1800")
	branch := createBranch(t, server, "Cal hypothesis")
	scope := "?branch=" + branch

	rec := do(t, server, http.MethodPost, "/api/v1/persons/duplicates/"+a+"/"+b+"/dismiss"+scope, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("branch DismissDuplicate status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	if n := duplicateCount(t, server, scope); n != 0 {
		t.Errorf("branch duplicates = %d, want 0", n)
	}
	if list := dismissedList(t, server, scope); list.Total != 1 {
		t.Errorf("branch dismissed = %d, want 1", list.Total)
	}
	if n := duplicateCount(t, server, ""); n != 1 {
		t.Errorf("mainline duplicates = %d, want 1 (the branch's decision stays on the branch)", n)
	}

	if rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branch+"/merge", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("MergeBranch status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	if n := duplicateCount(t, server, ""); n != 0 {
		t.Errorf("mainline duplicates after merge = %d, want 0", n)
	}
}
