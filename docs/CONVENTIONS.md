# Conventions

Code patterns, standards, and practices for my-family development.

This file is the **canonical source** for conventions. CLAUDE.md and CONTRIBUTING.md reference this file rather than duplicating details.

## Branch Naming

```
feat/NNN-feature-name    # Feature branches (NNN = GitHub issue number)
fix/NNN-bug-description  # Bug fix branches
```

- `NNN` = GitHub issue number
- Use kebab-case short description
- Examples: `feat/91-name-variants-ui`, `fix/142-date-parsing`

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

[optional body]

[optional footer]
```

**Types** (only these 7 are used):

| Type | Use for | In Changelog? |
|------|---------|---------------|
| `feat` | New user-facing features | Yes |
| `fix` | User-facing bug fixes (not build/tooling) | Yes |
| `perf` | Performance improvements | Yes |
| `docs` | Documentation only | No |
| `refactor` | Code restructuring (no behavior change) | No |
| `ci` | CI/CD, dev infrastructure, tooling | No |
| `chore` | Maintenance, formatting, deps, build fixes | No |

**Note**: `feat` and `fix` are reserved for user-facing changes.

**PR titles** use descriptive format (NOT conventional commits) to avoid duplicate changelog entries with release-please.

**Examples:**
```
feat(gedcom): add support for GEDCOM 7.0 media cropping
fix(api): handle empty surname in person creation
refactor(query): extract date parsing to shared utility
chore(deps): bump svelte from 5.48.3 to 5.49.1
```

## Go Code Style

### Beyond `go fmt`

- Use meaningful variable names (not single letters except in tight loops)
- Prefer early returns over deep nesting
- Group related declarations
- Order: constants, types, variables, functions

### Error Handling

```go
// DO: Wrap errors with context
if err != nil {
    return fmt.Errorf("importing person %s: %w", name, err)
}

// DON'T: Lose context
if err != nil {
    return err
}
```

### Naming

```go
// Exported types: PascalCase
type PersonService struct {}

// Unexported: camelCase
type personRepository struct {}

// Interfaces: describe behavior, often -er suffix
type PersonReader interface {}

// Constructors: NewXxx
func NewPersonService() *PersonService {}
```

### Package Organization

```
internal/
├── api/            # HTTP handlers, OpenAPI spec, generated server code
├── command/        # Command handlers (CQRS write side)
├── config/         # Configuration
├── domain/         # Pure domain types (Person, Family, events, enums)
├── exporter/       # JSON/CSV export
├── gedcom/         # GEDCOM import/export (uses gedcom-go library)
├── media/          # Thumbnail generation
├── query/          # Query services (CQRS read side)
├── repository/     # Interfaces (EventStore, ReadModelStore) + shared code
│   ├── memory/     # In-memory implementation (tests, demo mode)
│   ├── postgres/   # PostgreSQL implementation
│   └── sqlite/     # SQLite implementation
├── storage/        # Backend selection: config -> memory/SQLite/PostgreSQL stores (ADR-002)
└── web/            # Embedded frontend assets
```

## API Design

### REST Conventions

- Use plural nouns for collections: `/api/v1/persons`, `/api/v1/families`
- Use kebab-case for multi-word paths: `/api/v1/family-trees`
- Standard HTTP methods: GET (read), POST (create), PUT (replace), PATCH (update), DELETE
- Return appropriate status codes: 200, 201, 204, 400, 404, 500

### Response Format

```json
{
  "data": { ... },
  "meta": {
    "total": 100,
    "page": 1,
    "per_page": 20
  }
}
```

### Error Response

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human readable message",
    "details": [
      { "field": "birthDate", "message": "Invalid date format" }
    ]
  }
}
```

### OpenAPI Spec

- `internal/api/openapi.yaml` is the source of truth for the API contract
- Go server code generated via `make generate-api` (oapi-codegen)
- TypeScript types generated via `make generate-types`
- Never hand-edit `internal/api/generated.go` or `web/src/lib/api/types.generated.ts`

## Frontend (Svelte)

### Component Organization

```
web/src/
├── lib/
│   ├── components/    # Reusable UI components
│   │   ├── export/    # Export-related components
│   │   └── ...        # Domain components (PersonCard, FamilyCard, etc.)
│   ├── api/           # API client + generated types
│   └── utils/         # Helper functions
├── routes/            # SvelteKit routes (pages)
└── app.css            # Global styles (Tailwind)
```

### Naming

