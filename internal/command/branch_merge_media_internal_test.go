package command

// Malformed-input paths of the merge's media owner check (#759). The events the
// command layer writes always decode and always carry a valid owner type, so
// these refusals are only reachable with a hand-built replay group.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
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
