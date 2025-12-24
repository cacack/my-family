# Contributing to My Family

Thank you for your interest in contributing to the my-family genealogy platform.

## Development Setup

See [README.md](./README.md) for prerequisites and quick start.

### Pre-commit Hook (Recommended)

Install the pre-commit hook to catch issues before committing:

```bash
ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
```

The hook checks:
- Code formatting (`go fmt`)
- Linting (`golangci-lint`)
- Static analysis (`go vet`)
- Tests pass
- Per-package coverage (minimum 85% per package)

**Tool discovery**: The hook automatically finds `golangci-lint` in PATH, `~/go/bin`, or `$(go env GOPATH)/bin`.

Install golangci-lint if needed:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Code Standards

Follow the patterns documented in [specs/CONVENTIONS.md](./specs/CONVENTIONS.md):
- Go: standard formatting (`go fmt`), idiomatic error handling
- Git: conventional commits, feature branches
- API: OpenAPI-first design
- Frontend: Svelte 5 patterns, Tailwind CSS

### Commit Conventions

We use conventional commits to maintain clear project history and automate changelog generation.

#### Commit Types

| Type | Use for | Appears in Changelog? |
|------|---------|----------------------|
| `feat` | New user-facing features | Yes |
| `fix` | Bug fixes | Yes |
| `perf` | Performance improvements | Yes |
| `docs` | Documentation only | No |
| `refactor` | Code restructuring (no behavior change) | No |
| `ci` | CI/CD, dev infrastructure, tooling | No |
| `chore` | Maintenance, formatting, dependencies | No |

**Note**: Only these 7 types are used. `feat` and `fix` are reserved for user-facing changes.

Examples:
- `feat(source): add citation confidence levels` ✅ (user-facing feature)
- `fix(gedcom): handle malformed DATE tags` ✅ (user-facing bug fix)
- `ci(deps): update golangci-lint to v1.55` ✅ (tooling, won't appear in changelog)
- `feat(ci): add coverage gate` ❌ (should be `ci(coverage): add threshold check`)

#### Commit Messages vs PR Titles

To avoid duplicate changelog entries, use different formats:

| Where | Format | Example |
|-------|--------|---------|
| Commit messages | `type(scope): description` | `feat(parser): add date support` |
| PR titles | Descriptive (NOT conventional commit) | `Add date support` |

**Why?** Release-please uses merge commits for semi-linear history. Using conventional commit format in both PR titles and commit messages creates duplicate changelog entries.

## Feature Development Workflow

When implementing a feature from [GitHub Issues](https://github.com/cacack/my-family/issues):

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
