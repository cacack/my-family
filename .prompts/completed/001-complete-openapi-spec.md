<objective>
Complete the OpenAPI specification for sources, citations, and export endpoints.

Issue #133 identified spec drift - endpoints exist in the codebase but are not documented in the OpenAPI spec. This blocks future contract testing (#17) and makes the API harder to consume.
</objective>

<context>
Read project conventions first:
@CLAUDE.md

Target file to modify:
@internal/api/openapi.yaml

Reference files for request/response schemas:
@internal/api/source_handlers.go - Contains Go structs to translate to OpenAPI schemas
@internal/api/server.go - Route definitions showing all endpoints
</context>

<requirements>
## Source Endpoints (add to paths section)

Add these 7 endpoints under a new `sources` tag:
- `GET /sources` - List sources with pagination (limit, offset, sort, order, q params)
- `POST /sources` - Create source
- `GET /sources/search` - Search sources (q param, limit)
- `GET /sources/:id` - Get source by ID
- `PUT /sources/:id` - Update source
- `DELETE /sources/:id` - Delete source (version query param)
- `GET /sources/:id/citations` - Get citations for a source

## Citation Endpoints (add to paths section)

Add these 5 endpoints under a new `citations` tag:
- `POST /citations` - Create citation
- `GET /citations/:id` - Get citation by ID
- `PUT /citations/:id` - Update citation
- `DELETE /citations/:id` - Delete citation (version query param)
- `GET /persons/:id/citations` - Get citations for a person

## Export Endpoints (add to paths section)

Add these 3 endpoints under a new `export` tag:
- `GET /export/tree` - Export full family tree
- `GET /export/persons` - Export persons data
- `GET /export/families` - Export families data

## Schemas (add to components/schemas section)

Create these schemas by translating the Go structs in source_handlers.go:
- `Source` - Full source object with all fields
- `SourceCreate` - Request body for POST /sources
- `SourceUpdate` - Request body for PUT /sources/:id (includes version)
- `SourceDetail` - Source with embedded citations array
- `SourceList` - Paginated list response
- `SourceSearchResults` - Search results with query field
- `Citation` - Full citation object
- `CitationCreate` - Request body for POST /citations
- `CitationUpdate` - Request body for PUT /citations/:id (includes version)
- `CitationList` - List of citations with total

## Bug Fix

Add missing `versionParam` to components/parameters:
```yaml
versionParam:
  name: version
  in: query
  description: Entity version for optimistic locking
  schema:
    type: integer
    format: int64
```

## Tags

Add these tags to the tags section:
- `sources` - Source record management
- `citations` - Citation management
- `export` - Data export
</requirements>

<implementation>
Follow existing patterns in openapi.yaml:
- Use `$ref` for reusable parameters (limitParam, offsetParam, versionParam)
- Use `$ref` for common responses (BadRequest, NotFound, Conflict)
- Include operationId for each endpoint (camelCase, descriptive)
- Include summary and tags for each endpoint
- Use proper HTTP status codes (200, 201, 204, 400, 404, 409)
- Mark required fields appropriately in schemas
- Use format: uuid for ID fields
- Use format: int64 for version fields
</implementation>

<output>
Modify: `./internal/api/openapi.yaml`
- Add sources, citations, export tags
- Add all endpoint paths
- Add all schemas
- Add versionParam
</output>

<verification>
Before completing, verify:
1. Run OpenAPI linter: `npx @redocly/cli lint internal/api/openapi.yaml`
2. Confirm all 15 endpoints are documented (7 source + 5 citation + 3 export)
3. Confirm versionParam is defined and referenced correctly
4. Confirm all schemas match the Go struct field names and types
</verification>

<success_criteria>
- OpenAPI linter passes with no errors
- All source endpoints documented with request/response schemas
- All citation endpoints documented with request/response schemas
- All export endpoints documented
- versionParam bug fixed
- Schemas accurately reflect Go structs in source_handlers.go
</success_criteria>
