# ADR-001: Event Sourcing with CQRS-lite

**Status:** Accepted
**Date:** 2025-12-07
**Decision Makers:** Chris
**Related Features:** 001-genealogy-mvp

## Context

The my-family genealogy platform needs a data persistence strategy that supports:

1. **Full audit trail** - Who changed what, when, and why (core differentiator per ETHOS.md)
2. **Future git-style branching** - Explore hypotheses without polluting main tree
3. **Rollback capability** - Mistakes must be recoverable
4. **Merge with review** - Bring proven research into main tree with diff view

Traditional CRUD persistence overwrites data, losing history. This conflicts directly with the project's vision of treating genealogy research like code - versioned, branched, and collaborative.

## Decision Drivers

- Git-inspired workflow is a core differentiator (ETHOS.md)
- Genealogy data changes must be auditable and reversible
- Future features (branching, merge, collaborative forks) depend on having complete history
- Research integrity requires tracking data provenance
- Must support both PostgreSQL and SQLite backends

## Considered Options

### Option 1: Traditional CRUD with Audit Log

**Description:** Standard create/update/delete operations with a separate audit_log table recording changes.

**Pros:**
- Simpler initial implementation
- Familiar pattern for most developers
- Direct querying of current state

**Cons:**
- Audit log is separate from data - can drift or be incomplete
- Reconstructing past states requires complex logic
- Branching/merging would require separate implementation
- No natural support for "what changed between versions"

### Option 2: Event Sourcing with CQRS-lite

**Description:** Store all changes as immutable events. Current state is derived by replaying events. Separate command (write) and query (read) models.

**Pros:**
- Complete, immutable history by design
- Natural foundation for branching (branch = filtered event stream)
- Time-travel queries built-in (replay to any point)
- Events are the source of truth - audit is automatic
- Supports future collaborative features (merge = event stream reconciliation)

**Cons:**
- More complex than CRUD
- Requires maintaining read models (projections)
- Event schema evolution needs careful handling
- Larger storage footprint (events + read models)

### Option 3: Temporal Tables (SQL:2011)

**Description:** Database-managed versioning with period columns and system-time queries.

**Pros:**
- Database handles versioning automatically
- Standard SQL feature (PostgreSQL, SQL Server, MariaDB)
- Simple queries for current state

**Cons:**
- SQLite doesn't support temporal tables
- Row-level versioning, not semantic events
- Harder to implement branching semantics
- Less flexibility for custom merge strategies

## Decision

We chose **Option 2: Event Sourcing with CQRS-lite** because:

1. **Aligns with core vision** - The git-inspired workflow is not just a nice-to-have; it's a primary differentiator. Event sourcing provides the natural foundation.

2. **Future-proofs architecture** - Branching, merging, and collaborative features become extensions of the event model rather than bolted-on afterthoughts.

3. **Audit is automatic** - Every change is an event. There's no separate audit log to maintain or reconcile.

4. **Enables time-travel** - "Show me this tree as of 2023" is trivial with event replay.

5. **Database-agnostic** - Events are just data; works identically on PostgreSQL and SQLite.

The "lite" in CQRS-lite means we use synchronous projections (see ADR-003) and avoid the complexity of distributed event buses for the MVP.

## Consequences

### Positive

- Complete audit trail from day one
- Natural path to branching/merging features
- Events serve as integration points (webhooks, sync)
- Debugging is easier - can replay exact sequence of changes
- Testing with Given-When-Then pattern matches domain language

### Negative

- Higher initial complexity than CRUD
- Mitigation: Start with in-memory event store for testing, layer in persistence
- Developers need to think in events, not state mutations
- Mitigation: Clear documentation and examples in codebase
- Storage grows with all changes, not just current state
- Mitigation: Acceptable for genealogy scale; snapshot strategy if needed

### Neutral

- Read models must be kept in sync with events (projection handlers)
- Event schema changes require migration strategy (versioned event types)

## Implementation Notes

Key implementation details:

- **Stream per aggregate**: Events grouped by Person ID or Family ID
- **Optimistic locking**: Version field prevents concurrent writes
- **JSON serialization**: Human-readable, debuggable, sufficient performance
- **Synchronous projections**: Read models updated in same transaction (MVP simplicity)

```
internal/
├── domain/events.go      # Event type definitions
├── command/              # Write side - validates and emits events
├── query/                # Read side - queries read models
└── repository/
    ├── eventstore.go     # EventStore interface
    ├── projection.go     # Event -> read model handlers
    ├── postgres/         # PostgreSQL implementation
    └── sqlite/           # SQLite implementation
```

## References

- [ETHOS.md - Git-Inspired Workflow](../ETHOS.md)
- [Martin Fowler - Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html)
- [Greg Young - CQRS Documents](https://cqrs.files.wordpress.com/2010/11/cqrs_documents.pdf)
