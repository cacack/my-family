package integration_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestBranchClose_ResearchRetained drives #836 over the real HTTP API on every
// backend: a branch that disproves its hypothesis is closed with an outcome and
// a reason, its overlay is purged, and its research — above all the searches
// that found nothing — stays reachable: rebuilt from the branch's events by
// GET /branches/{id}/research, and copied to the mainline by the promote call.
//
// Fixtures use neutral placeholder names only (public repo — no real PII).
func TestBranchClose_ResearchRetained(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			runBranchCloseResearchRetained(t, newServer(t, st))
		})
	}
}

func runBranchCloseResearchRetained(t *testing.T, server *api.Server) {
	t.Helper()

	// --- Main: a person and a research log about them. ---
	person := createPerson(t, server, "Robin", "Placeholder")
	mainLog := createResearchLog(t, server, "", person, "County archive")

	// --- Branch: the line of research that will be disproved. ---
	created := mustDo(t, server, http.MethodPost, "/api/v1/branches",
		`{"name":"parent-theory","hypothesis":"Was Robin the child of the Example family?"}`, http.StatusCreated)
	branchID := mustString(t, created, "id")
	branchPath := "/api/v1/branches/" + branchID

	// A negative search about a mainline person: the evidence that matters.
	notFound := createResearchLog(t, server, branchID, person, "Parish registers")
	// A search about a person that only ever existed on the branch.
	branchPerson := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/persons", branchID),
		`{"given_name":"Jamie","surname":"Hypothetical","gender":"unknown"}`, http.StatusCreated), "id")
	aboutBranchPerson := createResearchLog(t, server, branchID, branchPerson, "Census index")
	// An edit of main's log on the branch.
	mainLogPath := "/api/v1/research-logs/" + mainLog
	mustDo(t, server, http.MethodPut, scoped(mainLogPath, branchID),
		fmt.Sprintf(`{"notes":"Re-checked on the branch","version":%d}`, entityVersion(t, server, mainLogPath, branchID)),
		http.StatusOK)
	// A log written and then deleted on the branch.
	discarded := createResearchLog(t, server, branchID, person, "Wrong archive")
	discardedPath := "/api/v1/research-logs/" + discarded
	mustDo(t, server, http.MethodDelete,
		fmt.Sprintf("%s&version=%d", scoped(discardedPath, branchID), entityVersion(t, server, discardedPath, branchID)),
		"", http.StatusNoContent)
	// An evidence analysis and a proof summary.
	analysis := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/evidence-analyses", branchID),
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"No baptism found"}`, person), http.StatusCreated), "id")
	proof := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/proof-summaries", branchID),
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Not the Example family's child","argument":"No record supports it"}`, person),
		http.StatusCreated), "id")

	// Promotion is only for a closed branch.
	if resp := mustDo(t, server, http.MethodPost, branchPath+"/research-logs/promote", `{}`, http.StatusConflict); resp["code"] != "branch_not_closed" {
		t.Errorf("promote on an active branch code = %v, want branch_not_closed", resp["code"])
	}
	// A close outcome must be a verdict that ends the branch.
	mustDo(t, server, http.MethodPost, branchPath+"/close", `{"outcome":"proved"}`, http.StatusBadRequest)

	// --- Close as disproved. ---
	closed := mustDo(t, server, http.MethodPost, branchPath+"/close",
		`{"outcome":"disproved","reason":"The baptism register names other parents."}`, http.StatusOK)
	if closed["status"] != "archived" || closed["outcome"] != "disproved" ||
		closed["close_reason"] != "The baptism register names other parents." || closed["closed_at"] == nil {
		t.Fatalf("closed branch = %v, want archived/disproved with reason and closed_at", closed)
	}
	if resp := mustDo(t, server, http.MethodPost, branchPath+"/close", `{"outcome":"abandoned"}`, http.StatusConflict); resp["code"] != "branch_not_active" {
		t.Errorf("second close code = %v, want branch_not_active", resp["code"])
	}

	// The record survives in the registry and the list.
	got := getEntity(t, server, branchPath, "")
	if got["outcome"] != "disproved" || got["close_reason"] != "The baptism register names other parents." {
		t.Errorf("GET closed branch = %v", got)
	}
	var listed map[string]any
	for _, item := range jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/branches", "", http.StatusOK), "items") {
		if m, _ := item.(map[string]any); m["id"] == branchID {
			listed = m
		}
	}
	if listed == nil || listed["outcome"] != "disproved" || listed["close_reason"] == nil {
		t.Errorf("listed closed branch = %v", listed)
	}

	// The overlay is gone...
	mustDo(t, server, http.MethodGet, scoped("/api/v1/research-logs/"+notFound, branchID), "", http.StatusNotFound)
	// ...but the research is rebuilt from the events.
	archive := mustDo(t, server, http.MethodGet, branchPath+"/research", "", http.StatusOK)
	logs := jsonArray(t, archive, "research_logs")
	if len(logs) != 3 {
		t.Fatalf("archived research logs = %d (%v), want 3", len(logs), logs)
	}
	byID := map[string]map[string]any{}
	for _, raw := range logs {
		entry, _ := raw.(map[string]any)
		log, _ := entry["log"].(map[string]any)
		byID[mustString(t, log, "id")] = entry
	}
	if entry := byID[notFound]; entry == nil || entry["created_on_branch"] != true ||
		entry["subject_name"] != "Robin Placeholder" || entry["log"].(map[string]any)["outcome"] != "not_found" {
		t.Errorf("archived negative search = %v", entry)
	}
	if entry := byID[aboutBranchPerson]; entry == nil || entry["subject_name"] != "Jamie Hypothetical" {
		t.Errorf("archived log about a branch-only person = %v", entry)
	}
	if entry := byID[mainLog]; entry == nil || entry["created_on_branch"] != false ||
		entry["log"].(map[string]any)["notes"] != "Re-checked on the branch" {
		t.Errorf("archived edit of a mainline log = %v", entry)
	}
	if byID[discarded] != nil {
		t.Errorf("a log deleted on the branch is listed: %v", byID[discarded])
	}
	if archive["deleted_count"] != float64(1) || archive["truncated"] != false {
		t.Errorf("deleted_count/truncated = %v/%v, want 1/false", archive["deleted_count"], archive["truncated"])
	}
	analyses := jsonArray(t, archive, "evidence_analyses")
	if len(analyses) != 1 || analyses[0].(map[string]any)["analysis"].(map[string]any)["id"] != analysis {
		t.Errorf("archived analyses = %v, want %s", analyses, analysis)
	}
	summaries := jsonArray(t, archive, "proof_summaries")
	if len(summaries) != 1 || summaries[0].(map[string]any)["summary"].(map[string]any)["id"] != proof {
		t.Errorf("archived proof summaries = %v, want %s", summaries, proof)
	}

	// --- Promote: only the log created on the branch about a mainline subject
	// lands; the others are skipped with their reasons. ---
	promoted := mustDo(t, server, http.MethodPost, branchPath+"/research-logs/promote", `{}`, http.StatusOK)
	if ids := jsonArray(t, promoted, "promoted"); len(ids) != 1 || ids[0] != notFound {
		t.Errorf("promoted = %v, want [%s]", ids, notFound)
	}
	skipped := map[string]string{}
	for _, raw := range jsonArray(t, promoted, "skipped") {
		m, _ := raw.(map[string]any)
		skipped[m["id"].(string)] = m["reason"].(string)
	}
	if skipped[mainLog] != "not_created_on_branch" || skipped[aboutBranchPerson] != "subject_not_on_main" {
		t.Errorf("skipped = %v", skipped)
	}
	onMain := getEntity(t, server, "/api/v1/research-logs/"+notFound, "")
	if onMain["outcome"] != "not_found" || !strings.Contains(fmt.Sprint(onMain["notes"]), "closed as disproved") ||
		!strings.Contains(fmt.Sprint(onMain["notes"]), "The baptism register names other parents.") {
		t.Errorf("promoted log on main = %v", onMain)
	}

	// Promoting again is safe; an unknown id is reported, not an error.
	again := mustDo(t, server, http.MethodPost, branchPath+"/research-logs/promote",
		fmt.Sprintf(`{"log_ids":[%q,%q]}`, notFound, person), http.StatusOK)
	if ids := jsonArray(t, again, "promoted"); len(ids) != 0 {
		t.Errorf("second promote promoted %v, want none", ids)
	}
	reasons := map[string]string{}
	for _, raw := range jsonArray(t, again, "skipped") {
		m, _ := raw.(map[string]any)
		reasons[m["id"].(string)] = m["reason"].(string)
	}
	if reasons[notFound] != "already_promoted" || reasons[person] != "not_found" {
		t.Errorf("second promote skipped = %v", reasons)
	}

	// A promoted log deleted on main stays deleted when the branch's logs are
	// promoted again: main's stream for it is not empty, so the append is
	// refused on every backend and the log is reported as already promoted.
	mustDo(t, server, http.MethodDelete, fmt.Sprintf("/api/v1/research-logs/%s?version=%v", notFound, onMain["version"]), "", http.StatusNoContent)
	third := mustDo(t, server, http.MethodPost, branchPath+"/research-logs/promote", `{}`, http.StatusOK)
	if ids := jsonArray(t, third, "promoted"); len(ids) != 0 {
		t.Errorf("promotion after a delete on main promoted %v, want none", ids)
	}
	reasons = map[string]string{}
	for _, raw := range jsonArray(t, third, "skipped") {
		m, _ := raw.(map[string]any)
		reasons[m["id"].(string)] = m["reason"].(string)
	}
	if reasons[notFound] != "already_promoted" {
		t.Errorf("promotion after a delete on main skipped = %v", reasons)
	}
	mustDo(t, server, http.MethodGet, "/api/v1/research-logs/"+notFound, "", http.StatusNotFound)

	// --- DELETE still closes; an open branch is recorded as abandoned. ---
	other := createBranch(t, server, "dropped-line")
	mustDo(t, server, http.MethodDelete, "/api/v1/branches/"+other, "", http.StatusNoContent)
	if got := getEntity(t, server, "/api/v1/branches/"+other, ""); got["outcome"] != "abandoned" || got["status"] != "archived" {
		t.Errorf("deleted branch = %v, want archived/abandoned", got)
	}

	mustDo(t, server, http.MethodGet, "/api/v1/branches/"+person+"/research", "", http.StatusNotFound)
	mustDo(t, server, http.MethodPost, "/api/v1/branches/"+person+"/close", `{"outcome":"abandoned"}`, http.StatusNotFound)
	mustDo(t, server, http.MethodPost, "/api/v1/branches/"+person+"/research-logs/promote", `{}`, http.StatusNotFound)
}
