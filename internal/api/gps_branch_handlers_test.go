package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// ============================================================================
// Branch scope on the GPS artifacts (sub-issue E of #676, issue #760)
// ============================================================================

// gpsClient bundles the request helpers the GPS branch tests share.
type gpsClient struct {
	t      *testing.T
	server *api.Server
}

func (c gpsClient) status(method, path, body string) int {
	c.t.Helper()
	return do(c.t, c.server, method, path, body).Code
}

func (c gpsClient) mustJSON(method, path, body string, want int) map[string]any {
	c.t.Helper()
	rec := do(c.t, c.server, method, path, body)
	if rec.Code != want {
		c.t.Fatalf("%s %s: status = %d, want %d. Body: %s", method, path, rec.Code, want, rec.Body.String())
	}
	return decodeJSON(c.t, rec)
}

func (c gpsClient) total(path string) int {
	c.t.Helper()
	n, _ := c.mustJSON(http.MethodGet, path, "", http.StatusOK)["total"].(float64)
	return int(n)
}

func (c gpsClient) count(path string) int {
	c.t.Helper()
	rec := do(c.t, c.server, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		c.t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		c.t.Fatalf("GET %s: parse %q: %v", path, rec.Body.String(), err)
	}
	return len(out)
}

func version(m map[string]any) int64 {
	v, _ := m["version"].(float64)
	return int64(v)
}

