package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestMergeBranch_SnapshotBefore covers #833 at the API: snapshot_before takes
// a mainline "Before merging <branch>" snapshot, the merge response and the
// merged branch's record name it, and the record's replayed-through position
// bounds a comparison that lists exactly the merge's change.
func TestMergeBranch_SnapshotBefore(t *testing.T) {
	server := setupBranchTestServer()
	personID, branchID, _ := forkAndEditPerson(t, server, "Byron")

	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{"snapshot_before":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Merge: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	snapshot, ok := decodeJSON(t, rec)["pre_merge_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("merge response has no pre_merge_snapshot: %s", rec.Body.String())
	}
	if name, _ := snapshot["name"].(string); name == "" || name[:len("Before merging ")] != "Before merging " {
		t.Errorf("pre_merge_snapshot.name = %v", snapshot["name"])
	}

	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+branchID+"/compare", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("compare: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	record, _ := decodeJSON(t, rec)["merge_record"].(map[string]any)
	if record["pre_merge_snapshot_id"] != snapshot["id"] {
		t.Errorf("merge_record.pre_merge_snapshot_id = %v, want %v", record["pre_merge_snapshot_id"], snapshot["id"])
	}
	through, ok := record["replayed_through_position"].(float64)
	if !ok {
		t.Fatalf("merge_record has no replayed_through_position: %v", record)
	}

	body, changes := comparisonChanges(t, server, fmt.Sprintf("/api/v1/snapshots/%s/compare-current?until=%d", snapshot["id"], int64(through)))
	if len(changes) != 1 || changes[0][0] != "updated" || body["to_position"] != through {
		t.Errorf("the merge's comparison = %v (to %v), want the one replayed edit of %s", changes, body["to_position"], personID)
	}

	rec = do(t, server, http.MethodGet, fmt.Sprintf("/api/v1/snapshots/%s/compare-current?until=-1", snapshot["id"]), "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("until before the snapshot: status = %d, want 400. Body: %s", rec.Code, rec.Body.String())
	}
}
