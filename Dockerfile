# Build stage: Frontend
FROM node:26-alpine@sha256:725aeba2364a9b16beae49e180d83bd597dbd0b15c47f1f28875c290bfd255b9 AS frontend-builder

WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# Build stage: Backend
FROM golang:1.26.4-alpine@sha256:f23e8b227fb4493eabe03bede4d5a32d04092da71962f1fb79b5f7d1e6c2a17f AS backend-builder

# Install build dependencies for CGO (required for SQLite)
RUN apk add --no-cache gcc musl-dev

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

# Build with CGO enabled for SQLite support
ENV CGO_ENABLED=1
RUN go build -o myfamily -ldflags="-s -w" ./cmd/myfamily

# Runtime stage
FROM alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

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
