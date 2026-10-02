# ADR-002: Dual Database Strategy (PostgreSQL + SQLite)

**Status:** Accepted — implemented (wired into `serve` in #735)
**Date:** 2025-12-07 (implementation notes updated 2026-09 for #735 and #822)
**Decision Makers:** Chris
**Related Features:** 001-genealogy-mvp

## Context

The my-family platform targets self-hosters with varying technical capabilities and infrastructure:

1. **Power users** - Have Docker, can run PostgreSQL, want production-grade features
2. **Casual users** - Want a single binary download, no database setup, "just works"
3. **Demo/evaluation** - Try before committing to infrastructure

The ETHOS.md emphasizes "Easy self-hosting: Docker one-liner, not a 20-step guide" and "Offline-first / PWA: Researchers work in archives without internet."

## Decision Drivers

- Self-hosting must be accessible to non-technical users
- Demo mode should require zero infrastructure
- Production deployments need PostgreSQL features (pgvector for future AI, PostGIS for place mapping)
- Offline use cases require embedded database
- Single codebase should support both

## Considered Options

### Option 1: PostgreSQL Only

**Description:** Require PostgreSQL for all deployments. Provide Docker Compose for easy setup.

**Pros:**
- Single implementation path
- Full feature set everywhere (JSONB, tsvector, trigram, future pgvector)
- Simpler testing matrix

**Cons:**
- Higher barrier to entry for casual users
- No true offline mode
- Demo requires database setup or hosted instance
- Conflicts with "easy self-hosting" principle

### Option 2: SQLite Only

**Description:** Use SQLite exclusively. Embed database in application.

**Pros:**
- Zero-config deployment
- True offline capability
- Single binary distribution
- Simpler backup (copy file)

**Cons:**
- No pgvector for future AI features
- No PostGIS for geographic features
- Limited concurrent write performance
- Missing advanced search (trigram fuzzy matching)

### Option 3: PostgreSQL Primary + SQLite Fallback

**Description:** Support both databases. PostgreSQL for production/advanced features, SQLite for local/demo/offline.

**Pros:**
- Best of both worlds
- Progressive complexity - start simple, scale up
- Demo mode with zero infrastructure
- Future-proof for advanced PostgreSQL features

**Cons:**
- Two implementations to maintain
- Feature parity challenges (fuzzy search differs)
- Testing requires both paths
- Slightly larger codebase

## Decision

We chose **Option 3: PostgreSQL Primary + SQLite Fallback** because:

1. **Honors the self-hosting principle** - Users can download a binary and run immediately with SQLite, then migrate to PostgreSQL when ready.

2. **Enables future differentiation** - PostgreSQL unlocks pgvector (AI embeddings for smart search), PostGIS (historical place mapping), and advanced full-text search.

3. **Supports real offline use** - Genealogists work in archives, courthouses, and cemeteries without internet. SQLite enables true offline capability.

4. **Demo without friction** - Evaluators can try the full application without any database setup.

The repository layer abstracts database differences behind interfaces (`EventStore`, `ReadModelStore`), minimizing duplication.

## Consequences

### Positive

- Zero-config getting started experience
- True offline capability for field research
- Path to advanced features via PostgreSQL
- Flexible deployment options (Docker, binary, hybrid)

### Negative

- Two implementations of persistence layer
- Mitigation: Interface-based design; shared tests verify both implementations
- Feature differences between databases
- Mitigation: Document clearly; SQLite ports pg_trgm's fuzzy matching to Go, and a cross-backend search test pins the results (see Name Search below)
- Larger test matrix
- Mitigation: testcontainers for PostgreSQL; in-memory/file SQLite for fast tests

### Neutral

- Configuration determines which backend is used at startup
- Migration path from SQLite to PostgreSQL is manual (export/import)

## Implementation Notes

### Database Selection

Implemented in `internal/storage` (`storage.Open`), which `serve` calls at startup:

```go
// internal/storage/storage.go
// 1. DEMO_MODE=true     -> in-memory stores (sample data, resettable, no persistence)
// 2. DATABASE_URL set   -> PostgreSQL at that URL
// 3. otherwise          -> SQLite at SQLITE_PATH (default ./myfamily.db)
```

- **One database per backend.** All four stores — event log, read model,
  snapshots, branch registry — live in the single database the config names
  (DB-006). Each store runs its own DDL and migrations when constructed, so
  startup brings the schema up to date; the read model is constructed first
  (DB-007).
- **No silent fallback (DB-008).** If the selected backend cannot be opened —
  PostgreSQL unreachable, or the SQLite file's directory missing — `serve`
  exits non-zero with the reason. It never quietly runs in memory and drops every write on restart.
- **Memory is demo-only.** The in-memory stores are reachable only through
  `DEMO_MODE` (which overrides `DATABASE_URL`/`SQLITE_PATH`). There is no other
  opt-in: tests construct memory stores directly, and the E2E suite points
  `SQLITE_PATH` at a fresh temporary file per run.
- **The startup log names the store in use** — `Database: SQLite (<path>)`,
  `Database: PostgreSQL (<url with password redacted>)` or
  `Database: In-memory (no persistence)`.
- **Stores are closed on shutdown, after in-flight requests drain.** On
  SIGINT/SIGTERM the HTTP server stops accepting connections and waits (up to
  30s) for running handlers to finish; only then is the database closed. A
  server that fails to start (port in use) exits non-zero.
- **`DATABASE_URL` secrets are never logged.** The startup line redacts the
  password; a URL that does not parse is rejected with a fixed message, and
  driver errors are scrubbed of the connection string and password before
  they are reported.
- **Known limitation: projection failures are not recoverable yet.** A
  command appends its events and then projects them as a separate step, and a
  projection error does not fail the command: it is logged at error level
  (`projection failed after append`) and otherwise dropped. With persistent
  storage such an event stays in the log without its read-model row, and there
  is no projection rebuild yet, so PR-001 (projection in the same transaction
  as append) is a target, not a guarantee, for the SQL backends. Making
  projection failures fail the command, or adding a rebuild, is tracked in
  [#845](https://github.com/cacack/my-family/issues/845).

### SQLite Driver and Builds

The SQLite driver is `modernc.org/sqlite`, a pure-Go translation of SQLite: it
needs no cgo and no C toolchain, so every build can open a SQLite database. The
PostgreSQL driver (`github.com/lib/pq`) is also pure Go.

| Build | cgo | SQLite | PostgreSQL | Demo |
|-------|-----|--------|------------|------|
| Docker image (`Dockerfile`) | off | yes (default, `/data/myfamily.db`) | yes | yes |
| `go build` / `make binary` | either | yes | yes | yes |
| Release archives (`.goreleaser.yaml`) | off | yes | yes | yes |

Until #822 the driver was `github.com/mattn/go-sqlite3`, a cgo package: the
`CGO_ENABLED=0` release archives could not open SQLite at all, and FTS5 was only
compiled in under a build tag no shipped build set. `make check-release-sqlite`
(run in CI) builds the binary the way releases are built, serves from a fresh
SQLite file, and cross-compiles every release target, so that cannot regress
silently.

### Name Search

Decided in #822. For the same plain or fuzzy query both databases find the same
people, apart from the known differences listed below (DB-005); they get there
differently.

| Query | PostgreSQL | SQLite |
|-------|------------|--------|
| Plain (`q=`) | tsvector match **or** `full_name ILIKE '%q%'` (and alternate names' full names/nicknames) | the `ILIKE '%q%'` arm alone, on the person's full name and alternate names' full names/nicknames — a scan, no index |
| Fuzzy (`fuzzy=true`) | pg_trgm `%` (similarity ≥ 0.3) on given name, surname, full name and alternate names/nicknames | the same similarity, computed in Go (`repository.TrigramSimilarity`, a port of pg_trgm tested against it) over every person that passes the date/place filters |

SQLite's own `LOWER` and `LIKE` fold only ASCII case and have no escape
character by default, so they would disagree with `ILIKE` on names such as
`MÜLLER` and on queries containing `\`. The SQLite store instead registers
`ilike_contains(value, query)` with the driver: a Go function
(`repository.ContainsFold`, tested against PostgreSQL) that follows `ILIKE`'s
rules — case folded for every letter, `%` and `_` as wildcards, backslash as the
escape character. The place filters use it too, and the in-memory store calls
the same function, so all three backends apply one definition. Query and place
filters are trimmed on every backend.

SQLite does **not** use FTS5. Plain FTS5 matches whole words, so `John` would not
find `Johnson`, which makes it stricter than PostgreSQL's substring arm; adding a
substring arm back brings the full scan back with it. One search strategy is
simpler than two. Earlier versions could create FTS5 tables (`persons_fts`,
`person_names_fts`) and triggers; the read model drops them on startup so saves
stop maintaining an index nothing reads.

The scan is linear in the number of people. Alternate names are matched in one
uncorrelated subquery; a join that rescans every alternate name for every person
is quadratic (about 10 s at 10,000 people, over 90 s at 100,000), and
`TestSearchPersons_PlainSearchPlan` pins the query plan against it. Measured with
`BenchmarkSearchPersons` on a 2.1 GHz Xeon, one alternate name per person:

| People | Plain | Fuzzy |
|--------|-------|-------|
| 20,000 | ~0.05 s | ~0.35 s |
| 100,000 | ~0.25 s | ~2 s |

Fuzzy search costs more because every name is scored in Go. Revisit if search on
large SQLite trees is measurably slow for users (#14, #489): FTS5 is built into
`modernc.org/sqlite`, and its `trigram` tokenizer can serve substring queries of
three or more characters from an index; fuzzy scoring would need its own
candidate filter (for example, shared trigrams) rather than FTS5.

`internal/integration/search_parity_test.go` runs the #822 query set (`John`,
`Joh`, `O'Brien`, `Smith-Jones`, a multi-word query, fuzzy and plain), plus
non-ASCII case, wildcards and escapes, padded place filters and prefixed
alternate names, through the HTTP API on memory, SQLite and PostgreSQL and
requires identical results.

Known differences that remain:

- **Word order, stemming and punctuation.** PostgreSQL's tsvector arm splits the
  query into words, so it also matches the words of a multi-word query in any
  order (`Smith John`), English stems, and queries wrapped in punctuation
  (`(Mary)` and `"Mary-Ann"` find Mary-Ann O'Brien). SQLite matches the query as
  one literal substring and finds nobody for those.
- **Case folding and word characters depend on PostgreSQL's locale.** SQLite
  and memory fold every letter (Go's `unicode.ToLower`), as `ILIKE` does in a
  UTF-8 locale; a PostgreSQL database created with the `C` ctype folds only
  ASCII. Likewise, fuzzy search splits names into words where glibc's UTF-8
  locales do: word characters are Unicode's Alphabetic property plus digits, so
  Indic vowel signs, Arabic harakat and letter numbers stay inside a word
  (`repository.TrigramSimilarity` agrees with pg_trgm on every code point
  PostgreSQL 16 on `C.UTF-8` treats as a word character). Under the `C` ctype
  pg_trgm treats every non-ASCII character as a separator.
- **Soundex.** PostgreSQL uses `difference() >= 3`; SQLite and memory require
  equal Soundex codes.
- **Which matches fill the limit.** Search returns at most `limit` people (20 by
  default, at most 100). Plain searches order by relevance (`ts_rank`) on
  PostgreSQL and by name on SQLite, so when more people match than the limit,
  the two return different subsets of the same match set. Fuzzy searches order
  by similarity on both, scoring each person as PostgreSQL's `DISTINCT ON (id)
  ORDER BY is_primary DESC, rank_score DESC` does: the best of their own names
  and their primary alternate name (its nickname included), or, only when none
  of those match, the best of their other alternate names
  (`TestSearchParity_FuzzyPrimaryName`). Ties are broken by surname, given
  name and id, so both return the same people in the same order
  (`TestSearchParity_FuzzyLimit`) as long as PostgreSQL's collation orders names by code point, as the `C` and
  `C.UTF-8` collations do. The in-memory demo store keeps relevance results in
  the order it finds them.

### Repository Interfaces

```
internal/repository/
├── eventstore.go         # EventStore interface
├── readmodel.go          # ReadModelStore interface
├── postgres/
│   ├── eventstore.go     # PostgreSQL EventStore
│   └── readmodel.go      # PostgreSQL ReadModelStore (tsvector, pg_trgm)
└── sqlite/
    ├── eventstore.go     # SQLite EventStore
    └── readmodel.go      # SQLite ReadModelStore (substring + trigram search)
```

### Feature Differences

| Feature | PostgreSQL | SQLite |
|---------|------------|--------|
| Name search | tsvector + GIN index, `ILIKE` substring | `ILIKE`-equivalent substring scan (no FTS5) |
| Fuzzy matching | pg_trgm extension | pg_trgm similarity ported to Go |
| JSON storage | Native JSONB | TEXT with JSON encoding |
| Future: Vector search | pgvector extension | Not available |
| Future: Geography | PostGIS extension | Not available |

## References

- [ETHOS.md - Easy self-hosting](../ETHOS.md)
