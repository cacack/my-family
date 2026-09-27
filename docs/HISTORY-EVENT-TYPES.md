# History Event Types

Every change-log view — global history (`GET /history`), entity history (mainline and
`?branch=`), branch compare and merge review (`GET /branches/{id}/compare`), and snapshot
compare — renders events through **one** table: `historyEventCatalog` in
[`internal/query/history_catalog.go`](../internal/query/history_catalog.go)
([#739](https://github.com/cacack/my-family/issues/739),
[#827](https://github.com/cacack/my-family/issues/827)).

Each event type the event store can hold (the cases of `StoredEvent.DecodeEvent`) is either

- **mapped** to the `entity_type` and `action` its `ChangeEntry` reports, or
- **excluded** because it is not a change to anyone's family tree.

There is no third outcome: no response carries `unknown`. The same table filters the global
history **in the store** (`EventStore.ReadGlobalHistory` excludes the excluded types and every
research branch's own events before `LIMIT`/`OFFSET`), so `total` and `has_more` describe exactly
the set `items` is paged from. The `entity_type` query filter is derived from it too.

**Adding an event type:** add it to `DecodeEvent` (ES-007) *and* to `historyEventCatalog`.
`TestHistoryCatalog_CoversEveryEventType` parses `DecodeEvent`'s switch and fails for any type the
catalog does not classify (or classifies outside the OpenAPI enums);
`TestHistoryCatalog_MapsEveryBranchAwareEventType` fails for any branch-writable type that is not
mapped or not exercised by the every-type compare fixture
(`TestCompareBranch_EveryBranchWritableType`, memory/SQLite/PostgreSQL).

## Mapped

| Event type | `entity_type` | `action` | Entry name | Links to |
|---|---|---|---|---|
| `PersonCreated` / `PersonUpdated` / `PersonDeleted` | `person` | created / updated / deleted | person's name | person page |
| `NameAdded` / `NameUpdated` / `NameRemoved` | `person` | updated | person's name; `changes.name` holds the variant ("Given Surname") before/after | person page |
| `PersonMerged` | `person` | merged | surviving person; `changes.merged_person` and each resolved field | person page |
| `FamilyCreated` / `FamilyUpdated` / `FamilyDeleted` | `family` | created / updated / deleted | "Partner & Partner" | family page |
| `ChildLinkedToFamily` / `ChildUnlinkedFromFamily` | `family` | updated | family; `changes.children` names the child | family page |
| `SourceCreated` / `SourceUpdated` / `SourceDeleted` | `source` | created / updated / deleted | title | source page |
| `CitationCreated` / `CitationUpdated` / `CitationDeleted` | `citation` | created / updated / deleted | "Source title (Fact)" | its source (`parent_entity_*`) |
| `MediaCreated` / `MediaUpdated` / `MediaDeleted` | `media` | created / updated / deleted | title (else filename) | its owner (`parent_entity_*`) |
| `NoteCreated` / `NoteUpdated` / `NoteDeleted` | `note` | created / updated / deleted | text excerpt | — (no note page) |
| `SubmitterCreated` / `SubmitterUpdated` / `SubmitterDeleted` | `submitter` | created / updated / deleted | name | — (no submitter page) |
| `RepositoryCreated` / `RepositoryUpdated` / `RepositoryDeleted` | `repository` | created / updated / deleted | name | repository page |
| `AssociationCreated` / `AssociationUpdated` / `AssociationDeleted` | `association` | created / updated / deleted | "role: Person and Associate" | the person (`parent_entity_*`) |
| `LifeEventCreated` / `LifeEventUpdated` / `LifeEventDeleted` | `life_event` | created / updated / deleted | "Fact, date, place" | its person or family (`parent_entity_*`) |
| `AttributeCreated` / `AttributeUpdated` / `AttributeDeleted` | `attribute` | created / updated / deleted | "Fact: value, date" | its person (`parent_entity_*`) |
| `LDSOrdinanceCreated` / `LDSOrdinanceUpdated` / `LDSOrdinanceDeleted` | `lds_ordinance` | created / updated / deleted | "type, date, temple" | its person or family (`parent_entity_*`) |
| `EvidenceAnalysisCreated` / `EvidenceAnalysisUpdated` / `EvidenceAnalysisDeleted` | `evidence_analysis` | created / updated / deleted | "Fact: conclusion" | analysis page |
| `EvidenceConflictDetected` / `EvidenceConflictResolved` | `evidence_conflict` | created / updated | "Fact: description" | conflict page |
| `ResearchLogCreated` / `ResearchLogUpdated` / `ResearchLogDeleted` | `research_log` | created / updated / deleted | "search description (repository)" | research log page |
| `ProofSummaryCreated` / `ProofSummaryUpdated` / `ProofSummaryDeleted` | `proof_summary` | created / updated / deleted | "Fact: conclusion" | proof summary page |

Updates (and merges) carry field-level `changes` with **both** `old_value` and `new_value`. The
old value is derived from the entity's earlier events as the scope sees them (the mainline, or a
branch's overlay view of the stream — `branchVisibleStreamEvents`), read for the whole batch with
one set-based event-store read per side; names of persons, families, sources and citations still
come from the read model in one batched lookup per type (#697). A person a family, association
or child link refers to who is gone from the read model (deleted since) is named from their own
folded stream, read for all such people with one more set-based read per side — never a raw id
while the log still names them. Nothing is read per entry.

## Excluded (not genealogical changes)

| Event type | Why |
|---|---|
| `GedcomImported` | Import audit record; the persons, families and sources it created each have their own events. |
| `SnapshotCreated` / `SnapshotDeleted` | Research-artifact markers (#624): a snapshot names a position in the log, it changes no data. |
| `BranchCreated` / `BranchDeleted` / `BranchMerged` / `BranchMergeResumed` | Branch lifecycle (ADR-005 §Merge): they describe a research branch, not the data on it; a merge's replayed changes appear as their own events. |

These events remain in the append-only log (ES-002) as the audit record; only the change-log
views leave them out. The branch diff's `researchMetadataEventTypes` is a subset of this list
(`TestHistoryCatalog_ExcludesResearchMetadata`).

## Branch scope

- **Global history** is the mainline's: a research branch's own events are filtered out in the
  store, matching the `MainlineNotice` the history page shows. They appear once a merge replays them.
- **Entity history with `?branch=`**, **branch compare** and **merge review** render a branch's
  events with the same table; names resolve through the branch overlay and old values come from
  the branch's view of each stream.
