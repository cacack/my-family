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

// mapEntityTypeToEventTypes maps an entity type to its corresponding event types.
func mapEntityTypeToEventTypes(entityType string) []string {
	switch entityType {
	case "person":
		return []string{"PersonCreated", "PersonUpdated", "PersonDeleted"}
	case "family":
		return []string{"FamilyCreated", "FamilyUpdated", "FamilyDeleted", "ChildLinkedToFamily", "ChildUnlinkedFromFamily"}
	case "source":
		return []string{"SourceCreated", "SourceUpdated", "SourceDeleted"}
	case "citation":
		return []string{"CitationCreated", "CitationUpdated", "CitationDeleted"}
	default:
		return nil
	}
}
