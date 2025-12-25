<objective>
Implement history query methods in the PostgreSQL EventStore for issue #35.
This enables querying event history from PostgreSQL storage.
</objective>

<context>
Issue: #35 - Change history and audit trail
Repository: github.com/cacack/my-family

The EventStore interface has been extended with history query methods.
Now we need to implement them for PostgreSQL.

@internal/repository/eventstore.go - EventStore interface with new history methods
@internal/repository/postgres/eventstore.go - PostgreSQL EventStore implementation
</context>

<requirements>
Implement these methods in the PostgreSQL EventStore:

1. `ReadByStream(ctx, streamID, limit, offset) (*HistoryPage, error)`
   - Query events WHERE stream_id = $1 ORDER BY version DESC
   - Include total count for pagination
   - Use existing indexes

2. `ReadGlobalByTime(ctx, fromTime, toTime, eventTypes, limit, offset) (*HistoryPage, error)`
   - Query events with optional time range filter
   - Optional event_type filter (for filtering by entity type)
   - ORDER BY timestamp DESC (most recent first)
   - Use ANY($N) for event types array parameter

PostgreSQL considerations:
- Use COUNT(*) OVER() for total count in single query
- Use $N placeholder syntax (not ?)
- Handle nil/empty eventTypes slice with conditional WHERE clause
- Handle nil time boundaries
- Return empty HistoryPage (not nil) when no results
</requirements>

<implementation>
1. Add ReadByStream method to PostgreSQL EventStore
2. Add ReadGlobalByTime method to PostgreSQL EventStore
3. Create helper function for building parameterized queries
4. Add tests for both methods
</implementation>

<output>
Files to modify:
- `internal/repository/postgres/eventstore.go` - Add method implementations

Files to create/update:
- Update `internal/repository/postgres/eventstore_test.go` with history tests
</output>

<verification>
- [ ] Both methods implemented and compile
- [ ] Tests cover: empty results, single result, pagination, time filtering
- [ ] Uses PostgreSQL-specific syntax ($1, $2, etc.)
- [ ] Run `go test ./internal/repository/postgres/...`
- [ ] Run `make check-coverage` to verify 85% threshold
</verification>

<success_criteria>
- ReadByStream returns paginated event history for a stream
- ReadGlobalByTime returns filtered global history
- All tests pass
- Coverage threshold met
</success_criteria>
