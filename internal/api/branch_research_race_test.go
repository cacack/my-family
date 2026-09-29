package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// losingBranchUpdateEventStore makes the next BranchUpdated append lose its
// optimistic-concurrency check, as if a rival write to the branch's stream had
// landed after the edit read the branch.
type losingBranchUpdateEventStore struct {
	repository.EventStore
	lose bool
}

func (s *losingBranchUpdateEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string,
	events []domain.Event, expectedVersion int64, scope repository.AppendScope,
) error {
	if s.lose && len(events) == 1 && events[0].EventType() == "BranchUpdated" {
		s.lose = false
		return repository.ErrConcurrencyConflict
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// TestUpdateBranch_ConcurrentWriteIsA409 pins the mapping of a lost version
// race on PATCH /branches/{id} to 409 branch_changed (#835), with nothing
// recorded.
func TestUpdateBranch_ConcurrentWriteIsA409(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	base := memory.NewEventStore()
	events := &losingBranchUpdateEventStore{EventStore: base}
	server := api.NewServer(cfg, events, memory.NewReadModelStore(),
		memory.NewSnapshotStore(base), nil, api.WithBranchStore(memory.NewBranchStore()))

	rec := do(t, server, http.MethodPost, "/api/v1/branches", `{"name":"mary","hypothesis":"Q"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateBranch status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	id, _ := decodeJSON(t, rec)["id"].(string)

	events.lose = true
	rec = do(t, server, http.MethodPatch, "/api/v1/branches/"+id, `{"hypothesis":"H"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("PATCH status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeJSON(t, rec)["code"]; code != "branch_changed" {
		t.Errorf("code = %v, want branch_changed", code)
	}

	rec = do(t, server, http.MethodGet, "/api/v1/branches/"+id, "")
	if hyp := decodeJSON(t, rec)["hypothesis"]; hyp != "Q" {
		t.Errorf("hypothesis after refused edit = %v, want Q", hyp)
	}
}
