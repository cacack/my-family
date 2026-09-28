package integration_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestBranchReads_KinshipAndSearch covers the reads #829 scoped to a branch —
// search, the families list, the group sheet, the Ahnentafel, descendancy and
// the relationship calculator — on every backend. For each one, a person or
// family created on the branch appears on the branch and not on the mainline,
// and one deleted on the branch still appears on the mainline but not on the
// branch.
func TestBranchReads_KinshipAndSearch(t *testing.T) {
	forEachBackend(t, runBranchReads)
}

// branchReadsFixture is the tree runBranchReads reads: a mainline couple with
// one child (Alpha) and a second, unrelated mainline family; then, on the
// branch, a new child (Gamma), a new family, and Alpha and the second family
// deleted.
type branchReadsFixture struct {
	branch, father, mother, alpha, gamma string
	family, doomedFamily, branchFamily   string
}

func seedBranchReads(t *testing.T, server *api.Server) branchReadsFixture {
	t.Helper()
	var f branchReadsFixture
	// The pedigree places parents by gender, so the couple needs one.
	f.father = mustString(t, mustDo(t, server, http.MethodPost, "/api/v1/persons",
		`{"given_name":"Faramond","surname":"Kinread","gender":"male"}`, http.StatusCreated), "id")
	f.mother = mustString(t, mustDo(t, server, http.MethodPost, "/api/v1/persons",
		`{"given_name":"Mirabel","surname":"Kinread","gender":"female"}`, http.StatusCreated), "id")
	f.alpha = createPerson(t, server, "Alphonse", "Kinread")
	f.family = createFamily(t, server, f.father, f.mother)
	mustDo(t, server, http.MethodPost, "/api/v1/families/"+f.family+"/children",
		fmt.Sprintf(`{"person_id":%q}`, f.alpha), http.StatusCreated)
	f.doomedFamily = createFamily(t, server,
		createPerson(t, server, "Doran", "Kinread"), createPerson(t, server, "Delphine", "Kinread"))

	f.branch = createBranch(t, server, "kinship reads")
	f.gamma = mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/persons", f.branch),
		`{"given_name":"Gwendolyn","surname":"Kinread","gender":"female"}`, http.StatusCreated), "id")
	mustDo(t, server, http.MethodPost, scoped("/api/v1/families/"+f.family+"/children", f.branch),
		fmt.Sprintf(`{"person_id":%q}`, f.gamma), http.StatusCreated)
	f.branchFamily = mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/families", f.branch),
		fmt.Sprintf(`{"partner1_id":%q,"relationship_type":"marriage"}`, f.gamma), http.StatusCreated), "id")
	// A linked person cannot be deleted, so the branch unlinks Alpha first.
	mustDo(t, server, http.MethodDelete, scoped("/api/v1/families/"+f.family+"/children/"+f.alpha, f.branch), "", http.StatusNoContent)
	mustDo(t, server, http.MethodDelete, scoped("/api/v1/persons/"+f.alpha, f.branch), "", http.StatusNoContent)
	mustDo(t, server, http.MethodDelete, scoped("/api/v1/families/"+f.doomedFamily, f.branch), "", http.StatusNoContent)
	return f
}

