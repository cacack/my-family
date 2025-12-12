# Tasks: My Family Genealogy MVP

**Input**: Design documents from `/specs/001-genealogy-mvp/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/openapi.yaml

**Tests**: Tests are included as part of each phase. The spec requires comprehensive testing per Constitution II (80% coverage target).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

This is a Go web application with embedded Svelte frontend:
- **Backend**: `cmd/myfamily/`, `internal/`
- **Frontend**: `web/src/`
- **Specs**: `specs/001-genealogy-mvp/`

---

## Phase 1: Setup (Project Initialization)

**Purpose**: Initialize Go module, frontend project, and basic tooling

- [x] T001 Initialize Go module with `go mod init` in repository root
- [x] T002 [P] Create Makefile with build, test, fmt, vet, generate targets per quickstart.md
- [x] T003 [P] Create .gitignore for Go, Node, and IDE files
- [x] T004 [P] Initialize Svelte 5 project with Vite in web/ directory
- [x] T005 [P] Configure Tailwind CSS in web/tailwind.config.js
- [x] T006 [P] Copy OpenAPI spec to internal/api/openapi.yaml for embedding
- [x] T007 Add Go dependencies: Echo, Ent, oapi-codegen, validator, uuid, gedcom-go
- [x] T008 Add frontend dependencies: d3, typescript types in web/package.json

**Checkpoint**: `go build ./...` compiles (with empty packages), `cd web && npm run dev` starts

---

## Phase 2: Foundational (Domain & Repository Layers)

**Purpose**: Core domain types and event store that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete. This implements the domain layer and repository interfaces from plan.md Phase 1-2.

### Domain Layer (internal/domain/)

**Reference**: [plan.md Phase 1](./plan.md#phase-1-domain-layer), [data-model.md](./data-model.md)

- [x] T009 [P] Implement DateQualifier enum and GenDate type in internal/domain/date.go per data-model.md#GenDate
- [x] T010 [P] Implement Gender, RelationType, ChildRelationType enums in internal/domain/enums.go per data-model.md#Enumerations
- [x] T011 [P] Implement Place value object in internal/domain/place.go per data-model.md#Place
- [x] T012 Implement Person entity with Validate() in internal/domain/person.go per data-model.md#Person
- [x] T013 Implement Family and FamilyChild entities in internal/domain/family.go per data-model.md#Family
- [x] T014 Implement all domain events in internal/domain/events.go per data-model.md#Domain-Events
- [x] T015 [P] Write table-driven tests for GenDate parsing in internal/domain/date_test.go
- [x] T016 [P] Write tests for Person validation in internal/domain/person_test.go
- [x] T017 [P] Write tests for Family validation in internal/domain/family_test.go
- [x] T018 [P] Write tests for event JSON round-trip in internal/domain/events_test.go

### Repository Layer (internal/repository/)

**Reference**: [plan.md Phase 2](./plan.md#phase-2-repository-layer), [data-model.md#Event-Store-Schema](./data-model.md#event-store-schema)

- [x] T019 Define EventStore interface in internal/repository/eventstore.go per research.md#3
- [x] T020 Define ReadModelStore interface in internal/repository/readmodel.go per data-model.md#Read-Model-Schema
- [x] T021 Implement in-memory EventStore in internal/repository/memory/eventstore.go for testing
- [x] T022 [P] Write EventStore interface tests in internal/repository/eventstore_test.go
- [x] T023 Implement projection handlers in internal/repository/projection.go per research.md#3
- [x] T024 [P] Write projection tests in internal/repository/projection_test.go

### Configuration

- [x] T025 Implement config loading in internal/config/config.go per quickstart.md#Configuration

**Checkpoint**: `go test ./internal/domain/... ./internal/repository/...` passes. Foundation ready for user stories.

---

## Phase 3: User Story 2 - Manage Person Records (Priority: P2)

**Note**: US2 is implemented before US1 because GEDCOM import (US1) depends on having person management working first.

**Goal**: Users can create, read, update, and delete person records with all fields (name, dates, places, gender, notes)

**Independent Test**: Create a person via API, retrieve it, update a field, verify the change persists

**Reference**: [spec.md US2](./spec.md#user-story-2---manage-person-records-priority-p2), [plan.md Phase 3a](./plan.md#phase-3a-command-layer)

### Command Layer for US2

- [x] T026 [US2] Define CommandHandler interface in internal/command/handler.go
- [x] T027 [US2] Implement CreatePersonCommand in internal/command/person_commands.go
- [x] T028 [US2] Implement UpdatePersonCommand in internal/command/person_commands.go
- [x] T029 [US2] Implement DeletePersonCommand in internal/command/person_commands.go
- [x] T030 [P] [US2] Write command tests with in-memory store in internal/command/person_commands_test.go

### Query Layer for US2

- [x] T031 [US2] Implement ListPersonsQuery in internal/query/person_queries.go
- [x] T032 [US2] Implement GetPersonQuery in internal/query/person_queries.go
- [x] T033 [P] [US2] Write query tests in internal/query/person_queries_test.go

### API Layer for US2

**Reference**: [contracts/openapi.yaml](./contracts/openapi.yaml) /persons endpoints

- [x] T034 [US2] Setup Echo server with middleware stack in internal/api/server.go per research.md#1
- [x] T035 [US2] Implement request validation in internal/api/validation.go per research.md#1 (validation in handlers)
- [x] T036 [US2] Implement custom error handler in internal/api/middleware.go
- [x] T037 [US2] Implement createPerson handler in internal/api/handlers.go (POST /persons)
- [x] T038 [US2] Implement listPersons handler in internal/api/handlers.go (GET /persons)
- [x] T039 [US2] Implement getPerson handler in internal/api/handlers.go (GET /persons/{id})
- [x] T040 [US2] Implement updatePerson handler in internal/api/handlers.go (PUT /persons/{id})
- [x] T041 [US2] Implement deletePerson handler in internal/api/handlers.go (DELETE /persons/{id})
- [x] T042 [P] [US2] Write API contract tests in internal/api/handlers_test.go

### SQLite Persistence for US2

**Reference**: [data-model.md#SQLite-Adaptations](./data-model.md#sqlite-adaptations), [plan.md Phase 2](./plan.md#phase-2-repository-layer)

*Deferred: MVP uses in-memory stores. SQLite will be added in Phase 11.*

- [ ] T043 [US2] Implement SQLite EventStore in internal/repository/sqlite/eventstore.go
- [ ] T044 [US2] Implement SQLite read models (persons table) in internal/repository/sqlite/readmodel.go
- [ ] T045 [P] [US2] Write SQLite integration tests in internal/repository/sqlite/eventstore_test.go

### Entry Point for US2

- [x] T046 [US2] Create main.go with server startup in cmd/myfamily/main.go per quickstart.md

**Checkpoint**: Person CRUD works via API. `curl POST /api/v1/persons` creates a person, GET retrieves it.

---

## Phase 4: User Story 3 - Create Family Units (Priority: P3)

**Goal**: Users can create family units linking partners and children, supporting multiple marriages and single-parent families

**Independent Test**: Create two persons, create a family with them as partners, add a child, verify relationships from each person's perspective

**Reference**: [spec.md US3](./spec.md#user-story-3---create-family-units-priority-p3), [plan.md Phase 3a](./plan.md#phase-3a-command-layer)

### Command Layer for US3

- [x] T047 [US3] Implement CreateFamilyCommand in internal/command/family_commands.go
- [x] T048 [US3] Implement UpdateFamilyCommand in internal/command/family_commands.go
- [x] T049 [US3] Implement DeleteFamilyCommand in internal/command/family_commands.go
- [x] T050 [US3] Implement LinkChildCommand with circular ancestry detection in internal/command/family_commands.go
- [x] T051 [US3] Implement UnlinkChildCommand in internal/command/family_commands.go
- [x] T052 [P] [US3] Write family command tests in internal/command/family_commands_test.go

### Query Layer for US3

- [x] T053 [US3] Implement ListFamiliesQuery in internal/query/family_queries.go
- [x] T054 [US3] Implement GetFamilyQuery with denormalized partner/children in internal/query/family_queries.go
- [x] T055 [P] [US3] Write family query tests in internal/query/family_queries_test.go

### API Layer for US3

**Reference**: [contracts/openapi.yaml](./contracts/openapi.yaml) /families endpoints

- [x] T056 [US3] Implement createFamily handler in internal/api/handlers.go (POST /families)
- [x] T057 [US3] Implement listFamilies handler in internal/api/handlers.go (GET /families)
- [x] T058 [US3] Implement getFamily handler in internal/api/handlers.go (GET /families/{id})
- [x] T059 [US3] Implement updateFamily handler in internal/api/handlers.go (PUT /families/{id})
- [x] T060 [US3] Implement deleteFamily handler in internal/api/handlers.go (DELETE /families/{id})
- [x] T061 [US3] Implement addChildToFamily handler in internal/api/handlers.go (POST /families/{id}/children)
- [x] T062 [US3] Implement removeChildFromFamily handler in internal/api/handlers.go (DELETE /families/{id}/children/{personId})
- [x] T063 [P] [US3] Write family API tests in internal/api/family_handlers_test.go

### SQLite Persistence for US3

*Deferred: MVP uses in-memory stores. SQLite will be added in Phase 11.*

- [ ] T064 [US3] Add families and family_children tables to internal/repository/sqlite/readmodel.go
- [ ] T065 [US3] Add pedigree_edges table to internal/repository/sqlite/readmodel.go
- [x] T066 [US3] Update person projections to include family relationships (done in repository/projection.go)

**Checkpoint**: Family CRUD works via API. Can create family, add children, verify relationships.

---

## Phase 5: User Story 1 - Import GEDCOM (Priority: P1)

**Goal**: Users can import GEDCOM 5.5 files preserving all individuals, families, relationships, and date precision

**Independent Test**: Import a valid GEDCOM file, verify all persons and families are created with correct data

**Reference**: [spec.md US1](./spec.md#user-story-1---import-existing-research-priority-p1), [research.md#5](./research.md#5-gedcom-processing), [plan.md Phase 5a](./plan.md#phase-5a-gedcom-processing)

### GEDCOM Processing for US1

*Note: Used iand/gedcom library instead of cacack/gedcom-go (more mature, better API).*

- [x] T067 [US1] Implement GEDCOM parser wrapper using github.com/iand/gedcom in internal/gedcom/importer.go
- [x] T068 [US1] Implement encoding detection (UTF-8 → Windows-1252 → ISO-8859-1) in internal/gedcom/importer.go
- [x] T069 [US1] Implement GEDCOM date parsing to GenDate in internal/gedcom/importer.go per research.md#5
- [x] T070 [US1] Implement individual (INDI) to PersonCreated event mapping in internal/gedcom/importer.go
- [x] T071 [US1] Implement family (FAM) to FamilyCreated/ChildLinked events in internal/gedcom/importer.go
- [x] T072 [US1] Implement graceful error recovery per research.md#5 error table in internal/gedcom/importer.go
- [x] T073 [US1] Implement ImportGedcomCommand in internal/command/gedcom_commands.go
- [x] T074 [P] [US1] Write GEDCOM import tests with sample files in internal/gedcom/importer_test.go

### API Layer for US1

**Reference**: [contracts/openapi.yaml](./contracts/openapi.yaml) POST /gedcom/import

- [x] T075 [US1] Implement importGedcom handler with multipart file upload in internal/api/handlers.go
- [x] T076 [P] [US1] Write import API tests in internal/api/import_handlers_test.go

**Checkpoint**: GEDCOM import works via API. Upload a .ged file, verify persons and families created.

---

## Phase 6: User Story 4 - View Pedigree Chart (Priority: P4)

**Goal**: Users can view an interactive ancestor chart for any person with pan/zoom navigation

**Independent Test**: Select a person with known ancestors, view pedigree, navigate to different person, verify accuracy

**Reference**: [spec.md US4](./spec.md#user-story-4---view-pedigree-chart-priority-p4), [research.md#4](./research.md#4-svelte-5--d3js-integration), [plan.md Phase 3b](./plan.md#phase-3b-query-layer)

### Query Layer for US4

- [x] T077 [US4] Implement GetPedigreeQuery with recursive ancestor traversal in internal/query/pedigree_queries.go
- [x] T078 [P] [US4] Write pedigree query tests in internal/query/pedigree_queries_test.go

### API Layer for US4

**Reference**: [contracts/openapi.yaml](./contracts/openapi.yaml) GET /pedigree/{id}

- [x] T079 [US4] Implement getPedigree handler in internal/api/handlers.go
- [x] T080 [P] [US4] Write pedigree API tests in internal/api/pedigree_handlers_test.go

### Frontend for US4

**Reference**: [research.md#4](./research.md#4-svelte-5--d3js-integration)

- [x] T081 [US4] Create API client in web/src/lib/api/client.ts per contracts/openapi.yaml
- [x] T082 [US4] Implement PedigreeChart component with D3.js tree layout in web/src/lib/components/PedigreeChart.svelte
- [x] T083 [US4] Add pan/zoom navigation with d3.zoom() in web/src/lib/components/PedigreeChart.svelte
- [x] T084 [US4] Implement click-to-navigate on ancestor nodes in PedigreeChart.svelte
- [x] T085 [US4] Create pedigree page route in web/src/routes/pedigree/[id]/+page.svelte
- [ ] T086 [P] [US4] Write PedigreeChart component tests in web/src/lib/components/PedigreeChart.test.ts

**Checkpoint**: Pedigree chart displays ancestors, pan/zoom works, clicking ancestor re-centers chart.

---

## Phase 7: User Story 5 - Search People (Priority: P5)

**Goal**: Users can search by name with partial and fuzzy matching, results within 1 second for 10K individuals

**Independent Test**: Search partial name, verify relevant results; search misspelling, verify fuzzy match finds correct person

**Reference**: [spec.md US5](./spec.md#user-story-5---search-people-priority-p5), [research.md#6](./research.md#6-full-text-search)

### Query Layer for US5

*Note: Basic search implemented with in-memory store. FTS5 and fuzzy matching deferred to Phase 11 (SQLite).*

- [x] T087 [US5] Implement SearchPersonsQuery in internal/query/person_queries.go (basic contains matching for MVP)
- [ ] T088 [US5] Implement fuzzy matching (deferred to Phase 11 with SQLite FTS5)
- [x] T089 [P] [US5] Write search query tests in internal/query/person_queries_test.go

### Repository Layer for US5

*Deferred: FTS5 will be added in Phase 11 with SQLite.*

- [ ] T090 [US5] Add FTS5 virtual table and sync triggers to internal/repository/sqlite/readmodel.go per data-model.md#SQLite-FTS5

### API Layer for US5

**Reference**: [contracts/openapi.yaml](./contracts/openapi.yaml) GET /search

- [x] T091 [US5] Implement searchPersons handler in internal/api/handlers.go
- [x] T092 [P] [US5] Write search API tests in internal/api/handlers_test.go

### Frontend for US5

- [x] T093 [US5] Implement SearchBox component with debounce in web/src/lib/components/SearchBox.svelte
- [x] T094 [US5] Add search to header/navigation in web/src/routes/+layout.svelte
- [ ] T095 [P] [US5] Write SearchBox component tests in web/src/lib/components/SearchBox.test.ts

**Checkpoint**: Search works via API and UI. Partial names and fuzzy matches return correct results.

---

## Phase 8: User Story 6 - Export Data (Priority: P6)

**Goal**: Users can export all data as valid GEDCOM 5.5 file with round-trip fidelity

**Independent Test**: Export data, re-import to another tool, verify all data preserved

**Reference**: [spec.md US6](./spec.md#user-story-6---export-data-priority-p6), [research.md#5](./research.md#5-gedcom-processing)

### GEDCOM Export for US6

- [x] T096 [US6] Implement GEDCOM 5.5 generation from read models in internal/gedcom/exporter.go
- [x] T097 [US6] Implement stable @XREF@ generation from UUIDs in internal/gedcom/exporter.go
- [x] T098 [US6] Implement GenDate to GEDCOM date format conversion in internal/gedcom/exporter.go
- [x] T099 [P] [US6] Write export tests in internal/gedcom/exporter_test.go
- [x] T100 [P] [US6] Write round-trip test in internal/api/export_handlers_test.go (TestExportGedcom_RoundTrip)

### API Layer for US6

**Reference**: [contracts/openapi.yaml](./contracts/openapi.yaml) GET /gedcom/export

- [x] T101 [US6] Implement exportGedcom handler in internal/api/handlers.go
- [x] T102 [P] [US6] Write export API tests in internal/api/export_handlers_test.go

**Checkpoint**: Export produces valid GEDCOM 5.5 file. Round-trip preserves all data.

---

## Phase 9: User Story 7 - REST API Access (Priority: P7)

**Goal**: All functionality exposed via well-documented REST API with consistent JSON responses

**Independent Test**: Perform all operations via API only, verify responses match OpenAPI spec

**Reference**: [spec.md US7](./spec.md#user-story-7---rest-api-access-priority-p7), [contracts/openapi.yaml](./contracts/openapi.yaml)

### API Documentation for US7

- [ ] T103 [US7] Embed OpenAPI spec and serve at /api/docs in internal/api/openapi.go
- [ ] T104 [US7] Add Swagger UI or ReDoc for interactive documentation
- [ ] T105 [P] [US7] Verify all endpoints match OpenAPI spec (contract test coverage)

**Checkpoint**: API docs accessible at /api/docs. All operations work via API alone.

---

## Phase 10: Frontend MVP (Web UI)

**Goal**: Web UI consuming REST API for all user operations

**Reference**: [plan.md Phase 5b](./plan.md#phase-5b-frontend)

### Core Components

- [x] T106 [P] Implement PersonCard component in web/src/lib/components/PersonCard.svelte
- [x] T107 [P] Implement FamilyCard component in web/src/lib/components/FamilyCard.svelte

### Pages

- [x] T108 Create home/dashboard page in web/src/routes/+page.svelte
- [x] T109 Create person list page with pagination in web/src/routes/persons/+page.svelte
- [x] T110 Create person detail/edit page in web/src/routes/persons/[id]/+page.svelte
- [x] T111 Create family management page in web/src/routes/families/+page.svelte
- [x] T112 Create GEDCOM import page with file upload in web/src/routes/import/+page.svelte

### Integration

- [x] T113 Embed frontend build in Go binary via go:embed in cmd/myfamily/main.go
- [x] T114 Configure Vite build output for embedding

**Checkpoint**: Full web UI works. Can browse persons, view pedigree, search, import/export via UI.

---

## Phase 11: PostgreSQL Support (Production Database)

**Goal**: Support PostgreSQL as primary database with tsvector search and pg_trgm fuzzy matching

**Reference**: [data-model.md#Event-Store-Schema](./data-model.md#event-store-schema), [research.md#6](./research.md#6-full-text-search)

- [ ] T115 [P] Implement PostgreSQL EventStore in internal/repository/postgres/eventstore.go
- [ ] T116 [P] Implement PostgreSQL read models with tsvector in internal/repository/postgres/readmodel.go
- [ ] T117 Add pg_trgm fuzzy search for PostgreSQL in internal/query/person_queries.go
- [ ] T118 Write PostgreSQL integration tests with testcontainers in internal/repository/postgres/eventstore_test.go
- [ ] T119 Add database auto-detection/selection in internal/config/config.go

**Checkpoint**: Same API works with both SQLite and PostgreSQL. Tests pass on both.

---

## Phase 12: Polish & Deployment

**Purpose**: Docker packaging, documentation, final validation

**Reference**: [plan.md Phase 6](./plan.md#phase-6-integration--deployment), [quickstart.md](./quickstart.md)

- [ ] T120 [P] Create Dockerfile with multi-stage build per quickstart.md#Docker-Deployment
- [ ] T121 [P] Create docker-compose.yml with app and postgres services
- [ ] T122 Update README.md with setup instructions
- [ ] T123 Run full E2E validation per plan.md verification criteria
- [ ] T124 Performance test: import 5K GEDCOM, search 10K tree, verify timing meets SC criteria

**Checkpoint**: `docker compose up` runs full stack. All success criteria validated.

---

## Dependencies & Execution Order

### Phase Dependencies

```
Phase 1: Setup
    ↓
