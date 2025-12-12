# Research: My Family Genealogy MVP

**Branch**: `001-genealogy-mvp` | **Date**: 2025-12-07

## Overview

This document consolidates research findings for the genealogy MVP implementation. All "NEEDS CLARIFICATION" items from the Technical Context have been resolved through the technology stack specification provided.

---

## 1. Echo HTTP Framework Best Practices

### Decision: Hybrid Architecture with OpenAPI Code Generation

**Rationale**: Use `oapi-codegen` for spec-first API development, generating type-safe handlers from OpenAPI specification. This aligns with FR-012 (REST API with JSON responses) and provides automatic documentation.

**Alternatives Considered**:
- `swaggo/swag` (code-first): Rejected - requires annotation maintenance, spec-first better for API-driven design
- Manual routing: Rejected - loses type safety and documentation benefits

### Key Implementation Patterns

**Middleware Stack** (order matters):
1. `middleware.Recover()` - panic recovery
2. `middleware.RequestID()` - request tracing
3. Custom structured logger (JSON format for production)
4. `middleware.CORS()` - frontend access
5. Custom error handler for consistent JSON error responses

**Request Validation**: Use `go-playground/validator/v10` with custom validator registered on Echo instance.

**Testing**: Use `httptest` with mocked services, table-driven tests for handler scenarios.

---

## 2. Ent ORM for Event Sourcing

### Decision: Use `field.Bytes()` for Event Data with Application-Level JSON

**Rationale**: Enables dual-database support (PostgreSQL + SQLite) without sacrificing functionality. The MVP doesn't require PostgreSQL JSONB query features.

**Alternatives Considered**:
- `field.JSON()` with PostgreSQL-only JSONB: Rejected - breaks SQLite compatibility requirement
- Raw SQL without ORM: Rejected - loses type safety and migration tooling

### Schema Design

**Event Store Tables**:
```go
// events table
field.UUID("id").Default(uuid.New)
field.UUID("stream_id")           // Aggregate ID
field.String("stream_type")       // "Person", "Family"
field.String("event_type")        // "PersonCreated", etc.
field.Bytes("data")               // JSON-encoded event
field.Bytes("metadata")           // Correlation ID, timestamp
field.Int64("version")            // Per-aggregate version
field.Int64("position").Unique()  // Global ordering
field.Time("timestamp")

// Indexes
index.Fields("stream_id", "version").Unique()
index.Fields("position")
```

