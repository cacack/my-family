package command

// Malformed-input paths of the merge's media owner check (#759). The events the
// command layer writes always decode and always carry a valid owner type, so
// these refusals are only reachable with a hand-built replay group.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestCheckMediaOwnerSurvives_MalformedInput(t *testing.T) {
	h := &Handler{}
	streamID := uuid.New()
	group := func(data string) streamGroup {
		return streamGroup{streamID: streamID, streamType: "Media", events: []repository.StoredEvent{
			{StreamID: streamID, EventType: "MediaCreated", Data: []byte(data)},
		}}
	}

	err := h.checkMediaOwnerSurvives(context.Background(), group(`{not json`), evidencePlan{})
	if err == nil || errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("undecodable MediaCreated: err = %v, want a decode error", err)
	}

	err = h.checkMediaOwnerSurvives(context.Background(),
		group(`{"entity_type":"planet","entity_id":"`+uuid.NewString()+`"}`), evidencePlan{})
	if !errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("unknown owner type: err = %v, want ErrMergeDanglingReference", err)
	}

	// A stream that uploads and then deletes its item leaves nothing to check.
	deleted := group(`{"entity_type":"planet"}`)
	deleted.events = append(deleted.events, repository.StoredEvent{StreamID: streamID, EventType: "MediaDeleted"})
	if err := h.checkMediaOwnerSurvives(context.Background(), deleted, evidencePlan{}); err != nil {
		t.Errorf("upload then delete: err = %v, want nil", err)
	}
}

// listMediaFailStore fails every owner media listing.
type listMediaFailStore struct{ repository.ReadModelStore }

func (listMediaFailStore) ListMediaForEntity(context.Context, string, uuid.UUID, repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	return nil, 0, errors.New("listing failed")
}

// recordingEventStore records the stream set, start position and limit of
// each ReadStreamsForBranch and returns the canned events of those streams
// after that position (capped at the limit), or fails when err is set.
type recordingEventStore struct {
	repository.EventStore
	asked  [][]uuid.UUID
	froms  []int64
	limits []int
	events []repository.StoredEvent
	err    error
}

func (s *recordingEventStore) ReadStreamsForBranch(_ context.Context, ids []uuid.UUID, _ domain.BranchID, from int64, limit int) ([]repository.StoredEvent, error) {
	s.asked = append(s.asked, ids)
	s.froms = append(s.froms, from)
	s.limits = append(s.limits, limit)
	if s.err != nil {
		return nil, s.err
	}
	asked := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		asked[id] = true
	}
	var out []repository.StoredEvent
	for _, evt := range s.events {
		if asked[evt.StreamID] && evt.Position > from && len(out) < limit {
			out = append(out, evt)
		}
	}
	return out, nil
}

// checkOwnerDeleteOrphansNoMedia lists the owner's main media page by page,
// asks in ONE set-based query for the first main event on any candidate after
// the branch's delete — never the candidates' histories, whose MediaCreated
// events carry the file bytes — and refuses only an item main wrote to after
// that delete. Failures to read either
// side are errors, not refusals.
func TestCheckOwnerDeleteOrphansNoMedia(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	owner := uuid.New()
	deleting := personGroup(owner, "PersonDeleted")
	deleting.events[0].Position = 100

	// More items than one listing page, so the listing must page.
	var ids []uuid.UUID
	for i := 0; i < mediaPageSize+1; i++ {
		id := uuid.New()
		ids = append(ids, id)
		if err := store.SaveMedia(ctx, domain.MainBranchID, &repository.MediaReadModel{
			ID: id, EntityType: "person", EntityID: owner, Title: "Scan", Version: 1,
		}); err != nil {
			t.Fatalf("SaveMedia failed: %v", err)
		}
	}
	events := &recordingEventStore{}
	h := &Handler{readStore: store, eventStore: events}

	if err := h.checkOwnerDeleteOrphansNoMedia(ctx, deleting, evidencePlan{}); err != nil {
		t.Fatalf("items with no later main writes: err = %v, want nil", err)
	}
	if len(events.asked) != 1 || len(events.asked[0]) != len(ids) {
		t.Fatalf("scans = %d (first of %d streams), want one scan of all %d items", len(events.asked), len(events.asked[0]), len(ids))
	}
	if events.froms[0] != 100 || events.limits[0] != 1 {
		t.Errorf("scan from %d limit %d, want only the first event after the delete (from 100, limit 1)", events.froms[0], events.limits[0])
	}

	late := ids[len(ids)-1]
	events.events = []repository.StoredEvent{
		{StreamID: ids[0], EventType: "MediaCreated", Position: 50},
		{StreamID: late, EventType: "MediaCreated", Position: 150},
	}
	err := h.checkOwnerDeleteOrphansNoMedia(ctx, deleting, evidencePlan{})
	if !errors.Is(err, ErrMergeDanglingReference) || !strings.Contains(err.Error(), late.String()) {
		t.Errorf("item uploaded after the branch's delete: err = %v, want ErrMergeDanglingReference naming %s", err, late)
	}
	// The same item carried by the replay is the replay's business.
	replayed := map[uuid.UUID]streamGroup{late: mediaUploadGroup(late, "person", owner)}
	if err := h.checkOwnerDeleteOrphansNoMedia(ctx, deleting, evidencePlan{replayed: replayed}); err != nil {
		t.Errorf("late item the replay carries: err = %v, want nil", err)
	}

	// Streams that delete no media owner are not checked at all.
	for _, group := range []streamGroup{
		personGroup(owner, "PersonUpdated"),
		mediaUploadGroup(uuid.New(), "person", owner),
	} {
		if err := (&Handler{}).checkOwnerDeleteOrphansNoMedia(ctx, group, evidencePlan{}); err != nil {
			t.Errorf("%s stream without an owner delete: err = %v, want nil", group.streamType, err)
		}
	}

	events.err = errors.New("scan failed")
	if err := h.checkOwnerDeleteOrphansNoMedia(ctx, deleting, evidencePlan{}); err == nil || errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("failed history scan: err = %v, want a read error", err)
	}
	failing := &Handler{readStore: listMediaFailStore{store}, eventStore: events}
	if err := failing.checkOwnerDeleteOrphansNoMedia(ctx, deleting, evidencePlan{}); err == nil || errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("failed listing: err = %v, want a read error", err)
	}
}

func TestOwnerDeleteOf(t *testing.T) {
	id := uuid.New()
	cases := []struct {
		group streamGroup
		want  string
		ok    bool
	}{
		{personGroup(id, "PersonCreated", "PersonDeleted"), "person", true},
		{streamGroup{streamType: "family", events: []repository.StoredEvent{{EventType: "FamilyDeleted", Position: 7}}}, "family", true},
		{streamGroup{streamType: "Source", events: []repository.StoredEvent{{EventType: "SourceDeleted", Position: 7}}}, "source", true},
		{personGroup(id, "PersonUpdated"), "", false},
		// A delete event on the wrong stream type is not an owner delete.
		{streamGroup{streamType: "Family", events: []repository.StoredEvent{{EventType: "PersonDeleted"}}}, "", false},
	}
	for _, tc := range cases {
		got, _, ok := ownerDeleteOf(tc.group)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ownerDeleteOf(%s %v) = %q, %v; want %q, %v", tc.group.streamType, tc.group.events, got, ok, tc.want, tc.ok)
		}
	}
}
