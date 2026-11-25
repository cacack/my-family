# ADR-010: Backup Archive Format and Restore Path

**Status:** Accepted
**Date:** 2026-11-24
**Decision Makers:** Project maintainer
**Related Features:** [#1641](https://github.com/cacack/my-family/issues/1641) (this decision),
[#15](https://github.com/cacack/my-family/issues/15) (write a backup),
[#1642](https://github.com/cacack/my-family/issues/1642) (restore),
[#1573](https://github.com/cacack/my-family/issues/1573) (complete tree export)

## Context

my-family has no backup. The README tells SQLite operators to copy the database file while the
server is stopped, and says nothing for PostgreSQL. Three pieces of work need one answer to "what
is in the file": the backup command (#15), restore (#1642) and the user-level complete export
(#1573). This record fixes that answer before any of them is built.

What the code gives us to work with:

- The event log (`events`, `streams`) is the source of truth and append-only (ES-002). Every read
  model table can be dropped and re-projected from it (ES-004); `myfamily rebuild` does exactly
  that (`repository.RebuildReadModel`).
- Media bytes are in the log (`MediaCreated.FileData`, `internal/domain/events.go`), so a copy of
  the log holds the files too.
- The branch and snapshot registries are projected from the log (`projectBranchCreated`,
  `projectSnapshotCreated` in `internal/repository/projection.go`), so a replay into an empty
  database re-creates them.
- `EventStore.ReadAll` reads the log in position order. Positions are not dense: SQLite assigns
  `MAX(position)+1` inside the append transaction, PostgreSQL a `BIGSERIAL`, which can leave gaps
  and can commit a lower position after a higher one.
- Positions are referenced from outside the log: a branch's `base_position` is a log position. A
  restore that renumbered events would break every branch.
- Some tables are held nowhere but the database: accounts and settings (ADR-007), the auth audit
  log. A copy of the log alone loses every user.
- The two backends store the same logical values differently (SQLite `TEXT` times and `0/1`
  booleans, PostgreSQL `timestamptz` and `boolean`), and ADR-002 promises one feature set on both.

## Decision Drivers

- One file restores onto SQLite or PostgreSQL, whichever wrote it (ADR-002, DB-001).
- The archive must not depend on the read-model schema, which changes freely (ADR-006).
- An archive written by an older release must restore into a newer one.
- A backup runs while the server keeps writing, and must still be a consistent copy.
- An operator can check a file is intact without restoring it.
- Simple enough to explain in the README in one paragraph, and readable with standard tools
  (Your Research Is Yours, ETHOS).

## Considered Options

### Sub-decision 1 — What the archive copies

#### Option 1A: Database-native copy (a SQLite file, a `pg_dump`)

**Pros:** no new code for the data itself; fast; byte-exact.

**Cons:** two formats, one per backend, so a SQLite backup cannot become a PostgreSQL instance; a
`pg_dump` needs the PostgreSQL client tools beside the binary (ADR-004's single binary); it copies
the read model, tying the file to the read-model schema version of the release that wrote it; a
SQLite file copied while the server writes needs the online-backup API or `VACUUM INTO`, a third
code path.

#### Option 1B: Event-log archive plus the tables held nowhere else (chosen)

**Pros:** one format for both backends; independent of the read-model schema, because restore
replays the log as `rebuild` does; event shapes already evolve through the upcasters of ADR-006;
the file is small compared with a copy that includes the derived tables.

**Cons:** restore costs a full replay; every table outside the log needs a listed, tested place in
the archive, and a new one must be added deliberately.

### Sub-decision 2 — Container

#### Option 2A: A directory of files

**Cons:** not one file; nothing to download from the app; easy to copy partly.

#### Option 2B: A tar stream

**Cons:** no index, so verifying or reading the manifest means reading the whole file; no
standard per-entry checksum.

#### Option 2C: A zip file (chosen)

**Pros:** one file with an index, readable by every operating system without extra tools; Go's
`archive/zip` reads and writes it, Zip64 included, so a tree over 4 GB is fine; entries are
compressed one by one, which suits the base64 media inside the log.

### Sub-decision 3 — Consistency while the server writes

#### Option 3A: Read the log up to the highest position seen at the start

**Cons:** not safe on PostgreSQL: a transaction holding a lower `BIGSERIAL` position can commit
after the backup read the highest one, and the backup would silently miss that event.

#### Option 3B: One read snapshot for everything (chosen)

The backup opens one read transaction (PostgreSQL `REPEATABLE READ, READ ONLY`; SQLite a read
transaction, which in the WAL mode the store already uses is a snapshot that does not block
writers). The last log position is the highest position visible in that snapshot, and the log up
to it and every carried table are read in the same transaction. This is the owner's decision ("fix
the last log position when the backup starts, read the log up to it, read the carried tables in
one read transaction") made exact: the snapshot, not a separate `MAX(position)` query, decides
which events are in.

#### Option 3C: Stop the server, or take the exclusive lock

**Cons:** downtime, which is what #15 exists to remove.

## Decision

### 1. An event-log archive, not a database-native copy

The archive holds the event log and the tables the log cannot rebuild. Restore loads them into an
empty database and replays the log, as `myfamily rebuild` does. One file restores into either
backend.

### 2. Tables outside the log

Every table a backend creates falls in one of these groups. The read-model tables are the ones
`ResetReadModel` drops (`branchScopedTables` and `mainOnlyTables` in each backend's
`readmodel.go` and `rebuild.go`, the same list on both).

| Table | Rebuilt from the log | In the archive |
|---|---|---|
| `events`, `streams` | They are the log | **Carried**, as the log |
| Every read-model table: `persons`, `families`, `family_children`, `pedigree_edges`, `person_names`, `person_external_ids`, `family_external_ids`, `life_events`, `attributes`, `associations`, `sources`, `source_external_ids`, `citations`, `citation_external_ids`, `notes`, `media`, `evidence_analyses`, `evidence_conflicts`, `research_logs`, `proof_summaries`, `duplicate_dismissals`, `submitters`, `repositories`, `repository_external_ids`, `lds_ordinances`, `places`, `place_aliases`, `projection_failures` | Yes, by the replay | Left out |
| `branches` (branch registry) | Yes, from the branch lifecycle events | Left out |
| `snapshots` (snapshot registry) | Yes, from `SnapshotCreated`/`SnapshotDeleted` | Left out |
| `users` | No | **Carried**, with the argon2id password hashes |
| `tree_memberships` | No | **Carried** |
| `site_settings` | No | **Carried** |
| `tree_settings` | No | **Carried** |
| `account_settings` | No | **Carried** |
| `auth_audit` | No | **Carried** |
| `api_tokens` | No | **Carried**; only the token hashes are stored |
| `sessions` | No | Left out: everyone signs in again after a restore |
| `password_resets` | No | Left out: a pending reset link must be issued again |
| `invites` | No | Left out: a pending invite must be sent again |
| `rebuild_state` | Bookkeeping | Left out; the restore's replay writes and clears it |
| `read_model_schema` | Bookkeeping | Left out; the restore's replay stamps it (ADR-006) |
| The migration version stamp of ADR-011 ([#1803](https://github.com/cacack/my-family/issues/1803), [#1816](https://github.com/cacack/my-family/issues/1816)) | Bookkeeping | Left out; restore stamps the binary's version |

A new table must be added to this table, in this ADR, in the same change that creates it.

**Secrets.** Nothing outside the database is needed to use what the archive carries. Checked
against the code:

- `internal/config` reads `DATABASE_URL`, `SQLITE_PATH`, `PORT`, `LOG_LEVEL`, `LOG_FORMAT`,
  `DEMO_MODE`, `AUTH_MODE`, `TRUSTED_PROXIES` and `AUTH_AUDIT_RETENTION_DAYS`. None is a signing or
  encryption key. `DATABASE_URL` may hold the database password; it is the target's configuration,
  not data, and is not carried.
- Password hashes are self-contained argon2id strings with their own salt and parameters
  (`internal/auth/password.go`), so they verify on any instance.
- Session, API token, invite and reset tokens are stored as unkeyed SHA-256 hashes
  (`hashToken`, `internal/auth/service.go`). No server key is involved, so a carried API token
  keeps working after a restore. Left-out tokens simply stop existing.

### 3. The complete export (#1573) uses the same layout without the account tables

The export is the same zip: the same manifest and the same log entries, with `tree_settings` as
the only carried table. It leaves out `users`, `tree_memberships`, `api_tokens`, `auth_audit`,
`account_settings` (accounts) and `site_settings` (instance configuration). A reader treats a
missing table entry as an empty table, so restore reads an export too; the instance then has no
owner and starts in setup mode, as an upgraded instance without accounts does today.

Because the export is the log, the record fields #1573 documents are the fields of each event
type, not of read-model rows.

### 4. Restore accepts only an empty database

Empty means no row in `events`, `streams` or any carried table. Restore refuses anything else and
leaves it unchanged; merging a backup into an instance with data is not supported.

### 5. Layout, versioning and integrity

```
manifest.json
events.ndjson
streams.ndjson
tables/users.json
tables/tree_memberships.json
tables/site_settings.json
tables/tree_settings.json
tables/account_settings.json
tables/auth_audit.json
tables/api_tokens.json
```

- **`manifest.json`**: `format_version` (integer, starting at 1), `app_version` (the release that
  wrote it), `created_at` (RFC 3339, UTC), `event_count`, `last_position` (the highest log
  position in the archive, 0 for an empty log), and `entries`: every other entry's name and
  SHA-256 (lowercase hex). The manifest is the only entry it does not list.
- **`events.ndjson`**: one JSON object per line, in position order, with every `events` column by
  name: `id`, `stream_id`, `stream_type`, `branch_id`, `version`, `event_type`, `data`,
  `metadata`, `timestamp`, `position`. `data` and `metadata` are the stored JSON, embedded as JSON
  values and not upcast. Positions are kept, gaps included.
- **`streams.ndjson`**: one object per `streams` row, by column name.
- **`tables/<table>.json`**: one JSON array per carried table, each row an object keyed by column
  name.

Values are logical, not backend-native: times are RFC 3339 strings in UTC, booleans are JSON
booleans, UUIDs are strings, so a file from SQLite loads into PostgreSQL and the reverse.

**Integrity.** Verification reads every entry and checks its SHA-256 against the manifest, that
the manifest lists every entry and the zip holds every listed one, and that `events.ndjson` holds
`event_count` lines ending at `last_position`. A failure names the entry. The zip's own CRC-32 is
not relied on.

**Versioning.** A reader refuses a `format_version` newer than it knows and reads every older one.
An archive from an older release restores into a newer one:

- Events are stored as written and go through the upcasters of ADR-006 when the replay decodes
  them.
- Carried tables load by column name into the current shape. A column the archive lacks takes the
  column's default; a column the binary does not know fails the restore.
- A non-additive change to a carried table's shape (a migration under ADR-011) bumps
  `format_version`, and the reader converts older entries to the new shape, as an upcaster does for
  an event.

### 6. A consistent copy while the server writes

Option 3B: one read snapshot. The backup opens the store with shared access (as `serve` does, not
the exclusive lock of `rebuild`), reads the log and every carried table inside one read
transaction, and records the highest position that snapshot holds as `last_position`. The log is
append-only, so nothing the snapshot sees changes underneath it.

Older databases hold a few values only in the read model, which `rebuild` logs before it replays:
coordinates and EXIDs imported before #1225, brick walls marked before #802, snapshots created
before #624. A backup of such a database would lose them, so the backup checks for them and refuses,
naming `myfamily rebuild` (which logs them once, for good). The backup itself stays read-only.

### 7. The app does not encrypt the archive

The archive is a plain zip holding the whole tree, living people and private records included, plus
usernames and password hashes. The README says what it holds and that the operator must protect
it as they protect the database. Passphrase encryption is a later option
([#1817](https://github.com/cacack/my-family/issues/1817)); `format_version` leaves room for it.

### The restore path

1. Read and verify the whole archive (Decision 5). A damaged or too-new archive is refused before
   the database is opened.
2. Open the target with the exclusive lock `rebuild` takes (`storage.AccessRebuild`), so restore
   refuses while a server holds the database.
3. Refuse a target that is not empty (Decision 4).
4. In one transaction, insert `streams`, then `events` with every column as archived, positions
   included, then the carried tables. On PostgreSQL, set the `events.position` sequence past
   `last_position`. A failure rolls back and leaves the target empty.
5. Replay the log with `RebuildReadModel`, which re-creates the read model and the branch and
   snapshot registries and stamps the read-model schema version; stamp the ADR-011 migration
   version.
6. Report the event count and the final position, which equal the manifest's.

The in-memory backend (`DEMO_MODE`) has nothing to back up or restore into; both commands refuse
it, as `rebuild` does.

## Consequences

### Positive

- One file moves an instance between SQLite and PostgreSQL, which nothing supports today.
- Backups survive read-model schema changes: the read model is never in them.
- The format is documented, inspectable with `unzip` and a text editor, and shared by the backup
  and the complete export.
- Verification needs no database.

### Negative

- Restore takes as long as a full `rebuild`.
- Mitigation: restore is rare, and it is the same replay operators already run with `rebuild`.
- The archive is unencrypted and holds password hashes and living people.
- Mitigation: documented in the README; argon2id hashes resist offline guessing; #1817 adds
  encryption.
- After a restore everyone signs in again, and pending invites and password resets are gone.
- Mitigation: the README's restore section says so.
- Every new non-derived table is a change to this ADR and to the archive.
- Mitigation: the table above is the checklist, and #15's test asserts which tables the archive
  holds.

### Neutral

- A database from before #1225, #802 or #624 needs one `myfamily rebuild` before its first backup.
- The archive is roughly the size of the event log, compressed, since media bytes are in the log.

## References

- [ADR-001](./001-event-sourcing-cqrs.md) - Event sourcing; the log is the source of truth
- [ADR-002](./002-dual-database-strategy.md) - Dual database strategy
- [ADR-005](./005-research-branch-data-model.md) - Branches as an overlay on the shared log (`base_position`)
- [ADR-006](./006-schema-evolution.md) - Read-model rebuild and event upcasting
- [ADR-007](./007-access-model.md) - Accounts, sessions and settings outside the event log
- ADR-011 ([#1803](https://github.com/cacack/my-family/issues/1803)) - Schema changes to the tables outside the rebuild
- [#15](https://github.com/cacack/my-family/issues/15), [#1642](https://github.com/cacack/my-family/issues/1642), [#1573](https://github.com/cacack/my-family/issues/1573), [#1817](https://github.com/cacack/my-family/issues/1817)
