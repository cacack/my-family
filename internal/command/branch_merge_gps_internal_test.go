package command

// Malformed-input paths of the merge's GPS artifact checks (#760). The events
// the command layer writes always decode and always use a known stream type, so
// these refusals are only reachable with a hand-built replay group.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

func TestGPSOutcomeOf_MalformedInput(t *testing.T) {
	streamID := uuid.New()
	for _, tc := range []struct{ eventType, data string }{
		{"EvidenceAnalysisCreated", `{not json`},
		{"ResearchLogUpdated", `{not json`},
		{"ProofSummaryUpdated", `{"changes":{"subject_id":"not-a-uuid"}}`},
	} {
		group := streamGroup{streamID: streamID, streamType: "EvidenceAnalysis", events: []repository.StoredEvent{
			{StreamID: streamID, EventType: tc.eventType, Data: []byte(tc.data)},
		}}
		if _, err := gpsOutcomeOf(group); err == nil {
			t.Errorf("%s %s: err = nil, want a decode error", tc.eventType, tc.data)
		}
		if err := (&Handler{}).checkGPSSubjectSurvives(context.Background(), group, evidencePlan{}); err == nil ||
			errors.Is(err, ErrMergeDanglingReference) {
			t.Errorf("%s %s: checkGPSSubjectSurvives err = %v, want a decode error", tc.eventType, tc.data, err)
		}
	}
}

func TestGPSArtifactOnMain_UnknownStreamType(t *testing.T) {
	_, err := (&Handler{}).gpsArtifactOnMain(context.Background(), "Planet", uuid.New())
	if !errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("unknown stream type: err = %v, want ErrMergeDanglingReference", err)
	}
}

// mainWriteAfterLanding finds the landed replay's own events on main by their
// payload ids and reports main's first later write to the stream; a landed
// stream whose events main's log does not hold, or an event without an id, is
// an error rather than a verdict.
func TestMainWriteAfterLanding(t *testing.T) {
	stream := uuid.New()
	evt := func(id uuid.UUID, position int64) repository.StoredEvent {
		return repository.StoredEvent{
			StreamID: stream, EventType: "EvidenceAnalysisUpdated", Position: position,
			Data: []byte(`{"id":"` + id.String() + `"}`),
		}
	}
	before, ours1, ours2, after := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	group := streamGroup{streamID: stream, streamType: "EvidenceAnalysis", events: []repository.StoredEvent{evt(ours1, 0), evt(ours2, 0)}}

	got, err := mainWriteAfterLanding(group, []repository.StoredEvent{evt(before, 3), evt(ours1, 7), evt(ours2, 8)})
	if err != nil || got != nil {
		t.Errorf("no write after landing: got %v, err %v; want nil, nil", got, err)
	}
	got, err = mainWriteAfterLanding(group, []repository.StoredEvent{evt(before, 3), evt(ours1, 7), evt(ours2, 8), evt(after, 12)})
	if err != nil || got == nil || got.Position != 12 {
		t.Errorf("write after landing: got %v, err %v; want the event at position 12", got, err)
	}
	if _, err := mainWriteAfterLanding(group, []repository.StoredEvent{evt(before, 3)}); err == nil {
		t.Error("landed events missing from main: err = nil, want an error")
	}
	noID := repository.StoredEvent{StreamID: stream, EventType: "EvidenceAnalysisUpdated", Position: 4, Data: []byte(`{}`)}
	if _, err := mainWriteAfterLanding(group, []repository.StoredEvent{noID}); err == nil {
		t.Error("main event without an id: err = nil, want an error")
	}
	if _, err := mainWriteAfterLanding(streamGroup{streamID: stream, events: []repository.StoredEvent{noID}}, nil); err == nil {
		t.Error("replayed event without an id: err = nil, want an error")
	}
}