**Migration Strategy**: Use Atlas (Ent's migration tool) for production. Event store schema is append-only after deployment.

---

## 3. Event Sourcing with CQRS-lite

### Decision: Synchronous Projections for MVP

**Rationale**: Simplicity for single-user MVP. All read model updates happen in the same transaction as event append. This provides immediate consistency without eventual consistency complexity.

**Alternatives Considered**:
- Asynchronous projections with workers: Deferred - adds complexity, needed for multi-user
- Hybrid (some sync, some async): Deferred to post-MVP

### Event Store Design

**Stream per Aggregate**: Events grouped by aggregate ID (Person, Family).

**Serialization**: JSON via `encoding/json`. Human-readable, debuggable, sufficient performance for genealogy workloads.

**Optimistic Locking**: Version field per aggregate prevents concurrent writes.

### Snapshot Strategy

**Decision**: Defer snapshots until needed.

**Rationale**: Genealogy aggregates (Person, Family) unlikely to accumulate >100 events in typical usage. Implement when aggregate load times exceed 100ms.

### Testing Approach

- **Given-When-Then** pattern for aggregate tests
- **In-memory event store** for unit/integration tests
- **Projection rebuild tests** to ensure read models can be regenerated
- **Concurrency tests** for optimistic locking

---

## 4. Svelte 5 + D3.js Integration

### Decision: Hybrid Rendering - D3 Calculations, Svelte Templates

**Rationale**: Best performance and developer experience. D3 handles complex layout algorithms; Svelte handles DOM rendering with its efficient diffing.

**Alternatives Considered**:
- Pure D3 DOM manipulation: Rejected - fights Svelte's reactivity, harder to test
- Pure Svelte with custom layout: Rejected - reinventing D3's proven algorithms

### Pedigree Chart Implementation

**Layout**: `d3.tree()` for ancestor chart (standard pedigree view).

**Rendering**:
```svelte
<script>
  let treeLayout = $derived(d3.tree().size([width, height]));
  let root = $derived(d3.hierarchy(familyData));
  let treeData = $derived(treeLayout(root));
</script>

{#each treeData.links() as link}
  <path d={d3.linkVertical()(link)} />
{/each}

{#each treeData.descendants() as node (node.id)}
  <PersonNode {node} />
{/each}
```

### Pan/Zoom Navigation

**Implementation**: `d3.zoom()` behavior with Svelte 5 `$state` for transform tracking.

**Features**:
- Scale extent limits (0.5x to 5x)
- Programmatic zoom-to-fit for initial view
- Touch gesture support (built-in)

### Performance Strategy

**For MVP (target: 10,000 individuals)**:
1. Full SVG rendering for <500 visible nodes
2. Progressive disclosure (collapse distant branches)
3. Viewport culling for large trees
4. Keyed `{#each}` blocks for efficient updates

### Testing

- **Unit tests**: D3 calculations isolated from rendering
- **Component tests**: Svelte Testing Library for interaction
- **Visual regression**: Deferred unless issues arise

---

## 5. GEDCOM Processing

### Decision: Graceful Degradation with Full Round-Trip Fidelity

**Rationale**: Users import data from various sources with varying quality. Rejecting imports for minor issues creates poor UX. Preserving all data enables lossless re-export.

### Date Handling

**Data Model**:
```go
type GenDate struct {
    Raw       string        // Original GEDCOM string
    Qualifier DateQualifier // ABT, BET, BEF, AFT, etc.
    Date1     *ParsedDate   // Primary date
    Date2     *ParsedDate   // Second date for ranges
    Calendar  string        // DGREGORIAN (default)
}

type ParsedDate struct {
    Year  *int // nil if unknown
    Month *int // 1-12
    Day   *int // 1-31
}
```

**Supported Formats** (GEDCOM 5.5 spec):
- Exact: `1 JAN 1850`
- Approximate: `ABT 1850`, `CAL 1850`, `EST 1850`
- Ranges: `BET 1850 AND 1860`, `FROM 1850 TO 1860`
- Bounded: `BEF 1850`, `AFT 1850`
- Partial: `JAN 1850`, `1850`

### Character Encoding

**Import Flow**:
1. Read HEAD.CHAR for declared encoding
2. Detect BOM if present
3. Try declared encoding, fallback: UTF-8 → Windows-1252 → ISO-8859-1
4. Convert to UTF-8 for internal storage

**Export**: Always UTF-8 with explicit HEAD.CHAR declaration.

### Error Recovery Strategy

**Validation Levels**:
- `Error`: Block import (e.g., completely unparseable file)
- `Warning`: Import with flag (e.g., invalid date format stored as raw)
- `Info`: Note without blocking (e.g., unknown custom tag preserved)

**Specific Strategies**:
| Issue | Recovery |
|-------|----------|
| Missing INDI/FAM record | Skip cross-reference, warn |
| Invalid date format | Store raw text, flag for review |
| Duplicate @XREF@ | Auto-rename with suffix |
| Invalid characters | Replace with U+FFFD, warn |
| Circular relationships | Detect and reject cycle, warn |

### Round-Trip Preservation

**Storage**:
- Original @XREF@ as alternate ID
- Unknown/custom tags as `RawGedcomTag` slice
- Original date string alongside parsed components

**Export**:
- Generate stable @XREF@ based on internal UUID
- Re-emit preserved custom tags
- Use GEDCOM 5.5 standard tag order

### Duplicate Detection (Deferred to Post-MVP)

The spec mentions flagging potential duplicates during import based on name and date similarity. This is deferred to post-MVP for the following reasons:

- MVP focus is on reliable import/export without data loss
- Duplicate detection adds UI complexity (merge/keep separate workflow)
- False positives could frustrate users more than missed duplicates
- Users can search and manually merge after import

**Future Implementation Approach**:
- Compare incoming records against existing by: surname + given_name similarity (Levenshtein), birth_year within ±5 years
- Flag as "potential duplicate" rather than auto-merge
- Present in import results for user review

---

## 6. Full-Text Search

### Decision: Database-Native Search (PostgreSQL tsvector / SQLite FTS5)

**Rationale**: Avoids external dependencies (Elasticsearch, etc.) while meeting <500ms search requirement for 10,000 individuals.

### PostgreSQL Implementation

```sql
ALTER TABLE persons ADD COLUMN search_vector tsvector;
CREATE INDEX idx_persons_search ON persons USING GIN(search_vector);

-- Update trigger
UPDATE persons SET search_vector =
  to_tsvector('english', coalesce(given_name,'') || ' ' || coalesce(surname,''));
```

### SQLite Implementation

```sql
CREATE VIRTUAL TABLE persons_fts USING fts5(
  given_name, surname,
  content='persons', content_rowid='id'
);
```

### Fuzzy Matching (FR-010)

**Approach**: Use PostgreSQL `pg_trgm` extension for trigram similarity:
```sql
CREATE EXTENSION pg_trgm;
CREATE INDEX idx_persons_trgm ON persons USING GIN(surname gin_trgm_ops);

-- Query
SELECT * FROM persons
WHERE surname % 'Katherine'  -- Finds Catherine, Kathryn, etc.
ORDER BY similarity(surname, 'Katherine') DESC;
```

For SQLite: Implement application-level fuzzy matching with Levenshtein distance for common name variations.

---

## Summary of Key Decisions

| Area | Decision | Rationale |
|------|----------|-----------|
| API Framework | Echo + oapi-codegen | Spec-first, type-safe, documented |
| ORM | Ent with bytes for JSON | Dual-database compatibility |
| Event Sourcing | Synchronous projections | MVP simplicity, immediate consistency |
| Serialization | JSON | Debuggable, sufficient performance |
| Pedigree Chart | D3 layout + Svelte render | Best of both worlds |
| GEDCOM Import | Graceful degradation | UX priority, preserve all data |
| Search | Native DB full-text | No external dependencies |

---

## Unresolved Items

None. All technical decisions are resolved based on the provided technology stack specification.

## Next Steps

1. Generate data-model.md with entity definitions
2. Create OpenAPI contract in contracts/openapi.yaml
3. Generate quickstart.md for developer onboarding
