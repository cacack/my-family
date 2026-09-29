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
| 8 | Case in `DecodeEvent()` switch, and a row in `historyEventCatalog` (mapped or excluded, see [HISTORY-EVENT-TYPES.md](./HISTORY-EVENT-TYPES.md)) | Event deserialization; every change-log view renders it | Integration test; `TestHistoryCatalog_CoversEveryEventType` |

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
| Source | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Citation | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Media | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Note | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| Submitter | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | Complete |
| Association | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Complete |
| LDSOrdinance | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | Complete |
| LifeEvent | ✅ | ✅ | ⚠️ | ✅ | ✅ | ⚠️ | ✅ | ⚠️ | Partial |
| Attribute | ✅ | ✅ | ⚠️ | ✅ | ✅ | ⚠️ | ✅ | ⚠️ | Partial |
| Repository | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | Complete |
| EvidenceAnalysis | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | ✅ | Complete |
| EvidenceConflict | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | ✅ | Complete |
| ResearchLog | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | ✅ | Complete |
| ProofSummary | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | ✅ | Complete |
| Snapshot | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | ✅ | Complete |
| Branch | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A | N/A | Complete |

Legend: ✅ Complete | ⚠️ Partial/Needed | ❌ Missing/pending | ⛔ Blocked on a decision | N/A Not applicable

The **Branch** column means "this entity can be written on a research branch"
([ADR-005](./adr/005-research-branch-data-model.md); `?branch=` on the API). Both ❌ and N/A mean
main-only today — the API does not expose `?branch=` on those operations, and an event-sourced write
attempted on a branch scope is rejected with `ErrEventTypeNotBranchAware` (BR-006) — but they mean
it for opposite reasons:

