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

| Entity | Domain | Events | Commands | Projections | ReadModel | API | GEDCOM | Status |
|--------|--------|--------|----------|-------------|-----------|-----|--------|--------|
| Person | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| PersonName | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Family | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Source | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Citation | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Media | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Repository | ✅ | ✅ | ⚠️ | ⚠️ | ⚠️ | ⚠️ | ⚠️ | Partial |
| LifeEvent | ✅ | ✅ | ⚠️ | ✅ | ✅ | ⚠️ | ✅ | Partial |
| Attribute | ✅ | ✅ | ⚠️ | ✅ | ✅ | ⚠️ | ✅ | Partial |
| Snapshot | ✅ | ✅ | ⚠️ | ⚠️ | ⚠️ | ✅ | N/A | Partial |

Legend: ✅ Complete | ⚠️ Partial/Needed | ❌ Missing

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
