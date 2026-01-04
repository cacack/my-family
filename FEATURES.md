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

---

See [GitHub Issues](https://github.com/cacack/my-family/issues) for planned features.
