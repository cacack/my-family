package domain

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUnwrapEvent_PlainEvent(t *testing.T) {
	event := NewPersonUpdated(uuid.New(), map[string]any{"surname": "Byron"})
	inner, meta, at := UnwrapEvent(event)
	if got, ok := inner.(PersonUpdated); !ok || got.ID != event.ID || meta != nil || !at.Equal(event.Timestamp) {
		t.Errorf("UnwrapEvent(plain) = %v, %v, %s; want the event itself, no metadata, its OccurredAt", inner, meta, at)
	}
}

func TestStamp_DelegatesAndUnwraps(t *testing.T) {
	event := NewPersonUpdated(uuid.New(), map[string]any{"surname": "Byron"})
	recorded := event.Timestamp.Add(time.Hour)
	prov := &MergeProvenance{BranchID: uuid.New(), BranchName: "Byron theory", Note: "why"}
	stamped := Stamp(event, EventMetadata{MergedFromBranch: prov}, recorded)

	if stamped.EventType() != "PersonUpdated" || stamped.AggregateID() != event.PersonID || !stamped.OccurredAt().Equal(event.Timestamp) {
		t.Errorf("stamped event does not delegate: %s %s %s", stamped.EventType(), stamped.AggregateID(), stamped.OccurredAt())
	}
	inner, meta, at := UnwrapEvent(stamped)
	if _, ok := inner.(PersonUpdated); !ok {
		t.Fatalf("inner = %T, want PersonUpdated", inner)
	}
	if meta == nil || meta.MergedFromBranch != prov || !at.Equal(recorded) {
		t.Errorf("UnwrapEvent(stamped) = %v, %s; want the stamp's metadata and record time", meta, at)
	}

	// A stamp without metadata or record time stores neither.
	inner, meta, at = UnwrapEvent(Stamp(Stamp(event, EventMetadata{}, time.Time{}), EventMetadata{}, time.Time{}))
	if _, ok := inner.(PersonUpdated); !ok || meta != nil || !at.Equal(event.Timestamp) {
		t.Errorf("empty stamp = %T, %v, %s; want the bare event", inner, meta, at)
	}
}

func TestEncodeForStore(t *testing.T) {
	event := NewPersonUpdated(uuid.New(), map[string]any{"surname": "Byron"})
	want, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	data, meta, _, err := EncodeForStore(Stamp(event, EventMetadata{UserID: "u1"}, time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, want) {
		t.Errorf("payload = %s, want the inner event's %s", data, want)
	}
	if string(meta) != `{"user_id":"u1"}` {
		t.Errorf("metadata = %s", meta)
	}
	if _, meta, _, _ = EncodeForStore(event); meta != nil {
		t.Errorf("plain event metadata = %s, want nil", meta)
	}
	if _, _, _, err := EncodeForStore(Stamp(badEvent{}, EventMetadata{}, time.Time{})); err == nil {
		t.Error("an unencodable event encoded")
	}
}

// badEvent cannot be JSON-encoded.
type badEvent struct {
	BaseEvent
	C chan int `json:"c"`
}

func (badEvent) EventType() string      { return "Bad" }
func (badEvent) AggregateID() uuid.UUID { return uuid.Nil }

// A claim written before #832 carries no merge record and still decodes, and
// one written since round-trips its record.
func TestBranchMerged_RecordIsAdditive(t *testing.T) {
	branchID, stream := uuid.New(), uuid.New()
	legacy := `{"id":"` + uuid.NewString() + `","timestamp":"2025-01-01T00:00:00Z","branch_id":"` + branchID.String() +
		`","base_position":3,"merged_at_position":9,"note":"old","replay_stream_versions":{"` + stream.String() + `":1},` +
		`"resolution_rationales":{"` + stream.String() + `":"register"}}`
	var old BranchMerged
	if err := json.Unmarshal([]byte(legacy), &old); err != nil {
		t.Fatalf("decoding a pre-#832 claim: %v", err)
	}
	if old.HasRecord() || old.Resolutions != nil || old.Exclusions != nil || old.SkippedStreamIDs != nil {
		t.Errorf("pre-#832 claim decoded with a record: %+v", old)
	}
	if old.Note != "old" || old.ResolutionRationales[stream] != "register" || old.ReplayStreamVersions[stream] != 1 {
		t.Errorf("pre-#832 claim lost data: %+v", old)
	}

	claim := NewBranchMerged(branchID, 3, 9, "new", map[uuid.UUID]int64{stream: 1})
	count := 0
	claim.ReplayedEventCount = &count
	claim.Resolutions = []MergeDecision{{StreamID: stream, Kind: "edit_edit", Fields: []string{"surname"}, Resolution: "main", Rationale: "r"}}
	claim.Exclusions = []MergeExclusion{{StreamID: uuid.New(), EntityType: "person", EntityName: "Ada"}}
	claim.SkippedStreamIDs = []uuid.UUID{stream}
	raw, err := json.Marshal(claim)
	if err != nil {
		t.Fatal(err)
	}
	var back BranchMerged
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !back.HasRecord() || *back.ReplayedEventCount != 0 || len(back.Resolutions) != 1 || back.Resolutions[0].Fields[0] != "surname" ||
		len(back.Exclusions) != 1 || back.Exclusions[0].EntityName != "Ada" || len(back.SkippedStreamIDs) != 1 {
		t.Errorf("record did not round-trip: %s", raw)
	}
}

func TestEventMetadata_IsZero(t *testing.T) {
	if !(EventMetadata{}).IsZero() {
		t.Error("empty metadata is not zero")
	}
	for _, m := range []EventMetadata{{CorrelationID: "c"}, {CausationID: "c"}, {UserID: "u"}, {MergedFromBranch: &MergeProvenance{}}} {
		if m.IsZero() {
			t.Errorf("%+v reported zero", m)
		}
	}
}
