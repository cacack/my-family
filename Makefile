.PHONY: build test fmt vet generate clean run dev setup check-coverage

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

# Run all tests (Go + frontend)
test:
	go test ./...
	cd web && npm test -- --run

# Run only Go tests
test-go:
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

# All checks (CI validation)
check: fmt vet test
	cd web && npm run check

# Full CI check including lint (requires golangci-lint)
check-full: fmt vet lint test
	cd web && npm run check

# Check coverage thresholds (same as CI)
check-coverage:
	go test -coverprofile=coverage.out ./...
	go-test-coverage --config=.testcoverage.yml --profile=coverage.out

# Setup development environment
setup: deps
	go install github.com/vladopajic/go-test-coverage/v2@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
	ln -sf ../../scripts/pre-push .git/hooks/pre-push
	@echo "✓ Development environment ready"
