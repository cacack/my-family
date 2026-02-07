# Build stage: Frontend
FROM node:22-alpine AS frontend-builder

WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# Build stage: Backend
FROM golang:1.25.7-alpine AS backend-builder

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
FROM alpine:3.19

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
