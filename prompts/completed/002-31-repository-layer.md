<objective>
Implement the repository layer for sources and citations, including read models, projections, and all storage backends.

This is Phase 2 of implementing GitHub issue #31. The repository layer provides storage and retrieval for source/citation data across all supported backends (memory, SQLite, PostgreSQL).
</objective>

<context>
Project: Self-hosted genealogy software with event sourcing + CQRS architecture.
Issue: #31 - Sources and citations foundation (GPS)

Depends on: Phase 1 (domain layer) must be complete.

Review existing repository patterns:
@internal/repository/readmodel.go - ReadModelStore interface, PersonReadModel, FamilyReadModel
@internal/repository/eventstore.go - StoredEvent.DecodeEvent() switch statement
@internal/repository/projection.go - Projector with projectXxx methods
@internal/repository/memory/readmodel.go - In-memory implementation
@internal/repository/sqlite/readmodel.go - SQLite implementation
@internal/repository/postgres/readmodel.go - PostgreSQL implementation
</context>

<requirements>
1. Add read model types to `internal/repository/readmodel.go`:
   - SourceReadModel: ID, SourceType, Title, Author, Publisher, PublishDateRaw, PublishDateSort (*time.Time), URL, RepositoryName, CollectionName, CallNumber, Notes, GedcomXref, CitationCount (int), Version, UpdatedAt
   - CitationReadModel: ID, SourceID, SourceTitle (denormalized), FactType, FactOwnerID, Page, Volume, SourceQuality, InformantType, EvidenceType, QuotedText, Analysis, TemplateID, GedcomXref, Version, CreatedAt

2. Extend ReadModelStore interface in `internal/repository/readmodel.go`:
   - GetSource(ctx, id) (*SourceReadModel, error)
   - ListSources(ctx, opts ListOptions) ([]SourceReadModel, int, error)
   - SearchSources(ctx, query string, limit int) ([]SourceReadModel, error)
   - SaveSource(ctx, *SourceReadModel) error
   - DeleteSource(ctx, id) error
   - GetCitation(ctx, id) (*CitationReadModel, error)
   - GetCitationsForSource(ctx, sourceID) ([]CitationReadModel, error)
   - GetCitationsForPerson(ctx, personID) ([]CitationReadModel, error)
   - GetCitationsForFact(ctx, factType, factOwnerID) ([]CitationReadModel, error)
   - SaveCitation(ctx, *CitationReadModel) error
   - DeleteCitation(ctx, id) error

3. Add event decoding to `internal/repository/eventstore.go`:
   - Add cases for SourceCreated, SourceUpdated, SourceDeleted
   - Add cases for CitationCreated, CitationUpdated, CitationDeleted

4. Add projector methods to `internal/repository/projection.go`:
   - projectSourceCreated, projectSourceUpdated, projectSourceDeleted
   - projectCitationCreated, projectCitationUpdated, projectCitationDeleted
   - When projecting citations, denormalize SourceTitle from the source
   - When projecting citation create/delete, update CitationCount on the source

5. Implement memory backend in `internal/repository/memory/readmodel.go`:
   - Add sources and citations maps
   - Implement all new interface methods

6. Implement SQLite backend in `internal/repository/sqlite/readmodel.go`:
   - Add CREATE TABLE for sources and citations (in ensureSchema or similar)
   - Implement all new interface methods with SQL queries
   - Use appropriate indexes: citations(source_id), citations(fact_type, fact_owner_id)

7. Implement PostgreSQL backend in `internal/repository/postgres/readmodel.go`:
   - Same as SQLite but with PostgreSQL-specific syntax if needed
   - Use appropriate indexes
</requirements>

<implementation>
Follow existing patterns:
- Use sync.RWMutex for memory store thread safety
- Use sql.NullString for nullable VARCHAR columns in SQL
- PublishDateSort should be derived from PublishDateRaw using existing date parsing logic
- CitationCount on SourceReadModel is denormalized - update it when citations are added/removed
- For GetCitationsForPerson, query where FactType starts with "person_" AND FactOwnerID matches
- ListOptions already supports pagination (Limit, Offset) and sorting (SortBy, SortOrder)
</implementation>

<output>
Modify files:
- `./internal/repository/readmodel.go` - Add types and interface methods
- `./internal/repository/eventstore.go` - Add event decoding
- `./internal/repository/projection.go` - Add projector methods
- `./internal/repository/memory/readmodel.go` - Memory implementation
- `./internal/repository/sqlite/readmodel.go` - SQLite implementation
- `./internal/repository/postgres/readmodel.go` - PostgreSQL implementation

Add test files:
- `./internal/repository/memory/readmodel_test.go` - Update with source/citation tests
- `./internal/repository/sqlite/readmodel_test.go` - Update with source/citation tests
- `./internal/repository/postgres/readmodel_test.go` - Update with source/citation tests
</output>

<verification>
Before completing:
1. Run `go build ./...` - must compile without errors
2. Run `go test ./internal/repository/...` - all tests must pass
3. Verify sources can be saved, retrieved, listed, and deleted in all backends
4. Verify citations are linked to sources correctly
5. Verify GetCitationsForPerson returns only person-related citations
6. Verify CitationCount updates correctly when citations are added/removed
</verification>

<success_criteria>
- All three backends (memory, sqlite, postgres) implement the full interface
- Projector correctly transforms events into read models
- Denormalized fields (SourceTitle, CitationCount) stay in sync
- All existing tests continue to pass
- New tests cover source/citation CRUD operations
</success_criteria>
