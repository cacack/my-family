# Contributing to My Family

Thank you for your interest in contributing to the my-family genealogy platform.

## Development Setup

See [README.md](./README.md) for prerequisites and quick start.

### Quick Setup

```bash
make setup
```

This installs tools and git hooks:
- `golangci-lint` - Linting
- `go-test-coverage` - Coverage threshold checking
- Pre-commit hook (fast: format, lint, vet, tests)
- Pre-push hook (coverage thresholds)

### Git Hooks

**Pre-commit** (runs on every commit):
- Code formatting (`go fmt`)
- Linting (`golangci-lint`)
- Static analysis (`go vet`)
- Tests pass

**Pre-push** (runs before push):
- Coverage thresholds (85% per-package, 75% total)

This split keeps commits fast while catching coverage issues before CI.

### Manual Tool Installation

If not using `make setup`:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/vladopajic/go-test-coverage/v2@latest
ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
ln -sf ../../scripts/pre-push .git/hooks/pre-push
```

## Development Standards

These standards guide all development work. See [docs/CONVENTIONS.md](./docs/CONVENTIONS.md) for detailed code patterns.

### Code Quality

- Code MUST pass `go vet` and `go fmt` without warnings or changes
- All exported functions, types, and packages MUST have documentation comments
- Functions MUST have a single, clear responsibility
- Error handling MUST be explicit; never ignore returned errors
- Dependencies MUST be minimal and justified; prefer standard library

### Testing Standards

- Unit tests MUST cover core business logic (models, services)
- Integration tests MUST verify data persistence and retrieval
- Tests MUST be deterministic and not depend on external services
- Test names MUST describe the scenario being tested (e.g., `TestPerson_AddChild_DuplicateReturnsError`)
- Coverage target: 85% for core packages

### Performance Requirements

- Single record operations (add, view, edit): <100ms response time
- Bulk imports (1000 records): <10 seconds
- Search operations: <500ms for databases up to 10,000 individuals
- Memory usage: <100MB for typical family trees (<5000 individuals)

### Documentation Requirements

- README.md MUST stay current with installation and usage instructions
- Breaking changes MUST be documented in a changelog
- API changes MUST update relevant documentation before merge

## Code Standards

Follow the patterns documented in [docs/CONVENTIONS.md](./docs/CONVENTIONS.md):
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
- `feat(source): add citation confidence levels` (user-facing feature)
- `fix(gedcom): handle malformed DATE tags` (user-facing bug fix)
- `ci(deps): update golangci-lint to v1.55` (tooling, won't appear in changelog)

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
git pull origin main
git checkout -b NNN-feature-name
```

### 2. Implement and Test

```bash
# Develop the feature
go build ./...
go test ./...

# Check coverage
make check-coverage

# Format and lint
go fmt ./...
go vet ./...
```

### 3. Commit and Push

```bash
git add .
git commit -m "feat(scope): description"
git push -u origin NNN-feature-name
```

### 4. Create Pull Request

```bash
gh pr create
```

## Architecture Decision Records

For project-wide architectural decisions, use the template in [docs/adr/TEMPLATE.md](./docs/adr/TEMPLATE.md).

Existing decisions are documented in [docs/adr/](./docs/adr/).

## Linked Library: gedcom-go

This project uses `github.com/cacack/gedcom-go`. When making changes:

1. Keep changes atomic (one logical unit per commit)
2. Add tests in the gedcom-go repo
3. Run tests in both repos: `go test ./...`
4. Commit separately with descriptive messages
5. Document which my-family feature required the change

## Questions?

Open an issue or discussion on GitHub.
