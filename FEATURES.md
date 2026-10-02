# Features

Completed features in my-family genealogy software.

## Data Management

- **GEDCOM 5.5 Import** - Import existing family trees from any GEDCOM-compatible software
  - Ancestry.com: Preserves `_APID` links to Ancestry records
  - FamilySearch: Preserves `_FSFTID` Family Tree identifiers
- **GEDCOM 5.5 Export** - Export your data for backup or use in other tools
- **JSON/CSV Export** - Export persons and families as JSON or CSV with configurable field selection
- **Person Management** - Create, edit, and delete individual records with names, dates, places, gender, and notes
- **Family Management** - Create family units linking partners and children, supporting multiple marriages and single-parent families
- **Flexible Date Formats** - Support for exact dates, approximate dates (circa), date ranges, and bounded dates (before/after)
- **Relationship Types** - Biological, adopted, step, and foster relationship qualifiers for children

## Visualization

- **Pedigree Chart** - Interactive ancestor chart with D3.js, pan/zoom navigation, click-to-navigate, collapsible branches with ancestor counts, supports up to 10 generations
- **Descendancy Chart** - Interactive descendant chart showing children, grandchildren, and beyond with spouse display, pan/zoom navigation, up to 10 generations
- **Ahnentafel Report** - Traditional numbered ancestor list with configurable generations (2-10), print support, and text export
- **Geographic Heat Map** - Interactive world map showing birth and death locations with color-coded markers, zoom/pan, and click-through to person details
- **Person Detail View** - Complete view of individual records with all associated data
- **Family View** - View family units with partners and children

## Research Quality

- **Uncertainty Indicators** - Visual badges showing research confidence (certain/probable/possible/unknown)
- **Confidence Filtering** - Filter person lists by research status
- **Research Status Analytics** - Dashboard showing distribution of confidence levels across the database
- **Data Quality Scores** - Analytics page showing records needing attention with actionable issues
- **Brick Wall Tracker** - Mark research dead ends, add notes, and celebrate breakthroughs with resolution tracking and browse page
- **Discovery Feed** - Prioritized research suggestions on the dashboard identifying missing data, orphaned records, unassessed persons, and quality gaps
- **Citation Template UI** - Browse 25 Evidence Explained citation templates by category, select templates when adding citations for dynamic field rendering with live-formatted preview
- **Evidence Analysis** - Aggregate multiple citations per fact with researcher conclusions, separating what sources say from what you believe (GPS-compliant)
- **Conflict Tracking** - Automatic detection of contradictory evidence across analyses for the same fact, with resolution workflow
- **Research Logs** - Document research activity including repository searched, search description, and outcome (found/not found/inconclusive) for negative evidence tracking
- **Proof Summaries** - Attach written proof arguments for non-obvious conclusions, linking supporting evidence analyses

## Research Branches

- **Research Branches** - Explore an unproven hypothesis on an isolated branch off the main tree; edits on a branch stay invisible to the mainline until you promote them. A branch covers people, families, partner and child links, names, sources, citations, notes, media, associations, evidence analyses, evidence conflicts, research logs and proof summaries, and person merges. Search, browse, the map, the pedigree, descendancy, Ahnentafel, family group sheet and relationship calculator, the history panels and snapshots all follow the active branch; pages that stay mainline-only (quality checks, research suggestions, the global history, brick walls, repositories, exports) say so, and GEDCOM import and rollback are withdrawn while a branch is active
- **Live Overlay** - A branch is a live view over the mainline, not a frozen copy: records it has not changed show the mainline's current data, and each branch shows how far the mainline has moved since it was created
- **Research Record** - Each branch carries the research question it tests, the people and families it is about, the proof summaries that argue it, and an outcome (open, proved, disproved, inconclusive, superseded, abandoned)
- **Close with an Outcome** - Close a branch without merging and record why; its research logs, evidence analyses and proof summaries stay readable, and its research logs (including searches that found nothing) can be copied to the mainline
- **Branch Comparison** - See what a branch changed for every kind of record it can write, alongside what the mainline changed underneath it since the branch forked
- **Conflict Detection** - Classifies genuine divergence (both sides editing the same field, one side deleting what the other edited, colliding creates) and distinguishes it from harmless overlap where the two sides agree
- **Merge with Review** - Promote a branch back to the mainline with a per-entity decision on every conflict, the branch, mainline and fork-point values shown side by side, bulk decisions, and a merge note explaining why. Any record can be left out. The original research keeps its own timestamps in the audit trail, and nothing is ever rewritten
- **Pre-Merge Checks** - Before merging, the review lists every reference the merge would break (with a one-step fix for each), warns about changed facts the branch has not documented with evidence, and shows the validation issues, quality issues and duplicates the branch would introduce
- **Merge Safety Net** - Take a snapshot of the mainline just before merging, then open exactly what the merge changed
- **Resumable Merge** - A merge interrupted partway is flagged in the branch list and on the branch, and can be finished from the UI
- **Merge Record** - A merged branch records when it was merged, the reasoning behind it and every decision the review made; changes a merge brought in are marked in the change history

