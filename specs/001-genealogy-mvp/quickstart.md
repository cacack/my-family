# Quickstart: My Family Genealogy MVP

**Branch**: `001-genealogy-mvp` | **Date**: 2025-12-07

## Prerequisites

- Go 1.22+
- Node.js 20+ (for frontend development)
- Docker (optional, for PostgreSQL)
- SQLite 3.x (included in most systems)

## Project Setup

### 1. Clone and Initialize

```bash
git clone <repository-url>
cd my-family
git checkout 001-genealogy-mvp
```

### 2. Install Go Dependencies

```bash
go mod download
```

### 3. Install Frontend Dependencies

```bash
cd web
npm install
cd ..
```

## Development Workflow

### Running with SQLite (Default)

```bash
# Build and run
go run ./cmd/myfamily serve

# Server starts at http://localhost:8080
# API at http://localhost:8080/api/v1
# Swagger UI at http://localhost:8080/api/docs
```

SQLite database will be created at `./myfamily.db`.

### Running with PostgreSQL

```bash
# Start PostgreSQL with Docker
docker compose up -d postgres

# Run with PostgreSQL
DATABASE_URL="postgres://myfamily:myfamily@localhost:5432/myfamily?sslmode=disable" \
  go run ./cmd/myfamily serve
```

### Frontend Development

```bash
# Terminal 1: Run backend
go run ./cmd/myfamily serve

# Terminal 2: Run frontend dev server (with hot reload)
cd web
npm run dev
```

Frontend dev server runs at `http://localhost:5173` with API proxy to `:8080`.

## Build Commands

```bash
# Build all Go packages
go build ./...

# Run all tests
go test ./...

# Run specific test
go test -v ./... -run TestName

# Format code
go fmt ./...

# Static analysis
go vet ./...

# Generate code from OpenAPI spec
go generate ./...

# Build frontend for production
cd web && npm run build

# Build single binary (includes embedded frontend)
go build -o myfamily ./cmd/myfamily
```

## Project Structure

```
my-family/
├── cmd/myfamily/           # Application entry point
├── internal/
│   ├── domain/             # Pure domain types (Person, Family, events)
│   ├── command/            # Command handlers (CQRS write side)
│   ├── query/              # Query services (CQRS read side)
│   ├── repository/         # Event store and read model persistence
│   ├── api/                # HTTP handlers and OpenAPI server
│   ├── gedcom/             # GEDCOM import/export
│   └── config/             # Configuration
├── web/                    # Svelte frontend
│   ├── src/
│   │   ├── lib/components/ # Reusable components
│   │   └── routes/         # SvelteKit pages
│   └── tests/
├── specs/                  # Feature specifications
└── docker-compose.yml
```

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | (none) | PostgreSQL connection string |
| `SQLITE_PATH` | `./myfamily.db` | SQLite database path |
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `LOG_FORMAT` | `text` | Log format (text, json) |

If `DATABASE_URL` is not set, the application uses SQLite.

## Testing

### Go Tests

```bash
# Unit tests (fast, no external deps)
go test ./internal/domain/... ./internal/command/...

# Integration tests (requires Docker for testcontainers)
go test ./internal/repository/... ./internal/api/...

# All tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Frontend Tests

```bash
cd web
npm run test        # Run tests
npm run test:watch  # Watch mode
npm run test:coverage
```

## API Quick Reference

### Persons

```bash
# List persons
curl http://localhost:8080/api/v1/persons

# Create person
curl -X POST http://localhost:8080/api/v1/persons \
  -H "Content-Type: application/json" \
  -d '{"given_name": "John", "surname": "Doe", "birth_date": "1 JAN 1850"}'

# Get person
curl http://localhost:8080/api/v1/persons/{id}

# Update person
curl -X PUT http://localhost:8080/api/v1/persons/{id} \
  -H "Content-Type: application/json" \
  -d '{"given_name": "John", "surname": "Smith", "version": 1}'

# Delete person
curl -X DELETE http://localhost:8080/api/v1/persons/{id}
```

### Families

```bash
# Create family
curl -X POST http://localhost:8080/api/v1/families \
  -H "Content-Type: application/json" \
  -d '{"partner1_id": "uuid", "partner2_id": "uuid", "relationship_type": "marriage"}'

# Add child to family
curl -X POST http://localhost:8080/api/v1/families/{id}/children \
  -H "Content-Type: application/json" \
  -d '{"person_id": "uuid", "relationship_type": "biological"}'
```

### Search

```bash
# Search by name
curl "http://localhost:8080/api/v1/search?q=John"

# Fuzzy search
curl "http://localhost:8080/api/v1/search?q=Katherine&fuzzy=true"
```

### Pedigree

```bash
# Get 4-generation pedigree
curl http://localhost:8080/api/v1/pedigree/{person_id}

# Get 6-generation pedigree
curl "http://localhost:8080/api/v1/pedigree/{person_id}?generations=6"
```

### GEDCOM

```bash
# Import GEDCOM file
curl -X POST http://localhost:8080/api/v1/gedcom/import \
  -F "file=@/path/to/family.ged"

# Export to GEDCOM
curl http://localhost:8080/api/v1/gedcom/export -o export.ged
```

## Docker Deployment

### Build Docker Image

```bash
docker build -t myfamily .
```

### Run with SQLite

```bash
docker run -p 8080:8080 -v $(pwd)/data:/data \
  -e SQLITE_PATH=/data/myfamily.db \
  myfamily
```

### Run with Docker Compose (PostgreSQL)

```bash
docker compose up -d
```

Access at http://localhost:8080

## Troubleshooting

### "database is locked" (SQLite)

SQLite doesn't handle concurrent writes well. This shouldn't occur in single-user mode but if it does:
- Ensure only one instance of the application is running
- Consider switching to PostgreSQL for development

### "connection refused" (PostgreSQL)

- Verify PostgreSQL is running: `docker compose ps`
- Check connection string format
- Ensure database exists: `docker compose exec postgres psql -U myfamily -c '\l'`

### Frontend not loading

- Check if backend is running on port 8080
- In development, ensure Vite dev server is using correct proxy
- Check browser console for CORS errors

### GEDCOM import fails

- Check file encoding (should be UTF-8 or ANSEL)
- Review import result for specific warnings/errors
- Test with smaller file to isolate issues
