package memory_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestEventStore_Provenance(t *testing.T) {
	runProvenanceScenario(t, memory.NewEventStore())
}

// runProvenanceScenario is #832's store contract, kept identical across the
// memory, SQLite and PostgreSQL stores (DB-001): a stamped event's metadata is
// persisted in the envelope and read back by every read path, the stamp's
// record time is the stored timestamp while the payload keeps the event's own,
// an event appended without metadata reads back with none and still decodes,
// and the global history keeps the any-branch types from every branch.
func runProvenanceScenario(t *testing.T, store repository.EventStore) {
	t.Helper()
	ctx := context.Background()
	personID := uuid.New()
	branchID := uuid.New()
	occurred := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)
	mergedAt := time.Now().UTC().Truncate(time.Millisecond)

	created := domain.NewPersonCreated(&domain.Person{ID: personID, GivenName: "Ada", Surname: "Lovelace"})
	created.Timestamp = occurred.Add(-time.Hour)
	if err := store.Append(ctx, personID, "Person", []domain.Event{created}, -1, repository.MainScope); err != nil {
		t.Fatalf("append created: %v", err)
	}
	lifecycle := domain.NewBranchCreated(&domain.Branch{ID: branchID, Name: "Byron theory"})
	lifecycle.Timestamp = mergedAt.Add(-time.Minute)
	if err := store.Append(ctx, branchID, "branch", []domain.Event{lifecycle}, -1,
		repository.AppendScope{BranchID: domain.BranchID(branchID)}); err != nil {
		t.Fatalf("append branch created: %v", err)
	}

	updated := domain.NewPersonUpdated(personID, map[string]any{"surname": "Byron"})
	updated.Timestamp = occurred
	provenance := &domain.MergeProvenance{
		BranchID: branchID, BranchName: "Byron theory", ClaimID: uuid.New(),
		MergedAtPosition: 7, MergedAt: mergedAt, Note: "baptism register",
	}
	stamped := domain.Stamp(updated, domain.EventMetadata{MergedFromBranch: provenance}, mergedAt)
	if err := store.Append(ctx, personID, "Person", []domain.Event{stamped}, 1, repository.MainScope); err != nil {
		t.Fatalf("append stamped: %v", err)
	}

	wantPayload, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	check := func(label string, events []repository.StoredEvent) {
		t.Helper()
		var plain, replay *repository.StoredEvent
		for i := range events {
			switch events[i].EventType {
			case "PersonCreated":
				plain = &events[i]
			case "PersonUpdated":
				replay = &events[i]
			}
		}
		if plain == nil || replay == nil {
			t.Fatalf("%s: read %d events, want the created and the stamped update", label, len(events))
		}
		if len(plain.Metadata) != 0 {
			t.Errorf("%s: unstamped event metadata = %s, want none", label, plain.Metadata)
		}
		if _, err := plain.DecodeEvent(); err != nil {
			t.Errorf("%s: unstamped event does not decode: %v", label, err)
		}
		var meta domain.EventMetadata
		if err := json.Unmarshal(replay.Metadata, &meta); err != nil {
			t.Fatalf("%s: metadata %q: %v", label, replay.Metadata, err)
		}
		got := meta.MergedFromBranch
		if got == nil || got.BranchID != branchID || got.BranchName != "Byron theory" || got.ClaimID != provenance.ClaimID ||
			got.MergedAtPosition != 7 || got.Note != "baptism register" || !got.MergedAt.Equal(mergedAt) {
			t.Errorf("%s: provenance = %+v, want %+v", label, got, provenance)
		}
		if !replay.Timestamp.Equal(mergedAt) {
			t.Errorf("%s: stored timestamp = %s, want the stamp's record time %s", label, replay.Timestamp, mergedAt)
		}
		var payload, want map[string]any
		if err := json.Unmarshal(replay.Data, &payload); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(wantPayload, &want); err != nil {
			t.Fatal(err)
		}
		if len(payload) != len(want) || payload["id"] != want["id"] || payload["timestamp"] != want["timestamp"] {
			t.Errorf("%s: payload = %s, want the inner event's own %s", label, replay.Data, wantPayload)
		}
		decoded, err := replay.DecodeEvent()
		if err != nil {
			t.Fatalf("%s: decode: %v", label, err)
		}
		if !decoded.OccurredAt().Equal(occurred) {
			t.Errorf("%s: decoded OccurredAt = %s, want the original %s", label, decoded.OccurredAt(), occurred)
		}
	}

	stream, err := store.ReadStream(ctx, personID)
	if err != nil {
		t.Fatal(err)
	}
	check("ReadStream", stream)
	page, err := store.ReadByStream(ctx, personID, domain.MainBranchID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	check("ReadByStream", page.Events)
	all, err := store.ReadAll(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	check("ReadAll", all)
	own, err := store.ReadBranch(ctx, domain.MainBranchID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	check("ReadBranch", own)
	set, err := store.ReadStreamsForBranch(ctx, []uuid.UUID{personID}, domain.MainBranchID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	check("ReadStreamsForBranch", set)

	main := domain.MainBranchID
	history, err := store.ReadGlobalHistory(ctx, repository.GlobalHistoryQuery{
		FromTime: mergedAt.Add(-2 * time.Minute), BranchID: &main,
		AnyBranchEventTypes: []string{"BranchCreated"}, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The window starts after the update occurred but before it was merged:
	// the stamped event is in it by its record time, and the branch's own
	// lifecycle event is kept although the page is the mainline's.
	if history.TotalCount != 2 || len(history.Events) != 2 ||
		history.Events[0].EventType != "BranchCreated" || history.Events[1].EventType != "PersonUpdated" {
		types := make([]string, 0, len(history.Events))
		for _, e := range history.Events {
			types = append(types, e.EventType)
		}
		t.Errorf("global history window = %v (total %d), want [BranchCreated PersonUpdated]", types, history.TotalCount)
	}
	mainOnly, err := store.ReadGlobalHistory(ctx, repository.GlobalHistoryQuery{BranchID: &main, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if mainOnly.TotalCount != 2 {
		t.Errorf("mainline history without any-branch types total = %d, want 2 (the branch's event excluded)", mainOnly.TotalCount)
	}
	onlyLifecycle, err := store.ReadGlobalHistory(ctx, repository.GlobalHistoryQuery{
		BranchID: &main, AnyBranchEventTypes: []string{"BranchCreated"}, IncludeEventTypes: []string{"BranchCreated"}, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if onlyLifecycle.TotalCount != 1 {
		t.Errorf("include filter over any-branch types total = %d, want 1", onlyLifecycle.TotalCount)
	}

	// A merge claim with no record is stored exactly as one written before
	// #832 (every record field is omitted), and one with a record keeps it.
	scope := repository.AppendScope{BranchID: domain.BranchID(branchID)}
	legacy := domain.NewBranchMerged(branchID, 0, 7, "old", map[uuid.UUID]int64{personID: 1})
	if err := store.Append(ctx, branchID, "branch", []domain.Event{legacy}, 1, scope); err != nil {
		t.Fatalf("append legacy claim: %v", err)
	}
	count := 1
	recorded := domain.NewBranchMerged(branchID, 0, 7, "new", map[uuid.UUID]int64{personID: 1})
	recorded.ReplayedEventCount = &count
	recorded.Resolutions = []domain.MergeDecision{{StreamID: personID, Kind: "edit_edit", Resolution: "branch"}}
	if err := store.Append(ctx, branchID, "branch", []domain.Event{recorded}, 2, scope); err != nil {
		t.Fatalf("append recorded claim: %v", err)
	}
	claims, err := store.ReadStream(ctx, branchID)
	if err != nil {
		t.Fatal(err)
	}
	var decodedClaims []domain.BranchMerged
	for i := range claims {
		if claims[i].EventType != "BranchMerged" {
			continue
		}
		decoded, err := claims[i].DecodeEvent()
		if err != nil {
			t.Fatalf("decode claim: %v", err)
		}
		decodedClaims = append(decodedClaims, decoded.(domain.BranchMerged))
	}
	if len(decodedClaims) != 2 || decodedClaims[0].HasRecord() || decodedClaims[0].Note != "old" ||
		!decodedClaims[1].HasRecord() || len(decodedClaims[1].Resolutions) != 1 {
		t.Errorf("claims read back as %+v, want one without a record and one with it", decodedClaims)
	}
}
