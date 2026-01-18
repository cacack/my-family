# ADR-002: Dual Database Strategy (PostgreSQL + SQLite)

**Status:** Accepted
**Date:** 2025-12-07
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
- Mitigation: Document clearly; SQLite uses application-level fuzzy matching vs pg_trgm
- Larger test matrix
- Mitigation: testcontainers for PostgreSQL; in-memory/file SQLite for fast tests

### Neutral

- Configuration determines which backend is used at startup
- Migration path from SQLite to PostgreSQL is manual (export/import)

## Implementation Notes

### Database Selection

```go
// internal/config/config.go
type Config struct {
    DatabaseURL string // PostgreSQL connection string (takes precedence)
    SQLitePath  string // SQLite file path (fallback)
}

// Selection logic in main.go:
// 1. If DATABASE_URL set -> PostgreSQL
// 2. Else if SQLITE_PATH set -> SQLite at that path
// 3. Else -> SQLite at ./myfamily.db (default)
```

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
    └── readmodel.go      # SQLite ReadModelStore (FTS5)
```

### Feature Differences

| Feature | PostgreSQL | SQLite |
|---------|------------|--------|
| Full-text search | tsvector + GIN index | FTS5 virtual table |
| Fuzzy matching | pg_trgm extension | Application-level Levenshtein |
| JSON storage | Native JSONB | TEXT with JSON encoding |
| Future: Vector search | pgvector extension | Not available |
| Future: Geography | PostGIS extension | Not available |

## References

- [ETHOS.md - Easy self-hosting](../ETHOS.md)
