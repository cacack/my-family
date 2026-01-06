// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// HistoryService provides query operations for change history and audit trails.
type HistoryService struct {
	eventStore repository.EventStore
	readStore  repository.ReadModelStore
}

// NewHistoryService creates a new history query service.
func NewHistoryService(eventStore repository.EventStore, readStore repository.ReadModelStore) *HistoryService {
	return &HistoryService{
		eventStore: eventStore,
		readStore:  readStore,
	}
}

// ChangeEntry represents a user-friendly change record in the system's history.
type ChangeEntry struct {
	ID         uuid.UUID              `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	EntityType string                 `json:"entity_type"` // "person", "family", "source", "citation", "import"
	EntityID   uuid.UUID              `json:"entity_id"`
	EntityName string                 `json:"entity_name"` // e.g., "John Smith"
	Action     string                 `json:"action"`      // "created", "updated", "deleted", "linked", "unlinked"
	Changes    map[string]FieldChange `json:"changes,omitempty"`
	UserID     *string                `json:"user_id,omitempty"`
}

// FieldChange represents before/after values for a field update.
type FieldChange struct {
	OldValue any `json:"old_value,omitempty"`
	NewValue any `json:"new_value,omitempty"`
}

// ChangeHistoryResult contains paginated change history results.
type ChangeHistoryResult struct {
	Entries    []ChangeEntry `json:"entries"`
	TotalCount int           `json:"total_count"`
	HasMore    bool          `json:"has_more"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
}

// GetGlobalHistoryInput contains options for global history queries.
type GetGlobalHistoryInput struct {
	FromTime   time.Time
	ToTime     time.Time
	EventTypes []string
	Limit      int
	Offset     int
}

