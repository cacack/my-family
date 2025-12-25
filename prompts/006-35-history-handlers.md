<objective>
Create API handlers for history endpoints and wire them into the server for issue #35.
This completes the audit trail feature by exposing it via the REST API.
</objective>

<context>
Issue: #35 - Change history and audit trail
Repository: github.com/cacack/my-family

The history query service is complete. Now we need HTTP handlers that:
1. Parse request parameters
2. Call the history service
3. Return JSON responses matching the OpenAPI spec

@internal/api/openapi.yaml - API spec with history endpoints
@internal/api/server.go - Server struct and route registration
@internal/api/handlers.go - Example handler patterns
@internal/query/history_queries.go - HistoryService to call
</context>

<requirements>
1. **Create `internal/api/history_handlers.go`** with handlers:

   ```go
   // GET /history - Global changelog
   func (s *Server) getGlobalHistory(c echo.Context) error

   // GET /persons/{id}/history - Person history
   func (s *Server) getPersonHistory(c echo.Context) error

   // GET /families/{id}/history - Family history
   func (s *Server) getFamilyHistory(c echo.Context) error

   // GET /sources/{id}/history - Source history
   func (s *Server) getSourceHistory(c echo.Context) error
   ```

2. **Response types** (in history_handlers.go):
   ```go
   type ChangeEntryResponse struct {
       ID         string                    `json:"id"`
       Timestamp  string                    `json:"timestamp"`
       EntityType string                    `json:"entity_type"`
       EntityID   string                    `json:"entity_id"`
       EntityName string                    `json:"entity_name"`
       Action     string                    `json:"action"`
       Changes    map[string]FieldChange    `json:"changes,omitempty"`
       UserID     *string                   `json:"user_id,omitempty"`
   }

   type ChangeHistoryResponse struct {
       Items   []ChangeEntryResponse `json:"items"`
       Total   int                   `json:"total"`
       Limit   int                   `json:"limit"`
       Offset  int                   `json:"offset"`
       HasMore bool                  `json:"has_more"`
   }
   ```

3. **Update `internal/api/server.go`**:
   - Add `historyService *query.HistoryService` to Server struct
   - Initialize in NewServer
   - Register routes in registerRoutes()

4. **Query parameter parsing** for /history:
   - `entity_type`: optional filter (person, family, source, citation)
   - `from`: optional ISO datetime
   - `to`: optional ISO datetime
   - `limit`: default 20, max 100
   - `offset`: default 0
</requirements>

<implementation>
1. Create history_handlers.go with response types and handlers
2. Update server.go to add historyService field
3. Update NewServer to create HistoryService
4. Add route registrations in registerRoutes()
5. Create history_handlers_test.go with tests
</implementation>

<output>
Files to create:
- `internal/api/history_handlers.go` - Handler implementations
- `internal/api/history_handlers_test.go` - Handler tests

Files to modify:
- `internal/api/server.go` - Add service and routes
</output>

<verification>
- [ ] All 4 handler functions implemented
- [ ] Routes registered correctly
- [ ] Response format matches OpenAPI spec
- [ ] Query params parsed correctly
- [ ] Error handling for invalid IDs, params
- [ ] Run `go test ./internal/api/...`
- [ ] Run `make check-coverage` to verify 85% threshold
- [ ] Manual test: `curl http://localhost:8080/api/v1/history`
</verification>

<success_criteria>
- All history endpoints return correct JSON responses
- Pagination works correctly
- Filters work (entity_type, time range)
- Integration tests pass
- 85%+ test coverage maintained
</success_criteria>
