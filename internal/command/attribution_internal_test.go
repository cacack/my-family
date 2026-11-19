package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// appendCapture captures the events handed to Append. Every other
// method is the nil embedded interface's, so a call to one panics.
type appendCapture struct {
	repository.EventStore
	appended []domain.Event
}

func (r *appendCapture) Append(_ context.Context, _ uuid.UUID, _ string, events []domain.Event, _ int64, _ repository.AppendScope) error {
	r.appended = events
	return nil
}

func appendThroughAttribution(t *testing.T, ctx context.Context, events ...domain.Event) []domain.Event {
	t.Helper()
	inner := &appendCapture{}
	if err := withAttribution(inner).Append(ctx, uuid.New(), "person", events, -1, repository.MainScope); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	return inner.appended
}

func TestReplayMetadata(t *testing.T) {
	provenance := &domain.MergeProvenance{BranchID: uuid.New(), BranchName: "hypothesis"}
	tests := []struct {
		name     string
		metadata string
		wantUser string
		wantCorr string
	}{
		{name: "authorless stays authorless"},
		{name: "keeps author and correlation", metadata: `{"user_id":"author-a","correlation_id":"c1"}`, wantUser: "author-a", wantCorr: "c1"},
		{name: "undecodable replays authorless", metadata: `{not json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := &repository.StoredEvent{ID: uuid.New(), EventType: "PersonUpdated"}
			if tt.metadata != "" {
				evt.Metadata = []byte(tt.metadata)
			}
			got := replayMetadata(evt, provenance)
			if got.UserID != tt.wantUser || got.CorrelationID != tt.wantCorr {
				t.Errorf("replayMetadata() = %+v, want user %q correlation %q", got, tt.wantUser, tt.wantCorr)
			}
			if got.MergedFromBranch != provenance {
				t.Errorf("MergedFromBranch = %v, want the merge's provenance", got.MergedFromBranch)
			}
		})
	}
}

func TestAttributingEventStore_StampsTheActor(t *testing.T) {
	ctx := domain.ContextWithActor(context.Background(), "user-a")
	event := domain.NewPersonCreated(domain.NewPerson("Ada", "Lovelace"))

	got := appendThroughAttribution(t, ctx, event)

	if len(got) != 1 {
		t.Fatalf("appended %d events, want 1", len(got))
	}
	_, meta, at := domain.UnwrapEvent(got[0])
	if meta == nil || meta.UserID != "user-a" {
		t.Errorf("metadata = %+v, want UserID user-a", meta)
	}
	if !at.Equal(event.OccurredAt()) {
		t.Errorf("recorded at %v, want the event's OccurredAt %v", at, event.OccurredAt())
	}
}

func TestAttributingEventStore_NoActorIsANoOp(t *testing.T) {
	event := domain.NewPersonCreated(domain.NewPerson("Ada", "Lovelace"))
	events := []domain.Event{event}

	inner := &appendCapture{}
	if err := withAttribution(inner).Append(context.Background(), uuid.New(), "person", events, -1, repository.MainScope); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if len(inner.appended) != 1 || &inner.appended[0] != &events[0] {
		t.Fatalf("without an actor the events slice must pass through unchanged")
	}
	if _, stamped := inner.appended[0].(domain.StampedEvent); stamped {
		t.Error("an event appended without an actor was stamped")
	}
}

func TestAttributingEventStore_NeverOverwritesAStamp(t *testing.T) {
	ctx := domain.ContextWithActor(context.Background(), "merger")
	provenance := &domain.MergeProvenance{BranchID: uuid.New(), BranchName: "theory", ClaimID: uuid.New()}
	mergedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	authored := domain.Stamp(domain.NewPersonCreated(domain.NewPerson("Ada", "Lovelace")),
		domain.EventMetadata{UserID: "author", MergedFromBranch: provenance}, mergedAt)
	authorless := domain.Stamp(domain.NewPersonCreated(domain.NewPerson("Ada", "Byron")),
		domain.EventMetadata{MergedFromBranch: provenance}, mergedAt)
	plain := domain.NewPersonCreated(domain.NewPerson("Ada", "King"))

	got := appendThroughAttribution(t, ctx, authored, authorless, plain)

	tests := []struct {
		name       string
		event      domain.Event
		wantUser   string
		wantMerged bool
	}{
		{name: "explicit author kept", event: got[0], wantUser: "author", wantMerged: true},
		{name: "explicitly authorless stays authorless", event: got[1], wantUser: "", wantMerged: true},
		{name: "unstamped event takes the actor", event: got[2], wantUser: "merger", wantMerged: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, meta, _ := domain.UnwrapEvent(tt.event)
			if meta == nil {
				t.Fatal("metadata dropped")
			}
			if meta.UserID != tt.wantUser {
				t.Errorf("UserID = %q, want %q", meta.UserID, tt.wantUser)
			}
			if (meta.MergedFromBranch != nil) != tt.wantMerged {
				t.Errorf("MergedFromBranch = %+v, want present=%v", meta.MergedFromBranch, tt.wantMerged)
			}
			if tt.wantMerged && *meta.MergedFromBranch != *provenance {
				t.Errorf("MergedFromBranch = %+v, want %+v", meta.MergedFromBranch, provenance)
			}
		})
	}
	for i := range 2 {
		if inner := got[i].(domain.StampedEvent).Event; isStamped(inner) {
			t.Errorf("event %d: a StampedEvent was stamped again", i)
		}
	}
}

func TestWithAttribution_WrapsOnce(t *testing.T) {
	if withAttribution(nil) != nil {
		t.Error("a nil store must stay nil")
	}
	once := withAttribution(memory.NewEventStore())
	twice := withAttribution(once)
	if twice != once {
		t.Error("an attributing store was wrapped again")
	}
	if inner := twice.(attributingEventStore).EventStore; isAttributing(inner) {
		t.Error("an attributing store wraps another attributing store")
	}
}

func isStamped(event domain.Event) bool {
	_, ok := event.(domain.StampedEvent)
	return ok
}

func isAttributing(store repository.EventStore) bool {
	_, ok := store.(attributingEventStore)
	return ok
}

// TestHandlerConstructors_AttributeAppends pins invariant ES-009's wiring:
// every constructor's handler appends through exactly one attributing store.
func TestHandlerConstructors_AttributeAppends(t *testing.T) {
	events := memory.NewEventStore()
	reads := memory.NewReadModelStore()
	branches := memory.NewBranchStore()
	tests := []struct {
		name    string
		handler *Handler
	}{
		{name: "NewHandler", handler: NewHandler(events, reads)},
		{name: "NewHandlerWithBranchStore", handler: NewHandlerWithBranchStore(events, reads, branches)},
		{name: "NewHandlerWithBranches", handler: NewHandlerWithBranches(events, reads, branches, memory.NewSnapshotStore(events))},
		{name: "NewHandlerWithRollbackService", handler: NewHandlerWithRollbackService(events, reads, query.NewRollbackService(events, reads))},
		{name: "already attributing store", handler: NewHandler(withAttribution(events), reads)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, ok := tt.handler.eventStore.(attributingEventStore)
			if !ok {
				t.Fatalf("eventStore is %T, want attributingEventStore", tt.handler.eventStore)
			}
			if isAttributing(store.EventStore) {
				t.Error("eventStore is wrapped twice")
			}
			if scoped := tt.handler.WithBranch(&domain.Branch{ID: uuid.New()}); !isAttributing(scoped.eventStore) {
				t.Error("a branch-scoped copy lost the attributing store")
			}
		})
	}
}
