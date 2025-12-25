<objective>
Extend the EventStore interface with history query methods for issue #35 (Change history and audit trail).
These methods enable querying events by entity and globally with pagination.
</objective>

<context>
Issue: #35 - Change history and audit trail
Repository: github.com/cacack/my-family

The event sourcing architecture already captures all changes. We need to expose
query methods that allow retrieving event history for specific entities and globally.

@internal/repository/eventstore.go - Current EventStore interface and StoredEvent struct
@internal/domain/events.go - Domain events with BaseEvent, EventMetadata
</context>

<requirements>
Add these methods to the EventStore interface:

1. `ReadByStream(ctx, streamID, limit, offset)` - Get events for a specific stream (entity) with pagination
2. `ReadGlobalByTime(ctx, fromTime, toTime, eventTypes, limit, offset)` - Get global events filtered by time range and optional event types

Add a new struct for history query results:
```go
type HistoryPage struct {
    Events     []StoredEvent
    TotalCount int
    HasMore    bool
}
```

Design considerations:
- Use existing indexes (stream_id, event_type, timestamp, position)
- Support cursor-based pagination via position for global queries
- Keep method signatures simple and consistent with existing patterns
</requirements>

<implementation>
1. Add `HistoryPage` struct to eventstore.go
2. Add `ReadByStream` method to EventStore interface
3. Add `ReadGlobalByTime` method to EventStore interface
4. Ensure method signatures match what SQLite/Postgres can efficiently implement
</implementation>

<output>
Files to modify:
- `internal/repository/eventstore.go` - Add interface methods and HistoryPage struct
</output>

<verification>
- [ ] Interface compiles (implementations will fail until updated - that's expected)
- [ ] Method signatures are clear and well-documented
- [ ] HistoryPage struct supports pagination needs
- [ ] No breaking changes to existing interface methods
</verification>

<success_criteria>
- EventStore interface extended with history query methods
- HistoryPage struct defined for paginated results
- Method documentation explains parameters and return values
</success_criteria>
