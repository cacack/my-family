# Implementation Plan: My Family Genealogy MVP

**Branch**: `001-genealogy-mvp` | **Date**: 2025-12-07 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-genealogy-mvp/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Self-hosted genealogy software implementing an event-sourced architecture with CQRS-lite pattern. Core functionality includes GEDCOM import/export, person and family management, pedigree chart visualization, and full-text search. Single binary deployment with embedded Svelte frontend. Supports PostgreSQL (primary) and SQLite (local/demo mode).

## Technical Context

**Language/Version**: Go 1.22+
**Primary Dependencies**: Echo (HTTP router), Ent (data layer), oapi-codegen (OpenAPI), github.com/cacack/gedcom-go (GEDCOM processing), Svelte 5 + Vite + D3.js + Tailwind CSS (frontend)
**Storage**: PostgreSQL (primary, required for future pgvector/PostGIS), SQLite (local/demo fallback)
**Testing**: `go test` with testcontainers for PostgreSQL integration tests, Vitest + Svelte Testing Library for frontend
**Target Platform**: Linux/macOS/Windows server, Docker container, single binary deployment
**Project Type**: Web application (Go backend with embedded Svelte frontend)
**Performance Goals**: Single record CRUD <100ms, bulk import 1000 records <10s, search <500ms, pedigree navigation <1s
**Constraints**: <100MB memory for typical trees (<5000 individuals), offline-capable with SQLite
**Scale/Scope**: 10,000 individuals per tree, 7 user stories, single-user MVP (no auth)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-Research Gate (Phase 0)

| Principle | Requirement | Status | Notes |
|-----------|-------------|--------|-------|
| I. Code Quality | Idiomatic Go, `go vet`/`go fmt` clean, doc comments, explicit errors, minimal deps | ✅ PASS | Standard Go project with justified dependencies |
| II. Testing Standards | Unit tests for business logic, integration tests for persistence, deterministic, 80% coverage target | ✅ PASS | Plan includes comprehensive test strategy with testcontainers |
| III. UX Consistency | CLI pattern `my-family <noun> <verb>`, JSON/human output, actionable errors, destructive confirmation, GEDCOM fidelity | ✅ PASS | API-first with both JSON and human-readable output |
| IV. Performance | CRUD <100ms, bulk import <10s, search <500ms, <100MB memory, indexed queries | ✅ PASS | Performance goals align with constitution limits |

### Violations Requiring Justification

None identified. The event-sourced architecture may seem complex, but it directly enables the future git-style branching feature specified in the architecture and provides audit trail for genealogical data changes.

## Project Structure

### Documentation (this feature)