func runBranchReads(t *testing.T, server *api.Server) {
	f := seedBranchReads(t, server)

	t.Run("searchPersons", func(t *testing.T) {
		assert := func(query, branchID string, wantFound bool, id string) {
			t.Helper()
			resp := mustDo(t, server, http.MethodGet, withScope("/api/v1/search?q="+query, branchID), "", http.StatusOK)
			if got := slices.Contains(itemIDs(t, resp), id); got != wantFound {
				t.Errorf("search %q on %q: found = %v, want %v", query, branchID, got, wantFound)
			}
		}
		assert("Gwendolyn", f.branch, true, f.gamma)
		assert("Gwendolyn", "", false, f.gamma)
		assert("Alphonse", f.branch, false, f.alpha)
		assert("Alphonse", "", true, f.alpha)
		// Phonetic matching reads the same scoped view.
		assert("Gwendolyn&soundex=true", f.branch, true, f.gamma)
		assert("Alphonse&soundex=true", f.branch, false, f.alpha)
	})

	t.Run("listFamilies", func(t *testing.T) {
		onBranch := itemIDs(t, mustDo(t, server, http.MethodGet, withScope("/api/v1/families?limit=100", f.branch), "", http.StatusOK))
		onMain := itemIDs(t, mustDo(t, server, http.MethodGet, "/api/v1/families?limit=100", "", http.StatusOK))
		if !slices.Contains(onBranch, f.branchFamily) || slices.Contains(onMain, f.branchFamily) {
			t.Errorf("branch-created family: on branch %v, on main %v", onBranch, onMain)
		}
		if slices.Contains(onBranch, f.doomedFamily) || !slices.Contains(onMain, f.doomedFamily) {
			t.Errorf("branch-deleted family: on branch %v, on main %v", onBranch, onMain)
		}
	})

	t.Run("getFamilyGroupSheet", func(t *testing.T) {
		sheetPath := "/api/v1/families/" + f.family + "/group-sheet"
		onBranch := childIDs(t, mustDo(t, server, http.MethodGet, withScope(sheetPath, f.branch), "", http.StatusOK))
		onMain := childIDs(t, mustDo(t, server, http.MethodGet, sheetPath, "", http.StatusOK))
		if !slices.Equal(onBranch, []string{f.gamma}) {
			t.Errorf("group sheet children on branch = %v, want only the branch-created child %s", onBranch, f.gamma)
		}
		if !slices.Equal(onMain, []string{f.alpha}) {
			t.Errorf("group sheet children on main = %v, want only %s", onMain, f.alpha)
		}
		mustDo(t, server, http.MethodGet, withScope("/api/v1/families/"+f.branchFamily+"/group-sheet", f.branch), "", http.StatusOK)
		mustDo(t, server, http.MethodGet, "/api/v1/families/"+f.branchFamily+"/group-sheet", "", http.StatusNotFound)
		mustDo(t, server, http.MethodGet, withScope("/api/v1/families/"+f.doomedFamily+"/group-sheet", f.branch), "", http.StatusNotFound)
	})

	t.Run("getAhnentafel", func(t *testing.T) {
		report := mustDo(t, server, http.MethodGet, withScope("/api/v1/ahnentafel/"+f.gamma, f.branch), "", http.StatusOK)
		var ids []string
		for _, e := range jsonArray(t, report, "entries") {
			ids = append(ids, e.(map[string]any)["id"].(string))
		}
		if !slices.Equal(ids, []string{f.gamma, f.father, f.mother}) {
			t.Errorf("ahnentafel entries on branch = %v, want [gamma father mother]", ids)
		}
		text := do(t, server, http.MethodGet, withScope("/api/v1/ahnentafel/"+f.gamma+"?format=text", f.branch), "")
		if text.Code != http.StatusOK || !strings.Contains(text.Body.String(), "Gwendolyn") {
			t.Errorf("text ahnentafel on branch: %d %s", text.Code, text.Body.String())
		}
		mustDo(t, server, http.MethodGet, "/api/v1/ahnentafel/"+f.gamma, "", http.StatusNotFound)
		mustDo(t, server, http.MethodGet, withScope("/api/v1/ahnentafel/"+f.alpha, f.branch), "", http.StatusNotFound)
		mustDo(t, server, http.MethodGet, "/api/v1/ahnentafel/"+f.alpha, "", http.StatusOK)
	})

	t.Run("getDescendancy", func(t *testing.T) {
		descendants := func(branchID string) []string {
			resp := mustDo(t, server, http.MethodGet, withScope("/api/v1/descendancy/"+f.father, branchID), "", http.StatusOK)
			root, _ := resp["root"].(map[string]any)
			var ids []string
			for _, c := range optionalArray(t, root, "children") {
				ids = append(ids, c.(map[string]any)["id"].(string))
			}
			return ids
		}
		if got := descendants(f.branch); !slices.Equal(got, []string{f.gamma}) {
			t.Errorf("descendants on branch = %v, want only %s", got, f.gamma)
		}
		if got := descendants(""); !slices.Equal(got, []string{f.alpha}) {
			t.Errorf("descendants on main = %v, want only %s", got, f.alpha)
		}
		mustDo(t, server, http.MethodGet, "/api/v1/descendancy/"+f.gamma, "", http.StatusNotFound)
		mustDo(t, server, http.MethodGet, withScope("/api/v1/descendancy/"+f.alpha, f.branch), "", http.StatusNotFound)
	})

	t.Run("getRelationship", func(t *testing.T) {
		rel := mustDo(t, server, http.MethodGet, withScope("/api/v1/relationship/"+f.gamma+"/"+f.father, f.branch), "", http.StatusOK)
		if rel["isRelated"] != true || rel["summary"] != "parent" {
			t.Errorf("gamma -> father on branch = %v / %v, want related parent", rel["isRelated"], rel["summary"])
		}
		mustDo(t, server, http.MethodGet, "/api/v1/relationship/"+f.gamma+"/"+f.father, "", http.StatusNotFound)
		mustDo(t, server, http.MethodGet, withScope("/api/v1/relationship/"+f.alpha+"/"+f.father, f.branch), "", http.StatusNotFound)
		rel = mustDo(t, server, http.MethodGet, "/api/v1/relationship/"+f.alpha+"/"+f.father, "", http.StatusOK)
		if rel["summary"] != "parent" {
			t.Errorf("alpha -> father on main = %v, want parent", rel["summary"])
		}
	})
}

// withScope is scoped for a path that may already carry a query string.
func withScope(path, branchID string) string {
	if branchID == "" {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "branch=" + branchID
}

// itemIDs lists the ids of a list response's `items`.
func itemIDs(t *testing.T, resp map[string]any) []string {
	t.Helper()
	var ids []string
	for _, item := range jsonArray(t, resp, "items") {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}
	return ids
}

// childIDs lists the ids of a group sheet's children.
func childIDs(t *testing.T, sheet map[string]any) []string {
	t.Helper()
	var ids []string
	for _, c := range optionalArray(t, sheet, "children") {
		ids = append(ids, c.(map[string]any)["id"].(string))
	}
	return ids
}
