<objective>
Implement the API layer for sources and citations, exposing REST endpoints for CRUD operations.

This is Phase 4 of implementing GitHub issue #31. The API layer provides HTTP endpoints that frontend applications and external tools can use to manage sources and citations.
</objective>

<context>
Project: Self-hosted genealogy software with Echo HTTP router.
Issue: #31 - Sources and citations foundation (GPS)

Depends on: Phase 1-3 (domain, repository, command/query) must be complete.

Review existing API patterns:
@internal/api/handlers.go - Handler methods, request/response types
@internal/api/server.go - Route registration with Echo
@internal/api/middleware.go - Error handling, validation
@internal/api/openapi.go - OpenAPI spec generation (if present)
</context>

<requirements>
1. Add request/response types to `internal/api/handlers.go` (or new source_handlers.go):

   Source endpoints:
   - CreateSourceRequest: source_type, title (required), author, publisher, publish_date, url, repository_name, collection_name, call_number, notes
   - UpdateSourceRequest: source_type, title, author, publisher, publish_date, url, repository_name, collection_name, call_number, notes (all optional)
   - SourceResponse: id, source_type, title, author, publisher, publish_date, url, repository_name, collection_name, call_number, notes, citation_count, version
   - SourceListResponse: sources []SourceResponse, total int

   Citation endpoints:
   - CreateCitationRequest: source_id (required), fact_type (required), fact_owner_id (required), page, volume, source_quality, informant_type, evidence_type, quoted_text, analysis, template_id
   - UpdateCitationRequest: page, volume, source_quality, informant_type, evidence_type, quoted_text, analysis, template_id (all optional)
   - CitationResponse: id, source_id, source_title, fact_type, fact_owner_id, page, volume, source_quality, informant_type, evidence_type, quoted_text, analysis, template_id, version

2. Add handler methods to Server:

   Source handlers:
   - listSources(c echo.Context) error - GET /api/v1/sources
   - createSource(c echo.Context) error - POST /api/v1/sources
   - getSource(c echo.Context) error - GET /api/v1/sources/:id
   - updateSource(c echo.Context) error - PUT /api/v1/sources/:id
   - deleteSource(c echo.Context) error - DELETE /api/v1/sources/:id
   - searchSources(c echo.Context) error - GET /api/v1/sources/search?q=

   Citation handlers:
   - getCitationsForSource(c echo.Context) error - GET /api/v1/sources/:id/citations
   - getCitationsForPerson(c echo.Context) error - GET /api/v1/persons/:id/citations
   - createCitation(c echo.Context) error - POST /api/v1/citations
   - getCitation(c echo.Context) error - GET /api/v1/citations/:id
   - updateCitation(c echo.Context) error - PUT /api/v1/citations/:id
   - deleteCitation(c echo.Context) error - DELETE /api/v1/citations/:id

3. Register routes in `internal/api/server.go`:
   - Group under /api/v1
   - Follow existing route registration patterns

4. Add/update tests in `internal/api/handlers_test.go` or new source_handlers_test.go
</requirements>

<implementation>
Follow existing patterns:
- Use c.Bind() for request parsing
- Use c.Validate() if validation middleware is configured
- Return appropriate HTTP status codes: 201 Created, 200 OK, 204 No Content, 400 Bad Request, 404 Not Found, 409 Conflict
- Parse UUID path parameters: uuid.Parse(c.Param("id"))
- Query params: c.QueryParam("q"), c.QueryParam("limit")
- For delete operations, read version from query param or request body
- Return JSON using c.JSON(status, response)
- Handle ErrNotFound -> 404, ErrInvalidInput -> 400, ErrVersionConflict -> 409

Standard pagination query params:
- limit (default 20, max 100)
- offset (default 0)
- sort (field name)
- order (asc/desc)
</implementation>

<output>
Modify/create files:
- `./internal/api/handlers.go` - Add source/citation handlers (or create source_handlers.go)
- `./internal/api/server.go` - Register new routes
- `./internal/api/handlers_test.go` - Add API tests (or create source_handlers_test.go)
</output>

<verification>
Before completing:
1. Run `go build ./...` - must compile without errors
2. Run `go test ./internal/api/...` - all tests must pass
3. Verify POST /api/v1/sources creates a source and returns 201
4. Verify GET /api/v1/sources/:id returns 404 for non-existent source
5. Verify DELETE /api/v1/sources/:id with wrong version returns 409
6. Verify GET /api/v1/persons/:id/citations returns citations for that person
</verification>

<success_criteria>
- All endpoints follow REST conventions
- Proper HTTP status codes for all responses
- Request validation returns helpful error messages
- Pagination works correctly for list endpoints
- Tests cover success and error scenarios
- Existing API tests continue to pass
</success_criteria>
