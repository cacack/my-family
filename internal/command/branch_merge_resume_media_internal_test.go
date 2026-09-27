package command

// Shapes of the media-owner rule and the media repair that only a hand-built
// replay can reach: a claim written before #685 has no plan, so its resume can
// meet streams already on main that a planned merge would never have landed in
// that order.

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func mediaUploadGroup(mediaID uuid.UUID, entityType string, owner uuid.UUID) streamGroup {
	return streamGroup{streamID: mediaID, streamType: "Media", events: []repository.StoredEvent{{
		StreamID: mediaID, EventType: "MediaCreated",
		Data: []byte(`{"media_id":"` + mediaID.String() + `","entity_type":"` + entityType + `","entity_id":"` + owner.String() + `"}`),
	}}}
}

func personGroup(personID uuid.UUID, eventTypes ...string) streamGroup {
	group := streamGroup{streamID: personID, streamType: "Person"}
	for _, eventType := range eventTypes {
		group.events = append(group.events, repository.StoredEvent{StreamID: personID, EventType: eventType, Data: []byte(`{}`)})
	}
	return group
}

// An owner-deleting stream counts as deleting AFTER the upload only while it
// is still to be replayed: on a resume, one already on main has deleted the
// owner, and an owner main removed since the claim is gone whatever its stream
// holds.
func TestCheckMediaOwnerSurvives_ResumeTerms(t *testing.T) {
	h := &Handler{readStore: memory.NewReadModelStore()}
	ctx := context.Background()
	mediaID, owner := uuid.New(), uuid.New()
	upload := mediaUploadGroup(mediaID, "person", owner)
	deleting := personGroup(owner, "PersonCreated", "PersonDeleted")
	creating := personGroup(owner, "PersonCreated")

	cases := []struct {
		name    string
		plan    evidencePlan
		refused bool
	}{
		{"owner deleted later in the replay", evidencePlan{
			replayed: map[uuid.UUID]streamGroup{owner: deleting},
			order:    map[uuid.UUID]int{mediaID: 0, owner: 1},
		}, false},
		{"owner deleted earlier in the replay", evidencePlan{
			replayed: map[uuid.UUID]streamGroup{owner: deleting},
			order:    map[uuid.UUID]int{mediaID: 1, owner: 0},
		}, true},
		{"owner's delete already on main", evidencePlan{
			replayed: map[uuid.UUID]streamGroup{owner: deleting},
			order:    map[uuid.UUID]int{mediaID: 0, owner: 1},
			landed:   map[uuid.UUID]bool{owner: true},
		}, true},
		{"owner created by the replay", evidencePlan{
			replayed: map[uuid.UUID]streamGroup{owner: creating},
		}, false},
		{"owner removed on main since the claim", evidencePlan{
			replayed: map[uuid.UUID]streamGroup{owner: creating},
			removed:  map[uuid.UUID]bool{owner: true},
		}, true},
		{"owner neither replayed nor on main", evidencePlan{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := h.checkMediaOwnerSurvives(ctx, upload, tc.plan)
			if refused := errors.Is(err, ErrMergeDanglingReference); refused != tc.refused || (err != nil && !refused) {
				t.Errorf("checkMediaOwnerSurvives = %v, want refused=%v", err, tc.refused)
			}
		})
	}
}

// getMediaFailStore fails every media read.
type getMediaFailStore struct{ repository.ReadModelStore }

func (getMediaFailStore) GetMedia(context.Context, domain.BranchID, uuid.UUID) (*repository.MediaReadModel, error) {
	return nil, errors.New("media read failed")
}

