package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestBranchMergeState_EndToEnd is #830's acceptance scenario against every
// backend: an interrupted merge reads as `merge_state: incomplete` on the
// branch, the list and the comparison, naming what is left; reading it writes
// nothing; a mainline change to the pending entity makes it need a decision,
// which the resume's refusal names too; and resuming with that decision makes
// the merge complete.
func TestBranchMergeState_EndToEnd(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			faulty := &faultyReplayStore{EventStore: st.events}
			wrapped := st
			wrapped.events = faulty
			runBranchMergeState(t, newServer(t, wrapped), st.events, faulty)
		})
	}
}

func runBranchMergeState(t *testing.T, server *api.Server, events repository.EventStore, faulty *faultyReplayStore) {
	t.Helper()
	ctx := context.Background()

	first := createPerson(t, server, "Alex", "Original")
	second := createPerson(t, server, "Sam", "Steady")
	branchID := createBranch(t, server, "state-line")
	branchPath := "/api/v1/branches/" + branchID
	for _, id := range []string{first, second} {
		path := "/api/v1/persons/" + id
		mustDo(t, server, http.MethodPut, scoped(path, branchID),
			fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, path, branchID)),
			http.StatusOK)
	}

	// An active branch carries no merge state.
	if got, ok := mustDo(t, server, http.MethodGet, branchPath, "", http.StatusOK)["merge_state"]; ok {
		t.Errorf("active branch merge_state = %v, want absent", got)
	}

	faulty.arm(2)
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()

	pendingID := second
	if mustString(t, getEntity(t, server, "/api/v1/persons/"+second, ""), "surname") == "Revised" {
		pendingID = first
	}

	// --- Every read reports the incomplete merge, and none of them writes. ---
	before := readBranchEvents(t, ctx, events, domain.MainBranchID)
	headBefore := len(readAllEvents(t, ctx, events))
	got := mustDo(t, server, http.MethodGet, branchPath, "", http.StatusOK)
	assertIncomplete(t, "GET /branches/{id}", got, pendingID, "ready", false)

	list := jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/branches", "", http.StatusOK), "items")
	var listed map[string]any
	for _, raw := range list {
		if item, ok := raw.(map[string]any); ok && item["id"] == branchID {
			listed = item
		}
	}
	if listed == nil {
		t.Fatalf("branch %s missing from the list", branchID)
	}
	assertIncomplete(t, "GET /branches", listed, pendingID, "ready", false)

	compared := mustDo(t, server, http.MethodGet, branchPath+"/compare", "", http.StatusOK)
	branch, ok := compared["branch"].(map[string]any)
	if !ok {
		t.Fatalf("compare branch = %v, want an object", compared["branch"])
	}
	assertIncomplete(t, "GET /branches/{id}/compare", branch, pendingID, "ready", false)

	if after := len(readAllEvents(t, ctx, events)); after != headBefore {
		t.Errorf("the merge-state reads appended %d event(s), want none", after-headBefore)
	}
	assertPrefixUnchanged(t, "main", before, readBranchEvents(t, ctx, events, domain.MainBranchID))
	if surname := mustString(t, getEntity(t, server, "/api/v1/persons/"+pendingID, ""), "surname"); surname == "Revised" {
		t.Errorf("pending entity surname on main = %q, want the reads to have replayed nothing", surname)
	}

	// --- A mainline change makes the pending entity need a decision. ---
	path := "/api/v1/persons/" + pendingID
	mustDo(t, server, http.MethodPut, path,
		fmt.Sprintf(`{"given_name":"Samuel","version":%d}`, entityVersion(t, server, path, "")), http.StatusOK)
	assertIncomplete(t, "GET /branches/{id} after a mainline change",
		mustDo(t, server, http.MethodGet, branchPath, "", http.StatusOK), pendingID, "main_changed", true)

	refused := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusConflict)
	if refused["code"] != "merge_resume_needs_resolution" {
		t.Fatalf("resume code = %v, want merge_resume_needs_resolution", refused["code"])
	}
	described := jsonArray(t, refused, "pending")
	if len(described) != 1 {
		t.Fatalf("refusal pending = %v, want one described entity", described)
	}
	entity, _ := described[0].(map[string]any)
	if entity["stream_id"] != pendingID || entity["reason"] != "main_changed" || entity["entity_type"] != "person" ||
		entity["entity_name"] == "" {
		t.Errorf("refusal pending[0] = %v, want the named person %s, main_changed", entity, pendingID)
	}

	// --- Resuming with the decision completes the merge. ---
	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"branch"}]}`, pendingID), http.StatusOK)
	if resumedBranch, _ := resumed["branch"].(map[string]any); resumedBranch["merge_state"] != "complete" {
		t.Errorf("resume result branch merge_state = %v, want complete", resumedBranch["merge_state"])
	}
	done := mustDo(t, server, http.MethodGet, branchPath, "", http.StatusOK)
	if done["merge_state"] != "complete" {
		t.Errorf("merge_state after resume = %v, want complete", done["merge_state"])
	}
	if pending, ok := done["merge_pending"]; ok {
		t.Errorf("merge_pending after resume = %v, want absent", pending)
	}
}

// assertIncomplete checks a branch object reports an incomplete merge whose
// only pending entity is wantID, pending for wantReason.
func assertIncomplete(t *testing.T, where string, branch map[string]any, wantID, wantReason string, needsResolution bool) {
	t.Helper()
	if branch["merge_state"] != "incomplete" {
		t.Errorf("%s: merge_state = %v, want incomplete", where, branch["merge_state"])
		return
	}
	pending := jsonArray(t, branch, "merge_pending")
	if len(pending) != 1 {
		t.Errorf("%s: merge_pending = %v, want exactly %s", where, pending, wantID)
		return
	}
	entity, _ := pending[0].(map[string]any)
	if entity["stream_id"] != wantID || entity["entity_type"] != "person" || entity["entity_name"] == "" ||
		entity["reason"] != wantReason || entity["needs_resolution"] != needsResolution {
		t.Errorf("%s: merge_pending[0] = %v, want named person %s, reason %s, needs_resolution %v",
			where, entity, wantID, wantReason, needsResolution)
	}
}

// readAllEvents reads the whole log, every branch included.
func readAllEvents(t *testing.T, ctx context.Context, events repository.EventStore) []repository.StoredEvent {
	t.Helper()
	all, err := events.ReadAll(ctx, 0, 1_000_000)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return all
}
