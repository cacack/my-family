<objective>
Implement uncertain data markers for GitHub issue #32.

Add the ability to mark individual facts as uncertain with visual indicators,
enabling users to distinguish between confirmed facts and speculative data.
This is critical for GPS-compliant genealogy research.
</objective>

<context>
Project: my-family - Self-hosted genealogy software (Go backend, Svelte frontend)
Issue: #32 - Uncertain data markers
Labels: priority:high, scope:core, area:research, value:high, effort:low

Read @CLAUDE.md for project conventions.

Key existing patterns:
- Domain entities in `internal/domain/` with validation
- Enums defined in `internal/domain/enums.go`
- OpenAPI spec at `internal/api/openapi.yaml`
- Svelte components in `web/src/lib/components/`
- Existing quality display: `QualityScore.svelte` for visual indicators
</context>

<requirements>
From issue #32 acceptance criteria:
- [ ] Can mark any fact as uncertain
- [ ] Uncertain data visually distinct
- [ ] Can filter to show only uncertain data
- [ ] Uncertainty survives export/import

Research status levels to implement:
- `certain` - Confirmed with strong evidence
- `probable` - Likely correct, good supporting evidence
- `possible` - Speculative, limited evidence
- `unknown` - Not yet assessed (default)
</requirements>

<implementation>
## Step 1: Add ResearchStatus enum

File: `internal/domain/enums.go`
- Add `ResearchStatus` type with constants: Certain, Probable, Possible, Unknown
- Add String() method and ParseResearchStatus() function
- Follow existing enum patterns in the file

## Step 2: Extend Person entity

File: `internal/domain/person.go`
- Add `ResearchStatus ResearchStatus` field
- Default to `Unknown` in validation/creation
- No breaking changes to existing API

## Step 3: Extend LifeEvent entity

File: `internal/domain/lifeevent.go`
- Add `ResearchStatus ResearchStatus` field
- Default to `Unknown`

## Step 4: Update OpenAPI schema

File: `internal/api/openapi.yaml`
- Add `research_status` enum schema
- Add field to Person, PersonResponse, CreatePersonRequest, UpdatePersonRequest
- Add field to LifeEvent schemas

## Step 5: Update read model

File: `internal/repository/readmodel.go`
- Add `ResearchStatus` to `PersonReadModel`
- Update projection logic

## Step 6: Update API handlers

File: `internal/api/handlers.go`
- Handle `research_status` in person create/update
- Include in responses

## Step 7: Create UncertaintyBadge component

File: `web/src/lib/components/UncertaintyBadge.svelte`
- Props: `status: 'certain' | 'probable' | 'possible' | 'unknown'`
- Visual design:
  - certain: green checkmark or solid indicator
  - probable: yellow/amber indicator
  - possible: orange question mark
  - unknown: gray dash or empty
- Compact badge suitable for inline display
- Tooltip explaining the status

## Step 8: Update API client types

File: `web/src/lib/api/client.ts`
- Add `ResearchStatus` type
- Add field to Person interface

## Step 9: Display in person detail view

File: `web/src/routes/persons/[id]/+page.svelte`
- Show UncertaintyBadge next to person name
- Show badges for uncertain facts/events
</implementation>

<output>
Create/modify files:
- `./internal/domain/enums.go` - Add ResearchStatus enum
- `./internal/domain/person.go` - Add research_status field
- `./internal/domain/lifeevent.go` - Add research_status field
- `./internal/api/openapi.yaml` - Add schemas and fields
- `./internal/repository/readmodel.go` - Update read model
- `./internal/api/handlers.go` - Update handlers
- `./web/src/lib/components/UncertaintyBadge.svelte` - New component
- `./web/src/lib/api/client.ts` - Update types
- `./web/src/routes/persons/[id]/+page.svelte` - Display badges
</output>

<verification>
Before declaring complete:
- [ ] Run `go build ./...` - no compilation errors
- [ ] Run `go test ./...` - all tests pass
- [ ] Run `make check-coverage` - verify 85% threshold
- [ ] Verify OpenAPI spec is valid (no schema errors)
- [ ] Test UncertaintyBadge renders all 4 states correctly
</verification>

<success_criteria>
- ResearchStatus enum works with String() and Parse functions
- Person and LifeEvent entities accept research_status field
- API accepts and returns research_status in person operations
- UncertaintyBadge displays distinct visual for each status level
- Person detail page shows uncertainty indicators
</success_criteria>
