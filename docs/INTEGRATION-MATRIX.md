# Feature Integration Matrix

Quick reference for ensuring new features integrate properly across the my-family architecture.

---

## Quick Reference

**For any new feature, answer these questions:**

1. **Does it change state?** - Needs events, commands, projections
2. **Does it store data?** - Needs PostgreSQL + SQLite implementations
3. **Does it have a UI?** - Needs frontend component
4. **Is it a GEDCOM concept?** - Needs import/export support
5. **Is it searchable?** - Needs search integration
6. **Does it affect quality?** - Needs QualityService updates
7. **Is it user-facing?** - Needs 85% test coverage

---

## Feature Categories

| Category | Examples | Complexity | Integration Scope |
|----------|----------|------------|-------------------|
| **Core Entity** | Person, Family, Source | High | Full stack (all layers) |
| **Supporting Entity** | Citation, Media, Repository | High | Full stack (all layers) |
| **Life Data** | LifeEvent, Attribute | Medium | Event layer up + Person model |
| **Research Tool** | Snapshot, Tag, Branch | Medium | Domain + Events + History |
| **Visualization** | PedigreeChart, Timeline | Low | Frontend + Query service |
| **Analytics** | QualityScore, Statistics | Low | Query + Frontend |
| **Import/Export** | GEDCOM, CSV, JSON | Medium | All entity types |
| **Browse/Search** | Surname index, Place browser | Low | Query + Frontend |

---

## Integration Requirements by Category

### Core/Supporting Entity Checklist (20 items)

New entity types (Person, Family, Source, Citation, Media, Repository) require integration at ALL layers.

#### Domain Layer (4 items)

| # | Requirement | Why | Verify |
|---|-------------|-----|--------|
| 1 | Struct with `ID` (UUID) field | Unique identification | `NewX()` sets UUID |
| 2 | `Version` field for optimistic locking | Concurrent write safety ([ADR-001](./adr/001-event-sourcing-cqrs.md)) | Schema inspection |
| 3 | `GedcomXref` field (if GEDCOM-representable) | Lossless round-trip ([ETHOS](./ETHOS.md): Respect the Data) | Field check |
| 4 | `Validate()` method returning `ValidationError` | Consistent validation | Unit test |

#### Event Layer (4 items)

| # | Requirement | Why | Verify |
|---|-------------|-----|--------|
| 5 | `XCreated`, `XUpdated`, `XDeleted` event types | Event sourcing ([ADR-001](./adr/001-event-sourcing-cqrs.md)) | Event exists |
| 6 | `NewXCreated()` factory using `NewBaseEvent()` | Consistent timestamps | Factory test |
| 7 | Events implement `Event` interface | Type safety | Compile check |
| 8 | Case in `DecodeEvent()` switch | Event deserialization | Integration test |

#### Command Layer (2 items)

| # | Requirement | Why | Verify |
|---|-------------|-----|--------|
| 9 | `CreateX`, `UpdateX`, `DeleteX` handlers | CQRS write side | Handler tests |
| 10 | Use `execute()` helper for persistence | Consistent transaction handling | Code review |

#### Projection Layer (2 items)

| # | Requirement | Why | Verify |
|---|-------------|-----|--------|
| 11 | `projectXCreated/Updated/Deleted` functions | Read model sync ([ADR-003](./adr/003-synchronous-projections.md)) | Projection tests |
| 12 | Case in `Projector.Project()` switch | Event routing | Integration test |

#### Read Model Layer (4 items)

| # | Requirement | Why | Verify |
|---|-------------|-----|--------|
| 13 | `XReadModel` struct with denormalized data | Query optimization | Schema review |
| 14 | Interface: `GetX`, `ListX`, `SaveX`, `DeleteX` | Consistent API | Interface check |
| 15 | PostgreSQL implementation | Primary database ([ADR-002](./adr/002-dual-database-strategy.md)) | Shared test suite |
| 16 | SQLite implementation | Fallback database ([ADR-002](./adr/002-dual-database-strategy.md)) | Shared test suite |

#### API Layer (2 items)

| # | Requirement | Why | Verify |
|---|-------------|-----|--------|
| 17 | OpenAPI spec endpoints | API-first architecture | Spec review |
| 18 | Handler implementation with type conversion | Contract compliance | Handler tests |

#### GEDCOM Integration (2 items)

