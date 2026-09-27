package command

// Shapes of the media-owner rule and the media repair that only a hand-built
// replay can reach: a claim written before #685 has no plan, so its resume can
// meet streams already on main that a planned merge would never have landed in
// that order.

import (
	"context"
	"errors"
	"testing"

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

// A media upload already on main may not lose the owner the replay creates to
// this call's own "main" resolution; any other resolution, or an owner main
// has, is fine.
func TestCheckLandedMediaOwners(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	h := &Handler{readStore: store}
	mediaID, owner := uuid.New(), uuid.New()
	upload := mediaUploadGroup(mediaID, "person", owner)
	creating := personGroup(owner, "PersonCreated")
	groups := []streamGroup{creating, upload}
	byID := map[uuid.UUID]streamGroup{owner: creating, mediaID: upload}
	view := resumeView{landed: map[uuid.UUID]bool{mediaID: true}}

	err := h.checkLandedMediaOwners(ctx, groups, byID, view, map[uuid.UUID]MergeResolution{owner: ResolveMain})
	if !errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("excluding the landed upload's owner: err = %v, want ErrMergeDanglingReference", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, byID, view, map[uuid.UUID]MergeResolution{owner: ResolveBranch}); err != nil {
		t.Errorf("replaying the owner: err = %v, want nil", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, byID, resumeView{}, map[uuid.UUID]MergeResolution{owner: ResolveMain}); err != nil {
		t.Errorf("upload not on main: err = %v, want nil", err)
	}

	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: owner, GivenName: "Owen", Version: 1}); err != nil {
		t.Fatalf("SavePerson failed: %v", err)
	}
	if err := h.checkLandedMediaOwners(ctx, groups, byID, view, map[uuid.UUID]MergeResolution{owner: ResolveMain}); err != nil {
		t.Errorf("owner main already has: err = %v, want nil", err)
	}

	bad := streamGroup{streamID: mediaID, streamType: "Media", events: []repository.StoredEvent{{StreamID: mediaID, EventType: "MediaCreated", Data: []byte(`{not json`)}}}
	if err := h.checkLandedMediaOwners(ctx, []streamGroup{bad}, byID, view, nil); err == nil || errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("undecodable landed upload: err = %v, want a decode error", err)
	}
}

func TestFinalSurvivor(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	if got := finalSurvivor(a, map[uuid.UUID]uuid.UUID{a: b, b: c}); got != c {
		t.Errorf("chain a→b→c ends at %s, want c", got)
	}
	if got := finalSurvivor(c, map[uuid.UUID]uuid.UUID{a: b}); got != c {
		t.Errorf("unmerged person ends at %s, want itself", got)
	}
	// A cycle cannot come from the command layer; it must still terminate.
	_ = finalSurvivor(a, map[uuid.UUID]uuid.UUID{a: b, b: a})
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
	if got := survivorsAfter(merges, 15); len(got) != 1 || got[b] != c {
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
