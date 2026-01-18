# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Self-hosted genealogy software written in Go. A premier self-hosted genealogy platform combining research rigor (GPS-compliant) with engaging storytelling, powered by a git-inspired workflow.

## Strategic Context

- [Project Ethos](./docs/ETHOS.md) - Vision, principles, success factors
- [GitHub Issues](https://github.com/cacack/my-family/issues) - Planned features and work
- [Conventions](./docs/CONVENTIONS.md) - Code patterns and standards
- [Architecture Decisions](./docs/adr/) - Key technical decisions with rationale
- [Contributing Guide](./CONTRIBUTING.md) - Development workflow

## Build Commands

```bash
go build ./...          # Build all packages
go test ./...           # Run all tests
go test -v ./... -run TestName  # Run a specific test
go fmt ./...            # Format code
go vet ./...            # Static analysis
make check-coverage     # Verify coverage thresholds (85% per-package)
make setup              # Install tools and hooks
```

**Important**: When adding new code, always run `make check-coverage` before declaring tests complete. CI enforces 85% per-package coverage.

## Commit Conventions

Use conventional commits with these types only:

| Type | Use for |
|------|---------|
| `feat` | New user-facing features |
| `fix` | User-facing bug fixes (not build/tooling) |
| `perf` | Performance improvements |
| `docs` | Documentation only |
| `refactor` | Code restructuring |
| `ci` | CI/CD and tooling |
| `chore` | Maintenance, formatting, deps, build fixes |

PR titles use descriptive format (not conventional commits). See [CONTRIBUTING.md](./CONTRIBUTING.md) for details.

## Architecture

The application uses event sourcing with CQRS-lite for a full audit trail (see [ADR-001](./docs/adr/001-event-sourcing-cqrs.md)):

```
internal/
├── domain/         # Pure domain types (Person, Family, events)
├── command/        # Command handlers (CQRS write side)
├── query/          # Query services (CQRS read side)
├── repository/     # Event store and read model persistence
│   ├── postgres/   # PostgreSQL implementation
│   └── sqlite/     # SQLite implementation
├── api/            # HTTP handlers and OpenAPI server
├── gedcom/         # GEDCOM import/export
└── config/         # Configuration
web/                # Svelte frontend
```

Key architectural decisions:
- [Event Sourcing](./docs/adr/001-event-sourcing-cqrs.md) - Full audit trail, future branching support
- [Dual Database](./docs/adr/002-dual-database-strategy.md) - PostgreSQL primary, SQLite fallback
- [Synchronous Projections](./docs/adr/003-synchronous-projections.md) - Immediate consistency for MVP
- [Single Binary](./docs/adr/004-single-binary-deployment.md) - Embedded frontend for easy deployment

## Active Technologies
- Go 1.25+ + Echo (HTTP router), Ent (data layer), oapi-codegen (OpenAPI), github.com/cacack/gedcom-go (GEDCOM processing), Svelte 5 + Vite + D3.js + Tailwind CSS (frontend)
- PostgreSQL (primary, required for future pgvector/PostGIS), SQLite (local/demo fallback)

## Linked Library Development

This project uses `github.com/cacack/gedcom-go` via a `replace` directive pointing to `/Users/chris/devel/home/gedcom-go`. When changes to gedcom-go are needed:

1. **Keep changes atomic**: Each gedcom-go enhancement should be a single logical unit (e.g., "add entity parsing", not mixed with unrelated fixes)
2. **Always add tests**: Any new gedcom-go functionality must include tests in that repo
3. **Run both test suites**: After gedcom-go changes, run `go test ./...` in both repos
4. **Commit separately**: gedcom-go changes should be committed to that repo independently, with their own descriptive commit message
5. **Document the dependency**: If adding new gedcom-go features, note what my-family feature required them
