package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// ============================================================================
// Branch-scoped entity history and mainline-only rollback (#823, #824)
// ============================================================================

// historyPage fetches an entity history and returns its total and the
// (action, origin) pairs of its items, failing the test on a non-200.
//
// Items whose action is "unknown" are dropped: creating a person also records
// its primary name on the person's stream, an event type the change log does
// not map yet and renders as "unknown" (pre-existing, tracked in #739). They
// still count toward total, on the mainline and on a branch alike.
func historyPage(t *testing.T, server *api.Server, path string) (total int, entries [][2]string) {
	t.Helper()
	rec := do(t, server, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	n, _ := body["total"].(float64)
	items, _ := body["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		action, _ := item["action"].(string)
		origin, _ := item["origin"].(string)
		if action == "unknown" {
			continue
		}
		entries = append(entries, [2]string{action, origin})
	}
	return int(n), entries
}

// entityVersion reads the version of an entity at path.
func entityVersion(t *testing.T, server *api.Server, path string) int64 {
	t.Helper()
	rec := do(t, server, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
	}
	v, ok := decodeJSON(t, rec)["version"].(float64)
	if !ok {
		t.Fatalf("GET %s: no version in %s", path, rec.Body.String())
	}
	return int64(v)
}

// updateSurname renames a person on the scope named by query ("" for main).
func updateSurname(t *testing.T, server *api.Server, personID, query, surname string) {
	t.Helper()
	version := entityVersion(t, server, "/api/v1/persons/"+personID+query)
	rec := do(t, server, http.MethodPut, "/api/v1/persons/"+personID+query,
		fmt.Sprintf(`{"surname":%q,"version":%d}`, surname, version))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT person%s: status = %d, want 200. Body: %s", query, rec.Code, rec.Body.String())
	}
}

func assertEntries(t *testing.T, label string, got [][2]string, want [][2]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: entries = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: entry %d = %v, want %v (all: %v)", label, i, got[i], want[i], got)
		}
	}
}

