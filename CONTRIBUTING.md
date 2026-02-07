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
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install github.com/vladopajic/go-test-coverage/v2@latest
ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
ln -sf ../../scripts/pre-push .git/hooks/pre-push
```

## Code Standards

Follow the patterns documented in [docs/CONVENTIONS.md](./docs/CONVENTIONS.md) - this is the **canonical source** for:
- Commit message format and types
- Go code style and naming
- Package organization
- API design patterns
- Testing conventions
- Database patterns
- Branch naming

### Key Points

- Code MUST pass `go vet` and `go fmt` without warnings or changes
- All exported functions, types, and packages MUST have documentation comments
- Error handling MUST be explicit with wrapped context; never ignore returned errors
- Dependencies MUST be minimal and justified; prefer standard library
- Coverage target: 85% for core packages (enforced by CI)
- Tests MUST be deterministic and not depend on external services

### Performance Requirements

- Single record operations (add, view, edit): <100ms response time
- Bulk imports (1000 records): <10 seconds
- Search operations: <500ms for databases up to 10,000 individuals
- Memory usage: <100MB for typical family trees (<5000 individuals)

## Feature Development Workflow

When implementing a feature from [GitHub Issues](https://github.com/cacack/my-family/issues):

### 1. Create Feature Branch

```bash
git checkout main
git pull origin main
git checkout -b feat/NNN-feature-name
```

### 2. Implement and Test

```bash
make build              # Build
make test               # Run all tests
make check-coverage     # Verify coverage thresholds
make lint               # Lint
```

### 3. Commit and Push

```bash
git add .
git commit -m "feat(scope): description"
git push -u origin feat/NNN-feature-name
```

### 4. Create Pull Request

```bash
gh pr create
```

**PR titles** use descriptive format (NOT conventional commits) to avoid duplicate changelog entries. See [CONVENTIONS.md](./docs/CONVENTIONS.md#commit-messages) for details.

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
