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
		if err := (&Handler{}).checkGPSSubjectSurvives(context.Background(), group, nil, nil, false); err == nil ||
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
