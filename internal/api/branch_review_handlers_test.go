package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

// The merge review's soft checks (#838): the HTTP surface of the evidence
// coverage and branch health. The scenario itself runs on every backend in
// internal/integration (TestBranchReviewChecks_EndToEnd).

func TestBranchReviewChecks_NoBranchStore(t *testing.T) {
	server := setupTestServer() // no WithBranchStore
	for _, path := range []string{
		"/api/v1/branches/" + unknownUUID + "/health",
		"/api/v1/branches/" + unknownUUID + "/evidence-coverage",
	} {
		if rec := do(t, server, http.MethodGet, path, ""); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503. Body: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestBranchReviewChecks_UnknownBranch(t *testing.T) {
	server := setupBranchTestServer()
	for _, path := range []string{
		"/api/v1/branches/" + unknownUUID + "/health",
		"/api/v1/branches/" + unknownUUID + "/evidence-coverage",
	} {
		if rec := do(t, server, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404. Body: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestBranchReviewChecks_ReportTheBranchsChanges(t *testing.T) {
	server := setupBranchTestServer()
	branchID := createBranch(t, server, "review")

	// Two people alike enough to be duplicates, both new on the branch; the
	// first lives to 145, an impossible age.
	var ids []string
	for _, death := range []string{`,"death_date":"1 JAN 1995"`, ""} {
		rec := do(t, server, http.MethodPost, "/api/v1/persons?branch="+branchID,
			`{"given_name":"Ada","surname":"Sample","gender":"female","birth_date":"1 JAN 1850","birth_place":"Springfield"`+death+`}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create person = %d: %s", rec.Code, rec.Body.String())
		}
		ids = append(ids, decodeJSON(t, rec)["id"].(string))
	}

	rec := do(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("health = %d: %s", rec.Code, rec.Body.String())
	}
	health := decodeJSON(t, rec)
	duplicates, _ := health["duplicates"].([]any)
	if len(duplicates) != 1 {
		t.Fatalf("duplicates = %v, want the new pair", health["duplicates"])
	}
	pair, _ := duplicates[0].(map[string]any)
	if got := fmt.Sprint(pair["person1_id"], pair["person2_id"]); got != fmt.Sprint(ids[0], ids[1]) && got != fmt.Sprint(ids[1], ids[0]) {
		t.Errorf("pair = %v, want %v", pair, ids)
	}
	if reasons, ok := pair["match_reasons"].([]any); !ok || len(reasons) == 0 {
		t.Errorf("match_reasons = %v", pair["match_reasons"])
	}
	for _, list := range []string{"validation_issues", "quality_issues"} {
		if _, ok := health[list].([]any); !ok {
			t.Errorf("%s = %v, want an array", list, health[list])
		}
	}
	quality, _ := health["quality_issues"].([]any)
	if len(quality) == 0 {
		t.Errorf("quality_issues = %v, want the new people's missing family connections", quality)
	}
	var named bool
	for _, raw := range health["validation_issues"].([]any) {
		if issue, _ := raw.(map[string]any); issue["code"] == "IMPOSSIBLE_AGE" && issue["record_name"] == "Ada Sample" && issue["record_type"] == "person" {
			named = true
		}
	}
	if !named {
		t.Errorf("validation_issues = %v, want the impossible age by name", health["validation_issues"])
	}

	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/evidence-coverage", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("evidence coverage = %d: %s", rec.Code, rec.Body.String())
	}
	coverage := decodeJSON(t, rec)
	uncovered, _ := coverage["uncovered"].([]any)
	// Name, gender and birth for each of the two new people, and one death.
	if coverage["changed_fact_count"] != float64(7) || len(uncovered) != 7 {
		t.Fatalf("coverage = %v, want seven undocumented facts", coverage)
	}
	first, _ := uncovered[0].(map[string]any)
	if first["kind"] != "fact" || first["subject_type"] != "person" || first["subject_name"] != "Ada Sample" || first["fact_type"] == nil {
		t.Errorf("uncovered[0] = %v", first)
	}
}
