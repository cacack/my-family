package integration_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/api"
)

// TestBranchMergeRecord_EndToEnd is #832's acceptance scenario against every
// backend: after a merge, the merged branch's compare carries the merge record
// (what was decided, by name, and what was left behind) and keeps the replayed
// copies out of the mainline column; the merged entity's history says which
// branch and note its change came from; and the global history shows the
// merge and its replayed changes in the window of the merge, not of the
// original edits.
func TestBranchMergeRecord_EndToEnd(t *testing.T) {
	forEachBackend(t, runBranchMergeRecord)
}

func runBranchMergeRecord(t *testing.T, server *api.Server) {
	t.Helper()

	ada := createPerson(t, server, "Ada", "Original")
	allegra := createPerson(t, server, "Allegra", "Original")
	branchID := createBranch(t, server, "Byron theory")
	updateSurname(t, server, "/api/v1/persons/"+ada, branchID, "Byron")
	updateSurname(t, server, "/api/v1/persons/"+allegra, branchID, "Clairmont")
	updateSurname(t, server, "/api/v1/persons/"+ada, "", "King")

	// Everything so far happened before the window opens.
	time.Sleep(20 * time.Millisecond)
	windowStart := time.Now().UTC()
	time.Sleep(20 * time.Millisecond)

	mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("the register settles it",
		fmt.Sprintf(`{"stream_id":%q,"resolution":"branch","rationale":"baptism register"}`, ada),
		resolution(allegra, "main")), http.StatusOK)

	// --- The merged branch shows its record. ---
	compare := mustDo(t, server, http.MethodGet, comparePath(branchID), "", http.StatusOK)
	record, ok := compare["merge_record"].(map[string]any)
	if !ok {
		t.Fatalf("merged branch compare carries no merge_record: %v", compare)
	}
	if record["recorded"] != true || record["note"] != "the register settles it" || record["replayed_event_count"] != float64(1) {
		t.Errorf("merge_record = %v", record)
	}
	if skipped := stringValues(t, record, "skipped_stream_ids"); len(skipped) != 1 || skipped[0] != allegra {
		t.Errorf("skipped_stream_ids = %v, want [%s]", skipped, allegra)
	}
	decisions := jsonArray(t, record, "decisions")
	if len(decisions) != 1 {
		t.Fatalf("decisions = %v, want the one conflict", decisions)
	}
	decision, _ := decisions[0].(map[string]any)
	if decision["stream_id"] != ada || decision["kind"] != "edit_edit" || decision["resolution"] != "branch" ||
		decision["rationale"] != "baptism register" || decision["entity_name"] == "" || decision["decided_at"] != "merge" {
		t.Errorf("decision = %v", decision)
	}
	exclusions := jsonArray(t, record, "exclusions")
	if len(exclusions) != 1 {
		t.Fatalf("exclusions = %v, want Allegra", exclusions)
	}
	if exclusion, _ := exclusions[0].(map[string]any); exclusion["stream_id"] != allegra || exclusion["entity_name"] != "Allegra Clairmont" {
		t.Errorf("exclusion = %v, want Allegra named as the branch had her", exclusion)
	}
	if compare["replayed_change_count"] != float64(1) {
		t.Errorf("replayed_change_count = %v, want 1", compare["replayed_change_count"])
	}
	for _, raw := range jsonArray(t, compare, "main_changes") {
		if entry, _ := raw.(map[string]any); entry["merged_from"] != nil {
			t.Errorf("main_changes lists the merge's replayed copy %v", entry)
		}
	}

	// --- The merged entity's history names the branch and the note. ---
	history := mustDo(t, server, http.MethodGet, "/api/v1/persons/"+ada+"/history", "", http.StatusOK)
	items := jsonArray(t, history, "items")
	last, _ := items[len(items)-1].(map[string]any)
	origin, ok := last["merged_from"].(map[string]any)
	if !ok || origin["branch_id"] != branchID || origin["branch_name"] != "Byron theory" || origin["note"] != "the register settles it" {
		t.Errorf("latest entry of the merged person = %v, want it via the merge of Byron theory", last)
	}

	// --- The global history window of the merge holds the merge and its replay. ---
	global := mustDo(t, server, http.MethodGet, "/api/v1/history?limit=50&from="+url.QueryEscape(windowStart.Format(time.RFC3339Nano)), "", http.StatusOK)
	var sawMerge, sawReplay bool
	for _, raw := range jsonArray(t, global, "items") {
		entry, _ := raw.(map[string]any)
		switch {
		case entry["entity_type"] == "branch" && entry["action"] == "merged":
			sawMerge = entry["entity_name"] == "Byron theory"
		case entry["entity_id"] == ada && entry["merged_from"] != nil:
			sawReplay = true
		case entry["entity_id"] == ada:
			t.Errorf("the window after the merge lists main's earlier edit %v", entry)
		}
	}
	if !sawMerge || !sawReplay {
		t.Errorf("history window from the merge: merge entry %v, replayed change %v; items = %v", sawMerge, sawReplay, global["items"])
	}
	branchOnly := mustDo(t, server, http.MethodGet, "/api/v1/history?entity_type=branch", "", http.StatusOK)
	if total := branchOnly["total"]; total != float64(2) {
		t.Errorf("branch lifecycle entries = %v, want created and merged", total)
	}
}

// lastMergedFrom returns the merged_from of a person's latest history entry.
func lastMergedFrom(t *testing.T, server *api.Server, personID string) map[string]any {
	t.Helper()
	items := jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/history", "", http.StatusOK), "items")
	last, _ := items[len(items)-1].(map[string]any)
	origin, _ := last["merged_from"].(map[string]any)
	if origin == nil {
		t.Fatalf("person %s latest history entry has no merged_from: %v", personID, last)
	}
	return origin
}