- Components: PascalCase (`PersonCard.svelte`)
- Utilities: camelCase (`formatDate.ts`)

### State Management

- Use Svelte stores for shared state
- Keep component state local when possible

### Main Navigation

- `web/src/lib/navigation.ts` is the single source for header destinations; `MainNav.svelte` renders it inline at ≥1024px and as one sectioned Menu below.
- Add a new page to the group that matches its user intent (Browse, Research, History) rather than as a new top-level link; the header must stay one row at 1024px (`web/e2e/layout.spec.ts`).
- Use `match` for nested routes that live outside a link's own path; highlighting follows the group, not the URL prefix.

### UI Building Blocks

- **Page header**: a page with its own heading uses `PageHeader` (`title`, optional `description`, `back` (`{ href, label }`), an `actions` snippet, and `children` for rich notes). Don't hand-build an h1 + actions row. Entity detail pages (a back link plus Edit/Delete) and chart toolbars (pedigree, descendancy, Ahnentafel) still have their own bars.
- **Form row**: fields that sit side by side go in `FormRow`; pass `minColumnWidth` (default `14rem`) for rows of short fields. It spaces itself `1rem` below, so don't also put it in a flex/grid `gap` container. Don't redefine `.form-row` in a component.
- **Detail section**: a read-only block on a detail page is a `.info-section` with an `h2` label (small, uppercase, muted) over a `dl` of `dt`/`dd` pairs; empty values show `—`.
- **Buttons**: use shadcn `Button` for every control that looks like a button. `default` is the page's main action, `outline` a secondary action, `destructive` deletes, `warning` brick-wall changes. Use `ghost` only where its surroundings mark it as a control (a toolbar or menu), never on its own between sections.
- **Responsive**: toolbars and action rows `flex-wrap: wrap`; grids use `repeat(auto-fit, minmax(min(<width>, 100%), 1fr))`, not fixed `repeat(N, 1fr)`; long user text (names, titles, notes) gets `overflow-wrap: anywhere`. `web/e2e/layout.spec.ts` fails any page that scrolls sideways at 375–1440px.

## Testing

### Go Tests

```go
// Table-driven tests preferred
func TestParseDate(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Date
        wantErr bool
    }{
        {"exact date", "15 MAR 1842", Date{Day: 15, Month: 3, Year: 1842}, false},
        {"circa date", "ABT 1850", Date{Year: 1850, Approximate: true}, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### Test File Location

- Unit tests: Same package, `_test.go` suffix
- Integration tests: co-located in the package under test; cross-backend scenarios live in
  `internal/integration/`. No build tags — tests needing Docker skip themselves when it is absent.

### Frontend Tests

- Component tests with Testing Library (Vitest, `web/src/**/*.test.ts`)
- E2E tests with Playwright (`web/e2e/`), run via `make test-e2e` — opt-in, not part of `make test`

## Database

### Event Store + Read Model Pattern

This project does **not** use an ORM. Data access follows the event sourcing pattern:

- **EventStore** interface (`repository/eventstore.go`): Append-only event storage with optimistic concurrency
- **ReadModelStore** interface (`repository/readmodel.go`): Denormalized read models projected from events
- **Projections** (`repository/projection.go`): Synchronous handlers that update read models when events are appended

Both PostgreSQL and SQLite implement these interfaces identically. The `memory/` implementation is for tests.

### Migrations

- Schema DDL is embedded in each database implementation
- Both implementations must pass the same shared test suite (invariant DB-001)

## Documentation

### Code Comments

- Document *why*, not *what*
- All exported functions need doc comments
- Use `// TODO:` for future work (with context)

## Git Workflow

### Feature Development

1. Branch from `main`: `git checkout -b feat/NNN-feature-name`
2. Develop with atomic commits
3. Ensure tests pass: `make test`
4. Create PR when ready
5. Squash merge to main

### PR Checklist

- [ ] Tests added/updated
- [ ] Documentation updated if needed
- [ ] No commented-out code
- [ ] Linter passes (`make lint`)
- [ ] Build succeeds (`make build`)
- [ ] Coverage meets thresholds (`make check-coverage`)

---

## Related

- [ETHOS.md](./ETHOS.md) - Principles guiding these conventions
- [adr/](./adr/) - Architectural decisions
- [../CONTRIBUTING.md](../CONTRIBUTING.md) - Full contributor workflow
- [../CLAUDE.md](../CLAUDE.md) - Claude Code guidance
