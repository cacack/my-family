# Implementation Plan: [Feature Name]

**Feature Branch**: `NNN-feature-name`
**Spec**: [spec.md](./spec.md)
**Created**: YYYY-MM-DD
**Status**: Draft | Approved | In Progress | Complete

## Technical Approach

### Overview

[2-3 paragraphs describing the high-level approach]

### Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| [Decision 1] | [Choice] | [Why] |
| [Decision 2] | [Choice] | [Why] |

*For significant decisions, create an ADR in `specs/decisions/`*

## Data Model Changes

### New Entities

```
[Entity Name]
├── field1: type (description)
├── field2: type (description)
└── relationships...
```

### Schema Changes

- [ ] New Ent schema: `internal/ent/schema/xxx.go`
- [ ] Migration needed: Yes/No
- [ ] Backwards compatible: Yes/No

## API Changes

### New Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/xxx` | [Description] |
| POST | `/api/v1/xxx` | [Description] |

### OpenAPI Updates

- [ ] Update `specs/NNN-feature/contracts/openapi.yaml`
- [ ] Regenerate handlers with oapi-codegen

## Frontend Changes

### New Components

| Component | Location | Description |
|-----------|----------|-------------|
| `ComponentName.svelte` | `web/src/lib/components/` | [Description] |

### Route Changes

| Route | Component | Description |
|-------|-----------|-------------|
| `/path` | `Page.svelte` | [Description] |

## Implementation Phases

### Phase 1: [Name] (Foundation)

**Goal**: [What this phase achieves]

**Deliverables**:
- [ ] Deliverable 1
- [ ] Deliverable 2

**Verification**: [How to verify phase is complete]

### Phase 2: [Name]

[Repeat structure]

## Testing Strategy

### Unit Tests
- [ ] [Test area 1]
- [ ] [Test area 2]

### Integration Tests
- [ ] [Test area 1]

### Manual Testing
- [ ] [Scenario 1]

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| [Risk 1] | Low/Med/High | Low/Med/High | [How to handle] |

## Dependencies

### External
- [Library/service dependencies]

### Internal
- [Other features/code that must exist]

## Estimated Complexity

- **Backend**: Low / Medium / High
- **Frontend**: Low / Medium / High
- **Data Migration**: None / Simple / Complex
