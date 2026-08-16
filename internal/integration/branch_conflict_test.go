package integration_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// Merge-conflict detection and resolution, driven through the real HTTP API
// against every storage backend (issue #671).
//
// Conflict classification and resolution are well covered in memory
// (internal/query/merge_conflicts_test.go, internal/command/branch_merge_commands_test.go,
// internal/api/branch_handlers_test.go). What was never covered is the same
// path over a real SQL backend, which is what these scenarios add: the events a
// conflict is computed from are read back out of the store, so a backend that
// mis-orders, mis-scopes or mis-decodes them produces a different verdict here
// and nowhere else.
//
// Each scenario body is written ONCE and run against every entry in `backends`.
//
// Fixtures use neutral placeholder names only (public repo - no real PII).

// forEachBackend runs one scenario body against every backend in the harness's
// table. It exists so a scenario declares only its own steps; adding a backend
// to `backends` still adds it to every scenario at once.
//
// Use this when the scenario only needs the HTTP surface. A scenario that also
// needs the store bundle — to read the event log directly, say — iterates
// `backends` itself instead; TestBranchLifecycle_EndToEnd is the example.
func forEachBackend(t *testing.T, run func(t *testing.T, server *api.Server)) {
	t.Helper()
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			run(t, newServer(t, backend.setup(t)))
		})
	}
}

// ============================================================================
// edit/edit
// ============================================================================

// TestBranchConflict_EditEdit covers the ordinary disagreement: the branch and
// main both set the same field of the same person to different values.
//
// It asserts BOTH resolution directions. They need two branches rather than one,
// because a merge is terminal (ADR-005) - there is no un-merge to run the same
// conflict the other way. The two fork from the same point, before main moves,
// so each carries the same disagreement.
func TestBranchConflict_EditEdit(t *testing.T) {
	forEachBackend(t, runEditEditConflict)
}

