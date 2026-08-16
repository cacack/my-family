package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestBranchLifecycle_EndToEnd drives the epic-#54 flow — create a branch, edit
// it, review the diff, merge, verify main — over the real HTTP API against every
// storage backend.
//
// Merge had only ever been exercised in memory, so this is where DB-001
// (dual-database parity) is proven for the most complex branch operation. The
// scenario body lives once, in runBranchLifecycle, and every backend runs that
// same body: divergence between the databases shows up as a failing subtest
// rather than as a copy that quietly drifted.
//
// Fixtures use neutral placeholder names only (public repo — no real PII).
func TestBranchLifecycle_EndToEnd(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			runBranchLifecycle(t, newServer(t, st), st)
		})
	}
}

// runBranchLifecycle is the backend-agnostic scenario body.
//
// It takes the store bundle as well as the server because two of its assertions
// are about the event log itself (BR-004 append-only), which the HTTP API
// deliberately does not expose.
func runBranchLifecycle(t *testing.T, server *api.Server, st stores) {
	t.Helper()
	ctx := context.Background()

	// --- Seed main: two persons, a family linking them, and a prospective
	// child. The child is created on main so the branch's later link has a
	// mainline referent — a link to a branch-only person is refused at merge as
	// a dangling reference, which is a different scenario. ---
	partner1 := createPerson(t, server, "Alex", "Original")
	partner2 := createPerson(t, server, "Sam", "Steady")
	childID := createPerson(t, server, "Casey", "Prospect")
	familyID := createFamily(t, server, partner1, partner2)

	personPath := "/api/v1/persons/" + partner1
	namesPath := personPath + "/names"
	familyPath := "/api/v1/families/" + familyID
	childrenPath := familyPath + "/children"

	// --- Fork. Everything above is main's history; nothing is written to main
	// after this point, so the branch has no divergence to reconcile. ---
	branchID := createBranch(t, server, "research-line")
	branchPath := "/api/v1/branches/" + branchID

	// --- Branch-scoped edits across two top-level entity types and their
	// sub-entities: person + person name, family + family child. These are the
	// four branch-aware event types the overlay supports for these entities. ---
	mustDo(t, server, http.MethodPut, scoped(personPath, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, personPath, branchID)),
		http.StatusOK)

	mustDo(t, server, http.MethodPost, scoped(namesPath, branchID),
		`{"given_name":"Alexandra","surname":"Revised","name_type":"birth"}`,
		http.StatusCreated)

	mustDo(t, server, http.MethodPut, scoped(familyPath, branchID),
		fmt.Sprintf(`{"marriage_place":"Springfield","version":%d}`, entityVersion(t, server, familyPath, branchID)),
		http.StatusOK)

	mustDo(t, server, http.MethodPost, scoped(childrenPath, branchID),
		fmt.Sprintf(`{"person_id":%q}`, childID), http.StatusCreated)

	// The branch's own view carries all four edits.
	assertBranchEdits(t, server, personPath, namesPath, familyPath, childID, branchID)

	// --- Isolation: an unscoped read still sees exactly what main was seeded
	// with. This is the guarantee the merge below is about to spend. ---
	assertMainUnedited(t, server, personPath, namesPath, familyPath)

	// --- Compare: both edited streams show on the branch side, main has moved
	// on neither of them, so there is nothing to reconcile. ---
	compare := mustDo(t, server, http.MethodGet, branchPath+"/compare", "", http.StatusOK)
	branchChanges := jsonArray(t, compare, "branch_changes")
	changedEntities := entryField(t, branchChanges, "entity_id")
	if !contains(changedEntities, partner1) {
		t.Errorf("branch_changes entity_ids = %v, want the edited person %s", changedEntities, partner1)
	}
	if !contains(changedEntities, familyID) {
		t.Errorf("branch_changes entity_ids = %v, want the edited family %s", changedEntities, familyID)
	}
	if mainChanges := jsonArray(t, compare, "main_changes"); len(mainChanges) != 0 {
		t.Errorf("main_changes = %v, want none (main saw no work after the fork)", mainChanges)
	}
	if overlap := jsonArray(t, compare, "overlapping_stream_ids"); len(overlap) != 0 {
		t.Errorf("overlapping_stream_ids = %v, want none", overlap)
	}
	if conflicts := jsonArray(t, compare, "conflicts"); len(conflicts) != 0 {
		t.Errorf("conflicts = %v, want none", conflicts)
	}
	if compare["has_more"] != false {
		t.Errorf("has_more = %v, want false", compare["has_more"])
	}

	// --- Snapshot both logs so the merge's effect on them can be checked
	// afterwards (BR-004). ---
	branchUUID := uuid.MustParse(branchID)
	branchEventsBefore := readBranchEvents(t, ctx, st.events, domain.BranchID(branchUUID))
	mainEventsBefore := readBranchEvents(t, ctx, st.events, domain.MainBranchID)

	// The merge replays the branch's genealogy events and not its own lifecycle
	// (ADR-005 §Merge). The two are told apart by stream rather than by a copy of
	// the event-type list: a branch's own stream carries only created/merged/
	// deleted, and every genealogy event sits on its aggregate's stream.
	wantReplayed := 0
	for _, event := range branchEventsBefore {
		if event.StreamID != branchUUID {
			wantReplayed++
		}
	}
	if wantReplayed < 4 {
		t.Fatalf("branch holds %d replayable events, want the 4 edits above", wantReplayed)
	}

	// --- Merge. ---
	merge := mustDo(t, server, http.MethodPost, branchPath+"/merge",
		`{"note":"Confirmed against the parish register"}`, http.StatusOK)
	mergedBranch, _ := merge["branch"].(map[string]any)
	if mergedBranch["status"] != "merged" {
		t.Errorf("branch.status = %v, want merged", mergedBranch["status"])
	}
	if mergedBranch["merged_at"] == nil {
		t.Error("branch.merged_at missing on a merged branch")
	}
	if got := merge["replayed_event_count"]; got != float64(wantReplayed) {
		t.Errorf("replayed_event_count = %v, want %d (every non-lifecycle branch event)", got, wantReplayed)
	}
	if skipped := jsonArray(t, merge, "skipped_stream_ids"); len(skipped) != 0 {
		t.Errorf("skipped_stream_ids = %v, want none", skipped)
	}
	// The registry agrees with the merge response on a re-read.
	if status := getEntity(t, server, branchPath, "")["status"]; status != "merged" {
		t.Errorf("GET branch status = %v, want merged", status)
	}

	// --- The point of the exercise: main now carries every branch edit. The
	// assertions are the same ones that just proved main did NOT have them, so
	// a merge that silently did nothing fails here. ---
	assertBranchEdits(t, server, personPath, namesPath, familyPath, childID, "")

	// --- BR-004, branch side: the merge added the BranchMerged marker to the
	// branch's own log and changed nothing that was already there. ---
	branchEventsAfter := readBranchEvents(t, ctx, st.events, domain.BranchID(branchUUID))
	assertPrefixUnchanged(t, "branch", branchEventsBefore, branchEventsAfter)
	added := branchEventsAfter[len(branchEventsBefore):]
	if len(added) != 1 || added[0].EventType != "BranchMerged" {
		t.Errorf("branch log gained %v, want exactly [BranchMerged]", eventTypes(added))
	}

	// --- BR-004, main side: main's existing events are untouched and the
	// replay appended at new positions only. ---
	mainEventsAfter := readBranchEvents(t, ctx, st.events, domain.MainBranchID)
	assertPrefixUnchanged(t, "main", mainEventsBefore, mainEventsAfter)
	replayed := mainEventsAfter[len(mainEventsBefore):]
	if len(replayed) != wantReplayed {
		t.Errorf("main gained %d events, want %d", len(replayed), wantReplayed)
	}
	if len(mainEventsBefore) > 0 && len(replayed) > 0 {
		lastOld := mainEventsBefore[len(mainEventsBefore)-1].Position
		if replayed[0].Position <= lastOld {
			t.Errorf("replay wrote at position %d, at or before main's pre-merge head %d — not append-only",
				replayed[0].Position, lastOld)
		}
	}

	// --- A merged branch is no longer readable: resolveBranchScope refuses any
	// non-active branch by status, before it ever reaches the read model.
	//
	// Note what this does NOT prove. BR-003 also requires the merge to PURGE the
	// branch's overlay rows, and this 404 is insensitive to that — it would look
	// identical if every overlay row survived. Nor can the purge be observed
	// through the API here: the merge promotes the branch's values onto main, so
	// after it a branch-scoped read and a main read resolve to the same values
	// whether or not the overlay was purged. Purge-on-merge is covered by
	// TestProjector_BranchMergedPurgesOverlay (memory only); proving it on the
	// SQL backends needs a store-level row check this harness does not do. ---
	rec := do(t, server, http.MethodGet, scoped(personPath, branchID), "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("read of the merged branch: status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
	}
}

