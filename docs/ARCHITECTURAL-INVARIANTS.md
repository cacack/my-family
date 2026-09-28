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
| **DB-002** | `EventStore.Append` fails on version mismatch (optimistic locking), scoped per `(stream_id, branch_id)` — see BR-005 | Concurrency test |
| **DB-003** | `ReadModelStore` returns `nil` (not error) for missing entities | Interface contract test |
| **DB-004** | No PostgreSQL-specific features without SQLite fallback or graceful degradation | Feature parity checklist |
| **DB-005** | Full-text search works on both databases (tsvector vs FTS5) | Search integration test |
| **DB-006** | Event store and read model table names are disjoint — both stores may share one database | Integration harness builds both stores on a single database per backend |
| **DB-007** | On a database predating the `life_events` rename, the read model store must be constructed before the event store; the event store refuses (`ErrReadModelEventsTable`) rather than mutating a read-model table | Per-backend legacy-migration tests in `internal/repository/{sqlite,postgres}/` |
| **DB-008** | `serve` runs on the backend its config selects — `DEMO_MODE` → memory, `DATABASE_URL` → PostgreSQL, otherwise SQLite at `SQLITE_PATH` — and refuses to start if that backend cannot be opened; it never silently falls back to memory | `internal/storage/storage_test.go` (selection per config, restart-persistence on SQLite and PostgreSQL, cgo-less and unreachable-database refusals) |

### Projection Invariants (PR) - Source: [ADR-003](./adr/003-synchronous-projections.md)