// A media upload already on main may not be left attached to an owner main
// does not have by this call's own "main" resolution of that owner's stream;
// any other resolution, an owner main has, or an item main removed is fine.
func TestCheckLandedMediaOwners(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	h := &Handler{readStore: store}
	mediaID, owner := uuid.New(), uuid.New()
	upload := mediaUploadGroup(mediaID, "person", owner)
	creating := personGroup(owner, "PersonCreated")
	groups := []streamGroup{creating, upload}
	view := resumeView{landed: map[uuid.UUID]bool{mediaID: true}}

	err := h.checkLandedMediaOwners(ctx, groups, view, map[uuid.UUID]MergeResolution{owner: ResolveMain})
	if !errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("excluding the landed upload's owner: err = %v, want ErrMergeDanglingReference", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, view, map[uuid.UUID]MergeResolution{owner: ResolveBranch}); err != nil {
		t.Errorf("replaying the owner: err = %v, want nil", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, resumeView{}, map[uuid.UUID]MergeResolution{owner: ResolveMain}); err != nil {
		t.Errorf("upload not on main: err = %v, want nil", err)
	}

	// Main's log explains the item's absence (main deleted it, or an owner
	// it had cascaded it away): nothing is left to orphan.
	removed := resumeView{landed: view.landed, removed: map[uuid.UUID]bool{mediaID: true}}
	if err := h.checkLandedMediaOwners(ctx, groups, removed, map[uuid.UUID]MergeResolution{owner: ResolveMain}); err != nil {
		t.Errorf("item main removed: err = %v, want nil", err)
	}

	// Main's row names the owner that counts: a person merge on main moved
	// the item to a survivor main has.
	survivor := uuid.New()
	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: survivor, GivenName: "Sam", Version: 1}); err != nil {
		t.Fatalf("SavePerson failed: %v", err)
	}
	if err := store.SaveMedia(ctx, domain.MainBranchID, &repository.MediaReadModel{
		ID: mediaID, EntityType: "person", EntityID: survivor, Title: "Scan", Version: 2,
	}); err != nil {
		t.Fatalf("SaveMedia failed: %v", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, view, map[uuid.UUID]MergeResolution{owner: ResolveMain}); err != nil {
		t.Errorf("item moved to a survivor main has: err = %v, want nil", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, view, map[uuid.UUID]MergeResolution{survivor: ResolveMain}); err != nil {
		t.Errorf("survivor main has, resolved to main: err = %v, want nil", err)
	}
	failing := &Handler{readStore: getMediaFailStore{store}}
	if err := failing.checkLandedMediaOwners(ctx, groups, view, nil); err == nil || errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("failed media read: err = %v, want a read error", err)
	}
	if err := store.DeleteMedia(ctx, domain.MainBranchID, mediaID); err != nil {
		t.Fatalf("DeleteMedia failed: %v", err)
	}

	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: owner, GivenName: "Owen", Version: 1}); err != nil {
		t.Fatalf("SavePerson failed: %v", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, view, map[uuid.UUID]MergeResolution{owner: ResolveMain}); err != nil {
		t.Errorf("owner main already has: err = %v, want nil", err)
	}

	bad := streamGroup{streamID: mediaID, streamType: "Media", events: []repository.StoredEvent{{StreamID: mediaID, EventType: "MediaCreated", Data: []byte(`{not json`)}}}
	if err := h.checkLandedMediaOwners(ctx, []streamGroup{bad}, view, nil); err == nil || errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("undecodable landed upload: err = %v, want a decode error", err)
	}
}

func TestFinalSurvivor(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	early, late := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	chain := map[uuid.UUID]personMerge{
		a: {mergedID: a, survivorID: b, at: late},
		b: {mergedID: b, survivorID: c, at: early},
	}
	if got, at := finalSurvivor(a, chain); got != c || !at.Equal(late) {
		t.Errorf("chain a→b→c ends at %s (%v), want c at the latest merge", got, at)
	}
	if got, at := finalSurvivor(c, map[uuid.UUID]personMerge{a: {mergedID: a, survivorID: b}}); got != c || !at.IsZero() {
		t.Errorf("unmerged person ends at %s (%v), want itself and no merge time", got, at)
	}
	// A cycle cannot come from the command layer; it must still terminate.
	_, _ = finalSurvivor(a, map[uuid.UUID]personMerge{a: {mergedID: a, survivorID: b}, b: {mergedID: b, survivorID: a}})
}

