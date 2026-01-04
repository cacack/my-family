<objective>
Implement surname and place browsing for GitHub issue #29.

Add browsing interfaces for surnames and places with occurrence counts,
enabling users to explore their family tree by common names and locations.
This is a core navigation feature for genealogy research.
</objective>

<context>
Project: my-family - Self-hosted genealogy software (Go backend, Svelte frontend)
Issue: #29 - Browse by surname and place
Labels: priority:high, scope:core, area:search, value:high, effort:low

Read @CLAUDE.md for project conventions.

Key existing patterns:
- API endpoints in `internal/api/openapi.yaml`
- Handlers in `internal/api/handlers.go`
- Query services in `internal/query/`
- Repository interfaces in `internal/repository/`
- SQLite impl in `internal/repository/sqlite/`
- PostgreSQL impl in `internal/repository/postgres/`
- Svelte pages in `web/src/routes/`
- SearchBox component pattern in `web/src/lib/components/SearchBox.svelte`
</context>

<requirements>
From issue #29 acceptance criteria:
- [ ] Surname list shows all surnames with counts
- [ ] Clicking surname shows matching people
- [ ] Places list shows hierarchy (Country > State > County > City)
- [ ] Can drill down through place hierarchy

UI Requirements:
- [ ] Surname browser with alphabetical quick-nav (A-Z)
- [ ] Place hierarchy as expandable tree or breadcrumb navigation
- [ ] Count badges displayed next to each item
- [ ] Responsive design for mobile/tablet
- [ ] Loading states for large lists
</requirements>

<implementation>
## Step 1: Add browse endpoints to OpenAPI

File: `internal/api/openapi.yaml`

Add endpoints:
```yaml
/browse/surnames:
  get:
    summary: Get surname index with counts
    parameters:
      - name: letter
        in: query
        schema:
          type: string
          pattern: "^[A-Z]$"
    responses:
      200:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SurnameIndexResponse'

/browse/places:
  get:
    summary: Get place hierarchy with counts
    parameters:
      - name: parent
        in: query
        schema:
          type: string
        description: Parent place to get children of (empty for top level)
    responses:
      200:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PlaceIndexResponse'
```

Add schemas:
- `SurnameIndexResponse`: `{index: [{letter, count}], surnames: [{surname, count}]}`
- `PlaceIndexResponse`: `{places: [{name, fullPath, count, hasChildren}]}`

## Step 2: Create browse service

File: `internal/query/browse_service.go` (new)

```go
type BrowseService struct {
    store repository.ReadModelStore
}

func (s *BrowseService) GetSurnameIndex(ctx context.Context) ([]LetterCount, error)
func (s *BrowseService) GetSurnamesByLetter(ctx context.Context, letter string) ([]SurnameCount, error)
func (s *BrowseService) GetPlaceHierarchy(ctx context.Context, parent string) ([]PlaceCount, error)
```

## Step 3: Add repository interface methods

File: `internal/repository/interfaces.go`

Add to ReadModelStore:
```go
GetSurnameIndex(ctx context.Context) ([]LetterCount, error)
GetSurnamesByLetter(ctx context.Context, letter string) ([]SurnameCount, error)
GetPlaceHierarchy(ctx context.Context, parent string) ([]PlaceCount, error)
```

## Step 4: Implement SQLite browse queries

File: `internal/repository/sqlite/browse.go` (new)

Surname query:
```sql
SELECT UPPER(SUBSTR(surname, 1, 1)) as letter, COUNT(*) as count
FROM persons WHERE surname != '' GROUP BY letter ORDER BY letter
```

```sql
SELECT surname, COUNT(*) as count FROM persons
WHERE UPPER(SUBSTR(surname, 1, 1)) = ? AND surname != ''
GROUP BY surname ORDER BY surname
```

Place hierarchy (parse comma-separated places):
- Top level: Extract last segment (country) from all places
- Drill-down: Filter by parent prefix, extract next segment

## Step 5: Implement PostgreSQL browse queries

File: `internal/repository/postgres/browse.go` (new)

Similar to SQLite but use PostgreSQL string functions:
- `LEFT(surname, 1)` instead of `SUBSTR`
- `SPLIT_PART` for place parsing

## Step 6: Create browse handlers

File: `internal/api/browse_handlers.go` (new)

```go
func (s *Server) HandleGetSurnameIndex(c echo.Context) error
func (s *Server) HandleGetPlaceIndex(c echo.Context) error
```

Register routes in server setup.

## Step 7: Create SurnameBrowser component

File: `web/src/lib/components/SurnameBrowser.svelte`

Features:
- A-Z letter buttons across top (horizontal scrollable on mobile)
- Highlight current letter
- Show letter counts in badges
- List surnames with counts when letter selected
- Click surname to navigate to search results
- Loading skeleton while fetching

## Step 8: Create PlaceBrowser component

File: `web/src/lib/components/PlaceBrowser.svelte`

Features:
- Breadcrumb navigation: All > USA > Illinois > Springfield
- List of child places with counts and expand indicators
- Click to drill down or view people at that place
- Back/up navigation
- Loading states

## Step 9: Update API client

File: `web/src/lib/api/client.ts`

Add methods:
```typescript
getSurnameIndex(): Promise<SurnameIndexResponse>
getSurnamesByLetter(letter: string): Promise<SurnameCount[]>
getPlaceHierarchy(parent?: string): Promise<PlaceCount[]>
```

## Step 10: Create browse pages

File: `web/src/routes/browse/surnames/+page.svelte`
File: `web/src/routes/browse/places/+page.svelte`

Wire up components with data fetching.

## Step 11: Add navigation links

Update main navigation to include Browse menu with Surnames and Places links.
</implementation>

<output>
Create/modify files:
- `./internal/api/openapi.yaml` - Add browse endpoints and schemas
- `./internal/query/browse_service.go` - New service file
- `./internal/repository/interfaces.go` - Add interface methods
- `./internal/repository/sqlite/browse.go` - SQLite implementation
- `./internal/repository/postgres/browse.go` - PostgreSQL implementation
- `./internal/api/browse_handlers.go` - New handlers
- `./web/src/lib/api/client.ts` - Add browse methods
- `./web/src/lib/components/SurnameBrowser.svelte` - New component
- `./web/src/lib/components/PlaceBrowser.svelte` - New component
- `./web/src/routes/browse/surnames/+page.svelte` - New page
- `./web/src/routes/browse/places/+page.svelte` - New page
</output>

<verification>
Before declaring complete:
- [ ] Run `go build ./...` - no compilation errors
- [ ] Run `go test ./...` - all tests pass
- [ ] Run `make check-coverage` - verify 85% threshold
- [ ] Test surname index returns correct letter counts
- [ ] Test place hierarchy returns proper tree structure
- [ ] Verify A-Z navigation works in browser
- [ ] Verify place drill-down works correctly
</verification>

<success_criteria>
- /browse/surnames endpoint returns letter index and surname counts
- /browse/places endpoint returns hierarchical place data
- SurnameBrowser shows A-Z quick nav with counts
- PlaceBrowser shows expandable hierarchy
- Clicking items navigates to filtered person results
- Both components responsive on mobile
</success_criteria>
