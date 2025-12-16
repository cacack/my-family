# My Family

[![CI](https://github.com/cacack/my-family/actions/workflows/ci.yml/badge.svg)](https://github.com/cacack/my-family/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/cacack/my-family/graph/badge.svg)](https://codecov.io/gh/cacack/my-family)
[![Go Report Card](https://goreportcard.com/badge/github.com/cacack/my-family)](https://goreportcard.com/report/github.com/cacack/my-family)
[![Go Version](https://img.shields.io/github/go-mod/go-version/cacack/my-family)](https://github.com/cacack/my-family)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](https://opensource.org/licenses/AGPL-3.0)

Self-hosted genealogy software written in Go with an embedded Svelte frontend.

## Features

- GEDCOM 5.5 import/export with full round-trip fidelity
- Person and family management via REST API
- Interactive pedigree chart visualization
- Full-text search with fuzzy matching
- Single binary deployment with embedded web UI
- Supports both SQLite (default) and PostgreSQL

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
- Go 1.22+
- Node.js 20+
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

API documentation: http://localhost:8080/api/docs

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

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for the feature development workflow and code standards.

## License

MIT
