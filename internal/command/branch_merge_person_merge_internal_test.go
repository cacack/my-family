package command

// The person-merge replay helpers (#834) read stored payloads directly, so
// their ordering and decoding rules are tested here on hand-built groups.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// mergeGroup is a survivor's stream carrying one PersonMerged at position.
func mergeGroup(survivor, merged uuid.UUID, position int64) streamGroup {
	return streamGroup{streamID: survivor, streamType: "Person", events: []repository.StoredEvent{{
		StreamID: survivor, EventType: "PersonMerged", Position: position,
		Data: []byte(`{"survivor_id":"` + survivor.String() + `","merged_id":"` + merged.String() + `"}`),
	}}}
}

// plainGroup is a stream whose one event's payload carries the given text.
func plainGroup(streamID uuid.UUID, streamType, payload string, position int64) streamGroup {
	return streamGroup{streamID: streamID, streamType: streamType, events: []repository.StoredEvent{{
		StreamID: streamID, EventType: streamType + "Updated", Position: position, Data: []byte(payload),
	}}}
}

// createdGroup is a stream the branch created, whose creation payload carries
// the given text.
func createdGroup(streamID uuid.UUID, streamType, payload string, position int64) streamGroup {
	return streamGroup{streamID: streamID, streamType: streamType, events: []repository.StoredEvent{{
		StreamID: streamID, EventType: streamType + "Created", Position: position, Data: []byte(payload),
	}}}
}

// survivorCreatedGroup is a survivor's stream the branch created at created
// and merged a person into at mergedAt.
func survivorCreatedGroup(survivor, merged uuid.UUID, created, mergedAt int64) streamGroup {
	group := mergeGroup(survivor, merged, mergedAt)
	group.events = append([]repository.StoredEvent{{
		StreamID: survivor, EventType: "PersonCreated", Position: created, Data: []byte(`{}`),
	}}, group.events...)
	return group
}

func streamOrder(groups []streamGroup) []uuid.UUID {
	ids := make([]uuid.UUID, len(groups))
	for i, g := range groups {
		ids[i] = g.streamID
	}
	return ids
}

func TestOrderPersonMergesForReplay(t *testing.T) {
	survivor, merged, family, unrelated := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	t.Run("no merges keeps the order", func(t *testing.T) {
		groups := []streamGroup{plainGroup(family, "Family", `{}`, 1), plainGroup(unrelated, "Person", `{}`, 2)}
		got, err := orderPersonMergesForReplay(groups)
		if err != nil || len(got) != 2 || got[0].streamID != family {
			t.Fatalf("order = %v (err=%v), want unchanged", streamOrder(got), err)
		}
	})

	t.Run("merge moves after every stream that mentions the merged person", func(t *testing.T) {
		groups := []streamGroup{
			mergeGroup(survivor, merged, 5),
			plainGroup(merged, "Person", `{}`, 2),
			plainGroup(unrelated, "Person", `{}`, 3),
			plainGroup(family, "Family", `{"partner1_id":"`+merged.String()+`"}`, 4),
			plainGroup(uuid.Nil, "Note", `{}`, 6),
		}
		got, err := orderPersonMergesForReplay(groups)
		if err != nil {
			t.Fatal(err)
		}
		want := []uuid.UUID{merged, unrelated, family, survivor, uuid.Nil}
		if order := streamOrder(got); !equalIDs(order, want) {
			t.Errorf("order = %v, want %v", order, want)
		}
	})

	t.Run("a merge already after its references stays put", func(t *testing.T) {
		groups := []streamGroup{plainGroup(merged, "Person", `{}`, 1), mergeGroup(survivor, merged, 2)}
		got, err := orderPersonMergesForReplay(groups)
		if err != nil || !equalIDs(streamOrder(got), []uuid.UUID{merged, survivor}) {
			t.Errorf("order = %v (err=%v), want unchanged", streamOrder(got), err)
		}
	})

	t.Run("a survivor the branch created lands before the streams that name it", func(t *testing.T) {
		// Branch: create S, create family F with S as partner, create family F2
		// with M as partner, merge M into S. Moving S's stream (creation and
		// merge together) after F2 would jump over F, which would then land
		// before S exists. The merge's references move before it instead.
		created := survivorCreatedGroup(survivor, merged, 1, 4)
		other := uuid.New()
		groups := []streamGroup{
			created,
			createdGroup(family, "Family", `{"partner1_id":"`+survivor.String()+`"}`, 2),
			createdGroup(other, "Family", `{"partner1_id":"`+merged.String()+`"}`, 3),
		}
		got, err := orderPersonMergesForReplay(groups)
		if err != nil {
			t.Fatal(err)
		}
		want := []uuid.UUID{other, survivor, family}
		if order := streamOrder(got); !equalIDs(order, want) {
			t.Errorf("order = %v, want %v", order, want)
		}
	})

	t.Run("a moved reference stays after what it needs the branch to create", func(t *testing.T) {
		partner := uuid.New()
		groups := []streamGroup{
			mergeGroup(survivor, merged, 4),
			createdGroup(partner, "Person", `{}`, 2),
			createdGroup(family, "Family", `{"partner1_id":"`+merged.String()+`","partner2_id":"`+partner.String()+`"}`, 3),
		}
		got, err := orderPersonMergesForReplay(groups)
		if err != nil {
			t.Fatal(err)
		}
		want := []uuid.UUID{partner, family, survivor}
		if order := streamOrder(got); !equalIDs(order, want) {
			t.Errorf("order = %v, want %v", order, want)
		}
	})

	t.Run("a stream that needs the created survivor and names the merged person is refused", func(t *testing.T) {
		association := uuid.New()
		groups := []streamGroup{
			survivorCreatedGroup(survivor, merged, 1, 3),
			createdGroup(association, "Association",
				`{"person_id":"`+survivor.String()+`","associate_id":"`+merged.String()+`"}`, 2),
		}
		if _, err := orderPersonMergesForReplay(groups); !errors.Is(err, ErrMergeDanglingReference) {
			t.Errorf("err = %v, want ErrMergeDanglingReference", err)
		}
	})

	t.Run("a malformed merge is an error", func(t *testing.T) {
		bad := mergeGroup(survivor, merged, 1)
		bad.events[0].Data = []byte("not json")
		if _, err := orderPersonMergesForReplay([]streamGroup{bad}); err == nil {
			t.Error("want a decoding error")
		}
		if _, err := branchPersonMerges([]streamGroup{bad}); err == nil {
			t.Error("branchPersonMerges: want a decoding error")
		}
		if _, err := personMergeSurvivor(bad.events[0]); err == nil {
			t.Error("personMergeSurvivor: want a decoding error")
		}
	})
}