// With no main row, a landed upload whose owner main merged away counts as
// attached to the final survivor the repair re-links it to: excluding the
// merged-away owner is then harmless, and only the survivor's absence would
// orphan it.
func TestCheckLandedMediaOwners_RelinkedToSurvivor(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	h := &Handler{readStore: store}
	mediaID, owner, survivor := uuid.New(), uuid.New(), uuid.New()
	groups := []streamGroup{personGroup(owner, "PersonUpdated"), personGroup(survivor, "PersonUpdated"), mediaUploadGroup(mediaID, "person", owner)}
	view := resumeView{landed: map[uuid.UUID]bool{mediaID: true}, relinked: map[uuid.UUID]uuid.UUID{mediaID: survivor}}

	if err := h.checkLandedMediaOwners(ctx, groups, view, map[uuid.UUID]MergeResolution{owner: ResolveMain}); err != nil {
		t.Errorf("merged-away owner resolved to main: err = %v, want nil (the item moves to the survivor)", err)
	}
	err := h.checkLandedMediaOwners(ctx, groups, view, map[uuid.UUID]MergeResolution{survivor: ResolveMain})
	if !errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("survivor main lacks, resolved to main: err = %v, want ErrMergeDanglingReference", err)
	}
	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: survivor, GivenName: "Sam", Version: 1}); err != nil {
		t.Fatalf("SavePerson failed: %v", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, view, map[uuid.UUID]MergeResolution{survivor: ResolveMain}); err != nil {
		t.Errorf("survivor main has, resolved to main: err = %v, want nil", err)
	}
}

// appendMainMerge records a PersonMerged of merged into survivor on main.
func appendMainMerge(t *testing.T, events repository.EventStore, survivor, merged uuid.UUID) {
	t.Helper()
	merge := domain.NewPersonMerged(survivor, merged, nil, nil, nil, nil, nil, nil, nil)
	if err := events.Append(context.Background(), survivor, "Person", []domain.Event{merge}, -1, repository.MainScope); err != nil {
		t.Fatalf("Append PersonMerged failed: %v", err)
	}
}

// relinkMergedMedia moves the re-projected row to the survivor with its
// version and bytes untouched, follows a merge of that survivor recorded
// after the resume's own scan, and leaves a vanished row alone.
func TestRelinkMergedMedia(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	events := memory.NewEventStore()
	h := &Handler{readStore: store, eventStore: events}
	mediaID, owner, first, second := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	group := mediaUploadGroup(mediaID, "person", owner)
	uploaded := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	bytesIn := []byte("scan bytes")
	if err := store.SaveMedia(ctx, domain.MainBranchID, &repository.MediaReadModel{
		ID: mediaID, EntityType: "person", EntityID: owner, Title: "Scan", FileData: bytesIn, Version: 3, UpdatedAt: uploaded,
	}); err != nil {
		t.Fatalf("SaveMedia failed: %v", err)
	}

	// The resume's scan saw owner→first; first→second lands after it.
	appendMainMerge(t, events, first, owner)
	appendMainMerge(t, events, second, first)
	if err := h.relinkMergedMedia(ctx, group, mergeRelink{target: first, mergedAt: uploaded.Add(time.Hour), scanFrom: 0}); err != nil {
		t.Fatalf("relinkMergedMedia failed: %v", err)
	}
	row, err := store.GetMediaWithData(ctx, domain.MainBranchID, mediaID)
	if err != nil || row == nil {
		t.Fatalf("GetMediaWithData = %v, %v", row, err)
	}
	if row.EntityID != second || row.Version != 3 || !bytes.Equal(row.FileData, bytesIn) {
		t.Errorf("row = owner %s version %d bytes %q; want the final survivor, version 3 and the bytes kept", row.EntityID, row.Version, row.FileData)
	}
	if !row.UpdatedAt.After(uploaded) {
		t.Errorf("updated_at = %v, want the merge's time", row.UpdatedAt)
	}

	if err := store.DeleteMedia(ctx, domain.MainBranchID, mediaID); err != nil {
		t.Fatalf("DeleteMedia failed: %v", err)
	}
	if err := h.relinkMergedMedia(ctx, group, mergeRelink{target: second}); err != nil {
		t.Errorf("vanished row: err = %v, want nil", err)
	}
	failing := &Handler{readStore: getMediaFailStore{store}, eventStore: events}
	if err := failing.relinkMergedMedia(ctx, group, mergeRelink{target: second}); err == nil {
		t.Errorf("failed media read: want an error")
	}
}

