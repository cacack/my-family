# ADR-011: Schema Migrations for the Tables Outside the Rebuild

**Status:** Accepted
**Date:** 2026-11-24
**Decision Makers:** Project maintainer
**Related Features:** [#1803](https://github.com/cacack/my-family/issues/1803) (this decision),
[#1816](https://github.com/cacack/my-family/issues/1816) (the runner)

## Context

[ADR-006](./006-schema-evolution.md) changes the read model by stamping a version and rebuilding
it from the log, and changes event shapes by upcasting on decode. Neither covers the tables
`myfamily rebuild` leaves alone. Some of them, accounts and settings, hold data that exists
nowhere else, so a rebuild cannot repair them and a lost row is lost for good.

Until now a schema change to these tables could assume an empty or disposable database. The owner
plans to host a real tree before 1.0, so the first non-additive change to an account or settings
table needs a path that keeps a deployment's users.

What exists today:

- Each store creates its tables at start-up with `CREATE TABLE IF NOT EXISTS`. The account and
  settings tables have never changed shape on either backend.
- The `events` table changed twice by one-off migration in the event store's constructor: an
  added `branch_id` column and a composite version key, which on SQLite is a full table copy
  (`migrateBranchID`, `migrateCompositeVersionKey` in `internal/repository/sqlite/eventstore.go`).
- The branch registry adds columns at start-up. SQLite discards every `ALTER TABLE` result on the
  reasoning that a duplicate column is the only possible failure
  (`internal/repository/sqlite/branches.go`); PostgreSQL uses `ADD COLUMN IF NOT EXISTS`.
- None of these tables carries a version.

## Decision Drivers

- Data held nowhere else must never be lost or left half-changed by an upgrade.
- Both backends behave the same (DB-001), from the same list of changes.
- The event log is append-only and is never rewritten (ES-002); a change to its shape keeps every
  row, position and id.
- A self-hoster upgrades by replacing the binary; anything more is a documented step.
- No new dependency for what a short loop can do.

## Considered Options

### Sub-decision 1 — How these tables change shape

#### Option 1A: Additive changes only

New columns and tables, with defaults, by `ADD COLUMN` at start-up, as the read model does.

**Pros:** no mechanism.
**Cons:** no answer for the change that cannot follow the rule: a rename, a type or key change, a
split table, a backfill. The first one would ship a one-off migration like the `events` ones,
which is the gap this decision closes. A database's shape cannot be read from anything, so a
check has to probe columns.

#### Option 1B: Numbered migrations with a version stamp, hand-written runner (chosen)

One ordered list of migrations per backend, the same names in the same order, and a stamp that
records the highest one applied. A small runner applies the ones after the stamp.

**Pros:** any change has a path, including the ones 1A cannot make; the database says what shape
it is in, so older and newer databases can be told apart; each change is written and tested once.
**Cons:** a runner and a stamp table to maintain; each change is written twice, once per backend.

#### Option 1C: A migration library (golang-migrate, goose)

**Pros:** a known tool.
**Cons:** a dependency with its own table, locking and file layout, for a loop of a few dozen
lines; still one set of migrations per dialect; it does not fit stores that create their own tables
in their constructors, or SQLite migrations that must switch foreign keys off outside the
transaction.

### Sub-decision 2 — The event-log tables

#### Option 2A: `events` and `streams` use the same migrations (chosen)

**Pros:** one mechanism for every table outside the rebuild; the next change to the log gets the
same transaction, stamp and refusal.
**Cons:** the existing one-off migrations must move into the baseline.

#### Option 2B: Keep hand-written one-off migrations for the log

**Cons:** two mechanisms; a one-off migration has no stamp, so it must detect its own state on
every start, forever.

### Sub-decision 3 — Start-up against an older or newer database

#### Option 3A: Migrate an older database automatically; refuse a newer one (chosen)

**Pros:** an upgrade is replacing the binary. Migrations run in a transaction and touch a few
small tables, so they are quick, unlike a rebuild, and need no operator decision on timing.
**Cons:** an operator who skipped the backup has no undo.

#### Option 3B: Refuse an older database and ask for `myfamily migrate`, as ADR-006 does for a rebuild

**Pros:** the operator chooses the moment.
**Cons:** a manual step on every upgrade with nothing to decide; ADR-006 refuses because a rebuild
is long and needs every server stopped, which a migration is not.

### Sub-decision 4 — A failed change at start-up

#### Option 4A: Never ignored (chosen)

#### Option 4B: Ignore the expected failure, as SQLite's `ADD COLUMN` pass does

**Cons:** "the only possible failure" is an assumption; a locked database, a full disk or a typo
reads the same, and the server then runs against a shape it cannot use.

## Decision

### 1. The tables outside the rebuild

Every table a backend creates is either a read-model table, dropped and re-created by
`ResetReadModel` (`branchScopedTables` and `mainOnlyTables`, the same set on both backends), or
one of these sixteen, which it does not drop. Checked against both backends:

| Table | Rebuilt from the log | Schema changes by |
|---|---|---|
| `events`, `streams` | They are the log | This ADR's migrations |
| `users`, `tree_memberships`, `sessions`, `api_tokens`, `invites`, `password_resets`, `auth_audit` | No: accounts ([ADR-007](./007-access-model.md), AC-001) | This ADR's migrations |
| `site_settings`, `tree_settings`, `account_settings` | No: settings are never in the log | This ADR's migrations |
| `branches` (branch registry) | Yes, from the branch lifecycle events | ADR-006, see below |
| `snapshots` (snapshot registry) | Yes, from `SnapshotCreated`/`SnapshotDeleted`; the rebuild logs pre-#624 snapshots first (ADR-006) | ADR-006, see below |
| `rebuild_state`, `read_model_schema` | Bookkeeping that must outlive the reset | Never; ADR-006 owns them |

The twelve tables marked "this ADR's migrations" are the **migrated tables**, plus the stamp table
below. [ADR-010](./010-backup-archive-format.md) §2 groups the same tables for backup; a new table
is added to both lists in the change that creates it.

**The branch and snapshot registries** are derived: a rebuild's replay upserts every row from the
log, so they follow ADR-006, not this ADR. They are not in the reset today, so:

- An additive column stays in the store's start-up DDL, and returns its error. SQLite checks the
  column first (`pragma_table_info`), as `SnapshotStore.migrateBranchID` does, instead of
  discarding a failed `ADD COLUMN`.
- A non-additive change bumps `repository.ReadModelSchemaVersion` and adds the table to the reset
  in the same change, after confirming, as the snapshot backfill did, that every row has an event
  behind it.

### 2. Numbered migrations with a version stamp

- Each backend has one ordered list of migrations, each a name and a function that applies it in
  a transaction. The lists are the same length with the same names in the same order on both
  backends; a test compares them. A migration's version is its position in the list, from 1.
- The stamp is one row in a table `store_schema`, beside `read_model_schema`: the version and name
  of the last migration applied. No stamp is version 0.
- A small hand-written runner in `internal/repository`, called by each backend with its list,
  applies every migration after the stamp, in order. No migration library.
- **Migration 1 is the baseline**: the DDL the stores run for the migrated tables today, including
  the existing `events` upgrades (the `branch_id` column and the composite version key). It runs
  against unstamped databases whose shape varies by release, so it alone must be idempotent; on a
  database already at the current shape it changes nothing and only stamps. Every later migration
  runs exactly once, against the shape the one before it left, and need not be idempotent.
- The stores stop creating or altering the migrated tables in their constructors; the runner owns
  that DDL. `storage.Open` runs it after constructing the read-model store and before the others,
  keeping the order DB-007 needs (the read model renames a pre-#733 `events` table before the
  event log touches that name).
- Two servers can start together, since `serve` takes only a shared lock. Each migration's
  transaction takes the database's write lock before it re-reads the stamp (on PostgreSQL a
  transaction advisory lock, as `AccountStore` uses for the last owner), so a migration another
  process already applied is skipped, not run twice.
- A SQLite migration that copies a table to change a key needs foreign keys off, which SQLite only
  allows outside a transaction. The runner turns them off around that migration, and the
  migration checks the row count before it commits, as `migrateCompositeVersionKey` does now.
- Migrations are never edited once released, and there are no down migrations: the way back is
  the backup (§4).

The read model keeps ADR-006's scheme unchanged: stamp, refuse, rebuild.

### 3. `events` and `streams` follow the same answer

They are migrated tables. A change to their shape keeps every row, position and id, and never
rewrites an event's data: a change to what an event means is still an upcaster (ADR-006). The
existing one-off migrations move into the baseline and stop running on their own.

### 4. Start-up against an older or newer database

- **Older** (stamp below the binary's last migration): `storage.Open` applies the pending
  migrations, logging each one's version and name before it runs. It does this for every command
  that opens the store (`serve`, `rebuild`, `user`), so a command run before the first `serve` of a
  new release never writes into the old shape. It never takes a backup: the documented upgrade
  procedure is to stop every server, take a backup with the old release, then start the new one.
- **Newer** (stamp above the binary's last migration): `storage.Open` reads the stamp first (a
  missing `store_schema` table is version 0) and refuses before it constructs any store or runs
  the read-model check, for every command, naming both versions. An older binary must not
  write into a shape it does not know, and an older `rebuild` must not be offered as the fix. The
  way back is to restore the backup taken before the upgrade, or to run the newer release. The
  backup command, which reads without constructing the stores (ADR-010 §6), reads the stamp and
  refuses any version but its own, since its archive's `format_version` describes the binary's
  shapes.
- The in-memory backend has no stamp and is never checked, as in ADR-006.

### 5. A failed migration is never ignored

Each migration runs in its own transaction with its stamp update. If it fails, the transaction
rolls back, the stamp and the tables stay at the last good version, and start-up stops with an
error naming the migration. Both backends' DDL is transactional, so this holds on each.

The start-up `ALTER TABLE` statements on the tables outside the rebuild whose errors are discarded
today (the SQLite `events.branch_id` column and branch registry columns) either move into the
baseline or return their error (#1816).

### Reviewer rule

A change to the shape of a migrated table arrives as a new numbered migration at the end of both
backends' lists, with a test that runs it against the previous shape; released migrations are
never edited. It is DB-010 in [ARCHITECTURAL-INVARIANTS.md](../ARCHITECTURAL-INVARIANTS.md), and
[CONVENTIONS.md](../CONVENTIONS.md#migrations) says how.

## Consequences

### Positive

- Any change to account, settings or event-log tables has a path that keeps the rows, on both
  backends, including the ones `ADD COLUMN` cannot make.
- A database says what shape it is in, so an older binary refuses a newer database instead of
  damaging it.
- The `events` upgrades stop probing the table's shape on every start.
- No start-up schema change on these tables fails silently.

### Negative

- Every migration is written twice, once per backend.
  - Mitigation: a test fails when the two lists differ in length or names, and each migration is
    tested against the previous shape on its backend.
- Automatic migration has no undo without a backup.
  - Mitigation: the upgrade procedure says to back up first, `serve` logs each migration before it
    runs it, and a newer database is refused with the way back in the message.
- The baseline must stay idempotent across every older shape it may meet.
  - Mitigation: it is today's DDL, already idempotent, and the existing legacy-shape tests cover
    it.

### Neutral

- Two version stamps now sit side by side: `read_model_schema` (ADR-006, rebuild) and
  `store_schema` (this ADR, migrate). They change independently.
- A backup archive (ADR-010) leaves the stamp out; a restore stamps the binary's version, and a
  non-additive migration of a carried table bumps the archive's `format_version`.

## References

- [ADR-002](./002-dual-database-strategy.md) — both backends, one database (DB-001, DB-006, DB-007)
- [ADR-006](./006-schema-evolution.md) — read-model stamp and rebuild, event upcasting
- [ADR-007](./007-access-model.md) — accounts outside the event log (AC-001)
- [ADR-010](./010-backup-archive-format.md) — the same tables, grouped for backup
- `ResetReadModel` in `internal/repository/{sqlite,postgres}/rebuild.go`
