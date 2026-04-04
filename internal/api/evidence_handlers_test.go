package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Evidence Analysis tests
// ============================================================================

func TestCreateEvidenceAnalysis(t *testing.T) {
	server := setupTestServer()

	body := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born in 1800","research_status":"probable"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["fact_type"] != "person_birth" {
		t.Errorf("fact_type = %v, want person_birth", resp["fact_type"])
	}
	if resp["conclusion"] != "Born in 1800" {
		t.Errorf("conclusion = %v, want Born in 1800", resp["conclusion"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

func TestCreateEvidenceAnalysis_MissingConclusion(t *testing.T) {
	server := setupTestServer()

	body := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestGetEvidenceAnalysis(t *testing.T) {
	server := setupTestServer()

	// Create an analysis
	createBody := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born in 1800"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Get the analysis
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evidence-analyses/%s", id), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["id"] != id {
		t.Errorf("id = %v, want %s", resp["id"], id)
	}
}

func TestGetEvidenceAnalysis_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-analyses/00000000-0000-0000-0000-000000000099", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListEvidenceAnalyses(t *testing.T) {
	server := setupTestServer()

	// Create two analyses
	for i := range 2 {
		body := fmt.Sprintf(`{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Conclusion %d"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-analyses", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	total := int(resp["total"].(float64))
	if total < 2 {
		t.Errorf("total = %d, want >= 2", total)
	}
}

func TestUpdateEvidenceAnalysis(t *testing.T) {
	server := setupTestServer()

	// Create
	createBody := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born in 1800"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Update
	updateBody := `{"conclusion":"Born in 1801","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/evidence-analyses/%s", id), strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(updateRec.Body.Bytes(), &resp)
	if resp["conclusion"] != "Born in 1801" {
		t.Errorf("conclusion = %v, want Born in 1801", resp["conclusion"])
	}
}

func TestDeleteEvidenceAnalysis(t *testing.T) {
	server := setupTestServer()

	// Create
	createBody := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born in 1800"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Delete
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/evidence-analyses/%s?version=1", id), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Verify it's gone
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evidence-analyses/%s", id), http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("Status after delete = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestGetAnalysesByFact(t *testing.T) {
	server := setupTestServer()

	// Create an analysis
	body := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born in 1800"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	// Get by fact
	req := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-analyses/by-fact?factType=person_birth&subjectId=00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp) < 1 {
		t.Errorf("Expected at least 1 analysis, got %d", len(resp))
	}
}

// ============================================================================
// Evidence Conflict tests
// ============================================================================

func TestListEvidenceConflicts(t *testing.T) {
	server := setupTestServer()

	// Create two analyses with conflicting conclusions to trigger auto-detection
	body1 := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000002","conclusion":"Born in 1800"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec1, req1)

	body2 := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000002","conclusion":"Born in 1801"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec2, req2)

	// Check that a conflict was auto-detected
	var created2 map[string]any
	json.Unmarshal(rec2.Body.Bytes(), &created2)

	// List conflicts
	req := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-conflicts", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	total := int(resp["total"].(float64))
	if total < 1 {
		t.Errorf("total = %d, want >= 1 (expected auto-detected conflict)", total)
	}
}

func TestResolveEvidenceConflict(t *testing.T) {
	server := setupTestServer()

	// Create conflicting analyses
	body1 := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000003","conclusion":"Born in 1800"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec1, req1)

	body2 := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000003","conclusion":"Born in 1801"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-analyses", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec2, req2)

	var created2 map[string]any
	json.Unmarshal(rec2.Body.Bytes(), &created2)
	conflictID, ok := created2["conflict_id"].(string)
	if !ok || conflictID == "" {
		t.Fatal("Expected conflict auto-detection to produce a conflict_id, but none was returned")
	}

	// Resolve the conflict
	resolveBody := `{"resolution":"Born in 1800 is correct per census records","version":1}`
	resolveReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/evidence-conflicts/%s/resolve", conflictID), strings.NewReader(resolveBody))
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(resolveRec, resolveReq)

	if resolveRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", resolveRec.Code, http.StatusOK, resolveRec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(resolveRec.Body.Bytes(), &resp)
	if resp["status"] != "resolved" {
		t.Errorf("status = %v, want resolved", resp["status"])
	}
}

func TestGetConflictsBySubject(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-conflicts/by-subject/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ============================================================================
// Research Log tests
// ============================================================================

func TestCreateResearchLog(t *testing.T) {
	server := setupTestServer()

	searchDate := time.Now().UTC().Format(time.RFC3339)
	body := `{"subject_id":"00000000-0000-0000-0000-000000000001","subject_type":"person","repository":"FamilySearch","search_description":"Searched 1850 census","outcome":"found","search_date":"` + searchDate + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/research-logs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["repository"] != "FamilySearch" {
		t.Errorf("repository = %v, want FamilySearch", resp["repository"])
	}
	if resp["outcome"] != "found" {
		t.Errorf("outcome = %v, want found", resp["outcome"])
	}
}

func TestListResearchLogs(t *testing.T) {
	server := setupTestServer()

	// Create a log entry
	searchDate := time.Now().UTC().Format(time.RFC3339)
	body := `{"subject_id":"00000000-0000-0000-0000-000000000001","subject_type":"person","repository":"Ancestry","search_description":"Census search","outcome":"not_found","search_date":"` + searchDate + `"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/research-logs", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research-logs", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetResearchLog(t *testing.T) {
	server := setupTestServer()

	searchDate := time.Now().UTC().Format(time.RFC3339)
	body := `{"subject_id":"00000000-0000-0000-0000-000000000001","subject_type":"person","repository":"NARA","search_description":"Military records","outcome":"inconclusive","search_date":"` + searchDate + `"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/research-logs", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Get
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/research-logs/%s", id), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestUpdateResearchLog(t *testing.T) {
	server := setupTestServer()

	searchDate := time.Now().UTC().Format(time.RFC3339)
	body := `{"subject_id":"00000000-0000-0000-0000-000000000001","subject_type":"person","repository":"NARA","search_description":"Military records","outcome":"inconclusive","search_date":"` + searchDate + `"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/research-logs", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Update
	updateBody := `{"outcome":"found","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/research-logs/%s", id), strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
}

func TestDeleteResearchLog(t *testing.T) {
	server := setupTestServer()

	searchDate := time.Now().UTC().Format(time.RFC3339)
	body := `{"subject_id":"00000000-0000-0000-0000-000000000001","subject_type":"person","repository":"NARA","search_description":"Military records","outcome":"found","search_date":"` + searchDate + `"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/research-logs", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Delete
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/research-logs/%s?version=1", id), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestGetResearchLogsBySubject(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/research-logs/by-subject/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ============================================================================
// Proof Summary tests
// ============================================================================

func TestCreateProofSummary(t *testing.T) {
	server := setupTestServer()

	body := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born 1800 in Virginia","argument":"Census records and church records agree on 1800 birth year."}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/proof-summaries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["conclusion"] != "Born 1800 in Virginia" {
		t.Errorf("conclusion = %v, want Born 1800 in Virginia", resp["conclusion"])
	}
}

func TestGetProofSummary(t *testing.T) {
	server := setupTestServer()

	// Create
	body := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born 1800","argument":"Census evidence"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/proof-summaries", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Get
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/proof-summaries/%s", id), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestListProofSummaries(t *testing.T) {
	server := setupTestServer()

	// Create
	body := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born 1800","argument":"Evidence supports"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/proof-summaries", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proof-summaries", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestUpdateProofSummary(t *testing.T) {
	server := setupTestServer()

	// Create
	body := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born 1800","argument":"Census evidence"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/proof-summaries", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Update
	updateBody := `{"conclusion":"Born 1801","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/proof-summaries/%s", id), strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
}

func TestDeleteProofSummary(t *testing.T) {
	server := setupTestServer()

	// Create
	body := `{"fact_type":"person_birth","subject_id":"00000000-0000-0000-0000-000000000001","conclusion":"Born 1800","argument":"Census evidence"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/proof-summaries", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	id := created["id"].(string)

	// Delete
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/proof-summaries/%s?version=1", id), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestGetProofSummaryByFact(t *testing.T) {
	server := setupTestServer()

	// Create
	body := `{"fact_type":"person_death","subject_id":"00000000-0000-0000-0000-000000000005","conclusion":"Died 1860","argument":"Death certificate"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/proof-summaries", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	// Get by fact
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proof-summaries/by-fact?factType=person_death&subjectId=00000000-0000-0000-0000-000000000005", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp) < 1 {
		t.Errorf("Expected at least 1 proof summary, got %d", len(resp))
	}
}