// TestEvidenceAnalysesAndConflicts_BranchScope walks the evidence-analysis and
// evidence-conflict surface on a branch: a rival analysis written on the branch
// records its evidence conflict on the branch only, and a branch resolution of
// main's conflict leaves main's conflict open (the overlay is resolved before
// the status is matched).
func TestEvidenceAnalysesAndConflicts_BranchScope(t *testing.T) {
	server := setupBranchTestServer()
	c := gpsClient{t: t, server: server}
	personID := createPerson(t, server, "Ada", "Lovelace")
	byFact := fmt.Sprintf("/api/v1/evidence-analyses/by-fact?factType=person_birth&subjectId=%s", personID)

	// Two disagreeing analyses on main record one open evidence conflict.
	c.mustJSON(http.MethodPost, "/api/v1/evidence-analyses",
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1815"}`, personID), http.StatusCreated)
	rival := c.mustJSON(http.MethodPost, "/api/v1/evidence-analyses",
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1816"}`, personID), http.StatusCreated)
	conflictID, _ := rival["conflict_id"].(string)
	if conflictID == "" {
		t.Fatal("rival analysis on main recorded no evidence conflict")
	}

	branchID := createBranch(t, server, "Birth year")
	onBranch := "?branch=" + branchID

	// A third reading on the branch: main never sees it.
	third := c.mustJSON(http.MethodPost, "/api/v1/evidence-analyses"+onBranch,
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1817"}`, personID), http.StatusCreated)
	thirdID, _ := third["id"].(string)
	if got := c.status(http.MethodGet, "/api/v1/evidence-analyses/"+thirdID, ""); got != http.StatusNotFound {
		t.Errorf("Mainline GET of a branch analysis: status = %d, want 404", got)
	}
	if got := c.total("/api/v1/evidence-analyses"); got != 2 {
		t.Errorf("Mainline analysis total = %d, want 2", got)
	}
	if got := c.total("/api/v1/evidence-analyses" + onBranch); got != 3 {
		t.Errorf("Branch analysis total = %d, want 3", got)
	}
	if got := c.count(byFact); got != 2 {
		t.Errorf("Mainline analyses by fact = %d, want 2", got)
	}
	if got := c.count(byFact + "&branch=" + branchID); got != 3 {
		t.Errorf("Branch analyses by fact = %d, want 3", got)
	}

	// Edit and delete it on the branch.
	edited := c.mustJSON(http.MethodPut, "/api/v1/evidence-analyses/"+thirdID+onBranch,
		fmt.Sprintf(`{"notes":"branch reading","version":%d}`, version(third)), http.StatusOK)
	if edited["notes"] != "branch reading" {
		t.Errorf("Branch analysis notes = %v, want the edit", edited["notes"])
	}
	if got := c.status(http.MethodDelete, fmt.Sprintf("/api/v1/evidence-analyses/%s?branch=%s&version=%d", thirdID, branchID, version(edited)), ""); got != http.StatusNoContent {
		t.Fatalf("Delete analysis on branch: status = %d, want 204", got)
	}
	if got := c.status(http.MethodGet, "/api/v1/evidence-analyses/"+thirdID+onBranch, ""); got != http.StatusNotFound {
		t.Errorf("Branch GET of a deleted analysis: status = %d, want 404", got)
	}

	// Resolve main's evidence conflict on the branch only.
	conflict := c.mustJSON(http.MethodGet, "/api/v1/evidence-conflicts/"+conflictID+onBranch, "", http.StatusOK)
	resolved := c.mustJSON(http.MethodPost, "/api/v1/evidence-conflicts/"+conflictID+"/resolve"+onBranch,
		fmt.Sprintf(`{"resolution":"The register wins","version":%d}`, version(conflict)), http.StatusOK)
	if resolved["status"] != "resolved" {
		t.Errorf("Branch conflict status = %v, want resolved", resolved["status"])
	}
	if got := c.mustJSON(http.MethodGet, "/api/v1/evidence-conflicts/"+conflictID, "", http.StatusOK)["status"]; got != "open" {
		t.Errorf("Mainline conflict status = %v, want open", got)
	}
	if got := c.total("/api/v1/evidence-conflicts?status=open"); got != 1 {
		t.Errorf("Mainline open conflicts = %d, want 1", got)
	}
	if got := c.total("/api/v1/evidence-conflicts?status=open&branch=" + branchID); got != 0 {
		t.Errorf("Branch open conflicts = %d, want 0 (the branch resolved it)", got)
	}
	if got := c.total("/api/v1/evidence-conflicts?status=resolved&branch=" + branchID); got != 1 {
		t.Errorf("Branch resolved conflicts = %d, want 1", got)
	}
	if got := c.count("/api/v1/evidence-conflicts/by-subject/" + personID + onBranch); got != 1 {
		t.Errorf("Branch conflicts by subject = %d, want 1", got)
	}
}

// TestResearchLogsAndProofSummaries_BranchScope walks the research-log and
// proof-summary surface on a branch.
func TestResearchLogsAndProofSummaries_BranchScope(t *testing.T) {
	server := setupBranchTestServer()
	c := gpsClient{t: t, server: server}
	personID := createPerson(t, server, "Ada", "Lovelace")

	mainLog := c.mustJSON(http.MethodPost, "/api/v1/research-logs",
		fmt.Sprintf(`{"subject_id":%q,"subject_type":"person","repository":"County Archive","search_description":"Baptisms","outcome":"not_found","search_date":"2024-03-01T00:00:00Z"}`, personID),
		http.StatusCreated)
	mainLogID, _ := mainLog["id"].(string)
	mainProof := c.mustJSON(http.MethodPost, "/api/v1/proof-summaries",
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1815","argument":"The census and the register agree"}`, personID),
		http.StatusCreated)
	mainProofID, _ := mainProof["id"].(string)

	branchID := createBranch(t, server, "Negative searches")
	onBranch := "?branch=" + branchID

	// Research logs: a branch-only entry, and an edit of main's entry that moves
	// its search date (a change the projection used to drop).
	c.mustJSON(http.MethodPost, "/api/v1/research-logs"+onBranch,
		fmt.Sprintf(`{"subject_id":%q,"subject_type":"person","repository":"Parish Chest","search_description":"Burials","outcome":"found","search_date":"2024-04-01T00:00:00Z"}`, personID),
		http.StatusCreated)
	edited := c.mustJSON(http.MethodPut, "/api/v1/research-logs/"+mainLogID+onBranch,
		fmt.Sprintf(`{"outcome":"found","search_date":"2024-05-02T00:00:00Z","version":%d}`, version(mainLog)), http.StatusOK)
	if edited["outcome"] != "found" || edited["search_date"] != "2024-05-02T00:00:00Z" {
		t.Errorf("Branch log after edit = %v / %v, want found on 2024-05-02", edited["outcome"], edited["search_date"])
	}
	if got := c.mustJSON(http.MethodGet, "/api/v1/research-logs/"+mainLogID, "", http.StatusOK)["outcome"]; got != "not_found" {
		t.Errorf("Mainline log outcome = %v, want not_found", got)
	}
	if got := c.total("/api/v1/research-logs"); got != 1 {
		t.Errorf("Mainline research log total = %d, want 1", got)
	}
	if got := c.total("/api/v1/research-logs" + onBranch); got != 2 {
		t.Errorf("Branch research log total = %d, want 2", got)
	}
	if got := c.count("/api/v1/research-logs/by-subject/" + personID + onBranch); got != 2 {
		t.Errorf("Branch research logs by subject = %d, want 2", got)
	}
	if got := c.status(http.MethodDelete, fmt.Sprintf("/api/v1/research-logs/%s?branch=%s&version=%d", mainLogID, branchID, version(edited)), ""); got != http.StatusNoContent {
		t.Fatalf("Delete research log on branch: status = %d, want 204", got)
	}
	if got := c.status(http.MethodGet, "/api/v1/research-logs/"+mainLogID+onBranch, ""); got != http.StatusNotFound {
		t.Errorf("Branch GET of a deleted log: status = %d, want 404", got)
	}
	if got := c.count("/api/v1/research-logs/by-subject/" + personID); got != 1 {
		t.Errorf("Mainline research logs by subject = %d, want 1", got)
	}

	// Proof summaries.
	branchProof := c.mustJSON(http.MethodPost, "/api/v1/proof-summaries"+onBranch,
		fmt.Sprintf(`{"fact_type":"person_death","subject_id":%q,"conclusion":"Died 1852","argument":"The burial register"}`, personID),
		http.StatusCreated)
	branchProofID, _ := branchProof["id"].(string)
	if got := c.status(http.MethodGet, "/api/v1/proof-summaries/"+branchProofID, ""); got != http.StatusNotFound {
		t.Errorf("Mainline GET of a branch proof: status = %d, want 404", got)
	}
	updated := c.mustJSON(http.MethodPut, "/api/v1/proof-summaries/"+mainProofID+onBranch,
		fmt.Sprintf(`{"argument":"Revised on the branch","version":%d}`, version(mainProof)), http.StatusOK)
	if got := c.mustJSON(http.MethodGet, "/api/v1/proof-summaries/"+mainProofID, "", http.StatusOK)["argument"]; got != "The census and the register agree" {
		t.Errorf("Mainline proof argument = %v, want the original", got)
	}
	byFact := fmt.Sprintf("/api/v1/proof-summaries/by-fact?factType=person_death&subjectId=%s", personID)
	if got := c.count(byFact); got != 0 {
		t.Errorf("Mainline proofs by fact = %d, want 0", got)
	}
	if got := c.count(byFact + "&branch=" + branchID); got != 1 {
		t.Errorf("Branch proofs by fact = %d, want 1", got)
	}
	if got := c.total("/api/v1/proof-summaries" + onBranch); got != 2 {
		t.Errorf("Branch proof total = %d, want 2", got)
	}
	if got := c.status(http.MethodDelete, fmt.Sprintf("/api/v1/proof-summaries/%s?branch=%s&version=%d", mainProofID, branchID, version(updated)), ""); got != http.StatusNoContent {
		t.Fatalf("Delete proof on branch: status = %d, want 204", got)
	}
	if got := c.total("/api/v1/proof-summaries"); got != 1 {
		t.Errorf("Mainline proof total after the branch delete = %d, want 1", got)
	}
}

// TestGPSBranchScope_UnknownBranch pins the shared 404 for a branch id that was
// never created, on every operation #760 scoped.
func TestGPSBranchScope_UnknownBranch(t *testing.T) {
	server := setupBranchTestServer()
	scope := "?branch=" + unknownUUID
	subject := unknownUUID

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"listEvidenceAnalyses", http.MethodGet, "/api/v1/evidence-analyses" + scope, ""},
		{"createEvidenceAnalysis", http.MethodPost, "/api/v1/evidence-analyses" + scope,
			fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"x"}`, subject)},
		{"getEvidenceAnalysis", http.MethodGet, "/api/v1/evidence-analyses/" + unknownUUID + scope, ""},
		{"updateEvidenceAnalysis", http.MethodPut, "/api/v1/evidence-analyses/" + unknownUUID + scope, `{"notes":"x","version":1}`},
		{"deleteEvidenceAnalysis", http.MethodDelete, "/api/v1/evidence-analyses/" + unknownUUID + scope + "&version=1", ""},
		{"getAnalysesByFact", http.MethodGet, "/api/v1/evidence-analyses/by-fact?factType=person_birth&subjectId=" + subject + "&branch=" + unknownUUID, ""},
		{"listEvidenceConflicts", http.MethodGet, "/api/v1/evidence-conflicts" + scope, ""},
		{"getEvidenceConflict", http.MethodGet, "/api/v1/evidence-conflicts/" + unknownUUID + scope, ""},
		{"resolveEvidenceConflict", http.MethodPost, "/api/v1/evidence-conflicts/" + unknownUUID + "/resolve" + scope, `{"resolution":"x","version":1}`},
		{"getConflictsBySubject", http.MethodGet, "/api/v1/evidence-conflicts/by-subject/" + subject + scope, ""},
		{"listResearchLogs", http.MethodGet, "/api/v1/research-logs" + scope, ""},
		{"createResearchLog", http.MethodPost, "/api/v1/research-logs" + scope,
			fmt.Sprintf(`{"subject_id":%q,"subject_type":"person","repository":"x","search_description":"x","outcome":"found","search_date":"2024-01-01T00:00:00Z"}`, subject)},
		{"getResearchLog", http.MethodGet, "/api/v1/research-logs/" + unknownUUID + scope, ""},
		{"updateResearchLog", http.MethodPut, "/api/v1/research-logs/" + unknownUUID + scope, `{"notes":"x","version":1}`},
		{"deleteResearchLog", http.MethodDelete, "/api/v1/research-logs/" + unknownUUID + scope + "&version=1", ""},
		{"getResearchLogsBySubject", http.MethodGet, "/api/v1/research-logs/by-subject/" + subject + scope, ""},
		{"listProofSummaries", http.MethodGet, "/api/v1/proof-summaries" + scope, ""},
		{"createProofSummary", http.MethodPost, "/api/v1/proof-summaries" + scope,
			fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"x","argument":"x"}`, subject)},
		{"getProofSummary", http.MethodGet, "/api/v1/proof-summaries/" + unknownUUID + scope, ""},
		{"updateProofSummary", http.MethodPut, "/api/v1/proof-summaries/" + unknownUUID + scope, `{"argument":"x","version":1}`},
		{"deleteProofSummary", http.MethodDelete, "/api/v1/proof-summaries/" + unknownUUID + scope + "&version=1", ""},
		{"getProofSummaryByFact", http.MethodGet, "/api/v1/proof-summaries/by-fact?factType=person_birth&subjectId=" + subject + "&branch=" + unknownUUID, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, server, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("Status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestGPSBranchScope_ArchivedBranch pins the terminal-branch contract on the
// #760 operations: reads 404 (the overlay is purged), writes 409 (read-only).
func TestGPSBranchScope_ArchivedBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	branchID := createBranch(t, server, "Abandoned")
	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("Delete branch: status = %d, want 204", rec.Code)
	}
	scope := "?branch=" + branchID

	for _, path := range []string{
		"/api/v1/evidence-analyses" + scope,
		"/api/v1/evidence-conflicts" + scope,
		"/api/v1/research-logs" + scope,
		"/api/v1/proof-summaries" + scope,
	} {
		if rec := do(t, server, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404. Body: %s", path, rec.Code, rec.Body.String())
		}
	}
	for _, write := range []struct{ path, body string }{
		{"/api/v1/evidence-analyses" + scope, fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"x"}`, personID)},
		{"/api/v1/proof-summaries" + scope, fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"x","argument":"x"}`, personID)},
	} {
		if rec := do(t, server, http.MethodPost, write.path, write.body); rec.Code != http.StatusConflict {
			t.Errorf("POST %s on an archived branch: status = %d, want 409. Body: %s", write.path, rec.Code, rec.Body.String())
		}
	}
}
