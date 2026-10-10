# Review UI Cohesion

Experience the running app in a real browser and find layout, navigation, and consistency defects. Fix what is local; card what is structural.

---

## Context

Scope: $ARGUMENTS (a route, a route group, or "all"; default "all")

## Setup

1. `make binary`, then start on a throwaway DB and a non-default port so no real data is touched:
   `SQLITE_PATH=<scratch>/ui.db PORT=8282 ./myfamily serve`
2. Seed realistic data: `curl -F file=@testdata/gedcom-5.5/royal92.ged http://127.0.0.1:8282/api/v1/gedcom/import` (long names, deep trees, 3k people). Also check at least one route against an **empty** DB.
3. Enumerate routes from `web/src/routes/` — include detail pages (`/persons/<id>`, `/families/<id>`, …) using real ids from the API.

## Matrix

For every route in scope: widths **375 / 768 / 1280 / 1440**, plus data states **empty / typical / extreme** where the page lists data.

## Measure (don't just eyeball)

Script it with Playwright (`browser_run_code_unsafe` or a scratch script) and record per route × width:

- Page horizontal overflow: `document.documentElement.scrollWidth > innerWidth`
- Elements extending past the viewport's right edge (top offenders, with selector + overflow px)
- Header height (wrapping/growth) and whether every nav item is visible and reachable
- Console errors and failed network requests
- A full-page screenshot — then **look at it**

## Interact

- Open every menu and dropdown; click the item's whole row, not just its text
- Keyboard: Tab through the header, Enter/Escape on menus, global shortcuts (`?` for help)
- Back/forward, deep-link reload, active-nav highlighting on nested routes
- Visualisations (pedigree, descendancy, map): pan/zoom, fit to container, resize

## Cohesion

Compare across pages: page header/title pattern, container width and padding, empty/loading/error states, button styles (shadcn `Button` vs ad-hoc), table vs card lists, hard-coded colors vs theme tokens.

## Triage

| Bucket | Rule | Action |
|--------|------|--------|
| **Fix now** | Local CSS/markup change in 1-2 files | Fix in themed PRs (not per page); re-measure to verify |
| **Card up** | Changes page behavior or structure (nav redesign, mobile pattern) | Issue with observable acceptance criteria + screenshot; link #21 for responsive work |
| **Convention** | Same thing done N ways | Note for a UI section in `docs/CONVENTIONS.md` |

### Writing "card up" issues

Don't just card the symptom's patch — ask whether the UI's *structure* still fits the feature set. Features accrete; a layout sized for five pages breaks at fifteen. For each structural card, consider and record:

- **Information architecture**: should items be regrouped, nested (menus, sub-nav, tabs), demoted (overflow/"More"), or moved closer to where they're used (e.g. per-person actions on the person page, not global nav)?
- **Growth**: where does the *next* feature in this area go? Prefer a structure with an obvious slot for it over one that needs re-laying-out again.
- **Options**: sketch 2-3 alternatives (ASCII or screenshot) with trade-offs, and a recommendation.
- **Acceptance criteria**: observable — e.g. "every primary destination reachable at 375px in ≤2 interactions; no horizontal overflow at 375/768/1280/1440".

## Output

A findings table — route, width, symptom, measured evidence, root cause (file:line), bucket — grouped by root cause, since many symptoms share one. Lock fixed defects in with `web/e2e/layout.spec.ts` (no horizontal overflow, no console errors per route × width).