- **❌ = pending.** The entity is destined for branch scoping and simply is not there yet. No row
  is in this state today: Snapshot, the last one, became branch-scoped with
  [#839](https://github.com/cacack/my-family/issues/839) (see its note below).
- **⛔ = blocked on a decision.** Branch scoping is neither scheduled nor ruled out, because a prior
  question has to be answered first. No row is in this state today. Brick walls are — they bypass
  the event-sourced pipeline, and an entity whose state never passes through the event log has no
  branch-tagged events to project or replay — but they are operations rather than an entity, so
  they have no row here. Snapshot left this state when
  [#624](https://github.com/cacack/my-family/issues/624) made it event-sourced, and was then
  branch-scoped by [#839](https://github.com/cacack/my-family/issues/839). See
  [ADR-005, "Entities that stay main-only"](./adr/005-research-branch-data-model.md#entities-that-stay-main-only).
- **N/A = decided.** Branch scoping does not apply to the entity. Submitter, Repository and
  LDSOrdinance — along with RepositoryExternalID, which has no row in this matrix — are permanently
  main-only: file-/archive-level metadata and transcribed sacramental records, not claims a research
  hypothesis forks. See
  [ADR-005, "Entities that stay main-only"](./adr/005-research-branch-data-model.md#entities-that-stay-main-only).
  (Branch itself is N/A for the structural reason that a branch cannot live on a branch.)

The column says nothing about branch *reads*: the browse and map aggregates are branch-aware without
being entities of their own — see
[Branch coverage detail](#branch-coverage-detail-669-read--670-write--756-aggregates--757-facts--758-evidence--759-media--760-gps) below.

Notes on partial rows:

- **EvidenceAnalysis / EvidenceConflict / ResearchLog / ProofSummary** (the GPS artifacts): GEDCOM is N/A — GEDCOM has no record for a research analysis, conflict, log or proof argument, so they are neither imported nor exported. Branch ✅ since [#760](https://github.com/cacack/my-family/issues/760). An *evidence* conflict is a genealogical finding (two analyses disagree about a fact); it is unrelated to a branch *merge* conflict.
- **LifeEvent / Attribute**: no dedicated CRUD commands or API endpoints; only bulk export (`/export/events`, `/export/attributes`). Branch ⚠️: the read model, projections and BR-006 allowlist are branch-scoped ([#757](https://github.com/cacack/my-family/issues/757)) — a branch delete of their owner tombstones them, the cemetery index and the group sheet's negated events read them through the overlay (the group-sheet endpoint takes `?branch=` since [#829](https://github.com/cacack/my-family/issues/829)), and a branch merge can carry their events — but with no command of their own there is no API path that writes one on a branch.
- **Snapshot**: event-sourced since [#624](https://github.com/cacack/my-family/issues/624) — `Handler.CreateSnapshot` / `DeleteSnapshot` emit `SnapshotCreated` / `SnapshotDeleted` and the projection writes the registry, so snapshots created from that point on rebuild from the log. Rows predating #624 have no event and would not survive a rebuild (see ADR-005 "Still open"); rebuild tooling ([#680](https://github.com/cacack/my-family/issues/680)) must backfill them. GEDCOM is N/A (a research marker is not a genealogy record). Branch ✅ since [#839](https://github.com/cacack/my-family/issues/839): the registry carries a `branch_id` on all three backends (existing rows migrate to the mainline), a snapshot taken on a branch marks `(branch_id, position)`, the events carry the branch in their payload (an older event without one decodes as mainline), and list/get/create/delete plus both comparisons take `?branch=` and answer for the active branch's snapshots only. A branch comparison reads the branch's view of the log — its own events plus the mainline events it inherits, labelled by `origin` — and comparing snapshots from different branches is refused (409 `snapshot_branch_mismatch`). `GET /snapshots/{id}/compare-current` compares a snapshot with the current log head, on the mainline or a branch. See ADR-005, *Implementation Note — branch-scoped snapshots and compare to now*.
- **Branch**: create, delete/archive (#670) and merge ([#55](https://github.com/cacack/my-family/issues/55), delivered) are implemented, with list/get/compare queries and a `/branches` API. `BranchMerged` is emitted by `Handler.claimMerge` and projected to the registry. `BranchMergeResumed` ([#685](https://github.com/cacack/my-family/issues/685)) is emitted by `Handler.ResumeMerge` when a resume records its decisions; it is decoded (ES-007) and handled as a projection no-op (PR-004). The frontend surface (switcher, banner, `/branches` list and comparison view) ships with [#94](https://github.com/cacack/my-family/issues/94) and [#95](https://github.com/cacack/my-family/issues/95): `/branches/{id}` is the merge review, so `POST /branches/{id}/merge` is driven from the UI — conflict resolution, per-entity exclusion, and the merge itself. GEDCOM and the Branch column are N/A: a branch is not a genealogy record and cannot itself live on a branch.

### Branch coverage detail (#669 read / #670 write / #756 aggregates / #757 facts / #758 evidence / #759 media / #760 GPS)

Nineteen read-model types carry a `branch_id` of their own and are branch-aware by copy-on-write
overlay: the seven-type #669 slice, the three person/family fact types of #757, the four
evidence types of #758, media metadata (#759; the file bytes are shared, never copied) and the
four GPS artifact types of #760. Branch
**writes** cover a narrower set, because a write also needs a branch-scoped command path:

| Read-model type | Branch reads (#669) | Branch writes (#670) | How it is written on a branch |
|---|---|---|---|
| Person | ✅ | ✅ | `createPerson` / `updatePerson` / `deletePerson` |
| PersonName | ✅ | ✅ | `addPersonName` / `updatePersonName` / `deletePersonName` |
| Family | ✅ | ✅ | `createFamily` / `updateFamily` / `deleteFamily` |
| FamilyChild | ✅ | ✅ | `addChildToFamily` / `removeChildFromFamily` |
| PedigreeEdge | ✅ | ✅ | derived — reprojected from branch-scoped child link/unlink |
| PersonExternalID | ✅ | ❌ | written only by GEDCOM import, which is main-only by design (#670 non-goal) |
| FamilyExternalID | ✅ | ❌ | same as PersonExternalID |
| LifeEvent (#757) | ✅ | ⚠️ | no command of its own; tombstoned by a branch `deletePerson` / `deleteFamily` |
| Attribute (#757) | ✅ | ⚠️ | same as LifeEvent |
| Association (#757) | ✅ | ✅ | `createAssociation` / `updateAssociation` / `deleteAssociation` |
| Source (#758) | ✅ | ✅ | `createSource` / `updateSource` / `deleteSource` (the delete cascades to the source's external IDs and citations on the branch) |
| SourceExternalID (#758) | ✅ | ⚠️ | written only by GEDCOM import (main-only); tombstoned by a branch `deleteSource` |
| Citation (#758) | ✅ | ✅ | `createCitation` / `updateCitation` / `deleteCitation`; the denormalized source title and the source's citation count resolve on the branch. Known gap: bumping the count writes a branch copy of the source, which then hides later main edits to that source on the branch (stale view, tracked in [#815](https://github.com/cacack/my-family/issues/815)) |
| Note (#758) | ✅ | ✅ | `createNote` / `updateNote` / `deleteNote` |
| Media (#759) | ✅ | ✅ | `uploadPersonMedia` / `updateMedia` / `deleteMedia`; metadata only — a branch row stores no copy of the file or thumbnail, which are read from main's row. Also tombstoned by a branch `deletePerson` / `deleteFamily` / `deleteSource` |
| EvidenceAnalysis (#760) | ✅ | ✅ | `createEvidenceAnalysis` / `updateEvidenceAnalysis` / `deleteEvidenceAnalysis`; the automatic evidence-conflict check compares the analyses the branch sees and records its conflict on the branch. Also tombstoned by a branch `deletePerson` / `deleteFamily` of its subject |
| EvidenceConflict (#760) | ✅ | ✅ | recorded by `createEvidenceAnalysis` / `updateEvidenceAnalysis`; `resolveEvidenceConflict`. `ListUnresolvedConflicts` resolves the overlay before it filters on status, so a branch resolution hides main's open row on the branch only |
| ResearchLog (#760) | ✅ | ✅ | `createResearchLog` / `updateResearchLog` / `deleteResearchLog` |
| ProofSummary (#760) | ✅ | ✅ | `createProofSummary` / `updateProofSummary` / `deleteProofSummary` |

Those 11 write operations plus 5 reads (`listPersons`, `getPerson`, `getFamily`, `getPersonNames`,
`getPedigree`) were the original #669/#670 slice. Sub-issue A of #676
([#756](https://github.com/cacack/my-family/issues/756)) added six aggregate reads that derive from
that slice's overlay and own no `branch_id` column of their own:

| operationId | Method | Path |
|---|---|---|
| `browseSurnames` | GET | `/browse/surnames` |
| `getPersonsBySurname` | GET | `/browse/surnames/{surname}/persons` |
| `browsePlaces` | GET | `/browse/places` |
| `getPersonsByPlace` | GET | `/browse/places/{place}/persons` |
| `getPersonsByCemetery` | GET | `/browse/cemeteries/{place}/persons` |
| `getMapLocations` | GET | `/map/locations` |

Sub-issue B ([#757](https://github.com/cacack/my-family/issues/757)) added seven more: the cemetery
index, now that `life_events` carries a `branch_id`, and the six association operations:

| operationId | Method | Path |
|---|---|---|
| `browseCemeteries` | GET | `/browse/cemeteries` |
| `listAssociations` | GET | `/associations` |
| `createAssociation` | POST | `/associations` |
| `getAssociation` | GET | `/associations/{id}` |
| `updateAssociation` | PUT | `/associations/{id}` |
| `deleteAssociation` | DELETE | `/associations/{id}` |
| `listAssociationsForPerson` | GET | `/persons/{id}/associations` |

Sub-issue C ([#758](https://github.com/cacack/my-family/issues/758)) added eighteen for the
evidence. Source and citation history, restore points and rollback stay mainline, as rollback does
for every entity:

| operationId | Method | Path |
|---|---|---|
| `listSources` | GET | `/sources` |
| `createSource` | POST | `/sources` |
| `searchSources` | GET | `/sources/search` |
| `getSource` | GET | `/sources/{id}` |
| `updateSource` | PUT | `/sources/{id}` |
| `deleteSource` | DELETE | `/sources/{id}` |
| `getCitationsForSource` | GET | `/sources/{id}/citations` |
| `createCitation` | POST | `/citations` |
| `getCitation` | GET | `/citations/{id}` |
| `updateCitation` | PUT | `/citations/{id}` |
| `deleteCitation` | DELETE | `/citations/{id}` |
| `formatCitation` | GET | `/citations/{id}/format` |
| `getCitationsForPerson` | GET | `/persons/{id}/citations` |
| `listNotes` | GET | `/notes` |
| `createNote` | POST | `/notes` |
| `getNote` | GET | `/notes/{id}` |
| `updateNote` | PUT | `/notes/{id}` |
| `deleteNote` | DELETE | `/notes/{id}` |

Sub-issue D ([#759](https://github.com/cacack/my-family/issues/759)) added seven for media. The
content and thumbnail reads take the scope although the bytes are shared, so a branch-deleted item
is not-found there too:

| operationId | Method | Path |
|---|---|---|
| `listPersonMedia` | GET | `/persons/{id}/media` |
| `uploadPersonMedia` | POST | `/persons/{id}/media` |
| `getMedia` | GET | `/media/{id}` |
| `updateMedia` | PUT | `/media/{id}` |
| `deleteMedia` | DELETE | `/media/{id}` |
| `downloadMedia` | GET | `/media/{id}/content` |
| `getMediaThumbnail` | GET | `/media/{id}/thumbnail` |

Sub-issue E ([#760](https://github.com/cacack/my-family/issues/760)) added twenty-two for the GPS
artifacts:

| operationId | Method | Path |
|---|---|---|
| `listEvidenceAnalyses` | GET | `/evidence-analyses` |
| `createEvidenceAnalysis` | POST | `/evidence-analyses` |
| `getEvidenceAnalysis` | GET | `/evidence-analyses/{id}` |
| `updateEvidenceAnalysis` | PUT | `/evidence-analyses/{id}` |
| `deleteEvidenceAnalysis` | DELETE | `/evidence-analyses/{id}` |
| `getAnalysesByFact` | GET | `/evidence-analyses/by-fact` |
| `listEvidenceConflicts` | GET | `/evidence-conflicts` |
| `getEvidenceConflict` | GET | `/evidence-conflicts/{id}` |
| `resolveEvidenceConflict` | POST | `/evidence-conflicts/{id}/resolve` |
| `getConflictsBySubject` | GET | `/evidence-conflicts/by-subject/{subjectId}` |
| `listResearchLogs` | GET | `/research-logs` |
| `createResearchLog` | POST | `/research-logs` |
| `getResearchLog` | GET | `/research-logs/{id}` |
| `updateResearchLog` | PUT | `/research-logs/{id}` |
| `deleteResearchLog` | DELETE | `/research-logs/{id}` |
| `getResearchLogsBySubject` | GET | `/research-logs/by-subject/{subjectId}` |
| `listProofSummaries` | GET | `/proof-summaries` |
| `createProofSummary` | POST | `/proof-summaries` |
| `getProofSummary` | GET | `/proof-summaries/{id}` |
| `updateProofSummary` | PUT | `/proof-summaries/{id}` |
| `deleteProofSummary` | DELETE | `/proof-summaries/{id}` |
| `getProofSummaryByFact` | GET | `/proof-summaries/by-fact` |

[#824](https://github.com/cacack/my-family/issues/824) added ten more. The two history reads return
the branch's view of the entity's stream, each entry labelled `origin` (`branch` or `main`). The
eight restore-point and rollback operations declare the scope **only to refuse it** with 409
`rollback_mainline_only`, because rollback stays mainline-only
([ADR-005, "Entity history and rollback on a branch"](./adr/005-research-branch-data-model.md#implementation-note--entity-history-and-rollback-on-a-branch-823-824-delivered)):

| operationId | Method | Path |
|---|---|---|
| `getPersonHistory` | GET | `/persons/{id}/history` |
| `getFamilyHistory` | GET | `/families/{id}/history` |
| `getPersonRestorePoints`, `getFamilyRestorePoints`, `getSourceRestorePoints`, `getCitationRestorePoints` | GET | `/{persons,families,sources,citations}/{id}/restore-points` (refused) |
| `rollbackPerson`, `rollbackFamily`, `rollbackSource`, `rollbackCitation` | POST | `/{persons,families,sources,citations}/{id}/rollback` (refused) |

[#829](https://github.com/cacack/my-family/issues/829) added six reads that answered from the
mainline while a branch was active: search, the families list and the kinship reports. Descendancy
and the relationship calculator walk the tree through set-based overlay reads, one batch per
generation (`GetFamiliesForPersons`, `GetFamilyChildrenByFamilyIDs`, `GetPedigreeEdgesByPersonIDs`,
`GetPersonsByIDs`), rather than a chain of single-row reads per person:

| operationId | Method | Path |
|---|---|---|
| `searchPersons` | GET | `/search` |
| `listFamilies` | GET | `/families` |
| `getFamilyGroupSheet` | GET | `/families/{id}/group-sheet` |
| `getAhnentafel` | GET | `/ahnentafel/{id}` (JSON and text) |
| `getDescendancy` | GET | `/descendancy/{id}` |
| `getRelationship` | GET | `/relationship/{personId1}/{personId2}` |

That is **92 API operations carrying `?branch=`**. Treat `internal/api/openapi.yaml` as the count
of record — the drift test described below re-derives it from the spec on every run.

The frontend mirrors exactly those 92 in `isBranchScopedRequest()`
(`web/src/lib/api/client.ts`), matching on method as well as path, since two methods on one path
need not agree. The free-text `{surname}` and `{place}` segments are
matched as a single non-empty, non-slash segment rather than as a UUID, so a percent-encoded place
name still resolves. A drift test in `web/src/lib/api/client.test.ts` parses `openapi.yaml` and
fails in **both** directions, so the allowlist cannot silently fall behind the spec.

Branch scoping is no longer confined to the seven-type slice, so "everything else is main-only" is
not the rule. The surfaces that *are* still mainline-only while a branch is active render
`MainlineNotice.svelte`, so the UI never presents mainline data as branch data. Within browse and
map that is now exactly one: brick walls (not event-sourced, so branch-scoping them means first
deciding whether they become event-sourced —
[ADR-005, "Entities that stay main-only"](./adr/005-research-branch-data-model.md#entities-that-stay-main-only),
[#761](https://github.com/cacack/my-family/issues/761)). The surname index and per-surname list,
the place index and per-place list, the cemetery index and per-cemetery person list, and the map
all follow the active branch, and so do the source list and source detail pages (#758), the
person media gallery (#759) and the `/evidence` pages and person evidence panel (#760). Since #829
so do every search surface (`/search`, the header search box and the person picker), the families
list, the dashboard's family count and recent families, `/analytics`, the family group sheet,
`/ahnentafel/{id}`, `/descendancy/{id}` and `/relationship`. With every #676 sub-issue delivered,
what still renders the notice is mainline by nature (quality checks, research suggestions, the
global change history, snapshots) or by decision (brick walls, repositories, exports); grow the allowlist and the notice coverage together if that changes. The person and
family history panels follow the branch (#824); their Restore tab and rollback dialog are withdrawn
on a branch instead of labelled.

Mainline-only *writes* get a guard, not a notice. GEDCOM import always writes the mainline, so
`/import` and the onboarding import step withdraw their upload controls while a branch is active
(`BranchImportBlocked.svelte`, offering the switch back to the mainline), the onboarding wizard
never opens on a branch, and the API refuses an import request carrying `?branch=` with a 400
(#825). Repositories, main-only by decision, stay editable on a branch but say they are shared
across all branches. Rollback is refused the same way: the UI withdraws it and the API answers
`?branch=` with a 409 (#824).

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
rollback is main-only (`Handler.rollbackEntity`; a `?branch=` rollback or restore-point request is
refused with 409 `rollback_mainline_only`, #824). [#676](https://github.com/cacack/my-family/issues/676)
widened branch writes to every entity type that is not main-only by decision; `MergePersons`
(`PersonMerged`) is the one command still refused on a branch, because a branch merge cannot yet
replay it safely (see `branchAwareEventTypes` in `internal/command/handler.go`).

Merging a branch back into `main` is **not** a gap: [#55](https://github.com/cacack/my-family/issues/55)
delivered the command and `POST /branches/{id}/merge`, and the merge *review* UI
([#95](https://github.com/cacack/my-family/issues/95)) drives it from `/branches/{id}` — resolve
each conflict, or leave a whole entity behind as a `main` resolution. A merge interrupted mid-replay
is finished with `POST /branches/{id}/merge/resume` ([#685](https://github.com/cacack/my-family/issues/685);
surfaced in the UI by [#830](https://github.com/cacack/my-family/issues/830): `GET /branches`,
`GET /branches/{id}` and the comparison report `merge_state: incomplete` with the named pending
entities — computed read-only from the resume's own plan and read-model repair check, so a merge whose last replay landed but failed to project reads as incomplete too — the list, the stale-branch notice and the
branch page flag it, and the branch page's "Finish merge" resolves any pending entity and resumes). The resume covers every stream a branch can write — persons, families, associations, (#758) sources, citations and notes, (#759) media, and (#760) evidence analyses, evidence conflicts, research logs and proof summaries — replayed in the merge's evidence order under its evidence rules, including the media-owner rule and the three GPS rules (an artifact must land on a subject main will have, an edit on an artifact main still has, and a subject delete must not cascade onto research main added or changed after the fork, or changed after the branch's own edit of it landed); an auto-planned stream that breaks one is reported pending. A media stream's read-model repair re-projects main's own events onto main only, so it never copies file bytes into a branch or drops shared ones; a GPS row missing because main deleted its subject is recognised as cascaded, not resurrected. The one case neither can repair soundly (an item or artifact whose projection failed before main merged its owner or subject person away) is refused before anything is written. What is still outstanding is partial merge
([#684](https://github.com/cacack/my-family/issues/684)): excluding an entity is not the same as
promoting a subset of one entity's changes.

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