// TestPersonHistory_BranchOnlyPerson is #823: a person created on a branch has
// a history on that branch (it used to 404, which blanked the detail page),
// while the mainline still does not know the person.
func TestPersonHistory_BranchOnlyPerson(t *testing.T) {
	server := setupBranchTestServer()
	branchID := createBranch(t, server, "Speculative")
	onBranch := "?branch=" + branchID

	rec := do(t, server, http.MethodPost, "/api/v1/persons"+onBranch, `{"given_name":"Hypothetical","surname":"Ancestor"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Branch create: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	personID, _ := decodeJSON(t, rec)["id"].(string)
	updateSurname(t, server, personID, onBranch, "Forebear")

	_, entries := historyPage(t, server, "/api/v1/persons/"+personID+"/history"+onBranch)
	assertEntries(t, "branch history", entries, [][2]string{{"created", "branch"}, {"updated", "branch"}})

	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/history", ""); rec.Code != http.StatusNotFound {
		t.Errorf("Mainline history of a branch-only person: status = %d, want 404", rec.Code)
	}
}

// TestPersonHistory_BranchInheritsMainline pins the overlay definition of a
// branch's entity history (#824): every mainline event until the branch's first
// write to the entity is inherited (the live overlay shows it), the branch's own
// events follow, and mainline events after that first write — or any other
// branch's events — are not part of the branch's view.
func TestPersonHistory_BranchInheritsMainline(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	updateSurname(t, server, personID, "", "King")
	untouchedID := createPerson(t, server, "Grace", "Hopper")
	branchID := createBranch(t, server, "Byron theory")
	otherID := createBranch(t, server, "Other theory")
	onBranch := "?branch=" + branchID
	historyPath := "/api/v1/persons/" + personID + "/history"

	// A mainline edit after the fork to a person the branch never touches: the
	// live overlay shows it on the branch, so the branch history inherits it.
	updateSurname(t, server, untouchedID, "", "Murray")
	untouchedTotal, entries := historyPage(t, server, "/api/v1/persons/"+untouchedID+"/history"+onBranch)
	assertEntries(t, "untouched person", entries, [][2]string{{"created", "main"}, {"updated", "main"}})
	if mainTotal, _ := historyPage(t, server, "/api/v1/persons/"+untouchedID+"/history"); mainTotal != untouchedTotal {
		t.Errorf("Untouched person branch history total = %d, want the mainline's %d", untouchedTotal, mainTotal)
	}

	inherited, _ := historyPage(t, server, historyPath+onBranch)

	// The branch's own edit, then another branch's edit and a mainline edit.
	updateSurname(t, server, personID, onBranch, "Lovelace-Byron")
	updateSurname(t, server, personID, "?branch="+otherID, "Other")
	updateSurname(t, server, personID, "", "Main-after-branch-write")

	total, entries := historyPage(t, server, historyPath+onBranch)
	if total != inherited+1 {
		t.Errorf("Branch history total = %d, want %d", total, inherited+1)
	}
	assertEntries(t, "branch after own edit", entries, [][2]string{
		{"created", "main"}, {"updated", "main"}, {"updated", "branch"},
	})

	// The mainline history is unchanged by any branch and carries no origin.
	mainTotal, entries := historyPage(t, server, historyPath)
	if mainTotal != inherited+1 {
		t.Errorf("Mainline history total = %d, want %d", mainTotal, inherited+1)
	}
	assertEntries(t, "mainline", entries, [][2]string{
		{"created", ""}, {"updated", ""}, {"updated", ""},
	})

	// Pagination is applied after the branch filter.
	paged, entries := historyPage(t, server, fmt.Sprintf("%s%s&limit=2&offset=%d", historyPath, onBranch, total-2))
	if paged != total {
		t.Errorf("Paged branch history total = %d, want %d", paged, total)
	}
	assertEntries(t, "paged branch", entries, [][2]string{{"updated", "main"}, {"updated", "branch"}})
	rec := do(t, server, http.MethodGet, historyPath+onBranch+"&limit=2&offset=0", "")
	if hasMore, _ := decodeJSON(t, rec)["has_more"].(bool); !hasMore {
		t.Errorf("First page has_more = false, want true")
	}
	rec = do(t, server, http.MethodGet, fmt.Sprintf("%s%s&limit=2&offset=%d", historyPath, onBranch, total-2), "")
	if hasMore, _ := decodeJSON(t, rec)["has_more"].(bool); hasMore {
		t.Errorf("Last page has_more = true, want false")
	}
}

// TestPersonHistory_BranchDeletedPerson: a person the branch deleted is not
// found in the branch's view, so neither is their history.
func TestPersonHistory_BranchDeletedPerson(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	branchID := createBranch(t, server, "Deletion theory")
	if rec := do(t, server, http.MethodDelete, "/api/v1/persons/"+personID+"?branch="+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Branch delete: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/history?branch="+branchID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("Branch history of a branch-deleted person: status = %d, want 404", rec.Code)
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/history", ""); rec.Code != http.StatusOK {
		t.Errorf("Mainline history: status = %d, want 200", rec.Code)
	}
}

// TestFamilyHistory_BranchOnlyFamily is #823 for families: a family created on
// a branch (between a mainline partner and a branch-only one) has a history on
// the branch, including its child links, and none on the mainline.
func TestFamilyHistory_BranchOnlyFamily(t *testing.T) {
	server := setupBranchTestServer()
	p1 := createPerson(t, server, "John", "Smith")
	branchID := createBranch(t, server, "Marriage theory")
	onBranch := "?branch=" + branchID

	rec := do(t, server, http.MethodPost, "/api/v1/persons"+onBranch, `{"given_name":"Mary","surname":"Jones"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Branch person create: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	p2, _ := decodeJSON(t, rec)["id"].(string)

	rec = do(t, server, http.MethodPost, "/api/v1/families"+onBranch,
		fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q}`, p1, p2))
	if rec.Code != http.StatusCreated {
		t.Fatalf("Branch family create: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	familyID, _ := decodeJSON(t, rec)["id"].(string)

	version := entityVersion(t, server, "/api/v1/families/"+familyID+onBranch)
	rec = do(t, server, http.MethodPut, "/api/v1/families/"+familyID+onBranch,
		fmt.Sprintf(`{"marriage_place":"Boston","version":%d}`, version))
	if rec.Code != http.StatusOK {
		t.Fatalf("Branch family update: status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	total, entries := historyPage(t, server, "/api/v1/families/"+familyID+"/history"+onBranch)
	if total != 2 {
		t.Errorf("Branch family history total = %d, want 2", total)
	}
	assertEntries(t, "branch family", entries, [][2]string{{"created", "branch"}, {"updated", "branch"}})

	if rec := do(t, server, http.MethodGet, "/api/v1/families/"+familyID+"/history", ""); rec.Code != http.StatusNotFound {
		t.Errorf("Mainline history of a branch-only family: status = %d, want 404", rec.Code)
	}
}

// TestFamilyHistory_BranchInheritsMainline: a mainline family's history on a
// branch that has not touched it is the mainline's, labelled as inherited.
func TestFamilyHistory_BranchInheritsMainline(t *testing.T) {
	server := setupBranchTestServer()
	familyID := createFamily(t, server, createPerson(t, server, "John", "Smith"), createPerson(t, server, "Mary", "Jones"))
	branchID := createBranch(t, server, "Untouched")

	_, entries := historyPage(t, server, "/api/v1/families/"+familyID+"/history?branch="+branchID)
	assertEntries(t, "inherited family", entries, [][2]string{{"created", "main"}})
}

// TestHistory_BranchScopeErrors: the history endpoints validate ?branch= like
// every other scoped read.
func TestHistory_BranchScopeErrors(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	familyID := createFamily(t, server, personID, createPerson(t, server, "William", "King"))
	archived := createBranch(t, server, "Abandoned")
	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+archived, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete branch: status = %d", rec.Code)
	}

	for _, path := range []string{
		"/api/v1/persons/" + personID + "/history",
		"/api/v1/families/" + familyID + "/history",
	} {
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
}

// TestRollback_RefusedOnBranch is the #824 API backstop: rollback and restore
// points are mainline-only, so a request carrying ?branch= is refused with 409
// rollback_mainline_only for every entity type — and the mainline is left
// untouched (the old behavior silently rolled main back).
func TestRollback_RefusedOnBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	updateSurname(t, server, personID, "", "King")
	familyID := createFamily(t, server, personID, createPerson(t, server, "William", "King"))
	sourceID := createSource(t, server, "Parish register")
	citationID := createCitation(t, server, sourceID, personID)
	branchID := createBranch(t, server, "Research")
	onBranch := "?branch=" + branchID

	// A person that exists only on the branch is refused too, not 404'd.
	rec := do(t, server, http.MethodPost, "/api/v1/persons"+onBranch, `{"given_name":"Only","surname":"Branch"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Branch create: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	branchOnlyID, _ := decodeJSON(t, rec)["id"].(string)

	mainVersion := entityVersion(t, server, "/api/v1/persons/"+personID)

	for _, tc := range []struct{ kind, id string }{
		{"persons", personID},
		{"persons", branchOnlyID},
		{"families", familyID},
		{"sources", sourceID},
		{"citations", citationID},
	} {
		base := "/api/v1/" + tc.kind + "/" + tc.id
		rec := do(t, server, http.MethodGet, base+"/restore-points"+onBranch, "")
		if rec.Code != http.StatusConflict {
			t.Errorf("GET %s/restore-points on branch: status = %d, want 409. Body: %s", base, rec.Code, rec.Body.String())
		} else if code := decodeJSON(t, rec)["code"]; code != "rollback_mainline_only" {
			t.Errorf("GET %s/restore-points on branch: code = %v, want rollback_mainline_only", base, code)
		}

		rec = do(t, server, http.MethodPost, base+"/rollback"+onBranch, `{"target_version":1}`)
		if rec.Code != http.StatusConflict {
			t.Errorf("POST %s/rollback on branch: status = %d, want 409. Body: %s", base, rec.Code, rec.Body.String())
		} else if code := decodeJSON(t, rec)["code"]; code != "rollback_mainline_only" {
			t.Errorf("POST %s/rollback on branch: code = %v, want rollback_mainline_only", base, code)
		}

		// Unknown branch: 404 like every scoped operation.
		if rec := do(t, server, http.MethodPost, base+"/rollback?branch="+unknownUUID, `{"target_version":1}`); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s/rollback on unknown branch: status = %d, want 404", base, rec.Code)
		}
		if rec := do(t, server, http.MethodGet, base+"/restore-points?branch="+unknownUUID, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s/restore-points on unknown branch: status = %d, want 404", base, rec.Code)
		}
	}

	if got := entityVersion(t, server, "/api/v1/persons/"+personID); got != mainVersion {
		t.Errorf("Mainline person version = %d after refused branch rollback, want %d", got, mainVersion)
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID, ""); decodeJSON(t, rec)["surname"] != "King" {
		t.Errorf("Mainline surname changed by a refused branch rollback")
	}

	// The mainline rollback still works.
	rec = do(t, server, http.MethodPost, "/api/v1/persons/"+personID+"/rollback", `{"target_version":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Mainline rollback: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/restore-points", ""); rec.Code != http.StatusOK {
		t.Errorf("Mainline restore points: status = %d, want 200", rec.Code)
	}
}

// TestRollback_ArchivedBranch: a rollback naming a terminal branch is a write
// to it and so 409s at scope resolution; restore points on it 404 (reads of a
// terminal branch have no view).
func TestRollback_ArchivedBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	branchID := createBranch(t, server, "Abandoned")
	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete branch: status = %d", rec.Code)
	}
	if rec := do(t, server, http.MethodPost, "/api/v1/persons/"+personID+"/rollback?branch="+branchID, `{"target_version":1}`); rec.Code != http.StatusConflict {
		t.Errorf("Rollback on archived branch: status = %d, want 409", rec.Code)
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/restore-points?branch="+branchID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("Restore points on archived branch: status = %d, want 404", rec.Code)
	}
}