func equalIDs(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFinalMergeSurvivor(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	into := map[uuid.UUID]uuid.UUID{a: b, b: c}
	if got, ok := finalMergeSurvivor(a, into); !ok || got != c {
		t.Errorf("finalMergeSurvivor(a) = %s, %v, want c", got, ok)
	}
	if _, ok := finalMergeSurvivor(c, into); ok {
		t.Error("finalMergeSurvivor(c) reports a merge for a person never merged away")
	}
	// A malformed cycle terminates.
	if _, ok := finalMergeSurvivor(a, map[uuid.UUID]uuid.UUID{a: b, b: a}); !ok {
		t.Error("a cycle still reports the person as merged")
	}
	if survivorLookup(nil) != nil {
		t.Error("survivorLookup(nil) should follow nothing")
	}
}

func TestPendingPersonMerges_SkipsLandedMerges(t *testing.T) {
	s1, m1, s2, m2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	replayed := map[uuid.UUID]streamGroup{s1: mergeGroup(s1, m1, 1), s2: mergeGroup(s2, m2, 2)}
	got, err := pendingPersonMerges(replayed, map[uuid.UUID]bool{s1: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[m1]; ok {
		t.Error("a merge already on main vouches for a reference landing now")
	}
	if got[m2] != s2 {
		t.Errorf("pending merge of m2 = %s, want s2", got[m2])
	}
}

func TestValidateMergedPersonsNotStale_RefusesAMissingPin(t *testing.T) {
	survivor, merged := uuid.New(), uuid.New()
	h := &Handler{}
	plan := &query.MergePlan{Branch: &domain.Branch{ID: uuid.New()}, MainStreamVersions: map[uuid.UUID]int64{survivor: 1}}
	groups := []streamGroup{mergeGroup(survivor, merged, 1)}

	err := h.validateMergedPersonsNotStale(context.Background(), plan, groups, nil)
	if !errors.Is(err, ErrMergePlanIncomplete) {
		t.Fatalf("err = %v, want ErrMergePlanIncomplete", err)
	}
	// A survivor stream resolved to main replays nothing, so its merged person
	// needs no pin.
	if err := h.validateMergedPersonsNotStale(context.Background(), plan, groups,
		map[uuid.UUID]MergeResolution{survivor: ResolveMain}); err != nil {
		t.Errorf("resolved to main: err = %v, want nil", err)
	}
}

func TestPersonReferences_PersonMergedNamesTheSurvivor(t *testing.T) {
	survivor, merged := uuid.New(), uuid.New()
	got, err := personReferences(mergeGroup(survivor, merged, 1).events[0])
	if err != nil || !equalIDs(got, []uuid.UUID{survivor}) {
		t.Errorf("personReferences = %v (err=%v), want the survivor only", got, err)
	}
}
