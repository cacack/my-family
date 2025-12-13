# ADR-004: Single Binary Deployment with Embedded Frontend

**Status:** Accepted
**Date:** 2025-12-07
**Decision Makers:** Chris
**Related Features:** 001-genealogy-mvp

## Context

The my-family platform has two components:

1. **Go backend** - REST API, business logic, persistence
2. **Svelte frontend** - Web UI for interacting with the API

These components need a deployment strategy. The ETHOS.md emphasizes "Easy self-hosting: Docker one-liner, not a 20-step guide" and targets users with varying technical capabilities.

## Decision Drivers

- Self-hosting must be simple for non-technical users
- Minimize moving parts in deployment
- Support both Docker and direct binary execution
- Development workflow should remain comfortable (hot reload)
- Cross-platform builds (Linux, macOS, Windows)

## Considered Options

### Option 1: Separate Frontend Service

**Description:** Frontend built and served independently (nginx, CDN, or Node server). Backend is API-only.

**Pros:**
- Independent scaling of frontend/backend
- Frontend can be served from CDN
- Clear separation of concerns
- Standard microservices pattern

**Cons:**
- Two things to deploy and configure
- CORS configuration required
- More complex Docker Compose
- Harder for non-technical self-hosters

### Option 2: Single Binary with Embedded Frontend

**Description:** Frontend assets compiled into Go binary via `go:embed`. Single executable serves both API and static files.

**Pros:**
- Single artifact to distribute
- No CORS issues (same origin)
- Simpler deployment - one process, one port
- Works without Docker
- Cross-platform with `GOOS`/`GOARCH`

**Cons:**
- Larger binary size (~10-20MB with assets)
- Must rebuild Go binary for frontend changes
- No independent frontend scaling

### Option 3: Sidecar Container

**Description:** Docker Compose with backend container and nginx sidecar serving frontend, sharing a network.

**Pros:**
- Independent builds
- Nginx handles static file caching efficiently
- Familiar pattern for ops teams

**Cons:**
- Requires Docker (no bare binary option)
- Two containers to manage
- More complex compose file
- Network configuration between containers

## Decision

We chose **Option 2: Single Binary with Embedded Frontend** because:

1. **Simplest deployment** - Download binary, run it. No containers, no web servers, no configuration files.

2. **Self-hosting friendly** - Technical barrier is minimal. Works on any platform Go supports.

3. **No CORS complexity** - Frontend and API share the same origin. No preflight requests, no configuration.

4. **Docker still works** - Single binary embeds cleanly in minimal Docker image (scratch/alpine).

5. **Genealogy scale** - We're not serving millions of users. Independent scaling is not a requirement.

The build overhead (frontend change requires Go rebuild) is acceptable given the deployment benefits.

## Consequences

### Positive

- Single artifact distribution (binary or Docker image)
- Zero-config deployment for users
- No CORS setup required
- Smaller attack surface (one process, one port)
- Consistent versioning (frontend and backend always match)

### Negative

- Binary size increases (~10-20MB for frontend assets)
- Mitigation: Still small by modern standards; UPX compression available if needed
- Frontend changes require Go rebuild
- Mitigation: Development mode serves frontend separately with hot reload
- Cannot independently update frontend without new release
- Mitigation: Acceptable for self-hosted software; users update whole package

### Neutral

- `go:embed` directive bundles assets at compile time
- Makefile orchestrates frontend build → Go build

## Implementation Notes

### Build Process

```makefile
# Makefile
.PHONY: build

build: build-frontend build-backend

build-frontend:
	cd web && npm run build

build-backend: build-frontend
	go build -o myfamily ./cmd/myfamily
```

### Embedding

```go
// internal/web/embed.go
//go:build !dev

package web

import "embed"

//go:embed all:dist
var Assets embed.FS
```

```go
// internal/web/embed_dev.go
//go:build dev

package web

import "os"

// Development mode - serve from filesystem for hot reload
var Assets = os.DirFS("web/dist")
```

### Server Integration

```go
// internal/api/server.go
func SetupRoutes(e *echo.Echo) {
    // API routes
    api := e.Group("/api")
    // ... register handlers ...

    // Serve embedded frontend
    e.GET("/*", echo.WrapHandler(
        http.FileServer(http.FS(web.Assets)),
    ))
}
```

### Development Workflow

```bash
# Terminal 1: Frontend with hot reload
cd web && npm run dev

# Terminal 2: Backend (API only)
go run -tags dev ./cmd/myfamily

# Frontend dev server proxies /api to backend
```

### Docker Build

```dockerfile
# Multi-stage build
FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.22-alpine AS backend
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN go build -o myfamily ./cmd/myfamily

FROM alpine:3.19
COPY --from=backend /app/myfamily /usr/local/bin/
ENTRYPOINT ["myfamily"]
```

## References

- [ETHOS.md - Easy self-hosting](../ETHOS.md)
- [Go embed directive](https://pkg.go.dev/embed)
- [plan.md - Project Structure](../001-genealogy-mvp/plan.md)
