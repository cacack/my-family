# Meta-Prompts

Quality enhancement prompts for Claude Code during feature implementation.

## Available Prompts

| Prompt | Purpose |
|--------|---------|
| [research-feature.md](./research-feature.md) | Research prior art before specifying a feature |
| [implement-with-gps.md](./implement-with-gps.md) | Implement GPS-compliant source/citation/evidence support |
| [implement-git-workflow.md](./implement-git-workflow.md) | Add versioning, audit trail, branching capabilities |
| [review-accessibility.md](./review-accessibility.md) | Check components for a11y compliance |
| [write-tests.md](./write-tests.md) | Generate tests following project patterns |
| [bring-to-life.md](./bring-to-life.md) | Enhance engagement and storytelling features |

## Usage

Use with `/create-prompt` during feature implementation:

```bash
# Research a feature
/create-prompt research-feature "002-media-management"

# Implement with GPS compliance
/create-prompt implement-with-gps "003-source-citations"

# Review accessibility
/create-prompt review-accessibility "PersonCard"

# Generate tests
/create-prompt write-tests "internal/service/person"
```

## When to Use

- **research-feature**: Before `/speckit.specify`, to understand prior art and standards
- **implement-with-gps**: For features involving sources, citations, evidence analysis
- **implement-git-workflow**: For features needing versioning, history, rollback
- **review-accessibility**: After UI implementation, before PR
- **write-tests**: After implementation, to ensure coverage
- **bring-to-life**: For features with narrative, timeline, or engagement aspects

## Related

- [../../specs/ETHOS.md](../../specs/ETHOS.md) - Core differentiators these prompts support
- [../../specs/BACKLOG.md](../../specs/BACKLOG.md) - Feature pipeline using these prompts
- [../../CONTRIBUTING.md](../../CONTRIBUTING.md) - Development workflow