// GetEntityHistory retrieves the change history for a specific entity.
func (s *HistoryService) GetEntityHistory(ctx context.Context, entityType string, entityID uuid.UUID, limit, offset int) (*ChangeHistoryResult, error) {
	// Validate inputs
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Read events for this entity's stream
	page, err := s.eventStore.ReadByStream(ctx, entityID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	// Transform events to change entries
	entries, err := s.transformStoredEvents(ctx, page.Events)
	if err != nil {
		return nil, fmt.Errorf("transforming events: %w", err)
	}

	return &ChangeHistoryResult{
		Entries:    entries,
		TotalCount: page.TotalCount,
		HasMore:    page.HasMore,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// GetGlobalHistory retrieves system-wide change history with optional time and type filters.
func (s *HistoryService) GetGlobalHistory(ctx context.Context, input GetGlobalHistoryInput) (*ChangeHistoryResult, error) {
	// Validate inputs
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}
	if input.Offset < 0 {
		input.Offset = 0
	}

	// Read events from event store
	page, err := s.eventStore.ReadGlobalByTime(ctx, input.FromTime, input.ToTime, input.EventTypes, input.Limit, input.Offset)
	if err != nil {
		return nil, fmt.Errorf("reading global history: %w", err)
	}

	// Transform events to change entries
	entries, err := s.transformStoredEvents(ctx, page.Events)
	if err != nil {
		return nil, fmt.Errorf("transforming events: %w", err)
	}

	return &ChangeHistoryResult{
		Entries:    entries,
		TotalCount: page.TotalCount,
		HasMore:    page.HasMore,
		Limit:      input.Limit,
		Offset:     input.Offset,
	}, nil
}

// transformStoredEvents converts raw StoredEvents to user-friendly ChangeEntries.
func (s *HistoryService) transformStoredEvents(ctx context.Context, events []repository.StoredEvent) ([]ChangeEntry, error) {
	entries := make([]ChangeEntry, 0, len(events))

	for _, evt := range events {
		entry := ChangeEntry{
			ID:        evt.ID,
			Timestamp: evt.Timestamp,
			EntityID:  evt.StreamID,
		}

		// Map event type to entity type and action
		entityType, action := s.mapEventTypeToEntityAndAction(evt.EventType)
		entry.EntityType = entityType
		entry.Action = action

		// Extract changes for update events
		if action == "updated" {
			changes, err := s.extractChanges(evt)
			if err == nil && len(changes) > 0 {
				entry.Changes = changes
			}
		}

		// Extract user ID from metadata if present
		if len(evt.Metadata) > 0 {
			var metadata domain.EventMetadata
			if err := json.Unmarshal(evt.Metadata, &metadata); err == nil && metadata.UserID != "" {
				entry.UserID = &metadata.UserID
			}
		}

		// Enrich with entity name from read model
		entityName := s.getEntityName(ctx, entityType, evt.StreamID, &evt)
		entry.EntityName = entityName

		entries = append(entries, entry)
	}

	return entries, nil
}

// mapEventTypeToEntityAndAction maps domain event types to entity types and actions.
func (s *HistoryService) mapEventTypeToEntityAndAction(eventType string) (entityType, action string) {
	switch eventType {
	case "PersonCreated":
		return "person", "created"
	case "PersonUpdated":
		return "person", "updated"
	case "PersonDeleted":
		return "person", "deleted"
	case "FamilyCreated":
		return "family", "created"
	case "FamilyUpdated":
		return "family", "updated"
	case "FamilyDeleted":
		return "family", "deleted"
	case "ChildLinkedToFamily":
		return "family", "linked"
	case "ChildUnlinkedFromFamily":
		return "family", "unlinked"
	case "SourceCreated":
		return "source", "created"
	case "SourceUpdated":
		return "source", "updated"
	case "SourceDeleted":
		return "source", "deleted"
	case "CitationCreated":
		return "citation", "created"
	case "CitationUpdated":
		return "citation", "updated"
	case "CitationDeleted":
		return "citation", "deleted"
	case "GedcomImported":
		return "import", "created"
	default:
		return "unknown", "unknown"
	}
}

// extractChanges extracts field-level changes from update events.
func (s *HistoryService) extractChanges(evt repository.StoredEvent) (map[string]FieldChange, error) {
	// Decode the event to access its Changes field
	domainEvent, err := evt.DecodeEvent()
	if err != nil {
		return nil, err
	}

	// Extract changes based on event type
	switch e := domainEvent.(type) {
	case domain.PersonUpdated:
		return s.convertChangesMap(e.Changes), nil
	case domain.FamilyUpdated:
		return s.convertChangesMap(e.Changes), nil
	case domain.SourceUpdated:
		return s.convertChangesMap(e.Changes), nil
	case domain.CitationUpdated:
		return s.convertChangesMap(e.Changes), nil
	default:
		return nil, nil
	}
}

// convertChangesMap converts domain event changes to FieldChange map.
func (s *HistoryService) convertChangesMap(changes map[string]any) map[string]FieldChange {
	result := make(map[string]FieldChange)
	for field, value := range changes {
		// Handle different change representations
		// For now, we assume the value is the new value
		// A more sophisticated implementation would track old/new pairs
		result[field] = FieldChange{
			NewValue: value,
		}
	}
	return result
}

// getEntityName looks up the display name for an entity from the read model.
func (s *HistoryService) getEntityName(ctx context.Context, entityType string, entityID uuid.UUID, evt *repository.StoredEvent) string {
	switch entityType {
	case "person":
		return s.getPersonName(ctx, entityID, evt)
	case "family":
		return s.getFamilyName(ctx, entityID, evt)
	case "source":
		return s.getSourceName(ctx, entityID, evt)
	case "citation":
		return s.getCitationName(ctx, entityID, evt)
	case "import":
		return s.getImportName(evt)
	default:
		return entityID.String()
	}
}

// getPersonName retrieves or constructs a person's name.
func (s *HistoryService) getPersonName(ctx context.Context, personID uuid.UUID, evt *repository.StoredEvent) string {
	// Try to get from read model first
	person, err := s.readStore.GetPerson(ctx, personID)
	if err == nil && person != nil {
		return person.FullName
	}

	// Fallback: extract name from creation event
	if evt.EventType == "PersonCreated" {
		var created domain.PersonCreated
		if err := json.Unmarshal(evt.Data, &created); err == nil {
			if created.GivenName != "" || created.Surname != "" {
				return fmt.Sprintf("%s %s", created.GivenName, created.Surname)
			}
		}
	}

	// Last resort: use ID
	return personID.String()
}

// getFamilyName retrieves or constructs a family's name.
// TODO: evt parameter reserved for extracting name from event data when read model unavailable
func (s *HistoryService) getFamilyName(ctx context.Context, familyID uuid.UUID, _ *repository.StoredEvent) string {
	// Try to get from read model first
	family, err := s.readStore.GetFamily(ctx, familyID)
	if err == nil && family != nil {
		if family.Partner1Name != "" && family.Partner2Name != "" {
			return fmt.Sprintf("%s & %s", family.Partner1Name, family.Partner2Name)
		}
		if family.Partner1Name != "" {
			return family.Partner1Name
		}
		if family.Partner2Name != "" {
			return family.Partner2Name
		}
	}

	// Fallback: use ID
	return familyID.String()
}

// getSourceName retrieves or constructs a source's name.
func (s *HistoryService) getSourceName(ctx context.Context, sourceID uuid.UUID, evt *repository.StoredEvent) string {
	// Try to get from read model first
	source, err := s.readStore.GetSource(ctx, sourceID)
	if err == nil && source != nil {
		return source.Title
	}

	// Fallback: extract title from creation event
	if evt.EventType == "SourceCreated" {
		var created domain.SourceCreated
		if err := json.Unmarshal(evt.Data, &created); err == nil {
			if created.Title != "" {
				return created.Title
			}
		}
	}

	// Last resort: use ID
	return sourceID.String()
}

// getCitationName retrieves or constructs a citation's name.
// TODO: evt parameter reserved for extracting name from event data when read model unavailable
func (s *HistoryService) getCitationName(ctx context.Context, citationID uuid.UUID, _ *repository.StoredEvent) string {
	// Try to get from read model first
	citation, err := s.readStore.GetCitation(ctx, citationID)
	if err == nil && citation != nil {
		return fmt.Sprintf("%s (%s)", citation.SourceTitle, citation.FactType)
	}

	// Fallback: use ID
	return citationID.String()
}

// getImportName constructs a name for an import event.
func (s *HistoryService) getImportName(evt *repository.StoredEvent) string {
	if evt.EventType == "GedcomImported" {
		var imported domain.GedcomImported
		if err := json.Unmarshal(evt.Data, &imported); err == nil {
			return fmt.Sprintf("GEDCOM Import: %s", imported.Filename)
		}
	}
	return "Import"
}
