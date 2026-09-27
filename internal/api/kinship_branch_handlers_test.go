package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
)

// The reads #829 scoped to a branch: search, the families list, the group
// sheet, the Ahnentafel, descendancy and the relationship calculator. The
// integration package runs the same assertions on every backend; these pin the
// handler wiring, including the scope's own 400/404 answers.

// kinshipBranchFixture is a mainline couple with one child (Alpha), then a
// branch that adds a second child (Gamma) and a family of Gamma's, and deletes
// Alpha.
type kinshipBranchFixture struct {
	server                               *api.Server
	branch, father, mother, alpha, gamma string
	family, branchFamily                 string
}

func seedKinshipBranch(t *testing.T) kinshipBranchFixture {
	t.Helper()
	f := kinshipBranchFixture{server: setupBranchTestServer()}
	mustStatus := func(method, path, body string, want int) map[string]any {
		t.Helper()
		rec := do(t, f.server, method, path, body)
		if rec.Code != want {
			t.Fatalf("%s %s = %d, want %d. Body: %s", method, path, rec.Code, want, rec.Body.String())
		}
		if strings.TrimSpace(rec.Body.String()) == "" {
			return nil
		}
		return decodeJSON(t, rec)
	}
	person := func(branch, given, gender string) string {
		path := "/api/v1/persons"
		if branch != "" {
			path += "?branch=" + branch
		}
		id, _ := mustStatus(http.MethodPost, path,
			fmt.Sprintf(`{"given_name":%q,"surname":"Kinhandler","gender":%q}`, given, gender), http.StatusCreated)["id"].(string)
		return id
	}

	f.father = person("", "Fabian", "male")
	f.mother = person("", "Marisol", "female")
	f.alpha = person("", "Aldous", "male")
	f.family = createFamily(t, f.server, f.father, f.mother)
	mustStatus(http.MethodPost, "/api/v1/families/"+f.family+"/children",
		fmt.Sprintf(`{"person_id":%q}`, f.alpha), http.StatusCreated)

	f.branch = createBranch(t, f.server, "kinship handlers")
	scope := "?branch=" + f.branch
	f.gamma = person(f.branch, "Guinevere", "female")
	mustStatus(http.MethodPost, "/api/v1/families/"+f.family+"/children"+scope,
		fmt.Sprintf(`{"person_id":%q}`, f.gamma), http.StatusCreated)
	f.branchFamily, _ = mustStatus(http.MethodPost, "/api/v1/families"+scope,
		fmt.Sprintf(`{"partner1_id":%q,"relationship_type":"marriage"}`, f.gamma), http.StatusCreated)["id"].(string)
	mustStatus(http.MethodDelete, "/api/v1/families/"+f.family+"/children/"+f.alpha+scope, "", http.StatusNoContent)
	mustStatus(http.MethodDelete, "/api/v1/persons/"+f.alpha+scope, "", http.StatusNoContent)
	return f
}

// scopedPath appends the branch scope to a path that may carry a query string.
func scopedPath(path, branch string) string {
	if branch == "" {
		return path
	}
	if strings.Contains(path, "?") {
		return path + "&branch=" + branch
	}
	return path + "?branch=" + branch
}

