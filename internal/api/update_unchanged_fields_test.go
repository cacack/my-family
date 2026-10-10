package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestOneFieldEdit_ListsOneChange is the #900 regression: the person form sends
// every field, and a save that only edits the birth place must show exactly one
// change in the person's history and in the branch compare, not one per field.
func TestOneFieldEdit_ListsOneChange(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodPost, "/api/v1/persons",
		`{"given_name":"Ada","surname":"Lovelace","gender":"female","birth_date":"1875","birth_place":"London"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	created := decodeJSON(t, rec)
	personID, _ := created["id"].(string)
	version, _ := created["version"].(float64)
	branchID := createBranch(t, server, "Birthplace theory")

	// The full form, as the person edit page submits it, with one field edited.
	form := `{"given_name":"Ada","surname":"Lovelace","birth_date":"1875","birth_place":"%s",` +
		`"death_date":"","death_place":"","notes":"","version":%d}`
	if rec := do(t, server, http.MethodPut, fmt.Sprintf("/api/v1/persons/%s?branch=%s", personID, branchID),
		fmt.Sprintf(form, "Bath", int64(version))); rec.Code != http.StatusOK {
		t.Fatalf("Branch update: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, server, http.MethodPut, "/api/v1/persons/"+personID,
		fmt.Sprintf(form, "Bristol", int64(version))); rec.Code != http.StatusOK {
		t.Fatalf("Main update: status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	assertOneBirthPlaceChange := func(where string, entry map[string]any) {
		t.Helper()
		if entry["action"] != "updated" {
			t.Fatalf("%s: action = %v, want updated", where, entry["action"])
		}
		changes, _ := entry["changes"].(map[string]any)
		if _, ok := changes["birth_place"]; !ok || len(changes) != 1 {
			t.Errorf("%s: changes = %v, want only birth_place", where, changes)
		}
	}

	rec = do(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/history", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("History: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	var update map[string]any
	for _, item := range decodeJSON(t, rec)["items"].([]any) {
		if entry, _ := item.(map[string]any); entry["action"] == "updated" {
			update = entry
		}
	}
	if update == nil {
		t.Fatalf("History has no update entry: %s", rec.Body.String())
	}
	assertOneBirthPlaceChange("history", update)

	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/compare", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Compare: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON(t, rec)
	for _, side := range []string{"branch_changes", "main_changes"} {
		entries, _ := resp[side].([]any)
		if len(entries) != 1 {
			t.Fatalf("len(%s) = %d, want 1. Body: %s", side, len(entries), rec.Body.String())
		}
		entry, _ := entries[0].(map[string]any)
		assertOneBirthPlaceChange(side, entry)
	}
	conflicts, _ := resp["conflicts"].([]any)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %v, want one", conflicts)
	}
	conflict, _ := conflicts[0].(map[string]any)
	if values, _ := conflict["field_values"].([]any); len(values) != 1 {
		t.Errorf("conflict field_values = %v, want only birth_place", conflict["field_values"])
	}
}
