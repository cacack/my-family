package integration_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestBranchMergeSnapshot_EndToEnd is #833's acceptance scenario against every
// backend: a merge asked to snapshot first marks the mainline "Before merging
// <branch>", the merged branch's record names that snapshot and the last
// position the merge replayed, and comparing the snapshot up to that position
// lists exactly the merge's changes - however far the mainline has moved since.
func TestBranchMergeSnapshot_EndToEnd(t *testing.T) {
	forEachBackend(t, runBranchMergeSnapshot)
}

func runBranchMergeSnapshot(t *testing.T, server *api.Server) {
	t.Helper()

	ada := createPerson(t, server, "Ada", "Original")
	bystander := createPerson(t, server, "Mary", "Bystander")
	branchID := createBranch(t, server, "Byron theory")
	updateSurname(t, server, "/api/v1/persons/"+ada, branchID, "Byron")
	created := mustString(t, mustDo(t, server, http.MethodPost, "/api/v1/persons?branch="+branchID,
		`{"given_name":"Allegra","surname":"Byron","gender":"female"}`, http.StatusCreated), "id")
	// A mainline edit before the merge: not the merge's, and before the snapshot.
	updateSurname(t, server, "/api/v1/persons/"+bystander, "", "Before")

	merged := mustDo(t, server, http.MethodPost, mergePath(branchID), `{"note":"proven","snapshot_before":true}`, http.StatusOK)
	snapshot, ok := merged["pre_merge_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("merge response has no pre_merge_snapshot: %v", merged)
	}
	snapshotID := mustString(t, snapshot, "id")
	if snapshot["name"] != "Before merging Byron theory" || snapshot["branch_id"] != nil {
		t.Errorf("pre_merge_snapshot = %v, want a mainline snapshot named for the branch", snapshot)
	}
	listed := jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/snapshots", "", http.StatusOK), "items")
	if len(listed) != 1 {
		t.Errorf("mainline snapshots = %v, want the pre-merge one", listed)
	}

	// The mainline moves on after the merge.
	updateSurname(t, server, "/api/v1/persons/"+bystander, "", "After")

	record, ok := mustDo(t, server, http.MethodGet, comparePath(branchID), "", http.StatusOK)["merge_record"].(map[string]any)
	if !ok {
		t.Fatal("merged branch has no merge_record")
	}
	if record["pre_merge_snapshot_id"] != snapshotID {
		t.Errorf("merge_record.pre_merge_snapshot_id = %v, want %s", record["pre_merge_snapshot_id"], snapshotID)
	}
	through, ok := record["replayed_through_position"].(float64)
	if !ok || through <= snapshot["position"].(float64) {
		t.Fatalf("merge_record.replayed_through_position = %v, want a position after the snapshot's %v", record["replayed_through_position"], snapshot["position"])
	}

	// Exactly the merge's effect: Ada's edit and Allegra's creation (her name
	// record included), each carrying the merge's provenance.
	exact := mustDo(t, server, http.MethodGet, fmt.Sprintf("/api/v1/snapshots/%s/compare-current?until=%d", snapshotID, int64(through)), "", http.StatusOK)
	if exact["to_position"] != through || exact["head_position"].(float64) <= through {
		t.Errorf("bounded comparison ran to %v (head %v), want %v before the head", exact["to_position"], exact["head_position"], through)
	}
	changes := jsonArray(t, exact, "changes")
	seen := map[string]bool{}
	for _, raw := range changes {
		entry, _ := raw.(map[string]any)
		seen[fmt.Sprint(entry["entity_id"])] = true
		if origin, _ := entry["merged_from"].(map[string]any); origin == nil || origin["branch_id"] != branchID {
			t.Errorf("change %v in the merge's range does not come from the merge", entry)
		}
	}
	if len(changes) != 3 || !seen[ada] || !seen[created] {
		t.Errorf("merge's changes = %v, want Ada's edit and Allegra's creation", changes)
	}

	// Compared to now, the later mainline edit shows too.
	now := mustDo(t, server, http.MethodGet, "/api/v1/snapshots/"+snapshotID+"/compare-current", "", http.StatusOK)
	if got := len(jsonArray(t, now, "changes")); got != len(changes)+1 {
		t.Errorf("changes since before the merge = %d, want the merge's %d and the later edit", got, len(changes))
	}

	// An end before the snapshot is refused.
	mustDo(t, server, http.MethodGet, "/api/v1/snapshots/"+snapshotID+"/compare-current?until=0", "", http.StatusBadRequest)

	// A merge that does not ask takes no snapshot.
	plain := createBranch(t, server, "Plain")
	updateSurname(t, server, "/api/v1/persons/"+ada, plain, "Plain")
	if resp := mustDo(t, server, http.MethodPost, mergePath(plain), `{}`, http.StatusOK); resp["pre_merge_snapshot"] != nil {
		t.Errorf("merge without snapshot_before took one: %v", resp["pre_merge_snapshot"])
	}
	plainRecord, _ := mustDo(t, server, http.MethodGet, comparePath(plain), "", http.StatusOK)["merge_record"].(map[string]any)
	if plainRecord["pre_merge_snapshot_id"] != nil {
		t.Errorf("merge_record.pre_merge_snapshot_id = %v, want none", plainRecord["pre_merge_snapshot_id"])
	}
}