Phase 2: Foundational (Domain + Repository) ← BLOCKS ALL USER STORIES
    ↓
┌───────────────────────────────────────────────────────────────┐
│ User Stories can proceed in priority order or parallel:       │
│                                                               │
│   Phase 3: US2 (Person CRUD) ← Required for US1, US3          │
│       ↓                                                       │
│   Phase 4: US3 (Families) ← Required for US1, US4             │
│       ↓                                                       │
│   Phase 5: US1 (Import) ← Can now use Person + Family         │
│       ↓                                                       │
│   Phase 6: US4 (Pedigree) ← Uses pedigree_edges from US3      │
│       ↓                                                       │
│   Phase 7: US5 (Search) ← Independent                         │
│       ↓                                                       │
│   Phase 8: US6 (Export) ← Uses read models                    │
│       ↓                                                       │
│   Phase 9: US7 (API Docs) ← All endpoints exist               │
└───────────────────────────────────────────────────────────────┘
    ↓
Phase 10: Frontend MVP
    ↓
Phase 11: PostgreSQL (parallel with Phase 10)
    ↓
Phase 12: Polish & Deployment
```

### Story Dependencies

| Story | Depends On | Can Parallel With |
|-------|------------|-------------------|
| US2 (Persons) | Foundation only | - |
| US3 (Families) | US2 (needs persons) | - |
| US1 (Import) | US2, US3 | - |
| US4 (Pedigree) | US3 (needs pedigree_edges) | US5, US6 |
| US5 (Search) | US2 (needs persons table) | US4, US6 |
| US6 (Export) | US2, US3 | US4, US5 |
| US7 (API Docs) | All endpoints | - |

### Parallel Opportunities by Phase

**Phase 2 (Foundational)**:
```
Parallel: T009, T010, T011 (domain types)
Parallel: T015, T016, T017, T018 (domain tests)
Parallel: T022, T024 (repository tests)
```

**Phase 3 (US2)**:
```
Parallel: T030, T033, T042, T045 (all tests)
```

**Later Phases**:
- US4, US5, US6 can run in parallel after US3 completes
- Frontend (Phase 10) can parallel with PostgreSQL (Phase 11)

---

## Implementation Strategy

### MVP First (Phases 1-5)

1. Complete Setup (Phase 1)
2. Complete Foundational (Phase 2) - **CRITICAL GATE**
3. Complete US2: Person Management (Phase 3)
4. Complete US3: Family Units (Phase 4)
5. Complete US1: GEDCOM Import (Phase 5)
6. **STOP and VALIDATE**: Import a real GEDCOM file, verify data
7. Deploy/demo if ready - users can import existing research!

### Incremental Delivery

| Increment | Phases | User Value |
|-----------|--------|------------|
| MVP | 1-5 | Import GEDCOM, manage persons/families via API |
| +Pedigree | 6 | Visualize ancestry |
| +Search | 7 | Find people quickly |
| +Export | 8 | Backup and share data |
| +Docs | 9 | Developer integration |
| +UI | 10 | Full web interface |
| +Production | 11-12 | PostgreSQL, Docker deployment |

---

## Summary

| Phase | Tasks | Purpose |
|-------|-------|---------|
| 1 | T001-T008 (8) | Setup |
| 2 | T009-T025 (17) | Foundational |
| 3 (US2) | T026-T046 (21) | Person Management |
| 4 (US3) | T047-T066 (20) | Family Units |
| 5 (US1) | T067-T076 (10) | GEDCOM Import |
| 6 (US4) | T077-T086 (10) | Pedigree Chart |
| 7 (US5) | T087-T095 (9) | Search |
| 8 (US6) | T096-T102 (7) | Export |
| 9 (US7) | T103-T105 (3) | API Docs |
| 10 | T106-T114 (9) | Frontend |
| 11 | T115-T119 (5) | PostgreSQL |
| 12 | T120-T124 (5) | Deployment |
| **Total** | **124 tasks** | |

### Tasks per User Story

| Story | Task Count | Key Deliverable |
|-------|------------|-----------------|
| US1 | 10 | GEDCOM import |
| US2 | 21 | Person CRUD + API |
| US3 | 20 | Family management |
| US4 | 10 | Pedigree visualization |
| US5 | 9 | Name search |
| US6 | 7 | GEDCOM export |
| US7 | 3 | API documentation |

### Independent Test Criteria

| Story | How to Verify Independence |
|-------|---------------------------|
| US1 | Import GEDCOM → verify persons/families exist |
| US2 | Create person via POST → GET returns same data |
| US3 | Create family with children → verify from each person's view |
| US4 | Open pedigree → ancestors displayed correctly |
| US5 | Search partial name → relevant results returned |
| US6 | Export → reimport → data matches |
| US7 | All operations via curl/API client → responses match spec |
