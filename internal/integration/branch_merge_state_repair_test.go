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

// TestBranchMergeState_ProjectionFailure: on every backend, a merge whose
// last stream reached main's log but failed to project answers
// merge_partially_applied and then reads as incomplete — the stream pending
// as needs_repair — until a resume repairs it (#830).
func TestBranchMergeState_ProjectionFailure(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			reads := &faultyReadStore{ReadModelStore: st.read}
			wrapped := st
			wrapped.read = reads
			runMergeStateProjectionFailure(t, newServer(t, wrapped), reads)
		})
	}
}

func runMergeStateProjectionFailure(t *testing.T, server *api.Server, reads *faultyReadStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	branchID := createBranch(t, server, "repair-line")
	branchPath := "/api/v1/branches/" + branchID
	path := "/api/v1/persons/" + person
	mustDo(t, server, http.MethodPut, scoped(path, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, path, branchID)), http.StatusOK)

	reads.failFor(uuid.MustParse(person))
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	reads.failFor(uuid.Nil)
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}

	assertIncomplete(t, "GET /branches/{id}", mustDo(t, server, http.MethodGet, branchPath, "", http.StatusOK),
		person, "needs_repair", false)
	if surname := mustString(t, getEntity(t, server, path, ""), "surname"); surname != "Original" {
		t.Errorf("main surname = %q after the read, want it still unrepaired", surname)
	}

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if resumedBranch, _ := resumed["branch"].(map[string]any); resumedBranch["merge_state"] != "complete" {
		t.Errorf("resume result merge_state = %v, want complete", resumedBranch["merge_state"])
	}
	if surname := mustString(t, getEntity(t, server, path, ""), "surname"); surname != "Revised" {
		t.Errorf("main surname = %q after the resume, want the repaired change", surname)
	}
}

// unreadableReadStore fails main GetPerson for one person while armed, so the
// merge state of a branch that merged that person cannot be read.
type unreadableReadStore struct {
	repository.ReadModelStore
	mu   sync.Mutex
	fail uuid.UUID
}

func (s *unreadableReadStore) failGetFor(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = id
}

func (s *unreadableReadStore) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	s.mu.Lock()
	fail := branchID.IsMain() && id == s.fail
	s.mu.Unlock()
	if fail {
		return nil, errors.New("injected read failure")
	}
	return s.ReadModelStore.GetPerson(ctx, branchID, id)
}

// TestBranchMergeState_UnreadableStateDoesNotFailReads: on every backend, a
// merged branch whose merge state cannot be read is reported as
// merge_state "unknown"; the list, the branch and its comparison still answer
// 200, and every other branch keeps its state.
func TestBranchMergeState_UnreadableStateDoesNotFailReads(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			reads := &unreadableReadStore{ReadModelStore: st.read}
			wrapped := st
			wrapped.read = reads
			runUnreadableMergeState(t, newServer(t, wrapped), reads)
		})
	}
}

func runUnreadableMergeState(t *testing.T, server *api.Server, reads *unreadableReadStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	poisoned := createBranch(t, server, "poisoned")
	healthy := createBranch(t, server, "healthy")
	path := "/api/v1/persons/" + person
	mustDo(t, server, http.MethodPut, scoped(path, poisoned),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, path, poisoned)), http.StatusOK)
	mustDo(t, server, http.MethodPost, scoped("/api/v1/persons", healthy),
		`{"given_name":"Kit","surname":"Healthy","gender":"unknown"}`, http.StatusCreated)
	mustDo(t, server, http.MethodPost, "/api/v1/branches/"+poisoned+"/merge", `{}`, http.StatusOK)
	mustDo(t, server, http.MethodPost, "/api/v1/branches/"+healthy+"/merge", `{}`, http.StatusOK)
	// Move the log's head, so no remembered verdict answers for the branch.
	createPerson(t, server, "Sam", "Later")

	reads.failGetFor(uuid.MustParse(person))
	defer reads.failGetFor(uuid.Nil)

	states := make(map[any]any)
	for _, raw := range jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/branches", "", http.StatusOK), "items") {
		if item, ok := raw.(map[string]any); ok {
			states[item["id"]] = item["merge_state"]
		}
	}
	if states[poisoned] != "unknown" || states[healthy] != "complete" {
		t.Errorf("list merge states = %v, want %s unknown and %s complete", states, poisoned, healthy)
	}
	if got := mustDo(t, server, http.MethodGet, "/api/v1/branches/"+poisoned, "", http.StatusOK); got["merge_state"] != "unknown" {
		t.Errorf("GET /branches/{id} merge_state = %v, want unknown", got["merge_state"])
	}
	compared := mustDo(t, server, http.MethodGet, "/api/v1/branches/"+poisoned+"/compare", "", http.StatusOK)
	if branch, _ := compared["branch"].(map[string]any); branch["merge_state"] != "unknown" {
		t.Errorf("compare branch merge_state = %v, want unknown", branch["merge_state"])
	}
}
