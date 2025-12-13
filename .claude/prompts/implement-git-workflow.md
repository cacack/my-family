# Implement with Git-Inspired Workflow Support

Implement a feature with proper versioning, audit trail, and branching support.

---

## Context

Feature: $ARGUMENTS

## Instructions

When implementing this feature, ensure it supports the git-inspired workflow:

### 1. Audit Trail

Every mutation should be logged:

```go
// Example audit log entry
type AuditEntry struct {
    ID          int
    EntityType  string    // "person", "family", "source"
    EntityID    int
    Action      string    // "create", "update", "delete"
    Field       string    // which field changed (for updates)
    OldValue    *string   // previous value (JSON for complex)
    NewValue    *string   // new value
    UserID      int       // who made the change
    Timestamp   time.Time
    Note        string    // optional reason for change
}
```

### 2. Rollback Support

Design for reversibility:

- Soft deletes over hard deletes where appropriate
- Store enough history to reconstruct previous states
- Consider event sourcing for critical entities

### 3. Branch Awareness (Future)

Structure data to support research branches:

- Branch ID on mutable data
- Main branch as default
- Ability to query "as of branch X"

### 4. Change Grouping

Support logical grouping of related changes:

```go
// Changes made together (like a commit)
type ChangeSet struct {
    ID        int
    BranchID  int
    UserID    int
    Message   string      // description of changes
    Timestamp time.Time
    Entries   []AuditEntry
}
```

### 5. Diff Capability

Enable comparison between states:

- Current vs previous
- Branch A vs Branch B
- Point-in-time snapshots

## Implementation Checklist

- [ ] Are all changes logged with who/when/what?
- [ ] Can changes be reversed/rolled back?
- [ ] Is the schema branch-aware (or ready to be)?
- [ ] Can related changes be grouped logically?
- [ ] Can users see change history for any entity?

## Output

Implement the feature with versioning/audit built into the data model, not as an afterthought.