// assertBranchEdits checks that the given scope shows all four edits. Called
// twice with the same expectations: once on the branch before the merge (where
// they prove the edits landed) and once on main after it (where they prove the
// merge promoted them). branchID is "" for the mainline.
func assertBranchEdits(t *testing.T, server *api.Server, personPath, namesPath, familyPath, childID, branchID string) {
	t.Helper()

	if surname := getEntity(t, server, personPath, branchID)["surname"]; surname != "Revised" {
		t.Errorf("person surname = %v, want Revised", surname)
	}

	names := jsonArray(t, getEntity(t, server, namesPath, branchID), "items")
	if given := entryField(t, names, "given_name"); !contains(given, "Alexandra") {
		t.Errorf("person names = %v, want the added Alexandra", given)
	}

	family := getEntity(t, server, familyPath, branchID)
	if place := family["marriage_place"]; place != "Springfield" {
		t.Errorf("family marriage_place = %v, want Springfield", place)
	}
	if linked := entryField(t, jsonArray(t, family, "children"), "person_id"); !contains(linked, childID) {
		t.Errorf("family children = %v, want the linked child %s", linked, childID)
	}
}

// assertMainUnedited checks that the mainline still holds exactly what it was
// seeded with — the branch edits have not leaked.
func assertMainUnedited(t *testing.T, server *api.Server, personPath, namesPath, familyPath string) {
	t.Helper()

	if surname := getEntity(t, server, personPath, "")["surname"]; surname != "Original" {
		t.Errorf("main person surname = %v, want Original — the branch edit leaked", surname)
	}
	if names := jsonArray(t, getEntity(t, server, namesPath, ""), "items"); len(names) != 1 {
		t.Errorf("main person names = %d, want 1 — the branch name leaked", len(names))
	}

	family := getEntity(t, server, familyPath, "")
	if place, ok := family["marriage_place"]; ok && place != "" {
		t.Errorf("main family marriage_place = %v, want unset — the branch edit leaked", place)
	}
	if children := optionalArray(t, family, "children"); len(children) != 0 {
		t.Errorf("main family children = %d, want 0 — the branch link leaked", len(children))
	}
}

