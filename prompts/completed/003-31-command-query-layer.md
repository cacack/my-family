<objective>
Implement the command and query layer for sources and citations, providing the business logic for CRUD operations.

This is Phase 3 of implementing GitHub issue #31. The command layer handles write operations (create, update, delete) while the query layer handles read operations (list, get, search).
</objective>

<context>
Project: Self-hosted genealogy software with CQRS architecture.
Issue: #31 - Sources and citations foundation (GPS)

Depends on: Phase 1 (domain) and Phase 2 (repository) must be complete.

Review existing command/query patterns:
@internal/command/handler.go - Handler struct with execute() helper
@internal/command/person_commands.go - CreatePerson, UpdatePerson, DeletePerson patterns
@internal/command/family_commands.go - Family command patterns
@internal/query/person_queries.go - PersonService with List, Get, Search
@internal/query/family_queries.go - FamilyService patterns
</context>

<requirements>
1. Create `internal/command/source_commands.go`:

   Input/Result types:
   - CreateSourceInput: SourceType, Title (required), Author, Publisher, PublishDate, URL, RepositoryName, CollectionName, CallNumber, Notes
   - CreateSourceResult: ID, Version
   - UpdateSourceInput: ID (required), Version (required for optimistic locking), plus all optional fields that can be updated
   - UpdateSourceResult: Version

   - CreateCitationInput: SourceID (required), FactType (required), FactOwnerID (required), Page, Volume, SourceQuality, InformantType, EvidenceType, QuotedText, Analysis, TemplateID
   - CreateCitationResult: ID, Version
   - UpdateCitationInput: ID (required), Version (required), plus all optional fields
   - UpdateCitationResult: Version

   Commands on Handler:
   - CreateSource(ctx, input) (*CreateSourceResult, error)
   - UpdateSource(ctx, input) (*UpdateSourceResult, error)
   - DeleteSource(ctx, id, version) error
   - CreateCitation(ctx, input) (*CreateCitationResult, error)
   - UpdateCitation(ctx, input) (*UpdateCitationResult, error)
   - DeleteCitation(ctx, id, version) error

2. Create `internal/query/source_queries.go`:

   Service struct:
   - SourceService with readStore repository.ReadModelStore

   Query types:
   - Source (response type with JSON tags)
   - Citation (response type with JSON tags)
   - SourceDetail (includes citations attached to this source)
   - ListSourcesInput: Limit, Offset, SortBy, SortOrder, Query (optional search term)
   - SourceListResult: Sources []Source, Total int

   Methods:
   - ListSources(ctx, input) (*SourceListResult, error)
   - GetSource(ctx, id) (*SourceDetail, error)
   - SearchSources(ctx, query, limit) ([]Source, error)
   - GetCitationsForPerson(ctx, personID) ([]Citation, error)
   - GetCitationsForFact(ctx, factType, factOwnerID) ([]Citation, error)

3. Add methods to Handler in `internal/command/handler.go` if needed (or keep in source_commands.go using the handler's execute() helper).

4. Create test files with table-driven tests following existing patterns.
</requirements>

<implementation>
Follow existing patterns:
- Commands validate input, create domain entity, validate entity, create event, call h.execute()
- h.execute() appends events and projects to read model
- Use ErrInvalidInput for validation errors (from command package)
- UpdateSource should load current version, apply changes, validate, emit SourceUpdated event
- DeleteSource should verify version matches before emitting SourceDeleted
- DeleteSource should fail if citations exist for this source (referential integrity)
- Query service converts ReadModels to response types (don't expose internal types)
- Use default limits (e.g., 20) when not specified
</implementation>

<output>
Create files:
- `./internal/command/source_commands.go` - NEW: Source and citation commands
- `./internal/command/source_commands_test.go` - NEW: Command tests
- `./internal/query/source_queries.go` - NEW: Source query service
- `./internal/query/source_queries_test.go` - NEW: Query tests
</output>

<verification>
Before completing:
1. Run `go build ./...` - must compile without errors
2. Run `go test ./internal/command/... ./internal/query/...` - all tests must pass
3. Verify CreateSource fails without title
4. Verify UpdateSource fails with wrong version (optimistic locking)
5. Verify DeleteSource fails if source has citations
6. Verify CreateCitation fails without SourceID or FactOwnerID
7. Verify GetCitationsForPerson returns correct citations
</verification>

<success_criteria>
- All CRUD operations work with proper validation
- Optimistic locking prevents concurrent modification conflicts
- Referential integrity maintained (can't delete source with citations)
- Query service returns properly formatted response types
- Test coverage for success cases and error cases
</success_criteria>