// A survivor merged again on every scan cannot hold the repair forever: it
// gives up with the retryable race error after reprojectAttempts passes.
func TestRelinkMergedMedia_GivesUpOnEndlessMerges(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	events := memory.NewEventStore()
	h := &Handler{readStore: store, eventStore: events}
	mediaID := uuid.New()
	chain := []uuid.UUID{uuid.New()}
	for i := 0; i < reprojectAttempts+1; i++ {
		next := uuid.New()
		appendMainMerge(t, events, next, chain[len(chain)-1])
		chain = append(chain, next)
	}
	if err := store.SaveMedia(ctx, domain.MainBranchID, &repository.MediaReadModel{
		ID: mediaID, EntityType: "person", EntityID: chain[0], Title: "Scan", Version: 1,
	}); err != nil {
		t.Fatalf("SaveMedia failed: %v", err)
	}
	// Each pass sees only one more link: the scan below its start is stale.
	calls := 0
	h.eventStore = &steppingMergeStore{EventStore: events, calls: &calls}
	err := h.relinkMergedMedia(ctx, mediaUploadGroup(mediaID, "person", chain[0]), mergeRelink{target: chain[0]})
	if !errors.Is(err, errReprojectRaced) {
		t.Errorf("endless merges: err = %v, want errReprojectRaced", err)
	}
}

// steppingMergeStore reveals one more of main's events on each ReadBranch
// call, so every merge scan finds exactly one new merge.
type steppingMergeStore struct {
	repository.EventStore
	calls *int
}

func (s *steppingMergeStore) ReadBranch(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	*s.calls++
	page, err := s.EventStore.ReadBranch(ctx, branchID, fromPosition, limit)
	if err != nil || len(page) <= *s.calls {
		return page, err
	}
	return page[:*s.calls], nil
}

func TestLastMediaOwner(t *testing.T) {
	first, second, mediaID := uuid.New(), uuid.New(), uuid.New()
	events := append(mediaUploadGroup(mediaID, "person", first).events, mediaUploadGroup(mediaID, "family", second).events...)
	owner, found, err := lastMediaOwner(events)
	if err != nil || !found || owner != (mediaOwner{entityType: "family", id: second}) {
		t.Errorf("lastMediaOwner = %+v, %v, %v; want the last create's family owner", owner, found, err)
	}
	if _, found, err := lastMediaOwner([]repository.StoredEvent{{EventType: "MediaUpdated"}}); found || err != nil {
		t.Errorf("history without a create: found=%v err=%v, want neither", found, err)
	}
	if _, _, err := lastMediaOwner([]repository.StoredEvent{{EventType: "MediaCreated", Data: []byte(`{not json`)}}); err == nil {
		t.Errorf("undecodable create: want an error")
	}
}

// One scan of main's person merges serves both repair consumers; each narrows
// it to the merges after its own start point.
func TestPersonMergeScanSharing(t *testing.T) {
	cases := []struct{ a, b, want int64 }{{-1, -1, -1}, {-1, 5, 5}, {5, -1, 5}, {3, 5, 3}, {5, 3, 3}}
	for _, tc := range cases {
		if got := earliestScan(tc.a, tc.b); got != tc.want {
			t.Errorf("earliestScan(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}

	a, b, c := uuid.New(), uuid.New(), uuid.New()
	merges := []personMerge{{mergedID: a, survivorID: b, position: 10}, {mergedID: b, survivorID: c, position: 20}}
	if got := survivorsAfter(merges, 15); len(got) != 1 || got[b].survivorID != c {
		t.Errorf("survivorsAfter(15) = %v, want only b→c", got)
	}
	if got := survivorsAfter(merges, -1); len(got) != 0 {
		t.Errorf("survivorsAfter(-1) = %v, want empty", got)
	}
	if got := mergedAwayAfter([]uuid.UUID{a, b}, 15, merges); len(got) != 1 || !got[b] {
		t.Errorf("mergedAwayAfter(15) = %v, want only b", got)
	}
	if got := mergedAwayAfter(nil, 0, merges); got != nil {
		t.Errorf("mergedAwayAfter with no candidates = %v, want nil", got)
	}
}
