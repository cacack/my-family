.PHONY: build test fmt vet generate clean run dev

# Build all Go packages
build:
	go build ./...

# Build the binary with embedded frontend
binary: frontend
	rm -rf internal/web/dist/*
	cp -r web/build/* internal/web/dist/
	go build -o myfamily ./cmd/myfamily

# Build frontend only
frontend:
	cd web && npm run build

# Run all tests
test:
	go test ./...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	go fmt ./...

# Static analysis
vet:
	go vet ./...

# Generate code (OpenAPI, etc.)
generate:
	go generate ./...

# Clean build artifacts
clean:
	rm -f myfamily coverage.out coverage.html
	rm -rf web/dist web/build web/.svelte-kit internal/web/dist

# Run the server (development)
run:
	go run ./cmd/myfamily serve

# Run frontend dev server
dev-frontend:
	cd web && npm run dev

# Install dependencies
deps:
	go mod download
	cd web && npm install

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# All checks
check: fmt vet lint test