func TestKinshipReads_BranchScope(t *testing.T) {
	f := seedKinshipBranch(t)

	tests := []struct {
		name string
		// path reads the branch-created entity (Gamma, or Gamma's family).
		created string
		// deleted reads the branch-deleted Alpha (or a family Gamma is not in).
		deleted string
		// contains is text the branch-scoped read of created must include.
		contains string
	}{
		{"searchPersons", "/api/v1/search?q=Guinevere", "", f.gamma},
		{"listFamilies", "/api/v1/families?limit=50", "", f.branchFamily},
		{"getFamilyGroupSheet", "/api/v1/families/" + f.branchFamily + "/group-sheet", "", "Guinevere"},
		{"getAhnentafel", "/api/v1/ahnentafel/" + f.gamma, "/api/v1/ahnentafel/" + f.alpha, f.mother},
		{"getAhnentafel text", "/api/v1/ahnentafel/" + f.gamma + "?format=text", "/api/v1/ahnentafel/" + f.alpha + "?format=text", "Guinevere"},
		{"getDescendancy", "/api/v1/descendancy/" + f.father, "/api/v1/descendancy/" + f.alpha, f.gamma},
		{"getRelationship", "/api/v1/relationship/" + f.gamma + "/" + f.father, "/api/v1/relationship/" + f.alpha + "/" + f.father, `"summary":"parent"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, f.server, http.MethodGet, scopedPath(tt.created, f.branch), "")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tt.contains) {
				t.Fatalf("on branch: %d, want 200 containing %q. Body: %s", rec.Code, tt.contains, rec.Body.String())
			}
			main := do(t, f.server, http.MethodGet, tt.created, "")
			if main.Code == http.StatusOK && strings.Contains(main.Body.String(), tt.contains) {
				t.Errorf("on main: the branch-created entity leaked. Body: %s", main.Body.String())
			}
			if tt.deleted != "" {
				if rec := do(t, f.server, http.MethodGet, scopedPath(tt.deleted, f.branch), ""); rec.Code != http.StatusNotFound {
					t.Errorf("branch-deleted read on branch = %d, want 404. Body: %s", rec.Code, rec.Body.String())
				}
				if rec := do(t, f.server, http.MethodGet, tt.deleted, ""); rec.Code != http.StatusOK {
					t.Errorf("branch-deleted read on main = %d, want 200. Body: %s", rec.Code, rec.Body.String())
				}
			}
		})
	}

	t.Run("branch-deleted person is not found by search or listed as a descendant", func(t *testing.T) {
		if body := do(t, f.server, http.MethodGet, scopedPath("/api/v1/search?q=Aldous", f.branch), "").Body.String(); strings.Contains(body, f.alpha) {
			t.Errorf("search on branch found the branch-deleted person: %s", body)
		}
		if body := do(t, f.server, http.MethodGet, "/api/v1/search?q=Aldous", "").Body.String(); !strings.Contains(body, f.alpha) {
			t.Errorf("search on main lost the person: %s", body)
		}
		if body := do(t, f.server, http.MethodGet, scopedPath("/api/v1/descendancy/"+f.father, f.branch), "").Body.String(); strings.Contains(body, f.alpha) {
			t.Errorf("descendancy on branch lists the branch-deleted child: %s", body)
		}
		if body := do(t, f.server, http.MethodGet, scopedPath("/api/v1/families/"+f.family+"/group-sheet", f.branch), "").Body.String(); strings.Contains(body, f.alpha) || !strings.Contains(body, f.gamma) {
			t.Errorf("group sheet on branch has the wrong children: %s", body)
		}
	})
}

func TestKinshipReads_BranchScopeErrors(t *testing.T) {
	f := seedKinshipBranch(t)
	paths := []string{
		"/api/v1/search?q=Guinevere",
		"/api/v1/families",
		"/api/v1/families/" + f.family + "/group-sheet",
		"/api/v1/ahnentafel/" + f.father,
		"/api/v1/descendancy/" + f.father,
		"/api/v1/relationship/" + f.father + "/" + f.mother,
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			if rec := do(t, f.server, http.MethodGet, scopedPath(path, "not-a-uuid"), ""); rec.Code != http.StatusBadRequest {
				t.Errorf("malformed branch = %d, want 400. Body: %s", rec.Code, rec.Body.String())
			}
			if rec := do(t, f.server, http.MethodGet, scopedPath(path, uuid.NewString()), ""); rec.Code != http.StatusNotFound {
				t.Errorf("unknown branch = %d, want 404. Body: %s", rec.Code, rec.Body.String())
			}
		})
	}

	// Parameter validation still answers first, whatever the scope.
	if rec := do(t, f.server, http.MethodGet, scopedPath("/api/v1/search", uuid.NewString()), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("search with no criterion on an unknown branch = %d, want 400", rec.Code)
	}
}