```text
specs/001-genealogy-mvp/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── openapi.yaml     # OpenAPI 3.0 specification
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Go Backend (single binary with embedded frontend)
cmd/
└── myfamily/
    └── main.go              # Application entry point

internal/
├── domain/                  # Pure Go domain layer (no external deps)
│   ├── person.go            # Person entity and value objects
│   ├── family.go            # Family entity
│   ├── date.go              # GenDate type for genealogical dates
│   ├── place.go             # Place value object
│   └── events.go            # Domain event definitions
├── command/                 # Command layer (CQRS write side)
│   ├── handler.go           # Command handler interface
│   ├── person_commands.go   # CreatePerson, UpdatePerson, DeletePerson
│   ├── family_commands.go   # CreateFamily, UpdateFamily, LinkChild
│   └── gedcom_commands.go   # ImportGedcom command
├── query/                   # Query layer (CQRS read side)
│   ├── person_queries.go    # Person list, detail, search
│   ├── family_queries.go    # Family queries
│   └── pedigree_queries.go  # Pedigree traversal
├── repository/              # Data access layer
│   ├── eventstore.go        # EventStore interface
│   ├── postgres/            # PostgreSQL implementations
│   │   ├── eventstore.go
│   │   └── readmodel.go
│   ├── sqlite/              # SQLite implementations
│   │   ├── eventstore.go
│   │   └── readmodel.go
│   └── projection.go        # Projection handlers
├── api/                     # HTTP API layer
│   ├── server.go            # Echo server setup
│   ├── handlers.go          # Generated from OpenAPI
│   ├── middleware.go        # Logging, error handling
│   └── openapi.go           # Embedded OpenAPI spec
├── gedcom/                  # GEDCOM processing
│   ├── importer.go          # GEDCOM to domain events
│   └── exporter.go          # Read models to GEDCOM
└── config/                  # Configuration loading
    └── config.go

# Frontend (embedded via go:embed)
web/
├── src/
│   ├── lib/
│   │   ├── components/      # Reusable Svelte components
│   │   │   ├── PersonCard.svelte
│   │   │   ├── FamilyCard.svelte
│   │   │   ├── PedigreeChart.svelte
│   │   │   └── SearchBox.svelte
│   │   └── api/             # API client services
│   │       └── client.ts
│   ├── routes/              # SvelteKit routes (pages)
│   │   ├── +page.svelte     # Home/dashboard
│   │   ├── persons/
│   │   │   ├── +page.svelte # Person list
│   │   │   └── [id]/
│   │   │       └── +page.svelte  # Person detail/edit
│   │   ├── families/
│   │   │   └── +page.svelte # Family management
│   │   ├── pedigree/
│   │   │   └── [id]/
│   │   │       └── +page.svelte  # Pedigree chart
│   │   └── import/
│   │       └── +page.svelte # GEDCOM import
│   └── app.html
├── static/
├── package.json
├── svelte.config.js
├── vite.config.ts
└── tailwind.config.js

# Tests
internal/
├── domain/
│   └── *_test.go            # Pure unit tests
├── command/
│   └── *_test.go            # Unit tests with in-memory event store
├── query/
│   └── *_test.go            # Integration tests with testcontainers
├── repository/
│   └── *_test.go            # Round-trip serialization tests
├── api/
│   └── *_test.go            # Contract tests (OpenAPI compliance)
└── gedcom/
    └── *_test.go            # Import/export round-trip tests

web/
└── src/
    └── lib/
        └── components/
            └── *.test.ts    # Vitest + Svelte Testing Library

# Configuration & Deployment
Dockerfile
docker-compose.yml
go.mod
go.sum
Makefile
```

**Structure Decision**: Web application pattern selected. Go backend with embedded Svelte frontend via `go:embed`. Single binary output contains both API server and static frontend assets. Follows idiomatic Go project layout with `cmd/` for entry points and `internal/` for private packages. Event sourcing architecture reflected in `command/`, `query/`, and `repository/` package separation.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No constitution violations requiring justification. Event sourcing complexity is a deliberate architectural choice documented in the technology stack input.

---

## Post-Design Constitution Check

*Re-evaluated after Phase 1 design completion.*

| Principle | Design Artifact | Status | Notes |
|-----------|-----------------|--------|-------|
| I. Code Quality | data-model.md, openapi.yaml | ✅ PASS | Clean domain model, type-safe API contract, standard Go patterns |
| II. Testing Standards | quickstart.md (test commands), data-model.md (event definitions) | ✅ PASS | Clear test strategy: domain unit tests, integration with testcontainers, round-trip GEDCOM tests |
| III. UX Consistency | openapi.yaml (error responses), data-model.md (GenDate) | ✅ PASS | Consistent error schema with code/message/details, GEDCOM date fidelity preserved |
| IV. Performance | data-model.md (indexes), research.md (query patterns) | ✅ PASS | Proper indexing strategy, denormalized read models, tsvector/FTS5 for search |

### Design Validation Summary

- **API Contract**: 15 endpoints covering all 7 user stories (persons CRUD, families CRUD, pedigree, search, GEDCOM import/export)
- **Data Model**: 3 core entities (Person, Family, FamilyChild), 10 domain events, 4 read model tables
- **Performance Design**: Indexes on all query patterns, denormalized read models, database-native full-text search
- **GEDCOM Fidelity**: GenDate type preserves qualifiers and original strings, round-trip preservation strategy documented

All constitution gates pass. Design is ready for task generation via `/speckit.tasks`.

---

## Implementation Guide

This section provides sequencing guidance for task generation. Implementation follows a strict dependency order where each phase builds on the previous.

### Dependency Graph

