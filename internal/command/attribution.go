package command

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// attributingEventStore is the event store every Handler appends through
// (#1407). When the context carries an actor (domain.ContextWithActor),
// Append stamps each event with that user ID in its envelope metadata, so
// every command records who made the change without any command signature
// naming the actor. An event that is already a domain.StampedEvent passes
// through untouched: its caller set the metadata explicitly (a merge replay
// keeps the branch event's original author), and that metadata wins. Without
// an actor the events pass through unchanged.
//
// Every other method delegates to the wrapped store.
type attributingEventStore struct {
	repository.EventStore
}

// withAttribution wraps store in an attributingEventStore, once: an already
// attributing store is returned as is.
func withAttribution(store repository.EventStore) repository.EventStore {
	if store == nil {
		return nil
	}
	if _, ok := store.(attributingEventStore); ok {
		return store
	}
	return attributingEventStore{EventStore: store}
}

// Append stamps events with the context's actor, then appends them.
func (s attributingEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	if actor := domain.ActorFromContext(ctx); actor != "" {
		attributed := make([]domain.Event, len(events))
		for i, event := range events {
			if _, stamped := event.(domain.StampedEvent); stamped {
				attributed[i] = event
				continue
			}
			attributed[i] = domain.Stamp(event, domain.EventMetadata{UserID: actor}, time.Time{})
		}
		events = attributed
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}
