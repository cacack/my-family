<objective>
Implement family group sheet view for GitHub issue #26.

Add a traditional genealogical family group sheet format showing a family unit
with parents and children, key events, and source citations. This is a standard
format used by genealogists worldwide for documenting family units.
</objective>

<context>
Project: my-family - Self-hosted genealogy software (Go backend, Svelte frontend)
Issue: #26 - Family group sheet view
Labels: priority:high, scope:core, area:visualization, value:high, effort:low

Read @CLAUDE.md for project conventions.

Key existing patterns:
- Family entity in `internal/domain/family.go`
- Family handlers in `internal/api/`
- Family detail page in `web/src/routes/families/[id]/`
- Citation display in `web/src/lib/components/CitationSection.svelte`
- Print styling can follow existing component patterns
</context>

<requirements>
From issue #26 acceptance criteria:
- [ ] Family group sheet displays correctly
- [ ] Shows all family members with key events
- [ ] Navigable to individual person details
- [ ] Printable version available

Standard family group sheet layout:
- Husband/Father section at top
- Wife/Mother section below
- Children listed in birth order
- Key events: Birth, Marriage, Death for each person
- Source citations for documented facts
</requirements>

<implementation>
## Step 1: Add group-sheet endpoint to OpenAPI

File: `internal/api/openapi.yaml`

Add endpoint:
```yaml
/families/{id}/group-sheet:
  get:
    summary: Get family group sheet data
    parameters:
      - name: id
        in: path
        required: true
        schema:
          type: string
          format: uuid
    responses:
      200:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/FamilyGroupSheetResponse'
```

Add schema:
```yaml
FamilyGroupSheetResponse:
  type: object
  properties:
    family_id:
      type: string
      format: uuid
    husband:
      $ref: '#/components/schemas/GroupSheetPerson'
    wife:
      $ref: '#/components/schemas/GroupSheetPerson'
    marriage:
      $ref: '#/components/schemas/GroupSheetEvent'
    children:
      type: array
      items:
        $ref: '#/components/schemas/GroupSheetChild'

GroupSheetPerson:
  type: object
  properties:
    id:
      type: string
      format: uuid
    full_name:
      type: string
    birth:
      $ref: '#/components/schemas/GroupSheetEvent'
    death:
      $ref: '#/components/schemas/GroupSheetEvent'
    father_name:
      type: string
    mother_name:
      type: string

GroupSheetEvent:
  type: object
  properties:
    date:
      type: string
    place:
      type: string
    citations:
      type: array
      items:
        $ref: '#/components/schemas/CitationBrief'

GroupSheetChild:
  allOf:
    - $ref: '#/components/schemas/GroupSheetPerson'
    - type: object
      properties:
        birth_order:
          type: integer
        relationship_type:
          type: string
        spouse_name:
          type: string
```

## Step 2: Create group sheet service method

File: `internal/query/family_service.go`

Add method:
```go
func (s *FamilyService) GetGroupSheet(ctx context.Context, familyID uuid.UUID) (*GroupSheetData, error)
```

This method should:
- Fetch family with partners
- Fetch person details for each partner (birth, death, parents)
- Fetch children with their details
- Fetch marriage event and citations
- Aggregate all data into GroupSheetData struct

## Step 3: Create group sheet handler

File: `internal/api/family_handlers.go`

Add handler:
```go
func (s *Server) HandleGetFamilyGroupSheet(c echo.Context) error {
    // Parse family ID
    // Call service.GetGroupSheet
    // Return JSON response
}
```

Register route: `GET /families/:id/group-sheet`

## Step 4: Update API client

File: `web/src/lib/api/client.ts`

Add types:
```typescript
interface GroupSheetPerson {
  id: string;
  full_name: string;
  birth?: GroupSheetEvent;
  death?: GroupSheetEvent;
  father_name?: string;
  mother_name?: string;
}

interface GroupSheetEvent {
  date?: string;
  place?: string;
  citations?: CitationBrief[];
}

interface FamilyGroupSheet {
  family_id: string;
  husband?: GroupSheetPerson;
  wife?: GroupSheetPerson;
  marriage?: GroupSheetEvent;
  children: GroupSheetChild[];
}
```

Add method:
```typescript
getFamilyGroupSheet(id: string): Promise<FamilyGroupSheet>
```

