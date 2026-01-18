# Conventions

Code patterns, standards, and practices for my-family development.

## Branch Naming

```
NNN-feature-name
```

- `NNN` = Three-digit feature number from backlog (e.g., `001`, `002`)
- `feature-name` = Kebab-case short description
- Examples: `001-genealogy-mvp`, `002-media-management`, `003-source-citations`

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

[optional body]

[optional footer]
```

**Types:**
- `feat` - New feature
- `fix` - Bug fix
- `docs` - Documentation only
- `refactor` - Code change that neither fixes a bug nor adds a feature
- `test` - Adding or correcting tests
- `chore` - Build process, dependencies, tooling

**Examples:**
```
feat(gedcom): add support for GEDCOM 7.0 media cropping
fix(api): handle empty surname in person creation
refactor(ent): extract date parsing to shared utility
test(import): add integration test for large GEDCOM files
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
├── api/          # HTTP handlers, OpenAPI-generated code
├── ent/          # Ent schema and generated code
│   └── schema/   # Entity definitions
├── gedcom/       # GEDCOM processing (uses gedcom-go library)
├── service/      # Business logic layer
└── repository/   # Data access (if separating from Ent)
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

## Frontend (Svelte)

### Component Organization

```
web/src/
├── lib/
│   ├── components/    # Reusable UI components
│   │   ├── ui/        # Generic (Button, Input, Modal)
│   │   └── genealogy/ # Domain-specific (PersonCard, FamilyTree)
│   ├── stores/        # Svelte stores for state
│   ├── api/           # API client functions
│   └── utils/         # Helper functions
├── routes/            # SvelteKit routes (pages)
└── app.css            # Global styles (Tailwind)
```

### Naming

- Components: PascalCase (`PersonCard.svelte`)
- Stores: camelCase with `$` prefix convention (`$personStore`)
- Utilities: camelCase (`formatDate.ts`)

### State Management

- Use Svelte stores for shared state
- Keep component state local when possible
- API state: consider using TanStack Query or similar

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
- Integration tests: `*_integration_test.go` with build tag if needed

### Frontend Tests

- Component tests with Testing Library
- E2E tests with Playwright for critical paths

## Database

### Ent Schema Conventions

```go
// Fields: use descriptive names, add comments
field.String("given_name").
    Comment("First/given name(s)").
    Optional(),

// Edges: name from the perspective of the entity
edge.To("children", Person.Type),
edge.From("parents", Person.Type).Ref("children"),
```

### Migrations

- Ent auto-migration for development
- Versioned migrations for production (Atlas or manual)

## Documentation

### Code Comments

- Document *why*, not *what*
- All exported functions need doc comments
- Use `// TODO:` for future work (with context)

### API Documentation

- OpenAPI spec is the source of truth
- Keep `openapi.yaml` in sync with implementation
- Generate handlers with oapi-codegen

## Git Workflow

### Feature Development

1. Branch from `main`: `git checkout -b NNN-feature-name`
2. Develop with atomic commits
3. Ensure tests pass: `go test ./...`
4. Create PR when ready
5. Squash merge to main

### PR Checklist

- [ ] Tests added/updated
- [ ] Documentation updated if needed
- [ ] No commented-out code
- [ ] Linter passes (`go vet ./...`)
- [ ] Build succeeds (`go build ./...`)

---

## Related

- [ETHOS.md](./ETHOS.md) - Principles guiding these conventions
- [adr/](./adr/) - Architectural decisions
- [../CONTRIBUTING.md](../CONTRIBUTING.md) - Full contributor workflow
- [../CLAUDE.md](../CLAUDE.md) - Claude Code guidance
