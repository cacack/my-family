<objective>
Add history/changelog API endpoints to the OpenAPI specification for issue #35.
These endpoints expose the audit trail functionality to API consumers.
</objective>

<context>
Issue: #35 - Change history and audit trail
Repository: github.com/cacack/my-family

The API needs endpoints to view change history both globally and per-entity.
This enables users to see what changed, when, and (eventually) by whom.

@internal/api/openapi.yaml - Current OpenAPI spec with persons, families, sources, citations
</context>

<requirements>
Add these endpoints:

1. `GET /history` - Global changelog
   - Query params: `entity_type` (optional), `from` (ISO datetime), `to` (ISO datetime), `limit`, `offset`
   - Returns paginated list of change entries

2. `GET /persons/{id}/history` - Person change history
   - Query params: `limit`, `offset`
   - Returns paginated list of changes to this person

3. `GET /families/{id}/history` - Family change history
   - Same pattern as persons

4. `GET /sources/{id}/history` - Source change history
   - Same pattern as persons

Add these schemas:

```yaml
ChangeEntry:
  type: object
  required: [id, timestamp, entity_type, entity_id, action]
  properties:
    id:
      type: string
      format: uuid
    timestamp:
      type: string
      format: date-time
    entity_type:
      type: string
      enum: [person, family, source, citation]
    entity_id:
      type: string
      format: uuid
    entity_name:
      type: string
      description: Human-readable name of the entity
    action:
      type: string
      enum: [created, updated, deleted]
    changes:
      type: object
      description: Field-level changes for updates
      additionalProperties:
        $ref: '#/components/schemas/FieldChange'
    user_id:
      type: string
      description: ID of user who made the change (null if single-user)

FieldChange:
  type: object
  properties:
    old_value:
      description: Previous value (null for new fields)
    new_value:
      description: New value (null for removed fields)

ChangeHistoryResponse:
  type: object
  required: [items, total]
  properties:
    items:
      type: array
      items:
        $ref: '#/components/schemas/ChangeEntry'
    total:
      type: integer
    limit:
      type: integer
    offset:
      type: integer
    has_more:
      type: boolean
```
</requirements>

<implementation>
1. Add new `history` tag to tags section
2. Add `/history` path with GET operation
3. Add `/persons/{id}/history` path
4. Add `/families/{id}/history` path
5. Add `/sources/{id}/history` path
6. Add ChangeEntry, FieldChange, and ChangeHistoryResponse schemas
7. Add common query parameters for history endpoints
</implementation>

<output>
Files to modify:
- `internal/api/openapi.yaml` - Add history endpoints and schemas
</output>

<verification>
- [ ] OpenAPI spec is valid YAML
- [ ] Endpoints follow existing patterns in the spec
- [ ] Response schemas match what the backend will provide
- [ ] Query parameters documented with descriptions
</verification>

<success_criteria>
- 4 new history endpoints defined (global + 3 entity types)
- ChangeEntry schema captures all needed fields
- Pagination supported on all history endpoints
- Spec validates without errors
</success_criteria>