## Step 5: Create FamilyGroupSheet component

File: `web/src/lib/components/FamilyGroupSheet.svelte`

Layout structure:
```
+------------------------------------------+
|           FAMILY GROUP SHEET             |
+------------------------------------------+
| HUSBAND                                  |
| Name: [clickable link]                   |
| Birth: [date] at [place]    [citations]  |
| Death: [date] at [place]    [citations]  |
| Father: [name]  Mother: [name]           |
+------------------------------------------+
| WIFE                                     |
| Name: [clickable link]                   |
| Birth: [date] at [place]    [citations]  |
| Death: [date] at [place]    [citations]  |
| Father: [name]  Mother: [name]           |
+------------------------------------------+
| MARRIAGE                                 |
| Date: [date]  Place: [place] [citations] |
+------------------------------------------+
| CHILDREN                                 |
| # | Name | Birth | Death | Spouse       |
| 1 | ...  | ...   | ...   | ...          |
| 2 | ...  | ...   | ...   | ...          |
+------------------------------------------+
```

Features:
- Clean, traditional layout
- Names are clickable links to person detail
- Citation indicators (small numbers or icons)
- Hover/click to see full citation
- Responsive: stack sections vertically on mobile

## Step 6: Add print styles

File: `web/src/lib/components/FamilyGroupSheet.svelte` (scoped styles)

Or add to `web/src/app.css`:

```css
@media print {
  .family-group-sheet {
    font-family: serif;
    font-size: 11pt;
    color: black;
    background: white;
  }

  .family-group-sheet a {
    color: black;
    text-decoration: none;
  }

  .family-group-sheet .no-print {
    display: none;
  }

  .family-group-sheet table {
    border-collapse: collapse;
    width: 100%;
  }

  .family-group-sheet th,
  .family-group-sheet td {
    border: 1px solid #333;
    padding: 4px 8px;
  }
}
```

## Step 7: Create group sheet page

File: `web/src/routes/families/[id]/group-sheet/+page.svelte`

```svelte
<script>
  import { page } from '$app/stores';
  import { getFamilyGroupSheet } from '$lib/api/client';
  import FamilyGroupSheet from '$lib/components/FamilyGroupSheet.svelte';

  const familyId = $page.params.id;
  let groupSheet = $state(null);
  let loading = $state(true);

  $effect(() => {
    getFamilyGroupSheet(familyId).then(data => {
      groupSheet = data;
      loading = false;
    });
  });
</script>

<div class="container">
  <div class="actions no-print">
    <a href="/families/{familyId}">Back to Family</a>
    <button onclick={() => window.print()}>Print</button>
  </div>

  {#if loading}
    <p>Loading...</p>
  {:else}
    <FamilyGroupSheet data={groupSheet} />
  {/if}
</div>
```

## Step 8: Add link from family detail page

File: `web/src/routes/families/[id]/+page.svelte`

Add a "View Group Sheet" button/link that navigates to the group-sheet subpage.
</implementation>

<output>
Create/modify files:
- `./internal/api/openapi.yaml` - Add group-sheet endpoint and schemas
- `./internal/query/family_service.go` - Add GetGroupSheet method
- `./internal/api/family_handlers.go` - Add handler
- `./web/src/lib/api/client.ts` - Add types and method
- `./web/src/lib/components/FamilyGroupSheet.svelte` - New component
- `./web/src/routes/families/[id]/group-sheet/+page.svelte` - New page
- `./web/src/routes/families/[id]/+page.svelte` - Add link to group sheet
</output>

<verification>
Before declaring complete:
- [ ] Run `go build ./...` - no compilation errors
- [ ] Run `go test ./...` - all tests pass
- [ ] Run `make check-coverage` - verify 85% threshold
- [ ] Test group sheet endpoint returns complete family data
- [ ] Verify component displays all sections correctly
- [ ] Test print preview shows clean, formatted output
- [ ] Verify person name links navigate correctly
</verification>

<success_criteria>
- /families/{id}/group-sheet endpoint returns aggregated family data
- FamilyGroupSheet component displays traditional layout
- All person names are clickable links
- Citations are displayed for documented events
- Print button produces clean, formatted output
- Link from family detail page works
</success_criteria>
