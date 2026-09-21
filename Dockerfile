# Build stage: Frontend
# Held at 24 (Active LTS) to match .nvmrc and CI. Node 25+ enables the Web
# Storage API by default, whose globals shadow jsdom's in the test suite
# (vitest-dev/vitest#8757, still open). Keeping every Node surface on one
# version is the point; revisit when that lands upstream.
FROM node:25-alpine@sha256:bdf2cca6fe3dabd014ea60163eca3f0f7015fbd5c7ee1b0e9ccb4ced6eb02ef4 AS frontend-builder

WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# Build stage: Backend
FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS backend-builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Remove local replace directive for Docker build (uses published version)
RUN sed -i '/^replace.*=> \//d' go.mod && go mod download

# Copy source code
COPY . .

# Remove local replace directive from copied source
RUN sed -i '/^replace.*=> \//d' go.mod

# Copy built frontend into internal/web/dist for embedding
COPY --from=frontend-builder /app/web/build ./internal/web/dist

# The SQLite driver is pure Go, so the image builds without cgo, exactly like
# the release archives (.goreleaser.yaml); see ADR-002.
ENV CGO_ENABLED=0
RUN go build -o myfamily -ldflags="-s -w" ./cmd/myfamily

# Runtime stage
FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=backend-builder /app/myfamily .

# Create data directory for SQLite
RUN mkdir -p /data

# Set environment variables
ENV PORT=8080
ENV SQLITE_PATH=/data/myfamily.db
ENV LOG_FORMAT=json

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/api/v1/persons || exit 1

# Run the application
CMD ["./myfamily", "serve"]
