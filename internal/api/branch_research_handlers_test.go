package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
)

// createProofSummaryOn creates a proof summary for personID, on branchID when
// it is non-empty, and returns its id.
func createProofSummaryOn(t *testing.T, server *api.Server, branchID, personID, conclusion string) string {
	t.Helper()
	path := "/api/v1/proof-summaries"
	if branchID != "" {
		path += "?branch=" + branchID
	}
	rec := do(t, server, http.MethodPost, path, fmt.Sprintf(
		`{"fact_type":"person_birth","subject_id":%q,"conclusion":%q,"argument":"The register agrees"}`,
		personID, conclusion))
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateProofSummary status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	id, _ := decodeJSON(t, rec)["id"].(string)
	return id
}

func TestCreateBranch_WithResearchRecord(t *testing.T) {
	server := setupBranchTestServer()
	person := createPerson(t, server, "Ada", "Placeholder")
	other := createPerson(t, server, "Bo", "Placeholder")
	family := createFamily(t, server, person, other)
	proof := createProofSummaryOn(t, server, "", person, "Born 1815")

	rec := do(t, server, http.MethodPost, "/api/v1/branches", fmt.Sprintf(`{
		"name": "Ada's parents",
		"hypothesis": "Was Ada the daughter of Bo?",
		"subjects": [{"type":"person","id":%q},{"type":"family","id":%q}],
		"outcome": "inconclusive",
		"proof_summary_ids": [%q]
	}`, person, family, proof))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON(t, rec)
	if resp["hypothesis"] != "Was Ada the daughter of Bo?" || resp["outcome"] != "inconclusive" {
		t.Errorf("response = %v", resp)
	}
	subjects, _ := resp["subjects"].([]any)
	if len(subjects) != 2 {
		t.Fatalf("subjects = %v", resp["subjects"])
	}
	first, _ := subjects[0].(map[string]any)
	if first["type"] != "person" || first["id"] != person || first["name"] != "Ada Placeholder" {
		t.Errorf("first subject = %v", first)
	}
	second, _ := subjects[1].(map[string]any)
	if second["name"] != "Ada Placeholder & Bo Placeholder" {
		t.Errorf("family subject name = %v", second["name"])
	}
	summaries, _ := resp["proof_summaries"].([]any)
	if len(summaries) != 1 || summaries[0].(map[string]any)["conclusion"] != "Born 1815" {
		t.Errorf("proof_summaries = %v", resp["proof_summaries"])
	}

	// The list carries the record but does not resolve display data.
	list := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/branches", ""))
	items, _ := list["items"].([]any)
	item, _ := items[0].(map[string]any)
	if item["outcome"] != "inconclusive" || item["hypothesis"] == nil || item["proof_summaries"] != nil {
		t.Errorf("list item = %v", item)
	}
	if listSubject, _ := item["subjects"].([]any)[0].(map[string]any); listSubject["name"] != nil {
		t.Errorf("list subject carries a name: %v", listSubject)
	}
}

