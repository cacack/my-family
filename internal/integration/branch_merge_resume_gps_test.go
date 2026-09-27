package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestBranchMergeResume_GPS drives, on every backend, a resume of a merge
// that carries GPS artifacts (#760 on top of #685): artifacts interrupted
// mid-replay and resumed idempotently, a subject main deletes after the
// interruption, and a research-log projection that failed after its append.
func TestBranchMergeResume_GPS(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			t.Run("resumes GPS artifacts", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeGPSArtifacts(t, newServer(t, wrapped), wrapped, faulty)
			})
			t.Run("main deletes a subject after the interruption", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeGPSSubjectDeletedOnMain(t, newServer(t, wrapped), faulty)
			})
			t.Run("repairs a failed research log projection", func(t *testing.T) {
				st := backend.setup(t)
				reads := &faultyReadStore{ReadModelStore: st.read}
				wrapped := st
				wrapped.read = reads
				runResumeRepairsGPSProjection(t, newServer(t, wrapped), wrapped, reads)
			})
		})
	}
}

func (s *faultyReadStore) failGPSFor(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failGPS = id
}

func (s *faultyReadStore) SaveResearchLog(ctx context.Context, branchID domain.BranchID, log *repository.ResearchLogReadModel) error {
	s.mu.Lock()
	fail := branchID.IsMain() && log.ID == s.failGPS
	s.mu.Unlock()
	if fail {
		return errors.New("injected research log read-model failure during projection")
	}
	return s.ReadModelStore.SaveResearchLog(ctx, branchID, log)
}

// createResearchLog records a negative search about subject on the given scope
// ("" for main) and returns its id.
func createResearchLog(t *testing.T, server *api.Server, branchID, subject, repo string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, scoped("/api/v1/research-logs", branchID),
		fmt.Sprintf(`{"subject_id":%q,"subject_type":"person","repository":%q,"search_description":"Baptisms","outcome":"not_found","search_date":"2024-03-01T00:00:00Z"}`,
			subject, repo), http.StatusCreated)
	return mustString(t, resp, "id")
}

// runResumeGPSArtifacts: the branch edits main's evidence analysis, then
// records a research log and a proof summary. The replay stops after the
// analysis edit; the resume replays the rest without touching the analysis
// again, and a second resume changes nothing.
func runResumeGPSArtifacts(t *testing.T, server *api.Server, st stores, faulty *faultyReplayStore) {
	t.Helper()
	ctx := context.Background()
	person := createPerson(t, server, "Ada", "Lovelace")
	analysis := mustString(t, mustDo(t, server, http.MethodPost, "/api/v1/evidence-analyses",
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1815"}`, person), http.StatusCreated), "id")
	branchID := createBranch(t, server, "gps-work")
	branchPath := "/api/v1/branches/" + branchID
	analysisPath := "/api/v1/evidence-analyses/" + analysis
	mustDo(t, server, http.MethodPut, scoped(analysisPath, branchID),
		fmt.Sprintf(`{"conclusion":"Born 1815, London","version":%d}`, entityVersion(t, server, analysisPath, branchID)), http.StatusOK)
	logID := createResearchLog(t, server, branchID, person, "Parish chest")
	proof := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/proof-summaries", branchID),
		fmt.Sprintf(`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1815","argument":"The register agrees"}`, person),
		http.StatusCreated), "id")

	faulty.arm(2)
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}
	if got := mustString(t, getEntity(t, server, analysisPath, ""), "conclusion"); got != "Born 1815, London" {
		t.Fatalf("main analysis after the interruption = %q, want the landed edit", got)
	}
	mustDo(t, server, http.MethodGet, "/api/v1/research-logs/"+logID, "", http.StatusNotFound)
	analysisBefore := mainEventCount(t, ctx, st.events, analysis)

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if already := entryStrings(t, jsonArray(t, resumed, "already_replayed_stream_ids")); len(already) != 1 || already[0] != analysis {
		t.Errorf("already_replayed_stream_ids = %v, want [%s]", already, analysis)
	}
	if got := resumed["replayed_event_count"]; got != float64(2) {
		t.Errorf("replayed_event_count = %v, want 2 (the log and the proof)", got)
	}
	if got := mustString(t, getEntity(t, server, "/api/v1/research-logs/"+logID, ""), "repository"); got != "Parish chest" {
		t.Errorf("main research log repository = %q, want the branch's", got)
	}
	mustDo(t, server, http.MethodGet, "/api/v1/proof-summaries/"+proof, "", http.StatusOK)
	if got := mainEventCount(t, ctx, st.events, analysis); got != analysisBefore {
		t.Errorf("analysis has %d main events after resume, want %d (no duplicate replay)", got, analysisBefore)
	}

	mainBefore := readBranchEvents(t, ctx, st.events, domain.MainBranchID)
	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := again["replayed_event_count"]; got != float64(0) {
		t.Errorf("second resume replayed_event_count = %v, want 0", got)
	}
	if got := jsonArray(t, again, "reprojected_stream_ids"); len(got) != 0 {
		t.Errorf("second resume reprojected_stream_ids = %v, want none", got)
	}
	if after := readBranchEvents(t, ctx, st.events, domain.MainBranchID); len(after) != len(mainBefore) {
		t.Errorf("main gained %d events on a no-op resume", len(after)-len(mainBefore))
	}
}

