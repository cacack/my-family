package api

import (
	"github.com/cacack/my-family/internal/query"
)

// Note: ChangeHistoryResponse, ChangeEntry, and FieldChange types are now defined in generated.go
// from the OpenAPI spec.

// convertQueryChangeEntryToGenerated converts a query.ChangeEntry to generated ChangeEntry format.
func convertQueryChangeEntryToGenerated(entry query.ChangeEntry) ChangeEntry {
	entityName := entry.EntityName
	resp := ChangeEntry{
		Id:         entry.ID,
		Timestamp:  entry.Timestamp,
		EntityType: ChangeEntryEntityType(entry.EntityType),
		EntityId:   entry.EntityID,
		EntityName: &entityName,
		Action:     ChangeEntryAction(entry.Action),
		UserId:     entry.UserID,
	}
	if entry.ParentEntityType != "" && entry.ParentEntityID != nil {
		parentType := ChangeEntryParentEntityType(entry.ParentEntityType)
		parentID := *entry.ParentEntityID
		resp.ParentEntityType = &parentType
		resp.ParentEntityId = &parentID
	}
	if entry.Origin != "" {
		origin := ChangeEntryOrigin(entry.Origin)
		resp.Origin = &origin
	}

	if len(entry.Changes) > 0 {
		changes := make(map[string]FieldChange)
		for field, change := range entry.Changes {
			changes[field] = FieldChange{
				OldValue: change.OldValue,
				NewValue: change.NewValue,
			}
		}
		resp.Changes = &changes
	}

	return resp
}

// mapEntityTypeToEventTypes maps an entity type to the event types whose
// change entries report it, from the history's authoritative event-type table
// (query.HistoryEventTypesForEntity), so the entity_type filter and the
// entries it returns can never disagree.
func mapEntityTypeToEventTypes(entityType string) []string {
	return query.HistoryEventTypesForEntity(entityType)
}
