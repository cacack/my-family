package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// faultyReplayStore wraps a backend's event store and, once armed, fails the
// Nth mainline Append. A merge's claim is written on the branch's own scope, so
// counting only mainline appends made after arming counts exactly the replay.
type faultyReplayStore struct {
	repository.EventStore
	mu          sync.Mutex
	armed       bool
	failAt      int
	mainAppends int
}

var errInjectedReplayFailure = errors.New("injected storage failure during replay")

func (s *faultyReplayStore) arm(failAt int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.armed, s.failAt, s.mainAppends = true, failAt, 0
}

func (s *faultyReplayStore) disarm() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.armed = false
}

func (s *faultyReplayStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	s.mu.Lock()
	fail := false
	if s.armed && scope.BranchID.IsMain() {
		s.mainAppends++
		fail = s.mainAppends == s.failAt
	}
	s.mu.Unlock()
	if fail {
		return errInjectedReplayFailure
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// TestBranchMergeResume_EndToEnd is #685's acceptance scenario against every
// backend: a merge whose replay fails after its first entity is resumed to
// completion over HTTP, without duplicating the entity that already landed,
// and a second resume is a no-op. The fault is injected at the event store, so
// the SQL backends' real transactional Append is what bounds the failure.
func TestBranchMergeResume_EndToEnd(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			faulty := &faultyReplayStore{EventStore: st.events}
			wrapped := st
			wrapped.events = faulty
			runBranchMergeResume(t, newServer(t, wrapped), st.events, faulty)
		})
	}
}

func runBranchMergeResume(t *testing.T, server *api.Server, events repository.EventStore, faulty *faultyReplayStore) {
	t.Helper()
	ctx := context.Background()

	// Two mainline persons, both renamed on a branch: two streams to replay.
	first := createPerson(t, server, "Alex", "Original")
	second := createPerson(t, server, "Sam", "Steady")
	branchID := createBranch(t, server, "resume-line")
	branchPath := "/api/v1/branches/" + branchID
	for _, id := range []string{first, second} {
		path := "/api/v1/persons/" + id
		mustDo(t, server, http.MethodPut, scoped(path, branchID),
			fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, path, branchID)),
			http.StatusOK)
	}

	// --- The merge's replay fails on its second entity. ---
	faulty.arm(2)
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}

	// Exactly one entity landed; which one is the replay's order, so find it.
	surnames := map[string]string{
		first:  mustString(t, getEntity(t, server, "/api/v1/persons/"+first, ""), "surname"),
		second: mustString(t, getEntity(t, server, "/api/v1/persons/"+second, ""), "surname"),
	}
	landed, pending := first, second
	if surnames[second] == "Revised" {
		landed, pending = second, first
	}
	if surnames[landed] != "Revised" || surnames[pending] == "Revised" {
		t.Fatalf("after the interrupted merge surnames = %v, want exactly one replayed", surnames)
	}
	landedBefore := mainEventCount(t, ctx, events, landed)

	// A plain merge retry is refused: the branch is terminal.
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusConflict)

	// --- Resume finishes the replay. ---
	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := resumed["replayed_event_count"]; got != float64(1) {
		t.Errorf("replayed_event_count = %v, want 1 (only the entity that had not landed)", got)
	}
	if already := entryStrings(t, jsonArray(t, resumed, "already_replayed_stream_ids")); len(already) != 1 || already[0] != landed {
		t.Errorf("already_replayed_stream_ids = %v, want [%s]", already, landed)
	}
	if skipped := jsonArray(t, resumed, "skipped_stream_ids"); len(skipped) != 0 {
		t.Errorf("skipped_stream_ids = %v, want none", skipped)
	}
	for _, id := range []string{first, second} {
		if got := mustString(t, getEntity(t, server, "/api/v1/persons/"+id, ""), "surname"); got != "Revised" {
			t.Errorf("person %s surname on main = %q, want Revised", id, got)
		}
	}
	if got := mainEventCount(t, ctx, events, landed); got != landedBefore {
		t.Errorf("landed entity has %d main events after resume, want %d (no duplicate replay)", got, landedBefore)
	}

	// --- A second resume is a no-op, down to the log. ---
	mainBefore := readBranchEvents(t, ctx, events, domain.MainBranchID)
	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", `{}`, http.StatusOK)
	if got := again["replayed_event_count"]; got != float64(0) {
		t.Errorf("second resume replayed_event_count = %v, want 0", got)
	}
	if already := jsonArray(t, again, "already_replayed_stream_ids"); len(already) != 2 {
		t.Errorf("second resume already_replayed_stream_ids = %v, want both entities", already)
	}
	assertPrefixUnchanged(t, "main", mainBefore, readBranchEvents(t, ctx, events, domain.MainBranchID))
	if after := readBranchEvents(t, ctx, events, domain.MainBranchID); len(after) != len(mainBefore) {
		t.Errorf("main gained %d events on a no-op resume", len(after)-len(mainBefore))
	}
}

