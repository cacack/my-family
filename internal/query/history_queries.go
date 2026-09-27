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

// maxComparisonEvents caps how many raw events any one side of a diff reads —
// snapshot-vs-snapshot (CompareSnapshots) and branch-vs-main (CompareBranch)
// alike. It lives here, next to the transform both diffs feed, so the two
// cannot be tuned independently and silently disagree.
const maxComparisonEvents = 1000

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
	EntityType string                 `json:"entity_type"` // "person", "family", "source", "citation"
	EntityID   uuid.UUID              `json:"entity_id"`
	EntityName string                 `json:"entity_name"` // e.g., "John Smith"
	Action     string                 `json:"action"`      // "created", "updated", "deleted"
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

	// Read events for this entity's stream. Scoped to main: the history endpoints
	// take no ?branch= parameter, so an entity's audit trail is the mainline's and
	// must not interleave any branch's in-progress edits (ADR-005). Branch-scoped
	// history is future work — it needs an API parameter first.
	page, err := s.eventStore.ReadByStream(ctx, entityID, domain.MainBranchID, limit, offset)
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
//
// Entity names are resolved in two passes (#697): the first registers every
// entity the entries will name, one batched read-model lookup per entity type
// resolves them all, and the second builds the entries from that result — so
// the read-model query count does not grow with the number of events.
func (s *HistoryService) transformStoredEvents(ctx context.Context, events []repository.StoredEvent) ([]ChangeEntry, error) {
	return s.transformStoredEventsOn(ctx, domain.MainBranchID, events)
}

// transformStoredEventsOn is transformStoredEvents with entity names resolved
// through branchID's overlay (resolveEntityNamesOn): CompareBranch names the
// branch's own changes as the branch sees them, so an entity the branch
// created or renamed is labelled with its branch name rather than its id or
// main's stale name.
func (s *HistoryService) transformStoredEventsOn(ctx context.Context, branchID domain.BranchID, events []repository.StoredEvent) ([]ChangeEntry, error) {
	refs := newEntityRefs()
	for i := range events {
		entityType, _ := s.mapEventTypeToEntityAndAction(events[i].EventType)
		refs.addEvent(entityType, events[i].StreamID, &events[i])
	}
	names, err := s.resolveEntityNamesOn(ctx, branchID, refs)
	if err != nil {
		return nil, err
	}

	entries := make([]ChangeEntry, 0, len(events))

	for i := range events {
		evt := &events[i]

		// Map event type to entity type and action
		entityType, action := s.mapEventTypeToEntityAndAction(evt.EventType)

		// Skip events that don't map to valid OpenAPI entity types (e.g., GedcomImported)
		if entityType == "skip" {
			continue
		}

		entry := ChangeEntry{
			ID:        evt.ID,
			Timestamp: evt.Timestamp,
			EntityID:  evt.StreamID,
		}

		entry.EntityType = entityType
		entry.Action = action

		// Extract changes for update events
		if action == "updated" {
			changes, err := s.extractChanges(*evt, names)
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

		// Enrich with the entity name resolved above
		entry.EntityName = names.name(entityType, evt.StreamID, evt)

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
		return "family", "updated"
	case "ChildUnlinkedFromFamily":
		return "family", "updated"
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
		return "skip", ""
	case "SnapshotCreated", "SnapshotDeleted":
		// Snapshot markers are event-sourced for the audit trail (issue #624) but
		// are not genealogical changes: showing "a snapshot was taken" inside the
		// diff BETWEEN two snapshots is noise. The audit record remains in the
		// event log; only this change-log view skips it.
		//
		// KNOWN LIMITATION: skipping happens after the store paginates, so a
		// skipped event still counts toward TotalCount and still consumes a slot
		// in the page — global history under-fills and over-reports. That is
		// pre-existing (GedcomImported does the same) but snapshot create/delete
		// is a routine action where an import is not, so it is now easy to hit.
		// The real fix is shared with the ~30 event types that have no case here
		// and render as "unknown" in violation of the ChangeEntry enum; both want
		// one authoritative event-type table used to filter AT the store. Tracked
		// in issue #739 — do not fix piecemeal.
		return "skip", ""
	default:
		return "unknown", "unknown"
	}
}

// extractChanges extracts field-level changes from update events. names must
// hold the entities evt references (entityRefs.addEvent registers them).
func (s *HistoryService) extractChanges(evt repository.StoredEvent, names *entityNames) (map[string]FieldChange, error) {
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
	case domain.ChildLinkedToFamily:
		childName := names.personName(e.PersonID, nil)
		return map[string]FieldChange{
			"children": {NewValue: fmt.Sprintf("Child linked: %s", childName)},
		}, nil
	case domain.ChildUnlinkedFromFamily:
		childName := names.personName(e.PersonID, nil)
		return map[string]FieldChange{
			"children": {NewValue: fmt.Sprintf("Child unlinked: %s", childName)},
		}, nil
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
