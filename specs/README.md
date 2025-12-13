# Specifications

Project planning, design artifacts, and architectural decisions for my-family.

## Contents

| Document | Purpose |
|----------|---------|
| [ETHOS.md](./ETHOS.md) | Vision, core differentiators, success factors |
| [BACKLOG.md](./BACKLOG.md) | Prioritized feature list with promotion pipeline |
| [CONVENTIONS.md](./CONVENTIONS.md) | Code patterns, Git workflow, API design |

## Directories

| Directory | Purpose |
|-----------|---------|
| [decisions/](./decisions/) | Project-wide Architecture Decision Records (ADRs) |
| [TEMPLATE-feature/](./TEMPLATE-feature/) | Template for new feature specifications |
| [001-genealogy-mvp/](./001-genealogy-mvp/) | MVP feature specification |

## Feature Specification Workflow

When promoting a backlog item to implementation:

```bash
# 1. Create branch and spec folder
git checkout -b NNN-feature-name
cp -r specs/TEMPLATE-feature/* specs/NNN-feature-name/

# 2. Run speckit pipeline
/speckit.specify   # Define requirements → spec.md
/speckit.clarify   # Resolve ambiguities
/speckit.plan      # Design approach → plan.md
/speckit.tasks     # Break into tasks → tasks.md
/speckit.implement # Execute tasks

# 3. Validate
/speckit.analyze   # Cross-artifact consistency
go test ./...      # Run tests
```

See [BACKLOG.md](./BACKLOG.md#promoting-a-feature-to-implementation) for the complete workflow.

## Related

- [CONTRIBUTING.md](../CONTRIBUTING.md) - Developer workflow guide
- [CLAUDE.md](../CLAUDE.md) - Claude Code guidance
- [.claude/prompts/](../.claude/prompts/) - Meta-prompts for quality enhancement