// runResumeGPSSubjectDeletedOnMain: the branch renames a person and logs
// research about main's person O; the replay stops after the rename, and main
// then deletes O. Replaying the log would leave research about nothing, so it
// is pending, "branch" is refused, and "main" rolls forward without it.
func runResumeGPSSubjectDeletedOnMain(t *testing.T, server *api.Server, faulty *faultyReplayStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	subject := createPerson(t, server, "Owen", "Subject")
	branchID := createBranch(t, server, "subject-lost")
	branchPath := "/api/v1/branches/" + branchID
	personPath := "/api/v1/persons/" + person
	mustDo(t, server, http.MethodPut, scoped(personPath, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, personPath, branchID)), http.StatusOK)
	logID := createResearchLog(t, server, branchID, subject, "County archive")

	faulty.arm(2)
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	mustDo(t, server, http.MethodDelete, "/api/v1/persons/"+subject, "", http.StatusNoContent)

	refused := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusConflict)
	if pending := entryStrings(t, jsonArray(t, refused, "pending_stream_ids")); len(pending) != 1 || pending[0] != logID {
		t.Fatalf("pending_stream_ids = %v, want [%s]", pending, logID)
	}
	dangling := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"branch"}]}`, logID), http.StatusConflict)
	if dangling["code"] != "merge_dangling_reference" {
		t.Errorf("branch resolution code = %v, want merge_dangling_reference", dangling["code"])
	}
	done := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"main"}]}`, logID), http.StatusOK)
	if skipped := entryStrings(t, jsonArray(t, done, "skipped_stream_ids")); len(skipped) != 1 || skipped[0] != logID {
		t.Errorf("skipped_stream_ids = %v, want [%s]", skipped, logID)
	}
	mustDo(t, server, http.MethodGet, "/api/v1/research-logs/"+logID, "", http.StatusNotFound)
	if got := mustString(t, getEntity(t, server, personPath, ""), "surname"); got != "Revised" {
		t.Errorf("person surname on main = %q, want the landed rename", got)
	}

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := again["replayed_event_count"]; got != float64(0) {
		t.Errorf("second resume replayed_event_count = %v, want 0", got)
	}
}

// runResumeRepairsGPSProjection: the branch records a research log; the merge
// appends it but its main projection fails. The resume re-projects it from
// main's own log without appending, and a second resume finds nothing to do.
func runResumeRepairsGPSProjection(t *testing.T, server *api.Server, st stores, reads *faultyReadStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	branchID := createBranch(t, server, "log-line")
	branchPath := "/api/v1/branches/" + branchID
	logID := createResearchLog(t, server, branchID, person, "Parish chest")

	reads.failGPSFor(uuid.MustParse(logID))
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	reads.failGPSFor(uuid.Nil)
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}
	mustDo(t, server, http.MethodGet, "/api/v1/research-logs/"+logID, "", http.StatusNotFound)
	logBefore := mainEventCount(t, context.Background(), st.events, logID)

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := resumed["replayed_event_count"]; got != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0 (the log is already in the event log)", got)
	}
	if got := entryStrings(t, jsonArray(t, resumed, "reprojected_stream_ids")); len(got) != 1 || got[0] != logID {
		t.Errorf("reprojected_stream_ids = %v, want [%s]", got, logID)
	}
	if got := mainEventCount(t, context.Background(), st.events, logID); got != logBefore {
		t.Errorf("log has %d main events after resume, want %d (no re-append)", got, logBefore)
	}
	if got := mustString(t, getEntity(t, server, "/api/v1/research-logs/"+logID, ""), "repository"); got != "Parish chest" {
		t.Errorf("main research log repository = %q, want it re-projected", got)
	}

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := jsonArray(t, again, "reprojected_stream_ids"); len(got) != 0 {
		t.Errorf("second resume reprojected_stream_ids = %v, want none", got)
	}
}
