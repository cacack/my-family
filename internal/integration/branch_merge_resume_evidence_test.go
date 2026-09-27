package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/command"
)

// TestBranchMergeResume_Evidence drives, on every backend, a resume of a merge
// that carries sources and citations (#758): the resume must continue the
// merge's evidence order and apply its evidence rules.
func TestBranchMergeResume_Evidence(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			t.Run("resumes in evidence order", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeEvidenceOrder(t, newServer(t, wrapped), st, faulty)
			})
			t.Run("main deletes a cited source after the interruption", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeCitedSourceDeletedOnMain(t, newServer(t, wrapped), faulty)
			})
			t.Run("failed count bump is recounted", func(t *testing.T) {
				st := backend.setup(t)
				reads := &faultyReadStore{ReadModelStore: st.read}
				wrapped := st
				wrapped.read = reads
				runResumeRecountsCitationCount(t, newServer(t, wrapped), reads)
			})
			t.Run("cascaded citation is not resurrected", func(t *testing.T) {
				st := backend.setup(t)
				reads := &faultyReadStore{ReadModelStore: st.read}
				wrapped := st
				wrapped.read = reads
				runResumeCascadedCitationStaysGone(t, newServer(t, wrapped), reads)
			})
		})
	}
}

// createSource creates a source on the given scope and returns its id.
func createSource(t *testing.T, server *api.Server, branchID, title string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, scoped("/api/v1/sources", branchID),
		fmt.Sprintf(`{"source_type":"census","title":%q}`, title), http.StatusCreated)
	return mustString(t, resp, "id")
}

// createCitation cites a source for a person's birth on the given scope and
// returns the citation's id.
func createCitation(t *testing.T, server *api.Server, branchID, sourceID, personID string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, scoped("/api/v1/citations", branchID),
		fmt.Sprintf(`{"source_id":%q,"fact_type":"person_birth","fact_owner_id":%q,"page":"12"}`, sourceID, personID),
		http.StatusCreated)
	return mustString(t, resp, "id")
}

// runResumeEvidenceOrder: the branch retitles main's source D, creates a
// source L, re-points main's citation C from D to L, cites L anew (N) and
// deletes D. The branch touched D first, so a first-touch replay would delete
// D — cascading onto C, which main still has citing D — before C's re-point.
// The merge is interrupted after L lands; the resume must replay C and N
// before D's delete, leaving C re-pointed rather than lost.
func runResumeEvidenceOrder(t *testing.T, server *api.Server, st stores, faulty *faultyReplayStore) {
	t.Helper()
	ctx := context.Background()
	person := createPerson(t, server, "Alex", "Original")
	doomed := createSource(t, server, "", "1880 Census")
	mainCit := createCitation(t, server, "", doomed, person)
	branchID := createBranch(t, server, "consolidate-evidence")
	branchPath := "/api/v1/branches/" + branchID
	doomedPath := "/api/v1/sources/" + doomed

	mustDo(t, server, http.MethodPut, scoped(doomedPath, branchID),
		fmt.Sprintf(`{"title":"1880 Census (duplicate)","version":%d}`, entityVersion(t, server, doomedPath, branchID)),
		http.StatusOK)
	later := createSource(t, server, branchID, "1900 Census")

	// The API does not expose re-pointing a citation, so the branch's
	// re-point goes through the command layer on the same stores.
	branch, err := st.branches.Get(ctx, uuid.MustParse(branchID))
	if err != nil {
		t.Fatalf("Get branch failed: %v", err)
	}
	laterID := uuid.MustParse(later)
	handler := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots).WithBranch(branch)
	if _, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:       uuid.MustParse(mainCit),
		SourceID: &laterID,
		Version:  entityVersion(t, server, "/api/v1/citations/"+mainCit, branchID),
	}); err != nil {
		t.Fatalf("branch UpdateCitation failed: %v", err)
	}
	newCit := createCitation(t, server, branchID, later, person)
	mustDo(t, server, http.MethodDelete,
		fmt.Sprintf("%s&version=%d", scoped(doomedPath, branchID), entityVersion(t, server, doomedPath, branchID)),
		"", http.StatusNoContent)

	faulty.arm(2)
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}
	if got := mustString(t, getEntity(t, server, "/api/v1/citations/"+mainCit, ""), "source_id"); got != doomed {
		t.Fatalf("main citation source after the interruption = %s, want still %s", got, doomed)
	}

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if already := entryStrings(t, jsonArray(t, resumed, "already_replayed_stream_ids")); len(already) != 1 || already[0] != later {
		t.Errorf("already_replayed_stream_ids = %v, want [%s]", already, later)
	}
	moved := getEntity(t, server, "/api/v1/citations/"+mainCit, "")
	if got := mustString(t, moved, "source_id"); got != later {
		t.Errorf("main citation source = %s, want %s", got, later)
	}
	if got := mustString(t, moved, "source_title"); got != "1900 Census" {
		t.Errorf("main citation source_title = %q, want %q", got, "1900 Census")
	}
	if got := mustString(t, getEntity(t, server, "/api/v1/citations/"+newCit, ""), "source_title"); got != "1900 Census" {
		t.Errorf("branch citation source_title on main = %q, want %q", got, "1900 Census")
	}
	if got := getEntity(t, server, "/api/v1/sources/"+later, "")["citation_count"]; got != float64(2) {
		t.Errorf("main source citation_count = %v, want 2", got)
	}
	mustDo(t, server, http.MethodGet, doomedPath, "", http.StatusNotFound)

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := again["replayed_event_count"]; got != float64(0) {
		t.Errorf("second resume replayed_event_count = %v, want 0", got)
	}
	if got := jsonArray(t, again, "reprojected_stream_ids"); len(got) != 0 {
		t.Errorf("second resume reprojected_stream_ids = %v, want none", got)
	}
}

