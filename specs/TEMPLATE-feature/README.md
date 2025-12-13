# Feature Specification Template

Copy this directory when starting a new feature from the backlog.

## Usage

```bash
# From repo root
cp -r specs/TEMPLATE-feature/* specs/NNN-feature-name/
```

## Files

| File | Purpose | Created By |
|------|---------|------------|
| [spec.md](./spec.md) | User stories, acceptance criteria, requirements | `/speckit.specify` |
| [plan.md](./plan.md) | Architecture, data model, implementation phases | `/speckit.plan` |
| [tasks.md](./tasks.md) | Actionable tasks with verification criteria | `/speckit.tasks` |
| [research.md](./research.md) | Prior art, standards research, findings | Manual or meta-prompt |
| [decisions.md](./decisions.md) | Feature-specific ADRs | As needed |

## Workflow

1. **Research** (optional): Investigate prior art, document in `research.md`
2. **Specify**: Run `/speckit.specify` to create `spec.md`
3. **Clarify**: Run `/speckit.clarify` to resolve ambiguities
4. **Plan**: Run `/speckit.plan` to create `plan.md`
5. **Tasks**: Run `/speckit.tasks` to create `tasks.md`
6. **Implement**: Run `/speckit.implement` to execute tasks

## Quality Meta-Prompts

Enhance implementation with meta-prompts from `.claude/prompts/`:

| Prompt | Use When |
|--------|----------|
| `research-feature` | Before specifying, to understand prior art |
| `implement-with-gps` | Feature involves sources, citations, evidence |
| `implement-git-workflow` | Feature needs versioning, audit trail |
| `review-accessibility` | UI components need a11y review |
| `write-tests` | Generate tests following project patterns |
| `bring-to-life` | Enhance engagement, storytelling aspects |

## Related

- [../BACKLOG.md](../BACKLOG.md) - Feature backlog with promotion pipeline
- [../decisions/](../decisions/) - Project-wide ADRs
- [../../CONTRIBUTING.md](../../CONTRIBUTING.md) - Full developer workflow
