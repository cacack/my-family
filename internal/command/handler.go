// Package command provides CQRS command handlers for the genealogy application.
package command

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// Rollback-related errors.
var (
	ErrRollbackInvalidVersion = errors.New("invalid rollback version: must be positive and less than current version")
	ErrRollbackDeletedEntity  = errors.New("cannot rollback a deleted entity")
	ErrRollbackNoChanges      = errors.New("rollback to current version is a no-op")
)

// RollbackResult contains the result of a rollback operation.
type RollbackResult struct {
	EntityID   uuid.UUID      `json:"entity_id"`
	EntityType string         `json:"entity_type"`
	NewVersion int64          `json:"new_version"`
	Changes    map[string]any `json:"changes"`
}

// Handler processes commands and returns resulting domain events.
type Handler struct {
	eventStore      repository.EventStore
	readStore       repository.ReadModelStore
	projector       *repository.Projector
	rollbackService *query.RollbackService
}

// NewHandler creates a new command handler.
func NewHandler(eventStore repository.EventStore, readStore repository.ReadModelStore) *Handler {
	return &Handler{
		eventStore:      eventStore,
		readStore:       readStore,
		projector:       repository.NewProjector(readStore),
		rollbackService: query.NewRollbackService(eventStore, readStore),
	}
}

// NewHandlerWithRollbackService creates a new command handler with a custom rollback service.
// This is primarily useful for testing.
func NewHandlerWithRollbackService(eventStore repository.EventStore, readStore repository.ReadModelStore, rollbackService *query.RollbackService) *Handler {
	return &Handler{
		eventStore:      eventStore,
		readStore:       readStore,
		projector:       repository.NewProjector(readStore),
		rollbackService: rollbackService,
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
			// Projection can be rebuilt; ignore non-critical errors
			_ = err
		}
	}

	return newVersion, nil
}

// RollbackPerson rolls back a person to a specific version.
// It computes the changes needed and generates a compensating PersonUpdated event.
func (h *Handler) RollbackPerson(ctx context.Context, personID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Person", personID, targetVersion, func(id uuid.UUID) (bool, error) {
		person, err := h.readStore.GetPerson(ctx, id)
		if err != nil {
			return false, err
		}
		return person == nil, nil
	})
}

// RollbackFamily rolls back a family to a specific version.
// It computes the changes needed and generates a compensating FamilyUpdated event.
func (h *Handler) RollbackFamily(ctx context.Context, familyID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Family", familyID, targetVersion, func(id uuid.UUID) (bool, error) {
		family, err := h.readStore.GetFamily(ctx, id)
		if err != nil {
			return false, err
		}
		return family == nil, nil
	})
}

// RollbackSource rolls back a source to a specific version.
// It computes the changes needed and generates a compensating SourceUpdated event.
func (h *Handler) RollbackSource(ctx context.Context, sourceID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Source", sourceID, targetVersion, func(id uuid.UUID) (bool, error) {
		source, err := h.readStore.GetSource(ctx, id)
		if err != nil {
			return false, err
		}
		return source == nil, nil
	})
}

// RollbackCitation rolls back a citation to a specific version.
// It computes the changes needed and generates a compensating CitationUpdated event.
func (h *Handler) RollbackCitation(ctx context.Context, citationID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Citation", citationID, targetVersion, func(id uuid.UUID) (bool, error) {
		citation, err := h.readStore.GetCitation(ctx, id)
		if err != nil {
			return false, err
		}
		return citation == nil, nil
	})
}

// rollbackEntity is a generic helper that handles the rollback logic for any entity type.
// The isDeleted function checks if the entity is currently deleted in the read model.
func (h *Handler) rollbackEntity(ctx context.Context, entityType string, entityID uuid.UUID, targetVersion int64, isDeleted func(uuid.UUID) (bool, error)) (*RollbackResult, error) {
	// Validate target version is positive
	if targetVersion < 1 {
		return nil, ErrRollbackInvalidVersion
	}

	// Get current version from event store
	currentVersion, err := h.eventStore.GetStreamVersion(ctx, entityID)
	if err != nil {
		if errors.Is(err, repository.ErrStreamNotFound) {
			return nil, query.ErrNoEvents
		}
		return nil, err
	}

	// Validate target version is less than current version
	if targetVersion >= currentVersion {
		if targetVersion == currentVersion {
			return nil, ErrRollbackNoChanges
		}
		return nil, ErrRollbackInvalidVersion
	}

	// Check if entity is currently deleted (follow-up feature to handle recreation)
	deleted, err := isDeleted(entityID)
	if err != nil {
		return nil, err
	}
	if deleted {
		return nil, ErrRollbackDeletedEntity
	}

	// Compute rollback changes using RollbackService
	changes, err := h.rollbackService.ComputeRollbackChanges(ctx, entityType, entityID, targetVersion)
	if err != nil {
		return nil, err
	}

	// If no changes needed, this is a no-op
	if len(changes.Changes) == 0 {
		return &RollbackResult{
			EntityID:   entityID,
			EntityType: entityType,
			NewVersion: currentVersion,
			Changes:    changes.Changes,
		}, nil
	}

	// Create compensating event based on entity type
	var event domain.Event
	switch entityType {
	case "Person":
		event = domain.NewPersonUpdated(entityID, changes.Changes)
	case "Family":
		event = domain.NewFamilyUpdated(entityID, changes.Changes)
	case "Source":
		event = domain.NewSourceUpdated(entityID, changes.Changes)
	case "Citation":
		event = domain.NewCitationUpdated(entityID, changes.Changes)
	case "Media":
		event = domain.NewMediaUpdated(entityID, changes.Changes)
	default:
		return nil, errors.New("unsupported entity type for rollback: " + entityType)
	}

	// Append event with optimistic locking
	newVersion, err := h.execute(ctx, entityID.String(), entityType, []domain.Event{event}, currentVersion)
	if err != nil {
		return nil, err
	}

	return &RollbackResult{
		EntityID:   entityID,
		EntityType: entityType,
		NewVersion: newVersion,
		Changes:    changes.Changes,
	}, nil
}
