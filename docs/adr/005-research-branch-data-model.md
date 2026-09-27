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
  the remaining sub-issues of [#676](https://github.com/cacack/my-family/issues/676): evidence
  ([#758](https://github.com/cacack/my-family/issues/758)), media metadata
  ([#759](https://github.com/cacack/my-family/issues/759)) and GPS artifacts
  ([#760](https://github.com/cacack/my-family/issues/760)). Snapshots are pending too, though not
  as a #676 sub-issue: #624 made them event-sourced, and what remains is giving the registry a
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
residual staleness window below; a pre-#685 claim with no plan; or a replay that would now leave
`main` referencing a person it no longer has) resume refuses with
`ErrMergeResumeNeedsResolution` (`409 merge_resume_needs_resolution`), **writing nothing**, and
lists the streams. The caller reviews them with `compare` and resumes again with a resolution per
listed stream: `branch` replays over `main` as it now stands (asserting *that* version, so a
further write still trips the guard), `main` leaves the entity as `main` has it — the deliberate
roll-forward. A resolution for any stream the claim or an earlier resume already decided is a
`400`: a second request must not quietly re-decide what the first one reviewed. That rule needs the
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
caller, counting as present every stream already on `main` or still to be replayed — minus
persons this very request resolves to `main` — and lists any stream that fails it as pending. A
`main` resolution rolls the merge forward without it, recorded like any other decision; `branch`
is refused as a dangling reference. The check then runs once more over the final decision, and
also over the streams *already* on `main`: a `main` resolution may not exclude a person a landed
stream references and `main` lacks (reachable only from a pre-#685 claim, where the branch created
both the family and the child and only the family landed). Only this request's own resolutions are
checked against landed streams; a person `main` itself removed later is `main`'s change, not the
resume's to refuse. Every refusal — pending, dangling, concurrent — comes before the first write,
including the read-model repair below, so a `409` has written nothing at all.

A claim written before #685 has no plan, so its first resume must decide every stream not yet on
`main` — including, for a merge that in fact finished with claim-time `main` resolutions, streams
the original request already declined. The log cannot distinguish those from unreplayed ones, so
the operator should check `compare` before answering; once answered, the answer is final like any
other.

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
since #757 — association streams) writes the
aggregate's version as its last step, so a row behind the log is re-projected from the first event
past its version; re-running an event whose projection stopped midway is safe because its writes are
upserts and deletes, and the counter it bumps is part of the final write that did not happen. A
missing row is re-projected from the start unless the log explains its absence (the stream ends in a
delete; `main` merged the person away with `PersonMerged`, which writes nothing to the merged
person's stream; or an association's person is gone from `main`, whose delete cascade removes the
row without writing to the association's stream). Repaired streams are reported in
`reprojected_stream_ids`; a resume after that finds nothing behind.

The repair takes no lock and appends nothing, so it can run alongside another resume's repair or a
mainline write to the same entity. Most projections set the row's version outright, so re-running
an *old* event after a newer one was projected would roll the row back — version and fields. The
repair therefore never applies an event the row already reflects: it re-reads the row before each
event and skips any at or below the row's version. That leaves only the instant between that read
and the projection's own save, so before reporting a stream repaired it reads the row and `main`'s
stream version once more and, if a racing write slipped into that instant and was rolled back,
re-projects the missing tail from the log (a bounded number of times, then `500
merge_partially_applied`: resume again). A row that disappears during the repair is not re-created;
the next resume classifies it. The dangling-reference check also counts already-replayed streams as present,
since their events are on `main` in the log. The fault-injection
coverage is `internal/command/branch_merge_resume_test.go` and
`internal/command/branch_merge_resume_refs_test.go` (memory: dangling references, racing repairs,
associations) and `internal/integration/branch_merge_resume_test.go`, which drives an interrupted
merge and its resume over HTTP against memory, SQLite and PostgreSQL — including a resume-time
`main` decision that a later resume must neither re-ask nor reverse, a family whose child `main`
deleted after the interruption, and a replay whose projection failed after its append committed.

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
lives on the family's stream and names a person on another, and so (since #757 made associations
branch-writable) does `AssociationCreated`, which names two. Excluding a person — which a
`main`-deleted conflict *forces*, since `main` is then the only offered resolution — therefore does
not exclude the family event that links them, and the projection writes that row unconditionally
(the branch-scoping work dropped the FK cascade that would once have caught it). Left alone, this
returns a successful merge while `main` gains a family child pointing at a person it does not have.
The merge refuses instead (`ErrMergeDanglingReference`, `409 merge_dangling_reference`), checked
before the claim: a replayed link or association must name a person `main` already has or that the
replay itself will create. Dropping the link silently was rejected as the same class of defect per-conflict
review exists to prevent. Unlink events are not checked — removing a person `main` lacks is a no-op.

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

## References

- [ADR-001: Event Sourcing with CQRS-lite](./001-event-sourcing-cqrs.md)
- [ADR-003: Synchronous Projections for MVP](./003-synchronous-projections.md)
- [ARCHITECTURAL-INVARIANTS.md](../ARCHITECTURAL-INVARIANTS.md)
- [ETHOS.md - Git-Inspired Workflow](../ETHOS.md)
- Epic #54 (git-inspired research workflow); depends: #669, #670, #55; coordinates: #624, #680
