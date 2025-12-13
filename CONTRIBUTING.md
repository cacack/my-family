# Contributing to My Family

Thank you for your interest in contributing to the my-family genealogy platform.

## Development Setup

See [README.md](./README.md) for prerequisites and quick start.

## Code Standards

Follow the patterns documented in [specs/CONVENTIONS.md](./specs/CONVENTIONS.md):
- Go: standard formatting (`go fmt`), idiomatic error handling
- Git: conventional commits, feature branches
- API: OpenAPI-first design
- Frontend: Svelte 5 patterns, Tailwind CSS

## Feature Development Workflow

When implementing a backlog item from [BACKLOG.md](./specs/BACKLOG.md):

### 1. Create Feature Branch

```bash
git checkout main
git checkout -b NNN-feature-name
mkdir -p specs/NNN-feature-name
cp -r specs/TEMPLATE-feature/* specs/NNN-feature-name/
```

### 2. Specification Pipeline

```bash
# Research (optional but recommended)
# Document findings in specs/NNN-feature-name/research.md

# Specify requirements
/speckit.specify
# Output: specs/NNN-feature-name/spec.md

# Clarify ambiguities
/speckit.clarify

# Plan implementation
/speckit.plan
# Output: specs/NNN-feature-name/plan.md

# Generate tasks
/speckit.tasks
# Output: specs/NNN-feature-name/tasks.md
```

### 3. Implementation

```bash
/speckit.implement
```

Use meta-prompts for quality (in `.claude/prompts/`):

| Prompt | Purpose |
|--------|---------|
| `research-feature` | Research before implementing |
| `implement-with-gps` | Add source/citation/evidence support |
| `implement-git-workflow` | Add versioning/audit trail |
| `review-accessibility` | Check a11y compliance |
| `write-tests` | Generate tests following patterns |
| `bring-to-life` | Enhance engagement/storytelling |

### 4. Validate & Ship

```bash
/speckit.analyze      # Cross-artifact consistency check
go test ./...         # Run all tests
cd web && npm test    # Frontend tests
gh pr create          # Create pull request
```

## Template Reference

Feature specs use the template in `specs/TEMPLATE-feature/`:

| File | Purpose |
|------|---------|
| `spec.md` | User stories, acceptance criteria |
| `plan.md` | Architecture, data model, phases |
| `tasks.md` | Actionable implementation tasks |
| `research.md` | Prior art, standards research |
| `decisions.md` | Feature-specific ADRs |

## Architecture Decision Records

For project-wide decisions, use the template in `specs/decisions/TEMPLATE.md`.

## Linked Library: gedcom-go

This project uses `github.com/cacack/gedcom-go`. When making changes:

1. Keep changes atomic (one logical unit per commit)
2. Add tests in the gedcom-go repo
3. Run tests in both repos: `go test ./...`
4. Commit separately with descriptive messages
5. Document which my-family feature required the change

## Questions?

Open an issue or discussion on GitHub.
