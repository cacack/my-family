# Architectural Invariants

Rules that must hold true in the my-family codebase. Violations break architectural contracts.

---

## How to Use This Document

- **During Development**: Check your changes don't violate any invariants
- **During PR Review**: Verify invariants remain intact
- **In CI**: Automated tests verify testable invariants
- **When Adding Features**: Ensure new code establishes invariants for new patterns

---

## Invariant Categories

### Event Sourcing Invariants (ES) - Source: [ADR-001](./adr/001-event-sourcing-cqrs.md)

| ID | Rule | Verification |
|----|------|--------------|
| **ES-001** | All state changes emit domain events | Code review: no direct ReadModelStore writes in command handlers |
| **ES-002** | Events are append-only, never modified or deleted | EventStore interface has no Update/Delete methods |
| **ES-003** | Every domain entity has a `Version` field | Schema inspection; compile-time check |
| **ES-004** | Projections can be rebuilt from events | Projection rebuild test exists |
| **ES-005** | Events implement `Event` interface (`EventType()`, `AggregateID()`, `OccurredAt()`) | Compile-time interface satisfaction |
| **ES-006** | Event factories use `NewBaseEvent()` for consistent timestamps | Code review; factory tests |
| **ES-007** | `DecodeEvent()` handles all event types | Integration test with all event types |

### Database Invariants (DB) - Source: [ADR-002](./adr/002-dual-database-strategy.md)

| ID | Rule | Verification |
|----|------|--------------|
| **DB-001** | Both PostgreSQL and SQLite pass identical interface tests | Shared test suite runs against both |
| **DB-002** | `EventStore.Append` fails on version mismatch (optimistic locking) | Concurrency test |
| **DB-003** | `ReadModelStore` returns `nil` (not error) for missing entities | Interface contract test |
| **DB-004** | No PostgreSQL-specific features without SQLite fallback or graceful degradation | Feature parity checklist |
| **DB-005** | Full-text search works on both databases (tsvector vs FTS5) | Search integration test |

### Projection Invariants (PR) - Source: [ADR-003](./adr/003-synchronous-projections.md)

| ID | Rule | Verification |
|----|------|--------------|
| **PR-001** | Projections update in same transaction as event append | Code review; transaction test |
| **PR-002** | Read model version matches event stream version | Version consistency test |
| **PR-003** | Deleted entities removed from read model | Deletion projection test |
| **PR-004** | New event types have corresponding projection handlers | Projection coverage check |

### Deployment Invariants (DP) - Source: [ADR-004](./adr/004-single-binary-deployment.md)

| ID | Rule | Verification |
|----|------|--------------|
| **DP-001** | Single binary contains embedded frontend | Build verification |
| **DP-002** | Development mode supports frontend hot reload | Manual verification |
| **DP-003** | API and frontend served from same origin (no CORS needed) | Configuration check |

### Domain Model Invariants (DM) - Source: [ETHOS.md](./ETHOS.md) + Code Patterns

| ID | Rule | Verification |
|----|------|--------------|
| **DM-001** | Every domain entity has UUID `ID` field set by constructor | `NewX()` factory tests |
| **DM-002** | Domain entities have `Validate()` method | Interface check |
| **DM-003** | GEDCOM-representable entities have `GedcomXref` field | Schema inspection |
| **DM-004** | Validation errors use `ValidationError` type with `Field` + `Message` | Error type check |
| **DM-005** | `GenDate` used for all genealogical dates (supports qualifiers) | Type usage audit |
| **DM-006** | Enum types have `IsValid()` method | Enum pattern check |

### Data Integrity Invariants (DI) - Source: [ETHOS.md](./ETHOS.md) - "Respect the Data"

| ID | Rule | Verification |
|----|------|--------------|
| **DI-001** | Required fields enforced by `Validate()` | Validation tests |
| **DI-002** | Date ordering enforced (death >= birth) where applicable | Validation tests |
| **DI-003** | GEDCOM import/export is lossless for supported entities | Round-trip test |
| **DI-004** | No data loss on standard operations | Event sourcing ensures (ES-001, ES-002) |

### API Invariants (API) - Source: [CONVENTIONS.md](./CONVENTIONS.md)

| ID | Rule | Verification |
|----|------|--------------|
| **API-001** | All endpoints return standard error format | API error tests |
| **API-002** | List endpoints support pagination via `ListOptions` | Pagination tests |
| **API-003** | HTTP 404 for not found, 400 for validation, 409 for conflict | Status code tests |
| **API-004** | Plural nouns for collections (`/persons`, `/families`) | OpenAPI spec review |
| **API-005** | API changes reflected in OpenAPI spec | oapi-codegen generation check |

### Quality Invariants (QA) - Source: [ETHOS.md](./ETHOS.md) - GPS Compliance

| ID | Rule | Verification |
|----|------|--------------|
| **QA-001** | Quality scores are 0-100 | Score bounds test |
| **QA-002** | Missing required fields generate quality issues | Issue detection test |
| **QA-003** | Orphan persons (no family connections) are flagged | Orphan detection test |

### Test Invariants (TS) - Source: [CONTRIBUTING.md](../CONTRIBUTING.md)

| ID | Rule | Verification |
|----|------|--------------|
| **TS-001** | 85% per-package test coverage | `make check-coverage` |
| **TS-002** | Tests are deterministic (no flaky tests) | CI stability |
| **TS-003** | Table-driven tests preferred for multiple cases | Code review |

---

## Invariant Summary by Source

| Source Document | Invariant IDs | Count |
|-----------------|---------------|-------|
| ADR-001 (Event Sourcing) | ES-001 through ES-007 | 7 |
| ADR-002 (Dual Database) | DB-001 through DB-005 | 5 |
| ADR-003 (Sync Projections) | PR-001 through PR-004 | 4 |
| ADR-004 (Single Binary) | DP-001 through DP-003 | 3 |
| ETHOS.md | DM-001 through DM-006, DI-001 through DI-004, QA-001 through QA-003 | 13 |
| CONVENTIONS.md | API-001 through API-005 | 5 |
| CONTRIBUTING.md | TS-001 through TS-003 | 3 |
| **Total** | | **40** |

---

## Adding New Invariants

When establishing new architectural patterns:

1. Document the invariant in this file with unique ID (category prefix + number)
2. Reference the source (ADR, ETHOS.md, or new decision)
3. Define verification method
4. Add automated test if possible
5. Update [INTEGRATION-MATRIX.md](./INTEGRATION-MATRIX.md) if it affects feature checklists
6. Update [TESTING-STRATEGY.md](./TESTING-STRATEGY.md) with test mapping

---

## Invariant Violation Process

If you need to violate an invariant:

1. **Stop** - Invariants exist for good reasons
2. **Ask** - Is there a way to achieve the goal without violation?
3. **Document** - If violation is necessary, create ADR explaining why
4. **Update** - Modify invariant or mark as superseded with rationale
5. **Notify** - Ensure downstream documentation is updated

---

## See Also

- [INTEGRATION-MATRIX.md](./INTEGRATION-MATRIX.md) - Feature integration checklists
- [TESTING-STRATEGY.md](./TESTING-STRATEGY.md) - Tests that verify invariants
- [adr/](./adr/) - Architectural decisions these invariants derive from
- [ETHOS.md](./ETHOS.md) - Guiding principles

---

## Related

- [CONVENTIONS.md](./CONVENTIONS.md) - Code patterns and standards
- [../CONTRIBUTING.md](../CONTRIBUTING.md) - Development workflow
