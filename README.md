# My Family

[![CI](https://github.com/cacack/my-family/actions/workflows/ci.yml/badge.svg)](https://github.com/cacack/my-family/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/cacack/my-family/graph/badge.svg)](https://codecov.io/gh/cacack/my-family)
[![Go Report Card](https://goreportcard.com/badge/github.com/cacack/my-family)](https://goreportcard.com/report/github.com/cacack/my-family)
[![Go Version](https://img.shields.io/github/go-mod/go-version/cacack/my-family)](https://github.com/cacack/my-family)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](https://opensource.org/licenses/AGPL-3.0)

Self-hosted genealogy software written in Go with an embedded Svelte frontend.

## Features

A genealogy platform designed for research rigor and data ownership.

- **GEDCOM 5.5 import/export** - Full round-trip fidelity with your existing data
- **Flexible date handling** - Supports exact, approximate, ranges, and "before/after"
- **Family relationships** - Biological, adopted, step, and foster qualifiers
- **Interactive pedigree chart** - D3.js visualization with pan/zoom and keyboard navigation
- **Full-text search** - Fast fuzzy matching with keyboard-navigable results
- **Keyboard shortcuts** - Power user navigation (press `?` for help)
- **Accessibility** - High contrast, font scaling, screen reader support
- **API-first design** - Complete REST API with OpenAPI documentation
- **Event sourcing** - Full audit trail of all changes
- **Easy deployment** - Single binary or Docker, SQLite or PostgreSQL

See [FEATURES.md](./FEATURES.md) for the complete list.

## Quick Start

### Using Docker

```bash
# Run with SQLite (default)
docker compose up -d

# Access the application
open http://localhost:8080
```

### Building from Source

Prerequisites:
- Go 1.25+
- Node.js 22+
- SQLite 3.x (for local development)

```bash
# Install dependencies
go mod download
cd web && npm install && cd ..

# Build frontend
cd web && npm run build && cd ..

# Build and run
go build -o myfamily ./cmd/myfamily
./myfamily serve
```

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | (none) | PostgreSQL connection string (uses PostgreSQL if set) |
| `SQLITE_PATH` | `./myfamily.db` | SQLite database path |
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `LOG_FORMAT` | `text` | Log format (text, json) |

## API Endpoints

- `GET /api/v1/persons` - List persons
- `POST /api/v1/persons` - Create person
- `GET /api/v1/persons/{id}` - Get person
- `PUT /api/v1/persons/{id}` - Update person
- `DELETE /api/v1/persons/{id}` - Delete person
- `GET /api/v1/families` - List families
- `POST /api/v1/families` - Create family
- `GET /api/v1/families/{id}` - Get family
- `PUT /api/v1/families/{id}` - Update family
- `DELETE /api/v1/families/{id}` - Delete family
- `POST /api/v1/families/{id}/children` - Add child to family
- `DELETE /api/v1/families/{id}/children/{personId}` - Remove child
- `GET /api/v1/pedigree/{id}` - Get pedigree chart data
- `GET /api/v1/search?q=...` - Search persons
- `POST /api/v1/gedcom/import` - Import GEDCOM file
- `GET /api/v1/gedcom/export` - Export as GEDCOM
- `GET /api/v1/export/tree` - Export complete tree as JSON
- `GET /api/v1/export/persons` - Export persons as JSON or CSV
- `GET /api/v1/export/families` - Export families as JSON or CSV

API documentation: http://localhost:8080/api/v1/docs

## Development

```bash
# Run tests
go test ./...

# Run frontend tests
cd web && npm test

# Format code
go fmt ./...

# Static analysis
go vet ./...
```

## Documentation

- [ETHOS.md](./docs/ETHOS.md) - Project vision, principles, and success factors
- [CONVENTIONS.md](./docs/CONVENTIONS.md) - Code patterns and standards
- [Architecture Decisions](./docs/adr/) - Key technical decisions with rationale
- [FEATURES.md](./FEATURES.md) - Complete feature list
- [CONTRIBUTING.md](./CONTRIBUTING.md) - Development workflow and code standards

## License

AGPL-3.0
