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

**Location**: two homes, by scope.

- **Single-layer** integration tests are co-located in the package they test (e.g.
  `internal/api/branch_handlers_test.go` drives the HTTP server over memory stores;
  `internal/repository/postgres/*_test.go` drives a real database).
- **Cross-layer** scenarios that must hold on every backend live in `internal/integration/`, a
  test-only package that builds the real `api.Server` over each storage backend in turn.

There is no `integration` build tag; `go test ./...` runs everything, and tests requiring Docker
skip themselves when it is absent.

**Categories**:

| Category | What It Tests | Invariants Verified |
|----------|---------------|---------------------|
| Database | Both PostgreSQL and SQLite implementations | DB-001, DB-002, DB-003 |
| Event Flow | Command -> Event -> Projection -> ReadModel | ES-001, PR-001, PR-002 |
| API | HTTP endpoints return correct responses | API-001, API-002, API-003 |
| GEDCOM | Import/export round-trip | DI-003, DM-003 |
| Search | Full-text search on both databases | DB-005 |
| Branches | Branch lifecycle, diff, merge and conflict resolution on every backend | BR-003, BR-004, BR-005 |

---

## Integration Test Scenarios

Cross-feature tests that verify the system works as a whole.

### Critical Priority

| Scenario | Description | Invariants | Automation |
|----------|-------------|------------|------------|
| **Entity Lifecycle** | Create, update, delete entity; verify events emitted, read model updated, history queryable | ES-001, PR-001, PR-002, PR-003 | `internal/command/*_test.go`, `internal/repository/projection_test.go` |
| **GEDCOM Round-Trip** | Import GEDCOM, export GEDCOM, compare for losslessness | DI-003, DM-003 | `internal/gedcom/integration_test.go` |
| **Dual Database Parity** | Run identical scenario bodies against memory, SQLite and PostgreSQL | DB-001 | `internal/integration/`, plus the per-backend `branch_scenario_test.go` trio — see [Dual-Database Testing Pattern](#dual-database-testing-pattern) |
| **Branch Merge** | Create a branch, edit multiple entity types on it, compare against the mainline, merge, verify the mainline took the changes append-only and the merged branch is no longer readable | BR-004, BR-005, ES-002 | `internal/integration/branch_lifecycle_test.go` |
| **Branch Conflict Resolution** | Detect edit/edit, delete/edit and relationship conflicts through `compare`; refuse an undecided merge; resolve to either side and verify the mainline outcome | BR-003, BR-004 | `internal/integration/branch_conflict_test.go` |

### High Priority

| Scenario | Description | Invariants | Automation |
|----------|-------------|------------|------------|
| **Citation Chain** | Create source, create citation referencing source, verify source citation count updated | PR-001 | `internal/citation/*_test.go` |
| **Family Relationships** | Create persons, create family, link children; verify bidirectional relationships | Relationship consistency | `internal/command/family_commands_test.go`, `internal/query/family_queries_test.go` |
| **Optimistic Locking** | Attempt concurrent updates, verify version mismatch fails | DB-002 | `internal/repository/eventstore_test.go` |

### Medium Priority

| Scenario | Description | Invariants | Automation |
|----------|-------------|------------|------------|
| **Quality Computation** | Create person with varying completeness, verify quality scores | QA-001, QA-002 | `internal/query/quality_service_test.go` |
| **Search Indexing** | Create entity, search for it, verify found on both databases | DB-005 | `internal/repository/soundex_test.go`, plus search cases in each backend's `readmodel_test.go` |
| **Projection Rebuild** | Wipe read models, rebuild from events, verify consistency | ES-004 | `internal/repository/projection_test.go` |
| **Orphan Detection** | Create disconnected person, verify flagged by quality service | QA-003 | `internal/query/quality_service_test.go` |

---

## Dual-Database Testing Pattern

Both PostgreSQL and SQLite must pass identical tests (DB-001). Two mechanisms do this, for two
different reasons.

### 1. Table-driven, one copy — `internal/integration/`

Cross-layer scenarios that drive the real HTTP server live here. The scenario body is written once
and iterated over a backend table, so adding a backend adds it to every scenario at once.

```go
// internal/integration/harness_test.go
var backends = []backend{
    {"Memory", setupMemory},
    {"SQLite", setupSQLite},
    {"Postgres", setupPostgres},
}

// internal/integration/branch_conflict_test.go
func TestBranchConflict_EditEdit(t *testing.T) {
    forEachBackend(t, runEditEditConflict)  // one body, three backends
}
```

`stores` holds the repository *interfaces*, not the concrete types, so a scenario cannot reach for a
backend-specific method. `setupPostgres` runs a testcontainer and calls `t.Skip` under `-short` or
when Docker is absent, so `go test ./...` still passes on a machine without Docker — at the cost of
silently exercising two backends instead of three. CI runs without `-short` and has Docker.

### 2. Byte-identical copies — store-level parity

Store-level suites cannot use the table above: each backend's tests live in its own
`package <backend>_test`, and there is no exported test-helper package. Those scenarios are instead
**duplicated verbatim** into each backend package, and keeping the assertions byte-identical *is*
the parity guarantee:

| Scenario body | Copies |
|---|---|
| `runBranchScenario` (overlay, tombstones, `PurgeBranch`) | `internal/repository/{memory,sqlite,postgres}/branch_scenario_test.go` |
| `runBranchVersioningScenario` (per-`(stream, branch)` optimistic locking, BR-005) | `internal/repository/eventstore_test.go` (memory), `internal/repository/{sqlite,postgres}/eventstore_test.go` |

When editing one copy, edit all of them.

---

## E2E Tests

**Purpose**: Verify complete user workflows through the UI.

**Tool**: Playwright (chromium), config at `web/playwright.config.ts`, specs in `web/e2e/`.

**How it runs**: `make test-e2e` builds the single binary (`make binary` embeds the built SPA into
the Go server), then Playwright boots that binary and drives a real browser against it. The UI and
the API are same-origin behind one port, so there is no proxy and **no API mocking** — the real
stack answers every request. Seeding happens over the HTTP API in `web/e2e/global-setup.ts`.

E2E is **opt-in**: it is not part of `make test`. CI runs it in a dedicated `e2e` job.

The binary wires in-memory stores unconditionally (`cmd/myfamily/main.go`), so every boot is a clean
slate. The suite runs serially with no retries — it mutates real server state, and a merge is
terminal, so a retry would report the retry's symptom rather than the original failure.

**Automated paths**:

| Path | Spec |
|------|------|
| Branch switcher: list branches, scope to a branch, return to the mainline | `web/e2e/branch-switcher.spec.ts` |
| Merge review: view the diff, resolve a conflict, merge, verify the mainline | `web/e2e/merge-review.spec.ts` |

**Not yet automated** (candidates, no specs today):

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
| ES-001 | `internal/command/*_test.go` - verify events emitted | Automated |
| ES-002 | `internal/repository/eventstore_test.go` - no Update method | Automated |
| ES-003 | Schema inspection | Code review |
| ES-004 | `internal/repository/projection_test.go` | Automated |
| ES-005 | Compile-time check | Automated |
| ES-006 | `internal/domain/events_test.go` - factory tests | Automated |
| ES-007 | `internal/repository/eventstore_test.go` - decode all types | Automated |
| DB-001 | `internal/integration/` + the per-backend `branch_scenario_test.go` trio | Automated |
| DB-002 | `internal/repository/eventstore_test.go` - concurrency | Automated |
| DB-003 | `internal/repository/*_test.go` - nil for missing | Automated |
| DB-004 | Feature parity checklist | Code review |
| DB-005 | `internal/repository/soundex_test.go` + per-backend `readmodel_test.go` | Automated |
| DB-006 | `internal/integration/harness_test.go` - both stores on one database per backend | Automated |
| DB-007 | `internal/repository/{sqlite,postgres}/` - legacy-migration and construction-order tests | Automated |
| PR-001 | `internal/command/*_test.go` - transaction test | Automated |
| PR-002 | `internal/repository/projection_test.go` - version check | Automated |
| PR-003 | `internal/repository/projection_test.go` - deletion | Automated |
| PR-004 | Projection coverage check | Code review |
| BR-001 | `internal/command/branch_commands_test.go`, per-backend `branch_scenario_test.go` | Automated |
| BR-002 | Single `Append` path taking `repository.AppendScope` | Code review |
| BR-003 | Overlay/tombstone/purge-on-delete: per-backend `branch_scenario_test.go`. Purge-on-merge: `TestProjector_BranchMergedPurgesOverlay` (memory only — **gap** on SQL backends). Isolation: `internal/integration/branch_lifecycle_test.go` | Partial |
| BR-004 | `internal/command/branch_merge_commands_test.go`, `internal/integration/branch_lifecycle_test.go`, `internal/integration/branch_conflict_test.go` | Automated |
| BR-005 | `runBranchVersioningScenario` (three copies), `internal/integration/branch_lifecycle_test.go` | Automated |
| BR-006 | `internal/command/branch_commands_test.go` - rejects non-branch-aware events | Automated |
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
| DI-003 | `internal/gedcom/integration_test.go` | Automated |
| DI-004 | Event sourcing (ES-001, ES-002) | Automated |
| API-001 | `internal/api/handlers_test.go` - error format | Automated |
| API-002 | `internal/api/handlers_test.go` - pagination | Automated |
| API-003 | `internal/api/handlers_test.go`, `internal/api/contract_test.go` - status codes | Automated |
| API-004 | OpenAPI spec review | Code review |
| API-005 | oapi-codegen generation | CI |
| QA-001 | `internal/query/quality_service_test.go` - bounds | Automated |
| QA-002 | `internal/query/quality_service_test.go` - issues | Automated |
| QA-003 | `internal/query/quality_service_test.go` - orphans | Automated |
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
| `gedcom-5.5/minimal.ged` | Smallest valid GEDCOM for basic import tests |
| `gedcom-5.5/comprehensive.ged` | Broad feature coverage: multiple families, adoption, sources |
| `gedcom-5.5/555SAMPLE.GED` | Official GEDCOM 5.5.5 specification sample |
| `gedcom-5.5/ancestry-sample.ged` | Ancestry.com export with `_APID` vendor tags |
| `gedcom-5.5/familysearch-sample.ged` | FamilySearch export with `_FSFTID` vendor tags |
| `gedcom-5.5/royal92.ged` | Large public-domain royal genealogy for realistic-volume tests |
| `map-test.ged` | Place/coordinate data for geographic map tests |

GEDCOM 7.0 fixtures are tracked in [#622](https://github.com/cacack/my-family/issues/622).

---

## Coverage Requirements

`.testcoverage.yml` is the enforced source of truth; `make check-coverage` applies it. The defaults
are 85% per package and 75% total, with these overrides:

| Package | Minimum | Rationale |
|---------|---------|-----------|
| (default) | 85% | Per-package floor |
| (total) | 75% | Whole-project floor |
| `internal/api` | 60% | Many StrictServer error paths untested (see #164) |
| `internal/gedcom` | 75% | Import/export needs broad GEDCOM fixtures |
| `internal/command` | 80% | Close to the floor; error paths outstanding |
| `internal/exporter` | 84% | Uncovered code is store-failure error paths |
| `internal/repository` | 65% | Mostly interface definitions |
| `internal/repository/memory` | 65% | Test/demo implementation |
| `internal/demo` | 60% | Sample-data seeding |

Excluded from the gate: `cmd/myfamily`, `internal/web`, `internal/repository/postgres`,
`internal/repository/sqlite`, `internal/integration` (test-only package, no statements), and
generated code.

Frontend coverage is measurable via `npm run test:coverage` but is **not** gated in CI.

---

## CI Pipeline Integration

`.github/workflows/ci.yml` runs three test jobs. There is no `integration` build tag and no
`DATABASE_URL` in tests — PostgreSQL coverage comes from testcontainers, which each Postgres test
starts for itself and skips when Docker is unavailable.

```bash
# backend job
go test -race -coverprofile=coverage.out $COVERAGE_PKGS   # no -short, so Postgres runs
make check-coverage                                        # per-package + total thresholds

# frontend job
npm ci --ignore-scripts && npm run check && npm test -- --run

# e2e job
make binary
npx playwright install --with-deps chromium
npx playwright test
```

Every job runs under a harden-runner egress allowlist; a new external host needs an explicit entry
or the job fails with blocked egress.

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
