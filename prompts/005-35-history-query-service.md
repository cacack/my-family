<objective>
Create the history query service that transforms raw events into user-friendly change entries for issue #35.
This service is the bridge between the event store and the API handlers.
</objective>

<context>
Issue: #35 - Change history and audit trail
Repository: github.com/cacack/my-family

The EventStore now has history query methods. We need a query service layer that:
1. Calls the event store methods
2. Transforms raw StoredEvents into user-friendly ChangeEntry structs
3. Enriches entries with entity names from the read model

@internal/repository/eventstore.go - EventStore interface with history methods, StoredEvent
@internal/repository/readmodel.go - ReadModelStore for entity name lookups
@internal/query/person_queries.go - Example query service pattern
@internal/domain/events.go - Event types and their structure
</context>

<requirements>
Create `internal/query/history_queries.go` with:

1. **HistoryService struct**
   - Dependencies: EventStore, ReadModelStore
   - Constructor: NewHistoryService(eventStore, readStore)

2. **ChangeEntry struct**
   ```go
   type ChangeEntry struct {
       ID          uuid.UUID              `json:"id"`
       Timestamp   time.Time              `json:"timestamp"`
       EntityType  string                 `json:"entity_type"`  // "person", "family", "source", "citation"
       EntityID    uuid.UUID              `json:"entity_id"`
       EntityName  string                 `json:"entity_name"`  // e.g., "John Smith"
       Action      string                 `json:"action"`       // "created", "updated", "deleted"
       Changes     map[string]FieldChange `json:"changes,omitempty"`
       UserID      *string                `json:"user_id,omitempty"`
   }

   type FieldChange struct {
       OldValue any `json:"old_value,omitempty"`
       NewValue any `json:"new_value,omitempty"`
   }
   ```

3. **Methods**
   - `GetEntityHistory(ctx, entityType, entityID, limit, offset) (*ChangeHistoryResult, error)`
   - `GetGlobalHistory(ctx, input GetGlobalHistoryInput) (*ChangeHistoryResult, error)`

4. **Event transformation logic**
   - Map event types to actions: PersonCreated→created, PersonUpdated→updated, PersonDeleted→deleted
   - Extract entity type from stream_type or event_type
   - For update events, extract changes from the event data's `Changes` field
   - Look up entity names from read model (gracefully handle deleted entities)
</requirements>

<implementation>
1. Create history_queries.go with HistoryService
2. Implement event-to-ChangeEntry transformation
3. Implement entity name enrichment (with fallback for deleted entities)
4. Add comprehensive tests

Event type mapping:
- PersonCreated/Updated/Deleted → entity_type: "person"
- FamilyCreated/Updated/Deleted → entity_type: "family"
- SourceCreated/Updated/Deleted → entity_type: "source"
- CitationCreated/Updated/Deleted → entity_type: "citation"
- ChildLinkedToFamily/ChildUnlinkedFromFamily → entity_type: "family"
- GedcomImported → entity_type: "import"
</implementation>

<output>
Files to create:
- `internal/query/history_queries.go` - HistoryService implementation
- `internal/query/history_queries_test.go` - Comprehensive tests
</output>

<verification>
- [ ] HistoryService created with all methods
- [ ] ChangeEntry struct matches OpenAPI schema
- [ ] Event transformation handles all event types
- [ ] Entity name lookup works (and gracefully handles missing entities)
- [ ] Run `go test ./internal/query/...`
- [ ] Run `make check-coverage` to verify 85% threshold
</verification>

<success_criteria>
- Query service transforms events to user-friendly change entries
- Entity names included for context
- Update events show field-level changes
- All tests pass with 85%+ coverage
</success_criteria>