| # | Requirement | Why | Verify |
|---|-------------|-----|--------|
| 19 | Import parsing (if GEDCOM concept) | No vendor lock-in ([ETHOS](./ETHOS.md): Respect the Data) | Round-trip test |
| 20 | Export generation (if GEDCOM concept) | Data portability | Round-trip test |

---

### Life Data Checklist (LifeEvent, Attribute)

Life data entities are attached to persons, not standalone.

| Layer | Requirement | Why | Verify |
|-------|-------------|-----|--------|
| **Domain** | Struct with `PersonID` reference | Ownership linkage | Schema review |
| **Events** | Events include `PersonID` | Stream grouping | Event structure |
| **Projections** | Update both life data AND person read model | Denormalization | Integration test |
| **GEDCOM** | Parse from person record | GEDCOM structure | Import test |
| **Rest** | Same as Core Entity items 5-18 | Full integration | Checklist |

---

### Research Tool Checklist (Snapshot, Tag, Branch)

Version control features leveraging the event stream.

| Layer | Requirement | Why | Verify |
|-------|-------------|-----|--------|
| **Domain** | Struct representing research milestone | Git-inspired workflow ([ETHOS](./ETHOS.md): Differentiator #2) | Domain model |
| **Events** | Events capture state reference | Full audit trail | Event content |
| **History** | Queryable via HistoryService | Time travel capability | Query test |
| **Note** | Minimal read model, no GEDCOM mapping | N/A for export | - |

---

### Visualization Checklist (Charts, Maps, Timelines)

Frontend-heavy features with backend query support.

| Layer | Requirement | Why | Verify |
|-------|-------------|-----|--------|
| **Query** | Service providing structured data | Data shaping for visualization | Query tests |
| **API** | Endpoint returning visualization data | Frontend consumption | API test |
| **Frontend** | Svelte component (D3/canvas if complex) | User experience | Visual test |
| **Accessibility** | Keyboard nav, screen reader support | a11y ([ETHOS](./ETHOS.md): Success Factor) | a11y audit |

---

### Import/Export Checklist

Cross-cutting concern touching all entity types.

| Layer | Requirement | Why | Verify |
|-------|-------------|-----|--------|
| **All Entities** | Each entity type handled | Completeness | Entity inventory |
| **Round-trip** | Import -> Export produces equivalent data | No data loss ([ETHOS](./ETHOS.md): Respect the Data) | Diff test |
| **Xref Preservation** | GedcomXref fields maintained | GEDCOM compliance | Field check |
| **Error Handling** | Graceful handling of unknown tags | Forward compatibility | Error test |

---

## Entity Status Matrix

Current implementation status for tracking completeness.

| Entity | Domain | Events | Commands | Projections | ReadModel | API | GEDCOM | Branch | Status |
|--------|--------|--------|----------|-------------|-----------|-----|--------|--------|--------|
| Person | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️ | Complete |
| PersonName | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️ | Complete |
| Family | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️ | Complete |
| Source | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | Complete |
| Citation | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | Complete |
| Media | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | Complete |
| Note | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | Complete |
| Submitter | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | Complete |
| Association | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | Complete |
| LDSOrdinance | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | Complete |
| LifeEvent | ✅ | ✅ | ⚠️ | ✅ | ✅ | ⚠️ | ✅ | ❌ | Partial |
| Attribute | ✅ | ✅ | ⚠️ | ✅ | ✅ | ⚠️ | ✅ | ❌ | Partial |
| Repository | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | Complete |
| Snapshot | ✅ | ⚠️ | ⚠️ | ⚠️ | ✅ | ✅ | N/A | ❌ | Partial |
| Branch | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | N/A | Complete |

Legend: ✅ Complete | ⚠️ Partial/Needed | ❌ Missing | N/A Not applicable

The **Branch** column means "this entity can be written on a research branch"
([ADR-005](./adr/005-research-branch-data-model.md); `?branch=` on the API). ❌ means main-only —
the API does not expose `?branch=` on those operations, and an event-sourced write attempted on a
branch scope is rejected with `ErrEventTypeNotBranchAware` (BR-006). Widening this is
[#676](https://github.com/cacack/my-family/issues/676).

Notes on partial rows:

- **LifeEvent / Attribute**: no dedicated CRUD commands or API endpoints; only bulk export (`/export/events`, `/export/attributes`).
- **Snapshot**: bypasses the event-sourced pipeline — `SnapshotService` writes directly to `SnapshotStore` (implemented in all three backends, hence ReadModel ✅). A `SnapshotCreated` event type exists in `domain/events.go` but is never emitted, so Events/Commands/Projections remain partial.
- **Branch**: create, delete/archive (#670) and merge ([#55](https://github.com/cacack/my-family/issues/55), delivered) are implemented, with list/get/compare queries and a `/branches` API. `BranchMerged` is emitted by `Handler.claimMerge` and projected to the registry. The frontend surface (switcher, banner, `/branches` list and read-only comparison view) ships with [#94](https://github.com/cacack/my-family/issues/94); the merge *review* UI is [#95](https://github.com/cacack/my-family/issues/95), so `POST /branches/{id}/merge` is API-only for now — implemented and callable, just not yet driven from the UI. GEDCOM and the Branch column are N/A: a branch is not a genealogy record and cannot itself live on a branch.

### Branch coverage detail (#669 read / #670 write)

Seven read-model types are branch-aware (copy-on-write overlay, #669). Branch **writes** cover a
narrower set, because a write also needs a branch-scoped command path:

| Read-model type | Branch reads (#669) | Branch writes (#670) | How it is written on a branch |
|---|---|---|---|
| Person | ✅ | ✅ | `createPerson` / `updatePerson` / `deletePerson` |
| PersonName | ✅ | ✅ | `addPersonName` / `updatePersonName` / `deletePersonName` |
| Family | ✅ | ✅ | `createFamily` / `updateFamily` / `deleteFamily` |
| FamilyChild | ✅ | ✅ | `addChildToFamily` / `removeChildFromFamily` |
| PedigreeEdge | ✅ | ✅ | derived — reprojected from branch-scoped child link/unlink |
| PersonExternalID | ✅ | ❌ | written only by GEDCOM import, which is main-only by design (#670 non-goal) |
| FamilyExternalID | ✅ | ❌ | same as PersonExternalID |

Those 11 write operations plus 5 reads (`listPersons`, `getPerson`, `getFamily`, `getPersonNames`,
`getPedigree`) are the 16 API operations carrying `?branch=`.

The frontend mirrors exactly those 16 in `isBranchScopedRequest()`
(`web/src/lib/api/client.ts`), matching on method as well as path — `POST /families` takes
`?branch=` while `listFamilies` does not. Every other surface renders `MainlineNotice.svelte` while
a branch is active, so the UI never presents mainline data as branch data. Grow both with #676.

**Isolation is complete for these types.** Branch writes never touch `main` (proven end to end in
`internal/api/branch_handlers_test.go`), and the command layer resolves its *reads* — existence
checks, validation, and the expected version — through the same branch overlay, so a branch
behaves like a normal working copy:

- A person, name, or family may be created on a branch and then edited, renamed, or deleted on that
  same branch; the mainline never sees any of it.
- Repeated edits to the same record on one branch work: the expected version comes from the
  branch's own row, which matches the per-`(stream_id, branch_id)` event version (BR-005).
- Records the branch has not touched still resolve to `main`, so corrections made on `main` after
  the branch was created show through (the deliberate "live overlay" of ADR-005).

Remaining gaps, both deliberate: GEDCOM import/export is main-only (a stated non-goal of #670), and
rollback is main-only (`Handler.rollbackEntity`). Widening branch writes to the entity types outside
the seven-type slice is [#676](https://github.com/cacack/my-family/issues/676); merging a branch back
into `main` is [#55](https://github.com/cacack/my-family/issues/55).

---

## See Also

- [ARCHITECTURAL-INVARIANTS.md](./ARCHITECTURAL-INVARIANTS.md) - Rules that must always hold
- [TESTING-STRATEGY.md](./TESTING-STRATEGY.md) - How to verify integrations
- [ADR-001: Event Sourcing](./adr/001-event-sourcing-cqrs.md) - Why events are required
- [ADR-002: Dual Database](./adr/002-dual-database-strategy.md) - Why both implementations
- [ADR-003: Synchronous Projections](./adr/003-synchronous-projections.md) - Why projections in transaction
- [ADR-004: Single Binary](./adr/004-single-binary-deployment.md) - Deployment architecture
- [ETHOS.md](./ETHOS.md) - Guiding principles

---

## Related

- [CONVENTIONS.md](./CONVENTIONS.md) - Code patterns and standards
- [../CONTRIBUTING.md](../CONTRIBUTING.md) - Development workflow