func runEditEditConflict(t *testing.T, server *api.Server) {
	t.Helper()

	personID := createPerson(t, server, "Robin", "Original")
	personPath := "/api/v1/persons/" + personID

	branchWins := createBranch(t, server, "surname-branch-wins")
	branchLoses := createBranch(t, server, "surname-main-wins")

	updateSurname(t, server, personPath, branchWins, "Branchside")
	updateSurname(t, server, personPath, branchLoses, "Discarded")

	// Main's edit lands BEFORE any compare or merge. PlanMerge pins main's
	// stream versions between reading the branch side and the main side
	// (internal/query/merge_conflicts.go), so an edit arriving after the plan is
	// a stale-plan refusal rather than a conflict - a different scenario.
	updateSurname(t, server, personPath, "", "Mainside")

	// --- Detection: compare names the contested field and offers both sides. ---
	conflict := conflictFor(t,
		mustDo(t, server, http.MethodGet, comparePath(branchWins), "", http.StatusOK), personID)
	if kind := conflict["kind"]; kind != "edit_edit" {
		t.Errorf("conflict kind = %v, want edit_edit", kind)
	}
	if entityType := conflict["entity_type"]; entityType != "person" {
		t.Errorf("conflict entity_type = %v, want person", entityType)
	}
	if fields := stringValues(t, conflict, "fields"); !contains(fields, "surname") {
		t.Errorf("conflict fields = %v, want the contested surname", fields)
	}
	supported := stringValues(t, conflict, "supported_resolutions")
	if !contains(supported, "branch") || !contains(supported, "main") {
		t.Errorf("supported_resolutions = %v, want both sides for an edit_edit", supported)
	}

	// --- Refusal: an undecided conflict blocks the merge and writes nothing. ---
	rec := do(t, server, http.MethodPost, mergePath(branchWins), `{"note":"no decision made"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("merge with no resolution: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
	refusal := decodeJSON(t, rec)
	if code := refusal["code"]; code != "merge_conflicts" {
		t.Errorf("refusal code = %v, want merge_conflicts", code)
	}
	// The whole conflict list travels with the refusal, not just a count.
	conflictFor(t, refusal, personID)
	if got := personSurname(t, server, personID, ""); got != "Mainside" {
		t.Errorf("main surname = %q after a refused merge, want Mainside - the refusal wrote something", got)
	}

	// --- Resolution, branch side: main ends up with the branch's value. ---
	merged := mustDo(t, server, http.MethodPost, mergePath(branchWins),
		mergeBody("branch confirmed against the register", resolution(personID, "branch")), http.StatusOK)
	if skipped := jsonArray(t, merged, "skipped_stream_ids"); len(skipped) != 0 {
		t.Errorf("skipped_stream_ids = %v, want none - the stream was resolved to the branch", skipped)
	}
	if got := personSurname(t, server, personID, ""); got != "Branchside" {
		t.Errorf("main surname = %q, want Branchside - the branch resolution was not honored", got)
	}

	// --- Resolution, main side: the second branch's identical disagreement,
	// decided the other way. Main has since moved to Branchside, so this is the
	// same edit_edit shape with a different main value. ---
	second := conflictFor(t,
		mustDo(t, server, http.MethodGet, comparePath(branchLoses), "", http.StatusOK), personID)
	if kind := second["kind"]; kind != "edit_edit" {
		t.Errorf("second branch conflict kind = %v, want edit_edit", kind)
	}

	kept := mustDo(t, server, http.MethodPost, mergePath(branchLoses),
		mergeBody("main's reading stands", resolution(personID, "main")), http.StatusOK)
	if count := kept["replayed_event_count"]; count != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0 - the only stream was resolved to main", count)
	}
	if skipped := stringValues(t, kept, "skipped_stream_ids"); !contains(skipped, personID) {
		t.Errorf("skipped_stream_ids = %v, want the person %s", skipped, personID)
	}
	if got := personSurname(t, server, personID, ""); got != "Branchside" {
		t.Errorf("main surname = %q, want Branchside - the main resolution did not keep main's value", got)
	}
}

// ============================================================================
// delete/edit
// ============================================================================

// TestBranchConflict_DeleteEdit covers the asymmetric conflict: main deleted the
// entity the branch went on editing.
//
// Only "main" is offered, because replaying the branch's edits onto an absent
// read-model row is a no-op - a 200 there would report a merge that did the
// opposite of what the caller chose. So the branch side is refused outright
// (400 invalid_resolution) rather than honored hollowly.
func TestBranchConflict_DeleteEdit(t *testing.T) {
	forEachBackend(t, runDeleteEditConflict)
}

func runDeleteEditConflict(t *testing.T, server *api.Server) {
	t.Helper()

	personID := createPerson(t, server, "Jordan", "Doomed")
	personPath := "/api/v1/persons/" + personID
	branchID := createBranch(t, server, "still-editing")
	branchPath := "/api/v1/branches/" + branchID

	updateSurname(t, server, personPath, branchID, "Revised")

	// Main deletes the person the branch is still working on. The person belongs
	// to no family, so this is not refused as a linked-person delete.
	mustDo(t, server, http.MethodDelete, personPath, "", http.StatusNoContent)

	// --- Detection: main as the ONLY supported resolution. ---
	conflict := conflictFor(t,
		mustDo(t, server, http.MethodGet, comparePath(branchID), "", http.StatusOK), personID)
	if kind := conflict["kind"]; kind != "delete_edit" {
		t.Errorf("conflict kind = %v, want delete_edit", kind)
	}
	supported := stringValues(t, conflict, "supported_resolutions")
	if len(supported) != 1 || supported[0] != "main" {
		t.Errorf("supported_resolutions = %v, want exactly [main] - the branch side cannot be honored", supported)
	}

	// --- The unsupported side is rejected before anything is written. ---
	rec := do(t, server, http.MethodPost, mergePath(branchID),
		mergeBody("keep my edits", resolution(personID, "branch")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("merge resolved to branch: status = %d, want 400. Body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeJSON(t, rec)["code"]; code != "invalid_resolution" {
		t.Errorf("rejection code = %v, want invalid_resolution", code)
	}
	if status := getEntity(t, server, branchPath, "")["status"]; status != "active" {
		t.Errorf("branch status = %v after a rejected merge, want active - the rejection was not free", status)
	}

	// --- The supported side merges, skipping the stream. ---
	merged := mustDo(t, server, http.MethodPost, mergePath(branchID),
		mergeBody("accept main's deletion", resolution(personID, "main")), http.StatusOK)
	if count := merged["replayed_event_count"]; count != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0 - the only stream was resolved to main", count)
	}
	if skipped := stringValues(t, merged, "skipped_stream_ids"); !contains(skipped, personID) {
		t.Errorf("skipped_stream_ids = %v, want the person %s", skipped, personID)
	}
	// The merge did not resurrect what main deleted.
	if rec := do(t, server, http.MethodGet, personPath, ""); rec.Code != http.StatusNotFound {
		t.Errorf("main person after the merge: status = %d, want 404 - the merge resurrected a deleted entity", rec.Code)
	}
}

// ============================================================================
// relationship divergence
// ============================================================================

// TestBranchConflict_Relationship covers structural divergence: the branch says
// a child belongs to a family and main says they do not.
//
// There is no fourth conflict kind for this. A link/unlink disagreement is an
// edit_edit keyed on the relationship rather than on a column name, encoded as
// `children[<person uuid>]` (query.childFieldKey), and this is the only place
// that encoding is exercised end to end - through the API, on a real database,
// where the two ChildLinked/ChildUnlinked events have to come back off the
// family's stream in order for the two sides to compare at all.
func TestBranchConflict_Relationship(t *testing.T) {
	forEachBackend(t, runRelationshipConflict)
}

func runRelationshipConflict(t *testing.T, server *api.Server) {
	t.Helper()

	partner1 := createPerson(t, server, "Morgan", "Household")
	partner2 := createPerson(t, server, "Riley", "Household")
	childID := createPerson(t, server, "Quinn", "Household")
	familyID := createFamily(t, server, partner1, partner2)

	familyPath := "/api/v1/families/" + familyID
	childrenPath := familyPath + "/children"

	branchID := createBranch(t, server, "child-belongs-here")

	// The branch's finding: this child belongs to this family.
	mustDo(t, server, http.MethodPost, scoped(childrenPath, branchID),
		fmt.Sprintf(`{"person_id":%q}`, childID), http.StatusCreated)

	// Main reaches the opposite conclusion, in the two steps the API requires:
	// ChildUnlinkedFromFamily is only accepted for a child the family actually
	// has, and a pre-fork link would sit before the base position where neither
	// side compares it. Two events, one net effect - the classifier compares each
	// side's FINAL value for a field, so main asserts "unlinked" against the
	// branch's "linked".
	mustDo(t, server, http.MethodPost, childrenPath,
		fmt.Sprintf(`{"person_id":%q}`, childID), http.StatusCreated)
	mustDo(t, server, http.MethodDelete, childrenPath+"/"+childID, "", http.StatusNoContent)

	// --- Detection: the conflict is on the FAMILY, named by the child's id. ---
	conflict := conflictFor(t,
		mustDo(t, server, http.MethodGet, comparePath(branchID), "", http.StatusOK), familyID)
	if kind := conflict["kind"]; kind != "edit_edit" {
		t.Errorf("conflict kind = %v, want edit_edit - a link/unlink divergence is an edit_edit on the relationship", kind)
	}
	if entityType := conflict["entity_type"]; entityType != "family" {
		t.Errorf("conflict entity_type = %v, want family", entityType)
	}
	wantField := "children[" + childID + "]"
	if fields := stringValues(t, conflict, "fields"); !contains(fields, wantField) {
		t.Errorf("conflict fields = %v, want the relationship key %s", fields, wantField)
	}

	// --- Undecided, it blocks the merge like any other conflict. ---
	rec := do(t, server, http.MethodPost, mergePath(branchID), `{"note":"no decision made"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("merge with no resolution: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeJSON(t, rec)["code"]; code != "merge_conflicts" {
		t.Errorf("refusal code = %v, want merge_conflicts", code)
	}

	// --- Resolved to the branch, main gains the link it had dropped. ---
	merged := mustDo(t, server, http.MethodPost, mergePath(branchID),
		mergeBody("the parish register lists this child", resolution(familyID, "branch")), http.StatusOK)
	if skipped := jsonArray(t, merged, "skipped_stream_ids"); len(skipped) != 0 {
		t.Errorf("skipped_stream_ids = %v, want none - the family was resolved to the branch", skipped)
	}

	family := getEntity(t, server, familyPath, "")
	linked := entryField(t, optionalArray(t, family, "children"), "person_id")
	if !contains(linked, childID) {
		t.Errorf("main family children = %v, want the child %s - the branch resolution was not honored", linked, childID)
	}
}

// ============================================================================
// Shared step and assertion helpers
// ============================================================================

func comparePath(branchID string) string { return "/api/v1/branches/" + branchID + "/compare" }
func mergePath(branchID string) string   { return "/api/v1/branches/" + branchID + "/merge" }

// updateSurname renames a person on the given scope, reading the version from
// that same scope - a branch row's version diverges from main's as soon as the
// branch edits it. branchID is "" for the mainline.
func updateSurname(t *testing.T, server *api.Server, personPath, branchID, surname string) {
	t.Helper()
	mustDo(t, server, http.MethodPut, scoped(personPath, branchID),
		fmt.Sprintf(`{"surname":%q,"version":%d}`, surname, entityVersion(t, server, personPath, branchID)),
		http.StatusOK)
}

// personSurname reads one person's surname on the given scope.
func personSurname(t *testing.T, server *api.Server, personID, branchID string) string {
	t.Helper()
	surname, _ := getEntity(t, server, "/api/v1/persons/"+personID, branchID)["surname"].(string)
	return surname
}

// resolution renders one entry of the merge request's resolutions array.
func resolution(streamID, side string) string {
	return fmt.Sprintf(`{"stream_id":%q,"resolution":%q}`, streamID, side)
}

// mergeBody renders a merge request body carrying a note and zero or more
// resolutions.
func mergeBody(note string, resolutions ...string) string {
	if len(resolutions) == 0 {
		return fmt.Sprintf(`{"note":%q}`, note)
	}
	return fmt.Sprintf(`{"note":%q,"resolutions":[%s]}`, note, strings.Join(resolutions, ","))
}

// conflictFor returns the conflict a compare or a refused merge reported for one
// stream, failing the test when there is none. Both responses carry the same
// `conflicts` array, so one accessor serves both.
func conflictFor(t *testing.T, resp map[string]any, streamID string) map[string]any {
	t.Helper()
	conflicts := jsonArray(t, resp, "conflicts")
	for _, raw := range conflicts {
		entry, _ := raw.(map[string]any)
		if entry["stream_id"] == streamID {
			return entry
		}
	}
	t.Fatalf("no conflict reported for stream %s; conflicts = %v", streamID, conflicts)
	return nil
}

// stringValues reads a required array of strings out of a decoded response.
func stringValues(t *testing.T, resp map[string]any, field string) []string {
	t.Helper()
	raw := jsonArray(t, resp, field)
	values := make([]string, 0, len(raw))
	for _, entry := range raw {
		value, ok := entry.(string)
		if !ok {
			t.Fatalf("%s contains a non-string entry %v", field, entry)
		}
		values = append(values, value)
	}
	return values
}
