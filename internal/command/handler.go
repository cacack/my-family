// Package command provides CQRS command handlers for the genealogy application.
package command

import (
	"context"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Handler processes commands and returns resulting domain events.
type Handler struct {
	eventStore repository.EventStore
	readStore  repository.ReadModelStore
	projector  *repository.Projector
}

// NewHandler creates a new command handler.
func NewHandler(eventStore repository.EventStore, readStore repository.ReadModelStore) *Handler {
	return &Handler{
		eventStore: eventStore,
		readStore:  readStore,
		projector:  repository.NewProjector(readStore),
	}
}

// execute is a helper that appends events, projects them, and returns the new version.
func (h *Handler) execute(ctx context.Context, streamID string, streamType string, events []domain.Event, expectedVersion int64) (int64, error) {
	// Parse stream ID as UUID
	id, err := parseUUID(streamID)
	if err != nil {
		return 0, err
	}

	// Append events to event store
	if err := h.eventStore.Append(ctx, id, streamType, events, expectedVersion); err != nil {
		return 0, err
	}

	// Project events to read model (synchronous for MVP)
	newVersion := expectedVersion
	if expectedVersion < 0 {
		newVersion = 0
	}
	for _, event := range events {
		newVersion++
		if err := h.projector.Project(ctx, event, newVersion); err != nil {
			// Log but don't fail - projection can be rebuilt
			// In production, this should be handled more robustly
		}
	}

	return newVersion, nil
}
