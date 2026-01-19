# Testing Strategy

Test organization, coverage requirements, and cross-feature verification for my-family.

---

## Test Categories

### Unit Tests

**Purpose**: Verify individual functions and methods in isolation.

**Location**: Same package, `*_test.go` suffix

**Pattern**: Table-driven tests (see [CONVENTIONS.md](./CONVENTIONS.md))

**Coverage Target**: 85% per package (enforced by CI)

```go
func TestValidatePerson(t *testing.T) {
    tests := []struct {
        name    string
        person  domain.Person
        wantErr bool
    }{
        {"valid person", validPerson(), false},
        {"missing given name", personWithoutName(), true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.person.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Integration Tests

**Purpose**: Verify components work together correctly.

**Location**: Same package, `*_integration_test.go` suffix

**Categories**:

| Category | What It Tests | Invariants Verified |
|----------|---------------|---------------------|
| Database | Both PostgreSQL and SQLite implementations | DB-001, DB-002, DB-003 |
| Event Flow | Command -> Event -> Projection -> ReadModel | ES-001, PR-001, PR-002 |
| API | HTTP endpoints return correct responses | API-001, API-002, API-003 |
| GEDCOM | Import/export round-trip | DI-003, DM-003 |
| Search | Full-text search on both databases | DB-005 |

---

## Integration Test Scenarios

Cross-feature tests that verify the system works as a whole.

### Critical Priority

| Scenario | Description | Invariants | Automation |
|----------|-------------|------------|------------|
| **Entity Lifecycle** | Create, update, delete entity; verify events emitted, read model updated, history queryable | ES-001, PR-001, PR-002, PR-003 | `*_integration_test.go` |
| **GEDCOM Round-Trip** | Import GEDCOM, export GEDCOM, compare for losslessness | DI-003, DM-003 | `gedcom/roundtrip_test.go` |
| **Dual Database Parity** | Run identical test suite against PostgreSQL and SQLite | DB-001 | Shared test suite |

### High Priority

| Scenario | Description | Invariants | Automation |
|----------|-------------|------------|------------|
| **Citation Chain** | Create source, create citation referencing source, verify source citation count updated | PR-001 | `citation_integration_test.go` |
| **Family Relationships** | Create persons, create family, link children; verify bidirectional relationships | Relationship consistency | `family_integration_test.go` |
| **Optimistic Locking** | Attempt concurrent updates, verify version mismatch fails | DB-002 | `eventstore_test.go` |

### Medium Priority

| Scenario | Description | Invariants | Automation |
|----------|-------------|------------|------------|
| **Quality Computation** | Create person with varying completeness, verify quality scores | QA-001, QA-002 | `quality_test.go` |
| **Search Indexing** | Create entity, search for it, verify found on both databases | DB-005 | `search_integration_test.go` |
| **Projection Rebuild** | Wipe read models, rebuild from events, verify consistency | ES-004 | `projection_rebuild_test.go` |
| **Orphan Detection** | Create disconnected person, verify flagged by quality service | QA-003 | `quality_test.go` |

---

## Dual-Database Testing Pattern

Both PostgreSQL and SQLite must pass identical tests.

```go
// Shared test suite pattern for dual database
func TestReadModelStore(t *testing.T) {
    stores := []struct {
        name  string
        setup func(t *testing.T) repository.ReadModelStore
    }{
        {"PostgreSQL", setupPostgresStore},
        {"SQLite", setupSQLiteStore},
    }

    for _, s := range stores {
        t.Run(s.name, func(t *testing.T) {
            store := s.setup(t)
            t.Cleanup(func() { /* cleanup */ })

            // Run identical tests
            t.Run("SaveAndGet", func(t *testing.T) {
                testSaveAndGet(t, store)
            })
            t.Run("List", func(t *testing.T) {
                testList(t, store)
            })
            t.Run("Search", func(t *testing.T) {
                testSearch(t, store)
            })
        })
    }
}
```

---

## E2E Tests

**Purpose**: Verify complete user workflows through the UI.

**Tool**: Playwright

**Critical Paths**:

1. First-time setup and GEDCOM import
2. Add person -> Edit details -> View in pedigree chart
3. Add source -> Create citation -> Attach to person fact
4. Search for person by name
5. View person history/audit trail

---

## Invariant Test Mapping

Quick reference for which tests verify which invariants.

| Invariant | Test File(s) | Status |
|-----------|--------------|--------|
| ES-001 | `command/*_test.go` - verify events emitted | Automated |
| ES-002 | `repository/eventstore_test.go` - no Update method | Automated |
| ES-003 | Schema inspection | Code review |
| ES-004 | `repository/projection_rebuild_test.go` | Automated |
| ES-005 | Compile-time check | Automated |
| ES-006 | `domain/events_test.go` - factory tests | Automated |
| ES-007 | `repository/eventstore_test.go` - decode all types | Automated |
| DB-001 | Shared test suite | Automated |
| DB-002 | `repository/eventstore_test.go` - concurrency | Automated |
| DB-003 | `repository/*_test.go` - nil for missing | Automated |
| DB-004 | Feature parity checklist | Code review |
| DB-005 | `repository/search_test.go` | Automated |
| PR-001 | `command/*_test.go` - transaction test | Automated |
| PR-002 | `integration/*_test.go` - version check | Automated |
| PR-003 | `repository/projection_test.go` - deletion | Automated |
| PR-004 | Projection coverage check | Code review |
| DP-001 | Build verification | CI |
| DP-002 | Manual verification | Manual |
| DP-003 | Configuration check | Manual |
| DM-001 | `domain/*_test.go` - NewX tests | Automated |
| DM-002 | Interface check | Compile-time |
| DM-003 | Schema inspection | Code review |
| DM-004 | `domain/*_test.go` - error types | Automated |
| DM-005 | Type usage audit | Code review |
| DM-006 | `domain/enums_test.go` | Automated |
| DI-001 | `domain/*_test.go` - validation | Automated |
| DI-002 | `domain/person_test.go` - date ordering | Automated |
| DI-003 | `gedcom/roundtrip_test.go` | Automated |
| DI-004 | Event sourcing (ES-001, ES-002) | Automated |
| API-001 | `api/handler_test.go` - error format | Automated |
| API-002 | `api/handler_test.go` - pagination | Automated |
| API-003 | `api/handler_test.go` - status codes | Automated |
| API-004 | OpenAPI spec review | Code review |
| API-005 | oapi-codegen generation | CI |
| QA-001 | `query/quality_test.go` - bounds | Automated |
| QA-002 | `query/quality_test.go` - issues | Automated |
| QA-003 | `query/quality_test.go` - orphans | Automated |
| TS-001 | `make check-coverage` | CI |
| TS-002 | CI stability | CI |
| TS-003 | Code review | Code review |

---

## Manual Test Checklist

Some things require human verification.

| Category | Check | When |
|----------|-------|------|
| **Visual** | UI renders correctly on mobile | Before release |
| **Visual** | Charts/visualizations display properly | After D3 changes |
| **a11y** | Keyboard navigation works | Before release |
| **a11y** | Screen reader announces correctly | Major UI changes |
| **Performance** | Large tree (10K+ persons) loads acceptably | Performance changes |
| **Deployment** | Single binary runs on fresh system | Before release |

---

## Test Data Fixtures

Location: `testdata/`

| File | Purpose |
|------|---------|
| `valid.ged` | Valid GEDCOM for import tests |
| `edge-cases.ged` | GEDCOM with unusual but valid data |
| `invalid.ged` | Invalid GEDCOM for error handling tests |
| `large-tree.ged` | Large GEDCOM for performance tests |

---

## Coverage Requirements

| Package | Minimum | Rationale |
|---------|---------|-----------|
| `domain/*` | 90% | Core business logic |
| `command/*` | 85% | Write path |
| `query/*` | 85% | Read path |
| `repository/*` | 85% | Data access |
| `api/*` | 80% | HTTP layer |
| `gedcom/*` | 85% | Import/export |
| `web/*` (tests) | 70% | Frontend components |

---

## CI Pipeline Integration

```yaml
test:
  steps:
    - go test ./... -coverprofile=coverage.out
    - make check-coverage  # Fails if below 85%
    - docker-compose up -d postgres
    - DATABASE_URL=... go test ./... -tags integration
    - npm test --prefix web
```

---

## See Also

- [ARCHITECTURAL-INVARIANTS.md](./ARCHITECTURAL-INVARIANTS.md) - Rules tests verify
- [INTEGRATION-MATRIX.md](./INTEGRATION-MATRIX.md) - Feature checklists
- [CONVENTIONS.md](./CONVENTIONS.md) - Code patterns including test style
- [../CONTRIBUTING.md](../CONTRIBUTING.md) - Development workflow

---

## Related

- [adr/](./adr/) - Architectural decisions
- [ETHOS.md](./ETHOS.md) - Guiding principles