// mainEventCount is how many mainline events one entity's stream holds.
func mainEventCount(t *testing.T, ctx context.Context, events repository.EventStore, id string) int {
	t.Helper()
	all, err := events.ReadStream(ctx, uuid.MustParse(id))
	if err != nil {
		t.Fatalf("ReadStream(%s): %v", id, err)
	}
	count := 0
	for _, evt := range all {
		if evt.BranchID.IsMain() {
			count++
		}
	}
	return count
}

// entryStrings converts a decoded JSON array of strings.
func entryStrings(t *testing.T, values []any) []string {
	t.Helper()
	out := make([]string, 0, len(values))
	for i, raw := range values {
		value, ok := raw.(string)
		if !ok {
			t.Fatalf("entry %d = %v, want a string", i, raw)
		}
		out = append(out, value)
	}
	return out
}

// faultyReadStore wraps a backend's read-model store and, while armed, fails
// mainline SavePerson for one person — so a replay's Append commits to the log
// and its synchronous projection does not. For evidence (#758) it can instead
// fail one citation's own save (failCitation), or only a save of one source
// that changes its citation count (failSourceCount) — the count bump a
// citation projection makes after saving the citation. For media (#759) it can
// fail one media item's mainline save (failMedia).
type faultyReadStore struct {
	repository.ReadModelStore
	mu              sync.Mutex
	failPerson      uuid.UUID
	failCitation    uuid.UUID
	failSourceCount uuid.UUID
	failMedia       uuid.UUID
}

func (s *faultyReadStore) failFor(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failPerson = id
}

func (s *faultyReadStore) SavePerson(ctx context.Context, branchID domain.BranchID, person *repository.PersonReadModel) error {
	s.mu.Lock()
	fail := branchID.IsMain() && person.ID == s.failPerson
	s.mu.Unlock()
	if fail {
		return errors.New("injected read-model failure during projection")
	}
	return s.ReadModelStore.SavePerson(ctx, branchID, person)
}

func (s *faultyReadStore) failCitationFor(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCitation = id
}

func (s *faultyReadStore) failSourceCountFor(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failSourceCount = id
}

func (s *faultyReadStore) SaveCitation(ctx context.Context, branchID domain.BranchID, citation *repository.CitationReadModel) error {
	s.mu.Lock()
	fail := branchID.IsMain() && citation.ID == s.failCitation
	s.mu.Unlock()
	if fail {
		return errors.New("injected citation read-model failure during projection")
	}
	return s.ReadModelStore.SaveCitation(ctx, branchID, citation)
}

func (s *faultyReadStore) SaveSource(ctx context.Context, branchID domain.BranchID, source *repository.SourceReadModel) error {
	s.mu.Lock()
	watch := branchID.IsMain() && source.ID == s.failSourceCount
	s.mu.Unlock()
	if watch {
		current, err := s.GetSource(ctx, branchID, source.ID)
		if err != nil {
			return err
		}
		if current != nil && current.CitationCount != source.CitationCount {
			return errors.New("injected source count read-model failure during projection")
		}
	}
	return s.ReadModelStore.SaveSource(ctx, branchID, source)
}

