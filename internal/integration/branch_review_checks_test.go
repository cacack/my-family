package integration_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestBranchReviewChecks_EndToEnd is #838's acceptance scenario against every
// backend: a branch that introduces a validation warning, a duplicate person and
// an undocumented relationship shows them in its health and evidence-coverage
// checks — and only what it introduced, not the mainline's standing issues —
// while the mainline's own quality endpoints are unchanged.
func TestBranchReviewChecks_EndToEnd(t *testing.T) {
	forEachBackend(t, runBranchReviewChecks)
}

func healthPath(branchID string) string {
	return "/api/v1/branches/" + branchID + "/health"
}

func coveragePath(branchID string) string {
	return "/api/v1/branches/" + branchID + "/evidence-coverage"
}

func runBranchReviewChecks(t *testing.T, server *api.Server) {
	t.Helper()

	// The mainline: two people and their family. Ada has a birth year, so a
	// death 140 years later is an impossible age; Bob has no dates at all, a
	// standing mainline issue the branch must not be charged with.
	ada := createPerson(t, server, "Ada", "Sample")
	adaPath := "/api/v1/persons/" + ada
	mustDo(t, server, http.MethodPut, adaPath,
		fmt.Sprintf(`{"birth_date":"1 JAN 1850","version":%d}`, entityVersion(t, server, adaPath, "")), http.StatusOK)
	bob := createPerson(t, server, "Bob", "Sample")
	family := createFamily(t, server, ada, bob)
	mainValidation := mustDo(t, server, http.MethodGet, "/api/v1/quality/validation", "", http.StatusOK)

	branchID := createBranch(t, server, "review-checks")

	// A clean branch introduces nothing and changed nothing.
	health := mustDo(t, server, http.MethodGet, healthPath(branchID), "", http.StatusOK)
	for _, list := range []string{"validation_issues", "quality_issues", "duplicates"} {
		if got := jsonArray(t, health, list); len(got) != 0 {
			t.Errorf("clean branch %s = %v, want []", list, got)
		}
	}
	coverage := mustDo(t, server, http.MethodGet, coveragePath(branchID), "", http.StatusOK)
	if coverage["changed_fact_count"] != float64(0) || len(jsonArray(t, coverage, "uncovered")) != 0 {
		t.Errorf("clean branch coverage = %v", coverage)
	}

	// On the branch: Ada dies at 140, a second Ada Sample born
	// the same day appears, and a child joins the family.
	mustDo(t, server, http.MethodPut, scoped(adaPath, branchID),
		fmt.Sprintf(`{"death_date":"1 JAN 1990","version":%d}`, entityVersion(t, server, adaPath, branchID)), http.StatusOK)
	twin := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/persons", branchID),
		`{"given_name":"Ada","surname":"Sample","gender":"female","birth_date":"1 JAN 1850"}`, http.StatusCreated), "id")
	child := createPerson(t, server, "Cal", "Sample")
	mustDo(t, server, http.MethodPost, scoped("/api/v1/families/"+family+"/children", branchID),
		fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)
	// The twin's birth is documented on the branch; nothing else is.
	mustDo(t, server, http.MethodPost, scoped("/api/v1/evidence-analyses", branchID),
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1850"}`, twin), http.StatusCreated)

	// --- Branch health: the introduced warning and duplicate, nothing of main's. ---
	health = mustDo(t, server, http.MethodGet, healthPath(branchID), "", http.StatusOK)
	issues := jsonArray(t, health, "validation_issues")
	var impossibleAge map[string]any
	for _, raw := range issues {
		issue, _ := raw.(map[string]any)
		if issue["code"] == "IMPOSSIBLE_AGE" {
			impossibleAge = issue
		}
		if issue["record_id"] == bob {
			t.Errorf("branch health charges the branch with Bob's mainline issue %v", issue)
		}
	}
	if impossibleAge == nil {
		t.Fatalf("validation_issues = %v, want IMPOSSIBLE_AGE", issues)
	}
	if impossibleAge["severity"] != "warning" || impossibleAge["record_id"] != ada ||
		impossibleAge["record_type"] != "person" || impossibleAge["record_name"] != "Ada Sample" ||
		impossibleAge["key"] == "" {
		t.Errorf("IMPOSSIBLE_AGE = %v", impossibleAge)
	}
	if health["warning_count"].(float64) < 1 {
		t.Errorf("warning_count = %v, want the introduced warning counted", health["warning_count"])
	}
	duplicates := jsonArray(t, health, "duplicates")
	if len(duplicates) != 1 {
		t.Fatalf("duplicates = %v, want the one pair the branch introduced", duplicates)
	}
	pair, _ := duplicates[0].(map[string]any)
	if got := fmt.Sprint(pair["person1_id"], pair["person2_id"]); got != fmt.Sprint(ada, twin) && got != fmt.Sprint(twin, ada) {
		t.Errorf("duplicate pair = %v, want Ada and her twin", pair)
	}
	var twinOrphan bool
	for _, raw := range jsonArray(t, health, "quality_issues") {
		issue, _ := raw.(map[string]any)
		if issue["person_id"] == twin && issue["issue"] == "No family connections" {
			twinOrphan = true
		}
		if issue["person_id"] == bob {
			t.Errorf("quality_issues charges the branch with Bob's mainline issue %v", issue)
		}
	}
	if !twinOrphan {
		t.Errorf("quality_issues = %v, want the twin's missing family connection", health["quality_issues"])
	}

	// --- Evidence coverage: every change but the documented birth. ---
	coverage = mustDo(t, server, http.MethodGet, coveragePath(branchID), "", http.StatusOK)
	type fact struct{ kind, factType, subject string }
	got := map[fact]bool{}
	for _, raw := range jsonArray(t, coverage, "uncovered") {
		entry, _ := raw.(map[string]any)
		factType, _ := entry["fact_type"].(string)
		subject, _ := entry["subject_id"].(string)
		kind, _ := entry["kind"].(string)
		got[fact{kind, factType, subject}] = true
		if entry["subject_name"] == "" {
			t.Errorf("uncovered fact %v is not named", entry)
		}
	}
	want := map[fact]bool{
		{"fact", "person_death", ada}:   true,
		{"fact", "person_name", twin}:   true,
		{"fact", "person_gender", twin}: true,
		{"relationship", "", family}:    true,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("uncovered = %v, want %v", got, want)
	}
	if coverage["changed_fact_count"] != float64(len(want)+1) {
		t.Errorf("changed_fact_count = %v, want %d (the documented birth too)", coverage["changed_fact_count"], len(want)+1)
	}

	// --- The mainline's own checks are untouched by the branch. ---
	if after := mustDo(t, server, http.MethodGet, "/api/v1/quality/validation", "", http.StatusOK); fmt.Sprint(after) != fmt.Sprint(mainValidation) {
		t.Errorf("mainline validation changed with a branch edit:\nbefore %v\nafter  %v", mainValidation, after)
	}

	// --- Unknown and terminal branches. ---
	unknown := "/api/v1/branches/00000000-0000-4000-8000-000000000000"
	mustDo(t, server, http.MethodGet, unknown+"/health", "", http.StatusNotFound)
	mustDo(t, server, http.MethodGet, unknown+"/evidence-coverage", "", http.StatusNotFound)
	mustDo(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, "", http.StatusNoContent)
	mustDo(t, server, http.MethodGet, healthPath(branchID), "", http.StatusNotFound)
	// The log keeps an archived branch's changes, so its coverage still answers.
	mustDo(t, server, http.MethodGet, coveragePath(branchID), "", http.StatusOK)
}

