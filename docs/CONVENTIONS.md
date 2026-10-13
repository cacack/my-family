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
- Each enum in `internal/domain/enums.go` has a named schema of the same name that properties `$ref`; `internal/api/enums_contract_test.go` fails when the two value sets diverge
- `web/src/lib/api/client.ts` exports aliases of the generated types, never hand-written request/response shapes
- Option lists for enum-valued form controls are typed against the generated unions (`web/src/lib/utils/enumOptions.ts`), so an invalid option fails `svelte-check`

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
- View state a user would expect Back, reload or a shared link to keep (search, list sort/filter/page, tabs, chart options) lives in URL search params: read it from `$page.url` with the `readEnum`/`readPositiveInt`/`readFlag` helpers in `web/src/lib/utils/urlState.ts` and change it only through `setQuery`. Pass `push: true` for navigation-level changes (page, tab, submitted search); the default replace is for refinements (sort, filters, options). Leave defaults out of the URL. When a form is a draft until submitted (search), a refinement changes only its own param and keeps the submitted criteria from the URL; it never reads the unsubmitted form fields. On a tabbed page, `page` and the tab's filter belong to the active tab, so a tab switch drops them. A typed filter keeps its text in the input and writes the URL with a replace (after the debounce, if any); refill the input only when the URL changes from elsewhere (Back, a link), never from the value the page itself just wrote.

### Main Navigation

- `web/src/lib/navigation.ts` is the single source for header destinations; `MainNav.svelte` renders it inline at ≥1024px and as one sectioned Menu below.
- Add a new page to the group that matches its user intent (Browse, Research, History) rather than as a new top-level link; the header must stay one row at 1024px, including with a long research branch name active (`web/e2e/layout.spec.ts`).
- Use `match` for nested routes that live outside a link's own path; highlighting follows the group, not the URL prefix.

### UI Building Blocks

- **Page header**: a page with its own heading uses `PageHeader` (`title`, optional `description`, `back` (`{ href, label }`), a `badges` snippet for status badges beside the title, an `actions` snippet, and `children` for rich notes); chart pages (pedigree, descendancy) put their controls in `actions`. `PageHeader` prints, back link and actions included. A page whose heading lives in its body (entity detail pages, the Ahnentafel report, the family group sheet) uses `ActionBar` (`back` plus an `actions` snippet, such as Edit/Delete), which never prints. Don't hand-build an h1 + actions row or a back link + actions bar.
- **Form row**: fields that sit side by side go in `FormRow`; pass `minColumnWidth` (default `14rem`) for rows of short fields. It spaces itself `1rem` below, so don't also put it in a flex/grid `gap` container. Don't redefine `.form-row` in a component.
- **Required marker**: in a column-layout `<label>` (text above input), wrap the label text and its marker in one `<span>` (`<span>Title <span class="required">*</span></span>`) so the `*` stays on the text's line instead of becoming its own flex item.
- **Detail section**: a read-only block on a detail page is a `.info-section` with an `h2` label (small, uppercase, muted) over a `dl` of `dt`/`dd` pairs; empty values show `—`.
- **Buttons**: use shadcn `Button` for every control that looks like a button. `default` is the page's main action, `outline` a secondary action, `destructive` deletes, `warning` brick-wall changes. Use `ghost` only where its surroundings mark it as a control (a toolbar or menu), never on its own between sections.
- **Errors**: a failed load uses `ErrorState` (message + Retry; it replaces the content). A failed save, delete or other action uses `ActionError` inline and never replaces the page or the user's input. A whole-record save (person, family, source, repository and the evidence detail pages) refused with a 409 whose code is `VERSION_CONFLICT` (check with `isVersionConflict`) uses `SaveConflict`, which compares the user's edits with the latest version, driven by the shared `SaveConflictFlow` (`$lib/utils/saveConflict.svelte`) rather than per-page keep/discard logic; any other 409 is a refusal and shows its message in `ActionError`; `ConflictError` is only for sub-resource writes the client already retries. A fetch by id maps its error with `recordLoadError`, so a malformed id reads as "not found".
- **Responsive**: toolbars and action rows `flex-wrap: wrap`; grids use `repeat(auto-fit, minmax(min(<width>, 100%), 1fr))`, not fixed `repeat(N, 1fr)` (`web/src/lib/responsiveGrid.test.ts` fails one unless a `fixed columns:` comment on it or the line above says why); long user text (names, titles, notes) gets `overflow-wrap: anywhere`. `web/e2e/layout.spec.ts` fails any page that scrolls sideways at 375–1440px, and again at 375px with the Larger (150%) font setting.
- **Tree charts** (D3 pan/zoom, `web/src/lib/utils/chart.ts`): open with `fitTransform` clamped to `FIT_SCALE_RANGE`, so a big tree scrolls at a readable size instead of shrinking; chart text is at least `MIN_CARD_FONT_PX`. Call `touchScrollsPage(svg)` after `svg.call(zoom)` so one finger scrolls the page and two fingers pan the chart. Tap targets on chart nodes get an invisible hit circle (radius 14+) rather than a bigger drawn control. A page whose chart fills the viewport adds its route ID to `FULL_HEIGHT_ROUTES` in `web/src/routes/+layout.svelte`.
- **High contrast**: a light scheme (black on white, `#0000ee` links) defined once, in the `body.high-contrast` block of `web/src/app.css`; it must override every colour token `:root` declares, shadcn ones included. Don't set custom properties under `body.high-contrast` in a component (`web/src/lib/highContrast.test.ts` fails it); use the tokens instead of hard-coded colours. Every `<a>` there is an underlined link-blue link; a link that is a component part rather than a text link (a card, tile, nav item or button-styled link) carries `data-slot`, as `Button` and the shadcn menu items already do, and keeps its own colours. Only the attribute's presence is read; outside `ui/` use `card-link`, `action-link`, `nav-link` or `chip`, and `highContrast.test.ts` fails a card-, button-, chip- or tile-classed `<a>` without it. `web/e2e/high-contrast.spec.ts` runs axe `color-contrast` on the main pages and checks link styles.
- **Muted and accent colour**: secondary text uses `var(--color-text-muted)` (Tailwind `text-muted-foreground`), links and accent text `var(--color-link)` (`text-link`), and accent fills behind white text `var(--color-action)`. Don't hardcode slate or blue hex for text (icons follow the non-text rule below; border and chart colours are out of scope); the tokens in `web/src/app.css` are chosen to pass WCAG AA on every light surface. `web/e2e/a11y.spec.ts` fails any serious or critical axe finding on each route as loaded, at 375 and 1280px; states reached by interaction are not checked, so `web/src/lib/textContrast.test.ts` statically fails `text-slate-300/400/500`, `text-gray-400/500` and `color: #94a3b8`/`#cbd5e1` in any component (rules whose selector names an `svg` or an `icon` class, and `dark:` variants, are exempt). Every page the guards visit is listed in `web/e2e/routes.ts`.
- **Non-text colour**: an icon, chevron or focus indicator that shows state or affordance reaches 3:1 (WCAG 1.4.11) against the surface it sits on, as text reaches 4.5:1 above. Neutral icons use `var(--color-text-muted)`, focus outlines `var(--color-focus-ring)` and success icons `var(--color-success)`; the non-text tokens in `web/src/app.css` pass on every light surface. Purely decorative graphics, such as empty-state illustrations, are exempt. axe does not check non-text contrast, so `web/src/lib/nonTextContrast.test.ts` checks the tokens and statically fails an icon, svg or chevron colour, or a focus outline, below 3:1 on white in a component's `<style>` block (Tailwind classes and inline `style=` are not checked).

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