// TestBranchMergeResume_DecisionsAndProjection covers, on every backend, the
// ways a resume used to leave a merge unfinished: a "main" decision that a
// later resume could reverse, a family that could never replay because main
// deleted the child it links, and a replay whose projection failed after its
// Append committed.
func TestBranchMergeResume_DecisionsAndProjection(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			t.Run("main decision is final", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeMainDecisionIsFinal(t, newServer(t, wrapped), faulty)
			})
			t.Run("dangling family is rolled forward", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeRollsForwardDanglingFamily(t, newServer(t, wrapped), faulty)
			})
			t.Run("merged-away edited child is pending", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeMergedAwayEditedChild(t, newServer(t, wrapped), faulty)
			})
			t.Run("failed projection is repaired", func(t *testing.T) {
				st := backend.setup(t)
				reads := &faultyReadStore{ReadModelStore: st.read}
				wrapped := st
				wrapped.read = reads
				runResumeRepairsProjection(t, newServer(t, wrapped), st.events, reads)
			})
		})
	}
}

func runResumeMainDecisionIsFinal(t *testing.T, server *api.Server, faulty *faultyReplayStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	branchID := createBranch(t, server, "rejected-line")
	branchPath := "/api/v1/branches/" + branchID
	path := "/api/v1/persons/" + person
	mustDo(t, server, http.MethodPut, scoped(path, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, path, branchID)), http.StatusOK)

	faulty.arm(1)
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	mustDo(t, server, http.MethodPut, path,
		fmt.Sprintf(`{"given_name":"Alexa","version":%d}`, entityVersion(t, server, path, "")), http.StatusOK)

	mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"main"}]}`, person), http.StatusOK)

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := again["replayed_event_count"]; got != float64(0) {
		t.Errorf("bare resume replayed_event_count = %v, want 0", got)
	}
	mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"branch"}]}`, person), http.StatusBadRequest)
	if got := mustString(t, getEntity(t, server, path, ""), "surname"); got != "Original" {
		t.Errorf("surname on main = %q, want main's decision to stand", got)
	}
}