```
┌─────────────────────────────────────────────────────────────────────┐
│  Phase 1: Domain Layer (no dependencies)                            │
│  internal/domain/                                                   │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Phase 2: Repository Layer (depends on: domain)                     │
│  internal/repository/                                               │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                              ▼
┌────────────────────────────────┐  ┌────────────────────────────────┐
│  Phase 3a: Command Layer       │  │  Phase 3b: Query Layer         │
│  internal/command/             │  │  internal/query/               │
│  (depends on: domain, repo)    │  │  (depends on: domain, repo)    │
└────────────────────────────────┘  └────────────────────────────────┘
                    │                              │
                    └──────────────┬──────────────┘
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Phase 4: API Layer (depends on: command, query)                    │
│  internal/api/                                                      │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                              ▼
┌────────────────────────────────┐  ┌────────────────────────────────┐
│  Phase 5a: GEDCOM Processing   │  │  Phase 5b: Frontend            │
│  internal/gedcom/              │  │  web/                          │
│  (depends on: command, query)  │  │  (depends on: API running)     │
└────────────────────────────────┘  └────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Phase 6: Integration & Deployment                                  │
│  cmd/myfamily/, Dockerfile, docker-compose.yml                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Phase 1: Domain Layer

**Purpose**: Pure Go types with no external dependencies. Foundation for all other layers.

| File | Reference | Key Implementation Notes |
|------|-----------|-------------------------|
| `internal/domain/date.go` | [data-model.md#GenDate](./data-model.md#gendate-value-object), [research.md#5](./research.md#5-gedcom-processing) | GenDate struct with Raw, Qualifier, ParsedDate fields. Parse() and String() methods. See research.md for Go struct example. |
| `internal/domain/person.go` | [data-model.md#Person](./data-model.md#person) | Person entity with validation. Fields: id, given_name, surname, gender, birth_date, birth_place, death_date, death_place, notes, gedcom_xref. Validate() method enforces rules from data-model.md. |
| `internal/domain/family.go` | [data-model.md#Family](./data-model.md#family), [data-model.md#FamilyChild](./data-model.md#familychild) | Family entity + FamilyChild. Validation: at least one partner, no self-partnering, child not same as partner. |
| `internal/domain/place.go` | [data-model.md#Place](./data-model.md#place-value-object) | Simple value object wrapping place name string. |
| `internal/domain/events.go` | [data-model.md#Domain Events](./data-model.md#domain-events) | All event types: PersonCreated, PersonUpdated, PersonDeleted, FamilyCreated, FamilyUpdated, ChildLinkedToFamily, ChildUnlinkedFromFamily, FamilyDeleted, GedcomImported. Each event is a struct with JSON tags matching data-model.md schemas. |
| `internal/domain/enums.go` | [data-model.md#Enumerations](./data-model.md#enumerations) | Gender, RelationType, ChildRelationType, DateQualifier enums. |

**Verification**: `go test ./internal/domain/...` with table-driven tests for:
- GenDate parsing: all formats from [data-model.md examples](./data-model.md#gendate-value-object) (exact, ABT, BET, etc.)
- Person validation: empty name rejected, death_date >= birth_date
- Family validation: at least one partner, no circular refs
- Event JSON round-trip serialization

### Phase 2: Repository Layer

**Purpose**: Event store and read model persistence. Abstracts database operations.

| File | Reference | Key Implementation Notes |
|------|-----------|-------------------------|
| `internal/repository/eventstore.go` | [data-model.md#Event Store Schema](./data-model.md#event-store-schema), [research.md#3](./research.md#3-event-sourcing-with-cqrs-lite) | EventStore interface: Append(streamID, events, expectedVersion), ReadStream(streamID), ReadAll(fromPosition). Uses optimistic locking via version field. |
| `internal/repository/readmodel.go` | [data-model.md#Read Model Schema](./data-model.md#read-model-schema) | ReadModelStore interface for persons, families, family_children, pedigree_edges tables. |
| `internal/repository/projection.go` | [research.md#3](./research.md#3-event-sourcing-with-cqrs-lite) | Projection handlers that update read models from events. Synchronous projection (same transaction as event append). |
| `internal/repository/memory/eventstore.go` | - | In-memory EventStore for testing. No external deps. |
| `internal/repository/sqlite/eventstore.go` | [data-model.md#SQLite Adaptations](./data-model.md#sqlite-adaptations), [research.md#2](./research.md#2-ent-orm-for-event-sourcing) | SQLite EventStore using Ent. UUID as TEXT, JSONB as TEXT. |
| `internal/repository/sqlite/readmodel.go` | [data-model.md#SQLite Adaptations](./data-model.md#sqlite-adaptations) | SQLite read models with FTS5 for search. |
| `internal/repository/postgres/eventstore.go` | [data-model.md#Event Store Schema](./data-model.md#event-store-schema), [research.md#2](./research.md#2-ent-orm-for-event-sourcing) | PostgreSQL EventStore using Ent. BIGSERIAL for position, JSONB for data. |
| `internal/repository/postgres/readmodel.go` | [data-model.md#Read Model Schema](./data-model.md#read-model-schema), [research.md#6](./research.md#6-full-text-search) | PostgreSQL read models with tsvector for search, pg_trgm for fuzzy. |

**Verification**:
- EventStore: append events, read stream, verify version ordering
- Optimistic locking: concurrent append with same expectedVersion fails
- Projection: event → read model update verified
- Round-trip: event data survives serialize/deserialize

### Phase 3a: Command Layer

**Purpose**: CQRS write side. Handles commands, validates business rules, emits events.

| File | Reference | Key Implementation Notes |
|------|-----------|-------------------------|
| `internal/command/handler.go` | [research.md#3](./research.md#3-event-sourcing-with-cqrs-lite) | CommandHandler interface. Execute(ctx, command) returns events or error. |
| `internal/command/person_commands.go` | [data-model.md#Person Events](./data-model.md#person-events) | CreatePersonCommand, UpdatePersonCommand, DeletePersonCommand. Validate input, emit PersonCreated/Updated/Deleted events. |
| `internal/command/family_commands.go` | [data-model.md#Family Events](./data-model.md#family-events) | CreateFamilyCommand, UpdateFamilyCommand, DeleteFamilyCommand, LinkChildCommand, UnlinkChildCommand. Validate relationships, detect circular ancestry, emit events. |
| `internal/command/gedcom_commands.go` | [research.md#5](./research.md#5-gedcom-processing) | ImportGedcomCommand. Parses GEDCOM, emits PersonCreated/FamilyCreated/ChildLinked events, returns ImportResult with warnings/errors. |

**Verification**: Unit tests with in-memory EventStore. Given-When-Then pattern from [research.md#3](./research.md#3-event-sourcing-with-cqrs-lite).

### Phase 3b: Query Layer

**Purpose**: CQRS read side. Queries against denormalized read models.

| File | Reference | Key Implementation Notes |
|------|-----------|-------------------------|
| `internal/query/person_queries.go` | [contracts/openapi.yaml](./contracts/openapi.yaml) listPersons, getPerson, searchPersons | ListPersons(limit, offset, sort), GetPerson(id), SearchPersons(query, fuzzy). |
| `internal/query/family_queries.go` | [contracts/openapi.yaml](./contracts/openapi.yaml) listFamilies, getFamily | ListFamilies(limit, offset), GetFamily(id) with denormalized partner/children data. |
| `internal/query/pedigree_queries.go` | [contracts/openapi.yaml](./contracts/openapi.yaml) getPedigree, [research.md#4](./research.md#4-svelte-5--d3js-integration) | GetPedigree(personID, generations). Recursive ancestor traversal using pedigree_edges table. Returns tree structure for D3.js. |

**Verification**: Integration tests with testcontainers (PostgreSQL). Verify query results match expected structure.

### Phase 4: API Layer

**Purpose**: HTTP handlers connecting commands/queries to REST endpoints.

| File | Reference | Key Implementation Notes |
|------|-----------|-------------------------|
| `internal/api/server.go` | [research.md#1](./research.md#1-echo-http-framework-best-practices), [quickstart.md](./quickstart.md#configuration) | Echo server setup. Middleware stack: Recover → RequestID → Logger → CORS → ErrorHandler. Config from env vars. |
| `internal/api/handlers.go` | [contracts/openapi.yaml](./contracts/openapi.yaml) | Generated via oapi-codegen OR hand-written implementing ServerInterface. Maps HTTP requests to commands/queries. |
| `internal/api/middleware.go` | [research.md#1](./research.md#1-echo-http-framework-best-practices) | Custom error handler returning [Error schema](./contracts/openapi.yaml). Structured JSON logging. |
| `internal/api/openapi.go` | [contracts/openapi.yaml](./contracts/openapi.yaml) | Embed and serve OpenAPI spec at /api/docs. |
| `internal/api/validation.go` | [research.md#1](./research.md#1-echo-http-framework-best-practices) | go-playground/validator integration for request validation. |

**API-to-Handler Mapping**:

| Endpoint | Operation | Handler calls |
|----------|-----------|---------------|
| POST /persons | createPerson | CreatePersonCommand → PersonCreated |
| GET /persons | listPersons | ListPersonsQuery |
| GET /persons/{id} | getPerson | GetPersonQuery |
| PUT /persons/{id} | updatePerson | UpdatePersonCommand → PersonUpdated |
| DELETE /persons/{id} | deletePerson | DeletePersonCommand → PersonDeleted |
| POST /families | createFamily | CreateFamilyCommand → FamilyCreated |
| GET /families | listFamilies | ListFamiliesQuery |
| GET /families/{id} | getFamily | GetFamilyQuery |
| PUT /families/{id} | updateFamily | UpdateFamilyCommand → FamilyUpdated |
| DELETE /families/{id} | deleteFamily | DeleteFamilyCommand → FamilyDeleted |
| POST /families/{id}/children | addChildToFamily | LinkChildCommand → ChildLinkedToFamily |
| DELETE /families/{id}/children/{personId} | removeChildFromFamily | UnlinkChildCommand → ChildUnlinkedFromFamily |
| GET /pedigree/{id} | getPedigree | GetPedigreeQuery |
| GET /search | searchPersons | SearchPersonsQuery |
| POST /gedcom/import | importGedcom | ImportGedcomCommand |
| GET /gedcom/export | exportGedcom | ExportGedcomQuery |

**Verification**: Contract tests validating OpenAPI compliance. Use httptest with mocked command/query handlers.

### Phase 5a: GEDCOM Processing

**Purpose**: Import from and export to GEDCOM 5.5 format.

| File | Reference | Key Implementation Notes |
|------|-----------|-------------------------|
| `internal/gedcom/importer.go` | [research.md#5](./research.md#5-gedcom-processing), [data-model.md#GEDCOM Mapping](./data-model.md#gedcom-mapping) | Uses github.com/cacack/gedcom-go. Encoding detection (UTF-8 → Windows-1252 → ISO-8859-1). Graceful degradation per research.md error recovery table. Emits domain events. |
| `internal/gedcom/exporter.go` | [research.md#5](./research.md#5-gedcom-processing), [data-model.md#GEDCOM Mapping](./data-model.md#gedcom-mapping) | Reads from read models, generates GEDCOM 5.5 with UTF-8 encoding. Stable @XREF@ generation from UUIDs. Re-emit preserved custom tags. |

**Verification**: Round-trip tests. Import GEDCOM → export → import again → verify data matches. Test with sample files including edge cases (encoding issues, malformed dates, custom tags).

### Phase 5b: Frontend

**Purpose**: Svelte 5 SPA consuming the REST API.

| File | Reference | Key Implementation Notes |
|------|-----------|-------------------------|
| `web/src/lib/api/client.ts` | [contracts/openapi.yaml](./contracts/openapi.yaml) | TypeScript API client. Can be generated from OpenAPI or hand-written. |
| `web/src/lib/components/PersonCard.svelte` | - | Displays person summary. Used in lists and search results. |
| `web/src/lib/components/FamilyCard.svelte` | - | Displays family with partners and children. |
| `web/src/lib/components/PedigreeChart.svelte` | [research.md#4](./research.md#4-svelte-5--d3js-integration) | D3.js tree layout with Svelte rendering. d3.tree() for layout, Svelte {#each} for nodes/links. d3.zoom() for pan/zoom. |
| `web/src/lib/components/SearchBox.svelte` | - | Search input with debounced API calls. Fuzzy toggle. |
| `web/src/routes/+page.svelte` | - | Home/dashboard. Quick stats, recent activity. |
| `web/src/routes/persons/+page.svelte` | - | Person list with pagination and sorting. |
| `web/src/routes/persons/[id]/+page.svelte` | - | Person detail view and edit form. Shows family relationships. |
| `web/src/routes/families/+page.svelte` | - | Family list and create/edit. |
| `web/src/routes/pedigree/[id]/+page.svelte` | [research.md#4](./research.md#4-svelte-5--d3js-integration) | Full-page pedigree chart. Generation selector. Click-to-navigate. |
| `web/src/routes/import/+page.svelte` | - | GEDCOM file upload. Progress display. Warning/error review. |

**Verification**: Vitest + Svelte Testing Library. Test component rendering and user interactions.

### Phase 6: Integration & Deployment

**Purpose**: Wire everything together, create deployable artifacts.

| File | Reference | Key Implementation Notes |
|------|-----------|-------------------------|
| `cmd/myfamily/main.go` | [quickstart.md](./quickstart.md) | Entry point. Parse flags/env, initialize config, create repositories, wire up API server, embed frontend via go:embed. |
| `internal/config/config.go` | [quickstart.md#Configuration](./quickstart.md#configuration) | Config struct loading from env vars: DATABASE_URL, SQLITE_PATH, PORT, LOG_LEVEL, LOG_FORMAT. |
| `go.mod` | - | Module definition with all dependencies. |
| `Makefile` | [quickstart.md#Build Commands](./quickstart.md#build-commands) | build, test, fmt, vet, generate targets. |
| `Dockerfile` | [quickstart.md#Docker Deployment](./quickstart.md#docker-deployment) | Multi-stage build: build frontend, build Go binary, minimal runtime image. |
| `docker-compose.yml` | [quickstart.md#Docker Deployment](./quickstart.md#docker-deployment) | Services: myfamily app, postgres (optional). |

**Verification**:
- `go build ./...` succeeds
- `go test ./...` passes (all unit + integration tests)
- `docker build` produces working image
- `docker compose up` runs full stack
- Manual E2E: import GEDCOM, browse persons, view pedigree, search, export

### User Story to Implementation Mapping

| User Story | Spec Reference | Implementation Components |
|------------|----------------|--------------------------|
| US1: Import GEDCOM | [spec.md#US1](./spec.md) | `internal/gedcom/importer.go`, ImportGedcomCommand, POST /gedcom/import, import page |
| US2: Manage Persons | [spec.md#US2](./spec.md) | Person commands/queries, /persons endpoints, persons pages |
| US3: Create Families | [spec.md#US3](./spec.md) | Family commands/queries, /families endpoints, families page |
| US4: View Pedigree | [spec.md#US4](./spec.md) | GetPedigreeQuery, /pedigree endpoint, PedigreeChart.svelte |
| US5: Search People | [spec.md#US5](./spec.md) | SearchPersonsQuery (with fuzzy), /search endpoint, SearchBox.svelte |
| US6: Export Data | [spec.md#US6](./spec.md) | `internal/gedcom/exporter.go`, GET /gedcom/export |
| US7: REST API | [spec.md#US7](./spec.md) | All of `internal/api/`, [contracts/openapi.yaml](./contracts/openapi.yaml) |

### Critical Path

The minimum implementation order to achieve a working system:

1. **Domain types** (Person, Family, GenDate, events) - no deps
2. **In-memory EventStore** - enables testing without DB
3. **Person commands** (Create, Update) - basic write path
4. **SQLite EventStore + read models** - real persistence
5. **Person queries** (List, Get) - basic read path
6. **API server + person endpoints** - HTTP access
7. **Family commands/queries + endpoints** - relationships
8. **Pedigree query + endpoint** - visualization data
9. **Search query + endpoint** - find people
10. **GEDCOM import** - data entry at scale
11. **Frontend MVP** - usable UI
12. **GEDCOM export** - data portability
13. **PostgreSQL support** - production database
14. **Docker packaging** - deployment

### Known Deviations & Deferred Items

Items identified during spec-to-plan audit that are intentionally deferred or handled differently:

| Item | Spec Reference | Decision | Rationale |
|------|----------------|----------|-----------|
| **Human-readable output (FR-013)** | FR-013 | Deferred to UI layer | API returns JSON; web UI and any future CLI can format for human readability. No API-level text/plain needed for MVP. |
| **Duplicate detection during import** | Edge Cases | Deferred to post-MVP | Adds UI complexity (merge workflow). Users can search and manually merge. See [research.md §5](./research.md#duplicate-detection-deferred-to-post-mvp). |
| **Import performance (SC-001)** | SC-001 vs Constitution | Implement and measure | Spec: 30s for 5K. Constitution: 10s for 1K. Close enough—implement with batch inserts, optimize if benchmarks show issues. |
