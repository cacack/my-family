# ADR-006: Schema Evolution — Read-Model Rebuild and Event Upcasting

**Status:** Accepted
**Date:** 2026-11-12
**Decision Makers:** Project maintainer
**Related Features:** [#680](https://github.com/cacack/my-family/issues/680)

## Context

Each backend creates its read-model schema at startup with `CREATE TABLE IF NOT EXISTS` and a
best-effort `ALTER TABLE ... ADD COLUMN` pass. That covers additive columns and nothing else: it
cannot change a primary key (SQLite cannot alter one in place at all), a constraint or a column
type, drop a column, or re-derive rows whose projection changed. #669 was the first change to need
one of these, a composite `(id, branch_id)` key, and shipped a detect-and-refuse guard
(`ErrBranchesUnsupported`) instead of a general answer. Every entity that became branch-scoped
since (#676) repeated it.

The event log has the matching gap. [ADR-001](./001-event-sourcing-cqrs.md) listed "event schema
changes require migration strategy (versioned event types)" as a consequence, never implemented.
Events are JSON, which tolerates added fields, but a rename, removal or change of meaning had no
story.

The read model is derived from the log (ES-004), and `myfamily rebuild` (#845) already replays the
whole log into it, with the exclusive lock of #1232.

## Decision Drivers

- The event log is the source of truth and append-only (ES-002); it is never rewritten.
- The read model is derived, so dropping and re-projecting it loses nothing the log holds.
- A server must never run against a read model whose schema it cannot read or write.
- A rebuild needs every server stopped, so it cannot happen behind an operator's back.
- Both SQL backends must behave the same (DB-001).

## Considered Options

### Sub-decision 1 — Read-model schema changes

#### Option 1A: In-place migrations per change

**Pros:** no rebuild for any change.
**Cons:** one hand-written migration per change and backend; SQLite needs a table copy for every
key change; the read model is derived data, so this is work a replay does for free.

#### Option 1B: Version stamp, rebuild on mismatch, automatic

**Pros:** no operator step.
**Cons:** a rebuild needs every server stopped and can take a long time on a large tree; starting
one silently from `serve` hides a long outage and races other servers on PostgreSQL.

#### Option 1C: Version stamp, fail fast on mismatch, operator runs `myfamily rebuild`

**Pros:** one mechanism for every non-additive change; the operator decides when; the rebuild that
already exists does the work.
**Cons:** one manual step per schema bump.

### Sub-decision 2 — Event schema changes

#### Option 2A: Versioned event type names (`PersonUpdated.v2`)

**Cons:** every consumer (projection, history catalog, merge and conflict classifiers, decode)
grows a case per version, forever.

#### Option 2B: Envelope schema-version field

**Cons:** a column on every event in both stores, needed only when shapes cannot be told apart.

#### Option 2C: Upcasting on decode

**Pros:** old shapes are upgraded in one place, `DecodeEvent`, before any consumer sees them; the
type name and every consumer stay as they are.
**Cons:** an upcaster must recognise the old shape from the data alone.

## Decision

**Read model: Option 1C.**

- `repository.ReadModelSchemaVersion` is the schema the binary projects into, and each database
  stamps the version its read-model tables were created at (table `read_model_schema`, beside
  `rebuild_state`; both survive the reset).
- `storage.Open` for `serve` (`AccessServe`) runs `repository.CheckReadModelSchema`. A mismatch
  fails start-up with `ErrReadModelSchemaMismatch`, naming both versions and `myfamily rebuild`.
  It never rebuilds.
- A database with no stamp and an empty event log is new and is stamped. One with no stamp but with
  events predates the stamp (version 0), so it is a mismatch: every existing install rebuilds once.
- `ResetReadModel` drops and re-creates every read-model table (it used to delete rows only), and
  `RebuildReadModel` stamps the version after the reset. A rebuild therefore applies any schema
  change, including the composite keys a pre-#669 SQLite database lacks.
- Bump the version for any change an additive `ALTER TABLE ADD COLUMN` cannot apply: a key,
  constraint or type, a dropped column, or a change to what an existing event projects (as with
  #1203's BC sort dates). Additive columns stay on the `ALTER TABLE ADD COLUMN` pass, with no bump.

**Events: Option 2C.** `DecodeEvent` runs the event type's upcaster from `eventUpcasters`, if any,
on the stored JSON before decoding. Event type names are never versioned. An upcaster must return
data already in the current shape unchanged. Add an envelope version only when an upcaster cannot
tell two shapes apart. An added field needs no upcaster: it decodes to its zero value.

**Snapshots from before #624.** They exist only in the snapshot registry. The rebuild logs a
`SnapshotCreated` for each snapshot whose stream is empty, with its original creation time, before
the reset, so the log alone rebuilds the registry. This follows the backfill pattern of
`ImportedValuesBackfilled` (#1251) and `BrickWallBackfilled` (#802). It reuses `SnapshotCreated`
rather than a dedicated backfill type: snapshot events are already kept off branches and out of
history, so nothing would read the difference.

## Consequences

### Positive

- A read-model schema change is a version bump plus a rebuild, not a migration per backend.
- The branch-scoping refusal for a pre-#669 SQLite database now has a fix: `myfamily rebuild`.
- The forced first rebuild also re-projects the BC sort dates from before #1203.
- Event shapes can evolve without rewriting the log or touching consumers.

### Negative

- Every existing install must stop and run `myfamily rebuild` once on upgrade, and again on each
  later bump.
  - Mitigation: the refusal says exactly what to run; the README documents it.
- An upcaster that misreads a shape corrupts every replay of that type.
  - Mitigation: upcasters are unit-tested on both shapes; an envelope version is the fallback.

### Neutral

- The in-memory (demo) backend is never checked: its data does not outlive the process.
- The rebuild is not checked either, since it is what repairs a mismatch.

## References

- [ADR-001](./001-event-sourcing-cqrs.md) — event sourcing; ES-002, ES-004, ES-007
- [ADR-002](./002-dual-database-strategy.md) — both backends, one database (DB-006)
- [ADR-005](./005-research-branch-data-model.md) — composite keys (#669) and snapshots (#624)
- `internal/repository/schema.go`, `internal/repository/rebuild.go`, `DecodeEvent` in
  `internal/repository/eventstore.go`