| ID | Rule | Verification |
|----|------|--------------|
| **PR-001** | Projections update in same transaction as event append | Code review; transaction test |
| **PR-002** | Read model version matches event stream version | Version consistency test |
| **PR-003** | Deleted entities removed from read model | Deletion projection test |
| **PR-004** | New event types have corresponding projection handlers | Projection coverage check |
| **PR-005** | A live (synchronous) projection sees the same event a replay decodes: `Handler.execute` canonicalizes every `*Updated` changes map to its JSON shape (`repository.CanonicalizeChanges`) before append and projection, so projections read change values only in their decoded form (strings, `float64`, `[]any`, `map[string]any`, nil to clear) (#848) | `TestUpdatedCommands_LiveProjectionMatchesReplay` in `internal/integration/` (every `*Updated` command, all backends) |

### Deployment Invariants (DP) - Source: [ADR-004](./adr/004-single-binary-deployment.md)

| ID | Rule | Verification |
|----|------|--------------|
| **DP-001** | Single binary contains embedded frontend | Build verification |
| **DP-002** | Development mode supports frontend hot reload | Manual verification |
| **DP-003** | API and frontend served from same origin (no CORS needed) | Configuration check |

### Branch Invariants (BR) - Source: [ADR-005](./adr/005-research-branch-data-model.md)

| ID | Rule | Verification |
|----|------|--------------|
| **BR-001** | Every branch event carries a `branch_id`; `main` is the reserved branch id (`uuid.Nil` / `domain.MainBranchID`) | `TestHandler_Unscoped_WritesMain` (`internal/command`): an unscoped handler tags every stored event `MainBranchID`; `branch_scenario_test.go` in all three backends asserts branch events carry the branch id |
| **BR-002** | Branch events append to the shared global log, never a separate store (upholds ES-002) | Code review: one `EventStore.Append` path taking a `repository.AppendScope`; `ReadBranch` filters the shared log by `branch_id` — no per-branch store type exists |
| **BR-003** | Read-model rows carry `branch_id`; queries default to `main`, branch rows shadow `main` (copy-on-write overlay), deletes write tombstone rows. A branch's overlay is purged when the branch reaches a terminal status — `merged` or `archived` — so no terminal branch retains an isolated view | `internal/repository/{memory,sqlite,postgres}/branch_scenario_test.go` (overlay, tombstone, `PurgeBranch` on `BranchDeleted`); `TestProjector_BranchMergedPurgesOverlay` (`internal/repository`) for the merge purge; `TestBranchIsolation*` in `internal/command` and `internal/api`; `TestBranchLifecycle_EndToEnd` (`internal/integration`) asserts branch isolation on every backend, and that a merged branch is no longer readable — note it does **not** prove the purge itself, since the API refuses a terminal branch by status before reaching the read model; cross-backend purge-on-merge coverage is still a gap |
| **BR-004** | A merge re-appends only a branch's entity/domain mutation events onto `main` (excluding branch-lifecycle events and the `BranchMerged` marker) and records a single `BranchMerged` event; history is never rewritten | `TestMergeBranch_AppendOnly` (`internal/command`): the branch's own stored events are byte-identical after the merge and `main` gains only new events at new positions. `TestBranchService_PlanMerge_ReplaySetExcludesLifecycleEvents` (`internal/query`) pins the replay set to mutation events only; `TestMergeBranch_PreservesProvenance` (`internal/command`) pins the replayed payload and `OccurredAt` to the originals; `TestMergeBranch_SecondMergeIsRefused` and `TestMergeBranch_ConcurrentClaimLoses` pin the single `BranchMerged`. `TestBranchLifecycle_EndToEnd` and `TestBranchConflict_*` (`internal/integration`) re-verify the replay and both conflict-resolution directions against memory, SQLite and PostgreSQL |
| **BR-005** | Optimistic versioning is per-`(stream_id, branch_id)`. A branch's first write to an aggregate that exists on `main` seeds its version from the version the branch's read shows (#844) — that aggregate's **current** `main` version through the live overlay, or the version of the branch's own cross-stream shadow row (e.g. a source whose citation count a branch citation changed), reported by the command handler as `AppendScope.OverlayVersion` — then increments within the branch; concurrent branches never contend at write time | `runBranchVersioningScenario` — identical copies in `internal/repository/eventstore_test.go` (memory), `sqlite/eventstore_test.go`, `postgres/eventstore_test.go` (DB-001 parity); `TestBranchEditAfterMainCorrection` (`internal/repository/{memory,sqlite,postgres}`) for an aggregate `main` edited after the fork, including one the branch shadows cross-stream; `TestBranchOverlayStreams_*`/`TestBranchOverlayVersion` (`internal/command`) for the overlay-version resolvers; exercised end-to-end by `TestBranchLifecycle_EndToEnd` (`internal/integration`), whose branch edits seed from `main` versions on every backend |
| **BR-006** | A branch-scoped write is legal only for event types whose projection handler writes exclusively branch-keyed rows; any other event type is rejected before the append (`command.ErrEventTypeNotBranchAware`) | `TestExecute_RejectsNonBranchAwareEvent` (`internal/command`); the allowed set in `internal/command/handler.go` is derived from `internal/repository/projection.go`, and `TestBranchAwareEventTypes_LeaveMainUntouched` projects one probe per allowlisted type on a branch and asserts main is unchanged |

> **Implementation status (#669):** BR-003 and the branch-lifecycle side of PR-004 are
> realized for the first read-model slice — Person, PersonName, PersonExternalID, Family,
> FamilyExternalID, FamilyChild, PedigreeEdge — with copy-on-write overlay, tombstones, and
> `PurgeBranch` on `BranchDeleted` across the memory, sqlite, and postgres backends. Identical
> end-to-end scenario tests (`internal/repository/{memory,sqlite,postgres}/branch_scenario_test.go`)
> verify DB-001 parity.
>
> **Implementation status (#676 sub-issue A, [#756](https://github.com/cacack/my-family/issues/756)):**
> the browse and map aggregates — surname index, per-surname person list, place hierarchy,
> per-place person list, per-cemetery person list and map locations — are **delivered** as
> branch-aware reads. They own no `branch_id` of their own: each is computed over the BR-003
> overlay of the seven-type slice, so branch rows shadow `main` and tombstones drop out of the
> aggregate for free. Verified by `TestBranchScenario_AggregateIsolation`
> (`internal/repository/{memory,sqlite,postgres}/branch_scenario_test.go`, one identical copy per
> backend for DB-001 parity),
> `TestBrowseService_BranchScopeReachesStore` and `TestBrowseService_MainOnlyPathsUnscoped`
> (`internal/query/browse_service_test.go`), and the `?branch=` handler tests in
> `internal/api/browse_handlers_test.go`. The frontend allowlist
> (`web/src/lib/api/client.ts`) is pinned to the spec by a drift test in
> `web/src/lib/api/client.test.ts`. Still main-only, deliberately: brick walls, which are not
> event-sourced and so cannot be branch-scoped until they get the event-sourcing decision
> [#624](https://github.com/cacack/my-family/issues/624) made for snapshots
> ([#761](https://github.com/cacack/my-family/issues/761)).
>
> **Implementation status (#676 sub-issue B, [#757](https://github.com/cacack/my-family/issues/757)):**
> BR-003 now also covers the person/family facts — LifeEvent, Attribute and Association — on all
> three backends, with the manual `DeletePerson`/`DeleteFamily` cascade extended to them and the
> cemetery index (which reads `life_events`) joining the branch-aware aggregates. Their nine event
> types are on the BR-006 allowlist. Verified by `TestBranchScenario_FactOverlay` and
> `TestReadModelStore_BranchDeleteCascadesFacts` (identical copies per backend), the extended
> `TestReadModelStore_Delete*Cascade` tests, `TestBranchAssociationLifecycle` and
> `TestBranchAwareEventTypes_LeaveMainUntouched` (`internal/command`), and the `?branch=` handler
> tests in `internal/api/fact_branch_handlers_test.go`.
>
> **Implementation status (#676 sub-issue C, [#758](https://github.com/cacack/my-family/issues/758)):**
> BR-003 now also covers the evidence — Source, SourceExternalID, Citation and Note — on all three
> backends. `DeleteSource` cascades to the source's external identifiers and citations on the same
> branch only (replacing the dropped `sources(id)` foreign keys), a citation's denormalized source
> title and its source's citation count resolve through the same branch, and `SearchSources`
> resolves the overlay before it matches. Their nine event types are on the BR-006 allowlist.
> Verified by `TestBranchScenario_EvidenceOverlay` and `TestReadModelStore_DeleteSourceCascade`
> (identical copies per backend), `TestReadModelStore_MigratesEvidenceTablesToBranchKeys`
> (PostgreSQL) and `TestPreEvidence*` (SQLite), `TestBranchEvidenceLifecycle` and
> `TestBranchAwareEventTypes_LeaveMainUntouched` (`internal/command`), and the `?branch=` handler
> tests in `internal/api/evidence_branch_handlers_test.go`.
>
> **Implementation status (#676 sub-issue D, [#759](https://github.com/cacack/my-family/issues/759)):**
> BR-003 now also covers media **metadata** on all three backends. The file bytes are shared, never
> copied per branch: they live on the item's origin row, a branch shadow row stores NULL bytes, and
> `GetMediaWithData` / `GetMediaThumbnail` read them from the winning row else main's; a mainline
> delete keeps main's row as a tombstone while a branch still shows the item through a live shadow
> (ADR-005, "Implementation Note — media metadata"). `DeletePerson`, `DeleteFamily` and
> `DeleteSource` cascade to the owner's media on the same branch only, and the three `Media*`
> event types are on the BR-006 allowlist. Verified by `TestBranchScenario_MediaOverlay` and
> `TestReadModelStore_DeleteCascadesMedia` (identical copies per backend),
> `TestReadModelStore_MigratesMediaToBranchKeys` (PostgreSQL) and
> `TestPreMediaBranchSchemaRefusesBranchWrites` (SQLite), `TestBranchMediaLifecycle` and
> `TestBranchAwareEventTypes_LeaveMainUntouched` (`internal/command`), and the `?branch=` handler
> tests in `internal/api/media_branch_handlers_test.go`.
>
> **Implementation status (#676 sub-issue E, [#760](https://github.com/cacack/my-family/issues/760)):**
> BR-003 now also covers the GPS artifacts — EvidenceAnalysis, EvidenceConflict, ResearchLog and
> ProofSummary — on all three backends. Every filtered read (per fact, per subject and
> `ListUnresolvedConflicts`) resolves the overlay before it applies its predicate, so a branch that
> resolves an evidence conflict no longer lists it as unresolved while main still does.
> `DeletePerson` and `DeleteFamily` cascade to the GPS artifacts about the deleted subject on the
> same branch only. The eleven GPS event types are on the BR-006 allowlist; a merge refuses a
> replayed artifact whose subject will not exist on main (`ErrMergeDanglingReference`), and the
> merge conflict scan compares `EvidenceConflictResolved`. (An *evidence* conflict is a genealogical
> finding and has nothing to do with BR-004's *merge* conflicts.) Verified by
> `TestBranchScenario_GPSOverlay` and `TestReadModelStore_DeleteCascadesGPS` (identical copies per
> backend), `TestReadModelStore_MigratesGPSArtifactsToBranchKeys` (PostgreSQL) and
> `TestPreGPSBranchSchemaRefusesBranchWrites` (SQLite), `TestBranchGPS_*`,
> `TestMergeBranch_GPS*` and `TestBranchAwareEventTypes_LeaveMainUntouched` (`internal/command`),
> and the `?branch=` handler tests in `internal/api/gps_branch_handlers_test.go`. With it every
> #676 sub-issue is delivered.
>
> **BR-003's scope is bounded by decision, not only by progress.** Extending branch-scoping to the
> pending entity types was #676 (complete with #760), but four entities — Submitter, Repository,
> RepositoryExternalID and LDSOrdinance — will never carry a `branch_id`: they are file-/archive-level
> metadata and transcribed sacramental records, not claims a research hypothesis forks. Recorded in
> [ADR-005, "Entities that stay main-only"](./adr/005-research-branch-data-model.md#entities-that-stay-main-only)
> ([#761](https://github.com/cacack/my-family/issues/761)).
>
> **Implementation status (#670):** BR-005 and BR-006 arrived with the branch lifecycle
> (create / isolate / compare / archive) and the `?branch=` HTTP scope. Branch **writes** cover a
> narrower set of entity types than branch **reads** (BR-006 rejects the rest), but for the types
> they do cover the command layer resolves its reads through the branch overlay too, so a branch is
> fully editable rather than write-once — see the branch column of
> [INTEGRATION-MATRIX.md](./INTEGRATION-MATRIX.md#entity-status-matrix). GEDCOM import/export,
> and rollback stay main-only by design. `ReadByStream` is branch-filtered so branch edits never leak
> into an entity's mainline audit trail; person and family history take `?branch=` for the
> branch's own view of the stream (#824, `HistoryService.GetEntityHistoryOn`), and rollback and
> restore points refuse `?branch=` with 409 (`TestRollback_RefusedOnBranch`, `internal/api`).
>
> **Implementation status (#55):** BR-004 is realized and verified. `Handler.MergeBranch`
> (`internal/command/branch_merge_commands.go`) replays a branch's mutation events onto `main`
> from the plan `BranchService.PlanMerge` (`internal/query/merge_conflicts.go`) builds, and
> `POST /branches/{id}/merge` exposes it. The `active → merged` claim is atomic — `BranchMerged`
> is appended to the branch's own stream under BR-005's per-`(stream_id, branch_id)` uniqueness
> before anything is written to `main`, so exactly one of two concurrent merges wins. The claim
> and the replay are **not** one transaction; see the "Implementation Note — merge" section of
> [ADR-005](./adr/005-research-branch-data-model.md) for the failure mode and its bound.
> The plan is pinned to the `main` stream versions it was computed against
> (`MergePlan.MainStreamVersions`), captured **before** `PlanMerge` reads `main`'s side of the
> diff, checked before the claim and re-asserted per stream during the replay, so a mainline write
> landing after the conflict verdict is refused (`command.ErrMergePlanStale`,
> `409 merge_plan_stale`) rather than silently overridden (#698). Capturing the pin after that read
> instead would make it newer than the verdict and re-open the hole silently.
> `TestMergeBranch_StalePlanRefusesWriteInsideThePlanningWindow` (the ordering),
> `TestMergeBranch_StalePlanRefuses` and
> `TestMergeBranch_StalePlanRefusesBranchCreatedStream` (`internal/command`) pin it.
> BR-003's terminal-status purge now also fires on merge.
> Partial merge / cherry-pick remains out of scope, per ADR-005 §Merge.

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
| **DI-005** | A family always has at least one partner: `CreateFamily` and `UpdateFamily` (including `clear_partnerN`) both refuse a partnerless result with `ErrInvalidFamilyInput` (400) | `TestUpdateFamily_CannotClearLastPartner`, `TestCreateFamily_ValidationErrorsWrapSentinel` |

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
| ADR-002 (Dual Database) | DB-001 through DB-008 | 8 |
| ADR-003 (Sync Projections) | PR-001 through PR-005 | 5 |
| ADR-004 (Single Binary) | DP-001 through DP-003 | 3 |
| ADR-005 (Research Branches) | BR-001 through BR-006 | 6 |
| ETHOS.md | DM-001 through DM-006, DI-001 through DI-005, QA-001 through QA-003 | 14 |
| CONVENTIONS.md | API-001 through API-005 | 5 |
| CONTRIBUTING.md | TS-001 through TS-003 | 3 |
| **Total** | | **51** |

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
