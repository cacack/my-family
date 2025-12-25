<objective>
Implement history query methods in the SQLite EventStore for issue #35.
This enables querying event history from SQLite storage.
</objective>

<context>
Issue: #35 - Change history and audit trail
Repository: github.com/cacack/my-family

The EventStore interface has been extended with history query methods.
Now we need to implement them for SQLite.

@internal/repository/eventstore.go - EventStore interface with new history methods
@internal/repository/sqlite/eventstore.go - SQLite EventStore implementation
</context>

<requirements>
Implement these methods in the SQLite EventStore:

1. `ReadByStream(ctx, streamID, limit, offset) (*HistoryPage, error)`
   - Query events WHERE stream_id = ? ORDER BY version DESC
   - Include total count for pagination
   - Use existing idx_events_stream_version index

2. `ReadGlobalByTime(ctx, fromTime, toTime, eventTypes, limit, offset) (*HistoryPage, error)`
   - Query events with optional time range filter
   - Optional event_type filter (for filtering by entity type)
   - ORDER BY timestamp DESC (most recent first)
   - Use existing idx_events_event_type index

SQL considerations:
- Use COUNT(*) OVER() for total count in single query (SQLite supports window functions)
- Handle nil/empty eventTypes slice (means "all types")
- Handle nil time boundaries (means "unbounded")
- Return empty HistoryPage (not nil) when no results
</requirements>

<implementation>
1. Add ReadByStream method to SQLite EventStore
2. Add ReadGlobalByTime method to SQLite EventStore
3. Create helper function for building dynamic WHERE clause
4. Add tests for both methods
</implementation>

<output>
Files to modify:
- `internal/repository/sqlite/eventstore.go` - Add method implementations

Files to create:
- Update `internal/repository/sqlite/eventstore_test.go` with history tests
</output>

<verification>
- [ ] Both methods implemented and compile
- [ ] Tests cover: empty results, single result, pagination, time filtering
- [ ] Efficient use of existing indexes
- [ ] Run `go test ./internal/repository/sqlite/...`
- [ ] Run `make check-coverage` to verify 85% threshold
</verification>

<success_criteria>
- ReadByStream returns paginated event history for a stream
- ReadGlobalByTime returns filtered global history
- All tests pass
- Coverage threshold met
</success_criteria>