// runResumeRollsForwardDanglingFamily: the branch renames a partner and links
// main's person P into main's family F; the replay stops after the partner
// lands, and main then deletes P (allowed: P is not linked on main). F is
// reported pending, and a "main" resolution finishes the merge without a
// phantom child.
func runResumeRollsForwardDanglingFamily(t *testing.T, server *api.Server, faulty *faultyReplayStore) {
	t.Helper()
	partner := createPerson(t, server, "Alex", "Original")
	other := createPerson(t, server, "Robin", "Other")
	child := createPerson(t, server, "Casey", "Child")
	family := createFamily(t, server, partner, other)
	branchID := createBranch(t, server, "dangling-child")
	branchPath := "/api/v1/branches/" + branchID
	partnerPath := "/api/v1/persons/" + partner
	mustDo(t, server, http.MethodPut, scoped(partnerPath, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, partnerPath, branchID)), http.StatusOK)
	mustDo(t, server, http.MethodPost, scoped("/api/v1/families/"+family+"/children", branchID),
		fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)

	faulty.arm(2)
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	childPath := "/api/v1/persons/" + child
	mustDo(t, server, http.MethodDelete,
		fmt.Sprintf("%s?version=%d", childPath, entityVersion(t, server, childPath, "")), "", http.StatusNoContent)

	refused := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusConflict)
	if pending := entryStrings(t, refused["pending_stream_ids"].([]any)); len(pending) != 1 || pending[0] != family {
		t.Fatalf("pending_stream_ids = %v, want [%s]", pending, family)
	}
	done := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"main"}]}`, family), http.StatusOK)
	if skipped := entryStrings(t, done["skipped_stream_ids"].([]any)); len(skipped) != 1 || skipped[0] != family {
		t.Errorf("skipped_stream_ids = %v, want [%s]", skipped, family)
	}
	if got := mustString(t, getEntity(t, server, partnerPath, ""), "surname"); got != "Revised" {
		t.Errorf("partner surname on main = %q, want the landed rename", got)
	}
	if children, _ := getEntity(t, server, "/api/v1/families/"+family, "")["children"].([]any); len(children) != 0 {
		t.Errorf("main family children = %v, want no phantom child", children)
	}
}

// runResumeMergedAwayEditedChild: the branch edits main's person P and links
// P into main's family F; the replay fails before anything lands, and main then
// merges P into another person. That does not write to P's stream, so the plan
// still pins it — but replaying P's edit restores nothing, so both P and F are
// pending, "branch" is refused for P, and rolling forward leaves no phantom
// child.
func runResumeMergedAwayEditedChild(t *testing.T, server *api.Server, faulty *faultyReplayStore) {
	t.Helper()
	partner := createPerson(t, server, "Alex", "Original")
	other := createPerson(t, server, "Robin", "Other")
	child := createPerson(t, server, "Casey", "Child")
	survivor := createPerson(t, server, "Casey", "Survivor")
	family := createFamily(t, server, partner, other)
	branchID := createBranch(t, server, "merged-away-child")
	branchPath := "/api/v1/branches/" + branchID
	childPath := "/api/v1/persons/" + child
	mustDo(t, server, http.MethodPut, scoped(childPath, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, childPath, branchID)), http.StatusOK)
	mustDo(t, server, http.MethodPost, scoped("/api/v1/families/"+family+"/children", branchID),
		fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)

	faulty.arm(1)
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	mustDo(t, server, http.MethodPost, "/api/v1/persons/merge",
		fmt.Sprintf(`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`,
			survivor, child,
			entityVersion(t, server, "/api/v1/persons/"+survivor, ""), entityVersion(t, server, childPath, "")),
		http.StatusOK)

	refused := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusConflict)
	pending := entryStrings(t, refused["pending_stream_ids"].([]any))
	if len(pending) != 2 || pending[0] != child || pending[1] != family {
		t.Fatalf("pending_stream_ids = %v, want [%s %s]", pending, child, family)
	}
	mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"branch"},{"stream_id":%q,"resolution":"main"}]}`, child, family),
		http.StatusBadRequest)
	done := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"main"},{"stream_id":%q,"resolution":"main"}]}`, child, family),
		http.StatusOK)
	if got := done["replayed_event_count"]; got != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0", got)
	}
	if children, _ := getEntity(t, server, "/api/v1/families/"+family, "")["children"].([]any); len(children) != 0 {
		t.Errorf("main family children = %v, want no phantom child", children)
	}
}

func runResumeRepairsProjection(t *testing.T, server *api.Server, events repository.EventStore, reads *faultyReadStore) {
	t.Helper()
	ctx := context.Background()
	person := createPerson(t, server, "Alex", "Original")
	branchID := createBranch(t, server, "projection-line")
	branchPath := "/api/v1/branches/" + branchID
	path := "/api/v1/persons/" + person
	mustDo(t, server, http.MethodPut, scoped(path, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, path, branchID)), http.StatusOK)

	reads.failFor(uuid.MustParse(person))
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	reads.failFor(uuid.Nil)
	if got := mustString(t, getEntity(t, server, path, ""), "surname"); got != "Original" {
		t.Fatalf("surname on main = %q, want the failed projection to have left it behind", got)
	}
	logBefore := mainEventCount(t, ctx, events, person)

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := resumed["replayed_event_count"]; got != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0 (the events are already in the log)", got)
	}
	if got := entryStrings(t, jsonArray(t, resumed, "reprojected_stream_ids")); len(got) != 1 || got[0] != person {
		t.Errorf("reprojected_stream_ids = %v, want [%s]", got, person)
	}
	if got := mainEventCount(t, ctx, events, person); got != logBefore {
		t.Errorf("person has %d main events after resume, want %d (no re-append)", got, logBefore)
	}
	entity := getEntity(t, server, path, "")
	if got := mustString(t, entity, "surname"); got != "Revised" {
		t.Errorf("surname on main = %q, want the read model repaired", got)
	}
	if got := entity["version"]; got != float64(logBefore) {
		t.Errorf("read-model version = %v, want the log's %d", got, logBefore)
	}

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := jsonArray(t, again, "reprojected_stream_ids"); len(got) != 0 {
		t.Errorf("second resume reprojected_stream_ids = %v, want none", got)
	}
}
