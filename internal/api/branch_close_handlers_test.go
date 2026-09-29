package api_test

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
)

func TestCloseBranch_API(t *testing.T) {
	server := setupBranchTestServer()
	person := createPerson(t, server, "Ada", "Placeholder")
	branchID := createBranch(t, server, "theory")
	path := "/api/v1/branches/" + branchID

	rec := do(t, server, http.MethodPost, "/api/v1/research-logs?branch="+branchID, fmt.Sprintf(
		`{"subject_id":%q,"subject_type":"person","repository":"Parish","search_description":"Baptisms","outcome":"not_found","search_date":"2024-03-01T00:00:00Z"}`, person))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create research log = %d %s", rec.Code, rec.Body.String())
	}
	logID, _ := decodeJSON(t, rec)["id"].(string)

	for _, tc := range []struct {
		name, body, code string
		want             int
	}{
		{"missing outcome", `{}`, "", http.StatusBadRequest},
		{"open is not a close outcome", `{"outcome":"open"}`, "", http.StatusBadRequest},
		{"proved is merged, not closed", `{"outcome":"proved"}`, "validation_error", http.StatusBadRequest},
		{"reason too long", fmt.Sprintf(`{"outcome":"abandoned","reason":%q}`, strings.Repeat("a", 2001)), "", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, server, http.MethodPost, path+"/close", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.code != "" && decodeJSON(t, rec)["code"] != tc.code {
				t.Errorf("code = %v, want %s", decodeJSON(t, rec)["code"], tc.code)
			}
		})
	}

	rec = do(t, server, http.MethodPost, path+"/close", `{"outcome":"disproved","reason":"Other parents named"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("close = %d %s", rec.Code, rec.Body.String())
	}
	closed := decodeJSON(t, rec)
	if closed["status"] != "archived" || closed["outcome"] != "disproved" || closed["close_reason"] != "Other parents named" || closed["closed_at"] == nil {
		t.Errorf("closed = %v", closed)
	}
	if rec := do(t, server, http.MethodPost, path+"/close", `{"outcome":"abandoned"}`); rec.Code != http.StatusConflict ||
		decodeJSON(t, rec)["code"] != "branch_not_active" {
		t.Errorf("second close = %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, server, http.MethodGet, path+"/research", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("research = %d %s", rec.Code, rec.Body.String())
	}
	archive := decodeJSON(t, rec)
	logs, _ := archive["research_logs"].([]any)
	if len(logs) != 1 {
		t.Fatalf("research_logs = %v", archive["research_logs"])
	}
	entry, _ := logs[0].(map[string]any)
	if entry["subject_name"] != "Ada Placeholder" || entry["created_on_branch"] != true || entry["log"].(map[string]any)["id"] != logID {
		t.Errorf("archived log = %v", entry)
	}
	for _, field := range []string{"evidence_analyses", "proof_summaries"} {
		if arr, ok := archive[field].([]any); !ok || len(arr) != 0 {
			t.Errorf("%s = %v, want []", field, archive[field])
		}
	}

	rec = do(t, server, http.MethodPost, path+"/research-logs/promote", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	promoted := decodeJSON(t, rec)
	if ids, _ := promoted["promoted"].([]any); len(ids) != 1 || ids[0] != logID {
		t.Errorf("promoted = %v", promoted)
	}
	if skipped, ok := promoted["skipped"].([]any); !ok || len(skipped) != 0 {
		t.Errorf("skipped = %v, want []", promoted["skipped"])
	}
	rec = do(t, server, http.MethodPost, path+"/research-logs/promote", fmt.Sprintf(`{"log_ids":[%q]}`, logID))
	if rec.Code != http.StatusOK {
		t.Fatalf("second promote = %d %s", rec.Code, rec.Body.String())
	}
	skipped, _ := decodeJSON(t, rec)["skipped"].([]any)
	if len(skipped) != 1 || skipped[0].(map[string]any)["reason"] != "already_promoted" {
		t.Errorf("second promote skipped = %v", skipped)
	}
}

func TestCloseBranch_APIRefusals(t *testing.T) {
	server := setupBranchTestServer()
	active := createBranch(t, server, "active")
	unknown := "00000000-0000-0000-0000-00000000abcd"
	// One more id than the spec's maxItems: refused before the branch is read.
	tooMany := make([]string, command.MaxPromoteLogIDs+1)
	for i := range tooMany {
		tooMany[i] = strconv.Quote(uuid.New().String())
	}
	tooManyBody := `{"log_ids":[` + strings.Join(tooMany, ",") + `]}`

	for _, tc := range []struct {
		name, method, path, body, code string
		want                           int
	}{
		{"close unknown", http.MethodPost, "/api/v1/branches/" + unknown + "/close", `{"outcome":"abandoned"}`, "not_found", http.StatusNotFound},
		{"research unknown", http.MethodGet, "/api/v1/branches/" + unknown + "/research", "", "not_found", http.StatusNotFound},
		{"promote unknown", http.MethodPost, "/api/v1/branches/" + unknown + "/research-logs/promote", `{}`, "not_found", http.StatusNotFound},
		{"promote active", http.MethodPost, "/api/v1/branches/" + active + "/research-logs/promote", `{}`, "branch_not_closed", http.StatusConflict},
		{"promote too many ids", http.MethodPost, "/api/v1/branches/" + active + "/research-logs/promote", tooManyBody, "validation_error", http.StatusBadRequest},
		{"research of an active branch", http.MethodGet, "/api/v1/branches/" + active + "/research", "", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, server, tc.method, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.code != "" && decodeJSON(t, rec)["code"] != tc.code {
				t.Errorf("code = %v, want %s", decodeJSON(t, rec)["code"], tc.code)
			}
		})
	}
}