## History & Snapshots

- **Change History** - Browse every change to the tree, or to one person or family: what changed and when, field by field
- **Snapshots (Tags)** - Mark research milestones ("Pre-DNA results", "After courthouse trip") on the mainline or on a branch, and compare two snapshots or a snapshot with the current state
- **Restore Points and Rollback** - Roll a person or family back to an earlier version from its history panel (the API also covers sources and citations); rollback works on the mainline only

## Data Validation & Cleanup

- **Duplicate Detection** - Find potential duplicate persons with configurable confidence thresholds
- **Person Merge** - Safely merge duplicate records, consolidating data with field-level resolution; on a research branch the merge stays on the branch until the branch is merged
- **Batch Operations** - Merge multiple duplicate pairs or dismiss false positives in bulk
- **Date Validation** - Detect logical inconsistencies (death before birth, child older than parent)
- **Orphan Detection** - Identify records with broken references
- **Quality Reports** - Comprehensive data completeness metrics with coverage percentages

## Search

- **Name Search** - Search names and alternate names, including accented and upper-case names; SQLite and PostgreSQL find the same people for a query, apart from the few differences listed in [ADR-002](./docs/adr/002-dual-database-strategy.md#name-search)
- **Partial Matching** - Find people with partial name searches (`Joh` finds John and Johnson)
- **Fuzzy Matching** - Find spelling variants (`Smyth` finds Smith) with trigram similarity

## API & Architecture

- **REST API** - Complete API for all operations with JSON responses
- **OpenAPI Documentation** - Interactive API docs at `/api/v1/docs`
- **Event Sourcing** - Full audit trail with ACID guarantees
- **Dual Database Support** - SQLite for local/demo use, PostgreSQL for production

## Deployment

- **Single Binary** - Self-contained Go binary with embedded frontend
- **Docker Support** - Multi-stage Dockerfile and docker-compose for easy deployment
- **Automated Dependency Updates** - Dependabot configured for Go and npm dependencies

## Frontend

- **Svelte 5 + Vite** - Modern reactive frontend
- **Tailwind CSS** - Utility-first styling
- **Desktop Layout** - Optimized for desktop browsers (tablet/mobile responsive design tracked in [#21](https://github.com/cacack/my-family/issues/21))

## Keyboard Shortcuts

Power user navigation without touching the mouse:

- **Global Navigation** - `g h` home, `g p` people, `g f` families, `g s` sources
- **Quick Search** - `/` to focus search, arrow keys to navigate results
- **Help Overlay** - `?` shows all available shortcuts
- **Pedigree Chart** - Arrow keys navigate tree, `+`/`-` zoom, `r` reset view
- **Detail Pages** - `e` edit, `s` save, `Escape` cancel

## Accessibility

- **Font Size Controls** - Normal, Large (125%), Larger (150%)
- **High Contrast Mode** - WCAG AA compliant color scheme (4.5:1 ratio)
- **Reduced Motion** - Respects system preference, disables animations
- **Screen Reader Support** - ARIA labels, live regions, landmark navigation
- **Keyboard Navigation** - Skip link, focus traps in modals, full tab navigation
- **Settings Panel** - Accessible from header, persists preferences

---

See [GitHub Issues](https://github.com/cacack/my-family/issues) for planned features.
