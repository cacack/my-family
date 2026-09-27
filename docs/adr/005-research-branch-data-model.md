# ADR-005: Research-Branch Data Model

**Status:** Accepted
**Date:** 2026-07-20
**Decision Makers:** Chris
**Related Features:** v0.12 - Git Workflow (#54)

## Context

The git-inspired research workflow is the project's flagship differentiator: let researchers
explore an unproven hypothesis on an isolated branch, then merge it back to the main tree with
a reviewable diff when the evidence supports it (ETHOS.md, ROADMAP.md Phase 1). Epic #54 frames
this as "branches are pointers to event-stream forks, not copies of data," and ADR-001 anticipates
it ("branch = filtered event stream"). That framing is true — but *only at the event-store layer.*

The query/projection side breaks under that assertion. ADR-003 chose **synchronous, single-lineage
projections**: one linear event stream projected into one read model, updated in the same
transaction as the append. A branch introduces a second lineage of state that queries must be
able to see in isolation from `main`. Nothing in the current model expresses this — there is no
notion of "which branch is this row / this query / this event on."

This is the foundation the rest of v0.12 builds on, so it must be settled before any branch code
is written:

- **#669** (branch-aware read model & projections) needs concrete branch event types to project
  and a decided query-scoping mechanism to implement.
- **#670** (branch lifecycle: create / isolate / compare / delete) needs the storage model and
  the command/event surface.
- **#55** (merge with review) needs a defined merge operation and a semantic definition of a
  *conflict*.

This ADR produces the model. **It is a design artifact — no production code is introduced here.**

### What already exists (the substrate)

The branch model is built on machinery the codebase already has, not new invention:

- **A single append-only global event log with a monotonic `Position`.** `StoredEvent.Position`
  (`internal/repository/eventstore.go`) orders every event across all aggregate streams, and
  `EventStore.ReadAll(ctx, fromPosition, limit)` reads forward from any position. Streams are
  per-aggregate (keyed by UUID) with per-stream optimistic versioning, but they all share one
  ordered log.
- **Snapshots are named pointers to a global `Position`.** `domain.Snapshot`
  (`internal/domain/snapshot.go`) is `{Name, Description, Position}`; comparison
  (`internal/query/snapshot_queries.go`) diffs two snapshots by reading the events between their
  positions. A branch's *base point* reuses exactly this idea.

## Decision Drivers

- **Preserve ES-002 (append-only).** Whatever represents a branch must not introduce mutation or
  deletion of the event log.
- **Preserve the ADR-003 sync-projection model.** Branch scoping should extend the one
  projection path, not fork it into a second architecture.
- **Dual-database parity (DB-001/DB-004).** Every read-model operation is implemented twice
  (PostgreSQL + SQLite) and must stay in sync. The chosen mechanism must be *symmetric* across
  both engines — a design that is cheap on Postgres but awkward on SQLite doubles the maintenance
  surface.
- **Branches are lightweight and possibly numerous.** A hypothesis branch typically touches a
  handful of entities and may be short-lived; several may exist at once. Cost should scale with
  what a branch *changes*, not with the size of the whole tree.
- **Merge must be reviewable and conflicts must be well-defined** (drives #55).

## Considered Options

The design splits into three sub-decisions. Each is presented with its options; the overall
decision combines the chosen option from each.

### Sub-decision 1 — How are branch events stored?

#### Option 1A: Shared global log, tagged with a `branch_id`

**Description:** Branch events append to the same global log as `main`, each carrying a `branch_id`.
A branch is a small record: a name, an id, and a base `Position` on `main`. `main`'s events are
tagged with a reserved branch id.

**Pros:**
- One append-only log — ES-002 holds unchanged.
- Reuses the existing global `Position` ordering and the snapshot base-pointer idea directly.
- A branch stores only its own appended events (deltas), nothing copied.

**Cons:**
- Every event-log read that should be branch-scoped must filter on `branch_id`.

#### Option 1B: A separate event stream per branch

**Description:** Each branch gets its own physically separate event stream/log.

**Pros:**
- Strong physical isolation between branches.

**Cons:**
- Fragments the single global ordering that `Position`, snapshots, and history all depend on.
- Multiplies the storage/optimistic-locking model per branch.
- "Merge" becomes cross-stream reconciliation rather than a replay onto one log.

### Sub-decision 2 — How does a query scope to a branch?

#### Option 2A: `branch_id` dimension on read-model rows (copy-on-write overlay)

**Description:** Read-model tables gain a `branch_id` column. A branch edit projects a *shadow*
row tagged with the branch id; a query for entity `X` on branch `B` resolves `(branch_id=B, id=X)`
first and falls back to the reserved-`main` row when the branch hasn't touched `X`. A branch
*delete* writes a **tombstone** row so the entity is not resurrected by the `main` fallback.

**Pros:**
- Stores only deltas — a branch that edits five people stores five rows.
- One projection path and one query path, parameterized by `branch_id` — symmetric across
  PostgreSQL and SQLite (identical `ADD COLUMN` in both).
- Scales cheaply to many branches.

**Cons:**
- Every read-model query and projection handler must become branch-aware (thread `branch_id`
  through). Broad, but mechanical and shallow.
- Requires an explicit tombstone convention for branch deletes.

#### Option 2B: Replay-on-read

**Description:** Persist no branch rows. A branch read re-derives state on demand by folding
`main`'s events up to the base position plus the branch's own events.

**Pros:**
- No read-model schema change; strongest, most git-like isolation.

**Cons:**
- Reintroduces the exact cost projections exist to eliminate: list/search/tree queries would
  re-fold large portions of the tree on every request. Caching the result just recreates the
  "where is branch state stored?" problem (i.e. Option 2A or 2C).

#### Option 2C: Separate read-model tables per branch

**Description:** Each branch gets its own full set of projected tables / namespace; queries route
to the branch's table set.

**Pros:**
- Query logic barely changes — it points at a different namespace.

**Cons:**
- Duplicates the whole tree per branch even for a one-entity edit.
- The isolation mechanism *differs by engine* — Postgres has schemas, SQLite does not — so the
  dual-DB code splits into two divergent shapes, defeating the parity the architecture protects.
- Every migration must fan out across N branch namespaces.

### Sub-decision 3 — What is `main`?

#### Option 3A: A reserved, distinguished branch id

**Description:** `main` is a branch like any other, with a well-known id. Every query and
projection has one uniform, always-present scope.

**Pros:**
- One code path — no branch-vs-not special-casing anywhere.
- Existing rows/events backfill to the reserved id on migration.

**Cons:**
- A reserved-value convention every layer must know and honor.

#### Option 3B: The absence of a branch id (`NULL`)

**Description:** `main` rows/events carry no branch id; branch-ness is special-cased where it matters.

**Pros:**
- No sentinel value to reserve.

**Cons:**
- Every query and projection must special-case the `NULL`/non-`NULL` split.
- `NULL` semantics in SQL (indexing, `IN` matching) differ between engines, straining dual-DB parity.

## Decision

We adopt **1A + 2A + 3A**: a **shared append-only log tagged with `branch_id`**, queried through
a **`branch_id` copy-on-write overlay** on the read model, with **`main` as a reserved branch id**.

### The model

- **A branch** is a lightweight record: `{ id, name, description, base_position, created_at,
  status }`, where `base_position` is a `main` global `Position` — the same base-pointer concept
  as a snapshot. `status` is one of **`active`**, **`merged`**, or **`archived`**. Legal
  transitions: `active → merged` (on a successful merge) and `active → archived` (on discard/delete);
  `merged` and `archived` are terminal — a branch in either state accepts no further writes. Only
  an `active` branch is merge-eligible (see Merge).
- **`main`** is the reserved branch id, fixed as **`uuid.Nil`** and exposed as the constant
  `domain.MainBranchID` so downstream code cites one literal rather than re-deciding it. It is
  always present; there is no "not on a branch" state to special-case.
- **Branch events** append to the one global log, each tagged with its `branch_id`. Concretely,
  `branch_id` is added as a **column on `StoredEvent`** and a **new parameter on
  `EventStore.Append`**; the domain event structs (`PersonUpdated`, etc.) are **unchanged** —
  branch-ness is envelope metadata, not payload. `main` events carry `branch_id = MainBranchID`.
  ES-002 is untouched — nothing is mutated or deleted.
- **Optimistic versioning becomes per-`(streamID, branch_id)`.** Today `Append`/`GetStreamVersion`
  key the version counter on the aggregate `streamID` alone. Branch writes must not contend with
  `main` (or other branches) on that counter, or two isolated hypotheses touching the same person
  would spuriously fail at *write* time. So the version dimension gains `branch_id`: a branch's
  first write to an existing aggregate seeds its expected version from that aggregate's `main`
  version at `base_position`, then increments within the branch. Divergence between a branch and
  `main` is surfaced at *merge* time by conflict detection (below), never as a write-time
  concurrency error (preserves DB-002's meaning per scope).
- **Read-model rows** carry a `branch_id`. Branch edits write shadow rows; branch deletes write
  **tombstone** rows (the branch's shadow row for that entity with a `deleted = true` marker and
  no other fields) so the `main` fallback does not resurrect the entity. A branch-scoped query
  returns the branch's row for an entity when present (a tombstone resolves to "absent"),
  otherwise the `main` row.
- **Overlay semantics are *live*, not frozen.** Because unmatched entities fall back to the
  current `main` row, a branch reflects corrections made on `main` after the branch was created —
  *except* for entities the branch has overridden. This is a deliberate choice: unlike a git
  checkout (frozen for reproducible builds), a genealogy branch sits over a *living* dataset, and
  meanwhile-corrections on `main` are usually *wanted*. The `base_position` still anchors
  comparison and conflict detection (below); it does not freeze reads.

### Branch domain/event types (named for #669/#670)

These are the concrete events #669 projects and #670 emits. Field lists are indicative, to be
finalized in implementation:

- **`BranchCreated`** — `{ BranchID, Name, Description, BasePosition, OccurredAt }`. Establishes a
  branch off `main` at `BasePosition`.
- **`BranchDeleted`** — `{ BranchID, OccurredAt }`. Archives/discards a branch. Append-only: this
  records the deletion as a new event; it does not remove the branch's prior events from the log
  (ES-002). Projections drop the branch's overlay rows.
- **`BranchMerged`** — `{ BranchID, BasePosition, MergedAtPosition, OccurredAt }`. Records that a
  branch's changes were promoted to `main` (see Merge, below).
- **`BranchMergeResumed`** — `{ BranchID, MergedAtPosition, ReplayStreamVersions, Resolutions,
  OccurredAt }`. Added by #685: records the decisions a resumed merge made, and the replay plan
  they produce (see "Resuming an interrupted merge" in the merge implementation note).

All of them satisfy the existing `Event` interface (ES-005) and must be added to `DecodeEvent()`
(ES-007) and to projection handling (PR-004) when implemented.

**Entity-level deletes on a branch are not `BranchDeleted`.** Deleting a *person* (or any entity)
while working on a branch reuses the existing domain delete event (`PersonDeleted`, etc.) tagged
with the branch's `branch_id`; its projection writes the tombstone row described above.
`BranchDeleted` is the distinct *branch-lifecycle* event that discards the whole branch and drops
all of its overlay rows (shadows and tombstones alike). The two are separate code paths that must
agree on tombstone handling.

### Merge

A **merge** is the replay of a branch's own **entity/domain mutation events** onto `main`: each
such event is re-applied as a new `main` event (new `Position`, `main` branch id), and a single
`BranchMerged` event records the promotion with the source `BranchID` and base position. The
replay set is restricted to the events that changed genealogy data (`PersonUpdated`,
`PersonDeleted`, `ChildLinkedToFamily`, …); the branch-**lifecycle** events (`BranchCreated`,
`BranchDeleted`) and the merge **marker** (`BranchMerged`) are explicitly **excluded** — replaying
them onto `main` would be meaningless or corrupting. This preserves append-only history on both
sides — the branch's original events remain in the log as branch events; the merge adds new `main`
events rather than rewriting anything. Partial merge (promoting a subset of a branch's changes) is
a future extension and is out of scope for this ADR.

Three properties the merge operation must hold, so `main`'s audit trail stays trustworthy (#55
implements these):

- **Provenance is preserved.** A replayed `main` event carries the *original* branch event's
  `OccurredAt` and originating actor — the audit trail must reflect when the research was actually
  done, not when it was promoted. The merge timestamp lives on the `BranchMerged` event, not on
  the replayed events.
- **Merge is idempotent, and the guard is atomic.** A read-then-act check (`status != merged`
  before replaying) is not enough — two concurrent merge requests can both observe `active` and
  each append the branch's changes. The `active → merged` transition must be an **atomic
  compare-and-set** (or a unique merge token) performed **in the same transaction** as the replay,
  reprojection, and `BranchMerged` emission, so exactly one request wins and any retry is a no-op.
  Concurrent merges of the same branch are serialized on that CAS.
- **Replay is batched.** The re-append + reprojection of the branch's mutation events, the status
  CAS, and the `BranchMerged` emission all run in a single transaction (per ADR-003's
  synchronous-projection model) rather than one round trip per event.

### Conflict definition (drives #55)

A **conflict** exists when `main` and the branch have made **incompatible changes to the same
aggregate after the branch's `base_position`**. For each aggregate the branch modified, compare
the branch's changes against `main`'s events with `Position > base_position`. Three conflict
classes must be detected — the definition covers all event shapes, not just field updates:

1. **Edit vs. edit** — both sides change the same field (from an `*Updated` event's `Changes`
   map) to different values. That field is in conflict.
2. **Delete vs. edit** — one side deletes the aggregate (a `*Deleted` event / branch tombstone)
   while the other modifies it. The aggregate is in conflict; a merge must never silently
   resurrect a `main`-deleted entity or silently discard a `main` edit to a branch-deleted one.
3. **Create vs. create** — both sides independently create an aggregate that resolves to the same
   identity (e.g. same GEDCOM xref). Treated as a conflict pending review rather than a blind
   double-insert.

Non-`*Updated` structural events (e.g. link/unlink child, add/remove marriage) are compared at
the granularity of the relationship they assert: the same relationship changed divergently on
both sides is a conflict, following the same "incompatible change to the same target" rule.

A field/target changed on only one side (the other side untouched since `base_position`) is **not**
a conflict — it merges cleanly. Any conflict requires review before the merge can complete.

Conflict detection is computed from the **event log plus the base position alone** — it does not
depend on the read-model scoping mechanism (2A). This keeps merge/review logic (#55) decoupled
from the projection design.

### Interaction with snapshots and rollback (coordinates with #624)

Snapshots and branch base points are the same primitive — a named pointer to a global `Position`
— so they compose cleanly:

- A snapshot taken *on a branch* points to `(branch_id, position)`; on `main` it points to
  `(main, position)`, i.e. today's behavior.
- **Rollback** to a snapshot is a read/compare operation over positions and, under the overlay
  model, is naturally scoped by `branch_id`.

This ADR did **not** change how snapshots are created. It surfaced a coupling that **#624** had
to resolve: `SnapshotCreated` existed and decoded (ES-007) but was never emitted —
`SnapshotService.CreateSnapshot` wrote directly to the `SnapshotStore`, bypassing the
event-sourced pipeline. The recommendation recorded here was to route snapshot creation and
deletion through the event pipeline.

#### Implementation Note — snapshot event model (#624, delivered)

The recommendation was adopted. Snapshots are now event-sourced, exactly like the branch registry:

- **`CreateSnapshot` / `DeleteSnapshot` are commands** on `command.Handler`, not query-service
  methods. They append `SnapshotCreated` / the new `SnapshotDeleted` on the snapshot's own stream
  (stream type `snapshot`, the snapshot id as stream id) on `repository.MainScope`.
- **The registry is projection-written.** `projectSnapshotCreated` upserts the row and
  `projectSnapshotDeleted` drops it, so rebuilding the projection reconstructs the registry — the
  property a directly-written store could never have. `SnapshotStore` gained `Upsert` (all three
  backends) for idempotent replay, and a replayed delete against a missing row is a no-op.
- **The marker never perturbs what it marks.** `CreateSnapshot` reads the log head *before*
  appending, so the snapshot's `Position` excludes its own creation event. This is the answer to
  the chicken-and-egg the issue raised, and it matches how `CreateBranch` pins a base position.
- **Snapshot events are hidden from the change log.** `mapEventTypeToEntityAndAction` skips both
  types: they are audit records on the log, not genealogical changes, and a snapshot comparison
  reads a range that contains one of the two markers.

**Still open — branch-scoped snapshots.** The bullet above ("a snapshot taken *on a branch* points
to `(branch_id, position)`") is **not** implemented: the `snapshots` table has no `branch_id`
column. Rather than record a branch snapshot as if it were a mainline one, both commands refuse on
a branch-scoped handler with `ErrSnapshotNotBranchScoped`. Closing that gap means giving the
snapshot registry the same `branch_id` overlay treatment #676 is fanning out to the other
read-model entities.

**Still open — snapshots created before #624.** Rows written by the old direct-store path carry no
`SnapshotCreated` event, so "the registry rebuilds from the log" holds only for snapshots created
after this change. Nothing replays the log into a projector today, so no data is at risk yet; the
constraint is that rebuild tooling (#680) must backfill those rows — or consciously drop them —
rather than assume the log is complete. Deleting such a snapshot works: `DeleteSnapshot` reads the
snapshot's stream (`ReadStream`, then `scanSnapshotStream` over its mainline events), finds version
0 because the stream has no events, and appends the tombstone with `expectedVersion` -1 (a new
stream).

## Entities that stay main-only

Branch scoping is a bounded set, not a migration in progress. Three different reasons keep a
read-model entity on `main`, and they must not be confused:

- **Pending** — the entity is destined for a `branch_id` and simply has not been done yet. These are
  the remaining sub-issue of [#676](https://github.com/cacack/my-family/issues/676): GPS artifacts
  ([#760](https://github.com/cacack/my-family/issues/760)). (Media metadata,
  [#759](https://github.com/cacack/my-family/issues/759), is delivered; its file bytes are shared
  by design, not pending — see the implementation note below.) Snapshots are pending too, though
  not as a #676 sub-issue: #624 made them event-sourced, and what remains is giving the registry a
  `branch_id` (see *Interaction with snapshots and rollback*, "Still open — branch-scoped
  snapshots").
- **Blocked** — branch scoping is neither scheduled nor ruled out, because a prior question has to
  be answered first. This is brick walls, which wait on an event-sourcing decision of their own
  (below). Snapshots were here until [#624](https://github.com/cacack/my-family/issues/624) made
  that decision for them.
- **Decided** — the entity will not gain a `branch_id` at all. That set is fixed here.

### The decided set

**Submitter**, **Repository**, **RepositoryExternalID** and **LDSOrdinance** are permanently
main-only.

Submitters, repositories and their external identifiers are *file-* and *archive-level metadata* —
who supplied a GEDCOM, which archive holds a source document, and what that archive calls it. They
describe the provenance of the dataset and the identity of real-world institutions, not a
genealogical claim. LDS ordinance records are sacramental records transcribed from an external
authority. None of them is an artifact a **research hypothesis** forks: a branch exploring "was Mary
the daughter of John?" does not produce a competing version of an archive's street address, and a
branch that changed one would be asserting a fact about the world rather than about a family.

The cost side is the same as for any branch-scoped entity, and it buys nothing here: a `branch_id`
column, a composite `(id, branch_id)` primary key, a `branch_id`-leading index, overlay resolution
on three backends, tombstones, a manual cascade in every Delete method, and triplicate cross-backend
tests — for no expressible research use case.

**The decision is reversible.** The pattern is mechanical, so if a real use case appears the work is
ordinary rather than exploratory. This section is the place to revisit it; changing it means
amending this ADR, not silently adding a column.

### Blocked on an event-sourcing decision — brick walls

`SetBrickWall` and `ResolveBrickWall` (`internal/repository/{postgres,sqlite,memory}/readmodel.go`)
**write the read model directly, bypassing the event store.** There is no `BrickWallSet` event and no
projection handler; the read model is the system of record for brick-wall status.

An entity whose state never passes through the event log cannot be branch-scoped in the sense this
ADR defines: there are no branch-tagged events to project, nothing to replay on merge, and nothing
for conflict detection to compare against the base position. **Branch-scoping brick walls therefore
means first deciding whether they become event-sourced** — the same call
[#624](https://github.com/cacack/my-family/issues/624) must make for snapshots, and for the same
reason (see *Interaction with snapshots and rollback*, above). #624 has since answered it for
snapshots — emit the events and let a projection write the registry — and that answer is the natural
precedent for brick walls, but applying it to them is a separate change. This ADR records the
brick-wall question and its coupling; it does not answer it.

Until then brick walls stay main-only. Sub-issue A ([#756](https://github.com/cacack/my-family/issues/756))
applied only the *leak* fix — constraining the mainline UPDATE to mainline rows, so a mainline call
stops mutating every branch's shadow row (BR-003) — and left the scoping question open.

**Snapshots were in the same state, for the same reason, until #624.** `SnapshotService.CreateSnapshot`
used to write straight to the `SnapshotStore`, so `SnapshotCreated` decoded but was never emitted.
#624 routed creation and deletion through the event pipeline (see *Implementation Note — snapshot
event model*, above), which unblocks branch scoping: a snapshot now has events a `branch_id` can tag.
Snapshots therefore moved from blocked to pending, and `docs/INTEGRATION-MATRIX.md` marks their
Branch column ❌ rather than ⛔. They are still **not** a #676 sub-issue; the remaining work is the
`(branch_id, position)` registry described in "Still open — branch-scoped snapshots".

## Consequences

### Positive

- Branches are true event-stream forks with **zero data copying** — only deltas are stored, on
  both the log and the read model.
- ES-002 and the ADR-003 sync-projection model both remain intact; branch scoping is an
  *extension* (one added dimension), not a second architecture.
- `main` as a reserved id means one uniform code path — no branch-vs-not special-casing.
- Dual-DB parity is preserved: `branch_id` is an identical column addition in PostgreSQL and
  SQLite.
- Merge and conflict logic key off the event log + base position, so #55 is decoupled from the
  read-model design.

### Negative

- **Every read-model query and projection handler becomes branch-aware.** This is the bulk of
  #669's work — broad but mechanical, and done once across both stores.
  - Mitigation: default the scope to the reserved `main` id so all existing (non-branch) call
    sites behave unchanged. Both schemas add `branch_id` with a `MainBranchID` default, so
    existing read-model rows *and* existing event-log rows backfill to `main` on migration — no
    data rewrite, and BR-001 holds for historical events.
- **Branch deletes require a tombstone convention** so a deleted-on-branch entity is not
  resurrected by the `main` fallback.
  - Mitigation: the tombstone shape is specified above (a branch shadow row with `deleted = true`);
    treat it as a first-class projection case (parallels PR-003).
- **Live-overlay semantics can surprise** a user who expects a frozen snapshot of `main`.
  - Mitigation: the semantic is documented here and should surface in the branch UI (#94); the
    `base_position` remains available for explicit as-of comparison.

### Neutral

- Storage grows with branch *activity*, not branch *count* — consistent with the event-sourcing
  storage profile already accepted in ADR-001.
- The reserved-`main`-id constant becomes a small piece of shared vocabulary across domain,
  repository, and query layers.

## New Invariants

This ADR introduces the **Branch (BR)** invariant category — **BR-001 through BR-006**, covering
`branch_id` tagging with a reserved `main`, append-only branch events on the shared log,
`branch_id` read-model rows with copy-on-write overlay + tombstones, non-rewriting merges,
per-`(stream_id, branch_id)` optimistic versioning, and the branch-aware event-type restriction on
branch-scoped writes. Their canonical text and verification methods live in
[ARCHITECTURAL-INVARIANTS.md](../ARCHITECTURAL-INVARIANTS.md) (the single source of truth for
invariants, cited by ADRs rather than restated in them).

## Implementation Notes (for #669 / #670 / #55)

- **#669** — add `branch_id` to read-model tables in **both** `repository/postgres/` and
  `repository/sqlite/`; thread a branch scope (defaulting to reserved `main`) through query
  services and projection handlers; implement the shadow-row + tombstone resolution in the read
  path. Add all three lifecycle events — `BranchCreated`, `BranchDeleted`, **and `BranchMerged`** —
  to `DecodeEvent()` (ES-007) and projection handling (PR-004); the `BranchMerged` projection
  applies the `active → merged` status transition and triggers the affected read-model rebuild.
  **Performance
  constraints (load-bearing — get these right up front, they are expensive to retrofit once
  `branch_id` is threaded through both backends):**
  - **The overlay must resolve in one set-based query, never per-row (N+1).** List/search/tree
    queries — the ones ADR-003's projections exist to keep cheap — must fetch the branch overlay
    in a single statement, e.g. `SELECT DISTINCT ON (id) * … WHERE branch_id IN (:branch, :main)
    ORDER BY id, (branch_id = :branch) DESC` on Postgres, with an equivalent window-function /
    correlated-subquery form on SQLite. Both backends require a composite index `(id, branch_id)`.
  - **Caching cannot mask overlay cost.** Because the overlay is *live* (§The model), any `main`
    write can change any open branch's read of an untouched entity, so branch views are not
    cache-stable; the SQL path itself must be fast per request. Don't invest in a read-through
    cache as the mitigation.
- **#670** — commands + handlers for branch create/delete emitting `BranchCreated` /
  `BranchDeleted`; branch-scoped writes tag events with the branch id; compare = diff branch
  events vs `main` after `base_position`.
- **#55** — merge = replay branch-only events onto `main` (batched, provenance-preserving,
  idempotent — §Merge) + emit `BranchMerged`; conflict detection per the three classes above;
  reviewable diff from the same event comparison. **Conflict detection must scope its scan to the
  aggregates the branch actually touched**, not the whole global tail: derive the branch's set of
  `stream_id`s first (bounded by branch size), then read `main` events for just those streams
  after `base_position`. This needs an index on `(stream_id, position)`; a naive `ReadAll`-style
  full-tail scan grows with *all* `main` activity and is re-paid on every compare/merge call.

## Implementation Note — SQLite event-store migration (#670, delivered)

Making optimistic versioning per-`(stream_id, branch_id)` (§The model) required replacing the
`events` table's `UNIQUE(stream_id, version)` with `UNIQUE(stream_id, branch_id, version)`. SQLite
cannot alter a constraint in place, so this shipped as the **12-step table rebuild** SQLite
documents for exactly this case: detect the legacy constraint in `sqlite_master`, then in one
transaction create the new table from the shared DDL, copy every row with explicit column lists,
verify the row count, drop, rename, and recreate the indexes. It runs once, automatically, on the
first open of a pre-#670 database and logs a single line. PostgreSQL needs no table rebuild, but the
swap is not atomic either: it **drops** the old `UNIQUE(stream_id, version)` constraint and the old
`idx_events_stream_version` index, then **creates** the composite
`idx_events_stream_branch_version`. Operators should expect a brief window with neither uniqueness
rule in force, not an in-place replacement.

This is a **deliberate divergence from how #669 handled the analogous read-model change**, which
detects the stale schema and refuses to start. The read model is derived data and can be dropped
and re-projected, so refusing is recoverable; the event log is the source of truth and cannot be
regenerated, so a detect-and-refuse guard there would permanently lock every existing SQLite
install out of branches with no path forward. The rebuild is the only option that preserves
ES-002 while letting existing installs adopt branches.

Both halves of that contrast are inputs to **#680**, which owns the general migration-strategy gap
(read-model schema versioning + `rebuild-read-model`, and event schema evolution). This ADR does
not decide that strategy; it records one concrete case where the event store needed real migration
discipline and got a hand-written one.

## Implementation Note — merge (#55, delivered)

Merge shipped as `Handler.MergeBranch` (`internal/command/branch_merge_commands.go`) over a plan
built by `BranchService.PlanMerge` (`internal/query/merge_conflicts.go`), exposed as
`POST /branches/{id}/merge`. The invariant it upholds is BR-004, whose canonical text and
verification live in [ARCHITECTURAL-INVARIANTS.md](../ARCHITECTURAL-INVARIANTS.md). Four things
departed from §Merge as written and are recorded here. An interrupted replay is finished by
`Handler.ResumeMerge` / `POST /branches/{id}/merge/resume` (#685, delivered — see "Resuming an
interrupted merge" below).

**The atomic guard is the branch's own version constraint, not a status CAS.** §Merge asks for an
atomic compare-and-set on `active → merged`. The implementation gets that for free from a
mechanism this ADR already introduced: `BranchMerged` is appended to the **branch's own stream**,
at the version the request observed, before anything is written to `main`. Per-`(stream_id,
branch_id)` optimistic versioning (BR-005, `UNIQUE(stream_id, branch_id, version)`) makes that
append the CAS — two concurrent merges observe the same branch version, both attempt the append,
and exactly one succeeds; the loser gets `repository.ErrConcurrencyConflict`, surfaced as
`command.ErrMergeAlreadyClaimed` (HTTP `409 merge_already_claimed`), having written nothing to
`main`. The registry row is written by the projection (`MarkMerged`), never by a direct store
call, so a projection rebuild reconstructs the merge record — the same rule branch create and
delete follow. A sequential retry against an already-`merged` branch is refused earlier still, by
the status guard, as `409 branch_not_active`.

**The claim and the replay are NOT one transaction — bounded, and since #685 recoverable.** §Merge asks for the
CAS, the replay, the reprojection, and the `BranchMerged` emission to run *in a single
transaction*. They do not, because the codebase has no cross-store transaction facility:
ADR-003's synchronous projections commit per-append, and the event store and read-model store are
separate interfaces with no shared transaction handle. The failure mode is therefore real and
observable: if a replay append or projection fails partway, the branch is already `merged` while
`main` carries only some of the branch's entities. The merge itself does not retry or roll back;
the state is finished by resuming (below). Three things bound the damage:

- Replay issues **one `Append` per stream**, not per event, and the SQL backends wrap an `Append`
  in a transaction — so the failure granularity is a whole entity, never a half-applied one.
- The returned error names the stream that failed, and how many events *and* how many streams had
  already reached `main` — each against its own total — so the resulting state is diagnosable
  rather than silent.
- It is a distinct sentinel (`command.ErrMergePartiallyApplied`, surfaced as
  `500 merge_partially_applied`) rather than a generic failure, because it is the one merge outcome
  a client must not retry: the branch is terminal, so a retry returns `409 branch_not_active`,
  which is not evidence the merge completed. `GET /branches/{id}/compare` still works on a merged
  branch and is the way to see what actually landed; `POST /branches/{id}/merge/resume` is the way
  to finish it.

**Resuming an interrupted merge (#685, delivered).** `Handler.ResumeMerge`, exposed as
`POST /branches/{id}/merge/resume`, completes the replay append-only (ES-002): it appends only the
branch events not yet on `main` — plus, when the caller had to decide something, one decision record
on the branch's own stream — and never rewrites or removes anything. It needs no transaction, because
both halves of what it must know are durable in the log:

- **What landed is derived from `main`.** The replay re-appends each branch event *decoded*, so the
  domain event's own payload id (`BaseEvent.ID`) travels with it onto `main`. Resume reads `main`'s
  events on the replay set's streams after the claim's `MergedAtPosition` — one set-based, paged
  `ReadStreamsForBranch` scan, not a read per stream — and a stream whose branch events' ids are
  all present is already replayed. One `Append` per stream is atomic on every backend, so a stream
  is wholly there or not at all; a half-present stream cannot come from an interrupted merge and
  is refused rather than guessed at.
- **What remains is recorded on the claim, and on any resume that decided something.** `BranchMerged` now carries `ReplayStreamVersions`:
  every stream the merge will replay, mapped to its #698 pin (`main`'s version when the verdict was
  computed). A stream the branch touched but absent from the map was resolved to `main`. So the
  per-stream resolutions and the staleness pins survive the request that chose them, in the log.
  The field is deliberately not `omitempty`: an empty plan is stored as `{}` and decodes non-nil,
  while a claim written before #685 has no key and decodes nil — "replay nothing" and "plan not
  recorded" are different facts. A resume that decides streams appends `BranchMergeResumed` to the
  branch's own stream **before replaying anything**, carrying the whole updated plan (a stream
  resolved to `main` dropped, a stream resolved to `branch` re-pinned to the version the caller
  reviewed) plus the resolutions themselves for audit; the latest such record replaces the claim's
  plan. It is a lifecycle marker like `BranchMerged` — excluded from replay and diffs, handled in
  `DecodeEvent` (ES-007) and, as a no-op, in the projector (PR-004) — and its append asserts the
  branch stream's version, so two resumes cannot both record decisions
  (`409 merge_resume_concurrent`).

A remaining stream is replayed automatically only when `main` still sits at its pinned version —
the same guarantee the original attempt ran under, re-asserted at append time by the shared
`replayStream`. Otherwise (a mainline write landed on it after the claim, which is exactly the
residual staleness window below; `main` deleted the entity or merged it away since the claim; a
pre-#685 claim with no plan; or a replay that would now leave `main` referencing a person it no
longer has, or break an evidence or media-owner rule — see below) resume refuses with
`ErrMergeResumeNeedsResolution` (`409 merge_resume_needs_resolution`), **writing nothing**, and
lists the streams. The caller reviews them with `compare` and resumes again with a resolution per
listed stream: `branch` replays over `main` as it now stands (asserting *that* version, so a
further write still trips the guard), `main` leaves the entity as `main` has it — the deliberate
roll-forward. `branch` is a `400` for an entity `main` has removed since the claim — its stream
ends in a delete, `main` merged the person into another (`PersonMerged` writes only to the
survivor's stream, so the merged person's stream still sits at its pin), an association lost a
person to the delete cascade, or a citation or media item lost its source or owner the same way — for the reason `MergeBranch` offers only `main` on a main-side
delete: replaying edits onto an absent row appends them after its removal and restores nothing. A
resolution for any stream the claim or an earlier resume already decided is a
`400` (unless `main` has since moved that stream again; see below): a second request must not
quietly re-decide what the first one reviewed. That rule needs the
decision itself in the log, not just its effect: a `main` decision's effect is the *absence* of
replayed events, which a later resume cannot tell apart from "not yet replayed", so without
`BranchMergeResumed` the next resume would find the stream stale again, ask again, and accept
`branch`. Because the record lands before the replay, a resume interrupted after deciding leaves the
decision in force and the next resume carries it out without asking.

The third case exists because `main` can remove a person after the claim without touching any
stream the plan pinned: a branch that links `main`'s person P into `main`'s family F leaves P
unlinked on `main` until F replays, so `main` may delete P (or merge P away) in the interruption
window. F is not stale — `main` never wrote to it — so the plan would replay it automatically,
and the dangling-reference check (below) would refuse that replay on every attempt, with no
resolution accepted for F because it was "already decided". Resume therefore runs the
dangling-reference check over the streams the plan would replay *before* deciding which need the
caller and lists any stream that fails it as pending. A person counts as present if `main`'s read
model has them, or if a stream already on `main` or still to be replayed (not resolved to `main` by
this very request) *creates* them and `main` has not removed them since. Merely replaying a
person's stream does not count: when the branch both edited and linked P, P's stream of edits
replays onto a person `main` merged away without bringing them back, so without this rule the
resume would report success over a phantom child. Such a P is itself pending (removed on `main`),
and so is F. A
`main` resolution rolls the merge forward without it, recorded like any other decision; `branch`
is refused as a dangling reference. The check then runs once more over the final decision, and
also over the streams *already* on `main`: a `main` resolution may not exclude a person the branch
created whom a landed stream references and `main` lacks (reachable only from a pre-#685 claim, where the branch created
both the family and the child and only the family landed). Only this request's own resolutions are
checked against landed streams; a person `main` itself removed later is `main`'s change, not the
resume's to refuse. Every refusal — pending, dangling, concurrent — comes before the first write,
including the read-model repair below, so a `409` has written nothing at all.

**Evidence on resume (#758).** A resume applies the same two evidence rules `MergeBranch` checks
before its claim (`validateNoDanglingEvidence`), on the same terms as the person references above.
Its replay set is put in the merge's evidence order (`orderEvidenceForReplay`: sources that survive
the replay first, sources it deletes last), so a resumed merge continues the original order — the
remaining citations find their sources on `main`, and a doomed source's delete lands only after
every citation has left it. A stream the plan would replay automatically is listed as pending when
it is a citation whose *final* source `main` will not have (deleted on `main` after the claim, or
excluded by this request's `main` resolution) or a source delete that would cascade onto a citation
`main` still has (typically one `main` added after the claim). A source counts as one `main` will
have if a stream already on `main` or still to be replayed carries it and does not delete it —
unless `main` removed it since the claim — or otherwise if `main`'s read model has it. `main` rolls
such a stream forward without it; `branch` is refused as a dangling reference. The final decision is
checked again, and a `main` resolution may not exclude a source the replay creates while a citation
already on `main` cites it (reachable only from a pre-#685 claim).

**Media on resume (#759).** The merge's media-owner rule is part of the same shared check
(`checkEvidence`), so a resume applies it on the same terms: a media upload the plan would replay
automatically is pending when its owner (person, family or source) will not exist on `main` when it
lands — `main` deleted it after the claim, this request resolves the stream that creates it to
`main`, or a stream already on `main` deleted it (an owner-deleting stream counts as deleting
*after* the upload only while it is itself still to be replayed). `main` rolls such a stream
forward without it; `branch` is refused as a dangling reference, and a `main` resolution may not
exclude an owner the replay creates while an upload already on `main` is attached to it. Landed
detection is the usual payload-id scan, and a media stream's `main` row is read with `GetMedia`,
never the bytes. The read-model repair follows the version rule — every media projection writes the
row, version included, in one save — and cannot copy or lose file bytes: it projects `main`'s own
events onto `main` only, the only event carrying bytes is `MediaCreated` (whose bytes are `main`'s
own, in `main`'s log), `MediaUpdated` re-saves metadata with nil bytes (which keep what is stored),
and no branch row is ever written. A missing media row counts as removed for a reason the log
explains when its owner's `main` stream ends in a delete (the owner→media cascade writes nothing to
the media stream), following a person owner through any person merges `main` recorded since; a
pending edit of such an item resolves only to `main`, and a landed upload is not resurrected. One
case is refused rather than repaired: a landed upload whose projection failed and whose owner
person `main` then merged into a person it still has. `PersonMerged` would have moved the item to
the survivor, and that transfer is not in the media stream, so re-projecting it would attach it to
the merged-away person; the resume says so and writes nothing, and repairing that item needs a
read-model rebuild from the log (#680).

A claim written before #685 has no plan, so its first resume must decide every stream not yet on
`main` — including, for a merge that in fact finished with claim-time `main` resolutions, streams
the original request already declined. The log cannot distinguish those from unreplayed ones, so
the operator should check `compare` before answering; once answered, the answer is final like any
other.

"Final" is exact for a `main` decision: the stream leaves the plan and no later resume offers it
again. A `branch` decision instead re-pins the stream at the version `main` had when it was made,
and the plan vouches for the replay only while `main` is still at that version. If `main` writes to
the entity again (or removes a person its replay references) before the replay lands — the
decision's replay was interrupted, or lost the staleness race — that write was never reviewed, so
the #698 guard's reasoning applies afresh: the stream is pending again and a new request may decide
it either way. Keeping the old `branch` decision would silently override the unreviewed write.

Resume is idempotent: after completion it appends nothing and reports `replayed_event_count: 0`.
Concurrent resumes cannot both append a stream, because `replayStream` now passes the version it
read to `Append` unchanged — including `0` for a stream `main` has never seen, which asserts "no
prior events" — instead of translating `0` to the `-1` sentinel that switches the optimistic check
off. A resume that itself fails is reported as `500 merge_partially_applied` again, and resuming
again is the remedy. A claim whose registry projection never landed (branch still reads `active`)
is repaired by resume exactly as by `claimMerge`, then resumed — unless the branch was written to
in that window (an event after the claim's `MergedAtPosition`), which the recorded plan never
considered, so resume refuses rather than replay or drop it. An archived branch is refused
(`409 merge_not_claimed`), as is a branch never claimed.

**The read model is completed too.** If an earlier attempt's `Append` landed but its synchronous
projection then failed, the stream counts as replayed — appending its events again to re-drive the
projection would duplicate them — and `main`'s read model is behind the log for that entity. The
repo has no read-model rebuild command yet (#680), so resume repairs it itself: for every
already-replayed stream it compares `main`'s read-model version with `main`'s stream version. Every
projection handler for the branch-aware event set a branch can carry (BR-006: person, family and —
since #757 — association streams, since #758 source, citation and note streams, and since #759
media streams) writes the
aggregate's version as its last step, so a row behind the log is re-projected from the first event
past its version; re-running an event whose projection stopped midway is safe because its writes are
upserts and deletes, and the counter it bumps is part of the final write that did not happen. A
missing row is re-projected from the start unless the log explains its absence (the stream ends in a
delete; `main` merged the person away with `PersonMerged`, which writes nothing to the merged
person's stream; an association's person is gone from `main`, whose delete cascade removes the
row without writing to the association's stream; a citation's source was deleted on `main`, whose
cascade removes the citation the same way; or a media item's owner was, likewise — see *Media on
resume*). Repaired streams are reported in
`reprojected_stream_ids`; a resume after that finds nothing behind.

The one evidence write outside that version rule is a source's `citation_count`, which the citation
projections *step* in a save separate from the citation row: `CitationCreated` saves the citation
(version included) and then bumps the count, so a failure between the two leaves a citation level with
the log over a count one short; a re-point or delete steps counts before its own save, so re-running
one that failed at the save would step them twice. Neither is repairable by re-running events, so once
the replay is done the resume recounts, from `main`'s citations, the count of every source a citation
stream of the replay set on `main` has ever cited (and of every such source stream), and sets it
absolutely — correct whatever order the log's events landed in. A citation whose source needed it is
reported in `reprojected_stream_ids` too. The recount re-checks after writing, like the repair: a
racing citation projection shows up as a count that disagrees again, and a racing `SourceUpdated` its
save overwrote as a row behind the log, which is re-projected forward.

The repair takes no lock and appends nothing, so it can run alongside another resume's repair or a
mainline write to the same entity. Most projections set the row's version outright, so re-running
an *old* event after a newer one was projected would roll the row back — version and fields. The
repair therefore never applies an event the row already reflects: it re-reads the row before each
event and skips any at or below the row's version. That leaves only the instant between that read
and the projection's own save, so before reporting a stream repaired it reads the row and `main`'s
stream version once more and, if a racing write slipped into that instant and was rolled back,
re-projects the missing tail from the log (a bounded number of times, then `500
merge_partially_applied`: resume again). A row that disappears during the repair is not re-created;
the next resume classifies it. The dangling-reference check also counts a person created by an
already-replayed stream as present, since the creation is on `main` in the log, unless the log
shows `main` removed them since. The fault-injection
coverage is `internal/command/branch_merge_resume_test.go` and
`internal/command/branch_merge_resume_refs_test.go` (memory: dangling references, racing repairs,
associations), `internal/command/branch_merge_resume_evidence_test.go` (memory: evidence order,
evidence rules, citation-count recount, cascaded citations) and
`internal/integration/branch_merge_resume_test.go` plus `branch_merge_resume_evidence_test.go`,
which drive an interrupted
merge and its resume over HTTP against memory, SQLite and PostgreSQL — including a resume-time
`main` decision that a later resume must neither re-ask nor reverse, a family whose child `main`
deleted after the interruption, a replay whose projection failed after its append committed, a
merge carrying sources and citations (a re-pointed citation and a source delete included) interrupted
mid-replay, a cited source `main` deleted after the interruption, and (#759,
`branch_merge_resume_media_test.go` in both packages) a merge carrying media uploads and edits
interrupted mid-replay, a media owner `main` deleted after the interruption, and a failed media
projection repaired with the shared bytes intact on `main` and the branch.

**The conflict verdict is pinned to the versions it was computed against (#698, delivered).**
`PlanMerge` runs once, and a mainline write landing before the replay was never compared with the
branch's events — so replaying over it would silently override it, the exact outcome §Conflict
definition exists to prevent. Three pieces close that:

- **Plan-time capture, taken *before* `main` is read.** `PlanMerge` records `main`'s current
  version for every stream in the replay set (`MergePlan.MainStreamVersions`), alongside the
  conflict list. The versions and the verdict travel together because the verdict only means
  anything relative to them.

  **The order of `PlanMerge`'s reads is load-bearing**, which is why it composes the two halves of
  the diff itself rather than calling `loadBranchDiff` the way `CompareBranch` does: branch side
  first (it names the streams to pin), then the pin, then `main`'s side. Pinning *after* reading
  `main`'s tail would make the pin strictly **newer** than the verdict — a write landing in that
  window would be baked into the pin while never appearing in the tail the classifier compared, so
  `current == planned` would pass and the branch would replay over an event nothing ever compared
  it against. That is #698 itself, in the one direction the guard cannot observe. Pinning first
  makes the same write read as `current > planned`, a refusal, and keeps the compared tail a
  superset of the pinned state. `TestMergeBranch_StalePlanRefusesWriteInsideThePlanningWindow`
  (`internal/command`) pins the ordering.

  The capture reads one row per stream the *branch* touched. A stream that is then **replayed onto
  `main`** costs two further reads — the pre-claim check, and the one `replayStream` already made —
  for three in total. A stream resolved to `main` costs only the capture: both the pre-claim check
  and `replayStream` skip it, since branch events that are never replayed cannot override a
  mainline write. The passes that do happen are inherent, not waste: the pre-claim check exists
  precisely to observe a version *fresher* than the capture, so it cannot reuse it. Batching each
  pass into one set-based read is #697's business, not this guard's. The capture is deliberately not exposed on `CompareBranch`'s response, and
  `CompareBranch` does not pay for it: it is merge-plan internals with no meaning in a read-only
  diff.

  Every replayed stream **must** carry a pin. A missing entry is refused
  (`command.ErrMergePlanIncomplete`), never defaulted to `0` — a stream `main` has never seen is
  legitimately pinned at `0`, so treating absence as zero would silently wave through exactly the
  create-vs-create shape the guard exists for. `PlanMerge` satisfies this by construction; the
  refusal guards any future plan constructor; #685's resume applies the same never-default rule to
  the plan recorded on the claim, where an absent stream means "resolved to `main`". The
  create-vs-create class itself is *not* pinned: a colliding create on `main` lives on a different
  stream by definition, so it has no version in the replay set. That class is inert in v0.12 (its
  gate never opens without branch-scoped GEDCOM import, an explicit non-goal of epic #54), so the
  gap is recorded rather than built for.
- **A pre-claim refusal.** `MergeBranch` re-reads those versions and refuses with
  `command.ErrMergePlanStale` (`409 merge_plan_stale`) if any moved. It runs *before* the claim, so
  a stale plan is an ordinary refusal — nothing written, branch still `active`, re-plan via
  `GET /branches/{id}/compare` and retry. Checking after the claim would instead strand the branch
  `merged` with nothing replayed, and the retry would get `409 branch_not_active`.
- **A replay-time assertion.** `replayStream` compares the same planned version against the
  `GetStreamVersion` read it already performs, so it costs nothing extra. It is what catches the
  residual window.

The guard is an **explicit version comparison, not delegated to `Append`'s optimistic
concurrency**. All three backends — PostgreSQL (the primary, per ADR-002), SQLite and the
in-memory test double — gate that check on `expectedVersion >= 0`, so the `-1` a
branch-created stream was appended with turned it off entirely rather than asserting "no prior
events" — leaning on `Append` would have left precisely the case where `main` *gains* a stream the
branch also created completely unguarded. (Since #685 the replay passes `0` rather than `-1`, so
`Append` now does assert an empty stream; the explicit comparison stays, because `Append` only
compares against the version the replay itself just read, never against the plan's pin.) It is also deliberately **broader than a conflict**: any
mainline write to a replayed stream trips it, including one the classifier would have cleared. Rerunning
full conflict detection per stream was considered and rejected (it doubles the scan cost and still
leaves a window), so the guard fails safe on movement. A false positive costs one re-plan, after
which detection has run against the new `main` and the retry succeeds.

**Residual window, and what it costs recoverability.** The pre-claim check is not a lock, so a
write can still land between it and a given stream's append. The replay-time assertion catches it,
but by then the branch is claimed, so it surfaces as the partially-applied state above
(`500 merge_partially_applied`, message naming the stale plan) rather than as a clean refusal. A
resume then reports that stream as needing a resolution rather than replaying it over the write.
Shrinking that last window needs the cross-store transaction the codebase does not have; a
merge-wide lock is not an option, as it contradicts the per-`(stream, branch)` design this ADR
rests on.

This does **widen** what reaches the claimed-but-not-replayed state. Before the guard only a store
or projection failure got there; now any mainline write to a replayed stream in that window does,
including one the classifier would have cleared. For a single-stream branch the outcome is: branch
`merged`, `main` untouched, and a `500` telling the caller not to retry. That is a real regression
in recoverability, accepted because the alternative is the silent override, and bounded rather than
fixed: the replay's error states **explicitly whether `main` was modified at all**, so an operator
can tell "claimed, nothing replayed, `main` untouched" from a genuine half-application without
inferring it from counts. Either state is finished by resuming (#685, above).

**Conflict detection, and why the create-vs-create scan is gated.** All three classes from
§Conflict definition are detected, as a pure function (`classifyConflicts`) over the two event
slices, keyed on each side's *final* asserted value per field so a branch that edits and reverts
does not conflict. Structural link/unlink is compared as a synthetic field
(`children[<person-id>]`), which lands it in the edit-vs-edit class rather than a fourth one.
Edit-vs-edit and delete-vs-edit are scoped to the streams the branch touched, as the
Implementation Notes above require. Create-vs-create **cannot** be: a colliding create on `main`
is on a different stream by definition, so the only place to find it is `main`'s tail — exactly
the full-tail scan those notes call out as the anti-pattern. The compromise is a **gate**: the
tail read is issued only when the branch created at least one entity carrying a GEDCOM xref, since
an xref is the only identity two independent creates can share. In v0.12 that gate never opens —
xrefs are assigned only by GEDCOM import, and branch-scoped import is a stated non-goal of
epic #54 — so the cost is not paid today, and the class is already implemented for the day it
can be.
A truncated scan (either side hitting the comparison cap) is not merged at all: the command
refuses with `ErrBranchTooLargeToMerge` (`409 branch_too_large`) rather than promote half a branch
against an incomplete conflict list.

**Merge purges the branch's read-model overlay — at claim time, before the replay.**
`projectBranchMerged` calls `PurgeBranch` immediately after `MarkMerged`, and the claim runs
*before* any event reaches `main` (§the atomic guard, above). So the ordering is: claim → purge →
replay, not "purge once the promotion is done". Spelled out because it changes what the
partial-failure state looks like: if the replay then fails, the branch's work is absent from the
branch overlay *and* from `main`'s read model, present only in the event log.

That is survivable, and deliberately so. The log is the source of truth (ES-002), it is what
`PlanMerge` reads, and it is therefore what a resumed merge (#685) replays from — recovery never
needed the overlay. Nor does the purge widen what a client can observe: a merged branch's
`?branch=` reads already 404 the instant `Status` flips, so the rows it deletes were unreachable
from the moment the claim landed.

The rationale for purging at all is that the branch's state now lives on `main`, making the
overlay a stale duplicate; the purge makes the API's existing "its isolated view no longer exists"
answer true rather than merely enforced at the edge. Keeping it inside the projection (rather than
as a step the merge command runs after a successful replay) is what keeps a projection *rebuild*
hygienic: a rebuild reconstructs every branch's overlay from the log, and it is `BranchMerged`'s
own projection that tears the merged ones back down. `GET /branches/{id}/compare` still works on a
merged branch because it reads the event log, not the overlay. This extends BR-003's existing
purge-on-`BranchDeleted` behavior to the other terminal status.

**Per-entity resolutions do not compose with cross-entity references.** Resolutions are keyed by
aggregate, but the branch's events reference each other *across* aggregates: `ChildLinkedToFamily`
lives on the family's stream and names a person on another, as do the partners a `FamilyCreated`
names or a `FamilyUpdated` sets, and so (since #757 made associations branch-writable) does
`AssociationCreated`, which names two. Excluding a person — which a
`main`-deleted conflict *forces*, since `main` is then the only offered resolution — therefore does
not exclude the family event that links them, and the projection writes that row unconditionally
(the branch-scoping work dropped the FK cascade that would once have caught it). Left alone, this
returns a successful merge while `main` gains a family child (or partner) pointing at a person it
does not have.
The merge refuses instead (`ErrMergeDanglingReference`, `409 merge_dangling_reference`), checked
before the claim: a replayed link or association must name a person `main` already has or that the
replay itself will create — a stream that only edits the person does not count, since `main` can
merge a person away without writing to their stream, which leaves the branch's edits conflict-free
but the person gone. Dropping the link silently was rejected as the same class of defect per-conflict
review exists to prevent. Unlink events, and a `FamilyUpdated` that clears a partner, are not checked — removing a person
`main` lacks is a no-op.

#758 put sources and citations on the allowlist, which opened two more shapes of the same class,
now refused by the same check (`validateNoDanglingEvidence`). A citation lives on its own stream
and names a source on another. (1) A replayed citation whose *final* source — the last one a
`CitationCreated` or a `source_id` change in `CitationUpdated` set, unless the stream ends deleted
— must exist on `main` or be created and not deleted by the replay; otherwise `main` would gain a
citation of a source it does not have, with a blank title. (2) A replayed `SourceDeleted` is refused
while `main` has a citation of that source that the replay does not itself delete or re-point
elsewhere. The branch's own delete guard (`ErrSourceHasCitations`) only sees the branch's view, so a
citation `main` added after the fork would otherwise be deleted from `main` by the source→citation
cascade, with no `CitationDeleted` event and no conflict shown.

#759 put `MediaCreated` on the allowlist, which opened a third shape: a branch upload names its owner
(a person, family or source) on another stream, and the projection saves the media row without
checking that owner. A replayed upload whose stream does not end deleted is refused unless its owner
exists on `main` or is replayed — and a replayed owner that the replay itself deletes counts only
when its stream replays *after* the upload, so the owner's delete cascades the item on `main` as it
did on the branch. Otherwise `main` would gain a media item attached to nothing. A branch that
uploads to a person it created and then deletes that person is therefore refused; deleting the
media first makes it mergeable. `ResumeMerge` applies the same rule with its pending semantics
(see *Media on resume* under the merge implementation note).

**The claim is idempotent against its own interrupted attempt.** The claim's append is durable
before the projection that flips the registry status, so a projection failure leaves a branch that
is claimed in the log but still reads `active`. Because the CAS keys on the *stream version*, a
retry trusting only the registry would observe the already-incremented version, append a second
`BranchMerged`, and replay the whole branch onto `main` again. `claimMerge` therefore consults the
log — the only place a half-completed claim is visible — before appending: an existing
`BranchMerged` means the branch is claimed, so the command re-projects it to repair the registry
and then refuses. Refusing rather than silently continuing is deliberate: a second *merge* would
re-plan against a `main` the first attempt may already have written to. Finishing the first
attempt is `ResumeMerge`'s job, which works from the plan its claim recorded.

**Scan truncation is reported per side.** `branch_too_large` and `main_too_far_ahead` are distinct
refusals because they have distinct causes and remedies. A branch bigger than the cap is a
permanent property of that branch. A *mainline* tail bigger than the cap grows with unrelated
activity since the fork and says nothing about branch size — a three-event branch can trip it — so
reporting that as "your branch is too large" sends the user after a fix that does not exist.

**Not every conflict accepts both resolutions.** §Conflict definition says a conflict "requires
review", which implies a genuine choice; for two of the three classes the "branch wins" side of
that choice cannot be honored, so the implementation refuses it rather than appearing to accept it:

- **`delete_edit` where `main` is the deleter.** Replaying the branch's `*Updated` events onto
  `main` cannot resurrect the entity — the `*Updated` projections skip an absent read-model row,
  and no undelete event exists in the domain. Accepting `branch` would return a success with a
  non-zero replayed-event count while `main` stayed deleted, i.e. exactly the "silently discard"
  outcome this section exists to prevent.
- **`create_create`.** The two sides are different streams by construction, so promoting the
  branch's entity leaves `main`'s beside it — the duplicate the class exists to detect.

Each conflict therefore carries the resolutions it can actually honor
(`MergeConflict.SupportedResolutions`), a resolution outside that set is a `400`, and a review UI
can offer only the meaningful choices. Resurrecting a `main`-deleted entity would need an undelete
event; that is not in scope here.

**Conflict detection compares whatever a branch can write.** The classifier folds each event into
the net effect its side asserted, and the fold is keyed by event *shape*, not entity type:
aggregate deletes by the `*Deleted` suffix, child links by the relationship they assert, person
names per-name (`names[<name-id>]`), and everything else by its `Changes` map. The suffix rule
alone is not sufficient — `NameUpdated` ends in "Updated" but carries its fields flat with no
`Changes` map, so folding it that way read nothing and made two divergent renames look like
agreement. Because a gap here is silent (no conflict reported, branch edit promoted over main's
without review), the coupling is enforced by a test rather than by convention: every event type in
`command.BranchAwareEventTypes` must be comparable, or be listed with a reason why it needs no
comparison.

**Partial merge stays deferred**, consistent with §Merge. The delivered API takes per-aggregate
*resolutions* (`branch` or `main`), which lets a caller settle a conflict or exclude a whole
entity, but there is no way to promote a subset of one aggregate's changes and there is no status
that expresses a partially-merged branch — `merged` is terminal. Cherry-pick is tracked as
follow-up work (#684).

## Implementation Note — browse and map aggregates (#676 sub-issue A, #756, delivered)

**Aggregates are branch-aware without owning a `branch_id`.** The surname index, per-surname person
list, place hierarchy, per-place person list, per-cemetery person list and map locations are all
*derived* views: each is computed over the seven-type read-model slice, which already carries the
`branch_id` overlay of §The model. Scoping them therefore needed no new column, no new tombstone
rule and no second resolution path — the aggregate query resolves the overlay exactly once, in the
same set-based statement the Implementation Notes above demand, and the branch's shadow rows and
tombstones fall out of the count for free. This is the general shape for the rest of #676:
**a view that reads only branch-aware tables inherits branch-awareness; only a table that stores its
own rows needs its own `branch_id`.**

The API surface is six `GET` operations carrying `?branch=` (`browseSurnames`,
`getPersonsBySurname`, `browsePlaces`, `getPersonsByPlace`, `getPersonsByCemetery`,
`getMapLocations`), bringing the total to 22. Omitting the parameter is byte-identical to the
previous mainline behaviour.

Two browse surfaces stayed main-only in this pass, and said so in the UI via
`MainlineNotice.svelte`:

- **The cemetery *index*** (`browseCemeteries`) aggregates the `life_events` table, which had no
  `branch_id` yet. Sub-issue B ([#757](https://github.com/cacack/my-family/issues/757)) has since
  given it one — see the next note.
- **Brick walls** (`getBrickWalls`, `setPersonBrickWall`, `resolvePersonBrickWall`) are not
  event-sourced, so there is no branch-tagged event for BR-006 to allow and nothing for a merge to
  replay. This pass fixed only the mainline leak; the scoping question is recorded in
  *Entities that stay main-only* above.

**#676 does not eventually cover every entity.** Submitter, Repository, RepositoryExternalID and
LDSOrdinance are permanently main-only — see *Entities that stay main-only* above for the set and
the reasoning.

**A caveat on place accuracy, not on branch scoping.** `GetPlaceHierarchy` parses place strings
differently on SQLite and PostgreSQL — a pre-existing divergence, tracked as
[#763](https://github.com/cacack/my-family/issues/763) and untouched here. Branch overlay resolution
for the place views has cross-backend parity (`TestBranchScenario_AggregateIsolation` runs the same
scenario on all three backends); the *place parsing underneath it* does not yet.

## Implementation Note — person/family facts (#676 sub-issue B, #757, delivered)

**Life events, attributes and associations own their own `branch_id`.** Unlike the aggregates they
store rows, so each of `life_events`, `attributes` and `associations` gained the full §The model
treatment on all three backends: a `branch_id` column, a composite `(id, branch_id)` primary key, a
`branch_id`-leading index, a `deleted` tombstone, one set-based overlay query per read with a
main-scope fast path, and a place in `PurgeBranch`. Their `ReadModelStore` methods take the scope the
slice's do (an explicit `domain.BranchID`, or `ListOptions.BranchID` for the paged lists), the nine
projection handlers write only branch-keyed rows, and the nine event types are on the BR-006
allowlist. Each `*Created` is conflict-blind for the same reason `PersonCreated` is — a fact is its
own aggregate, so its create opens a stream main cannot have touched — while `*Updated` and
`*Deleted` fold into the merge conflict scan like any other entity.

**Per-id overlay, including for the per-owner lists.** `ListEventsForPerson`, `ListEventsForFamily`,
`ListAttributesForPerson` and `ListAssociationsForPerson` resolve every id the owner has on either
side through the same per-id overlay as the single-row reads, rather than copying the owner's whole
bucket forward the way external identifiers do. The facts have stable ids of their own, so a branch
that edits one fact still sees main's later additions to the same person, exactly as it sees main's
later persons. The owner filter is applied twice: once to pick the candidate ids and once to the
*winning* row, so a branch row that re-owned a fact (the shape `PersonMerged` writes) lists under
its new owner only.

**The manual cascade grew.** `DeletePerson` removes (on main) or tombstones (on a branch) the
person's life events and attributes and every association naming the person on either side;
`DeleteFamily` does the same for the family's life events. On main this closes a pre-existing gap:
a deleted person's life events used to survive as orphans, so the cemetery index kept counting
them. Cross-backend parity is pinned by `TestReadModelStore_*Cascade*` and
`TestBranchScenario_FactOverlay`.

**The cemetery pair is now fully scoped.** `browseCemeteries` carries `?branch=` and
`GetPersonsByCemetery` joins the `life_events` overlay to the `persons` overlay, so the index and
its click-through list agree on every scope and the UI dropped the cemetery `MainlineNotice`. The
association endpoints (`listAssociations`, `createAssociation`, `getAssociation`,
`updateAssociation`, `deleteAssociation`, `listAssociationsForPerson`) carry `?branch=` too,
bringing the total to 29. Life events and attributes have no endpoints of their own beyond the
mainline bulk exports, which stay mainline.

**Upgrading an existing database.** PostgreSQL migrates the three tables in place (the same
per-table primary-key swap #669 used). SQLite cannot alter a primary key, so a database created
before #757 keeps lone-id keys on these tables; `detectBranchCapable` now requires the composite key
on `life_events`, `attributes` and `associations` as well as `persons`, and such a database refuses
*every* branch write with `ErrBranchesUnsupported` until the read model is rebuilt (#680). Refusing
only fact writes would be worse: a branch `DeletePerson` has to tombstone the person's facts, so a
half-capable schema would accept branches it could not delete cleanly. The one exception is
`PurgeBranch`: a database built between #669 and #757 may already hold branches in its slice tables,
so purging is never refused — deleting or merging such a branch still drops its overlay rows.

## Implementation Note — evidence (#676 sub-issue C, #758, delivered)

**Sources, source external IDs, citations and notes own their own `branch_id`.** `sources`,
`citations` and `notes` get the same treatment as the person/family facts — composite `(id,
branch_id)` key, `branch_id`-leading index, `deleted` tombstone, one set-based overlay read with a
main-scope fast path, a place in `PurgeBranch` — and `source_external_ids` follows the
`person_external_ids` precedent: a per-source bucket keyed `(source_id, sequence, branch_id)`, where
a present branch bucket wins wholesale and an empty-marker row is its tombstone. The nine
`Source*`, `Citation*` and `Note*` event types are on the BR-006 allowlist; each `*Created` is
conflict-blind like the facts' creates, because every source, citation and note is its own
aggregate.

**Source before citation.** A citation row carries a denormalized source title, and its source row
carries a citation count. Both are derived in the projector, so both must resolve through the
branch the event is projected on: `projectCitationCreated`, `projectCitationUpdated` (when a
citation is re-pointed at another source) and `projectCitationDeleted` read the source with the
branch-scoped `GetSource` and write the count back on the same branch. A citation created on a
branch that retitled its source therefore carries the branch's title, not main's, and bumping the
count forks the source onto the branch rather than touching main's row. On merge these are simply
re-derived: the replayed `CitationCreated` bumps main's count and denormalizes main's title at that
point in the log.

*Merge replays sources around citations, not in first-touch order.* The replay is one append per
stream, so re-deriving only works if each citation stream lands while the sources it names are in
the right state on main. `orderEvidenceForReplay` puts every surviving source stream first (a
citation re-pointed at a source the branch created later still finds it, with its title and count)
and every source stream that ends in `SourceDeleted` last (the store's source→citation cascade then
runs only after every replayed citation has moved off or been deleted). All other streams keep
their first-touch order. Sources reference no other replayed aggregate, so the move is safe.

*Known consequence — a stale source view on the branch.* That fork is a side effect of creating,
re-pointing or deleting a *citation*, not of editing the source, yet it writes a full copy-on-write
row of the source (title, author, repository, …) on the branch, and a branch row always wins the
overlay. From then on the branch no longer sees main's later edits to that source: cite main's
"1880 Census" on a branch, retitle it on main, and the branch still shows the old title. No
`SourceUpdated` exists on the branch, so neither compare nor merge reports a divergence, and a merge
is unaffected (it replays events, and the replayed citation re-derives main's count and title). Persons and families fork only on an explicit edit;
sources can fork implicitly. The fix is to stop materialising `citation_count` and derive it at read
time from the resolved citations overlay, so a citation write never touches the source row — tracked as
[#815](https://github.com/cacack/my-family/issues/815) because it changes the source read path on all three backends.

**`SearchSources` resolves before it matches.** The title/author match runs over the resolved view
of every source (the overlay subquery), never over raw rows filtered afterwards, so a source whose
branch shadow and main row both match is returned once, a branch retitle is found only under its new
title, and a branch-deleted source never matches.

**The cascade replaces two foreign keys.** `citations.source_id` and `source_external_ids.source_id`
referenced `sources(id)`, which stops being unique once sources join the overlay, so both foreign
keys are dropped (on PostgreSQL by the migration, before the primary-key swap). `DeleteSource` now
deletes (main) or tombstones (branch) the source's external-ID bucket and every citation of it that
the branch sees, on that branch only — a sibling branch's own citation of the same source is never
touched, and neither is main's row when a branch deletes. The command layer still refuses to delete
a source that has citations, judged by the branch's view, so the cascade is the store's own
integrity guarantee rather than a user-visible behaviour change. Parity is pinned by
`TestReadModelStore_DeleteSourceCascade` and `TestBranchScenario_EvidenceOverlay`.

**API and UI.** Eighteen operations gained `?branch=` — the source, citation and note CRUD, source
search, the per-source and per-person citation lists and `formatCitation` — bringing the total to
47; the sources pages dropped their `MainlineNotice`. Source and citation history, restore points
and rollback stay mainline, as rollback does for every entity, and so do the GEDCOM exporter and
the bulk `/export/sources` and `/export/citations` endpoints.

**Upgrading an existing database.** PostgreSQL migrates the four tables in place. SQLite cannot
alter a primary key, so `detectBranchCapable` now also requires `branch_id` in the primary key of
`sources`, `source_external_ids`, `citations` and `notes`; a database created before #758 refuses
every branch write with `ErrBranchesUnsupported` until the read model is rebuilt (#680), exactly as
a pre-#757 one does. The check now looks for `branch_id` in the key rather than for a multi-column
key, because `source_external_ids` was already keyed by the composite `(source_id, sequence)`.

## Implementation Note — media metadata (#676 sub-issue D, #759, delivered)

**Only the metadata forks; the file bytes are shared, never copied per branch.** A branch stores
deltas whose cost scales with what it changes, so retitling a photo on a branch must not become a
multi-megabyte write. `media` therefore gets the usual overlay — composite `(id, branch_id)` key,
`branch_id`-leading index, `deleted` tombstone, one set-based overlay read with a main-scope fast
path, a place in `PurgeBranch`, and the six `Media*` store methods threaded with the branch scope —
but the two byte columns, `file_data` and `thumbnail_data`, follow their own rule.

**The decision: blobs stay on the media row, NULL on a shadow, read through a fallback.** Two
designs were on the table: keep the byte columns on `media` and leave them NULL on a branch shadow
row, or split them into a separate, deliberately branch-less `media_blobs` table keyed by media id.
The first was chosen because it keeps the invariant both simpler and directly testable:

- The bytes are written exactly once, by `MediaCreated`, onto the row that created the item — its
  *origin row*: main for a mainline upload, the branch's own row for an item uploaded on a branch
  (a new id, which owns its bytes). No later event carries bytes: `MediaUpdated` edits metadata
  only, and `projectMediaUpdated` reads the item with `GetMedia`, never `GetMediaWithData`.
- A branch shadow row of an item that has a main row is **metadata only** — its byte columns are
  NULL. `SaveMedia` enforces this in the statement itself: on a non-main branch, an id with a main
  row gets NULL bytes whatever the caller passes; and no save ever clears bytes already stored
  (nil means "keep").
- `GetMedia` and `ListMediaForEntity` never read the byte columns (their overlay projects an
  explicit metadata column list, so even the SQLite `ROW_NUMBER` window never carries a blob).
  `GetMediaWithData` and `GetMediaThumbnail` resolve the winning row by `(id, branch_id)` alone,
  then take the bytes from the winning row, else from main's row. A tombstone therefore hides the
  bytes too.

The rule is one sentence — *a shadow row's byte columns are NULL* — so the proof is one assertion
on the stored row, made in `TestBranchScenario_MediaOverlay` on all three backends after a branch
metadata edit (and after a branch `SaveMedia` handed the full record, bytes included), alongside
`GetMediaWithData` / `GetMediaThumbnail` returning main's bytes on that branch. A `media_blobs`
split would have needed a second table, a larger migration (a data move on PostgreSQL and a table
rebuild on SQLite), its own garbage collection, and a pre-#759 SQLite database could no longer serve
mainline media at all until rebuilt.

**Deletes never lose shared bytes.** A branch delete (and a branch cascade) writes a metadata-only
tombstone on the branch and never touches main's row. The one case that needed a rule of its own is
a *mainline* delete of an item that some branch still shows through a live shadow: hard-deleting
main's row would leave that shadow with metadata and no file. Instead main's row is kept as a
tombstone — hidden from main and from every branch without a row of its own, since a winning
tombstone hides the id — and is dropped once no live shadow needs it: when the last such branch
deletes the item, or when `PurgeBranch` purges it. A main delete with no live shadow is a plain
removal. `TestReadModelStore_DeleteCascadesMedia` and the scenario pin this on all three backends.

**The cascade is new, not a replacement.** Media never had a foreign key to its owner (the owner is
polymorphic: person, family or source), so deleting a person used to leave its media rows behind.
`DeletePerson`, `DeleteFamily` and `DeleteSource` now delete (main) or tombstone (branch) the
owner's media on the same branch, under the rule above. `ListMediaForEntity` decides the owner on
each item's *winning* row, like the life-event lists, so a branch that re-links an item
(`PersonMerged` moves a merged person's media to the survivor) lists it under its new owner only.

**BR-006 and merge.** `MediaCreated`, `MediaUpdated` and `MediaDeleted` join the allowlist;
`MediaCreated` is conflict-blind like the other per-entity creates. A merge interrupted mid-replay
resumes media streams like any other (#685): the media-owner rule applies with resume's pending
semantics, and the read-model repair never copies or drops shared bytes — see *Media on resume*.
`PersonMerged` stays off the allowlist: its media transfer is now branch-scoped, but it still
rewrites evidence-analysis and research rows that are main-only until #760.

**API and UI.** Seven operations gained `?branch=` — `getMedia`, `updateMedia`, `deleteMedia`,
`listPersonMedia`, `uploadPersonMedia`, `downloadMedia` and `getMediaThumbnail` — bringing the total
to 54. The content and thumbnail reads take the scope although the bytes are shared, because a
branch-deleted item must be not-found there too; the client's URL builders for `<img src>` and the
multipart upload, which bypass the request helper, append the scope themselves. Media history and
rollback stay mainline, as rollback does for every entity.

**Upgrading an existing database.** PostgreSQL migrates `media` in place and drops `NOT NULL` from
`file_data`. SQLite cannot alter either, so `detectBranchCapable` also requires `branch_id` in the
primary key of `media`; a database created before #759 refuses every branch write with
`ErrBranchesUnsupported` until the read model is rebuilt (#680). Its mainline media keeps working —
a metadata-only save supplies the row's own stored bytes, so the legacy `NOT NULL` never trips.

## References

- [ADR-001: Event Sourcing with CQRS-lite](./001-event-sourcing-cqrs.md)
- [ADR-003: Synchronous Projections for MVP](./003-synchronous-projections.md)
- [ARCHITECTURAL-INVARIANTS.md](../ARCHITECTURAL-INVARIANTS.md)
- [ETHOS.md - Git-Inspired Workflow](../ETHOS.md)
- Epic #54 (git-inspired research workflow); depends: #669, #670, #55; coordinates: #624, #680
