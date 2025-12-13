# ADR-003: Synchronous Projections for MVP

**Status:** Accepted
**Date:** 2025-12-07
**Decision Makers:** Chris
**Related Features:** 001-genealogy-mvp

## Context

Event sourcing (ADR-001) requires projections to maintain read models - denormalized views optimized for queries. When an event is appended, the corresponding read models must be updated.

The timing of this update has significant architectural implications:

- **Synchronous**: Update in same transaction as event append
- **Asynchronous**: Update via background workers consuming event stream

This decision specifically addresses the MVP phase targeting single-user deployments.

## Decision Drivers

- MVP targets single-user, self-hosted deployments
- Immediate consistency preferred for genealogy workflows (edit person, see change)
- Minimize operational complexity for self-hosters
- Must not preclude future multi-user support
- Development velocity - simpler architecture ships faster

## Considered Options

### Option 1: Synchronous Projections

**Description:** Read model updates happen in the same database transaction as event append. Command completes only when both event and projections are persisted.

**Pros:**
- Immediate consistency - no eventual consistency surprises
- Simpler mental model for developers
- No background workers to manage
- Single transaction = atomic success/failure
- Easier debugging (linear flow)

**Cons:**
- Command latency includes projection time
- Single projection failure blocks entire command
- Harder to add new projections to existing events
- Doesn't scale to high write throughput

### Option 2: Asynchronous Projections

**Description:** Events are appended, command returns, and background workers process events to update read models.

**Pros:**
- Decoupled write and read paths
- Can add new projections without replaying in transaction
- Scales to high write throughput
- Projection failures don't block writes

**Cons:**
- Eventual consistency - reads may lag writes
- Requires background worker infrastructure
- More complex error handling (dead letter queues)
- Harder to reason about system state
- Operational overhead for self-hosters

### Option 3: Hybrid (Sync Critical, Async Optional)

**Description:** Core projections (persons, families) synchronous; optional projections (analytics, search indexes) asynchronous.

**Pros:**
- Immediate consistency for core data
- Flexibility for future projections
- Partial failure isolation

**Cons:**
- Two patterns to understand and maintain
- Complexity without clear MVP benefit
- Still requires async infrastructure for optional path

## Decision

We chose **Option 1: Synchronous Projections** for the MVP because:

1. **Single-user context** - No concurrent write contention. Projection overhead is negligible for genealogy workloads.

2. **Immediate consistency** - Users expect to see their changes immediately. "I just edited grandma's birth date, why doesn't it show?" is a poor UX.

3. **Operational simplicity** - No background workers to monitor, restart, or debug. Single-process deployment.

4. **Development velocity** - Linear request flow is easier to implement, test, and debug.

5. **Not a permanent constraint** - The projection interface doesn't change; only the timing of invocation. Migration to async is additive.

## Consequences

### Positive

- Users see changes immediately after save
- Single-process deployment (important for self-hosting)
- Simpler error handling - transaction rolls back on any failure
- Easier to test - no async timing issues

### Negative

- Command latency includes all projection updates
- Mitigation: Acceptable for MVP scale; genealogy operations are not high-frequency
- Adding new projections requires handling existing events
- Mitigation: Document projection rebuild procedure for future additions
- Projection bug can block writes
- Mitigation: Comprehensive testing; projections are straightforward data transforms

### Neutral

- All read models updated atomically with event
- Projection code lives in same process as command handlers

## When to Revisit

This decision should be reconsidered when:

1. **Multi-user support** - Concurrent writes may cause contention
2. **High-volume imports** - Bulk operations may benefit from async projections
3. **Complex projections** - If projection logic becomes expensive (e.g., full-text reindexing)
4. **External integrations** - Webhooks or sync to external systems should be async

The migration path:

1. Extract projection handlers to separate package (already done)
2. Add event position tracking to projections
3. Introduce background worker consuming from last position
4. Switch command handler to append-only (no projection call)
5. Run projections async, catch up from stored position

## Implementation Notes

```go
// internal/command/handler.go
func (h *PersonCommandHandler) Handle(ctx context.Context, cmd Command) error {
    // Validate and create events
    events := h.process(cmd)

    // Single transaction: append events + update projections
    return h.repo.WithTransaction(ctx, func(tx Transaction) error {
        if err := tx.EventStore().Append(streamID, events); err != nil {
            return err
        }
        for _, event := range events {
            if err := h.projector.Apply(tx, event); err != nil {
                return err // Rolls back event append too
            }
        }
        return nil
    })
}
```

## References

- [ADR-001: Event Sourcing with CQRS-lite](./001-event-sourcing-cqrs.md)
- [research.md - Synchronous Projections](../001-genealogy-mvp/research.md#3-event-sourcing-with-cqrs-lite)
- [Martin Fowler - CQRS](https://martinfowler.com/bliki/CQRS.html)
