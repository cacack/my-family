package command

// The merge staleness guard's missing-pin refusal, tested from inside the
// package.
//
// It is white-box deliberately. The invariant under test — every replayed stream
// carries a pinned main version — holds by construction today: PlanMerge derives
// MergePlan.MainStreamVersions and MergePlan.ReplayEvents from the same event
// slice, so no black-box route through MergeBranch can produce a plan that
// violates it. The refusal exists for the SECOND plan constructor ADR-005
// anticipates (#685's stored, replayed plan), and a guard no test can reach is a
// guard that quietly stops working. Handing the guard a hand-built plan is the
// only way to exercise it, and that means reaching past the exported surface.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// mergeGuardHandler is a handler with just the collaborator the staleness guard
// and the replay use: the event store.
func mergeGuardHandler(t *testing.T) (*Handler, *memory.EventStore) {
	t.Helper()
	eventStore := memory.NewEventStore()
	return &Handler{eventStore: eventStore}, eventStore
}

// A plan missing the pin for a stream main has NEVER seen is the case the old
// single-value map read waved through: absent reads as 0, main reads 0, they
// compare equal, and the merge proceeds unguarded onto exactly the shape
// Append's `expectedVersion >= 0` gate also cannot catch.
func TestValidatePlanNotStale_MissingPinForUnseenStreamIsRefused(t *testing.T) {
	h, _ := mergeGuardHandler(t)
	streamID := uuid.New()

	plan := &query.MergePlan{
		Branch:             &domain.Branch{ID: uuid.New()},
		MainStreamVersions: map[uuid.UUID]int64{}, // the pin that should be here
	}
	groups := []streamGroup{{streamID: streamID, streamType: "person"}}

	err := h.validatePlanNotStale(context.Background(), plan, groups, nil)
	if !errors.Is(err, ErrMergePlanIncomplete) {
		t.Fatalf("validatePlanNotStale = %v, want ErrMergePlanIncomplete", err)
	}
}

// A stream resolved to main is not replayed, so it needs no pin and must not be
// refused for lacking one — the guard's scope rule has to survive this change.
func TestValidatePlanNotStale_MissingPinIgnoredForMainResolvedStream(t *testing.T) {
	h, _ := mergeGuardHandler(t)
	streamID := uuid.New()

	plan := &query.MergePlan{
		Branch:             &domain.Branch{ID: uuid.New()},
		MainStreamVersions: map[uuid.UUID]int64{},
	}
	groups := []streamGroup{{streamID: streamID, streamType: "person"}}
	resolutions := map[uuid.UUID]MergeResolution{streamID: ResolveMain}

	if err := h.validatePlanNotStale(context.Background(), plan, groups, resolutions); err != nil {
		t.Fatalf("validatePlanNotStale = %v, want nil for a stream resolved to main", err)
	}
}

// The replay repeats the two-value read rather than trusting the pre-claim check
// to have run, because past the claim an unguarded append is unrecoverable.
func TestReplayOntoMain_MissingPinIsRefusedBeforeAppending(t *testing.T) {
	h, eventStore := mergeGuardHandler(t)
	ctx := context.Background()
	streamID := uuid.New()
	branch := &domain.Branch{ID: uuid.New()}

	created := domain.NewPersonCreated(&domain.Person{ID: streamID, GivenName: "Ada", Surname: "Lovelace"})
	data, err := json.Marshal(created)
	if err != nil {
		t.Fatalf("marshalling the seed event failed: %v", err)
	}
	groups := []streamGroup{{
		streamID:   streamID,
		streamType: "person",
		events: []repository.StoredEvent{{
			StreamID:   streamID,
			StreamType: "person",
			EventType:  created.EventType(),
			Data:       data,
			Version:    1,
		}},
	}}

	_, _, err = h.replayOntoMain(ctx, branch, groups, map[uuid.UUID]int64{}, nil)
	if !errors.Is(err, ErrMergePlanIncomplete) {
		t.Fatalf("replayOntoMain = %v, want ErrMergePlanIncomplete", err)
	}

	onMain, err := eventStore.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	if len(onMain) != 0 {
		t.Errorf("main gained %d events from a refused replay, want 0", len(onMain))
	}
}