// runResumeCitedSourceDeletedOnMain: the branch renames a person and cites
// main's source S; the replay stops after the rename, and main then deletes S
// (allowed: main has no citation of it). Replaying the citation would orphan
// it, so it is pending, "branch" is refused, and "main" rolls forward.
func runResumeCitedSourceDeletedOnMain(t *testing.T, server *api.Server, faulty *faultyReplayStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	cited := createSource(t, server, "", "1880 Census")
	branchID := createBranch(t, server, "cite-then-lose")
	branchPath := "/api/v1/branches/" + branchID
	personPath := "/api/v1/persons/" + person
	mustDo(t, server, http.MethodPut, scoped(personPath, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, personPath, branchID)), http.StatusOK)
	cit := createCitation(t, server, branchID, cited, person)

	faulty.arm(2)
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	sourcePath := "/api/v1/sources/" + cited
	mustDo(t, server, http.MethodDelete,
		fmt.Sprintf("%s?version=%d", sourcePath, entityVersion(t, server, sourcePath, "")), "", http.StatusNoContent)

	refused := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusConflict)
	if pending := entryStrings(t, jsonArray(t, refused, "pending_stream_ids")); len(pending) != 1 || pending[0] != cit {
		t.Fatalf("pending_stream_ids = %v, want [%s]", pending, cit)
	}
	dangling := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"branch"}]}`, cit), http.StatusConflict)
	if dangling["code"] != "merge_dangling_reference" {
		t.Errorf("branch resolution code = %v, want merge_dangling_reference", dangling["code"])
	}
	done := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"main"}]}`, cit), http.StatusOK)
	if skipped := entryStrings(t, jsonArray(t, done, "skipped_stream_ids")); len(skipped) != 1 || skipped[0] != cit {
		t.Errorf("skipped_stream_ids = %v, want [%s]", skipped, cit)
	}
	mustDo(t, server, http.MethodGet, "/api/v1/citations/"+cit, "", http.StatusNotFound)
	if got := mustString(t, getEntity(t, server, personPath, ""), "surname"); got != "Revised" {
		t.Errorf("person surname on main = %q, want the landed rename", got)
	}
}

// runResumeRecountsCitationCount: the branch cites main's source S; the merge
// saves the citation but its projection fails bumping S's citation_count, so
// the citation row is level with the log over a count one short. The resume
// must recount S (reporting the citation as re-projected) without appending,
// and a second resume must find nothing to do.
func runResumeRecountsCitationCount(t *testing.T, server *api.Server, reads *faultyReadStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	cited := createSource(t, server, "", "1880 Census")
	branchID := createBranch(t, server, "count-bump")
	branchPath := "/api/v1/branches/" + branchID
	sourcePath := "/api/v1/sources/" + cited
	cit := createCitation(t, server, branchID, cited, person)

	reads.failSourceCountFor(uuid.MustParse(cited))
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	reads.failSourceCountFor(uuid.Nil)
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}
	mustDo(t, server, http.MethodGet, "/api/v1/citations/"+cit, "", http.StatusOK)
	if got := getEntity(t, server, sourcePath, "")["citation_count"]; got != float64(0) {
		t.Fatalf("citation_count after the failed bump = %v, want 0", got)
	}

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := resumed["replayed_event_count"]; got != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0 (the citation is already in the log)", got)
	}
	if got := entryStrings(t, jsonArray(t, resumed, "reprojected_stream_ids")); len(got) != 1 || got[0] != cit {
		t.Errorf("reprojected_stream_ids = %v, want [%s]", got, cit)
	}
	if got := getEntity(t, server, sourcePath, "")["citation_count"]; got != float64(1) {
		t.Errorf("citation_count = %v, want 1 (recounted)", got)
	}

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := jsonArray(t, again, "reprojected_stream_ids"); len(got) != 0 {
		t.Errorf("second resume reprojected_stream_ids = %v, want none", got)
	}
}

// runResumeCascadedCitationStaysGone: the branch cites main's source S; the
// citation's append lands but its projection fails, so main's read model never
// shows it and main may then delete S. The citation row is missing because S's
// delete cascade would have removed it, which main's log explains — the
// resume must not re-project it into an orphan.
func runResumeCascadedCitationStaysGone(t *testing.T, server *api.Server, reads *faultyReadStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	cited := createSource(t, server, "", "1880 Census")
	branchID := createBranch(t, server, "cite-then-cascade")
	branchPath := "/api/v1/branches/" + branchID
	sourcePath := "/api/v1/sources/" + cited
	cit := createCitation(t, server, branchID, cited, person)

	reads.failCitationFor(uuid.MustParse(cit))
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	reads.failCitationFor(uuid.Nil)
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}
	mustDo(t, server, http.MethodGet, "/api/v1/citations/"+cit, "", http.StatusNotFound)
	mustDo(t, server, http.MethodDelete,
		fmt.Sprintf("%s?version=%d", sourcePath, entityVersion(t, server, sourcePath, "")), "", http.StatusNoContent)

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := resumed["replayed_event_count"]; got != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0", got)
	}
	if got := jsonArray(t, resumed, "reprojected_stream_ids"); len(got) != 0 {
		t.Errorf("reprojected_stream_ids = %v, want none", got)
	}
	mustDo(t, server, http.MethodGet, "/api/v1/citations/"+cit, "", http.StatusNotFound)
	mustDo(t, server, http.MethodGet, sourcePath, "", http.StatusNotFound)
}
