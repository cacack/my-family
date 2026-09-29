package command

// personReferences decodes stored payloads directly, so the shapes it must read
// the way the projection does — a FamilyUpdated that clears a partner, or
// carries a value the projection stores as "no partner" — are tested here on
// hand-built events: no command writes them today.

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

func TestPersonReferences_FamilyPartners(t *testing.T) {
	p1, p2 := uuid.New(), uuid.New()
	cases := []struct {
		name      string
		eventType string
		data      string
		want      []uuid.UUID
	}{
		{"created with both partners", "FamilyCreated",
			`{"family_id":"` + uuid.NewString() + `","partner1_id":"` + p1.String() + `","partner2_id":"` + p2.String() + `"}`,
			[]uuid.UUID{p1, p2}},
		{"created with one partner", "FamilyCreated", `{"partner2_id":"` + p2.String() + `"}`, []uuid.UUID{p2}},
		{"created with no partner", "FamilyCreated", `{"family_id":"` + uuid.NewString() + `"}`, nil},
		{"update sets both partners", "FamilyUpdated",
			`{"changes":{"partner1_id":"` + p1.String() + `","partner2_id":"` + p2.String() + `"}}`,
			[]uuid.UUID{p1, p2}},
		{"update clears a partner with null", "FamilyUpdated", `{"changes":{"partner1_id":null}}`, nil},
		{"update clears a partner with an empty string", "FamilyUpdated", `{"changes":{"partner2_id":""}}`, nil},
		{"update with a value the projection ignores", "FamilyUpdated", `{"changes":{"partner1_id":"not-a-uuid","partner2_id":7}}`, nil},
		{"update not touching partners", "FamilyUpdated", `{"changes":{"marriage_place":"Paris"}}`, nil},
		{"update with no changes", "FamilyUpdated", `{}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := personReferences(repository.StoredEvent{EventType: tc.eventType, Data: []byte(tc.data)})
			if err != nil {
				t.Fatalf("personReferences failed: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("personReferences = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPersonReferences_MalformedFamilyPayload(t *testing.T) {
	for _, eventType := range []string{"FamilyCreated", "FamilyUpdated"} {
		if _, err := personReferences(repository.StoredEvent{EventType: eventType, Data: []byte(`{`)}); err == nil {
			t.Errorf("%s: personReferences accepted a malformed payload", eventType)
		}
	}
}

// blockErr runs a blocker-collecting check and folds what it found into the
// refusal a merge would return, so a test can assert on one error.
func blockErr(check func(*blockerList) error) error {
	var list blockerList
	if err := check(&list); err != nil {
		return err
	}
	if list.len() == 0 {
		return nil
	}
	return &MergeBlockedError{Blockers: list.items}
}

// includeCandidates offers "include" only when a "main" resolution of a stream
// that would bring the referenced entity back is what excluded it, and the
// stream accepts "branch".
func TestIncludeCandidates(t *testing.T) {
	person, deleted, conflicted, family := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	groups := []streamGroup{
		{streamID: person, streamType: "Person", events: []repository.StoredEvent{{EventType: "PersonCreated"}}},
		{streamID: deleted, streamType: "Person", events: []repository.StoredEvent{{EventType: "PersonCreated"}, {EventType: "PersonDeleted"}}},
		{streamID: conflicted, streamType: "Person", events: []repository.StoredEvent{{EventType: "PersonCreated"}}},
	}
	resolutions := map[uuid.UUID]MergeResolution{person: ResolveMain, deleted: ResolveMain, conflicted: ResolveMain}
	conflicts := []query.MergeConflict{{StreamID: conflicted, SupportedResolutions: []string{"main"}}}
	blockers := []MergeBlocker{
		{StreamID: family, ReferencedID: person, Kind: BlockerMissingPerson},
		{StreamID: family, ReferencedID: deleted, Kind: BlockerMissingPerson},
		{StreamID: family, ReferencedID: conflicted, Kind: BlockerMissingPerson},
		{StreamID: family, ReferencedID: uuid.New(), Kind: BlockerMissingPerson},
		{StreamID: person, ReferencedID: family, Kind: BlockerOwnerDeleteOrphansMedia},
	}
	got := includeCandidates(blockers, groups, resolutions, conflicts)
	want := []bool{true, false, false, false, false}
	if !slices.Equal(got, want) {
		t.Errorf("includeCandidates = %v, want %v", got, want)
	}
	if leavesEntityInExistence(streamGroup{}) {
		t.Error("an empty stream leaves nothing in existence")
	}
	if !leavesEntityInExistence(streamGroup{events: []repository.StoredEvent{{EventType: "EvidenceConflictDetected"}}}) {
		t.Error("a detected evidence conflict exists")
	}
}

// strayEventStore answers every multi-stream read with an event of a stream
// nobody asked for.
type strayEventStore struct{ repository.EventStore }

func (strayEventStore) ReadStreamsForBranch(context.Context, []uuid.UUID, domain.BranchID, int64, int) ([]repository.StoredEvent, error) {
	return []repository.StoredEvent{{StreamID: uuid.New(), EventType: "MediaUpdated"}}, nil
}

func TestFirstMainWritesAfter_RefusesAStrayStream(t *testing.T) {
	h := &Handler{eventStore: strayEventStore{}}
	if _, err := h.firstMainWritesAfter(context.Background(), []uuid.UUID{uuid.New()}, 0); err == nil {
		t.Error("a stream that was not asked for must be an error, not a loop")
	}
}

func TestBlockerListDropsRepeats(t *testing.T) {
	var list blockerList
	b := MergeBlocker{StreamID: uuid.New(), ReferencedID: uuid.New(), Kind: BlockerMissingSource}
	list.add(b)
	list.add(b)
	if list.len() != 1 || list.items[0].SuggestedResolution != FixLeaveOut {
		t.Errorf("list = %+v, want one blocker defaulting to leave_out", list.items)
	}
}