// readBranchEvents reads one branch's own events from the start of the log.
// The limit is generous: this scenario writes a couple of dozen events, so a
// truncated read would mean something is badly wrong.
func readBranchEvents(t *testing.T, ctx context.Context, eventStore repository.EventStore, branchID domain.BranchID) []repository.StoredEvent {
	t.Helper()
	events, err := eventStore.ReadBranch(ctx, branchID, 0, 1000)
	if err != nil {
		t.Fatalf("ReadBranch(%s): %v", branchID, err)
	}
	return events
}

// assertPrefixUnchanged checks that after is before plus new events at the end —
// the append-only guarantee (BR-004/ES-002). Identity is position + version +
// event type: a rewritten event would change at least one of them.
func assertPrefixUnchanged(t *testing.T, side string, before, after []repository.StoredEvent) {
	t.Helper()
	if len(after) < len(before) {
		t.Fatalf("%s log shrank from %d to %d events — the log is append-only", side, len(before), len(after))
	}
	for i, want := range before {
		got := after[i]
		if got.ID != want.ID || got.Position != want.Position || got.Version != want.Version || got.EventType != want.EventType {
			t.Fatalf("%s log entry %d changed: %s@pos%d/v%d -> %s@pos%d/v%d",
				side, i, want.EventType, want.Position, want.Version, got.EventType, got.Position, got.Version)
		}
	}
}

// eventTypes lists the event types of a slice, for failure messages.
func eventTypes(events []repository.StoredEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.EventType)
	}
	return types
}