// TestBranchReviewChecks_DeletingMainlineRecords: a branch that deletes a
// mainline family (the partners were never married) or person reports each
// deletion as an undocumented change, named as the mainline has it, until an
// analysis on the branch documents it.
func TestBranchReviewChecks_DeletingMainlineRecords(t *testing.T) {
	forEachBackend(t, runBranchReviewDeletions)
}

func runBranchReviewDeletions(t *testing.T, server *api.Server) {
	t.Helper()
	ada := createPerson(t, server, "Ada", "Sample")
	bob := createPerson(t, server, "Bob", "Sample")
	dee := createPerson(t, server, "Dee", "Sample")
	family := createFamily(t, server, ada, bob)

	branchID := createBranch(t, server, "never-married")
	mustDo(t, server, http.MethodDelete, scoped("/api/v1/families/"+family, branchID), "", http.StatusNoContent)
	mustDo(t, server, http.MethodDelete, scoped("/api/v1/persons/"+dee, branchID), "", http.StatusNoContent)

	coverage := mustDo(t, server, http.MethodGet, coveragePath(branchID), "", http.StatusOK)
	got := map[string]string{}
	for _, raw := range jsonArray(t, coverage, "uncovered") {
		entry, _ := raw.(map[string]any)
		if entry["kind"] != "deletion" {
			t.Errorf("uncovered entry %v, want a deletion", entry)
		}
		subject, _ := entry["subject_id"].(string)
		name, _ := entry["subject_name"].(string)
		got[subject] = name
	}
	if len(got) != 2 || got[family] == "" || got[dee] != "Dee Sample" {
		t.Errorf("uncovered = %v, want the named family %s and Dee %s", got, family, dee)
	}
	if coverage["changed_fact_count"] != float64(2) {
		t.Errorf("changed_fact_count = %v, want 2", coverage["changed_fact_count"])
	}

	// An analysis about the family on the branch documents its deletion.
	mustDo(t, server, http.MethodPost, scoped("/api/v1/evidence-analyses", branchID),
		fmt.Sprintf(`{"fact_type":"family_marriage","subject_id":%q,"conclusion":"Never married"}`, family), http.StatusCreated)
	coverage = mustDo(t, server, http.MethodGet, coveragePath(branchID), "", http.StatusOK)
	uncovered := jsonArray(t, coverage, "uncovered")
	if len(uncovered) != 1 {
		t.Fatalf("uncovered = %v, want only Dee's deletion", uncovered)
	}
	if entry, _ := uncovered[0].(map[string]any); entry["subject_id"] != dee {
		t.Errorf("uncovered = %v, want Dee's deletion", entry)
	}
}
