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

- **Pedigree Chart** - Interactive ancestor chart with D3.js, pan/zoom navigation, click-to-navigate
- **Ahnentafel Report** - Traditional numbered ancestor list with configurable generations (2-10), print support, and text export
- **Person Detail View** - Complete view of individual records with all associated data
- **Family View** - View family units with partners and children

## Search

- **Full-Text Search** - Fast name search with FTS5 (SQLite) or tsvector (PostgreSQL)
- **Partial Matching** - Find people with partial name searches

## API & Architecture

- **REST API** - Complete API for all operations with JSON responses
- **OpenAPI Documentation** - Interactive API docs at `/api/docs`
- **Event Sourcing** - Full audit trail with ACID guarantees
- **Dual Database Support** - SQLite for local/demo use, PostgreSQL for production

## Deployment

- **Single Binary** - Self-contained Go binary with embedded frontend
- **Docker Support** - Multi-stage Dockerfile and docker-compose for easy deployment
- **Automated Dependency Updates** - Dependabot configured for Go and npm dependencies

## Frontend

- **Svelte 5 + Vite** - Modern reactive frontend
- **Tailwind CSS** - Utility-first styling
- **Responsive Layout** - Works on desktop browsers

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