func TestCreateBranch_DefaultsAndEmptyArrays(t *testing.T) {
	server := setupBranchTestServer()
	rec := do(t, server, http.MethodPost, "/api/v1/branches", `{"name":"plain"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"outcome":"open"`, `"subjects":[]`, `"proof_summary_ids":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("body %s lacks %s", body, want)
		}
	}
	if strings.Contains(body, `"hypothesis"`) {
		t.Errorf("body %s carries an empty hypothesis", body)
	}
}

func TestCreateBranch_ResearchValidation(t *testing.T) {
	server := setupBranchTestServer()
	tests := []struct {
		name, body, code string
	}{
		{"hypothesis too long", fmt.Sprintf(`{"name":"x","hypothesis":%q}`, strings.Repeat("a", 2001)), "validation_error"},
		{"unknown outcome", `{"name":"x","outcome":"maybe"}`, "validation_error"},
		{"empty outcome", `{"name":"x","outcome":""}`, "validation_error"},
		{"bad subject type", fmt.Sprintf(`{"name":"x","subjects":[{"type":"source","id":%q}]}`, uuid.NewString()), "validation_error"},
		{"missing person", fmt.Sprintf(`{"name":"x","subjects":[{"type":"person","id":%q}]}`, uuid.NewString()), "invalid_reference"},
		{"missing proof", fmt.Sprintf(`{"name":"x","proof_summary_ids":[%q]}`, uuid.NewString()), "invalid_reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, server, http.MethodPost, "/api/v1/branches", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400. Body: %s", rec.Code, rec.Body.String())
			}
			if code := decodeJSON(t, rec)["code"]; code != tt.code {
				t.Errorf("code = %v, want %s", code, tt.code)
			}
		})
	}
}

func TestUpdateBranch(t *testing.T) {
	server := setupBranchTestServer()
	person := createPerson(t, server, "Ada", "Placeholder")
	branchID := createBranch(t, server, "Ada's parents")

	// A person and a proof summary created on the branch are valid references.
	rec := do(t, server, http.MethodPost, "/api/v1/persons?branch="+branchID,
		`{"given_name":"Branch","surname":"Only","gender":"unknown"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create branch person status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	branchPerson, _ := decodeJSON(t, rec)["id"].(string)
	proof := createProofSummaryOn(t, server, branchID, branchPerson, "Born 1820")

	rec = do(t, server, http.MethodPatch, "/api/v1/branches/"+branchID, fmt.Sprintf(`{
		"hypothesis": "Was Ada the daughter of Branch Only?",
		"subjects": [{"type":"person","id":%q},{"type":"person","id":%q}],
		"proof_summary_ids": [%q]
	}`, person, branchPerson, proof))
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON(t, rec)
	subjects, _ := resp["subjects"].([]any)
	if len(subjects) != 2 || subjects[1].(map[string]any)["name"] != "Branch Only" {
		t.Errorf("subjects = %v", resp["subjects"])
	}
	if summaries, _ := resp["proof_summaries"].([]any); len(summaries) != 1 {
		t.Errorf("proof_summaries = %v", resp["proof_summaries"])
	}

	// Partial: outcome only; the rest survives, and GET agrees.
	rec = do(t, server, http.MethodPatch, "/api/v1/branches/"+branchID, `{"outcome":"proved","description":"now described"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH outcome status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	got := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, ""))
	if got["outcome"] != "proved" || got["description"] != "now described" ||
		got["hypothesis"] != "Was Ada the daughter of Branch Only?" || len(got["subjects"].([]any)) != 2 {
		t.Errorf("GET after PATCH = %v", got)
	}

	// The comparison header carries the record, and no change entry for it.
	cmp := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/compare", ""))
	cmpBranch, _ := cmp["branch"].(map[string]any)
	if cmpBranch["outcome"] != "proved" || cmpBranch["proof_summaries"] == nil {
		t.Errorf("compare branch = %v", cmpBranch)
	}
	for _, entry := range cmp["branch_changes"].([]any) {
		if entry.(map[string]any)["entity_type"] == "branch" {
			t.Errorf("comparison lists a branch-metadata change: %v", entry)
		}
	}
}

func TestUpdateBranch_Errors(t *testing.T) {
	server := setupBranchTestServer()
	branchID := createBranch(t, server, "line")

	tests := []struct {
		name, path, body string
		status           int
		code             string
	}{
		{"empty body object", "/api/v1/branches/" + branchID, `{}`, http.StatusBadRequest, "validation_error"},
		{"bad outcome", "/api/v1/branches/" + branchID, `{"outcome":"maybe"}`, http.StatusBadRequest, "validation_error"},
		{"empty outcome", "/api/v1/branches/" + branchID, `{"outcome":""}`, http.StatusBadRequest, "validation_error"},
		{"description too long", "/api/v1/branches/" + branchID, fmt.Sprintf(`{"description":%q}`, strings.Repeat("a", 501)), http.StatusBadRequest, "validation_error"},
		{"duplicate subject", "/api/v1/branches/" + branchID, fmt.Sprintf(`{"subjects":[{"type":"person","id":%[1]q},{"type":"person","id":%[1]q}]}`, uuid.NewString()), http.StatusBadRequest, "validation_error"},
		{"missing family", "/api/v1/branches/" + branchID, fmt.Sprintf(`{"subjects":[{"type":"family","id":%q}]}`, uuid.NewString()), http.StatusBadRequest, "invalid_reference"},
		{"unknown branch", "/api/v1/branches/" + uuid.NewString(), `{"outcome":"proved"}`, http.StatusNotFound, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, server, http.MethodPatch, tt.path, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tt.status, rec.Body.String())
			}
			if code := decodeJSON(t, rec)["code"]; code != tt.code {
				t.Errorf("code = %v, want %s", code, tt.code)
			}
		})
	}

	if rec := do(t, server, http.MethodDelete, "/api/v1/branches/"+branchID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d", rec.Code)
	}
	rec := do(t, server, http.MethodPatch, "/api/v1/branches/"+branchID, `{"outcome":"disproved"}`)
	if rec.Code != http.StatusConflict || decodeJSON(t, rec)["code"] != "branch_not_active" {
		t.Errorf("archived PATCH = %d %s, want 409 branch_not_active", rec.Code, rec.Body.String())
	}

	unavailable := do(t, setupTestServer(), http.MethodPatch, "/api/v1/branches/"+branchID, `{"outcome":"proved"}`)
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Errorf("no registry PATCH = %d, want 503", unavailable.Code)
	}
}

// TestUpdateBranch_MergedAcceptsOnlyOutcome pins the documented merged-branch
// rule: the verdict can be recorded after the merge, nothing else changes.
func TestUpdateBranch_MergedAcceptsOnlyOutcome(t *testing.T) {
	server := setupBranchTestServer()
	_, branchID, _ := forkAndEditPerson(t, server, "Branchname")
	if rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", ""); rec.Code != http.StatusOK {
		t.Fatalf("merge status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	rec := do(t, server, http.MethodPatch, "/api/v1/branches/"+branchID, `{"outcome":"proved"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("outcome on merged = %d. Body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON(t, rec)
	if resp["outcome"] != "proved" || resp["status"] != "merged" || resp["merged_at"] == nil {
		t.Errorf("merged branch after PATCH = %v", resp)
	}

	rec = do(t, server, http.MethodPatch, "/api/v1/branches/"+branchID, `{"hypothesis":"too late"}`)
	if rec.Code != http.StatusConflict || decodeJSON(t, rec)["code"] != "branch_field_locked" {
		t.Errorf("hypothesis on merged = %d %s, want 409 branch_field_locked", rec.Code, rec.Body.String())
	}

	// An empty outcome is outside the enum: it must not reset the verdict.
	rec = do(t, server, http.MethodPatch, "/api/v1/branches/"+branchID, `{"outcome":""}`)
	if rec.Code != http.StatusBadRequest || decodeJSON(t, rec)["code"] != "validation_error" {
		t.Errorf("empty outcome on merged = %d %s, want 400 validation_error", rec.Code, rec.Body.String())
	}
	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID, "")
	if got := decodeJSON(t, rec)["outcome"]; got != "proved" {
		t.Errorf("outcome after empty-outcome PATCH = %v, want proved", got)
	}
}
