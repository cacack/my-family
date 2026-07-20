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

All three satisfy the existing `Event` interface (ES-005) and must be added to `DecodeEvent()`
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

This ADR does **not** change how snapshots are created. However, it surfaces a coupling that
**#624** must resolve: `SnapshotCreated` exists and decodes (ES-007) but is never emitted —
`SnapshotService.CreateSnapshot` writes directly to the `SnapshotStore`, bypassing the
event-sourced pipeline. **Recommendation for #624 (not implemented here):** route snapshot
creation/deletion through the event pipeline (emit `SnapshotCreated`, add a projection) so
snapshots carry the same audit-trail guarantee (ADR-001) as every other mutation, and so a
branch-scoped snapshot is expressible as a branch-tagged event. #624 remains the issue that
implements this decision.

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

This ADR introduces the **Branch (BR)** invariant category — **BR-001 through BR-004**, covering
`branch_id` tagging with a reserved `main`, append-only branch events on the shared log,
`branch_id` read-model rows with copy-on-write overlay + tombstones, and non-rewriting merges.
Their canonical text and verification methods live in
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

## References

- [ADR-001: Event Sourcing with CQRS-lite](./001-event-sourcing-cqrs.md)
- [ADR-003: Synchronous Projections for MVP](./003-synchronous-projections.md)
- [ARCHITECTURAL-INVARIANTS.md](../ARCHITECTURAL-INVARIANTS.md)
- [ETHOS.md - Git-Inspired Workflow](../ETHOS.md)
- Epic #54 (git-inspired research workflow); depends: #669, #670, #55; coordinates: #624
